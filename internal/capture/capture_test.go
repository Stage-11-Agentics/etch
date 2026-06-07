package capture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"forgejo.stage11.ai/s11/etch/internal/config"
)

func TestEnsureDirs(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDirs(dir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".etch", "sessions")); err != nil {
		t.Error("expected .etch/sessions/ to exist")
	}
	if _, err := os.Stat(filepath.Join(dir, ".etch", "sessions", ".map")); err != nil {
		t.Error("expected .etch/sessions/.map/ to exist")
	}
}

func TestMapping(t *testing.T) {
	dir := t.TempDir()
	EnsureDirs(dir)

	WriteMapping(dir, "entire-abc-123", "01ABC")
	got := LookupMapping(dir, "entire-abc-123")
	if got != "01ABC" {
		t.Errorf("expected 01ABC, got %q", got)
	}

	// Missing mapping
	got = LookupMapping(dir, "nonexistent")
	if got != "" {
		t.Errorf("expected empty for missing mapping, got %q", got)
	}

	// Empty session ID
	got = LookupMapping(dir, "")
	if got != "" {
		t.Errorf("expected empty for empty session ID, got %q", got)
	}

	CleanupMapping(dir, "entire-abc-123")
	got = LookupMapping(dir, "entire-abc-123")
	if got != "" {
		t.Errorf("expected empty after cleanup, got %q", got)
	}
}

