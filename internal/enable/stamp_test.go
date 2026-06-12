package enable_test

// ETCH-48 acceptance tests (docs/ENABLEMENT.md edge cases): worktree
// stamping, post-checkout self-propagation, and the committed-entries-win
// dedupe. The headline cases use real `git worktree add` with the test
// binary on PATH so the post-checkout hook actually runs, exactly as it
// will in the field.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Stage-11-Agentics/etch/internal/enable"
	"github.com/Stage-11-Agentics/etch/internal/testutil"
)

// handStampJSON is the exact interim hand-stamp file shape applied to the
// c11 pilot worktrees on 2026-06-12 — `enable` must detect it as already
// stamped, never duplicate it.
const handStampJSON = `{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "sh -c 'if grep -qs entire-agent-etch .claude/settings.json; then exit 0; fi; if ! command -v entire-agent-etch >/dev/null 2>&1; then exit 0; fi; exec entire-agent-etch session_start'"
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "sh -c 'if grep -qs entire-agent-etch .claude/settings.json; then exit 0; fi; if ! command -v entire-agent-etch >/dev/null 2>&1; then exit 0; fi; exec entire-agent-etch user_prompt_submit'"
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "sh -c 'if grep -qs entire-agent-etch .claude/settings.json; then exit 0; fi; if ! command -v entire-agent-etch >/dev/null 2>&1; then exit 0; fi; exec entire-agent-etch pre_tool_use'"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "sh -c 'if grep -qs entire-agent-etch .claude/settings.json; then exit 0; fi; if ! command -v entire-agent-etch >/dev/null 2>&1; then exit 0; fi; exec entire-agent-etch post_tool_use'"
          }
        ]
      }
    ],
    "SessionEnd": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "sh -c 'if grep -qs entire-agent-etch .claude/settings.json; then exit 0; fi; if ! command -v entire-agent-etch >/dev/null 2>&1; then exit 0; fi; exec entire-agent-etch session_end'"
          }
        ]
      }
    ]
  }
}
`

// TestNewWorktreeAutoStampedAndCaptures is the headline acceptance test:
// a worktree created AFTER `etch enable` is stamped by the post-checkout
// hook with zero manual steps, and its stamped dispatch command captures
// into the shared store.
func TestNewWorktreeAutoStampedAndCaptures(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	binDir := filepath.Dir(testutil.BinaryPath(t))

	if r := testutil.RunBinary(t, dir, []string{"enable"}, ""); r.ExitCode != 0 {
		t.Fatalf("enable exited %d: %s", r.ExitCode, r.Stderr)
	}

	// git worktree add fires post-checkout; the binary must be on PATH for
	// the hook to find it (same contract as the field).
	wt := filepath.Join(t.TempDir(), "wt-headline")
	gitWithBinary(t, dir, binDir, "worktree", "add", wt, "-b", "wt-headline")

	stamp := filepath.Join(wt, ".claude", "settings.local.json")
	data, err := os.ReadFile(stamp)
	if err != nil {
		t.Fatalf("new worktree was not auto-stamped: %v", err)
	}
	if !strings.Contains(string(data), enable.StampCommand("session_start")) {
		t.Fatalf("stamp does not carry the guarded session_start command:\n%s", data)
	}

	// The stamped command actually captures: dispatch session_start through
	// it (the worktree's branch has no committed hooks, so the grep guard
	// passes) and expect a wip in the shared state root (the main repo).
	r := shWithBinary(t, wt, binDir, enable.StampCommand("session_start"),
		`{"session_id":"headline-1","raw_data":{}}`)
	if r.exit != 0 {
		t.Fatalf("stamped dispatch exited %d: %s", r.exit, r.stderr)
	}
	if wips := findWipFiles(t, dir); len(wips) != 1 {
		t.Errorf("expected 1 wip in the shared store after stamped dispatch, got %d", len(wips))
	}
}

