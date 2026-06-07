//go:build density

package density_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"forgejo.stage11.ai/s11/etch/internal/testutil"
)

const sessionCount = 20

func TestDensity20Concurrent(t *testing.T) {
	dir := newTestRepoWithCommit(t)

	// Pre-build the binary before launching goroutines
	testutil.RunBinary(t, dir, []string{"info"}, "")

	var wg sync.WaitGroup
	errs := make(chan error, sessionCount)

	for i := 0; i < sessionCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if err := runFullSession(t, dir, idx); err != nil {
				errs <- fmt.Errorf("session %d: %w", idx, err)
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}

	// Verify exactly 20 refs exist
	refs := listSessionRefs(t, dir)
	if len(refs) != sessionCount {
		t.Fatalf("expected %d refs, got %d", sessionCount, len(refs))
	}

	// Verify each ref has valid, unique session data
	sessionIDs := make(map[string]bool)
	for _, ref := range refs {
		session := readSessionJSON(t, dir, ref)

		sid, _ := session["session_id"].(string)
		if sid == "" {
			t.Errorf("ref %s: missing session_id", ref)
			continue
		}
		if sessionIDs[sid] {
			t.Errorf("duplicate session_id: %s", sid)
		}
		sessionIDs[sid] = true

		if session["schema_version"] != "etch.session.v1" {
			t.Errorf("ref %s: schema_version = %v", ref, session["schema_version"])
		}
		if session["status"] != "complete" {
			t.Errorf("ref %s: status = %v", ref, session["status"])
		}
		if session["exit_reason"] != "normal" {
			t.Errorf("ref %s: exit_reason = %v", ref, session["exit_reason"])
		}

		// Verify agent-trace.json
		trace := readTraceJSON(t, dir, ref)
		if trace["version"] != "1.0" {
			t.Errorf("ref %s: agent-trace version = %v", ref, trace["version"])
		}
	}

	if len(sessionIDs) != sessionCount {
		t.Errorf("expected %d unique session IDs, got %d", sessionCount, len(sessionIDs))
	}

	// Push/fetch verification
	verifyPushFetch(t, dir, refs)
}

func TestDensityCrashRecovery(t *testing.T) {
	dir := newTestRepoWithCommit(t)

	// Pre-build
	testutil.RunBinary(t, dir, []string{"info"}, "")

	// Start a session but don't send session_end (simulate crash)
	crashSessionID := "density-crash-victim"
	startInput := fmt.Sprintf(`{"session_id":"%s","raw_data":{"model":"claude-opus-4-7"}}`, crashSessionID)
	r := testutil.RunBinary(t, dir, []string{"session_start"}, startInput)
	if r.ExitCode != 0 {
		t.Fatalf("crash victim session_start failed: %s", r.Stderr)
	}

	promptInput := fmt.Sprintf(`{"session_id":"%s","user_prompt":"doing work before crash"}`, crashSessionID)
	r = testutil.RunBinary(t, dir, []string{"user_prompt_submit"}, promptInput)
	if r.ExitCode != 0 {
		t.Fatalf("crash victim user_prompt_submit failed: %s", r.Stderr)
	}

	// Find the .wip file and overwrite the PID to a dead one so recovery detects it
	sessionsDir := filepath.Join(dir, ".etch", "sessions")
	wipFiles := findFiles(t, sessionsDir, ".wip.jsonl")
	if len(wipFiles) != 1 {
		t.Fatalf("expected 1 wip file, got %d", len(wipFiles))
	}

	// Rewrite the .wip file with a dead PID and old timestamp so recovery triggers
	rewriteWipWithDeadPID(t, wipFiles[0])

	// Start a new session — this triggers crash recovery of the orphaned session
	newSessionID := "density-recovery-trigger"
	startInput = fmt.Sprintf(`{"session_id":"%s","raw_data":{"model":"claude-opus-4-7"}}`, newSessionID)
	r = testutil.RunBinary(t, dir, []string{"session_start"}, startInput)
	if r.ExitCode != 0 {
		t.Fatalf("recovery trigger session_start failed: %s", r.Stderr)
	}

	// End the new session
	endInput := fmt.Sprintf(`{"session_id":"%s"}`, newSessionID)
	r = testutil.RunBinary(t, dir, []string{"session_end"}, endInput)
	if r.ExitCode != 0 {
		t.Fatalf("recovery trigger session_end failed: %s", r.Stderr)
	}

	// Verify: should have 2 refs — one complete, one incomplete
	refs := listSessionRefs(t, dir)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs after crash recovery, got %d", len(refs))
	}

	var foundComplete, foundIncomplete bool
	for _, ref := range refs {
		session := readSessionJSON(t, dir, ref)
		status, _ := session["status"].(string)
		exitReason, _ := session["exit_reason"].(string)

		if status == "complete" && exitReason == "normal" {
			foundComplete = true
		}
		if status == "incomplete" && exitReason == "crash" {
			foundIncomplete = true
		}
	}

	if !foundComplete {
		t.Error("missing complete session ref")
	}
	if !foundIncomplete {
		t.Error("missing incomplete (crash-recovered) session ref")
	}

	// Verify no orphaned .wip files remain
	remainingWip := findFiles(t, sessionsDir, ".wip.jsonl")
	if len(remainingWip) != 0 {
		t.Errorf("expected 0 wip files after recovery, got %d", len(remainingWip))
	}
}

