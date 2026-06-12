package hooks_test

// Adversarial tests for the repo-root batch (ETCH-34, ETCH-35, ETCH-40 finding 2):
// hooks must anchor .etch state at the main repo root regardless of the CWD they fire
// from (subdir, linked worktree), and commit-failure paths must be visible —
// never {"ok":true} while dropping data. The non-git case is owned by the
// operator-mode fast-exit guard (ETCH-47, docs/ENABLEMENT.md): silent exit 0.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Stage-11-Agentics/etch/internal/capture"
	"github.com/Stage-11-Agentics/etch/internal/testutil"
)

// realPath resolves symlinks so git-reported (physical) paths compare equal to
// t.TempDir() (logical) paths on macOS.
func realPath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", p, err)
	}
	return r
}

func assertNoEtch(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, ".etch")); !os.IsNotExist(err) {
		t.Errorf("unexpected .etch dir in %s", dir)
	}
}

// ETCH-34 gate: session_start at the repo root + remaining hooks from nested subdirs
// land in ONE record under one .etch at the root.
func TestHooksFromSubdirsProduceOneRecord(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	nested := filepath.Join(dir, "src", "deep", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	sid := "subdir-batch-001"

	r := testutil.RunBinary(t, dir, []string{"session_start"}, `{"session_id":"`+sid+`","raw_data":{"model":"claude-opus-4-8"}}`)
	assertOK(t, r, "session_start at root")

	wipFiles := findWipFiles(t, dir)
	if len(wipFiles) != 1 {
		t.Fatalf("expected 1 wip file at root, got %d", len(wipFiles))
	}
	ulid := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	r = testutil.RunBinary(t, filepath.Join(dir, "src"), []string{"user_prompt_submit"}, `{"session_id":"`+sid+`","user_prompt":"refactor the parser"}`)
	assertOK(t, r, "user_prompt_submit from src/")

	r = testutil.RunBinary(t, filepath.Join(dir, "src", "deep"), []string{"pre_tool_use"}, `{"session_id":"`+sid+`","tool_name":"Edit","tool_use_id":"tu-1","tool_input":{"file_path":"/tmp/x.go"}}`)
	assertOK(t, r, "pre_tool_use from src/deep/")

	r = testutil.RunBinary(t, nested, []string{"session_end"}, `{"session_id":"`+sid+`"}`)
	assertOK(t, r, "session_end from src/deep/nested/")

	// No scattered .etch dirs
	assertNoEtch(t, filepath.Join(dir, "src"))
	assertNoEtch(t, filepath.Join(dir, "src", "deep"))
	assertNoEtch(t, nested)

	// One coherent committed record at the root
	data := readRefBlob(t, dir, "refs/etch/sessions/"+ulid+":session.json")
	var session capture.Session
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatalf("invalid session JSON: %v", err)
	}
	if session.Prompt == nil || session.Prompt.Text != "refactor the parser" {
		t.Errorf("prompt lost across CWDs: %+v", session.Prompt)
	}
	if session.ToolUse.ByTool["Edit"] != 1 {
		t.Errorf("tool use lost across CWDs: %+v", session.ToolUse.ByTool)
	}
	if session.Status != "complete" {
		t.Errorf("status: got %s, want complete", session.Status)
	}

	// Buffer state fully cleaned up at the root
	if len(findWipFiles(t, dir)) != 0 {
		t.Error("wip file should be cleaned up after session_end")
	}
}