// TestStampYieldsToCommittedEntries is the dedupe rule: committed entries
// win; the stamp's embedded grep guard makes it exit 0 without dispatching.
func TestStampYieldsToCommittedEntries(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	binDir := filepath.Dir(testutil.BinaryPath(t))

	if r := testutil.RunBinary(t, dir, []string{"enable"}, ""); r.ExitCode != 0 {
		t.Fatalf("enable exited %d: %s", r.ExitCode, r.Stderr)
	}

	// Committed hooks arrive (e.g. the branch merges main's enablement
	// commit). The stamped command must now yield.
	if r := testutil.RunBinary(t, dir, []string{"install-hooks"}, ""); r.ExitCode != 0 {
		t.Fatalf("install-hooks exited %d: %s", r.ExitCode, r.Stderr)
	}
	r := shWithBinary(t, dir, binDir, enable.StampCommand("session_start"),
		`{"session_id":"dedupe-1","raw_data":{}}`)
	if r.exit != 0 {
		t.Fatalf("stamped command should yield with exit 0, got %d: %s", r.exit, r.stderr)
	}
	if r.stdout != "" {
		t.Errorf("yielding stamp produced output: %q", r.stdout)
	}
	if wips := findWipFiles(t, dir); len(wips) != 0 {
		t.Errorf("stamped command captured despite committed entries: %v (double capture)", wips)
	}

	// The committed entry is the one that captures — exactly one event.
	cmd := hookCommandFromSettings(t, filepath.Join(dir, ".claude", "settings.json"), "SessionStart")
	r = shWithBinary(t, dir, binDir, cmd, `{"session_id":"dedupe-1","raw_data":{}}`)
	if r.exit != 0 {
		t.Fatalf("committed dispatch exited %d: %s", r.exit, r.stderr)
	}
	if wips := findWipFiles(t, dir); len(wips) != 1 {
		t.Errorf("expected exactly 1 capture, got %d", len(wips))
	}
}

// TestInstallHooksOnOperatorModeRepoNoDoubleCapture is edge case 6:
// team-mode install-hooks arriving on a repo with operator mode active
// must not double-capture — same mechanics as above, asserted end to end
// by running BOTH dispatch shapes for the same event.
func TestInstallHooksOnOperatorModeRepoNoDoubleCapture(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	binDir := filepath.Dir(testutil.BinaryPath(t))

	if r := testutil.RunBinary(t, dir, []string{"enable"}, ""); r.ExitCode != 0 {
		t.Fatalf("enable exited %d: %s", r.ExitCode, r.Stderr)
	}
	if r := testutil.RunBinary(t, dir, []string{"install-hooks"}, ""); r.ExitCode != 0 {
		t.Fatalf("install-hooks exited %d: %s", r.ExitCode, r.Stderr)
	}

	// Claude Code merges settings.json + settings.local.json and runs both
	// entries. Simulate exactly that.
	committed := hookCommandFromSettings(t, filepath.Join(dir, ".claude", "settings.json"), "SessionStart")
	stamped := hookCommandFromSettings(t, filepath.Join(dir, ".claude", "settings.local.json"), "SessionStart")
	shWithBinary(t, dir, binDir, committed, `{"session_id":"both-1","raw_data":{}}`)
	shWithBinary(t, dir, binDir, stamped, `{"session_id":"both-1","raw_data":{}}`)

	if wips := findWipFiles(t, dir); len(wips) != 1 {
		t.Errorf("expected exactly 1 capture with both dispatch modes active, got %d", len(wips))
	}
}

