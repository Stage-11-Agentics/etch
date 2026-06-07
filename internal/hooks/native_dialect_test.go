package hooks_test

// Tests for the agent-runtime native hook dialect (Claude Code hook JSON) and
// the visible-warning behavior for payloads missing expected fields (ETCH-20).
// Payload shapes mirror live captures from Claude Code 2.1.168 — see
// docs/HOOK_CONTRACT.md.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgejo.stage11.ai/s11/etch/internal/capture"
	"forgejo.stage11.ai/s11/etch/internal/testutil"
)

// writeTranscriptFixture writes a minimal Claude Code transcript JSONL whose
// assistant entry carries message.model.
func writeTranscriptFixture(t *testing.T, dir, model string) string {
	t.Helper()
	path := filepath.Join(dir, "transcript.jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"hi"}}`,
		`{"type":"assistant","message":{"role":"assistant","model":"` + model + `","content":[]}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestNativeClaudeCodeLifecycle drives a full session using ONLY native
// Claude Code payloads (no user_prompt, no raw_data) and asserts the record
// is complete — including the model, which native payloads never carry and
// must be backfilled from the transcript at finalize.
func TestNativeClaudeCodeLifecycle(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	transcript := writeTranscriptFixture(t, dir, "claude-opus-4-8")
	sid := "native-cc-001"

	start := `{"session_id":"` + sid + `","transcript_path":"` + transcript + `","cwd":"` + dir + `","hook_event_name":"SessionStart","source":"startup"}`
	r := testutil.RunBinary(t, dir, []string{"session_start"}, start)
	assertOK(t, r, "session_start")
	if strings.Contains(r.Stderr, "etch: warning") {
		t.Errorf("session_start with transcript_path should not warn, got: %s", r.Stderr)
	}

	wipFiles := findWipFiles(t, dir)
	if len(wipFiles) != 1 {
		t.Fatalf("expected 1 wip file, got %d", len(wipFiles))
	}
	sessionULID := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	prompt := `{"session_id":"` + sid + `","transcript_path":"` + transcript + `","cwd":"` + dir + `","hook_event_name":"UserPromptSubmit","prompt":"run the native test"}`
	r = testutil.RunBinary(t, dir, []string{"user_prompt_submit"}, prompt)
	assertOK(t, r, "user_prompt_submit")
	if strings.Contains(r.Stderr, "etch: warning") {
		t.Errorf("native prompt should not warn, got: %s", r.Stderr)
	}

	tool := `{"session_id":"` + sid + `","transcript_path":"` + transcript + `","hook_event_name":"PreToolUse","tool_name":"Read","tool_use_id":"toolu_01X","tool_input":{"file_path":"/tmp/f.txt"}}`
	r = testutil.RunBinary(t, dir, []string{"pre_tool_use"}, tool)
	assertOK(t, r, "pre_tool_use")
	r = testutil.RunBinary(t, dir, []string{"post_tool_use"}, tool)
	assertOK(t, r, "post_tool_use")

	end := `{"session_id":"` + sid + `","transcript_path":"` + transcript + `","hook_event_name":"SessionEnd","reason":"other"}`
	r = testutil.RunBinary(t, dir, []string{"session_end"}, end)
	assertOK(t, r, "session_end")

	data := readRefBlob(t, dir, "refs/etch/sessions/"+sessionULID+":session.json")
	var session capture.Session
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatalf("invalid session JSON: %v", err)
	}

	if session.Status != "complete" {
		t.Errorf("status: got %s, want complete", session.Status)
	}
	if session.Agent.Model == nil || *session.Agent.Model != "claude-opus-4-8" {
		t.Errorf("model not backfilled from transcript: got %v", session.Agent.Model)
	}
	if session.Prompt == nil || session.Prompt.Text != "run the native test" {
		t.Errorf("native prompt not captured: got %v", session.Prompt)
	}
	if session.ExitReason != "other" {
		t.Errorf("native reason not captured: got %s", session.ExitReason)
	}
	if session.ToolUse.TotalCalls == 0 {
		t.Error("tool use not captured from native payloads")
	}
	if session.TranscriptRef == nil || session.TranscriptRef.LocalPath == nil || *session.TranscriptRef.LocalPath != transcript {
		t.Errorf("transcript_path not captured into transcript_ref: %+v", session.TranscriptRef)
	}
	if session.TranscriptRef != nil && !session.TranscriptRef.Available {
		t.Error("transcript availability not refreshed at finalize")
	}
}

// TestEntireDialectStillWorks locks the Entire HookInput dialect (the
// pre-existing contract exercised by scripts/smoke.sh).
func TestEntireDialectStillWorks(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	sid := "entire-dialect-001"
	r := testutil.RunBinary(t, dir, []string{"session_start"},
		`{"session_id":"`+sid+`","raw_data":{"model":"claude-opus-4-7"}}`)
	assertOK(t, r, "session_start")

	wipFiles := findWipFiles(t, dir)
	sessionULID := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	r = testutil.RunBinary(t, dir, []string{"user_prompt_submit"},
		`{"session_id":"`+sid+`","user_prompt":"entire dialect prompt"}`)
	assertOK(t, r, "user_prompt_submit")

	r = testutil.RunBinary(t, dir, []string{"session_end"}, `{"session_id":"`+sid+`"}`)
	assertOK(t, r, "session_end")

	data := readRefBlob(t, dir, "refs/etch/sessions/"+sessionULID+":session.json")
	var session capture.Session
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatalf("invalid session JSON: %v", err)
	}
	if session.Agent.Model == nil || *session.Agent.Model != "claude-opus-4-7" {
		t.Errorf("raw_data.model lost: got %v", session.Agent.Model)
	}
	if session.Prompt == nil || session.Prompt.Text != "entire dialect prompt" {
		t.Errorf("user_prompt lost: got %v", session.Prompt)
	}
}

// TestWarningsOnMissingFields asserts the ETCH-20 contract: payloads missing
// expected fields produce a visible stderr warning, exit 0, and unchanged
// stdout.
func TestWarningsOnMissingFields(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	sid := "warn-test-001"
	// No model in either dialect AND no transcript to derive it from → warn.
	r := testutil.RunBinary(t, dir, []string{"session_start"}, `{"session_id":"`+sid+`"}`)
	assertOK(t, r, "session_start")
	if !strings.Contains(r.Stderr, "etch: warning") {
		t.Errorf("session_start without model or transcript should warn, stderr: %q", r.Stderr)
	}

	// Prompt event with no prompt in either dialect → warn, exit 0, stdout OK.
	r = testutil.RunBinary(t, dir, []string{"user_prompt_submit"}, `{"session_id":"`+sid+`"}`)
	assertOK(t, r, "user_prompt_submit")
	if !strings.Contains(r.Stderr, "etch: warning") || !strings.Contains(r.Stderr, "user_prompt_submit") {
		t.Errorf("promptless user_prompt_submit should warn, stderr: %q", r.Stderr)
	}
	if !strings.Contains(r.Stdout, `"ok":true`) {
		t.Errorf("stdout must stay the OK contract, got: %q", r.Stdout)
	}

	// Tool event with no tool_name → warn.
	r = testutil.RunBinary(t, dir, []string{"pre_tool_use"}, `{"session_id":"`+sid+`"}`)
	assertOK(t, r, "pre_tool_use")
	if !strings.Contains(r.Stderr, "etch: warning") || !strings.Contains(r.Stderr, "tool_name") {
		t.Errorf("toolless pre_tool_use should warn, stderr: %q", r.Stderr)
	}

	// Missing session_id → warn (capture no-ops but says why).
	r = testutil.RunBinary(t, dir, []string{"user_prompt_submit"}, `{"prompt":"hello"}`)
	assertOK(t, r, "user_prompt_submit no session")
	if !strings.Contains(r.Stderr, "session_id") {
		t.Errorf("missing session_id should warn, stderr: %q", r.Stderr)
	}
}

// TestModelBackfillSoftFailure: unreadable/model-less transcripts must not
// break finalize — warn and commit with null model.
func TestModelBackfillSoftFailure(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	sid := "softfail-001"
	missing := filepath.Join(dir, "nope.jsonl")
	start := `{"session_id":"` + sid + `","transcript_path":"` + missing + `","hook_event_name":"SessionStart"}`
	r := testutil.RunBinary(t, dir, []string{"session_start"}, start)
	assertOK(t, r, "session_start")

	wipFiles := findWipFiles(t, dir)
	sessionULID := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	end := `{"session_id":"` + sid + `","transcript_path":"` + missing + `","hook_event_name":"SessionEnd","reason":"other"}`
	r = testutil.RunBinary(t, dir, []string{"session_end"}, end)
	assertOK(t, r, "session_end")
	if !strings.Contains(r.Stderr, "etch: warning") {
		t.Errorf("unreadable transcript should warn at finalize, stderr: %q", r.Stderr)
	}

	data := readRefBlob(t, dir, "refs/etch/sessions/"+sessionULID+":session.json")
	var session capture.Session
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatalf("invalid session JSON: %v", err)
	}
	if session.Status != "complete" {
		t.Errorf("soft failure must still finalize: status %s", session.Status)
	}
	if session.Agent.Model != nil {
		t.Errorf("model should be null on backfill failure, got %v", *session.Agent.Model)
	}
}
