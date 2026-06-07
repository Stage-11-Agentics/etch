package recovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"forgejo.stage11.ai/s11/etch/internal/schema"
)

func writeWIPFile(t *testing.T, dir, sessionID string, events []wipEvent) string {
	t.Helper()
	path := filepath.Join(dir, sessionID+".wip.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, ev := range events {
		data, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(f, "%s\n", data)
	}
	return path
}

func makeSessionStartEvent(sessionID string, ts time.Time) wipEvent {
	return wipEvent{
		HookType:  "session_start",
		SessionID: sessionID,
		Timestamp: ts.UTC().Format(time.RFC3339),
		PID:       os.Getpid(), // current process, so it's alive
		Runtime:   "claude-code",
		Model:     "claude-opus-4-7",
		Version:   "1.0.33",
		Branch:    "feat/test",
		HeadSHA:   "abc123",
		OS:        "darwin",
		Arch:      "arm64",
		GitUser:   "Test <test@test.local>",
		OSUser:    "test",
	}
}

func TestScanOrphaned_DetectsOldWIP(t *testing.T) {
	dir := t.TempDir()
	oldTime := time.Now().Add(-5 * time.Hour)

	ev := makeSessionStartEvent("01TEST_OLD", oldTime)
	ev.PID = 0 // no PID, so timeout check applies
	writeWIPFile(t, dir, "01TEST_OLD", []wipEvent{ev})

	orphaned, err := ScanOrphaned(dir, 4*time.Hour)
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
	dir := t.TempDir()
	recentTime := time.Now().Add(-1 * time.Hour)

	ev := makeSessionStartEvent("01TEST_RECENT", recentTime)
	// PID is os.Getpid(), so it's alive — should be skipped
	writeWIPFile(t, dir, "01TEST_RECENT", []wipEvent{ev})

	orphaned, err := ScanOrphaned(dir, 4*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphaned) != 0 {
		t.Fatalf("expected 0 orphaned, got %d", len(orphaned))
	}
}