func TestDensityRefUniqueness(t *testing.T) {
	dir := newTestRepoWithCommit(t)

	// Pre-build
	testutil.RunBinary(t, dir, []string{"info"}, "")

	// Run 20 sessions sequentially to test ULID uniqueness under rapid creation
	for i := 0; i < sessionCount; i++ {
		if err := runFullSession(t, dir, i); err != nil {
			t.Fatalf("session %d failed: %v", i, err)
		}
	}

	refs := listSessionRefs(t, dir)
	if len(refs) != sessionCount {
		t.Fatalf("expected %d refs, got %d", sessionCount, len(refs))
	}

	// Collect all session IDs and verify uniqueness
	ids := make(map[string]bool)
	var orderedIDs []string
	for _, ref := range refs {
		session := readSessionJSON(t, dir, ref)
		sid, _ := session["session_id"].(string)
		if ids[sid] {
			t.Errorf("duplicate session_id: %s", sid)
		}
		ids[sid] = true
		orderedIDs = append(orderedIDs, sid)
	}

	if len(ids) != sessionCount {
		t.Errorf("expected %d unique IDs, got %d", sessionCount, len(ids))
	}
}

// --- helpers ---

func newTestRepoWithCommit(t *testing.T) string {
	t.Helper()
	dir := testutil.NewTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.RunCmd(t, dir, "git", "add", "init.txt")
	testutil.RunCmd(t, dir, "git", "commit", "-m", "initial commit")
	return dir
}

func runFullSession(t *testing.T, dir string, idx int) error {
	t.Helper()
	entireSessionID := fmt.Sprintf("density-%03d", idx)

	// session_start
	startInput := fmt.Sprintf(`{"session_id":"%s","raw_data":{"model":"claude-opus-4-7"}}`, entireSessionID)
	r := testutil.RunBinary(t, dir, []string{"session_start"}, startInput)
	if r.ExitCode != 0 {
		return fmt.Errorf("session_start: exit %d: %s", r.ExitCode, r.Stderr)
	}

	// user_prompt_submit
	promptInput := fmt.Sprintf(`{"session_id":"%s","user_prompt":"density test session %d"}`, entireSessionID, idx)
	r = testutil.RunBinary(t, dir, []string{"user_prompt_submit"}, promptInput)
	if r.ExitCode != 0 {
		return fmt.Errorf("user_prompt_submit: exit %d: %s", r.ExitCode, r.Stderr)
	}

	// pre_tool_use
	toolInput := fmt.Sprintf(`{"session_id":"%s","tool_name":"Read","tool_use_id":"tu-%d","tool_input":{"file_path":"/tmp/test.go"}}`, entireSessionID, idx)
	r = testutil.RunBinary(t, dir, []string{"pre_tool_use"}, toolInput)
	if r.ExitCode != 0 {
		return fmt.Errorf("pre_tool_use: exit %d: %s", r.ExitCode, r.Stderr)
	}

	// post_tool_use
	r = testutil.RunBinary(t, dir, []string{"post_tool_use"}, toolInput)
	if r.ExitCode != 0 {
		return fmt.Errorf("post_tool_use: exit %d: %s", r.ExitCode, r.Stderr)
	}

	// session_end
	endInput := fmt.Sprintf(`{"session_id":"%s"}`, entireSessionID)
	r = testutil.RunBinary(t, dir, []string{"session_end"}, endInput)
	if r.ExitCode != 0 {
		return fmt.Errorf("session_end: exit %d: %s", r.ExitCode, r.Stderr)
	}

	return nil
}

