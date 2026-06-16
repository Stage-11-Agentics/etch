package doctor_test

// ETCH-46 acceptance tests. Doctor's binary check consults PATH, so every
// invocation pins PATH explicitly via RunBinaryWithEnv — with the test
// binary's dir for healthy cases, without it for the not-on-PATH case —
// keeping results identical on dev machines (where etch may be installed)
// and CI (where it isn't).

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Stage-11-Agentics/etch/internal/capture"
	"github.com/Stage-11-Agentics/etch/internal/schema"
	"github.com/Stage-11-Agentics/etch/internal/testutil"
	"github.com/Stage-11-Agentics/etch/internal/version"
)

type jsonReport struct {
	Repo     string `json:"repo"`
	Healthy  bool   `json:"healthy"`
	Warnings bool   `json:"warnings"`
	Checks   map[string]struct {
		Status string `json:"status"`
		Detail string `json:"detail"`
	} `json:"checks"`
}

var allChecks = []string{
	"binary", "currency", "enablement", "hooks", "refspec",
	"sessions", "wip-buffers", "stamps", "propagation", "dedupe",
}

func TestHealthyTeamModeRepo(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	installHooks(t, dir)
	writeSessionAged(t, dir, "01DOCTORFRESH0000000000001", time.Now().Add(-2*time.Hour))

	r, rep := runDoctor(t, dir, "--json")
	if r.ExitCode != 0 {
		t.Fatalf("healthy repo: doctor exited %d: %s%s", r.ExitCode, r.Stdout, r.Stderr)
	}
	if !rep.Healthy {
		t.Errorf("healthy=false: %+v", rep.Checks)
	}
	for _, name := range allChecks {
		if _, ok := rep.Checks[name]; !ok {
			t.Errorf("--json missing field for check %q", name)
		}
	}
	for name, want := range map[string]string{
		"binary": "ok", "hooks": "ok", "sessions": "ok", "enablement": "ok",
	} {
		if got := rep.Checks[name].Status; got != want {
			t.Errorf("%s: status %q, want %q (%s)", name, got, want, rep.Checks[name].Detail)
		}
	}
	if rep.Checks["refspec"].Status != "info" {
		t.Errorf("no-refspec must be info, got %q", rep.Checks["refspec"].Status)
	}
}

func TestHooksMissingFails(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	r, rep := runDoctor(t, dir, "--json")
	if r.ExitCode == 0 {
		t.Fatal("hooks-missing repo must exit non-zero")
	}
	if rep.Checks["hooks"].Status != "fail" {
		t.Errorf("hooks: status %q, want fail", rep.Checks["hooks"].Status)
	}
	if rep.Healthy {
		t.Error("healthy must be false")
	}
}

func TestPartialHooksFail(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	// Only one of the five events carries an etch entry.
	mustWrite(t, filepath.Join(dir, ".claude", "settings.json"),
		`{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"sh -c 'exec entire-agent-etch session_start'"}]}]}}`)

	r, rep := runDoctor(t, dir, "--json")
	if r.ExitCode == 0 {
		t.Fatal("partial hook coverage must exit non-zero")
	}
	if got := rep.Checks["hooks"]; got.Status != "fail" || !strings.Contains(got.Detail, "partial") {
		t.Errorf("hooks: got %+v, want fail with partial detail", got)
	}
}

func TestNoSessionsIsHealthy(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	installHooks(t, dir)

	r, rep := runDoctor(t, dir, "--json")
	if r.ExitCode != 0 {
		t.Fatalf("no-sessions repo must exit 0, got %d: %s", r.ExitCode, r.Stderr)
	}
	if got := rep.Checks["sessions"]; got.Status != "info" || !strings.Contains(got.Detail, "no sessions captured yet") {
		t.Errorf("sessions: got %+v", got)
	}
}