func TestAppendAndReadEvents(t *testing.T) {
	dir := t.TempDir()
	EnsureDirs(dir)

	sessionID := "01TEST"

	// Append a few events
	err := AppendEvent(dir, sessionID, "session_start", map[string]string{"foo": "bar"})
	if err != nil {
		t.Fatal(err)
	}

	err = AppendEvent(dir, sessionID, "user_prompt_submit", map[string]string{"prompt": "hello"})
	if err != nil {
		t.Fatal(err)
	}

	// Read back
	events, err := ReadEvents(dir, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Hook != "session_start" {
		t.Errorf("event 0 hook: expected session_start, got %s", events[0].Hook)
	}
	if events[1].Hook != "user_prompt_submit" {
		t.Errorf("event 1 hook: expected user_prompt_submit, got %s", events[1].Hook)
	}
	if events[0].Timestamp == "" {
		t.Error("event 0 should have a timestamp")
	}
}

func TestWipExists(t *testing.T) {
	dir := t.TempDir()
	EnsureDirs(dir)

	if WipExists(dir, "01NONE") {
		t.Error("expected false for nonexistent wip")
	}

	AppendEvent(dir, "01EXISTS", "session_start", map[string]string{})
	if !WipExists(dir, "01EXISTS") {
		t.Error("expected true for existing wip")
	}

	RemoveWip(dir, "01EXISTS")
	if WipExists(dir, "01EXISTS") {
		t.Error("expected false after removal")
	}
}

func TestFinalize(t *testing.T) {
	dir := newTestGitRepo(t)
	EnsureDirs(dir)

	sessionID := "01FINAL"

	// Simulate session start data
	startData := SessionStartData{
		SessionID: sessionID,
		Agent: AgentInfo{
			Runtime: "claude-code",
			Model:   strPtr("claude-opus-4-7"),
			Version: strPtr("0.01.001"),
		},
		Orchestration: Orchestration{
			Type:  "manual",
			Extra: map[string]any{},
		},
		Machine: MachineInfo{
			HostnameHash: "sha256:abc123",
			OS:           runtime.GOOS,
			Arch:         runtime.GOARCH,
		},
		Operator: OperatorInfo{
			GitUser: "Test <test@test.local>",
			OSUser:  "testuser",
		},
		GitState: &GitState{
			Branch:  "main",
			HeadSHA: gitHead(dir),
		},
	}

	AppendEvent(dir, sessionID, "session_start", startData)
	AppendEvent(dir, sessionID, "user_prompt_submit", PromptData{
		Prompt: "fix the bug", Source: "interactive",
	})
	AppendEvent(dir, sessionID, "pre_tool_use", ToolUseData{ToolName: "Read", FilePath: "/tmp/foo.go"})
	AppendEvent(dir, sessionID, "pre_tool_use", ToolUseData{ToolName: "Edit", FilePath: "/tmp/foo.go"})
	AppendEvent(dir, sessionID, "pre_tool_use", ToolUseData{ToolName: "Bash"})
	AppendEvent(dir, sessionID, "session_end", SessionEndData{
		GitState: &GitState{
			Branch:  "main",
			HeadSHA: gitHead(dir),
		},
		ExitReason: "normal",
	})

	session, err := Finalize(dir, dir, sessionID)
	if err != nil {
		t.Fatal(err)
	}

	// Validate the finalized session
	if session.SchemaVersion != SchemaVersion {
		t.Errorf("schema: got %s, want %s", session.SchemaVersion, SchemaVersion)
	}
	if session.SessionID != sessionID {
		t.Errorf("session_id: got %s, want %s", session.SessionID, sessionID)
	}
	if session.Status != "complete" {
		t.Errorf("status: got %s, want complete", session.Status)
	}
	if session.ExitReason != "normal" {
		t.Errorf("exit_reason: got %s, want normal", session.ExitReason)
	}
	if session.Agent.Runtime != "claude-code" {
		t.Errorf("agent.runtime: got %s, want claude-code", session.Agent.Runtime)
	}
	if session.Prompt == nil {
		t.Fatal("expected non-nil prompt")
	}
	if session.Prompt.Text != "fix the bug" {
		t.Errorf("prompt.text: got %s, want 'fix the bug'", session.Prompt.Text)
	}
	if session.Prompt.Source != "interactive" {
		t.Errorf("prompt.source: got %s, want interactive", session.Prompt.Source)
	}
	if session.ToolUse.TotalCalls != 3 {
		t.Errorf("tool_use.total_calls: got %d, want 3", session.ToolUse.TotalCalls)
	}
	if session.ToolUse.ByTool["Read"] != 1 {
		t.Errorf("tool_use.by_tool.Read: got %d, want 1", session.ToolUse.ByTool["Read"])
	}
	if session.ToolUse.ByTool["Edit"] != 1 {
		t.Errorf("tool_use.by_tool.Edit: got %d, want 1", session.ToolUse.ByTool["Edit"])
	}
	if session.ToolUse.ByTool["Bash"] != 1 {
		t.Errorf("tool_use.by_tool.Bash: got %d, want 1", session.ToolUse.ByTool["Bash"])
	}
	if session.Orchestration.Type != "manual" {
		t.Errorf("orchestration.type: got %s, want manual", session.Orchestration.Type)
	}
	if session.Timing.StartedAt == "" {
		t.Error("timing.started_at should not be empty")
	}
	if session.Timing.EndedAt == nil {
		t.Error("timing.ended_at should not be nil")
	}
	if session.Timing.DurationMs == nil {
		t.Error("timing.duration_ms should not be nil")
	}

	// Verify session.json file was written
	if !SessionJSONExists(dir, sessionID) {
		t.Error("session.json should exist after finalization")
	}

	// Re-read and verify JSON
	read, err := ReadSessionJSON(dir, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if read.SessionID != sessionID {
		t.Errorf("read-back session_id: got %s, want %s", read.SessionID, sessionID)
	}

	// Verify it's valid JSON by marshaling
	_, err = json.Marshal(session)
	if err != nil {
		t.Fatalf("session should be valid JSON: %v", err)
	}
}

func TestFinalizeEmpty(t *testing.T) {
	dir := t.TempDir()
	EnsureDirs(dir)

	_, err := Finalize(dir, dir, "01EMPTY")
	if err == nil {
		t.Error("expected error for empty/missing wip file")
	}
}

func TestCaptureMachine(t *testing.T) {
	m := CaptureMachine(config.Defaults(), "testsalt")
	if !strings.HasPrefix(m.HostnameHash, "sha256:") {
		t.Errorf("hostname_hash should start with sha256:, got %s", m.HostnameHash)
	}
	if m.OS != runtime.GOOS {
		t.Errorf("os: got %s, want %s", m.OS, runtime.GOOS)
	}
	if m.Arch != runtime.GOARCH {
		t.Errorf("arch: got %s, want %s", m.Arch, runtime.GOARCH)
	}
	if m.HostnameRaw != nil {
		t.Error("hostname_raw should be nil by default")
	}
}

func TestCaptureMachineSaltedHash(t *testing.T) {
	// Same machine, different salts → different hashes (cross-repo non-correlation).
	m1 := CaptureMachine(config.Defaults(), "saltA")
	m2 := CaptureMachine(config.Defaults(), "saltB")
	if m1.HostnameHash == m2.HostnameHash {
		t.Error("different salts should yield different hostname hashes")
	}
	// Same salt → stable hash (within-repo stability).
	m3 := CaptureMachine(config.Defaults(), "saltA")
	if m1.HostnameHash != m3.HostnameHash {
		t.Error("same salt should yield a stable hostname hash")
	}
}

func TestCaptureMachineRawOptIn(t *testing.T) {
	m := CaptureMachine(config.Settings{RawMachineIdentity: true}, "testsalt")
	if !strings.HasPrefix(m.HostnameHash, "sha256:") {
		t.Errorf("hostname_hash should still be set, got %s", m.HostnameHash)
	}
	if m.HostnameRaw == nil {
		t.Fatal("hostname_raw should be populated when RawMachineIdentity is true")
	}
	if *m.HostnameRaw == "" {
		t.Error("hostname_raw should not be empty")
	}
	hostname, _ := os.Hostname()
	if *m.HostnameRaw != hostname {
		t.Errorf("hostname_raw: got %s, want %s", *m.HostnameRaw, hostname)
	}
}

func TestCaptureOrchestrationDefaults(t *testing.T) {
	// Ensure no ETCH_ vars are set
	for _, key := range []string{
		"ETCH_ORCHESTRATOR_TYPE", "ETCH_DISPATCH_METHOD", "ETCH_TICKET_ID",
		"ETCH_RUN_ID", "ETCH_AGENT_ROLE", "ETCH_WORKFLOW_VERSION",
		"ETCH_ORCHESTRATION_EXTRA",
	} {
		t.Setenv(key, "")
	}

	o := CaptureOrchestration()
	if o.Type != "manual" {
		t.Errorf("type: got %s, want manual", o.Type)
	}
	if o.DispatchMethod != nil {
		t.Error("dispatch_method should be nil when unset")
	}
	if o.TicketID != nil {
		t.Error("ticket_id should be nil when unset")
	}
}

func TestCaptureOrchestrationWithEnv(t *testing.T) {
	t.Setenv("ETCH_ORCHESTRATOR_TYPE", "lattice-orchestrator")
	t.Setenv("ETCH_DISPATCH_METHOD", "c11_delegator")
	t.Setenv("ETCH_TICKET_ID", "FT-481")
	t.Setenv("ETCH_RUN_ID", "01RUN")
	t.Setenv("ETCH_AGENT_ROLE", "implementer")
	t.Setenv("ETCH_WORKFLOW_VERSION", "abc123")
	t.Setenv("ETCH_ORCHESTRATION_EXTRA", `{"phase":"impl","retry":2}`)

	o := CaptureOrchestration()
	if o.Type != "lattice-orchestrator" {
		t.Errorf("type: got %s, want lattice-orchestrator", o.Type)
	}
	if o.DispatchMethod == nil || *o.DispatchMethod != "c11_delegator" {
		t.Errorf("dispatch_method: got %v, want c11_delegator", o.DispatchMethod)
	}
	if o.TicketID == nil || *o.TicketID != "FT-481" {
		t.Errorf("ticket_id: got %v, want FT-481", o.TicketID)
	}
	if o.RunID == nil || *o.RunID != "01RUN" {
		t.Errorf("run_id: got %v, want 01RUN", o.RunID)
	}
	if o.Role == nil || *o.Role != "implementer" {
		t.Errorf("role: got %v, want implementer", o.Role)
	}
	if o.WorkflowVersion == nil || *o.WorkflowVersion != "abc123" {
		t.Errorf("workflow_version: got %v, want abc123", o.WorkflowVersion)
	}
	if o.Extra["phase"] != "impl" {
		t.Errorf("extra.phase: got %v, want impl", o.Extra["phase"])
	}
	if o.Extra["retry"] != float64(2) {
		t.Errorf("extra.retry: got %v, want 2", o.Extra["retry"])
	}
}

func TestCaptureGitState(t *testing.T) {
	dir := newTestGitRepo(t)
	gs := CaptureGitState(dir)

	if gs.Branch == "" {
		t.Error("branch should not be empty")
	}
	if gs.HeadSHA == "" {
		t.Error("head_sha should not be empty")
	}
	if gs.WorktreePath == "" {
		t.Error("worktree_path should not be empty")
	}
	if gs.RepoRoot == "" {
		t.Error("repo_root should not be empty")
	}
}

func TestCaptureGitEnd(t *testing.T) {
	dir := newTestGitRepo(t)

	startSHA := gitHead(dir)

	// Make a commit
	writeFile(t, dir, "newfile.txt", "content")
	gitCmd(t, dir, "add", "newfile.txt")
	gitCmd(t, dir, "commit", "-m", "test commit")

	gs := CaptureGitEnd(dir, startSHA)
	if gs.HeadSHA == startSHA {
		t.Error("head_sha should differ after commit")
	}
	if len(gs.CommitsProduced) == 0 {
		t.Error("commits_produced should not be empty")
	}
}

func TestCaptureGitEndNoChange(t *testing.T) {
	dir := newTestGitRepo(t)
	startSHA := gitHead(dir)

	gs := CaptureGitEnd(dir, startSHA)
	if len(gs.CommitsProduced) != 0 {
		t.Errorf("commits_produced should be empty when no new commits, got %v", gs.CommitsProduced)
	}
}

func TestGitDiffFiles(t *testing.T) {
	dir := newTestGitRepo(t)
	startSHA := gitHead(dir)

	writeFile(t, dir, "added.txt", "new")
	gitCmd(t, dir, "add", "added.txt")
	gitCmd(t, dir, "commit", "-m", "add file")

	files, err := gitDiffFiles(dir, startSHA, gitHead(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != "added.txt" {
		t.Errorf("path: got %s, want added.txt", files[0].Path)
	}
	if files[0].Action != "added" {
		t.Errorf("action: got %s, want added", files[0].Action)
	}
}

// TestGitDiffFiles_RenameAndNonASCII (ETCH-40 below-cut): renames must not
// corrupt into "old\tnew" pseudo-paths, and non-ASCII names must come
// through verbatim, not core.quotePath octal-escaped.
func TestGitDiffFiles_RenameAndNonASCII(t *testing.T) {
	dir := newTestGitRepo(t)

	// Seed a file large enough that a rename is detected as R100.
	content := strings.Repeat("line of stable content\n", 50)
	writeFile(t, dir, "original.txt", content)
	writeFile(t, dir, "héllo wörld.txt", "non-ascii ünïcode content\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "seed")
	startSHA := gitHead(dir)

	// Rename one file, modify the non-ASCII one, add a tab-in-name file.
	gitCmd(t, dir, "mv", "original.txt", "renamed.txt")
	writeFile(t, dir, "héllo wörld.txt", "non-ascii ünïcode content v2\n")
	writeFile(t, dir, "tab\there.txt", "tabbed\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "mutate")

	files, err := gitDiffFiles(dir, startSHA, gitHead(dir))
	if err != nil {
		t.Fatal(err)
	}

	byPath := map[string]string{}
	for _, f := range files {
		byPath[f.Path] = f.Action
		if strings.Contains(f.Path, "\\") {
			t.Errorf("octal-escaped path leaked through: %q", f.Path)
		}
		if strings.Contains(f.Path, ".txt\t") {
			t.Errorf("rename corrupted into a tab-joined pseudo-path: %q", f.Path)
		}
	}

	if byPath["original.txt"] != "deleted" {
		t.Errorf("rename old side: want original.txt deleted, got %q (all: %v)", byPath["original.txt"], byPath)
	}
	if byPath["renamed.txt"] != "added" {
		t.Errorf("rename new side: want renamed.txt added, got %q", byPath["renamed.txt"])
	}
	if byPath["héllo wörld.txt"] != "modified" {
		t.Errorf("non-ascii path: want modified verbatim, got %q (all: %v)", byPath["héllo wörld.txt"], byPath)
	}
	if byPath["tab\there.txt"] != "added" {
		t.Errorf("tab-in-name path: want added verbatim, got %q (all: %v)", byPath["tab\there.txt"], byPath)
	}
}

func TestCaptureC11Nil(t *testing.T) {
	t.Setenv("C11_WORKSPACE_ID", "")
	t.Setenv("C11_SURFACE_ID", "")
	t.Setenv("CMUX_WORKSPACE_ID", "")
	t.Setenv("CMUX_SURFACE_ID", "")

	c := CaptureC11()
	if c != nil {
		t.Error("expected nil when no c11 env vars set")
	}
}

func TestBuildPaneLineageSolo(t *testing.T) {
	t.Setenv("ETCH_PANE_LINEAGE", "")
	lineage := buildPaneLineage("My Pane")
	if len(lineage) != 1 || lineage[0] != "My Pane" {
		t.Errorf("solo lineage: got %v, want [\"My Pane\"]", lineage)
	}
}

func TestBuildPaneLineageWithParent(t *testing.T) {
	t.Setenv("ETCH_PANE_LINEAGE", `["Orchestrator","Delegator"]`)
	lineage := buildPaneLineage("Implementer")
	expected := []string{"Orchestrator", "Delegator", "Implementer"}
	if len(lineage) != len(expected) {
		t.Fatalf("lineage length: got %d, want %d (%v)", len(lineage), len(expected), lineage)
	}
	for i, v := range expected {
		if lineage[i] != v {
			t.Errorf("lineage[%d]: got %q, want %q", i, lineage[i], v)
		}
	}
}

func TestBuildPaneLineageNoCurrentTitle(t *testing.T) {
	t.Setenv("ETCH_PANE_LINEAGE", `["Orchestrator"]`)
	lineage := buildPaneLineage("")
	if len(lineage) != 1 || lineage[0] != "Orchestrator" {
		t.Errorf("parent-only lineage: got %v, want [\"Orchestrator\"]", lineage)
	}
}

func TestBuildPaneLineageInvalidJSON(t *testing.T) {
	t.Setenv("ETCH_PANE_LINEAGE", "not-json")
	lineage := buildPaneLineage("Current")
	if len(lineage) != 1 || lineage[0] != "Current" {
		t.Errorf("invalid JSON should be ignored: got %v, want [\"Current\"]", lineage)
	}
}

func TestCaptureTranscriptRef(t *testing.T) {
	ref := CaptureTranscriptRef("")
	if ref != nil {
		t.Error("expected nil for empty session_ref")
	}

	ref = CaptureTranscriptRef("/nonexistent/path.jsonl")
	if ref == nil {
		t.Fatal("expected non-nil for non-empty session_ref")
	}
	if ref.Available {
		t.Error("expected available=false for nonexistent path")
	}

	// Test with existing file
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.jsonl")
	os.WriteFile(path, []byte("{}"), 0o644)
	ref = CaptureTranscriptRef(path)
	if !ref.Available {
		t.Error("expected available=true for existing path")
	}
}

func TestInferRuntime(t *testing.T) {
	// Clear all runtime env vars
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CODEX_CLI", "")
	t.Setenv("GEMINI_CLI", "")

	if got := InferRuntime(); got != "unknown" {
		t.Errorf("expected unknown, got %s", got)
	}

	t.Setenv("CLAUDECODE", "1")
	if got := InferRuntime(); got != "claude-code" {
		t.Errorf("expected claude-code, got %s", got)
	}
}

func TestSanitizeFilename(t *testing.T) {
	if got := sanitizeFilename("a/b\\c"); got != "a_b_c" {
		t.Errorf("expected a_b_c, got %s", got)
	}
}

// helpers

func newTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "config", "user.email", "test@test.local")
	gitCmd(t, dir, "config", "user.name", "Test")
	writeFile(t, dir, "init.txt", "init")
	gitCmd(t, dir, "add", "init.txt")
	gitCmd(t, dir, "commit", "-m", "initial commit")
	return dir
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	out := gitOutput(dir, "git", args...)
	_ = out
}

func gitHead(dir string) string {
	return gitOutput(dir, "git", "rev-parse", "HEAD")
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func strPtr(s string) *string {
	return &s
}

func TestFinalizeAgentSessionID(t *testing.T) {
	dir := newTestGitRepo(t)
	EnsureDirs(dir)

	// With agent_session_id present in session_start data → preserved.
	upstream := "upstream-runtime-id-001"
	AppendEvent(dir, "01AGENTSID", "session_start", SessionStartData{
		SessionID:      "01AGENTSID",
		AgentSessionID: strPtr(upstream),
		Agent:          AgentInfo{Runtime: "claude-code"},
	})
	AppendEvent(dir, "01AGENTSID", "session_end", SessionEndData{ExitReason: "normal"})

	session, err := Finalize(dir, dir, "01AGENTSID")
	if err != nil {
		t.Fatal(err)
	}
	if session.AgentSessionID == nil || *session.AgentSessionID != upstream {
		t.Errorf("agent_session_id not preserved through Finalize: %v", session.AgentSessionID)
	}
	if session.SessionID != "01AGENTSID" {
		t.Errorf("minted session_id must stay canonical, got %s", session.SessionID)
	}

	// Without agent_session_id → null in the finalized record (key present).
	AppendEvent(dir, "01NOAGENTSID", "session_start", SessionStartData{
		SessionID: "01NOAGENTSID",
		Agent:     AgentInfo{Runtime: "codex"},
	})
	AppendEvent(dir, "01NOAGENTSID", "session_end", SessionEndData{ExitReason: "normal"})

	session2, err := Finalize(dir, dir, "01NOAGENTSID")
	if err != nil {
		t.Fatal(err)
	}
	if session2.AgentSessionID != nil {
		t.Errorf("agent_session_id should be nil when upstream supplied none, got %v", *session2.AgentSessionID)
	}

	// JSON shape: key present with null value, not omitted.
	data, err := json.Marshal(session2)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(data, &m)
	if v, present := m["agent_session_id"]; !present {
		t.Error("agent_session_id key must be present in marshaled record")
	} else if v != nil {
		t.Errorf("agent_session_id should marshal as null, got %v", v)
	}
	if v, present := m["tokens"]; !present {
		t.Error("tokens key must be present in marshaled record (reserved field)")
	} else if v != nil {
		t.Errorf("tokens must be null in v1, got %v", v)
	}
}
