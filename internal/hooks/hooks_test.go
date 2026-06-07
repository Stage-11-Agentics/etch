package hooks_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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

	// Verify .etch/sessions/ has a .wip.jsonl file
	wipFiles := findWipFiles(t, dir)
	if len(wipFiles) != 1 {
		t.Fatalf("expected 1 wip file, got %d", len(wipFiles))
	}

	// Verify mapping exists
	mapDir := filepath.Join(dir, ".etch", "sessions", ".map")
	entries, _ := os.ReadDir(mapDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(entries))
	}
}

// TestDuplicateSessionStartReusesSession (ETCH-40 finding 4): a second
// session_start for the same upstream session id must reuse the existing
// ULID/wip — one logical session, one record, no truncated 'crash' sibling.
func TestDuplicateSessionStartReusesSession(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	sid := "dup-start-001"
	input := `{"session_id":"` + sid + `","raw_data":{"model":"claude-opus-4-7"}}`

	r := testutil.RunBinary(t, dir, []string{"session_start"}, input)
	assertOK(t, r, "first session_start")

	wipFiles := findWipFiles(t, dir)
	if len(wipFiles) != 1 {
		t.Fatalf("expected 1 wip after first start, got %d", len(wipFiles))
	}
	ulid1 := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	// Same upstream session id again (resume / duplicate delivery).
	r = testutil.RunBinary(t, dir, []string{"session_start"}, input)
	assertOK(t, r, "duplicate session_start")

	wipFiles = findWipFiles(t, dir)
	if len(wipFiles) != 1 {
		t.Fatalf("duplicate start must not mint a second wip: got %d", len(wipFiles))
	}
	ulid2 := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")
	if ulid1 != ulid2 {
		t.Fatalf("duplicate start repointed the session: %s -> %s", ulid1, ulid2)
	}

	// Mapping still points at the original ULID.
	mapDir := filepath.Join(dir, ".etch", "sessions", ".map")
	entries, _ := os.ReadDir(mapDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(entries))
	}
	mapped, _ := os.ReadFile(filepath.Join(mapDir, entries[0].Name()))
	if strings.TrimSpace(string(mapped)) != ulid1 {
		t.Fatalf("mapping clobbered: want %s, got %s", ulid1, mapped)
	}

	// End the session: exactly one ref, complete/normal — no crash sibling.
	r = testutil.RunBinary(t, dir, []string{"session_end"}, `{"session_id":"`+sid+`"}`)
	assertOK(t, r, "session_end")

	out, err := exec.Command("git", "-C", dir, "for-each-ref", "refs/etch/sessions/").Output()
	if err != nil {
		t.Fatal(err)
	}
	refs := strings.Fields(strings.TrimSpace(string(out)))
	refCount := strings.Count(string(out), "refs/etch/sessions/")
	if refCount != 1 {
		t.Fatalf("expected exactly 1 session ref, got %d:\n%s", refCount, refs)
	}
	if !strings.Contains(string(out), ulid1) {
		t.Errorf("the one ref must be the original ULID %s:\n%s", ulid1, out)
	}
}

