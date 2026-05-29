package hooks_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forgejo.stage11.ai/s11/etch/internal/testutil"
)

func TestE2EFullLifecycle(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	entireSessionID := "e2e-lifecycle-001"

	// 1. session_start
	startInput := `{"session_id":"` + entireSessionID + `","session_ref":"/tmp/test.jsonl","raw_data":{"model":"claude-opus-4-7"}}`
	r := testutil.RunBinary(t, dir, []string{"session_start"}, startInput)
	assertOK(t, r, "session_start")

	// Verify .wip file exists
	wipFiles := findWipFiles(t, dir)
	if len(wipFiles) != 1 {
		t.Fatalf("expected 1 wip file after session_start, got %d", len(wipFiles))
	}

	// Extract the ULID from the wip filename
	wipBase := filepath.Base(wipFiles[0])
	sessionULID := strings.TrimSuffix(wipBase, ".wip.jsonl")

	// 2. user_prompt_submit (include a secret to verify redaction)
	promptInput := `{"session_id":"` + entireSessionID + `","user_prompt":"fix the bug, key is sk-ant-abc123456789012345678901234567890"}`
	r = testutil.RunBinary(t, dir, []string{"user_prompt_submit"}, promptInput)
	assertOK(t, r, "user_prompt_submit")

	// 3. pre_tool_use
	toolInput := `{"session_id":"` + entireSessionID + `","tool_name":"Read","tool_use_id":"tu-1","tool_input":{"file_path":"/tmp/foo.go"}}`
	r = testutil.RunBinary(t, dir, []string{"pre_tool_use"}, toolInput)
	assertOK(t, r, "pre_tool_use")

	// 4. post_tool_use
	r = testutil.RunBinary(t, dir, []string{"post_tool_use"}, toolInput)
	assertOK(t, r, "post_tool_use")

	// 5. session_end
	endInput := `{"session_id":"` + entireSessionID + `"}`
	r = testutil.RunBinary(t, dir, []string{"session_end"}, endInput)
	assertOK(t, r, "session_end")

	// VERIFY: .wip file is cleaned up
	wipFilesAfter := findWipFiles(t, dir)
	if len(wipFilesAfter) != 0 {
		t.Errorf("expected 0 wip files after session_end, got %d", len(wipFilesAfter))
	}

	// VERIFY: .session.json is cleaned up
	sessionJSONFiles := findSessionJSONFiles(t, dir)
	if len(sessionJSONFiles) != 0 {
		t.Errorf("expected 0 session.json files after commit, got %d", len(sessionJSONFiles))
	}

	// VERIFY: git ref exists
	refName := "refs/etch/sessions/" + sessionULID
	refCheck := exec.Command("git", "show-ref", "--verify", refName)
	refCheck.Dir = dir
	if err := refCheck.Run(); err != nil {
		t.Fatalf("ref %s does not exist: %v", refName, err)
	}

	// VERIFY: session.json in the ref is valid
	sessionData := gitShow(t, dir, refName+":session.json")
	var session map[string]any
	if err := json.Unmarshal([]byte(sessionData), &session); err != nil {
		t.Fatalf("invalid session.json in ref: %v", err)
	}

	if session["schema_version"] != "etch.session.v1" {
		t.Errorf("schema_version: got %v", session["schema_version"])
	}
	if session["session_id"] != sessionULID {
		t.Errorf("session_id: got %v, want %s", session["session_id"], sessionULID)
	}
	if session["status"] != "complete" {
		t.Errorf("status: got %v", session["status"])
	}
	if session["exit_reason"] != "normal" {
		t.Errorf("exit_reason: got %v", session["exit_reason"])
	}

	// VERIFY: prompt text has secret redacted
	prompt, _ := session["prompt"].(map[string]any)
	if prompt == nil {
		t.Fatal("expected non-nil prompt")
	}
	promptText, _ := prompt["text"].(string)
	if strings.Contains(promptText, "sk-ant-abc123456789") {
		t.Error("secret was NOT redacted from prompt text")
	}
	if !strings.Contains(promptText, "[REDACTED:") {
		t.Error("expected [REDACTED:...] marker in prompt text")
	}

	// VERIFY: agent-trace.json in the ref is valid
	traceData := gitShow(t, dir, refName+":agent-trace.json")
	var trace map[string]any
	if err := json.Unmarshal([]byte(traceData), &trace); err != nil {
		t.Fatalf("invalid agent-trace.json in ref: %v", err)
	}
	if trace["version"] != "1.0" {
		t.Errorf("agent-trace version: got %v", trace["version"])
	}
	traces, _ := trace["traces"].([]any)
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace entry, got %d", len(traces))
	}
	traceEntry, _ := traces[0].(map[string]any)
	if traceEntry["session_id"] != sessionULID {
		t.Errorf("trace session_id: got %v", traceEntry["session_id"])
	}

	// VERIFY: mapping file is cleaned up
	mapDir := filepath.Join(dir, ".etch", "sessions", ".map")
	entries, _ := os.ReadDir(mapDir)
	if len(entries) != 0 {
		t.Errorf("expected 0 mapping files after commit, got %d", len(entries))
	}
}