func listSessionRefs(t *testing.T, dir string) []string {
	t.Helper()
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname)", "refs/etch/sessions/")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git for-each-ref: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

func readSessionJSON(t *testing.T, dir, ref string) map[string]any {
	t.Helper()
	cmd := exec.Command("git", "show", ref+":session.json")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git show %s:session.json: %v", ref, err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("invalid session.json in %s: %v", ref, err)
	}
	return m
}

func readTraceJSON(t *testing.T, dir, ref string) map[string]any {
	t.Helper()
	cmd := exec.Command("git", "show", ref+":agent-trace.json")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git show %s:agent-trace.json: %v", ref, err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("invalid agent-trace.json in %s: %v", ref, err)
	}
	return m
}

func findFiles(t *testing.T, dir, suffix string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("reading dir %s: %v", dir, err)
	}
	var matched []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), suffix) {
			matched = append(matched, filepath.Join(dir, e.Name()))
		}
	}
	return matched
}

func verifyPushFetch(t *testing.T, dir string, expectedRefs []string) {
	t.Helper()

	// Create a bare remote
	bareDir := t.TempDir()
	testutil.RunCmd(t, bareDir, "git", "init", "--bare")

	// Add remote and configure refspec
	testutil.RunCmd(t, dir, "git", "remote", "add", "density-remote", bareDir)
	testutil.RunCmd(t, dir, "git", "config", "--add", "remote.density-remote.push", "refs/etch/sessions/*:refs/etch/sessions/*")
	testutil.RunCmd(t, dir, "git", "config", "--add", "remote.density-remote.fetch", "+refs/etch/sessions/*:refs/etch/sessions/*")

	// Push
	cmd := exec.Command("git", "push", "density-remote")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git push failed: %v\n%s", err, out)
	}

	// Clone into a second directory and fetch
	cloneDir := t.TempDir()
	testutil.RunCmd(t, cloneDir, "git", "clone", bareDir, "fetched")
	fetchedDir := filepath.Join(cloneDir, "fetched")

	testutil.RunCmd(t, fetchedDir, "git", "config", "--add", "remote.origin.fetch", "+refs/etch/sessions/*:refs/etch/sessions/*")

	cmd = exec.Command("git", "fetch", "origin")
	cmd.Dir = fetchedDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git fetch failed: %v\n%s", err, out)
	}

	// Verify ref count matches
	fetchedRefs := listSessionRefs(t, fetchedDir)
	if len(fetchedRefs) != len(expectedRefs) {
		t.Errorf("fetched ref count: got %d, want %d", len(fetchedRefs), len(expectedRefs))
	}

	// Verify content matches for each ref
	for _, ref := range fetchedRefs {
		session := readSessionJSON(t, fetchedDir, ref)
		if session["schema_version"] != "etch.session.v1" {
			t.Errorf("fetched ref %s: invalid schema_version", ref)
		}
		if session["status"] != "complete" {
			t.Errorf("fetched ref %s: status = %v", ref, session["status"])
		}
	}
}

func rewriteWipWithDeadPID(t *testing.T, wipPath string) {
	t.Helper()

	// Read the original content to get the session ID
	origData, err := os.ReadFile(wipPath)
	if err != nil {
		t.Fatalf("reading wip file: %v", err)
	}

	// Parse each line, update the PID and timestamp, then rewrite
	lines := strings.Split(strings.TrimSpace(string(origData)), "\n")
	var newLines []string
	oldTimestamp := time.Now().Add(-5 * time.Hour).UTC().Format(time.RFC3339Nano)

	for _, line := range lines {
		if line == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			newLines = append(newLines, line)
			continue
		}

		ev["ts"] = oldTimestamp
		if data, ok := ev["data"].(map[string]any); ok {
			data["pid"] = 999999 // dead PID
		}

		rewritten, err := json.Marshal(ev)
		if err != nil {
			newLines = append(newLines, line)
			continue
		}
		newLines = append(newLines, string(rewritten))
	}

	if err := os.WriteFile(wipPath, []byte(strings.Join(newLines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("rewriting wip file: %v", err)
	}

	// The recovery scan judges idleness on mtime (stat-first) — backdate it
	// to match the rewritten event timestamps.
	old := time.Now().Add(-5 * time.Hour)
	if err := os.Chtimes(wipPath, old, old); err != nil {
		t.Fatalf("backdating wip mtime: %v", err)
	}
}
