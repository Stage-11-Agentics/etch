// Package importer implements Etch's post-hoc ingestion path: it reads the
// session transcripts that agent runtimes already write to disk, parses them
// into capture.Session records, and commits them through the same boundary the
// live hook path uses. This is the universal floor described in
// docs/INGESTION.md — any runtime that persists a session log can be imported,
// including ones with no usable live hook surface.
//
// Two invariants make import safe alongside live capture:
//
//   - Hooks always win. Before committing, import skips any session whose
//     upstream id (agent_session_id) already has a committed record. Re-running
//     import is therefore idempotent: the second run sees the first run's
//     records and skips them.
//   - One engine. Parsers only build a *capture.Session; redaction, trace
//     generation, and the create-only ref write all happen in the shared
//     commit boundary (hooks.CommitImported → commitRecord).
package importer

import (
	"crypto/rand"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Stage-11-Agentics/etch/internal/capture"
	"github.com/Stage-11-Agentics/etch/internal/config"
	"github.com/Stage-11-Agentics/etch/internal/hooks"
	"github.com/oklog/ulid/v2"
)

// Parsed is a transcript parsed into a session plus the metadata the importer
// needs to attribute and dedup it. Cwd is the working directory recorded in the
// transcript, used to decide which repo a session belongs to.
type Parsed struct {
	Session *capture.Session
	Cwd     string
}

// Parser turns one runtime's on-disk transcripts into sessions. Implementations
// are pure readers: they never touch git or commit anything.
type Parser interface {
	// Runtime is the agent.runtime value this parser produces (e.g.
	// "claude-code"). Also the --runtime selector value.
	Runtime() string
	// Discover returns every candidate transcript file for this runtime on this
	// machine, irrespective of repo. The core filters by recorded cwd.
	Discover() ([]string, error)
	// Parse reads one transcript into a Parsed record. It returns (nil, nil)
	// for a file that is not a usable session (empty, malformed, no id) so a
	// single bad transcript never aborts a bulk import.
	Parse(path string) (*Parsed, error)
}

// registry is the set of known parsers, keyed by runtime.
var registry = map[string]Parser{
	(&ClaudeParser{}).Runtime(): &ClaudeParser{},
	(&CodexParser{}).Runtime():  &CodexParser{},
}

// Options controls an import run.
type Options struct {
	RepoRoot string    // git repo to attribute and commit sessions into
	Runtime  string    // "" = all known runtimes; otherwise a single runtime
	Since    time.Time // zero = no lower bound; sessions started before are skipped
	DryRun   bool      // report what would be imported without writing refs
}

// Result is the outcome of an import run.
type Result struct {
	Imported  int      // sessions committed (or that would be, under DryRun)
	Skipped   int      // sessions skipped because a record already existed
	OutOfRepo int      // transcripts whose cwd was outside RepoRoot
	Failed    int      // transcripts that failed to parse or commit
	Runtimes  []string // runtimes that ran
}

// Run executes an import against opts.RepoRoot, writing progress to w, using
// the registered parsers selected by opts.Runtime.
func Run(opts Options, w io.Writer) (Result, error) {
	parsers, err := selectParsers(opts.Runtime)
	if err != nil {
		return Result{}, err
	}
	return runImport(opts, parsers, w)
}

