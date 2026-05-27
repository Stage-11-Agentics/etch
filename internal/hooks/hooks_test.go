package hooks_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgejo.stage11.ai/s11/etch/internal/capture"
	"forgejo.stage11.ai/s11/etch/internal/testutil"
)

func TestSessionStartCreatesWip(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	input := `{"session_id":"test-sess-1","session_ref":"/tmp/transcript.jsonl","timestamp":"2026-05-27T00:00:00Z","raw_data":{"model":"claude-opus-4-7"}}`
	result := testutil.RunBinary(t, dir, []string{"session_start"}, input)
	if result.ExitCode != 0 {
		t.Fatalf("session_start exited %d: %s", result.ExitCode, result.Stderr)
	}

	m := testutil.MustParseJSON(t, result.Stdout)
	if m["ok"] != true {
		t.Errorf("expected ok=true, got %v", m["ok"])
	}

	// Verify .cairn/sessions/ has a .wip.jsonl file
	wipFiles := findWipFiles(t, dir)
	if len(wipFiles) != 1 {
		t.Fatalf("expected 1 wip file, got %d", len(wipFiles))
	}

	// Verify mapping exists
	mapDir := filepath.Join(dir, ".cairn", "sessions", ".map")
	entries, _ := os.ReadDir(mapDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(entries))
	}
}

func TestFullSessionLifecycle(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	entireSessionID := "lifecycle-test-001"

	// 1. session_start
	startInput := `{"session_id":"` + entireSessionID + `","session_ref":"/tmp/test.jsonl","raw_data":{"model":"claude-opus-4-7"}}`
	r := testutil.RunBinary(t, dir, []string{"session_start"}, startInput)
	assertOK(t, r, "session_start")

	// 2. user_prompt_submit
	promptInput := `{"session_id":"` + entireSessionID + `","user_prompt":"fix the login bug"}`
	r = testutil.RunBinary(t, dir, []string{"user_prompt_submit"}, promptInput)
	assertOK(t, r, "user_prompt_submit")

	// 3. pre_tool_use (Read)
	toolInput := `{"session_id":"` + entireSessionID + `","tool_name":"Read","tool_use_id":"tu-1","tool_input":{"file_path":"/tmp/foo.go"}}`
	r = testutil.RunBinary(t, dir, []string{"pre_tool_use"}, toolInput)
	assertOK(t, r, "pre_tool_use")

	// 4. post_tool_use (Read)
	r = testutil.RunBinary(t, dir, []string{"post_tool_use"}, toolInput)
	assertOK(t, r, "post_tool_use")

	// 5. pre_tool_use (Edit)
	editInput := `{"session_id":"` + entireSessionID + `","tool_name":"Edit","tool_use_id":"tu-2","tool_input":{"file_path":"/tmp/bar.go"}}`
	r = testutil.RunBinary(t, dir, []string{"pre_tool_use"}, editInput)
	assertOK(t, r, "pre_tool_use edit")

	// 6. session_end
	endInput := `{"session_id":"` + entireSessionID + `"}`
	r = testutil.RunBinary(t, dir, []string{"session_end"}, endInput)
	assertOK(t, r, "session_end")

	// Verify finalized session.json exists
	sessionFiles := findSessionJSONFiles(t, dir)
	if len(sessionFiles) != 1 {
		t.Fatalf("expected 1 session.json, got %d", len(sessionFiles))
	}

	// Read and validate the session
	data, err := os.ReadFile(sessionFiles[0])
	if err != nil {
		t.Fatal(err)
	}

	var session capture.Session
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatalf("invalid session JSON: %v", err)
	}

	if session.SchemaVersion != "cairn.session.v1" {
		t.Errorf("schema_version: got %s", session.SchemaVersion)
	}
	if session.Status != "complete" {
		t.Errorf("status: got %s, want complete", session.Status)
	}
	if session.ExitReason != "normal" {
		t.Errorf("exit_reason: got %s, want normal", session.ExitReason)
	}
	if session.Agent.Runtime == "" {
		t.Error("agent.runtime should not be empty")
	}
	if session.Agent.Model == nil || *session.Agent.Model != "claude-opus-4-7" {
		t.Errorf("agent.model: got %v, want claude-opus-4-7", session.Agent.Model)
	}
	if session.Prompt == nil {
		t.Fatal("expected non-nil prompt")
	}
	if session.Prompt.Text != "fix the login bug" {
		t.Errorf("prompt.text: got %q, want 'fix the login bug'", session.Prompt.Text)
	}
	if session.ToolUse.TotalCalls != 2 {
		t.Errorf("total_calls: got %d, want 2 (only pre_tool_use counts)", session.ToolUse.TotalCalls)
	}
	if session.ToolUse.ByTool["Read"] != 1 {
		t.Errorf("by_tool.Read: got %d, want 1", session.ToolUse.ByTool["Read"])
	}
	if session.ToolUse.ByTool["Edit"] != 1 {
		t.Errorf("by_tool.Edit: got %d, want 1", session.ToolUse.ByTool["Edit"])
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
	if session.GitStart == nil {
		t.Error("git_start should not be nil")
	}
	if session.GitEnd == nil {
		t.Error("git_end should not be nil")
	}
	if session.Machine.HostnameHash == "" {
		t.Error("machine.hostname_hash should not be empty")
	}
	if session.Operator.GitUser == "" {
		t.Error("operator.git_user should not be empty")
	}
}

