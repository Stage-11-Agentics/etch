// Package archive implements ref lifecycle compaction: old per-session refs
// (refs/etch/sessions/<ULID>) are merged into quarterly archive refs
// (refs/etch/archive/<YYYY-Q>) and the individual session refs are deleted.
package archive

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	sessionsPrefix = "refs/etch/sessions/"
	archivePrefix  = "refs/etch/archive/"
)

// Options controls an archive run.
type Options struct {
	RepoRoot      string
	ThresholdDays int
	Now           time.Time // injected "current time" so callers/tests control the cutoff
	DryRun        bool
	Since         *time.Time // optional lower bound on commit time (inclusive)
	Until         *time.Time // optional upper bound on commit time (inclusive)
	Quarter       string     // optional "YYYY-Qn" filter
}

// SessionEntry is a single session ref selected for archival.
type SessionEntry struct {
	ULID       string
	Ref        string
	CommitSHA  string
	CommitTime time.Time
}

// QuarterPlan groups the sessions destined for one archive ref.
type QuarterPlan struct {
	Label    string
	Sessions []SessionEntry
}

// Plan is the full set of work an archive run will perform.
type Plan struct {
	Quarters []QuarterPlan
}

// SessionCount returns the total number of sessions across all quarters.
func (p Plan) SessionCount() int {
	n := 0
	for _, q := range p.Quarters {
		n += len(q.Sessions)
	}
	return n
}

// Empty reports whether the plan has no sessions to archive.
func (p Plan) Empty() bool {
	return p.SessionCount() == 0
}

// quarterLabel returns the "YYYY-Qn" label for t in UTC.
func quarterLabel(t time.Time) string {
	t = t.UTC()
	q := (int(t.Month())-1)/3 + 1
	return fmt.Sprintf("%d-Q%d", t.Year(), q)
}

// BuildPlan enumerates session refs older than the threshold and groups them by quarter.
func BuildPlan(opts Options) (Plan, error) {
	out, err := runGit(opts.RepoRoot, nil,
		"for-each-ref",
		"--format=%(refname) %(objectname) %(committerdate:unix)",
		sessionsPrefix)
	if err != nil {
		return Plan{}, fmt.Errorf("for-each-ref: %w", err)
	}

	cutoff := opts.Now.UTC().AddDate(0, 0, -opts.ThresholdDays)

	byQuarter := map[string][]SessionEntry{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return Plan{}, fmt.Errorf("unexpected for-each-ref line: %q", line)
		}
		ref, sha, tsStr := fields[0], fields[1], fields[2]
		ts, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			return Plan{}, fmt.Errorf("parsing commit time %q: %w", tsStr, err)
		}
		commitTime := time.Unix(ts, 0).UTC()

		if !commitTime.Before(cutoff) {
			continue // too recent
		}
		if opts.Since != nil && commitTime.Before(opts.Since.UTC()) {
			continue
		}
		if opts.Until != nil && commitTime.After(opts.Until.UTC()) {
			continue
		}
		label := quarterLabel(commitTime)
		if opts.Quarter != "" && opts.Quarter != label {
			continue
		}

		ulid := strings.TrimPrefix(ref, sessionsPrefix)
		byQuarter[label] = append(byQuarter[label], SessionEntry{
			ULID:       ulid,
			Ref:        ref,
			CommitSHA:  sha,
			CommitTime: commitTime,
		})
	}

	labels := make([]string, 0, len(byQuarter))
	for l := range byQuarter {
		labels = append(labels, l)
	}
	sort.Strings(labels)

	plan := Plan{}
	for _, l := range labels {
		entries := byQuarter[l]
		sort.Slice(entries, func(i, j int) bool { return entries[i].ULID < entries[j].ULID })
		plan.Quarters = append(plan.Quarters, QuarterPlan{Label: l, Sessions: entries})
	}
	return plan, nil
}

// Archive executes the archival: it builds (or extends) one archive ref per quarter
// and deletes the individual session refs. Returns the executed plan.
func Archive(opts Options) (Plan, error) {
	plan, err := BuildPlan(opts)
	if err != nil {
		return Plan{}, err
	}
	for _, q := range plan.Quarters {
		if err := archiveQuarter(opts, q); err != nil {
			return Plan{}, err
		}
	}
	return plan, nil
}

