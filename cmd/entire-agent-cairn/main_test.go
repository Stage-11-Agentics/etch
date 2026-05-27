package main_test

import (
	"os"
	"path/filepath"
	"testing"

	"forgejo.stage11.ai/s11/etch/internal/testutil"
)

func TestInfoSubcommand(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	result := testutil.RunBinary(t, dir, []string{"info"}, "")
	if result.ExitCode != 0 {
		t.Fatalf("info exited %d: %s", result.ExitCode, result.Stderr)
	}

	m := testutil.MustParseJSON(t, result.Stdout)

	checks := map[string]any{
		"name":                    "cairn",
		"version":                 "0.01.001",
		"hooks":                   true,
		"transcript_analyzer":     true,
		"compact_transcript":      false,
		"token_calculator":        true,
		"text_generator":          false,
		"hook_response_writer":    false,
		"subagent_aware_extractor": true,
	}
	for k, want := range checks {
		got, ok := m[k]
		if !ok {
			t.Errorf("missing field %q", k)
			continue
		}
		if got != want {
			t.Errorf("field %q: got %v, want %v", k, got, want)
		}
	}
}

func TestParseHookSessionStart(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	input := `{"session_id":"test-123","session_ref":"/tmp/transcript.jsonl","timestamp":"2026-05-27T00:00:00Z","raw_data":{"model":"claude-opus-4-7"}}`
	result := testutil.RunBinary(t, dir, []string{"parse-hook", "--hook", "session-start"}, input)
	if result.ExitCode != 0 {
		t.Fatalf("parse-hook exited %d: %s", result.ExitCode, result.Stderr)
	}

	m := testutil.MustParseJSON(t, result.Stdout)
	if m["hook_type"] != "session_start" {
		t.Errorf("hook_type: got %v, want session_start", m["hook_type"])
	}
	if m["session_id"] != "test-123" {
		t.Errorf("session_id: got %v, want test-123", m["session_id"])
	}
	if m["model"] != "claude-opus-4-7" {
		t.Errorf("model: got %v, want claude-opus-4-7", m["model"])
	}
	if m["session_ref"] != "/tmp/transcript.jsonl" {
		t.Errorf("session_ref: got %v, want /tmp/transcript.jsonl", m["session_ref"])
	}
}

func TestParseHookUserPromptSubmit(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	input := `{"session_id":"test-456","timestamp":"2026-05-27T00:00:00Z","user_prompt":"fix the login bug"}`
	result := testutil.RunBinary(t, dir, []string{"parse-hook", "--hook", "user-prompt-submit"}, input)
	if result.ExitCode != 0 {
		t.Fatalf("parse-hook exited %d: %s", result.ExitCode, result.Stderr)
	}

	m := testutil.MustParseJSON(t, result.Stdout)
	if m["hook_type"] != "user_prompt_submit" {
		t.Errorf("hook_type: got %v, want user_prompt_submit", m["hook_type"])
	}
	if m["prompt"] != "fix the login bug" {
		t.Errorf("prompt: got %v, want 'fix the login bug'", m["prompt"])
	}
}

func TestParseHookPreToolUse(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	input := `{"session_id":"test-789","timestamp":"2026-05-27T00:00:00Z","tool_name":"Read","tool_use_id":"tu-1","tool_input":{"path":"/tmp/foo"}}`
	result := testutil.RunBinary(t, dir, []string{"parse-hook", "--hook", "pre-tool-use"}, input)
	if result.ExitCode != 0 {
		t.Fatalf("parse-hook exited %d: %s", result.ExitCode, result.Stderr)
	}

	m := testutil.MustParseJSON(t, result.Stdout)
	if m["hook_type"] != "pre_tool_use" {
		t.Errorf("hook_type: got %v, want pre_tool_use", m["hook_type"])
	}
	if m["tool_name"] != "Read" {
		t.Errorf("tool_name: got %v, want Read", m["tool_name"])
	}
	if m["tool_use_id"] != "tu-1" {
		t.Errorf("tool_use_id: got %v, want tu-1", m["tool_use_id"])
	}
}

func TestParseHookMissingHookFlag(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	result := testutil.RunBinary(t, dir, []string{"parse-hook"}, `{"session_id":"x"}`)
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code when --hook flag is missing")
	}
}

func TestStubSubcommands(t *testing.T) {
	stubs := []string{
		"extract-modified-files", "calculate-tokens",
		"extract-all-modified-files", "calculate-total-tokens",
	}
	dir := testutil.NewTestRepo(t)

	for _, sub := range stubs {
		t.Run(sub, func(t *testing.T) {
			result := testutil.RunBinary(t, dir, []string{sub}, `{"session_id":"stub-test"}`)
			if result.ExitCode != 0 {
				t.Fatalf("%s exited %d: %s", sub, result.ExitCode, result.Stderr)
			}
			m := testutil.MustParseJSON(t, result.Stdout)
			if m["ok"] != true {
				t.Errorf("%s: expected ok=true, got %v", sub, m["ok"])
			}
		})
	}
}

func TestHookSubcommandsReturnOK(t *testing.T) {
	hooks := []string{
		"session_start", "session_end", "user_prompt_submit", "stop",
		"pre_tool_use", "post_tool_use",
	}
	dir := testutil.NewTestRepo(t)
	// Need an initial commit for git operations
	os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0o644)
	testutil.RunCmd(t, dir, "git", "add", "init.txt")
	testutil.RunCmd(t, dir, "git", "commit", "-m", "initial")

	for _, hook := range hooks {
		t.Run(hook, func(t *testing.T) {
			result := testutil.RunBinary(t, dir, []string{hook}, `{"session_id":"hook-test","raw_data":{},"user_prompt":"test","tool_name":"Read"}`)
			if result.ExitCode != 0 {
				t.Fatalf("%s exited %d: %s", hook, result.ExitCode, result.Stderr)
			}
			m := testutil.MustParseJSON(t, result.Stdout)
			if m["ok"] != true {
				t.Errorf("%s: expected ok=true, got %v", hook, m["ok"])
			}
		})
	}
}

func TestUnknownSubcommand(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	result := testutil.RunBinary(t, dir, []string{"nonexistent"}, "")
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code for unknown subcommand")
	}
}

func TestNoSubcommand(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	result := testutil.RunBinary(t, dir, []string{}, "")
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code when no subcommand given")
	}
}