func TestStaleSessionWarns(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	installHooks(t, dir)
	writeSessionAged(t, dir, "01DOCTORSTALE0000000000001", time.Now().Add(-30*24*time.Hour))

	r, rep := runDoctor(t, dir, "--json")
	if r.ExitCode != 0 {
		t.Fatalf("stale age must be a warning (exit 0), got %d", r.ExitCode)
	}
	if got := rep.Checks["sessions"]; got.Status != "warn" {
		t.Errorf("sessions: status %q, want warn (%s)", got.Status, got.Detail)
	}
	if !rep.Warnings {
		t.Error("warnings flag should be set")
	}

	// A generous --warn-age clears it.
	_, rep = runDoctor(t, dir, "--json", "--warn-age", "100000")
	if got := rep.Checks["sessions"]; got.Status != "ok" {
		t.Errorf("with --warn-age 100000: status %q, want ok", got.Status)
	}
}

func TestBinaryNotOnPathFails(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	installHooks(t, dir)

	r := testutil.RunBinaryWithEnv(t, dir, []string{"doctor", "--json"}, "",
		map[string]string{"PATH": gitOnlyPath(t)})
	if r.ExitCode == 0 {
		t.Fatal("binary off PATH must exit non-zero")
	}
	var rep jsonReport
	if err := json.Unmarshal([]byte(r.Stdout), &rep); err != nil {
		t.Fatalf("parsing json: %v\n%s", err, r.Stdout)
	}
	if rep.Checks["binary"].Status != "fail" {
		t.Errorf("binary: status %q, want fail", rep.Checks["binary"].Status)
	}
}

// TestBrokenPathBinaryWarns: a different, broken build on PATH (exec fails
// or emits garbage) is a warn with the facts — never a doctor crash. This
// is the exact scenario doctor exists to catch.
func TestBrokenPathBinaryWarns(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	installHooks(t, dir)
	writeSessionAged(t, dir, "01DOCTORBROKEN000000000001", time.Now().Add(-time.Hour))

	fakeDir := t.TempDir()
	fake := filepath.Join(fakeDir, "entire-agent-etch")
	mustWrite(t, fake, "#!/bin/sh\necho not-json\nexit 1\n")
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatal(err)
	}

	r := testutil.RunBinaryWithEnv(t, dir, []string{"doctor", "--json"}, "",
		map[string]string{"PATH": fakeDir + ":" + gitOnlyPath(t)})
	var rep jsonReport
	if err := json.Unmarshal([]byte(r.Stdout), &rep); err != nil {
		t.Fatalf("parsing json: %v\n%s\n%s", err, r.Stdout, r.Stderr)
	}
	if got := rep.Checks["binary"]; got.Status != "warn" || !strings.Contains(got.Detail, "unknown") {
		t.Errorf("binary with broken PATH build: got %+v, want warn with unknown version", got)
	}
	if r.ExitCode != 0 {
		t.Errorf("warn-level binary mismatch must exit 0, got %d", r.ExitCode)
	}
}

func TestDisabledRepoIsHealthy(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	testutil.RunCmd(t, dir, "git", "config", "etch.enabled", "false")

	r, rep := runDoctor(t, dir, "--json")
	if r.ExitCode != 0 {
		t.Fatalf("explicitly disabled repo must be healthy (exit 0), got %d: %s", r.ExitCode, r.Stderr)
	}
	if got := rep.Checks["enablement"]; got.Status != "info" || !strings.Contains(got.Detail, "disabled") {
		t.Errorf("enablement: got %+v", got)
	}
	if got := rep.Checks["hooks"]; got.Status != "info" {
		t.Errorf("hooks with capture disabled: status %q, want info (%s)", got.Status, got.Detail)
	}
}

func TestOperatorModeAllGreen(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	if r := testutil.RunBinary(t, dir, []string{"enable"}, ""); r.ExitCode != 0 {
		t.Fatalf("enable: %s", r.Stderr)
	}
	writeSessionAged(t, dir, "01DOCTOROPMODE000000000001", time.Now().Add(-time.Hour))

	r, rep := runDoctor(t, dir, "--json")
	if r.ExitCode != 0 {
		t.Fatalf("operator-mode repo: exit %d: %s%s", r.ExitCode, r.Stdout, r.Stderr)
	}
	for name, want := range map[string]string{
		"enablement": "ok", "stamps": "ok", "propagation": "ok", "dedupe": "ok", "hooks": "ok",
	} {
		if got := rep.Checks[name].Status; got != want {
			t.Errorf("%s: status %q, want %q (%s)", name, got, want, rep.Checks[name].Detail)
		}
	}
}