func TestScanOrphaned_DetectsDeadPID(t *testing.T) {
	dir := t.TempDir()
	recentTime := time.Now().Add(-10 * time.Minute)

	ev := makeSessionStartEvent("01TEST_DEAD", recentTime)
	ev.PID = 99999999 // almost certainly not a real PID
	writeWIPFile(t, dir, "01TEST_DEAD", []wipEvent{ev})

	orphaned, err := ScanOrphaned(dir, 4*time.Hour)
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
	os.WriteFile(path, []byte("this is not json\n"), 0644)

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
	os.WriteFile(filepath.Join(dir, "session.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0644)

	orphaned, err := ScanOrphaned(dir, 4*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphaned) != 0 {
		t.Fatalf("expected 0 orphaned, got %d", len(orphaned))
	}
}

func TestRecoverSession_FullWIP(t *testing.T) {
	dir := t.TempDir()
	startTime := time.Now().Add(-2 * time.Hour)

	events := []wipEvent{
		makeSessionStartEvent("01FULL_SESSION", startTime),
		{
			HookType:  "user_prompt_submit",
			SessionID: "01FULL_SESSION",
			Timestamp: startTime.Add(2 * time.Second).UTC().Format(time.RFC3339),
			Prompt:    "Fix the bug in pagination",
			PromptSource: "interactive",
		},
		{
			HookType:  "pre_tool_use",
			SessionID: "01FULL_SESSION",
			Timestamp: startTime.Add(5 * time.Second).UTC().Format(time.RFC3339),
			ToolName:  "Read",
		},
		{
			HookType:  "post_tool_use",
			SessionID: "01FULL_SESSION",
			Timestamp: startTime.Add(6 * time.Second).UTC().Format(time.RFC3339),
			ToolName:  "Read",
		},
		{
			HookType:  "pre_tool_use",
			SessionID: "01FULL_SESSION",
			Timestamp: startTime.Add(10 * time.Second).UTC().Format(time.RFC3339),
			ToolName:  "Edit",
		},
	}

	path := writeWIPFile(t, dir, "01FULL_SESSION", events)
	session, err := RecoverSession(path)
	if err != nil {
		t.Fatal(err)
	}

	if session.SchemaVersion != schema.SchemaVersion {
		t.Errorf("expected schema version %s, got %s", schema.SchemaVersion, session.SchemaVersion)
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
	if session.Timing.DurationMS != nil {
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
	if session.Prompt.Source != "interactive" {
		t.Errorf("expected prompt source interactive, got %s", session.Prompt.Source)
	}
	if session.GitStart == nil {
		t.Fatal("expected git_start to be set")
	}
	if session.GitStart.Branch != "feat/test" {
		t.Errorf("expected branch feat/test, got %s", session.GitStart.Branch)
	}
	if session.ToolUse == nil {
		t.Fatal("expected tool_use to be set")
	}
	if session.ToolUse.ByTool["Read"] != 2 {
		t.Errorf("expected 2 Read calls, got %d", session.ToolUse.ByTool["Read"])
	}
	if session.ToolUse.ByTool["Edit"] != 1 {
		t.Errorf("expected 1 Edit call, got %d", session.ToolUse.ByTool["Edit"])
	}
	if session.Tokens != nil {
		t.Error("tokens is reserved in v1 — recovered sessions must have tokens=null")
	}
	if session.Orchestration == nil {
		t.Fatal("expected orchestration to be set")
	}
	if session.Orchestration.Type != "manual" {
		t.Errorf("expected orchestration type manual, got %s", session.Orchestration.Type)
	}
}

func TestRecoverSession_MinimalWIP(t *testing.T) {
	dir := t.TempDir()
	events := []wipEvent{
		{
			HookType:  "session_start",
			SessionID: "01MINIMAL",
			Timestamp: time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339),
			Runtime:   "codex",
		},
	}

	path := writeWIPFile(t, dir, "01MINIMAL", events)
	session, err := RecoverSession(path)
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
	if session.ToolUse != nil {
		t.Error("expected no tool_use for minimal session")
	}
	if session.Tokens != nil {
		t.Error("expected no tokens for minimal session")
	}
}

func TestRecoverSession_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.wip.jsonl")
	os.WriteFile(path, []byte(""), 0644)

	_, err := RecoverSession(path)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestRecoverSession_CorruptLines(t *testing.T) {
	dir := t.TempDir()
	startEvent := makeSessionStartEvent("01CORRUPT_MIX", time.Now().Add(-1*time.Hour))

	path := filepath.Join(dir, "01CORRUPT_MIX.wip.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(startEvent)
	fmt.Fprintf(f, "%s\n", data)
	fmt.Fprintf(f, "this line is garbage\n")
	fmt.Fprintf(f, "{\"invalid json\n")
	promptEvent := wipEvent{
		HookType:  "user_prompt_submit",
		SessionID: "01CORRUPT_MIX",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Prompt:    "Valid prompt after corrupt lines",
	}
	data, _ = json.Marshal(promptEvent)
	fmt.Fprintf(f, "%s\n", data)
	f.Close()

	session, err := RecoverSession(path)
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

func TestRecoverSession_SetsIncompleteStatus(t *testing.T) {
	dir := t.TempDir()
	events := []wipEvent{makeSessionStartEvent("01STATUS_CHECK", time.Now())}
	path := writeWIPFile(t, dir, "01STATUS_CHECK", events)

	session, err := RecoverSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != "incomplete" {
		t.Errorf("status: want incomplete, got %s", session.Status)
	}
	if session.ExitReason != "crash" {
		t.Errorf("exit_reason: want crash, got %s", session.ExitReason)
	}
	if session.Timing.EndedAt != nil {
		t.Errorf("ended_at: want nil, got %v", session.Timing.EndedAt)
	}
	if session.Timing.DurationMS != nil {
		t.Errorf("duration_ms: want nil, got %v", session.Timing.DurationMS)
	}
}

func TestRecoverSession_SessionIDFromFilename(t *testing.T) {
	dir := t.TempDir()
	// Events with no session_id — should fall back to filename
	events := []wipEvent{
		{
			HookType:  "user_prompt_submit",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Prompt:    "hello",
		},
	}
	path := writeWIPFile(t, dir, "01FROM_FILENAME", events)

	session, err := RecoverSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if session.SessionID != "01FROM_FILENAME" {
		t.Errorf("expected 01FROM_FILENAME, got %s", session.SessionID)
	}
}

func TestRecoverSession_WithOrchestration(t *testing.T) {
	dir := t.TempDir()
	ev := makeSessionStartEvent("01ORCH_TEST", time.Now().Add(-1*time.Hour))
	ev.OrchestrationType = "lattice-orchestrator"
	ev.DispatchMethod = "c11_delegator"
	ev.TicketID = "FT-481"
	ev.RunID = "01RUN_ID"
	ev.AgentRole = "implementer"
	ev.ParentSessionID = "01PARENT"
	ev.WorkflowVersion = "abc123"

	path := writeWIPFile(t, dir, "01ORCH_TEST", []wipEvent{ev})
	session, err := RecoverSession(path)
	if err != nil {
		t.Fatal(err)
	}

	orch := session.Orchestration
	if orch == nil {
		t.Fatal("expected orchestration")
	}
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
	os.WriteFile(path, []byte("{}"), 0644)

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
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, ".etch", "sessions")
	os.MkdirAll(sessionsDir, 0755)

	oldTime := time.Now().Add(-5 * time.Hour)
	ev := makeSessionStartEvent("01RECOVER_ALL", oldTime)
	ev.PID = 0
	writeWIPFile(t, sessionsDir, "01RECOVER_ALL", []wipEvent{
		ev,
		{
			HookType:  "user_prompt_submit",
			SessionID: "01RECOVER_ALL",
			Timestamp: oldTime.Add(2 * time.Second).UTC().Format(time.RFC3339),
			Prompt:    "Do the thing",
		},
	})

	var capturedSessions []*schema.Session
	writer := &testRefWriter{sessions: &capturedSessions}

	count, err := RecoverAll(sessionsDir, dir, 4*time.Hour, writer)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 recovered, got %d", count)
	}
	if len(capturedSessions) != 1 {
		t.Fatalf("expected 1 captured session, got %d", len(capturedSessions))
	}

	session := capturedSessions[0]
	if session.SessionID != "01RECOVER_ALL" {
		t.Errorf("expected 01RECOVER_ALL, got %s", session.SessionID)
	}
	if session.Status != "incomplete" {
		t.Errorf("expected incomplete, got %s", session.Status)
	}
	if session.Prompt == nil || session.Prompt.Text != "Do the thing" {
		t.Error("expected prompt to be captured")
	}

	// Verify .wip file was cleaned up
	entries, _ := os.ReadDir(sessionsDir)
	if len(entries) != 0 {
		t.Errorf("expected sessionsDir to be empty after recovery, got %d files", len(entries))
	}
}

func TestRecoverAll_SkipsActiveSession(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, ".etch", "sessions")
	os.MkdirAll(sessionsDir, 0755)

	recentTime := time.Now().Add(-10 * time.Minute)
	ev := makeSessionStartEvent("01ACTIVE", recentTime)
	// PID is os.Getpid(), which is alive, and timestamp is recent
	writeWIPFile(t, sessionsDir, "01ACTIVE", []wipEvent{ev})

	var capturedSessions []*schema.Session
	writer := &testRefWriter{sessions: &capturedSessions}

	count, err := RecoverAll(sessionsDir, dir, 4*time.Hour, writer)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 recovered, got %d", count)
	}

	// .wip file should still be there
	entries, _ := os.ReadDir(sessionsDir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file still in sessions dir, got %d", len(entries))
	}
}

func TestRecoverAll_MixedFiles(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, ".etch", "sessions")
	os.MkdirAll(sessionsDir, 0755)

	// Old orphaned file
	oldEv := makeSessionStartEvent("01OLD_ORPHAN", time.Now().Add(-6*time.Hour))
	oldEv.PID = 0
	writeWIPFile(t, sessionsDir, "01OLD_ORPHAN", []wipEvent{oldEv})

	// Recent active file
	recentEv := makeSessionStartEvent("01RECENT_ACTIVE", time.Now().Add(-30*time.Minute))
	writeWIPFile(t, sessionsDir, "01RECENT_ACTIVE", []wipEvent{recentEv})

	// Dead PID file (recent timestamp)
	deadEv := makeSessionStartEvent("01DEAD_PID", time.Now().Add(-5*time.Minute))
	deadEv.PID = 99999999
	writeWIPFile(t, sessionsDir, "01DEAD_PID", []wipEvent{deadEv})

	var capturedSessions []*schema.Session
	writer := &testRefWriter{sessions: &capturedSessions}

	count, err := RecoverAll(sessionsDir, dir, 4*time.Hour, writer)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 recovered, got %d", count)
	}

	// Only the active session's .wip should remain
	entries, _ := os.ReadDir(sessionsDir)
	if len(entries) != 1 {
		t.Errorf("expected 1 remaining file, got %d", len(entries))
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
	os.MkdirAll(etchDir, 0755)
	os.WriteFile(filepath.Join(etchDir, "settings.json"), []byte(`{"recovery_timeout_hours": 2}`), 0644)

	timeout := ReadTimeoutFromSettings(dir)
	if timeout != 2*time.Hour {
		t.Errorf("expected 2h, got %v", timeout)
	}
}

