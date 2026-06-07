package main_test

import (
	"os"
	"path/filepath"
	"strings"
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

	// Top-level shape must match Entire's external.InfoResponse (protocol v1).
	checks := map[string]any{
		"protocol_version": float64(1),
		"name":             "etch",
		"type":             "etch",
		"is_preview":       false,
		"version":          "0.01.001",
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

	// Capabilities object must match Entire's agent.DeclaredCaps; only hooks
	// is declared (everything else is not implemented to Entire's protocol).
	caps, ok := m["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("missing or invalid capabilities object: %v", m["capabilities"])
	}
	capChecks := map[string]any{
		"hooks":                    true,
		"transcript_analyzer":      false,
		"transcript_preparer":      false,
		"token_calculator":         false,
		"compact_transcript":       false,
		"text_generator":           false,
		"hook_response_writer":     false,
		"subagent_aware_extractor": false,
	}
	for k, want := range capChecks {
		got, ok := caps[k]
		if !ok {
			t.Errorf("missing capability %q", k)
			continue
		}
		if got != want {
			t.Errorf("capability %q: got %v, want %v", k, got, want)
		}
	}

	// hook_names drives Entire's per-hook subcommand registration.
	names, ok := m["hook_names"].([]any)
	if !ok || len(names) != 6 {
		t.Errorf("hook_names: got %v, want 6 names", m["hook_names"])
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
	if !strings.Contains(result.Stderr, "entire-agent-etch help") {
		t.Errorf("unknown-subcommand error should point at 'entire-agent-etch help', got: %s", result.Stderr)
	}
}

// dispatchedSubcommands mirrors the switch in main.go (minus help aliases).
// Every entry must appear in the help listing — this is the discovery contract.
var dispatchedSubcommands = []string{
	// operational
	"query", "index", "archive", "restore-archive", "setup-refspec",
	// install & protocol
	"info", "detect", "install-hooks", "uninstall-hooks", "are-hooks-installed",
	"parse-hook", "extract-modified-files", "calculate-tokens",
	// hook entry points
	"session_start", "session_end", "user_prompt_submit", "stop",
	"pre_tool_use", "post_tool_use",
	// stubs
	"extract-all-modified-files", "calculate-total-tokens",
}

func assertFullListing(t *testing.T, out string) {
	t.Helper()
	if !strings.Contains(out, "usage: entire-agent-etch <subcommand>") {
		t.Errorf("listing missing usage line:\n%s", out)
	}
	for _, name := range dispatchedSubcommands {
		if !strings.Contains(out, "  "+name+" ") && !strings.Contains(out, "  "+name+"\n") {
			t.Errorf("help listing missing subcommand %q", name)
		}
	}
}

func TestHelpSubcommands(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	for _, alias := range []string{"help", "--help", "-h"} {
		t.Run(alias, func(t *testing.T) {
			result := testutil.RunBinary(t, dir, []string{alias}, "")
			if result.ExitCode != 0 {
				t.Fatalf("%s exited %d: %s", alias, result.ExitCode, result.Stderr)
			}
			assertFullListing(t, result.Stdout)
		})
	}
}

func TestNoSubcommand(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	result := testutil.RunBinary(t, dir, []string{}, "")
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code when no subcommand given")
	}
	// Bare invocation is an error, but it must still show the full listing.
	assertFullListing(t, result.Stderr)
}

// TestListedSubcommandsAreDispatched guards the help listing against drift:
// every name the listing advertises must reach a real dispatch case (anything
// but "unknown subcommand"). A name added to usage.go but not main.go — or
// vice versa via dispatchedSubcommands above — fails here.
func TestListedSubcommandsAreDispatched(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	for _, name := range dispatchedSubcommands {
		t.Run(name, func(t *testing.T) {
			// Empty stdin: hooks/stubs parse it (or error), commands print
			// usage errors. The only unacceptable outcome is the default case.
			result := testutil.RunBinary(t, dir, []string{name}, "")
			if strings.Contains(result.Stderr, "unknown subcommand") {
				t.Errorf("%s is in the help listing but not dispatched:\n%s", name, result.Stderr)
			}
		})
	}
}