func archiveQuarter(opts Options, q QuarterPlan) error {
	archiveRef := archivePrefix + q.Label

	// Seed top-level tree entries from the existing archive ref (incremental).
	// entries maps ULID -> top-level tree line (mode tree <sha>\t<ULID>).
	entries := map[string]string{}
	parentSHA := ""
	if existing, ok := refExists(opts.RepoRoot, archiveRef); ok {
		parentSHA = existing
		lines, err := runGit(opts.RepoRoot, nil, "ls-tree", archiveRef)
		if err != nil {
			return fmt.Errorf("ls-tree %s: %w", archiveRef, err)
		}
		for _, line := range strings.Split(strings.TrimSpace(lines), "\n") {
			if line == "" {
				continue
			}
			// "040000 tree <sha>\t<ULID>"
			tab := strings.IndexByte(line, '\t')
			if tab < 0 {
				continue
			}
			name := line[tab+1:]
			entries[name] = line
		}
	}

	// Build a subtree for each session and add/overwrite its top-level entry.
	for _, s := range q.Sessions {
		subtreeSHA, err := buildSessionSubtree(opts.RepoRoot, s.Ref)
		if err != nil {
			return fmt.Errorf("building subtree for %s: %w", s.ULID, err)
		}
		entries[s.ULID] = fmt.Sprintf("040000 tree %s\t%s", subtreeSHA, s.ULID)
	}

	// mktree the top-level tree from sorted entries.
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	sort.Strings(names)
	var treeInput bytes.Buffer
	for _, n := range names {
		treeInput.WriteString(entries[n])
		treeInput.WriteByte('\n')
	}
	treeSHA, err := runGit(opts.RepoRoot, treeInput.Bytes(), "mktree")
	if err != nil {
		return fmt.Errorf("mktree archive tree for %s: %w", q.Label, err)
	}

	// commit-tree, accreting onto any prior archive commit.
	msg := fmt.Sprintf("etch archive %s\nsessions: %d", q.Label, len(q.Sessions))
	commitArgs := []string{"commit-tree", treeSHA, "-m", msg}
	if parentSHA != "" {
		commitArgs = append(commitArgs, "-p", parentSHA)
	}
	commitSHA, err := runGitEnv(opts.RepoRoot, nil, archiveCommitEnv(opts.Now), commitArgs...)
	if err != nil {
		return fmt.Errorf("commit-tree archive %s: %w", q.Label, err)
	}

	if _, err := runGit(opts.RepoRoot, nil, "update-ref", archiveRef, commitSHA); err != nil {
		return fmt.Errorf("update-ref %s: %w", archiveRef, err)
	}

	// Delete the individual session refs (old-value guard against races).
	for _, s := range q.Sessions {
		if _, err := runGit(opts.RepoRoot, nil, "update-ref", "-d", s.Ref, s.CommitSHA); err != nil {
			return fmt.Errorf("delete ref %s: %w", s.Ref, err)
		}
	}
	return nil
}

// buildSessionSubtree reads the two blobs from a session ref's tree and mktrees a
// subtree containing session.json and agent-trace.json.
func buildSessionSubtree(repoRoot, sessionRef string) (string, error) {
	lsOut, err := runGit(repoRoot, nil, "ls-tree", sessionRef)
	if err != nil {
		return "", fmt.Errorf("ls-tree %s: %w", sessionRef, err)
	}
	var subtree bytes.Buffer
	for _, line := range strings.Split(strings.TrimSpace(lsOut), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Reuse the session ref's blob entries verbatim (mode/type/sha\tname).
		subtree.WriteString(line)
		subtree.WriteByte('\n')
	}
	return runGit(repoRoot, subtree.Bytes(), "mktree")
}

// refExists returns the commit SHA a ref points at, and whether it exists.
func refExists(repoRoot, ref string) (string, bool) {
	out, err := runGit(repoRoot, nil, "rev-parse", "--verify", "--quiet", ref)
	if err != nil || out == "" {
		return "", false
	}
	return out, true
}

func archiveCommitEnv(now time.Time) []string {
	ts := fmt.Sprintf("%d +0000", now.UTC().Unix())
	return []string{
		"GIT_AUTHOR_NAME=etch",
		"GIT_AUTHOR_EMAIL=etch@localhost",
		"GIT_COMMITTER_NAME=etch",
		"GIT_COMMITTER_EMAIL=etch@localhost",
		"GIT_AUTHOR_DATE=" + ts,
		"GIT_COMMITTER_DATE=" + ts,
	}
}

func runGit(repoPath string, stdin []byte, args ...string) (string, error) {
	return runGitEnv(repoPath, stdin, nil, args...)
}

func runGitEnv(repoPath string, stdin []byte, extraEnv []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	if len(extraEnv) > 0 {
		cmd.Env = append(cmd.Environ(), extraEnv...)
	}
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}