// TestDisableRemovesStampsAndPostCheckout is edge case 3 plus cleanup:
// disable stops capture everywhere and best-effort removes what enable
// wrote, preserving foreign content.
func TestDisableRemovesStampsAndPostCheckout(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	binDir := filepath.Dir(testutil.BinaryPath(t))

	// Foreign local settings exist before enable (the real c11 main
	// checkout shape).
	mustWrite(t, filepath.Join(dir, ".claude", "settings.local.json"),
		`{"permissions":{"allow":["Bash(ls)"]}}`)

	if r := testutil.RunBinary(t, dir, []string{"enable"}, ""); r.ExitCode != 0 {
		t.Fatalf("enable exited %d: %s", r.ExitCode, r.Stderr)
	}
	wt := filepath.Join(t.TempDir(), "wt-disable")
	gitWithBinary(t, dir, binDir, "worktree", "add", wt, "-b", "wt-disable-48")
	if _, err := os.Stat(filepath.Join(wt, ".claude", "settings.local.json")); err != nil {
		t.Fatalf("precondition: worktree not stamped: %v", err)
	}

	if r := testutil.RunBinary(t, dir, []string{"disable"}, ""); r.ExitCode != 0 {
		t.Fatalf("disable exited %d: %s", r.ExitCode, r.Stderr)
	}

	// Stamps gone from every worktree; foreign content preserved.
	main := readFile(t, filepath.Join(dir, ".claude", "settings.local.json"))
	if strings.Contains(main, "entire-agent-etch") {
		t.Errorf("main checkout stamp not removed:\n%s", main)
	}
	if !strings.Contains(main, `"Bash(ls)"`) {
		t.Errorf("foreign permissions content lost on disable:\n%s", main)
	}
	if data, err := os.ReadFile(filepath.Join(wt, ".claude", "settings.local.json")); err == nil && strings.Contains(string(data), "entire-agent-etch") {
		t.Errorf("worktree stamp not removed:\n%s", data)
	}

	// Post-checkout hook (etch-only) removed entirely.
	if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", "post-checkout")); !os.IsNotExist(err) {
		t.Error("etch-only post-checkout hook should be removed on disable")
	}

	// And capture is off even if a stale stamp were executed.
	r := shWithBinary(t, wt, binDir, enable.StampCommand("session_start"),
		`{"session_id":"post-disable","raw_data":{}}`)
	if r.exit != 0 {
		t.Fatalf("stale stamped dispatch exited %d: %s", r.exit, r.stderr)
	}
	if wips := findWipFiles(t, dir); len(wips) != 0 {
		t.Errorf("capture happened after disable: %v", wips)
	}
}