// ETCH-34: .etch/settings.json at the repo root applies to sessions whose hooks fire
// from a subdir (custom redaction must not silently weaken).
func TestRootSettingsApplyFromSubdir(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	if err := os.MkdirAll(filepath.Join(dir, ".etch"), 0o755); err != nil {
		t.Fatal(err)
	}
	settings := `{"redaction_patterns":["SECRETTOKEN[0-9]+"]}`
	if err := os.WriteFile(filepath.Join(dir, ".etch", "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}

	sub := filepath.Join(dir, "src")
	os.MkdirAll(sub, 0o755)

	sid := "subdir-settings-001"
	r := testutil.RunBinary(t, sub, []string{"session_start"}, `{"session_id":"`+sid+`","raw_data":{}}`)
	assertOK(t, r, "session_start from subdir")

	wipFiles := findWipFiles(t, dir)
	if len(wipFiles) != 1 {
		t.Fatalf("expected wip at root for subdir session_start, got %d", len(wipFiles))
	}
	ulid := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	r = testutil.RunBinary(t, sub, []string{"user_prompt_submit"}, `{"session_id":"`+sid+`","user_prompt":"deploy with SECRETTOKEN12345 now"}`)
	assertOK(t, r, "user_prompt_submit from subdir")

	r = testutil.RunBinary(t, sub, []string{"session_end"}, `{"session_id":"`+sid+`"}`)
	assertOK(t, r, "session_end from subdir")

	data := readRefBlob(t, dir, "refs/etch/sessions/"+ulid+":session.json")
	var session capture.Session
	json.Unmarshal(data, &session)
	if session.Prompt == nil {
		t.Fatal("expected non-nil prompt")
	}
	if strings.Contains(session.Prompt.Text, "SECRETTOKEN12345") {
		t.Error("root settings.json redaction_patterns ignored for subdir session")
	}
	if !strings.Contains(session.Prompt.Text, "[REDACTED") {
		t.Errorf("expected redaction marker, got %q", session.Prompt.Text)
	}
}

// ETCH-34 / ETCH-40 f.2 gate: hooks fired from a linked worktree anchor state at the
// MAIN repo root, while git capture reflects the worktree's own checkout.
func TestHooksFromLinkedWorktree(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	wt := filepath.Join(t.TempDir(), "wt")
	run(t, dir, "git", "worktree", "add", wt, "-b", "feature")

	sid := "worktree-batch-001"
	r := testutil.RunBinary(t, wt, []string{"session_start"}, `{"session_id":"`+sid+`","raw_data":{"model":"claude-opus-4-8"}}`)
	assertOK(t, r, "session_start in worktree")

	// State must land at the main root, never inside the worktree
	assertNoEtch(t, wt)
	wipFiles := findWipFiles(t, dir)
	if len(wipFiles) != 1 {
		t.Fatalf("expected 1 wip at MAIN root for worktree session, got %d", len(wipFiles))
	}
	ulid := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	r = testutil.RunBinary(t, wt, []string{"user_prompt_submit"}, `{"session_id":"`+sid+`","user_prompt":"work in the worktree"}`)
	assertOK(t, r, "user_prompt_submit in worktree")

	// Produce a commit in the worktree so the diff has content
	os.WriteFile(filepath.Join(wt, "feature.go"), []byte("package feature"), 0o644)
	run(t, wt, "git", "add", "feature.go")
	run(t, wt, "git", "commit", "-m", "worktree commit")

	// End the session from a SUBDIR of the worktree
	sub := filepath.Join(wt, "pkg")
	os.MkdirAll(sub, 0o755)
	r = testutil.RunBinary(t, sub, []string{"session_end"}, `{"session_id":"`+sid+`"}`)
	assertOK(t, r, "session_end from worktree subdir")

	assertNoEtch(t, wt)
	assertNoEtch(t, sub)

	data := readRefBlob(t, dir, "refs/etch/sessions/"+ulid+":session.json")
	var session capture.Session
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatalf("invalid session JSON: %v", err)
	}

	// Git capture must reflect the worktree's own checkout
	if session.GitStart == nil {
		t.Fatal("git_start should not be nil")
	}
	if !session.GitStart.IsWorktree {
		t.Error("git_start.is_worktree: got false, want true")
	}
	if realPath(t, session.GitStart.WorktreePath) != realPath(t, wt) {
		t.Errorf("git_start.worktree_path: got %s, want %s", session.GitStart.WorktreePath, wt)
	}
	if session.GitStart.Branch != "feature" {
		t.Errorf("git_start.branch: got %s, want feature", session.GitStart.Branch)
	}
	if session.GitEnd == nil || len(session.GitEnd.CommitsProduced) != 1 {
		t.Errorf("git_end.commits_produced: got %+v, want exactly 1", session.GitEnd)
	}

	// Diff ran against the worktree's checkout, not the main one
	foundFeature := false
	for _, f := range session.FilesTouched {
		if f.Path == "feature.go" {
			foundFeature = true
			if f.Action != "added" {
				t.Errorf("feature.go action: got %s, want added", f.Action)
			}
		}
	}
	if !foundFeature {
		t.Errorf("files_touched missing worktree change feature.go: %+v", session.FilesTouched)
	}

	if session.Prompt == nil || session.Prompt.Text != "work in the worktree" {
		t.Errorf("prompt lost for worktree session: %+v", session.Prompt)
	}
}

// ETCH-34: an orphan that crashed under one checkout is recovered by a session_start
// fired from a different checkout (the linked worktree) — the sweep anchors at the
// shared state root.
func TestOrphanRecoveredFromWorktreeSessionStart(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	wt := filepath.Join(t.TempDir(), "wt")
	run(t, dir, "git", "worktree", "add", wt, "-b", "recovery-feature")

	// Plant an orphaned wip at the main root (>4h idle = past the default timeout)
	sessionsDir := filepath.Join(dir, ".etch", "sessions")
	os.MkdirAll(filepath.Join(sessionsDir, ".map"), 0o755)
	orphanID := "01REPOROOTORPHAN0000000000"
	ts := time.Now().Add(-5 * time.Hour).UTC().Format(time.RFC3339Nano)
	wip := `{"ts":"` + ts + `","hook":"session_start","data":{"session_id":"` + orphanID + `","agent":{"runtime":"claude-code","model":"claude-opus-4-8"},"orchestration":{"type":"manual","extra":{}},"git_state":{"branch":"main","head_sha":"abc123"}}}` + "\n"
	if err := os.WriteFile(filepath.Join(sessionsDir, orphanID+".wip.jsonl"), []byte(wip), 0o644); err != nil {
		t.Fatal(err)
	}
	// The recovery scan judges idleness on mtime — backdate it to match the events.
	old := time.Now().Add(-5 * time.Hour)
	if err := os.Chtimes(filepath.Join(sessionsDir, orphanID+".wip.jsonl"), old, old); err != nil {
		t.Fatal(err)
	}

	// Trigger recovery from INSIDE the worktree
	r := testutil.RunBinary(t, wt, []string{"session_start"}, `{"session_id":"recovery-trigger-wt","raw_data":{}}`)
	assertOK(t, r, "session_start in worktree (recovery trigger)")

	refCheck := exec.Command("git", "show-ref", "--verify", "refs/etch/sessions/"+orphanID)
	refCheck.Dir = dir
	if err := refCheck.Run(); err != nil {
		t.Fatalf("orphan was not recovered by worktree session_start: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sessionsDir, orphanID+".wip.jsonl")); !os.IsNotExist(err) {
		t.Error("orphan wip should be cleaned up after recovery")
	}
}

// ETCH-47 (supersedes the ETCH-35 fail-visible gate for this path): every
// hook in a non-git directory fast-exits — exit 0, no output, no .etch
// pollution (docs/ENABLEMENT.md). The guard runs before stdin is read; loud
// failure remains the contract for in-repo errors (see
// TestCommitFailureVisibleAndRecoverable).
func TestNonGitDirAllHooksFastExit(t *testing.T) {
	dir := t.TempDir()

	hooks := []string{"session_start", "user_prompt_submit", "pre_tool_use", "post_tool_use", "session_end", "stop"}
	for _, hook := range hooks {
		input := `{"session_id":"nogit-001","user_prompt":"hi","tool_name":"Read"}`
		r := testutil.RunBinary(t, dir, []string{hook}, input)

		if r.ExitCode != 0 {
			t.Errorf("%s: expected fast exit 0 in non-git dir, got %d (stderr: %s)", hook, r.ExitCode, r.Stderr)
		}
		if r.Stdout != "" || r.Stderr != "" {
			t.Errorf("%s: fast exit must be silent, got stdout=%q stderr=%q", hook, r.Stdout, r.Stderr)
		}
	}

	assertNoEtch(t, dir)
}

// ETCH-35: a commit failure at session_end is visible (non-ok, non-zero) and retains
// the wip + mapping for later recovery.
func TestCommitFailureVisibleAndRecoverable(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	sid := "commit-fail-001"
	r := testutil.RunBinary(t, dir, []string{"session_start"}, `{"session_id":"`+sid+`","raw_data":{}}`)
	assertOK(t, r, "session_start")

	wipFiles := findWipFiles(t, dir)
	if len(wipFiles) != 1 {
		t.Fatalf("expected 1 wip, got %d", len(wipFiles))
	}
	ulid := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	// Deterministic ref-write sabotage: a ref nested UNDER the session's ref name
	// makes update-ref fail with a directory/file conflict.
	run(t, dir, "git", "update-ref", "refs/etch/sessions/"+ulid+"/block", "HEAD")

	r = testutil.RunBinary(t, dir, []string{"session_end"}, `{"session_id":"`+sid+`"}`)
	if r.ExitCode == 0 {
		t.Error("session_end with failing commit should exit non-zero")
	}
	if strings.Contains(r.Stdout, `"ok":true`) {
		t.Errorf("session_end must not print ok:true on commit failure, got: %s", r.Stdout)
	}
	if !strings.Contains(r.Stdout, `"ok":false`) {
		t.Errorf("expected ok:false on stdout, got: %s", r.Stdout)
	}

	// wip + mapping retained so recovery can retry later
	if len(findWipFiles(t, dir)) != 1 {
		t.Error("wip must be retained after commit failure (recovery needs it)")
	}
	mapDir := filepath.Join(dir, ".etch", "sessions", ".map")
	entries, _ := os.ReadDir(mapDir)
	if len(entries) != 1 {
		t.Errorf("mapping must be retained after commit failure, got %d entries", len(entries))
	}
}

// TestCommitRetryViaRecovery (ETCH-40 finding 8): after a failed commit the
// retained wip contains the real end event — the next session_start's
// recovery pass must commit it as the truthful complete/normal record,
// exactly once. The full injection chain: sabotage → visible failure →
// unsabotage → sibling start → one correct ref.
func TestCommitRetryViaRecovery(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	sid := "commit-retry-001"
	r := testutil.RunBinary(t, dir, []string{"session_start"}, `{"session_id":"`+sid+`","raw_data":{}}`)
	assertOK(t, r, "session_start")
	wipFiles := findWipFiles(t, dir)
	ulid := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	// Sabotage the ref store, fail the end visibly.
	run(t, dir, "git", "update-ref", "refs/etch/sessions/"+ulid+"/block", "HEAD")
	r = testutil.RunBinary(t, dir, []string{"session_end"}, `{"session_id":"`+sid+`"}`)
	if r.ExitCode == 0 || strings.Contains(r.Stdout, `"ok":true`) {
		t.Fatalf("sabotaged session_end must fail visibly, got exit %d stdout %s", r.ExitCode, r.Stdout)
	}
	if len(findWipFiles(t, dir)) != 1 {
		t.Fatal("wip must be retained after the failed commit")
	}

	// Heal the ref store.
	run(t, dir, "git", "update-ref", "-d", "refs/etch/sessions/"+ulid+"/block")

	// Make the wip eligible for the recovery scan: neutralize the recorded
	// PID (in a dev environment the test's own agent ancestor is alive and
	// would veto) and age the mtime past the activity grace.
	raw, err := os.ReadFile(wipFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	doctored := regexp.MustCompile(`"pid":\d+,?`).ReplaceAll(raw, nil)
	doctored = regexp.MustCompile(`"pid_start_time":"[^"]*",?`).ReplaceAll(doctored, nil)
	if err := os.WriteFile(wipFiles[0], doctored, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-5 * time.Hour)
	if err := os.Chtimes(wipFiles[0], old, old); err != nil {
		t.Fatal(err)
	}

	// A sibling session_start triggers the retry.
	r = testutil.RunBinary(t, dir, []string{"session_start"}, `{"session_id":"sibling-after-fail","raw_data":{}}`)
	assertOK(t, r, "sibling session_start (retry trigger)")

	// Exactly one ref for the ULID, and it is the TRUTHFUL record: the wip
	// contained the real end event, so no 'crash' falsification.
	sessionData := gitShowOut(t, dir, "refs/etch/sessions/"+ulid+":session.json")
	var rec map[string]any
	if err := json.Unmarshal([]byte(sessionData), &rec); err != nil {
		t.Fatalf("recovered record unparseable: %v", err)
	}
	if rec["status"] != "complete" {
		t.Errorf("status: want complete (end event was recorded), got %v", rec["status"])
	}
	if rec["exit_reason"] != "normal" {
		t.Errorf("exit_reason: want normal, got %v", rec["exit_reason"])
	}

	// And the retained state is gone — the retry happened exactly once.
	if len(findWipFiles(t, dir)) != 1 { // only the sibling's own wip remains
		t.Errorf("expected only the sibling's wip to remain, got %d", len(findWipFiles(t, dir)))
	}
	entries, _ := os.ReadDir(filepath.Join(dir, ".etch", "sessions", ".map"))
	if len(entries) != 1 { // only the sibling's mapping
		t.Errorf("expected only the sibling's mapping to remain, got %d", len(entries))
	}
}

// TestStopRetryAfterFailedSessionEnd (ETCH-40 finding 8, the duplicate-end
// arm): a stop hook arriving while the failed-commit wip is still retained
// re-finalizes it. The reducer's end-event precedence must keep the
// session_end's truth (exit_reason normal, its git_end) — the stop must not
// degrade the record to 'unknown'.
func TestStopRetryAfterFailedSessionEnd(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	sid := "stop-retry-001"
	r := testutil.RunBinary(t, dir, []string{"session_start"}, `{"session_id":"`+sid+`","raw_data":{}}`)
	assertOK(t, r, "session_start")
	wipFiles := findWipFiles(t, dir)
	ulid := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	run(t, dir, "git", "update-ref", "refs/etch/sessions/"+ulid+"/block", "HEAD")
	r = testutil.RunBinary(t, dir, []string{"session_end"}, `{"session_id":"`+sid+`"}`)
	if r.ExitCode == 0 {
		t.Fatal("sabotaged session_end must fail")
	}

	run(t, dir, "git", "update-ref", "-d", "refs/etch/sessions/"+ulid+"/block")

	// The stop retries the commit through the intact mapping.
	r = testutil.RunBinary(t, dir, []string{"stop"}, `{"session_id":"`+sid+`"}`)
	assertOK(t, r, "stop (retry)")

	sessionData := gitShowOut(t, dir, "refs/etch/sessions/"+ulid+":session.json")
	var rec map[string]any
	if err := json.Unmarshal([]byte(sessionData), &rec); err != nil {
		t.Fatalf("record unparseable: %v", err)
	}
	if rec["status"] != "complete" {
		t.Errorf("status: want complete, got %v", rec["status"])
	}
	if rec["exit_reason"] != "normal" {
		t.Errorf("exit_reason: the recorded session_end must win over the stop; want normal, got %v", rec["exit_reason"])
	}

	if len(findWipFiles(t, dir)) != 0 {
		t.Error("wip must be cleaned up after the successful retry")
	}
	entries, _ := os.ReadDir(filepath.Join(dir, ".etch", "sessions", ".map"))
	if len(entries) != 0 {
		t.Error("mapping must be cleaned up after the successful retry")
	}
}

// gitShowOut returns `git show <ref>` output from dir.
func gitShowOut(t *testing.T, dir, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "show", ref)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git show %s: %v", ref, err)
	}
	return string(out)
}

// Regression guard for the REFUTED "exit_reason clobber" finding: a stop arriving
// after session_end already finalized must still exit 0 with {"ok":true}.
func TestStopAfterSessionEndStaysOK(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	sid := "stop-after-end-001"
	r := testutil.RunBinary(t, dir, []string{"session_start"}, `{"session_id":"`+sid+`","raw_data":{}}`)
	assertOK(t, r, "session_start")

	r = testutil.RunBinary(t, dir, []string{"session_end"}, `{"session_id":"`+sid+`"}`)
	assertOK(t, r, "session_end")

	r = testutil.RunBinary(t, dir, []string{"stop"}, `{"session_id":"`+sid+`"}`)
	assertOK(t, r, "stop after session_end (mapping-miss path is by design)")
}