// runImport is Run with an explicit parser set, the seam tests use to inject
// parsers pointed at synthetic transcript dirs.
func runImport(opts Options, parsers []Parser, w io.Writer) (Result, error) {
	var res Result

	existing, err := existingAgentSessionIDs(opts.RepoRoot)
	if err != nil {
		return res, fmt.Errorf("reading existing session refs: %w", err)
	}

	settings, _ := config.Load(opts.RepoRoot)
	salt, _ := config.EnsureHostnameSalt(opts.RepoRoot)

	// Attribution compares the transcript's recorded cwd against the repo root.
	// Resolve symlinks on the repo side so a session whose cwd was under e.g.
	// /tmp (a symlink to /private/tmp on macOS) still matches the canonical git
	// toplevel. Each cwd is canonicalized the same way at compare time.
	canonRepo := canonicalize(opts.RepoRoot)
	repoPrefix := strings.TrimRight(canonRepo, "/") + "/"

	for _, p := range parsers {
		res.Runtimes = append(res.Runtimes, p.Runtime())
		files, err := p.Discover()
		if err != nil {
			fmt.Fprintf(w, "warning: %s discovery failed: %v\n", p.Runtime(), err)
			continue
		}
		for _, f := range files {
			parsed, err := p.Parse(f)
			if err != nil {
				res.Failed++
				fmt.Fprintf(w, "warning: skipping %s: %v\n", f, err)
				continue
			}
			if parsed == nil || parsed.Session == nil {
				continue // not a usable session
			}
			s := parsed.Session

			// Repo attribution: keep sessions whose recorded cwd is at or under
			// the target repo root (both sides symlink-resolved).
			cwd := canonicalize(parsed.Cwd)
			if cwd != canonRepo && !strings.HasPrefix(cwd, repoPrefix) {
				res.OutOfRepo++
				continue
			}

			// --since lower bound on start time.
			if !opts.Since.IsZero() && s.Timing.StartedAt != "" {
				if t, err := time.Parse(time.RFC3339Nano, s.Timing.StartedAt); err == nil && t.Before(opts.Since) {
					continue
				}
			}

			// Dedup: hooks (and prior imports) win.
			if s.AgentSessionID != nil && existing[*s.AgentSessionID] {
				res.Skipped++
				continue
			}

			// Fill machine/operator identity from THIS machine — a transcript on
			// disk here means the session ran here. Git/timing/tooling come from
			// the parser.
			s.Machine = capture.CaptureMachine(settings, salt)
			s.Operator = capture.CaptureOperator(opts.RepoRoot)

			if opts.DryRun {
				res.Imported++
				fmt.Fprintf(w, "would import %s (%s, %s)\n", deref(s.AgentSessionID), s.Agent.Runtime, s.SessionID)
				continue
			}

			if err := hooks.CommitImported(opts.RepoRoot, s); err != nil {
				res.Failed++
				fmt.Fprintf(w, "warning: failed to commit %s: %v\n", deref(s.AgentSessionID), err)
				continue
			}
			// Guard against two transcripts for the same upstream id in one run.
			if s.AgentSessionID != nil {
				existing[*s.AgentSessionID] = true
			}
			res.Imported++
		}
	}

	return res, nil
}

func selectParsers(runtime string) ([]Parser, error) {
	if runtime == "" {
		out := make([]Parser, 0, len(registry))
		for _, p := range registry {
			out = append(out, p)
		}
		return out, nil
	}
	p, ok := registry[runtime]
	if !ok {
		known := make([]string, 0, len(registry))
		for k := range registry {
			known = append(known, k)
		}
		return nil, fmt.Errorf("unknown runtime %q (known: %s)", runtime, strings.Join(known, ", "))
	}
	return []Parser{p}, nil
}

// mintULID produces a ULID whose timestamp component is the session's start
// time, so imported refs sort chronologically alongside hook-captured ones.
func mintULID(start time.Time) string {
	if start.IsZero() {
		start = time.Now().UTC()
	}
	return ulid.MustNew(ulid.Timestamp(start), rand.Reader).String()
}

// existingAgentSessionIDs returns the set of upstream session ids already
// recorded in this repo, across both the canonical and local ref namespaces.
// This is the dedup key: import skips any upstream id already present.
func existingAgentSessionIDs(repoRoot string) (map[string]bool, error) {
	set := map[string]bool{}
	for _, prefix := range []string{"refs/etch/sessions/", "refs/etch/local/"} {
		refs, err := listRefs(repoRoot, prefix)
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			id, ok := agentSessionIDOfRef(repoRoot, ref)
			if ok && id != "" {
				set[id] = true
			}
		}
	}
	return set, nil
}

func listRefs(repoRoot, prefix string) ([]string, error) {
	out, err := runGit(repoRoot, "for-each-ref", prefix, "--format=%(refname)")
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil, nil
	}
	return strings.Split(trimmed, "\n"), nil
}

func agentSessionIDOfRef(repoRoot, ref string) (string, bool) {
	out, err := runGit(repoRoot, "show", ref+":session.json")
	if err != nil {
		return "", false
	}
	id, err := extractAgentSessionID([]byte(out))
	if err != nil {
		return "", false
	}
	return id, true
}

func runGit(repoRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// ResolveRepoRoot returns the git worktree root for dir.
func ResolveRepoRoot(dir string) (string, error) {
	out, err := runGit(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not a git repository (%s): %w", dir, err)
	}
	root := strings.TrimSpace(out)
	if root == "" {
		return "", fmt.Errorf("could not resolve repo root for %s", dir)
	}
	return filepath.Clean(root), nil
}

// canonicalize resolves symlinks in a path, falling back to a cleaned path
// when the target does not exist (e.g. a transcript's cwd whose directory has
// since been removed).
func canonicalize(p string) string {
	if p == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return filepath.Clean(p)
}

func deref(s *string) string {
	if s == nil {
		return "<none>"
	}
	return *s
}
