package enable_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Stage-11-Agentics/etch/internal/testutil"
)

// hookEvents are all guarded hook entrypoints.
var hookEvents = []string{
	"session_start", "session_end", "user_prompt_submit",
	"stop", "pre_tool_use", "post_tool_use",
}

func TestEnableSetsKeyAndWritesExcludes(t *testing.T) {
	dir := testutil.NewTestRepo(t)

	r := testutil.RunBinary(t, dir, []string{"enable"}, "")
	if r.ExitCode != 0 {
		t.Fatalf("enable exited %d: %s", r.ExitCode, r.Stderr)
	}

	if got := gitConfig(t, dir, "etch.enabled"); got != "true" {
		t.Errorf("etch.enabled = %q, want true", got)
	}

	exclude := readFile(t, filepath.Join(dir, ".git", "info", "exclude"))
	for _, want := range []string{".etch/*", "!.etch/settings.json", ".claude/settings.local.json"} {
		if !strings.Contains(exclude, want) {
			t.Errorf("info/exclude missing %q:\n%s", want, exclude)
		}
	}

	// The excludes must actually work: untracked operator-mode files stay
	// out of git status, while .etch/settings.json remains visible.
	mustWrite(t, filepath.Join(dir, ".claude", "settings.local.json"), `{}`)
	mustWrite(t, filepath.Join(dir, ".etch", "sessions", "x.wip.jsonl"), `{}`)
	mustWrite(t, filepath.Join(dir, ".etch", "settings.json"), `{}`)
	status := gitOut(t, dir, "status", "--porcelain", "-uall")
	if strings.Contains(status, "settings.local.json") || strings.Contains(status, "x.wip.jsonl") {
		t.Errorf("excluded files leaked into git status:\n%s", status)
	}
	if !strings.Contains(status, ".etch/settings.json") {
		t.Errorf(".etch/settings.json carve-out broken — should be visible:\n%s", status)
	}
}