// TestPostCheckoutChainsWithExistingHook: a pre-existing post-checkout is
// chained with politely — enable appends the marker block, disable removes
// only the block.
func TestPostCheckoutChainsWithExistingHook(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	binDir := filepath.Dir(testutil.BinaryPath(t))

	hookPath := filepath.Join(dir, ".git", "hooks", "post-checkout")
	foreign := "#!/bin/sh\necho custom-hook-ran > .custom-hook-marker\n"
	mustWrite(t, hookPath, foreign)
	if err := os.Chmod(hookPath, 0o755); err != nil {
		t.Fatal(err)
	}

	if r := testutil.RunBinary(t, dir, []string{"enable"}, ""); r.ExitCode != 0 {
		t.Fatalf("enable exited %d: %s", r.ExitCode, r.Stderr)
	}
	got := readFile(t, hookPath)
	if !strings.HasPrefix(got, foreign) {
		t.Errorf("foreign hook content disturbed:\n%s", got)
	}
	if !strings.Contains(got, "stamp-worktree") {
		t.Errorf("etch block not appended:\n%s", got)
	}

	// Both halves run: worktree add triggers the chained hook — foreign
	// marker appears in the new worktree AND the stamp lands.
	wt := filepath.Join(t.TempDir(), "wt-chain")
	gitWithBinary(t, dir, binDir, "worktree", "add", wt, "-b", "wt-chain-48")
	if _, err := os.Stat(filepath.Join(wt, ".custom-hook-marker")); err != nil {
		t.Errorf("pre-existing hook half did not run on worktree add: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wt, ".claude", "settings.local.json")); err != nil {
		t.Errorf("etch half did not stamp the new worktree: %v", err)
	}

	if r := testutil.RunBinary(t, dir, []string{"disable"}, ""); r.ExitCode != 0 {
		t.Fatalf("disable exited %d: %s", r.ExitCode, r.Stderr)
	}
	got = readFile(t, hookPath)
	if strings.Contains(got, "stamp-worktree") {
		t.Errorf("etch block not removed on disable:\n%s", got)
	}
	if !strings.Contains(got, "custom-hook-ran") {
		t.Errorf("foreign hook content lost on disable:\n%s", got)
	}
}

// TestCustomHooksPathPropagation is edge case 4: with an absolute
// core.hooksPath (husky-style indirection), the block lands in the
// effective dir and propagation still works.
func TestCustomHooksPathPropagation(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	binDir := filepath.Dir(testutil.BinaryPath(t))

	hooksDir := filepath.Join(t.TempDir(), "custom-hooks")
	testutil.RunCmd(t, dir, "git", "config", "core.hooksPath", hooksDir)

	if r := testutil.RunBinary(t, dir, []string{"enable"}, ""); r.ExitCode != 0 {
		t.Fatalf("enable exited %d: %s", r.ExitCode, r.Stderr)
	}
	if _, err := os.Stat(filepath.Join(hooksDir, "post-checkout")); err != nil {
		t.Fatalf("post-checkout not installed into effective hooks dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", "post-checkout")); !os.IsNotExist(err) {
		t.Error("post-checkout should not be in the default hooks dir when core.hooksPath is set")
	}

	wt := filepath.Join(t.TempDir(), "wt-hookspath")
	gitWithBinary(t, dir, binDir, "worktree", "add", wt, "-b", "wt-hookspath-48")
	if _, err := os.Stat(filepath.Join(wt, ".claude", "settings.local.json")); err != nil {
		t.Errorf("propagation through custom hooksPath failed: %v", err)
	}
}

// TestEnableStampingIdempotent is edge-case hygiene: rerunning enable, or
// enabling over the interim hand-stamps, changes nothing and duplicates
// nothing.
func TestEnableStampingIdempotent(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	// Pre-seed the exact 2026-06-12 hand-stamp file.
	stampPath := filepath.Join(dir, ".claude", "settings.local.json")
	mustWrite(t, stampPath, handStampJSON)

	if r := testutil.RunBinary(t, dir, []string{"enable"}, ""); r.ExitCode != 0 {
		t.Fatalf("enable exited %d: %s", r.ExitCode, r.Stderr)
	}
	first := readFile(t, stampPath)
	if first != handStampJSON {
		t.Errorf("hand-stamped file should be detected as current and left untouched;\ngot:\n%s", first)
	}

	hookPath := filepath.Join(dir, ".git", "hooks", "post-checkout")
	hookFirst := readFile(t, hookPath)

	if r := testutil.RunBinary(t, dir, []string{"enable"}, ""); r.ExitCode != 0 {
		t.Fatalf("second enable exited %d: %s", r.ExitCode, r.Stderr)
	}
	if got := readFile(t, stampPath); got != first {
		t.Errorf("rerun changed the stamp file:\n%s", got)
	}
	if got := readFile(t, hookPath); got != hookFirst {
		t.Errorf("rerun changed post-checkout:\n%s", got)
	}

	// No duplicated entries: each event carries exactly one etch command.
	if n := strings.Count(first, "entire-agent-etch session_start"); n != 1 {
		t.Errorf("session_start stamped %d times, want 1", n)
	}
}

// TestStampPreservesForeignLocalSettings: merging into a settings.local.json
// that carries operator content (permissions) keeps it byte-for-byte.
func TestStampPreservesForeignLocalSettings(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	stampPath := filepath.Join(dir, ".claude", "settings.local.json")
	mustWrite(t, stampPath, `{"permissions":{"allow":["Bash(cp x y)","WebSearch"]},"model":"opus"}`)

	if r := testutil.RunBinary(t, dir, []string{"enable"}, ""); r.ExitCode != 0 {
		t.Fatalf("enable exited %d: %s", r.ExitCode, r.Stderr)
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(readFile(t, stampPath)), &parsed); err != nil {
		t.Fatalf("stamped file is not valid JSON: %v", err)
	}
	// Foreign keys survive token-for-token (the file as a whole is
	// re-indented by the shared encoder, same as team-mode installs).
	if got := compactJSON(t, parsed["permissions"]); got != `{"allow":["Bash(cp x y)","WebSearch"]}` {
		t.Errorf("permissions not preserved: %s", got)
	}
	if got := compactJSON(t, parsed["model"]); got != `"opus"` {
		t.Errorf("model not preserved: %s", got)
	}
	if !strings.Contains(string(parsed["hooks"]), "entire-agent-etch session_end") {
		t.Errorf("hooks not stamped in: %s", parsed["hooks"])
	}
}

// TestEnableSkipsMissingWorktree: a pruned-but-listed worktree (directory
// deleted, `git worktree prune` never run) must not abort enable or
// resurrect the deleted directory; the remaining worktrees still stamp.
func TestEnableSkipsMissingWorktree(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	gone := filepath.Join(t.TempDir(), "wt-gone")
	testutil.RunCmd(t, dir, "git", "worktree", "add", gone, "-b", "wt-gone-48")
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	r := testutil.RunBinary(t, dir, []string{"enable"}, "")
	if r.ExitCode != 0 {
		t.Fatalf("enable should survive a missing worktree, exited %d: %s", r.ExitCode, r.Stderr)
	}
	if !strings.Contains(r.Stderr, "skipping missing worktree") {
		t.Errorf("expected a skip warning, stderr: %q", r.Stderr)
	}
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Error("enable resurrected the deleted worktree directory")
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "settings.local.json")); err != nil {
		t.Errorf("main checkout not stamped despite missing sibling: %v", err)
	}
}