func TestReadTimeoutFromSettings_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	etchDir := filepath.Join(dir, ".etch")
	os.MkdirAll(etchDir, 0755)
	os.WriteFile(filepath.Join(etchDir, "settings.json"), []byte("not json"), 0644)

	timeout := ReadTimeoutFromSettings(dir)
	if timeout != 4*time.Hour {
		t.Errorf("expected 4h default, got %v", timeout)
	}
}

func TestReadTimeoutFromSettings_FractionalHours(t *testing.T) {
	dir := t.TempDir()
	etchDir := filepath.Join(dir, ".etch")
	os.MkdirAll(etchDir, 0755)
	os.WriteFile(filepath.Join(etchDir, "settings.json"), []byte(`{"recovery_timeout_hours": 0.5}`), 0644)

	timeout := ReadTimeoutFromSettings(dir)
	if timeout != 30*time.Minute {
		t.Errorf("expected 30m, got %v", timeout)
	}
}

func TestRecoverSession_JSONSerialization(t *testing.T) {
	dir := t.TempDir()
	events := []wipEvent{makeSessionStartEvent("01JSON_TEST", time.Now().Add(-1*time.Hour))}
	path := writeWIPFile(t, dir, "01JSON_TEST", events)

	session, err := RecoverSession(path)
	if err != nil {
		t.Fatal(err)
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("session JSON is not valid: %v", err)
	}

	if parsed["schema_version"] != "etch.session.v1" {
		t.Error("schema_version mismatch in JSON")
	}
	if parsed["status"] != "incomplete" {
		t.Error("status mismatch in JSON")
	}
	if parsed["exit_reason"] != "crash" {
		t.Error("exit_reason mismatch in JSON")
	}

	timing := parsed["timing"].(map[string]any)
	if timing["ended_at"] != nil {
		t.Error("ended_at should be null in JSON")
	}
	if timing["duration_ms"] != nil {
		t.Error("duration_ms should be null in JSON")
	}
}

type testRefWriter struct {
	sessions *[]*schema.Session
}

func (w *testRefWriter) WriteSessionRef(_ string, session *schema.Session) error {
	*w.sessions = append(*w.sessions, session)
	return nil
}