func TestEnableIsIdempotent(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	excludePath := filepath.Join(dir, ".git", "info", "exclude")
	mustWrite(t, excludePath, "preexisting/\n")

	r := testutil.RunBinary(t, dir, []string{"enable"}, "")
	if r.ExitCode != 0 {
		t.Fatalf("first enable exited %d: %s", r.ExitCode, r.Stderr)
	}
	first := readFile(t, excludePath)
	stat1, _ := os.Stat(excludePath)

	time.Sleep(10 * time.Millisecond)
	r = testutil.RunBinary(t, dir, []string{"enable"}, "")
	if r.ExitCode != 0 {
		t.Fatalf("second enable exited %d: %s", r.ExitCode, r.Stderr)
	}
	second := readFile(t, excludePath)
	stat2, _ := os.Stat(excludePath)

	if first != second {
		t.Errorf("rerun changed info/exclude:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if !strings.HasPrefix(first, "preexisting/\n") {
		t.Errorf("foreign exclude content not preserved:\n%s", first)
	}
	if !stat2.ModTime().Equal(stat1.ModTime()) {
		t.Error("rerun rewrote an up-to-date info/exclude (should not touch the file)")
	}
}

func TestEnableRefreshesStaleBlock(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	excludePath := filepath.Join(dir, ".git", "info", "exclude")
	mustWrite(t, excludePath, "before/\n# >>> etch >>>\nstale-entry\n# <<< etch <<<\nafter/\n")

	r := testutil.RunBinary(t, dir, []string{"enable"}, "")
	if r.ExitCode != 0 {
		t.Fatalf("enable exited %d: %s", r.ExitCode, r.Stderr)
	}
	got := readFile(t, excludePath)
	if strings.Contains(got, "stale-entry") {
		t.Errorf("stale block content survived:\n%s", got)
	}
	if !strings.HasPrefix(got, "before/\n") || !strings.HasSuffix(got, "after/\n") {
		t.Errorf("neighbors of the etch block were disturbed:\n%s", got)
	}
	if !strings.Contains(got, ".claude/settings.local.json") {
		t.Errorf("refreshed block incomplete:\n%s", got)
	}
}

func TestEnableFromWorktreeWritesSharedState(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	wt := addWorktree(t, dir, "wt-enable")

	r := testutil.RunBinary(t, wt, []string{"enable"}, "")
	if r.ExitCode != 0 {
		t.Fatalf("enable in worktree exited %d: %s", r.ExitCode, r.Stderr)
	}

	// The key and excludes land in the MAIN repo's shared state, visible
	// from both the worktree and the main checkout.
	for _, d := range []string{dir, wt} {
		if got := gitConfig(t, d, "etch.enabled"); got != "true" {
			t.Errorf("etch.enabled from %s = %q, want true", d, got)
		}
	}
	exclude := readFile(t, filepath.Join(dir, ".git", "info", "exclude"))
	if !strings.Contains(exclude, ".etch/*") {
		t.Errorf("exclude block not in common dir:\n%s", exclude)
	}
}

func TestDisableStopsAllCaptureIncludingWorktrees(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	wt := addWorktree(t, dir, "wt-disable")

	// Committed hooks present (team mode) — the explicit off-switch must
	// win over them too.
	mustWrite(t, filepath.Join(dir, ".claude", "settings.json"),
		`{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"entire-agent-etch session_start"}]}]}}`)

	r := testutil.RunBinary(t, dir, []string{"disable"}, "")
	if r.ExitCode != 0 {
		t.Fatalf("disable exited %d: %s", r.ExitCode, r.Stderr)
	}
	if got := gitConfig(t, dir, "etch.enabled"); got != "false" {
		t.Fatalf("etch.enabled = %q, want false", got)
	}

	for _, root := range []string{dir, wt} {
		for _, ev := range hookEvents {
			r := testutil.RunBinary(t, root, []string{ev}, `{"session_id":"disabled-test","raw_data":{}}`)
			if r.ExitCode != 0 {
				t.Errorf("%s in %s exited %d, want 0", ev, root, r.ExitCode)
			}
			if r.Stdout != "" || r.Stderr != "" {
				t.Errorf("%s in %s produced output: stdout=%q stderr=%q", ev, root, r.Stdout, r.Stderr)
			}
		}
		if wips := findWipFiles(t, root); len(wips) != 0 {
			t.Errorf("disabled hooks created wip files in %s: %v", root, wips)
		}
	}
	if refs := sessionRefs(t, dir); len(refs) != 0 {
		t.Errorf("disabled hooks created session refs: %v", refs)
	}
}

// TestTeamModeWithoutKeyStillCaptures is the compatibility rule: committed
// hooks dispatching in a repo with NO etch.enabled key must capture exactly
// as before — team mode must not require `etch enable`.
func TestTeamModeWithoutKeyStillCaptures(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	r := testutil.RunBinary(t, dir, []string{"session_start"},
		`{"session_id":"team-mode-1","session_ref":"/tmp/t.jsonl","raw_data":{"model":"claude-opus-4-8"}}`)
	if r.ExitCode != 0 {
		t.Fatalf("session_start exited %d: %s", r.ExitCode, r.Stderr)
	}
	r = testutil.RunBinary(t, dir, []string{"user_prompt_submit"},
		`{"session_id":"team-mode-1","user_prompt":"hello"}`)
	if r.ExitCode != 0 {
		t.Fatalf("user_prompt_submit exited %d: %s", r.ExitCode, r.Stderr)
	}
	r = testutil.RunBinary(t, dir, []string{"session_end"}, `{"session_id":"team-mode-1"}`)
	if r.ExitCode != 0 {
		t.Fatalf("session_end exited %d: %s", r.ExitCode, r.Stderr)
	}

	refs := sessionRefs(t, dir)
	if len(refs) != 1 {
		t.Fatalf("expected exactly 1 session ref, got %v", refs)
	}
}

// TestEnabledTrueCaptures: the operator-mode on state behaves like the
// absent-key state for dispatch.
func TestEnabledTrueCaptures(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	if r := testutil.RunBinary(t, dir, []string{"enable"}, ""); r.ExitCode != 0 {
		t.Fatalf("enable exited %d: %s", r.ExitCode, r.Stderr)
	}

	r := testutil.RunBinary(t, dir, []string{"session_start"},
		`{"session_id":"op-mode-1","raw_data":{}}`)
	if r.ExitCode != 0 {
		t.Fatalf("session_start exited %d: %s", r.ExitCode, r.Stderr)
	}
	if wips := findWipFiles(t, dir); len(wips) != 1 {
		t.Fatalf("expected 1 wip after session_start, got %v", wips)
	}
}

// TestMalformedEnabledValueFailsOpen: anything other than a clean false
// reads as enabled — capture is the safe default for a key only etch writes.
func TestMalformedEnabledValueFailsOpen(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	testutil.RunCmd(t, dir, "git", "config", "etch.enabled", "maybe")

	r := testutil.RunBinary(t, dir, []string{"session_start"},
		`{"session_id":"malformed-1","raw_data":{}}`)
	if r.ExitCode != 0 {
		t.Fatalf("session_start exited %d: %s", r.ExitCode, r.Stderr)
	}
	if wips := findWipFiles(t, dir); len(wips) != 1 {
		t.Errorf("malformed value should fail open (capture), got %d wip files", len(wips))
	}
}

// TestSymlinkedGitDirStillGuardsCorrectly: a .git that is a symlink to a
// real git dir (a layout git supports) must not read as "not a repo" — that
// would silently disable capture.
func TestSymlinkedGitDirStillGuardsCorrectly(t *testing.T) {
	real := testutil.NewTestRepo(t)
	commitInitial(t, real)

	linked := filepath.Join(t.TempDir(), "linked")
	if err := os.MkdirAll(linked, 0o755); err != nil {
		t.Fatal(err)
	}
	// Move the working tree files + symlink .git into place.
	if err := os.Symlink(filepath.Join(real, ".git"), filepath.Join(linked, ".git")); err != nil {
		t.Fatal(err)
	}

	// No key: enabled — session_start must capture. State anchors at the
	// common dir's parent as seen from the hook's cwd, i.e. the symlink
	// side (linked/.etch).
	r := testutil.RunBinary(t, linked, []string{"session_start"}, `{"session_id":"symlink-1","raw_data":{}}`)
	if r.ExitCode != 0 {
		t.Fatalf("session_start exited %d: %s", r.ExitCode, r.Stderr)
	}
	if wips := findWipFiles(t, linked); len(wips) != 1 {
		t.Errorf("expected capture through symlinked .git, got %d wip files", len(wips))
	}

	// Explicit false: disabled, silently.
	testutil.RunCmd(t, real, "git", "config", "etch.enabled", "false")
	r = testutil.RunBinary(t, linked, []string{"pre_tool_use"}, `{"session_id":"symlink-1"}`)
	if r.ExitCode != 0 || r.Stdout != "" || r.Stderr != "" {
		t.Errorf("disabled hook through symlinked .git: exit=%d stdout=%q stderr=%q", r.ExitCode, r.Stdout, r.Stderr)
	}
}

func TestHooksOutsideGitRepoExitZeroSilently(t *testing.T) {
	dir := t.TempDir() // no git init
	for _, ev := range hookEvents {
		r := testutil.RunBinary(t, dir, []string{ev}, `{"session_id":"no-repo","raw_data":{}}`)
		if r.ExitCode != 0 {
			t.Errorf("%s outside a repo exited %d, want 0 (stderr: %s)", ev, r.ExitCode, r.Stderr)
		}
		if r.Stdout != "" || r.Stderr != "" {
			t.Errorf("%s outside a repo produced output: stdout=%q stderr=%q", ev, r.Stdout, r.Stderr)
		}
	}
}

func TestEnableOutsideGitRepoErrors(t *testing.T) {
	dir := t.TempDir()
	r := testutil.RunBinary(t, dir, []string{"enable"}, "")
	if r.ExitCode == 0 {
		t.Fatal("enable outside a git repo should fail")
	}
	if !strings.Contains(r.Stderr, "git repository") {
		t.Errorf("expected a clear not-a-repo error, got: %s", r.Stderr)
	}
}

// TestDisabledPathLatency measures the fast-exit path against the SPEC AC #13
// per-event budget (≤ 50 ms): with etch.enabled=false, pre_tool_use must
// stay well inside it, since it fires on every tool call.
func TestDisabledPathLatency(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	if r := testutil.RunBinary(t, dir, []string{"disable"}, ""); r.ExitCode != 0 {
		t.Fatalf("disable exited %d: %s", r.ExitCode, r.Stderr)
	}

	// Warm-up (binary build + page cache).
	testutil.RunBinary(t, dir, []string{"pre_tool_use"}, `{"session_id":"warm"}`)

	const n = 50
	durations := make([]time.Duration, 0, n)
	for i := 0; i < n; i++ {
		start := time.Now()
		r := testutil.RunBinary(t, dir, []string{"pre_tool_use"}, `{"session_id":"lat"}`)
		durations = append(durations, time.Since(start))
		if r.ExitCode != 0 {
			t.Fatalf("pre_tool_use exited %d: %s", r.ExitCode, r.Stderr)
		}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	median := durations[n/2]
	p90 := durations[n*90/100]
	t.Logf("disabled-path latency over %d runs: median=%v p90=%v max=%v", n, median, p90, durations[n-1])

	// Hard-assert on median and p90: the tail above that measures the test
	// machine's scheduler (the suite runs packages in parallel), not the
	// binary. The guard itself is a directory walk plus one config-file
	// read — single-digit milliseconds including process spawn.
	if median > 50*time.Millisecond {
		t.Errorf("disabled-path median %v exceeds the 50ms per-event budget (SPEC AC #13)", median)
	}
	if p90 > 50*time.Millisecond {
		t.Errorf("disabled-path p90 %v exceeds the 50ms per-event budget (SPEC AC #13)", p90)
	}
}

// --- helpers ---------------------------------------------------------------

func commitInitial(t *testing.T, dir string) {
	t.Helper()
	mustWrite(t, filepath.Join(dir, "init.txt"), "init")
	testutil.RunCmd(t, dir, "git", "add", "init.txt")
	testutil.RunCmd(t, dir, "git", "commit", "-m", "initial commit")
}

func addWorktree(t *testing.T, dir, name string) string {
	t.Helper()
	wt := filepath.Join(t.TempDir(), name)
	testutil.RunCmd(t, dir, "git", "worktree", "add", wt, "-b", name)
	return wt
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func gitConfig(t *testing.T, dir, key string) string {
	t.Helper()
	cmd := exec.Command("git", "config", "--get", key)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "" // unset
	}
	return strings.TrimSpace(string(out))
}

func sessionRefs(t *testing.T, dir string) []string {
	t.Helper()
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname)", "refs/etch/sessions/")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("for-each-ref failed: %v", err)
	}
	var refs []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l != "" {
			refs = append(refs, l)
		}
	}
	return refs
}

func findWipFiles(t *testing.T, root string) []string {
	t.Helper()
	dir := filepath.Join(root, ".etch", "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var found []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".wip.jsonl") {
			found = append(found, e.Name())
		}
	}
	return found
}
