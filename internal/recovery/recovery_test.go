package recovery

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forgejo.stage11.ai/s11/etch/internal/capture"
)

// hookEv builds a nested HookEvent line — the actual .wip.jsonl format
// written by capture.AppendEvent.
func hookEv(t *testing.T, ts time.Time, hook string, data any) capture.HookEvent {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return capture.HookEvent{
		Timestamp: ts.UTC().Format(time.RFC3339Nano),
		Hook:      hook,
		Data:      raw,
	}
}

// writeWIP writes events as a .wip.jsonl under repoRoot/.etch/sessions.
func writeWIP(t *testing.T, repoRoot, sid string, events []capture.HookEvent) string {
	t.Helper()
	dir := filepath.Join(repoRoot, ".etch", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, sid+".wip.jsonl")
	var buf bytes.Buffer
	for _, ev := range events {
		line, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// startData builds a session_start payload. pid 0 means "not recorded".
func startData(sid string, pid int, gs *capture.GitState) map[string]any {
	m := map[string]any{
		"session_id": sid,
		"agent": map[string]any{
			"runtime": "claude-code",
			"model":   "claude-opus-4-7",
			"version": "1.0.33",
		},
		"orchestration": map[string]any{"type": "manual", "extra": map[string]any{}},
		"machine": map[string]any{
			"hostname_hash": "sha256:test",
			"os":            "darwin",
			"os_version":    "Darwin 25.5.0",
			"arch":          "arm64",
		},
		"operator": map[string]any{"git_user": "Test <test@test.local>", "os_user": "test"},
	}
	if pid != 0 {
		m["pid"] = pid
	}
	if gs != nil {
		m["git_state"] = gs
	}
	return m
}

func defaultGitState() *capture.GitState {
	return &capture.GitState{Branch: "feat/test", HeadSHA: "abc123"}
}

// age backdates a wip's mtime — the scan judges idleness on mtime, so test
// fixtures written "in the past" must look it.
func age(t *testing.T, path string, d time.Duration) {
	t.Helper()
	old := time.Now().Add(-d)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

type testRefWriter struct {
	sessions []*capture.Session
}

func (w *testRefWriter) WriteSessionRef(_ string, session *capture.Session) error {
	w.sessions = append(w.sessions, session)
	return nil
}

func TestScanOrphaned_DetectsOldWIP(t *testing.T) {
	root := t.TempDir()
	oldTime := time.Now().Add(-5 * time.Hour)

	path := writeWIP(t, root, "01TEST_OLD", []capture.HookEvent{
		hookEv(t, oldTime, "session_start", startData("01TEST_OLD", 0, defaultGitState())),
	})
	age(t, path, 5*time.Hour)

	orphaned, err := ScanOrphaned(filepath.Join(root, ".etch", "sessions"), 4*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphaned) != 1 {
		t.Fatalf("expected 1 orphaned, got %d", len(orphaned))
	}
	if orphaned[0].SessionID != "01TEST_OLD" {
		t.Errorf("expected session ID 01TEST_OLD, got %s", orphaned[0].SessionID)
	}
	if orphaned[0].Reason != "timeout" {
		t.Errorf("expected reason timeout, got %s", orphaned[0].Reason)
	}
}

func TestScanOrphaned_IgnoresRecentWIP(t *testing.T) {
	root := t.TempDir()
	recentTime := time.Now().Add(-1 * time.Minute)

	// Fresh mtime (just written) — skipped on the stat alone.
	writeWIP(t, root, "01TEST_RECENT", []capture.HookEvent{
		hookEv(t, recentTime, "session_start", startData("01TEST_RECENT", 0, defaultGitState())),
	})

	orphaned, err := ScanOrphaned(filepath.Join(root, ".etch", "sessions"), 4*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphaned) != 0 {
		t.Fatalf("expected 0 orphaned, got %d", len(orphaned))
	}
}

// TestScanOrphaned_AlivePIDVetoesPastTimeout: THE finding-1 regression test.
// A session idle far past the timeout whose agent process is verifiably
// alive must never be orphaned — recovering it would destroy a live session.
func TestScanOrphaned_AlivePIDVetoesPastTimeout(t *testing.T) {
	root := t.TempDir()

	pid := os.Getpid()
	start, ok := capture.ProcessStartTime(pid)
	if !ok {
		t.Fatal("could not read own process start time")
	}
	data := startData("01ALIVE_IDLE", pid, defaultGitState())
	data["pid_start_time"] = start

	path := writeWIP(t, root, "01ALIVE_IDLE", []capture.HookEvent{
		hookEv(t, time.Now().Add(-30*time.Hour), "session_start", data),
	})
	age(t, path, 30*time.Hour) // way past the 4h timeout

	orphaned, err := ScanOrphaned(filepath.Join(root, ".etch", "sessions"), 4*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphaned) != 0 {
		t.Fatalf("alive agent must veto recovery even past timeout; got %d orphaned (%+v)", len(orphaned), orphaned)
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("live session's wip must remain on disk")
	}
}

// TestScanOrphaned_PIDReuseDoesNotVeto: an alive PID with a DIFFERENT start
// time is a recycled PID, not the agent — it cannot keep the wip alive.
func TestScanOrphaned_PIDReuseDoesNotVeto(t *testing.T) {
	root := t.TempDir()

	data := startData("01PID_REUSE", os.Getpid(), defaultGitState())
	data["pid_start_time"] = "Mon Jan  1 00:00:00 1990" // never this process

	path := writeWIP(t, root, "01PID_REUSE", []capture.HookEvent{
		hookEv(t, time.Now().Add(-10*time.Minute), "session_start", data),
	})
	age(t, path, 10*time.Minute)

	orphaned, err := ScanOrphaned(filepath.Join(root, ".etch", "sessions"), 4*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphaned) != 1 {
		t.Fatalf("recycled PID must not veto; got %d orphaned", len(orphaned))
	}
	if orphaned[0].Reason != "dead_pid" {
		t.Errorf("expected dead_pid, got %s", orphaned[0].Reason)
	}
}

func TestScanOrphaned_DetectsDeadPID(t *testing.T) {
	root := t.TempDir()
	recentTime := time.Now().Add(-10 * time.Minute)

	path := writeWIP(t, root, "01TEST_DEAD", []capture.HookEvent{
		hookEv(t, recentTime, "session_start", startData("01TEST_DEAD", 99999999, defaultGitState())),
	})
	age(t, path, 10*time.Minute) // past the activity grace, well before timeout

	orphaned, err := ScanOrphaned(filepath.Join(root, ".etch", "sessions"), 4*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphaned) != 1 {
		t.Fatalf("expected 1 orphaned, got %d", len(orphaned))
	}
	if orphaned[0].Reason != "dead_pid" {
		t.Errorf("expected reason dead_pid, got %s", orphaned[0].Reason)
	}
}

func TestScanOrphaned_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	orphaned, err := ScanOrphaned(dir, 4*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphaned) != 0 {
		t.Fatalf("expected 0 orphaned, got %d", len(orphaned))
	}
}

func TestScanOrphaned_NonExistentDir(t *testing.T) {
	orphaned, err := ScanOrphaned("/nonexistent/path", 4*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphaned) != 0 {
		t.Fatalf("expected 0 orphaned, got %d", len(orphaned))
	}
}

func TestScanOrphaned_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.wip.jsonl")
	os.WriteFile(path, []byte("this is not json\n"), 0o644)
	age(t, path, 6*time.Hour) // old enough to be considered — still skipped

	orphaned, err := ScanOrphaned(dir, 4*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt file has no valid events, so it's skipped
	if len(orphaned) != 0 {
		t.Fatalf("expected 0 orphaned (corrupt skipped), got %d", len(orphaned))
	}
}

func TestScanOrphaned_IgnoresNonWIPFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "session.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o644)

	orphaned, err := ScanOrphaned(dir, 4*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphaned) != 0 {
		t.Fatalf("expected 0 orphaned, got %d", len(orphaned))
	}
}

func TestRecoverSession_FullWIP(t *testing.T) {
	root := t.TempDir()
	startTime := time.Now().Add(-2 * time.Hour)

	writeWIP(t, root, "01FULL_SESSION", []capture.HookEvent{
		hookEv(t, startTime, "session_start", startData("01FULL_SESSION", 0, defaultGitState())),
		hookEv(t, startTime.Add(2*time.Second), "user_prompt_submit",
			capture.PromptData{Prompt: "Fix the bug in pagination", Source: "interactive"}),
		hookEv(t, startTime.Add(5*time.Second), "pre_tool_use",
			capture.ToolUseData{ToolName: "Read", ToolUseID: "tu_1"}),
		hookEv(t, startTime.Add(6*time.Second), "post_tool_use",
			capture.ToolUseData{ToolName: "Read", ToolUseID: "tu_1"}),
		hookEv(t, startTime.Add(8*time.Second), "pre_tool_use",
			capture.ToolUseData{ToolName: "Read", ToolUseID: "tu_2"}),
		hookEv(t, startTime.Add(10*time.Second), "pre_tool_use",
			capture.ToolUseData{ToolName: "Edit", ToolUseID: "tu_3"}),
	})

	session, err := RecoverSession(root, "01FULL_SESSION")
	if err != nil {
		t.Fatal(err)
	}

	if session.SchemaVersion != capture.SchemaVersion {
		t.Errorf("expected schema version %s, got %s", capture.SchemaVersion, session.SchemaVersion)
	}
	if session.SessionID != "01FULL_SESSION" {
		t.Errorf("expected session ID 01FULL_SESSION, got %s", session.SessionID)
	}
	if session.Status != "incomplete" {
		t.Errorf("expected status incomplete, got %s", session.Status)
	}
	if session.ExitReason != "crash" {
		t.Errorf("expected exit_reason crash, got %s", session.ExitReason)
	}
	if session.Timing.EndedAt != nil {
		t.Error("expected ended_at to be nil")
	}
	if session.Timing.DurationMs != nil {
		t.Error("expected duration_ms to be nil")
	}
	if session.Agent.Runtime != "claude-code" {
		t.Errorf("expected runtime claude-code, got %s", session.Agent.Runtime)
	}
	if session.Agent.Model == nil || *session.Agent.Model != "claude-opus-4-7" {
		t.Error("expected model claude-opus-4-7")
	}
	if session.Prompt == nil {
		t.Fatal("expected prompt to be set")
	}
	if session.Prompt.Text != "Fix the bug in pagination" {
		t.Errorf("expected prompt text, got %s", session.Prompt.Text)
	}
	if session.GitStart == nil {
		t.Fatal("expected git_start to be set")
	}
	if session.GitStart.Branch != "feat/test" {
		t.Errorf("expected branch feat/test, got %s", session.GitStart.Branch)
	}
	// git_end is the wip's last known snapshot: a copy of git_start, no
	// commits_produced, never live-captured (OUTPUT_SPEC §2c, plan-review R1).
	if session.GitEnd == nil {
		t.Fatal("expected git_end to be set (copy of git_start)")
	}
	if session.GitEnd.HeadSHA != session.GitStart.HeadSHA {
		t.Errorf("crash git_end.head_sha: want %s (== git_start), got %s", session.GitStart.HeadSHA, session.GitEnd.HeadSHA)
	}
	if len(session.GitEnd.CommitsProduced) != 0 {
		t.Errorf("crash git_end must have no commits_produced, got %v", session.GitEnd.CommitsProduced)
	}
	if session.ToolUse.TotalCalls != 3 {
		t.Errorf("expected 3 total calls (pre only), got %d", session.ToolUse.TotalCalls)
	}
	if session.ToolUse.ByTool["Read"] != 2 {
		t.Errorf("expected 2 Read calls, got %d", session.ToolUse.ByTool["Read"])
	}
	if session.ToolUse.ByTool["Edit"] != 1 {
		t.Errorf("expected 1 Edit call, got %d", session.ToolUse.ByTool["Edit"])
	}
	if session.Tokens != nil {
		t.Error("tokens must stay null (v1 spec: tokens are null/reserved)")
	}
	if session.Orchestration.Type != "manual" {
		t.Errorf("expected orchestration type manual, got %s", session.Orchestration.Type)
	}
}

// TestRecoverSession_ReDeliveredToolUse: the same pre_tool_use delivered twice
// (duplicate hook invocation under load) must count once.
func TestRecoverSession_ReDeliveredToolUse(t *testing.T) {
	root := t.TempDir()
	startTime := time.Now().Add(-5 * time.Hour)

	dup := capture.ToolUseData{ToolName: "Bash", ToolUseID: "tu_dup"}
	writeWIP(t, root, "01REDELIVER", []capture.HookEvent{
		hookEv(t, startTime, "session_start", startData("01REDELIVER", 0, defaultGitState())),
		hookEv(t, startTime.Add(1*time.Second), "pre_tool_use", dup),
		hookEv(t, startTime.Add(1*time.Second), "pre_tool_use", dup),
	})

	session, err := RecoverSession(root, "01REDELIVER")
	if err != nil {
		t.Fatal(err)
	}
	if session.ToolUse.TotalCalls != 1 {
		t.Errorf("re-delivered pre_tool_use must count once, got %d", session.ToolUse.TotalCalls)
	}
	if session.ToolUse.ByTool["Bash"] != 1 {
		t.Errorf("expected 1 Bash call, got %d", session.ToolUse.ByTool["Bash"])
	}
}

// TestRecoverSession_WithEndEvent: a wip retained after a failed commit
// (ETCH-40 finding 8) contains the real end event — recovery must commit the
// truthful complete/normal record, not a 'crash' falsification.
func TestRecoverSession_WithEndEvent(t *testing.T) {
	root := t.TempDir()
	startTime := time.Now().Add(-5 * time.Hour)
	endTime := startTime.Add(90 * time.Second)

	endGit := &capture.GitState{Branch: "feat/test", HeadSHA: "def456", CommitsProduced: []string{"def456"}}
	writeWIP(t, root, "01HASEND", []capture.HookEvent{
		hookEv(t, startTime, "session_start", startData("01HASEND", 0, defaultGitState())),
		hookEv(t, endTime, "session_end", capture.SessionEndData{GitState: endGit, ExitReason: "normal"}),
	})

	session, err := RecoverSession(root, "01HASEND")
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != "complete" {
		t.Errorf("retained-wip recovery: want status complete, got %s", session.Status)
	}
	if session.ExitReason != "normal" {
		t.Errorf("retained-wip recovery: want exit_reason normal, got %s", session.ExitReason)
	}
	if session.GitEnd == nil || session.GitEnd.HeadSHA != "def456" {
		t.Error("git_end must come from the recorded end event")
	}
	if len(session.Outcome.Commits) != 1 || session.Outcome.Commits[0] != "def456" {
		t.Errorf("outcome.commits must come from the recorded end event, got %v", session.Outcome.Commits)
	}
	if session.Timing.EndedAt == nil {
		t.Fatal("ended_at must be set from the recorded end event")
	}
	if session.Timing.DurationMs == nil {
		t.Fatal("duration must be computed for a recorded end")
	}
	if *session.Timing.DurationMs != 90_000 {
		t.Errorf("duration: want 90000ms, got %d", *session.Timing.DurationMs)
	}
}

// TestRecoveryParity_HasEnd: the strongest reducer regression guard — the
// SAME event stream through Finalize and RecoverSession must produce the
// same files_touched / duration / git_end / counts (plan-review R4: parity
// is asserted on the hasEnd path, where it is meaningful).
func TestRecoveryParity_HasEnd(t *testing.T) {
	// Real git repo with a commit between start and end SHA.
	repo := t.TempDir()
	mustGit := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	mustGit("init", "-q")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\n"), 0o644)
	mustGit("add", ".")
	mustGit("commit", "-q", "-m", "start")
	startSHA := mustGit("rev-parse", "HEAD")
	os.WriteFile(filepath.Join(repo, "b.txt"), []byte("two\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one+\n"), 0o644)
	mustGit("add", ".")
	mustGit("commit", "-q", "-m", "work")
	endSHA := mustGit("rev-parse", "HEAD")

	startTime := time.Now().Add(-5 * time.Hour)
	endTime := startTime.Add(42 * time.Second)
	gitStart := &capture.GitState{Branch: "main", HeadSHA: startSHA, WorktreePath: repo}
	gitEnd := &capture.GitState{Branch: "main", HeadSHA: endSHA, WorktreePath: repo, CommitsProduced: []string{endSHA}}

	events := []capture.HookEvent{
		hookEv(t, startTime, "session_start", startData("01PARITY", 0, gitStart)),
		hookEv(t, startTime.Add(1*time.Second), "user_prompt_submit",
			capture.PromptData{Prompt: "do work", Source: "interactive"}),
		hookEv(t, startTime.Add(2*time.Second), "pre_tool_use",
			capture.ToolUseData{ToolName: "Edit", ToolUseID: "tu_1", FilePath: filepath.Join(repo, "a.txt")}),
		hookEv(t, endTime, "session_end", capture.SessionEndData{GitState: gitEnd, ExitReason: "normal"}),
	}

	// Path A: the normal finalize path.
	rootA := t.TempDir()
	writeWIP(t, rootA, "01PARITY", events)
	finalized, err := capture.Finalize(rootA, repo, "01PARITY")
	if err != nil {
		t.Fatal(err)
	}

	// Path B: recovery on an identical wip.
	rootB := t.TempDir()
	writeWIP(t, rootB, "01PARITY", events)
	recovered, err := RecoverSession(rootB, "01PARITY")
	if err != nil {
		t.Fatal(err)
	}

	a, err := json.Marshal(finalized)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(recovered)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Errorf("recovery diverged from Finalize for the same events:\nfinalize: %s\nrecovery: %s", a, b)
	}

	// And both saw the real diff, not the fallback.
	if len(finalized.FilesTouched) != 2 {
		t.Fatalf("expected 2 files touched via git diff, got %v", finalized.FilesTouched)
	}
	if finalized.Timing.DurationMs == nil || *finalized.Timing.DurationMs != 42_000 {
		t.Error("expected 42000ms duration")
	}
}

