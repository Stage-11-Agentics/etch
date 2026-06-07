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

// writeLocalOnlySettings drops a .etch/settings.json configuring
// local_only_fields into the repo.
func writeLocalOnlySettings(t *testing.T, dir string, fields []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".etch"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, _ := json.Marshal(map[string]any{"local_only_fields": fields})
	if err := os.WriteFile(filepath.Join(dir, ".etch", "settings.json"), cfg, 0o644); err != nil {
		t.Fatal(err)
	}
}

// gitLogMessage reads the commit message of a ref.
func gitLogMessage(t *testing.T, dir, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "log", "-1", "--format=%B", ref)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log %s: %v", ref, err)
	}
	return string(out)
}

// refExists reports whether a fully-qualified ref exists.
func refExists(t *testing.T, dir, ref string) bool {
	t.Helper()
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// listRefs returns the refs under a prefix.
func listRefs(t *testing.T, dir, prefix string) []string {
	t.Helper()
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname)", prefix)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git for-each-ref %s: %v", prefix, err)
	}
	var refs []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l != "" {
			refs = append(refs, l)
		}
	}
	return refs
}

// Commit boundary: with local_only_fields configured, the canonical sessions
// ref holds the stripped projection and the full record lives only in
// refs/etch/local/ — including the trace blob and the ref's commit message.
func TestE2ELocalOnlyDualRef(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	run(t, dir, "git", "checkout", "-b", "secret-branch-zeus")

	const codename = "project-codename-hyperion-zeus"
	writeLocalOnlySettings(t, dir, []string{
		"prompt.text", "git_start.branch", "git_end.branch", "files_touched",
	})

	entireSessionID := "e2e-localonly-001"
	startInput := `{"session_id":"` + entireSessionID + `","raw_data":{"model":"claude-opus-4-7"}}`
	r := testutil.RunBinary(t, dir, []string{"session_start"}, startInput)
	assertOK(t, r, "session_start")

	wipFiles := findWipFiles(t, dir)
	sessionULID := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	promptInput := `{"session_id":"` + entireSessionID + `","user_prompt":"plan the ` + codename + ` rollout"}`
	r = testutil.RunBinary(t, dir, []string{"user_prompt_submit"}, promptInput)
	assertOK(t, r, "user_prompt_submit")

	// A committed file so files_touched (and the trace's files list) is populated.
	if err := os.WriteFile(filepath.Join(dir, "internal-plan.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-m", "add plan")

	endInput := `{"session_id":"` + entireSessionID + `"}`
	r = testutil.RunBinary(t, dir, []string{"session_end"}, endInput)
	assertOK(t, r, "session_end")

	sessionsRef := "refs/etch/sessions/" + sessionULID
	localRef := "refs/etch/local/" + sessionULID

	// Pushable record: stripped, marked, manifested.
	pushed := gitShow(t, dir, sessionsRef+":session.json")
	for _, leaked := range []string{codename, "secret-branch-zeus", "internal-plan.txt"} {
		if strings.Contains(pushed, leaked) {
			t.Errorf("pushable session.json leaks %q:\n%s", leaked, pushed)
		}
	}
	for _, want := range []string{"[LOCAL_ONLY:prompt.text]", "[LOCAL_ONLY:git_start.branch]", "local_only_stripped"} {
		if !strings.Contains(pushed, want) {
			t.Errorf("pushable session.json missing %q:\n%s", want, pushed)
		}
	}

	// Pushable trace: derived from the stripped record.
	pushedTrace := gitShow(t, dir, sessionsRef+":agent-trace.json")
	if strings.Contains(pushedTrace, "internal-plan.txt") {
		t.Errorf("pushable agent-trace.json leaks files_touched:\n%s", pushedTrace)
	}

	// Pushable ref's commit message: built from the stripped record.
	pushedMsg := gitLogMessage(t, dir, sessionsRef)
	if strings.Contains(pushedMsg, "secret-branch-zeus") {
		t.Errorf("pushable ref commit message leaks branch:\n%s", pushedMsg)
	}

	// Local record: full fidelity, no manifest.
	local := gitShow(t, dir, localRef+":session.json")
	for _, want := range []string{codename, "secret-branch-zeus", "internal-plan.txt"} {
		if !strings.Contains(local, want) {
			t.Errorf("local session.json missing %q — full fidelity lost:\n%s", want, local)
		}
	}
	if strings.Contains(local, "local_only_stripped") {
		t.Errorf("local session.json must not carry the strip manifest:\n%s", local)
	}
	localTrace := gitShow(t, dir, localRef+":agent-trace.json")
	if !strings.Contains(localTrace, "internal-plan.txt") {
		t.Errorf("local agent-trace.json lost files:\n%s", localTrace)
	}
	if !strings.Contains(gitLogMessage(t, dir, localRef), "secret-branch-zeus") {
		t.Error("local ref commit message lost branch")
	}
}

// Without local_only_fields, behavior is unchanged: full record in the
// sessions ref, no local namespace at all.
func TestE2ELocalOnlyEmptyConfigNoChange(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	writeLocalOnlySettings(t, dir, []string{})

	entireSessionID := "e2e-localonly-empty-001"
	startInput := `{"session_id":"` + entireSessionID + `","raw_data":{"model":"claude-opus-4-7"}}`
	r := testutil.RunBinary(t, dir, []string{"session_start"}, startInput)
	assertOK(t, r, "session_start")

	wipFiles := findWipFiles(t, dir)
	sessionULID := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	promptInput := `{"session_id":"` + entireSessionID + `","user_prompt":"ordinary work"}`
	r = testutil.RunBinary(t, dir, []string{"user_prompt_submit"}, promptInput)
	assertOK(t, r, "user_prompt_submit")

	endInput := `{"session_id":"` + entireSessionID + `"}`
	r = testutil.RunBinary(t, dir, []string{"session_end"}, endInput)
	assertOK(t, r, "session_end")

	pushed := gitShow(t, dir, "refs/etch/sessions/"+sessionULID+":session.json")
	if !strings.Contains(pushed, "ordinary work") {
		t.Error("full prompt should be present with empty config")
	}
	if strings.Contains(pushed, "local_only_stripped") {
		t.Error("no manifest expected with empty config")
	}
	if got := listRefs(t, dir, "refs/etch/local/"); len(got) != 0 {
		t.Errorf("no local refs expected with empty config, got %v", got)
	}
}

// Configured paths that match nothing in the record (typo'd or simply absent
// this session) are a full no-op: no local ref, no manifest, full record in
// the sessions ref.
func TestE2ELocalOnlyNoMatchNoLocalRef(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)
	writeLocalOnlySettings(t, dir, []string{"outcome.pr_number", "promt.text"})

	entireSessionID := "e2e-localonly-nomatch-001"
	startInput := `{"session_id":"` + entireSessionID + `","raw_data":{"model":"claude-opus-4-7"}}`
	r := testutil.RunBinary(t, dir, []string{"session_start"}, startInput)
	assertOK(t, r, "session_start")

	wipFiles := findWipFiles(t, dir)
	sessionULID := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	promptInput := `{"session_id":"` + entireSessionID + `","user_prompt":"ordinary work"}`
	r = testutil.RunBinary(t, dir, []string{"user_prompt_submit"}, promptInput)
	assertOK(t, r, "user_prompt_submit")

	endInput := `{"session_id":"` + entireSessionID + `"}`
	r = testutil.RunBinary(t, dir, []string{"session_end"}, endInput)
	assertOK(t, r, "session_end")

	pushed := gitShow(t, dir, "refs/etch/sessions/"+sessionULID+":session.json")
	if !strings.Contains(pushed, "ordinary work") {
		t.Error("record should be untouched when no path matches")
	}
	if strings.Contains(pushed, "local_only_stripped") {
		t.Error("no manifest expected when nothing stripped")
	}
	if got := listRefs(t, dir, "refs/etch/local/"); len(got) != 0 {
		t.Errorf("no local ref expected when nothing stripped, got %v", got)
	}
}