// TestEnableLeavesNonShellHookAlone: a python/node post-checkout must not be
// corrupted with sh syntax — warn and skip, content byte-identical.
func TestEnableLeavesNonShellHookAlone(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	hookPath := filepath.Join(dir, ".git", "hooks", "post-checkout")
	foreign := "#!/usr/bin/env python3\nprint('hi')\n"
	mustWrite(t, hookPath, foreign)
	if err := os.Chmod(hookPath, 0o755); err != nil {
		t.Fatal(err)
	}

	r := testutil.RunBinary(t, dir, []string{"enable"}, "")
	if r.ExitCode != 0 {
		t.Fatalf("enable exited %d: %s", r.ExitCode, r.Stderr)
	}
	if !strings.Contains(r.Stderr, "non-shell shebang") {
		t.Errorf("expected a non-shell warning, stderr: %q", r.Stderr)
	}
	if got := readFile(t, hookPath); got != foreign {
		t.Errorf("non-shell hook was modified:\n%s", got)
	}
}

// TestStampWorktreeRequiresOperatorMode: without etch.enabled=true the
// subcommand is a silent no-op — team mode and untouched repos never grow
// local stamps from a stray checkout.
func TestStampWorktreeRequiresOperatorMode(t *testing.T) {
	for _, setup := range []struct {
		name string
		key  string
	}{
		{"key absent", ""},
		{"key false", "false"},
	} {
		t.Run(setup.name, func(t *testing.T) {
			dir := testutil.NewTestRepo(t)
			commitInitial(t, dir)
			if setup.key != "" {
				testutil.RunCmd(t, dir, "git", "config", "etch.enabled", setup.key)
			}
			r := testutil.RunBinary(t, dir, []string{"stamp-worktree"}, "")
			if r.ExitCode != 0 || r.Stdout != "" || r.Stderr != "" {
				t.Errorf("expected silent no-op: exit=%d stdout=%q stderr=%q", r.ExitCode, r.Stdout, r.Stderr)
			}
			if _, err := os.Stat(filepath.Join(dir, ".claude", "settings.local.json")); !os.IsNotExist(err) {
				t.Error("stamp file created without operator mode")
			}
		})
	}
}

// --- helpers ----------------------------------------------------------------

type shResult struct {
	stdout, stderr string
	exit           int
}

// gitWithBinary runs git with the test binary's directory prepended to PATH
// so hooks can invoke entire-agent-etch by name.
func gitWithBinary(t *testing.T, dir, binDir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// shWithBinary executes a hook dispatch command string exactly as the agent
// runtime would: through sh, with the payload on stdin, cwd at the worktree
// root, and the binary reachable on PATH.
func shWithBinary(t *testing.T, dir, binDir, command, stdin string) shResult {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exit := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		} else {
			t.Fatalf("running %q: %v", command, err)
		}
	}
	return shResult{stdout: stdout.String(), stderr: stderr.String(), exit: exit}
}

// compactJSON normalizes raw JSON for comparison.
func compactJSON(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("invalid JSON %q: %v", raw, err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// hookCommandFromSettings extracts the etch dispatch command for one event
// from a settings file — used to run exactly what is installed.
func hookCommandFromSettings(t *testing.T, path, event string) string {
	t.Helper()
	var settings struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(readFile(t, path)), &settings); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	for _, m := range settings.Hooks[event] {
		for _, h := range m.Hooks {
			if strings.Contains(h.Command, "entire-agent-etch") {
				return h.Command
			}
		}
	}
	t.Fatalf("no etch command for %s in %s", event, path)
	return ""
}