func TestRecoverSession_MinimalWIP(t *testing.T) {
	root := t.TempDir()
	writeWIP(t, root, "01MINIMAL", []capture.HookEvent{
		hookEv(t, time.Now().Add(-1*time.Hour), "session_start", map[string]any{
			"session_id": "01MINIMAL",
			"agent":      map[string]any{"runtime": "codex"},
		}),
	})

	session, err := RecoverSession(root, "01MINIMAL")
	if err != nil {
		t.Fatal(err)
	}

	if session.SessionID != "01MINIMAL" {
		t.Errorf("expected session ID 01MINIMAL, got %s", session.SessionID)
	}
	if session.Status != "incomplete" {
		t.Errorf("expected incomplete, got %s", session.Status)
	}
	if session.ExitReason != "crash" {
		t.Errorf("expected crash, got %s", session.ExitReason)
	}
	if session.Agent.Runtime != "codex" {
		t.Errorf("expected runtime codex, got %s", session.Agent.Runtime)
	}
	if session.Prompt != nil {
		t.Error("expected no prompt for minimal session")
	}
	if session.ToolUse.TotalCalls != 0 {
		t.Error("expected no tool calls for minimal session")
	}
	if session.Tokens != nil {
		t.Error("expected no tokens for minimal session")
	}
}