func TestUnstampedWorktreeWarns(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	if r := testutil.RunBinary(t, dir, []string{"enable"}, ""); r.ExitCode != 0 {
		t.Fatalf("enable: %s", r.Stderr)
	}
	// Worktree added with the binary NOT reachable: the post-checkout
	// command -v guard no-ops and the worktree stays unstamped.
	wt := filepath.Join(t.TempDir(), "wt-unstamped")
	gitBarePath(t, dir, "worktree", "add", wt, "-b", "wt-unstamped-46")
	if _, err := os.Stat(filepath.Join(wt, ".claude", "settings.local.json")); err == nil {
		t.Fatal("precondition: worktree should be unstamped")
	}

	r, rep := runDoctor(t, dir, "--json")
	if r.ExitCode != 0 {
		t.Fatalf("stamp gap must be warn-level (exit 0), got %d", r.ExitCode)
	}
	if got := rep.Checks["stamps"]; got.Status != "warn" || !strings.Contains(got.Detail, "wt-unstamped") {
		t.Errorf("stamps: got %+v", got)
	}
}

func TestRelativeHooksPathWarns(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	testutil.RunCmd(t, dir, "git", "config", "core.hooksPath", ".githooks")
	if r := testutil.RunBinary(t, dir, []string{"enable"}, ""); r.ExitCode != 0 {
		t.Fatalf("enable: %s", r.Stderr)
	}

	_, rep := runDoctor(t, dir, "--json")
	if got := rep.Checks["propagation"]; got.Status != "warn" || !strings.Contains(got.Detail, "RELATIVE") {
		t.Errorf("propagation: got %+v, want relative-hooksPath warn", got)
	}
}

func TestUnguardedStampWarns(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	if r := testutil.RunBinary(t, dir, []string{"enable"}, ""); r.ExitCode != 0 {
		t.Fatalf("enable: %s", r.Stderr)
	}
	// Replace the main checkout's stamp with one lacking the grep guard.
	mustWrite(t, filepath.Join(dir, ".claude", "settings.local.json"),
		`{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"sh -c 'exec entire-agent-etch session_start'"}]},{"matcher":"","hooks":[{"type":"command","command":"x"}]}],"UserPromptSubmit":[{"matcher":"","hooks":[{"type":"command","command":"sh -c 'exec entire-agent-etch user_prompt_submit'"}]}],"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"sh -c 'exec entire-agent-etch pre_tool_use'"}]}],"PostToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"sh -c 'exec entire-agent-etch post_tool_use'"}]}],"SessionEnd":[{"matcher":"","hooks":[{"type":"command","command":"sh -c 'exec entire-agent-etch session_end'"}]}]}}`)

	_, rep := runDoctor(t, dir, "--json")
	if got := rep.Checks["dedupe"]; got.Status != "warn" || !strings.Contains(got.Detail, "grep guard") {
		t.Errorf("dedupe: got %+v, want unguarded-stamp warn", got)
	}
}

func TestGrepGuardFalsePositiveWarns(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	if r := testutil.RunBinary(t, dir, []string{"enable"}, ""); r.ExitCode != 0 {
		t.Fatalf("enable: %s", r.Stderr)
	}
	// settings.json mentions the binary (a permissions rule) but carries no
	// etch hook entries: every stamp yields to it and nothing captures.
	mustWrite(t, filepath.Join(dir, ".claude", "settings.json"),
		`{"permissions":{"allow":["Bash(entire-agent-etch:*)"]}}`)

	_, rep := runDoctor(t, dir, "--json")
	if got := rep.Checks["dedupe"]; got.Status != "warn" || !strings.Contains(got.Detail, "false positive") {
		t.Errorf("dedupe: got %+v, want false-positive warn", got)
	}
}

