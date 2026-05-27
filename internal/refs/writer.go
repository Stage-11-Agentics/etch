package refs

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

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
// agent-trace.json, then points refs/cairn/sessions/<sessionID> at it.
func WriteSessionRef(repoPath, sessionID string, sessionJSON, traceJSON []byte, meta RefMeta) error {
	sessionBlob, err := runGit(repoPath, sessionJSON, "hash-object", "-w", "--stdin")
	if err != nil {
		return fmt.Errorf("hash-object session.json: %w", err)
	}

	traceBlob, err := runGit(repoPath, traceJSON, "hash-object", "-w", "--stdin")
	if err != nil {
		return fmt.Errorf("hash-object agent-trace.json: %w", err)
	}

	treeInput := fmt.Sprintf("100644 blob %s\tagent-trace.json\n100644 blob %s\tsession.json\n", traceBlob, sessionBlob)
	treeSHA, err := runGit(repoPath, []byte(treeInput), "mktree")
	if err != nil {
		return fmt.Errorf("mktree: %w", err)
	}

	msg := formatCommitMessage(sessionID, meta)
	commitSHA, err := runGitEnv(repoPath, nil, commitEnv(meta.EndTime), "commit-tree", treeSHA, "-m", msg)
	if err != nil {
		return fmt.Errorf("commit-tree: %w", err)
	}

	refName := "refs/cairn/sessions/" + sessionID
	_, err = runGit(repoPath, nil, "update-ref", refName, commitSHA)
	if err != nil {
		return fmt.Errorf("update-ref: %w", err)
	}

	return nil
}

func formatCommitMessage(sessionID string, meta RefMeta) string {
	return fmt.Sprintf("cairn session %s\nagent: %s / %s\nstatus: %s\nbranch: %s\ncommits: %d\nduration: %ds",
		sessionID, meta.Runtime, meta.Model, meta.Status, meta.Branch, meta.CommitCount, meta.DurationSecs)
}

func commitEnv(endTime time.Time) []string {
	ts := fmt.Sprintf("%d +0000", endTime.Unix())
	return []string{
		"GIT_AUTHOR_NAME=cairn",
		"GIT_AUTHOR_EMAIL=cairn@localhost",
		"GIT_COMMITTER_NAME=cairn",
		"GIT_COMMITTER_EMAIL=cairn@localhost",
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