// The crash-recovery commit path must produce the same dual-ref projection
// as the normal path.
func TestE2ELocalOnlyCrashRecovery(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	const codename = "project-codename-recovered-atlas"
	writeLocalOnlySettings(t, dir, []string{"prompt.text"})

	sessionsDir := filepath.Join(dir, ".etch", "sessions")
	os.MkdirAll(filepath.Join(sessionsDir, ".map"), 0o755)

	orphanedID := "01TESTORPHANLOCALONLY00000"
	wipPath := filepath.Join(sessionsDir, orphanedID+".wip.jsonl")

	ts := time.Now().Add(-5 * time.Hour).UTC().Format(time.RFC3339Nano)
	wipContent := `{"ts":"` + ts + `","hook":"session_start","data":{"session_id":"` + orphanedID + `","agent":{"runtime":"claude-code","model":"claude-opus-4-7"},"orchestration":{"type":"manual","extra":{}},"machine":{"hostname_hash":"sha256:test","os":"darwin","os_version":"Darwin 25.5.0","arch":"arm64"},"operator":{"git_user":"Test <test@test.local>","os_user":"test"},"git_state":{"branch":"main","head_sha":"abc123"}}}` + "\n"
	wipContent += `{"ts":"` + ts + `","hook":"user_prompt_submit","data":{"prompt":"ship ` + codename + ` quietly","source":"interactive","truncated":false}}` + "\n"

	if err := os.WriteFile(wipPath, []byte(wipContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Trigger recovery via a fresh session_start.
	startInput := `{"session_id":"e2e-localonly-recovery-001","raw_data":{"model":"claude-opus-4-7"}}`
	r := testutil.RunBinary(t, dir, []string{"session_start"}, startInput)
	assertOK(t, r, "session_start (recovery trigger)")

	pushed := gitShow(t, dir, "refs/etch/sessions/"+orphanedID+":session.json")
	if strings.Contains(pushed, codename) {
		t.Errorf("recovery path leaked stripped field into sessions ref:\n%s", pushed)
	}
	if !strings.Contains(pushed, "[LOCAL_ONLY:prompt.text]") {
		t.Errorf("recovery path missing strip marker:\n%s", pushed)
	}

	local := gitShow(t, dir, "refs/etch/local/"+orphanedID+":session.json")
	if !strings.Contains(local, codename) {
		t.Errorf("recovery path lost full fidelity in local ref:\n%s", local)
	}
}

// THE transport gate: a session with sensitive data in a configured field,
// pushed with a bare `git push` over the setup-refspec config, arrives
// stripped on the remote and in a fresh clone; the full record never leaves
// the original repo.
func TestE2ELocalOnlyTransport(t *testing.T) {
	dir := testutil.NewTestRepo(t)
	commitInitial(t, dir)

	const codename = "project-codename-offwire-titan"
	writeLocalOnlySettings(t, dir, []string{"prompt.text"})

	remoteDir := t.TempDir()
	run(t, remoteDir, "git", "init", "--bare")
	run(t, dir, "git", "remote", "add", "origin", remoteDir)

	r := testutil.RunBinary(t, dir, []string{"setup-refspec"}, "")
	if r.ExitCode != 0 {
		t.Fatalf("setup-refspec failed: %s", r.Stderr)
	}

	// Capture a session whose prompt carries the sensitive value.
	entireSessionID := "e2e-localonly-transport-001"
	startInput := `{"session_id":"` + entireSessionID + `","raw_data":{"model":"claude-opus-4-7"}}`
	r = testutil.RunBinary(t, dir, []string{"session_start"}, startInput)
	assertOK(t, r, "session_start")

	wipFiles := findWipFiles(t, dir)
	sessionULID := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")

	promptInput := `{"session_id":"` + entireSessionID + `","user_prompt":"start ` + codename + ` now"}`
	r = testutil.RunBinary(t, dir, []string{"user_prompt_submit"}, promptInput)
	assertOK(t, r, "user_prompt_submit")

	endInput := `{"session_id":"` + entireSessionID + `"}`
	r = testutil.RunBinary(t, dir, []string{"session_end"}, endInput)
	assertOK(t, r, "session_end")

	// The core promise: a bare `git push` must not leak the stripped field.
	run(t, dir, "git", "push")

	// Remote: stripped record under sessions/, no local namespace at all.
	remoteJSON := gitShow(t, remoteDir, "refs/etch/sessions/"+sessionULID+":session.json")
	if strings.Contains(remoteJSON, codename) {
		t.Fatalf("LEAK: stripped field reached the remote:\n%s", remoteJSON)
	}
	if !strings.Contains(remoteJSON, "[LOCAL_ONLY:prompt.text]") {
		t.Errorf("remote record missing strip marker:\n%s", remoteJSON)
	}
	if got := listRefs(t, remoteDir, "refs/etch/local/"); len(got) != 0 {
		t.Errorf("refs/etch/local must never reach the remote, got %v", got)
	}

	// Fresh clone (second machine): fetches the stripped record only.
	cloneDir := filepath.Join(t.TempDir(), "clone")
	run(t, filepath.Dir(cloneDir), "git", "clone", remoteDir, cloneDir)
	r = testutil.RunBinary(t, cloneDir, []string{"setup-refspec"}, "")
	if r.ExitCode != 0 {
		t.Fatalf("setup-refspec in clone failed: %s", r.Stderr)
	}
	run(t, cloneDir, "git", "fetch", "origin")

	cloneJSON := gitShow(t, cloneDir, "refs/etch/sessions/"+sessionULID+":session.json")
	if strings.Contains(cloneJSON, codename) {
		t.Fatalf("LEAK: stripped field reached a clone:\n%s", cloneJSON)
	}
	if !strings.Contains(cloneJSON, "[LOCAL_ONLY:prompt.text]") {
		t.Errorf("cloned record missing strip marker:\n%s", cloneJSON)
	}
	if got := listRefs(t, cloneDir, "refs/etch/local/"); len(got) != 0 {
		t.Errorf("clone must have no local refs, got %v", got)
	}

	// Original repo: full fidelity intact, exactly where it was written.
	localJSON := gitShow(t, dir, "refs/etch/local/"+sessionULID+":session.json")
	if !strings.Contains(localJSON, codename) {
		t.Errorf("original repo lost the full-fidelity record:\n%s", localJSON)
	}
}
