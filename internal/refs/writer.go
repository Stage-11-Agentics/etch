package refs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	// SessionsRefPrefix is the canonical, pushable session namespace — the
	// one setup-refspec syncs. When local_only_fields is configured these
	// refs hold the stripped projection (ETCH-41).
	SessionsRefPrefix = "refs/etch/sessions/"
	// LocalRefPrefix holds the full-fidelity record when local_only_fields
	// is configured. No etch-configured refspec ever names it; it stays on
	// the machine that wrote it.
	LocalRefPrefix = "refs/etch/local/"
)

// ErrRefExists is returned when the canonical session ref already exists and
// the incoming record may not replace it. Session refs are immutable once
// committed (OUTPUT_SPEC: "once committed… never updated") — callers treat
// this as "already committed": keep the existing record, clean up local
// state, do not retry.
var ErrRefExists = errors.New("session ref already exists")

type RefMeta struct {
	Runtime      string
	Model        string
	Status       string
	Branch       string
	CommitCount  int
	DurationSecs int
	EndTime      time.Time
}

// WriteSessionRef creates an orphan commit containing session.json and
// agent-trace.json, then points refs/etch/sessions/<sessionID> at it.
//
// The canonical namespace is create-only: an existing record is never
// silently overwritten (ETCH-40 finding 3). One replacement is legitimate
// (and deliberate): a truthful `complete` record upgrading a premature
// incomplete/crash record for the same ULID — e.g. recovery committed a
// 'crash' record in the window where a real session_end's commit had failed,
// and the retried end now carries the full truth. Never the reverse, never
// complete→complete. The upgrade is a compare-and-swap on the observed SHA;
// losing the CAS re-reads and re-evaluates (plan-review R3), so concurrent
// writers converge on exactly one surviving record. All other conflicts
// return ErrRefExists.
func WriteSessionRef(repoPath, sessionID string, sessionJSON, traceJSON []byte, meta RefMeta) error {
	commitSHA, err := buildSessionCommit(repoPath, sessionID, sessionJSON, traceJSON, meta)
	if err != nil {
		return err
	}

	refName := SessionsRefPrefix + sessionID

	// Create-only: the empty old-value arg makes update-ref fail when the
	// ref already exists.
	_, createErr := runGit(repoPath, nil, "update-ref", refName, commitSHA, "")
	if createErr == nil {
		return nil
	}

	// The create failed. Only when the ref actually resolves is this an
	// exists-conflict — otherwise (D/F conflict, permissions, disk) it is a
	// real write failure that must stay visible to the caller.
	for attempt := 0; attempt < 3; attempt++ {
		existingSHA, err := runGit(repoPath, nil, "rev-parse", "--verify", refName)
		if err != nil {
			// The ref does not resolve: either it never existed (the create
			// failed for a real reason) or it vanished concurrently. One
			// more create attempt disambiguates.
			if _, cerr := runGit(repoPath, nil, "update-ref", refName, commitSHA, ""); cerr == nil {
				return nil
			} else {
				createErr = cerr
			}
			continue
		}

		if refStatus(repoPath, existingSHA) != "complete" && meta.Status == "complete" {
			if _, uerr := runGit(repoPath, nil, "update-ref", refName, commitSHA, existingSHA); uerr == nil {
				return nil
			}
			continue // CAS lost — re-read and re-evaluate
		}

		return fmt.Errorf("%w: %s (incoming status %q)", ErrRefExists, refName, meta.Status)
	}

	return fmt.Errorf("update-ref (create): %w", createErr)
}

// WriteSessionRefAt is WriteSessionRef with an explicit ref name, for writing
// the same session into a different namespace (refs/etch/local/, ETCH-41).
// Unlike the canonical namespace it overwrites: the local ref's documented
// contract is that recovery re-commits converge by overwrite.
func WriteSessionRefAt(repoPath, refName, sessionID string, sessionJSON, traceJSON []byte, meta RefMeta) error {
	commitSHA, err := buildSessionCommit(repoPath, sessionID, sessionJSON, traceJSON, meta)
	if err != nil {
		return err
	}

	if _, err := runGit(repoPath, nil, "update-ref", refName, commitSHA); err != nil {
		return fmt.Errorf("update-ref: %w", err)
	}

	return nil
}

// buildSessionCommit writes the two blobs, the tree, and the orphan commit,
// returning the commit SHA (no ref is touched).
func buildSessionCommit(repoPath, sessionID string, sessionJSON, traceJSON []byte, meta RefMeta) (string, error) {
	sessionBlob, err := runGit(repoPath, sessionJSON, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", fmt.Errorf("hash-object session.json: %w", err)
	}

	traceBlob, err := runGit(repoPath, traceJSON, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", fmt.Errorf("hash-object agent-trace.json: %w", err)
	}

	treeInput := fmt.Sprintf("100644 blob %s\tagent-trace.json\n100644 blob %s\tsession.json\n", traceBlob, sessionBlob)
	treeSHA, err := runGit(repoPath, []byte(treeInput), "mktree")
	if err != nil {
		return "", fmt.Errorf("mktree: %w", err)
	}

	msg := formatCommitMessage(sessionID, meta)
	commitSHA, err := runGitEnv(repoPath, nil, commitEnv(meta.EndTime), "commit-tree", treeSHA, "-m", msg)
	if err != nil {
		return "", fmt.Errorf("commit-tree: %w", err)
	}

	return commitSHA, nil
}

// refStatus reads the status field of the session.json blob in a session-ref
// commit. Returns "" when unreadable — treated as not-complete, i.e. an
// unreadable existing record may be upgraded by a complete one but never
// silently kept over it.
func refStatus(repoPath, commitSHA string) string {
	out, err := runGit(repoPath, nil, "show", commitSHA+":session.json")
	if err != nil {
		return ""
	}
	var s struct {
		Status string `json:"status"`
	}
	if json.Unmarshal([]byte(out), &s) != nil {
		return ""
	}
	return s.Status
}

func formatCommitMessage(sessionID string, meta RefMeta) string {
	return fmt.Sprintf("etch session %s\nagent: %s / %s\nstatus: %s\nbranch: %s\ncommits: %d\nduration: %ds",
		sessionID, meta.Runtime, meta.Model, meta.Status, meta.Branch, meta.CommitCount, meta.DurationSecs)
}

func commitEnv(endTime time.Time) []string {
	ts := fmt.Sprintf("%d +0000", endTime.Unix())
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
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}