func TestRecoverSession_EmptyFile(t *testing.T) {
	root := t.TempDir()
	writeWIP(t, root, "01EMPTY", nil)

	_, err := RecoverSession(root, "01EMPTY")
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestRecoverSession_CorruptLines(t *testing.T) {
	root := t.TempDir()
	startTime := time.Now().Add(-1 * time.Hour)

	path := writeWIP(t, root, "01CORRUPT_MIX", []capture.HookEvent{
		hookEv(t, startTime, "session_start", startData("01CORRUPT_MIX", 0, defaultGitState())),
	})
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("this line is garbage\n")
	f.WriteString("{\"invalid json\n")
	promptLine, _ := json.Marshal(hookEv(t, startTime.Add(time.Second), "user_prompt_submit",
		capture.PromptData{Prompt: "Valid prompt after corrupt lines", Source: "interactive"}))
	f.Write(append(promptLine, '\n'))
	f.Close()

	session, err := RecoverSession(root, "01CORRUPT_MIX")
	if err != nil {
		t.Fatal(err)
	}
	if session.SessionID != "01CORRUPT_MIX" {
		t.Errorf("expected 01CORRUPT_MIX, got %s", session.SessionID)
	}
	if session.Prompt == nil || session.Prompt.Text != "Valid prompt after corrupt lines" {
		t.Error("expected prompt from the valid line after corrupt lines")
	}
}

func TestRecoverSession_SessionIDFromFilename(t *testing.T) {
	root := t.TempDir()
	// No session_start event at all — the ULID comes from the filename.
	writeWIP(t, root, "01FROM_FILENAME", []capture.HookEvent{
		hookEv(t, time.Now(), "user_prompt_submit", capture.PromptData{Prompt: "hello", Source: "interactive"}),
	})

	session, err := RecoverSession(root, "01FROM_FILENAME")
	if err != nil {
		t.Fatal(err)
	}
	if session.SessionID != "01FROM_FILENAME" {
		t.Errorf("expected 01FROM_FILENAME, got %s", session.SessionID)
	}
}

func TestRecoverSession_WithOrchestration(t *testing.T) {
	root := t.TempDir()
	data := startData("01ORCH_TEST", 0, defaultGitState())
	data["orchestration"] = map[string]any{
		"type":             "lattice-orchestrator",
		"dispatch_method":  "c11_delegator",
		"ticket_id":        "FT-481",
		"run_id":           "01RUN_ID",
		"role":             "implementer",
		"workflow_version": "abc123",
		"extra":            map[string]any{},
	}
	data["parent_session_id"] = "01PARENT"
	writeWIP(t, root, "01ORCH_TEST", []capture.HookEvent{
		hookEv(t, time.Now().Add(-1*time.Hour), "session_start", data),
	})

	session, err := RecoverSession(root, "01ORCH_TEST")
	if err != nil {
		t.Fatal(err)
	}

	orch := session.Orchestration
	if orch.Type != "lattice-orchestrator" {
		t.Errorf("expected lattice-orchestrator, got %s", orch.Type)
	}
	if orch.DispatchMethod == nil || *orch.DispatchMethod != "c11_delegator" {
		t.Error("expected dispatch_method c11_delegator")
	}
	if orch.TicketID == nil || *orch.TicketID != "FT-481" {
		t.Error("expected ticket_id FT-481")
	}
	if session.ParentSessionID == nil || *session.ParentSessionID != "01PARENT" {
		t.Error("expected parent_session_id 01PARENT")
	}
}

func TestCleanupWIP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wip.jsonl")
	os.WriteFile(path, []byte("{}"), 0o644)

	if _, err := os.Stat(path); err != nil {
		t.Fatal("file should exist before cleanup")
	}

	if err := CleanupWIP(path); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should not exist after cleanup")
	}
}