func TestStopHook(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	entireSessionID := "stop-test-001"

	// session_start
	startInput := `{"session_id":"` + entireSessionID + `","raw_data":{"model":"claude-opus-4-7"}}`
	r := testutil.RunBinary(t, dir, []string{"session_start"}, startInput)
	assertOK(t, r, "session_start")

	// stop (instead of session_end)
	stopInput := `{"session_id":"` + entireSessionID + `"}`
	r = testutil.RunBinary(t, dir, []string{"stop"}, stopInput)
	assertOK(t, r, "stop")

	// Verify session.json
	sessionFiles := findSessionJSONFiles(t, dir)
	if len(sessionFiles) != 1 {
		t.Fatalf("expected 1 session.json, got %d", len(sessionFiles))
	}

	data, _ := os.ReadFile(sessionFiles[0])
	var session capture.Session
	json.Unmarshal(data, &session)

	if session.ExitReason != "unknown" {
		t.Errorf("exit_reason: got %s, want unknown (stop hook default)", session.ExitReason)
	}
}

func TestOrchestrationEnvVars(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	entireSessionID := "orch-test-001"

	// Run with CAIRN env vars
	startInput := `{"session_id":"` + entireSessionID + `","raw_data":{}}`
	r := testutil.RunBinaryWithEnv(t, dir, []string{"session_start"}, startInput, map[string]string{
		"CAIRN_ORCHESTRATOR_TYPE": "lattice-orchestrator",
		"CAIRN_DISPATCH_METHOD":  "c11_delegator",
		"CAIRN_TICKET_ID":        "FT-481",
		"CAIRN_RUN_ID":           "01RUN",
		"CAIRN_AGENT_ROLE":       "implementer",
	})
	assertOK(t, r, "session_start with env")

	// End session
	endInput := `{"session_id":"` + entireSessionID + `"}`
	r = testutil.RunBinaryWithEnv(t, dir, []string{"session_end"}, endInput, map[string]string{
		"CAIRN_ORCHESTRATOR_TYPE": "lattice-orchestrator",
		"CAIRN_DISPATCH_METHOD":  "c11_delegator",
		"CAIRN_TICKET_ID":        "FT-481",
		"CAIRN_RUN_ID":           "01RUN",
		"CAIRN_AGENT_ROLE":       "implementer",
	})
	assertOK(t, r, "session_end with env")

	sessionFiles := findSessionJSONFiles(t, dir)
	if len(sessionFiles) != 1 {
		t.Fatalf("expected 1 session.json, got %d", len(sessionFiles))
	}

	data, _ := os.ReadFile(sessionFiles[0])
	var session capture.Session
	json.Unmarshal(data, &session)

	if session.Orchestration.Type != "lattice-orchestrator" {
		t.Errorf("orchestration.type: got %s, want lattice-orchestrator", session.Orchestration.Type)
	}
	if session.Orchestration.DispatchMethod == nil || *session.Orchestration.DispatchMethod != "c11_delegator" {
		t.Errorf("dispatch_method: got %v", session.Orchestration.DispatchMethod)
	}
	if session.Orchestration.TicketID == nil || *session.Orchestration.TicketID != "FT-481" {
		t.Errorf("ticket_id: got %v", session.Orchestration.TicketID)
	}
}