func TestStampsWithoutKeyWarn(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	installHooks(t, dir)
	// Hand-stamp shape present, but no etch.enabled key.
	mustWrite(t, filepath.Join(dir, ".claude", "settings.local.json"),
		`{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"sh -c 'if grep -qs entire-agent-etch .claude/settings.json; then exit 0; fi; if ! command -v entire-agent-etch >/dev/null 2>&1; then exit 0; fi; exec entire-agent-etch session_start'"}]}]}}`)

	_, rep := runDoctor(t, dir, "--json")
	if got := rep.Checks["stamps"]; got.Status != "warn" || !strings.Contains(got.Detail, "etch.enabled") {
		t.Errorf("stamps: got %+v, want stamps-without-key warn", got)
	}
}

func TestOrphanWipWarnsLiveWipDoesNot(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	installHooks(t, dir)
	sessionsDir := filepath.Join(dir, ".etch", "sessions")

	// Orphan: dead-by-construction (PID-reuse signature: bogus start time)
	// and mtime far past the 4h default recovery timeout.
	writeWip(t, sessionsDir, "01DOCTORDEAD00000000000001", os.Getpid(), `"bogus-start-time"`, -30*time.Hour)
	r, rep := runDoctor(t, dir, "--json")
	if r.ExitCode != 0 {
		t.Fatalf("orphan wip must be warn-level (exit 0), got %d", r.ExitCode)
	}
	if got := rep.Checks["wip-buffers"]; got.Status != "warn" || !strings.Contains(got.Detail, "1 orphaned") {
		t.Errorf("wip-buffers: got %+v, want orphan warn", got)
	}

	// Live: this test process, real start time — never a warning, however old.
	if err := os.Remove(filepath.Join(sessionsDir, "01DOCTORDEAD00000000000001.wip.jsonl")); err != nil {
		t.Fatal(err)
	}
	startJSON, _ := json.Marshal(processStartTime(t))
	writeWip(t, sessionsDir, "01DOCTORLIVE00000000000001", os.Getpid(), string(startJSON), -30*time.Hour)
	_, rep = runDoctor(t, dir, "--json")
	if got := rep.Checks["wip-buffers"]; got.Status != "ok" || !strings.Contains(got.Detail, "1 live") {
		t.Errorf("wip-buffers: got %+v, want live ok", got)
	}
}