func TestCleanupWIP_NonExistent(t *testing.T) {
	err := CleanupWIP("/nonexistent/file.wip.jsonl")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestRecoverAll_Integration(t *testing.T) {
	root := t.TempDir()
	oldTime := time.Now().Add(-5 * time.Hour)

	path := writeWIP(t, root, "01RECOVER_ALL", []capture.HookEvent{
		hookEv(t, oldTime, "session_start", startData("01RECOVER_ALL", 0, defaultGitState())),
		hookEv(t, oldTime.Add(2*time.Second), "user_prompt_submit",
			capture.PromptData{Prompt: "Do the thing", Source: "interactive"}),
	})
	age(t, path, 5*time.Hour)

	// A mapping pointing at the orphan, and a stale session.json scratch file —
	// recovery must clean up all three (wip, mapping, scratch).
	mapDir := filepath.Join(root, ".etch", "sessions", ".map")
	os.MkdirAll(mapDir, 0o755)
	os.WriteFile(filepath.Join(mapDir, "upstream-id-1"), []byte("01RECOVER_ALL"), 0o644)
	os.WriteFile(filepath.Join(root, ".etch", "sessions", "01RECOVER_ALL.session.json"), []byte("{}"), 0o644)

	writer := &testRefWriter{}
	count, err := RecoverAll(root, 4*time.Hour, writer, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 recovered, got %d", count)
	}
	if len(writer.sessions) != 1 {
		t.Fatalf("expected 1 captured session, got %d", len(writer.sessions))
	}

	session := writer.sessions[0]
	if session.SessionID != "01RECOVER_ALL" {
		t.Errorf("expected 01RECOVER_ALL, got %s", session.SessionID)
	}
	if session.Status != "incomplete" {
		t.Errorf("expected incomplete, got %s", session.Status)
	}
	if session.Prompt == nil || session.Prompt.Text != "Do the thing" {
		t.Error("expected prompt to be captured")
	}

	sessionsDir := filepath.Join(root, ".etch", "sessions")
	entries, _ := os.ReadDir(sessionsDir)
	for _, e := range entries {
		if e.Name() != ".map" {
			t.Errorf("expected only .map left in sessions dir, found %s", e.Name())
		}
	}
	if _, err := os.Stat(filepath.Join(mapDir, "upstream-id-1")); !os.IsNotExist(err) {
		t.Error("stale mapping must be removed after recovery")
	}
}

// TestRecoverAll_SkipsActiveSession: the idle-timeout false positive from
// the deep review — a live (alive, verified PID) idle session must survive a
// sibling's RecoverAll untouched: no ref written, wip intact.
func TestRecoverAll_SkipsActiveSession(t *testing.T) {
	root := t.TempDir()

	pid := os.Getpid()
	start, ok := capture.ProcessStartTime(pid)
	if !ok {
		t.Fatal("could not read own process start time")
	}
	data := startData("01ACTIVE", pid, defaultGitState())
	data["pid_start_time"] = start

	path := writeWIP(t, root, "01ACTIVE", []capture.HookEvent{
		hookEv(t, time.Now().Add(-6*time.Hour), "session_start", data),
	})
	age(t, path, 6*time.Hour) // idle past the timeout, but the agent is alive

	writer := &testRefWriter{}
	count, err := RecoverAll(root, 4*time.Hour, writer, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 recovered, got %d", count)
	}
	if len(writer.sessions) != 0 {
		t.Fatalf("no ref may be written for a live session, got %d", len(writer.sessions))
	}

	if _, err := os.Stat(path); err != nil {
		t.Error("live session's wip must remain intact")
	}
}

func TestRecoverAll_RespectsExclude(t *testing.T) {
	root := t.TempDir()
	oldTime := time.Now().Add(-6 * time.Hour)

	p1 := writeWIP(t, root, "01EXCLUDED", []capture.HookEvent{
		hookEv(t, oldTime, "session_start", startData("01EXCLUDED", 0, defaultGitState())),
	})
	p2 := writeWIP(t, root, "01FAIRGAME", []capture.HookEvent{
		hookEv(t, oldTime, "session_start", startData("01FAIRGAME", 0, defaultGitState())),
	})
	age(t, p1, 6*time.Hour)
	age(t, p2, 6*time.Hour)

	writer := &testRefWriter{}
	count, err := RecoverAll(root, 4*time.Hour, writer, map[string]bool{"01EXCLUDED": true})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 recovered (excluded skipped), got %d", count)
	}
	if writer.sessions[0].SessionID != "01FAIRGAME" {
		t.Errorf("recovered the wrong session: %s", writer.sessions[0].SessionID)
	}
	if _, err := os.Stat(filepath.Join(root, ".etch", "sessions", "01EXCLUDED.wip.jsonl")); err != nil {
		t.Error("excluded wip must remain on disk")
	}
}