func TestPromptTruncation(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	entireSessionID := "truncate-test-001"

	// session_start
	startInput := `{"session_id":"` + entireSessionID + `","raw_data":{}}`
	r := testutil.RunBinary(t, dir, []string{"session_start"}, startInput)
	assertOK(t, r, "session_start")

	// Submit a huge prompt (> 32 KiB)
	bigPrompt := strings.Repeat("A", 40*1024)
	promptInput, _ := json.Marshal(map[string]string{
		"session_id":  entireSessionID,
		"user_prompt": bigPrompt,
	})
	r = testutil.RunBinary(t, dir, []string{"user_prompt_submit"}, string(promptInput))
	assertOK(t, r, "user_prompt_submit large")

	// session_end
	endInput := `{"session_id":"` + entireSessionID + `"}`
	r = testutil.RunBinary(t, dir, []string{"session_end"}, endInput)
	assertOK(t, r, "session_end")

	sessionFiles := findSessionJSONFiles(t, dir)
	data, _ := os.ReadFile(sessionFiles[0])
	var session capture.Session
	json.Unmarshal(data, &session)

	if session.Prompt == nil {
		t.Fatal("expected non-nil prompt")
	}
	if !session.Prompt.Truncated {
		t.Error("expected truncated=true for >32KiB prompt")
	}
	if len(session.Prompt.Text) > 32*1024 {
		t.Errorf("prompt text should be <= 32KiB, got %d bytes", len(session.Prompt.Text))
	}
}

func TestNoMappingGraceful(t *testing.T) {
	dir := testutil.NewTestRepo(t)

	// Call hooks without a prior session_start — should succeed gracefully
	for _, hook := range []string{"user_prompt_submit", "pre_tool_use", "post_tool_use", "session_end", "stop"} {
		input := `{"session_id":"nonexistent","user_prompt":"hi","tool_name":"Read"}`
		r := testutil.RunBinary(t, dir, []string{hook}, input)
		if r.ExitCode != 0 {
			t.Errorf("%s should succeed gracefully without mapping, got exit %d: %s", hook, r.ExitCode, r.Stderr)
		}
	}
}

// helpers

func commitInitial(t *testing.T, dir string) {
	t.Helper()
	os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0o644)
	run(t, dir, "git", "add", "init.txt")
	run(t, dir, "git", "commit", "-m", "initial commit")
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	testutil.RunCmd(t, dir, name, args...)
}

func assertOK(t *testing.T, r testutil.BinaryResult, label string) {
	t.Helper()
	if r.ExitCode != 0 {
		t.Fatalf("%s exited %d: %s", label, r.ExitCode, r.Stderr)
	}
	m := testutil.MustParseJSON(t, r.Stdout)
	if m["ok"] != true {
		t.Errorf("%s: expected ok=true, got %v", label, m["ok"])
	}
}

func findWipFiles(t *testing.T, dir string) []string {
	t.Helper()
	return findFiles(t, filepath.Join(dir, ".cairn", "sessions"), ".wip.jsonl")
}

func findSessionJSONFiles(t *testing.T, dir string) []string {
	t.Helper()
	return findFiles(t, filepath.Join(dir, ".cairn", "sessions"), ".session.json")
}

func findFiles(t *testing.T, dir, suffix string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
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