func TestE2EStopHookWritesRef(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	entireSessionID := "e2e-stop-001"

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

	// Verify ref exists
	refName := "refs/etch/sessions/" + sessionULID
	refCheck := exec.Command("git", "show-ref", "--verify", refName)
	refCheck.Dir = dir
	if err := refCheck.Run(); err != nil {
		t.Fatalf("ref %s does not exist after stop: %v", refName, err)
	}

	// Verify .wip cleaned up
	if len(findWipFiles(t, dir)) != 0 {
		t.Error("wip file should be cleaned up after stop")
	}
}

func TestE2ECrashRecovery(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	// Create a fake orphaned .wip.jsonl file with a dead PID
	sessionsDir := filepath.Join(dir, ".etch", "sessions")
	os.MkdirAll(filepath.Join(sessionsDir, ".map"), 0o755)

	orphanedID := "01TESTORPHAN00000000000000"
	wipPath := filepath.Join(sessionsDir, orphanedID+".wip.jsonl")

	ts := time.Now().Add(-5 * time.Hour).UTC().Format(time.RFC3339Nano)
	// Use PID 999999 which is almost certainly dead
	wipContent := `{"ts":"` + ts + `","hook":"session_start","data":{"session_id":"` + orphanedID + `","agent":{"runtime":"claude-code","model":"claude-opus-4-7"},"orchestration":{"type":"manual","extra":{}},"machine":{"hostname_hash":"sha256:test","os":"darwin","os_version":"Darwin 25.5.0","arch":"arm64"},"operator":{"git_user":"Test <test@test.local>","os_user":"test"},"git_state":{"branch":"main","head_sha":"abc123"}}}` + "\n"
	wipContent += `{"ts":"` + ts + `","hook":"user_prompt_submit","data":{"prompt":"fix bugs","source":"interactive","truncated":false}}` + "\n"

	if err := os.WriteFile(wipPath, []byte(wipContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Now run session_start — this should trigger recovery
	entireSessionID := "e2e-recovery-trigger-001"
	startInput := `{"session_id":"` + entireSessionID + `","raw_data":{"model":"claude-opus-4-7"}}`
	r := testutil.RunBinary(t, dir, []string{"session_start"}, startInput)
	assertOK(t, r, "session_start (recovery trigger)")

	// Verify the orphaned session was recovered as a ref
	refName := "refs/etch/sessions/" + orphanedID
	refCheck := exec.Command("git", "show-ref", "--verify", refName)
	refCheck.Dir = dir
	if err := refCheck.Run(); err != nil {
		t.Fatalf("recovered ref %s does not exist: %v", refName, err)
	}

	// Verify the orphaned .wip file is cleaned up
	if _, err := os.Stat(wipPath); !os.IsNotExist(err) {
		t.Error("orphaned .wip file should be cleaned up after recovery")
	}

	// Verify the recovered session has status=incomplete
	sessionData := gitShow(t, dir, refName+":session.json")
	var session map[string]any
	json.Unmarshal([]byte(sessionData), &session)
	if session["status"] != "incomplete" {
		t.Errorf("recovered session status: got %v, want incomplete", session["status"])
	}
	if session["exit_reason"] != "crash" {
		t.Errorf("recovered session exit_reason: got %v, want crash", session["exit_reason"])
	}
}

func TestE2ECapabilitySubcommands(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	// Create a full session with files and tokens
	entireSessionID := "e2e-caps-001"

	startInput := `{"session_id":"` + entireSessionID + `","raw_data":{"model":"claude-opus-4-7"}}`
	r := testutil.RunBinary(t, dir, []string{"session_start"}, startInput)
	assertOK(t, r, "session_start")

	wipFiles := findWipFiles(t, dir)
	sessionULID := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	promptInput := `{"session_id":"` + entireSessionID + `","user_prompt":"implement feature"}`
	r = testutil.RunBinary(t, dir, []string{"user_prompt_submit"}, promptInput)
	assertOK(t, r, "user_prompt_submit")

	// Create a real file change so files_touched is populated
	os.WriteFile(filepath.Join(dir, "new_file.go"), []byte("package main"), 0o644)
	run(t, dir, "git", "add", "new_file.go")
	run(t, dir, "git", "commit", "-m", "add new file")

	endInput := `{"session_id":"` + entireSessionID + `"}`
	r = testutil.RunBinary(t, dir, []string{"session_end"}, endInput)
	assertOK(t, r, "session_end")

	// Test extract-modified-files
	r = testutil.RunBinary(t, dir, []string{"extract-modified-files", sessionULID}, "")
	if r.ExitCode != 0 {
		t.Fatalf("extract-modified-files failed: %s", r.Stderr)
	}
	var files []map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(r.Stdout)), &files); err != nil {
		t.Fatalf("invalid extract-modified-files output: %v\n%s", err, r.Stdout)
	}
	found := false
	for _, f := range files {
		if f["path"] == "new_file.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("extract-modified-files did not include new_file.go, got: %v", files)
	}

	// Test calculate-tokens (tokens may be nil, should still return valid JSON)
	r = testutil.RunBinary(t, dir, []string{"calculate-tokens", sessionULID}, "")
	if r.ExitCode != 0 {
		t.Fatalf("calculate-tokens failed: %s", r.Stderr)
	}
	var tokens map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(r.Stdout)), &tokens); err != nil {
		t.Fatalf("invalid calculate-tokens output: %v\n%s", err, r.Stdout)
	}
}