func TestReadTimeoutFromSettings_Default(t *testing.T) {
	dir := t.TempDir()
	timeout := ReadTimeoutFromSettings(dir)
	if timeout != 4*time.Hour {
		t.Errorf("expected 4h default, got %v", timeout)
	}
}

func TestReadTimeoutFromSettings_Custom(t *testing.T) {
	dir := t.TempDir()
	etchDir := filepath.Join(dir, ".etch")
	os.MkdirAll(etchDir, 0o755)
	os.WriteFile(filepath.Join(etchDir, "settings.json"), []byte(`{"recovery_timeout_hours": 2}`), 0o644)

	timeout := ReadTimeoutFromSettings(dir)
	if timeout != 2*time.Hour {
		t.Errorf("expected 2h, got %v", timeout)
	}
}

func TestReadTimeoutFromSettings_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	etchDir := filepath.Join(dir, ".etch")
	os.MkdirAll(etchDir, 0o755)
	os.WriteFile(filepath.Join(etchDir, "settings.json"), []byte("not json"), 0o644)

	timeout := ReadTimeoutFromSettings(dir)
	if timeout != 4*time.Hour {
		t.Errorf("expected 4h default, got %v", timeout)
	}
}

func TestReadTimeoutFromSettings_FractionalHours(t *testing.T) {
	dir := t.TempDir()
	etchDir := filepath.Join(dir, ".etch")
	os.MkdirAll(etchDir, 0o755)
	os.WriteFile(filepath.Join(etchDir, "settings.json"), []byte(`{"recovery_timeout_hours": 0.5}`), 0o644)

	timeout := ReadTimeoutFromSettings(dir)
	if timeout != 30*time.Minute {
		t.Errorf("expected 30m, got %v", timeout)
	}
}

func TestRecoverSession_JSONSerialization(t *testing.T) {
	root := t.TempDir()
	writeWIP(t, root, "01JSON_TEST", []capture.HookEvent{
		hookEv(t, time.Now().Add(-1*time.Hour), "session_start", startData("01JSON_TEST", 0, defaultGitState())),
	})

	session, err := RecoverSession(root, "01JSON_TEST")
	if err != nil {
		t.Fatal(err)
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["status"] != "incomplete" {
		t.Errorf("round-trip status: want incomplete, got %v", parsed["status"])
	}
	// pid is wip-only recovery metadata and must never reach the record.
	if _, ok := parsed["pid"]; ok {
		t.Error("pid must not appear in the serialized session record")
	}
}