// TestResumeAfterCrashContinuesSession: a session_start for an upstream id
// whose wip looks crashed (dead PID, idle) must RESUME that wip, not let the
// recovery pass commit it out from under the resume.
func TestResumeAfterCrashContinuesSession(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	sid := "resume-after-crash-001"
	input := `{"session_id":"` + sid + `","raw_data":{"model":"claude-opus-4-7"}}`
	r := testutil.RunBinary(t, dir, []string{"session_start"}, input)
	assertOK(t, r, "first session_start")

	wipFiles := findWipFiles(t, dir)
	ulid1 := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	// Make the wip look crashed: idle for 6h (past the 4h timeout). The
	// recorded PID (the binary's transient ancestor walk found none → 0)
	// leaves the timeout governing.
	old := time.Now().Add(-6 * time.Hour)
	if err := os.Chtimes(wipFiles[0], old, old); err != nil {
		t.Fatal(err)
	}

	// The resume: same upstream id. Recovery runs in this invocation but the
	// resumed wip must be shielded from it.
	r = testutil.RunBinary(t, dir, []string{"session_start"}, input)
	assertOK(t, r, "resume session_start")

	if out, _ := exec.Command("git", "-C", dir, "for-each-ref", "refs/etch/sessions/").Output(); strings.Contains(string(out), ulid1) {
		t.Fatal("resumed session was crash-recovered out from under the resume")
	}
	if _, err := os.Stat(wipFiles[0]); err != nil {
		t.Fatal("resumed session's wip must survive")
	}

	// And the session still ends as one complete record.
	r = testutil.RunBinary(t, dir, []string{"session_end"}, `{"session_id":"`+sid+`"}`)
	assertOK(t, r, "session_end after resume")

	out, _ := exec.Command("git", "-C", dir, "for-each-ref", "refs/etch/sessions/").Output()
	if strings.Count(string(out), "refs/etch/sessions/") != 1 || !strings.Contains(string(out), ulid1) {
		t.Fatalf("expected exactly one ref for %s:\n%s", ulid1, out)
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

	// Get the ULID from the wip file
	wipFiles := findWipFiles(t, dir)
	if len(wipFiles) != 1 {
		t.Fatalf("expected 1 wip file, got %d", len(wipFiles))
	}
	sessionULID := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

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

	// Verify session is now in a git ref (not on disk)
	refName := "refs/etch/sessions/" + sessionULID
	data := readRefBlob(t, dir, refName+":session.json")

	var session capture.Session
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatalf("invalid session JSON: %v", err)
	}

	if session.SchemaVersion != "etch.session.v1" {
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

	wipFiles := findWipFiles(t, dir)
	sessionULID := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	// stop (instead of session_end)
	stopInput := `{"session_id":"` + entireSessionID + `"}`
	r = testutil.RunBinary(t, dir, []string{"stop"}, stopInput)
	assertOK(t, r, "stop")

	// Verify session in ref
	refName := "refs/etch/sessions/" + sessionULID
	data := readRefBlob(t, dir, refName+":session.json")
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

	envVars := map[string]string{
		"ETCH_ORCHESTRATOR_TYPE": "lattice-orchestrator",
		"ETCH_DISPATCH_METHOD":   "c11_delegator",
		"ETCH_TICKET_ID":         "FT-481",
		"ETCH_RUN_ID":            "01RUN",
		"ETCH_AGENT_ROLE":        "implementer",
	}

	// Run with ETCH env vars
	startInput := `{"session_id":"` + entireSessionID + `","raw_data":{}}`
	r := testutil.RunBinaryWithEnv(t, dir, []string{"session_start"}, startInput, envVars)
	assertOK(t, r, "session_start with env")

	wipFiles := findWipFiles(t, dir)
	sessionULID := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	// End session
	endInput := `{"session_id":"` + entireSessionID + `"}`
	r = testutil.RunBinaryWithEnv(t, dir, []string{"session_end"}, endInput, envVars)
	assertOK(t, r, "session_end with env")

	// Read session from ref
	refName := "refs/etch/sessions/" + sessionULID
	data := readRefBlob(t, dir, refName+":session.json")
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

	wipFiles := findWipFiles(t, dir)
	sessionULID := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

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

	refName := "refs/etch/sessions/" + sessionULID
	data := readRefBlob(t, dir, refName+":session.json")
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

// TestPromptTruncationRuneBoundary (ETCH-40 below-cut): a multi-byte rune
// straddling the 32KiB cut must be dropped whole, never sliced mid-rune
// (which JSON-degrades the tail to U+FFFD).
func TestPromptTruncationRuneBoundary(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	sid := "truncate-rune-001"
	r := testutil.RunBinary(t, dir, []string{"session_start"}, `{"session_id":"`+sid+`","raw_data":{}}`)
	assertOK(t, r, "session_start")
	wipFiles := findWipFiles(t, dir)
	ulid := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	// 32KiB-1 ASCII bytes, then 4-byte runes: the first 🜂 straddles the cut.
	bigPrompt := strings.Repeat("A", 32*1024-1) + strings.Repeat("🜂", 100)
	promptInput, _ := json.Marshal(map[string]string{
		"session_id":  sid,
		"user_prompt": bigPrompt,
	})
	r = testutil.RunBinary(t, dir, []string{"user_prompt_submit"}, string(promptInput))
	assertOK(t, r, "user_prompt_submit straddling rune")

	r = testutil.RunBinary(t, dir, []string{"session_end"}, `{"session_id":"`+sid+`"}`)
	assertOK(t, r, "session_end")

	data := readRefBlob(t, dir, "refs/etch/sessions/"+ulid+":session.json")
	var session capture.Session
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatal(err)
	}
	if session.Prompt == nil || !session.Prompt.Truncated {
		t.Fatal("expected a truncated prompt")
	}
	if !utf8.ValidString(session.Prompt.Text) {
		t.Error("truncated prompt is not valid UTF-8 — mid-rune slice")
	}
	if strings.ContainsRune(session.Prompt.Text, utf8.RuneError) {
		t.Error("truncated prompt contains U+FFFD — the tail was sliced mid-rune")
	}
	if got := len(session.Prompt.Text); got != 32*1024-1 {
		t.Errorf("expected cut backed off to 32KiB-1 (the rune dropped whole), got %d", got)
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
	return findFiles(t, filepath.Join(dir, ".etch", "sessions"), ".wip.jsonl")
}

func findSessionJSONFiles(t *testing.T, dir string) []string {
	t.Helper()
	return findFiles(t, filepath.Join(dir, ".etch", "sessions"), ".session.json")
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

func readRefBlob(t *testing.T, dir, refPath string) []byte {
	t.Helper()
	cmd := exec.Command("git", "show", refPath)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git show %s: %v", refPath, err)
	}
	return out
}