func TestE2ESetupRefspec(t *testing.T) {
	dir := testutil.NewTestRepo(t)

	// Create a remote so refspec config has somewhere to go
	remoteDir := t.TempDir()
	run(t, remoteDir, "git", "init", "--bare")
	run(t, dir, "git", "remote", "add", "origin", remoteDir)

	r := testutil.RunBinary(t, dir, []string{"setup-refspec"}, "")
	if r.ExitCode != 0 {
		t.Fatalf("setup-refspec failed: %s", r.Stderr)
	}
	if !strings.Contains(r.Stdout, "configured") {
		t.Errorf("expected 'configured' in output, got: %s", r.Stdout)
	}

	// Verify push refspec
	pushCheck := exec.Command("git", "config", "--get-all", "remote.origin.push")
	pushCheck.Dir = dir
	out, err := pushCheck.Output()
	if err != nil {
		t.Fatalf("reading push config: %v", err)
	}
	if !strings.Contains(string(out), "refs/etch/sessions/*:refs/etch/sessions/*") {
		t.Error("push refspec not configured")
	}

	// Verify fetch refspec
	fetchCheck := exec.Command("git", "config", "--get-all", "remote.origin.fetch")
	fetchCheck.Dir = dir
	out, err = fetchCheck.Output()
	if err != nil {
		t.Fatalf("reading fetch config: %v", err)
	}
	if !strings.Contains(string(out), "refs/etch/sessions/*:refs/etch/sessions/*") {
		t.Error("fetch refspec not configured")
	}

	// Run again — should be idempotent
	r = testutil.RunBinary(t, dir, []string{"setup-refspec"}, "")
	if r.ExitCode != 0 {
		t.Fatalf("setup-refspec (idempotent) failed: %s", r.Stderr)
	}

	// Verify not duplicated
	pushCheck2 := exec.Command("git", "config", "--get-all", "remote.origin.push")
	pushCheck2.Dir = dir
	out2, _ := pushCheck2.Output()
	count := strings.Count(string(out2), "refs/etch/sessions/*:refs/etch/sessions/*")
	if count != 1 {
		t.Errorf("push refspec duplicated: found %d entries", count)
	}
}

// gitShow reads a file from a git ref
func gitShow(t *testing.T, dir, refPath string) string {
	t.Helper()
	cmd := exec.Command("git", "show", refPath)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git show %s: %v", refPath, err)
	}
	return string(out)
}