func TestDoctorIsReadOnly(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	installHooks(t, dir)
	writeSessionAged(t, dir, "01DOCTORRDONLY000000000001", time.Now().Add(-time.Hour))

	before := snapshotTree(t, dir)
	runDoctor(t, dir)
	runDoctor(t, dir, "--json")
	after := snapshotTree(t, dir)
	if before != after {
		t.Errorf("doctor modified the repo:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// The currency check must always be present and carry a real status that
// surfaces the running binary's version. The test binary is built fresh by
// testutil (no ldflags) but inside the git worktree, so its identity comes
// from the Go VCS stamp — status is ok or warn, never fail, never absent.
func TestCurrencyCheckPresent(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	installHooks(t, dir)

	_, rep := runDoctor(t, dir, "--json")
	c, ok := rep.Checks["currency"]
	if !ok {
		t.Fatal("doctor --json missing the currency check")
	}
	if c.Status != "ok" && c.Status != "warn" {
		t.Errorf("currency status %q, want ok or warn", c.Status)
	}
	if !strings.Contains(c.Detail, version.Version) {
		t.Errorf("currency detail should surface the version, got %q", c.Detail)
	}
}

// info must expose commit + build_date so doctor (and humans) can read the
// PATH binary's identity over the discovery protocol.
func TestInfoExposesBuildIdentity(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	r := testutil.RunBinary(t, dir, []string{"info"}, "")
	if r.ExitCode != 0 {
		t.Fatalf("info exited %d: %s", r.ExitCode, r.Stderr)
	}
	m := testutil.MustParseJSON(t, r.Stdout)
	// The protocol contract is that the keys exist so doctor can read them.
	// Their values come from ldflags or the VCS stamp — an unstamped build
	// (e.g. a plain `go build`, or a build from a linked worktree where Go
	// omits VCS info) legitimately leaves them empty; resolution itself is
	// covered deterministically in the version package tests.
	for _, key := range []string{"commit", "build_date"} {
		if _, ok := m[key]; !ok {
			t.Errorf("info JSON missing %q field", key)
		}
	}
}

func TestUnknownFlagErrors(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	r := testutil.RunBinaryWithEnv(t, dir, []string{"doctor", "--bogus"}, "", pathEnv(t))
	if r.ExitCode == 0 {
		t.Fatal("unknown flag must error")
	}
}

func TestNonGitDirErrors(t *testing.T) {
	r := testutil.RunBinaryWithEnv(t, t.TempDir(), []string{"doctor"}, "", pathEnv(t))
	if r.ExitCode == 0 {
		t.Fatal("doctor outside a git repo must error")
	}
}

// --- helpers ----------------------------------------------------------------

// gitOnlyPath returns a PATH that resolves git and the core shell tools but
// never the etch binary — derived from where git actually lives instead of
// assuming /usr/bin, so the binary check asserts exactly one thing.
func gitOnlyPath(t *testing.T) string {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal("git not on PATH")
	}
	return filepath.Dir(gitPath) + ":/usr/bin:/bin"
}

// pathEnv returns a PATH that resolves git, sh, and the test binary —
// deterministic regardless of what the host has installed.
func pathEnv(t *testing.T) map[string]string {
	t.Helper()
	binDir := filepath.Dir(testutil.BinaryPath(t))
	return map[string]string{"PATH": binDir + ":" + gitOnlyPath(t)}
}

func runDoctor(t *testing.T, dir string, args ...string) (testutil.BinaryResult, jsonReport) {
	t.Helper()
	r := testutil.RunBinaryWithEnv(t, dir, append([]string{"doctor"}, args...), "", pathEnv(t))
	var rep jsonReport
	if len(args) > 0 && args[0] == "--json" {
		if err := json.Unmarshal([]byte(r.Stdout), &rep); err != nil {
			t.Fatalf("parsing doctor --json output: %v\n%s\n%s", err, r.Stdout, r.Stderr)
		}
	}
	return r, rep
}

func installHooks(t *testing.T, dir string) {
	t.Helper()
	if r := testutil.RunBinary(t, dir, []string{"install-hooks"}, ""); r.ExitCode != 0 {
		t.Fatalf("install-hooks: %s", r.Stderr)
	}
}

// writeSessionAged seeds a session ref whose committer date (creatordate)
// is endedAt — the field doctor's age check reads.
func writeSessionAged(t *testing.T, dir, ulid string, endedAt time.Time) {
	t.Helper()
	ended := endedAt.UTC().Format(time.RFC3339)
	testutil.WriteSession(t, dir, schema.Session{
		SessionID: ulid,
		Status:    "complete",
		Agent:     schema.Agent{Runtime: "claude-code"},
		Timing:    schema.Timing{EndedAt: &ended},
	})
}

func writeWip(t *testing.T, sessionsDir, ulid string, pid int, pidStartJSON string, mtimeOffset time.Duration) {
	t.Helper()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionsDir, ulid+".wip.jsonl")
	line := `{"ts":"` + time.Now().UTC().Format(time.RFC3339Nano) + `","hook":"session_start","data":{"session_id":"` + ulid + `","pid":` + strconv.Itoa(pid) + `,"pid_start_time":` + pidStartJSON + `}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(mtimeOffset)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

func processStartTime(t *testing.T) string {
	t.Helper()
	start, ok := capture.ProcessStartTime(os.Getpid())
	if !ok {
		t.Skip("cannot read own process start time")
	}
	return start
}

// snapshotTree fingerprints every file (path, size, mtime) under dir,
// including .git and .etch.
func snapshotTree(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			b.WriteString(rel + " " + strconv.FormatInt(info.Size(), 10) + " " + info.ModTime().String() + "\n")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// gitBarePath runs git with a PATH that cannot resolve the etch binary, so
// post-checkout's command -v guard no-ops.
func gitBarePath(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH=/usr/bin:/bin")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commitInitial(t *testing.T, dir string) {
	t.Helper()
	mustWrite(t, filepath.Join(dir, "init.txt"), "init")
	testutil.RunCmd(t, dir, "git", "add", "init.txt")
	testutil.RunCmd(t, dir, "git", "commit", "-m", "initial commit")
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
