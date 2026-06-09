package commands_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Stage-11-Agentics/etch/internal/testutil"
)

const (
	etchPush   = "refs/etch/sessions/*:refs/etch/sessions/*"
	etchFetch  = "+refs/etch/sessions/*:refs/etch/sessions/*"
	legacyForm = "refs/etch/sessions/*:refs/etch/sessions/*"
)

// configAll returns all values of a git config key in dir ("" entries dropped).
func configAll(t *testing.T, dir, key string) []string {
	t.Helper()
	cmd := exec.Command("git", "config", "--get-all", key)
	cmd.Dir = dir
	out, _ := cmd.Output() // exit 1 = no entries
	var values []string
	for _, line := range strings.Split(string(out), "\n") {
		if v := strings.TrimSpace(line); v != "" {
			values = append(values, v)
		}
	}
	return values
}

// newBareRemote creates a bare repo next to the test repo and wires it as a
// remote of dir under the given name.
func newBareRemote(t *testing.T, dir, name string) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), name+".git")
	testutil.RunCmd(t, filepath.Dir(bare), "git", "init", "--bare", bare)
	testutil.RunCmd(t, dir, "git", "remote", "add", name, bare)
	return bare
}

func commitFile(t *testing.T, dir string) {
	t.Helper()
	testutil.RunCmd(t, dir, "git", "commit", "--allow-empty", "-m", "test commit")
}

func lsRemote(t *testing.T, dir, remote string) string {
	t.Helper()
	cmd := exec.Command("git", "ls-remote", remote)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git ls-remote %s: %v\n%s", remote, err, out)
	}
	return string(out)
}

// --- Remote selection / validation (ETCH-18, ETCH-38) ---

func TestSetupRefspecNoRemoteFails(t *testing.T) {
	repo := testutil.NewTestRepo(t)

	res := testutil.RunBinary(t, repo, []string{"setup-refspec"}, "")
	if res.ExitCode == 0 {
		t.Fatalf("expected non-zero exit with no remote, got 0\nstdout: %s", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "no usable git remote") {
		t.Errorf("expected 'no usable git remote' in stderr, got: %s", res.Stderr)
	}
	if got := configAll(t, repo, "remote.origin.fetch"); len(got) != 0 {
		t.Errorf("expected no config writes, found remote.origin.fetch = %v", got)
	}
	if got := configAll(t, repo, "remote.origin.push"); len(got) != 0 {
		t.Errorf("expected no config writes, found remote.origin.push = %v", got)
	}
}

func TestSetupRefspecPhantomOriginFails(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	// Reproduce the ETCH-18 phantom: an origin section in config with refspec
	// entries but no URL ('git remote -v' shows a bare origin).
	testutil.RunCmd(t, repo, "git", "config", "--add", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")

	res := testutil.RunBinary(t, repo, []string{"setup-refspec"}, "")
	if res.ExitCode == 0 {
		t.Fatalf("expected non-zero exit against phantom origin, got 0\nstdout: %s", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "origin") || !strings.Contains(res.Stderr, "no URL") {
		t.Errorf("expected error naming phantom origin with no URL, got: %s", res.Stderr)
	}
	if got := configAll(t, repo, "remote.origin.push"); len(got) != 0 {
		t.Errorf("expected no push config writes against phantom origin, found %v", got)
	}
}

func TestSetupRefspecNormalOrigin(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	newBareRemote(t, repo, "origin")

	res := testutil.RunBinary(t, repo, []string{"setup-refspec"}, "")
	if res.ExitCode != 0 {
		t.Fatalf("expected success, got exit %d\nstderr: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, `remote "origin"`) {
		t.Errorf("expected output to name the remote, got: %s", res.Stdout)
	}

	fetch := configAll(t, repo, "remote.origin.fetch")
	wantFetch := false
	for _, v := range fetch {
		if v == etchFetch {
			wantFetch = true
		}
		if v == legacyForm {
			t.Errorf("legacy no-'+' fetch refspec present: %v", fetch)
		}
	}
	if !wantFetch {
		t.Errorf("fetch refspec %q missing, got %v", etchFetch, fetch)
	}

	push := configAll(t, repo, "remote.origin.push")
	if len(push) != 2 || push[0] != etchPush || push[1] != "HEAD" {
		t.Errorf("expected push entries [%q, HEAD], got %v", etchPush, push)
	}
	if !strings.Contains(res.Stdout, "push.default=current") {
		t.Errorf("expected push-semantics notice, got: %s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "git fetch origin") {
		t.Errorf("expected fetch hint, got: %s", res.Stdout)
	}
}

func TestSetupRefspecIdempotent(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	newBareRemote(t, repo, "origin")

	first := testutil.RunBinary(t, repo, []string{"setup-refspec"}, "")
	if first.ExitCode != 0 {
		t.Fatalf("first run failed: %s", first.Stderr)
	}
	second := testutil.RunBinary(t, repo, []string{"setup-refspec"}, "")
	if second.ExitCode != 0 {
		t.Fatalf("second run failed: %s", second.Stderr)
	}

	push := configAll(t, repo, "remote.origin.push")
	if len(push) != 2 {
		t.Errorf("rerun duplicated push entries: %v", push)
	}
	fetch := 0
	for _, v := range configAll(t, repo, "remote.origin.fetch") {
		if v == etchFetch {
			fetch++
		}
	}
	if fetch != 1 {
		t.Errorf("rerun duplicated fetch entries (count %d)", fetch)
	}
	// Rerun of an etch-only config must NOT claim user refspecs exist.
	if strings.Contains(second.Stdout, "remain authoritative") {
		t.Errorf("spurious 'remain authoritative' notice on rerun: %s", second.Stdout)
	}
}

func TestSetupRefspecSingleNonOriginRemote(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	newBareRemote(t, repo, "forgejo")

	res := testutil.RunBinary(t, repo, []string{"setup-refspec"}, "")
	if res.ExitCode != 0 {
		t.Fatalf("expected success with single non-origin remote, got exit %d\nstderr: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, `"forgejo"`) {
		t.Errorf("expected output to name remote forgejo, got: %s", res.Stdout)
	}
	if push := configAll(t, repo, "remote.forgejo.push"); len(push) != 2 || push[0] != etchPush {
		t.Errorf("expected etch+HEAD push refspecs on forgejo, got %v", push)
	}
	if origin := configAll(t, repo, "remote.origin.push"); len(origin) != 0 {
		t.Errorf("origin must not be configured, got %v", origin)
	}
}

func TestSetupRefspecMultipleRemotesNoOriginFails(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	newBareRemote(t, repo, "forgejo")
	newBareRemote(t, repo, "github")

	res := testutil.RunBinary(t, repo, []string{"setup-refspec"}, "")
	if res.ExitCode == 0 {
		t.Fatalf("expected non-zero exit with multiple non-origin remotes, got 0\nstdout: %s", res.Stdout)
	}
	for _, want := range []string{"forgejo", "github", "--remote"} {
		if !strings.Contains(res.Stderr, want) {
			t.Errorf("expected %q in error, got: %s", want, res.Stderr)
		}
	}
}

func TestSetupRefspecRemoteFlag(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	newBareRemote(t, repo, "forgejo")
	newBareRemote(t, repo, "github")

	// Per-remote reruns: the documented dual-remote path (SPEC criterion 5).
	for _, args := range [][]string{
		{"setup-refspec", "--remote", "forgejo"},
		{"setup-refspec", "--remote=github"},
	} {
		res := testutil.RunBinary(t, repo, args, "")
		if res.ExitCode != 0 {
			t.Fatalf("%v failed: %s", args, res.Stderr)
		}
	}
	for _, remote := range []string{"forgejo", "github"} {
		if push := configAll(t, repo, "remote."+remote+".push"); len(push) != 2 || push[0] != etchPush {
			t.Errorf("remote %s: expected etch+HEAD push refspecs, got %v", remote, push)
		}
		if fetch := configAll(t, repo, "remote."+remote+".fetch"); !contains(fetch, etchFetch) {
			t.Errorf("remote %s: missing etch fetch refspec, got %v", remote, fetch)
		}
	}
}

func TestSetupRefspecRemoteFlagErrors(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	newBareRemote(t, repo, "origin")

	res := testutil.RunBinary(t, repo, []string{"setup-refspec", "--remote", "bogus"}, "")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit for --remote bogus")
	}
	if !strings.Contains(res.Stderr, `"bogus" not found`) {
		t.Errorf("expected not-found error, got: %s", res.Stderr)
	}

	res = testutil.RunBinary(t, repo, []string{"setup-refspec", "--frobnicate"}, "")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit for unknown flag")
	}
	if !strings.Contains(res.Stderr, "unknown argument") {
		t.Errorf("expected unknown-argument error, got: %s", res.Stderr)
	}

	res = testutil.RunBinary(t, repo, []string{"setup-refspec", "--remote"}, "")
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit for --remote without value")
	}
}

// --- Fetch refspec '+' and legacy upgrade (ETCH-38 / ETCH-22) ---

func TestSetupRefspecUpgradesLegacyFetchEntry(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	newBareRemote(t, repo, "origin")
	// Pre-seed the pre-ETCH-38 form written by older binaries.
	testutil.RunCmd(t, repo, "git", "config", "--add", "remote.origin.fetch", legacyForm)

	res := testutil.RunBinary(t, repo, []string{"setup-refspec"}, "")
	if res.ExitCode != 0 {
		t.Fatalf("expected success, got exit %d\nstderr: %s", res.ExitCode, res.Stderr)
	}

	var etchEntries []string
	for _, v := range configAll(t, repo, "remote.origin.fetch") {
		if strings.Contains(v, "refs/etch/sessions") {
			etchEntries = append(etchEntries, v)
		}
	}
	if len(etchEntries) != 1 || etchEntries[0] != etchFetch {
		t.Errorf("expected single upgraded fetch entry %q, got %v", etchFetch, etchEntries)
	}
}

// --- Pre-existing user push refspecs (ETCH-16 conditional rule) ---

func TestSetupRefspecPreservesUserPushRefspecs(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	newBareRemote(t, repo, "origin")
	userSpec := "refs/heads/main:refs/heads/main"
	testutil.RunCmd(t, repo, "git", "config", "--add", "remote.origin.push", userSpec)

	res := testutil.RunBinary(t, repo, []string{"setup-refspec"}, "")
	if res.ExitCode != 0 {
		t.Fatalf("expected success, got exit %d\nstderr: %s", res.ExitCode, res.Stderr)
	}

	push := configAll(t, repo, "remote.origin.push")
	if !contains(push, userSpec) || !contains(push, etchPush) {
		t.Errorf("expected user + etch push refspecs preserved, got %v", push)
	}
	if contains(push, "HEAD") {
		t.Errorf("HEAD must not be injected into a hand-tuned push config, got %v", push)
	}
	if !strings.Contains(res.Stdout, "remain authoritative") {
		t.Errorf("expected 'remain authoritative' notice, got: %s", res.Stdout)
	}
}

func TestSetupRefspecHealsLegacyEtchOnlyPushConfig(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	newBareRemote(t, repo, "origin")
	commitFile(t, repo)
	testutil.RunCmd(t, repo, "git", "branch", "-M", "main")
	// The pre-fix ETCH-16 state: an older binary (or the old README manual
	// config) wrote ONLY the etch push refspec, hijacking bare 'git push'.
	testutil.RunCmd(t, repo, "git", "config", "--add", "remote.origin.push", etchPush)

	res := testutil.RunBinary(t, repo, []string{"setup-refspec"}, "")
	if res.ExitCode != 0 {
		t.Fatalf("expected success, got exit %d\nstderr: %s", res.ExitCode, res.Stderr)
	}

	push := configAll(t, repo, "remote.origin.push")
	if len(push) != 2 || push[0] != etchPush || push[1] != "HEAD" {
		t.Errorf("legacy etch-only push config not healed: expected [%q, HEAD], got %v", etchPush, push)
	}
	if !strings.Contains(res.Stdout, "push.default=current") {
		t.Errorf("expected push-semantics notice after heal, got: %s", res.Stdout)
	}

	// The healed config must actually un-hijack a plain push.
	testutil.RunCmd(t, repo, "git", "push", "origin")
	remote := lsRemote(t, repo, "origin")
	if !strings.Contains(remote, "refs/heads/main") {
		t.Errorf("plain push still hijacked after heal\nls-remote:\n%s", remote)
	}
}

// --- ETCH-16 regression: plain push carries branch AND etch refs ---

func TestSetupRefspecPlainPushCarriesBranchAndEtchRefs(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	newBareRemote(t, repo, "origin")
	commitFile(t, repo)
	testutil.RunCmd(t, repo, "git", "branch", "-M", "main")
	testutil.RunCmd(t, repo, "git", "update-ref", "refs/etch/sessions/01TESTSESSIONREF", "HEAD")

	res := testutil.RunBinary(t, repo, []string{"setup-refspec"}, "")
	if res.ExitCode != 0 {
		t.Fatalf("setup-refspec failed: %s", res.Stderr)
	}

	testutil.RunCmd(t, repo, "git", "push", "origin")

	remote := lsRemote(t, repo, "origin")
	if !strings.Contains(remote, "refs/heads/main") {
		t.Errorf("ETCH-16 regression: branch did not reach remote after plain push\nls-remote:\n%s", remote)
	}
	if !strings.Contains(remote, "refs/etch/sessions/01TESTSESSIONREF") {
		t.Errorf("etch ref did not reach remote after plain push\nls-remote:\n%s", remote)
	}
}

// --- E2e round-trip: second clone fetches etch refs (ETCH-24) ---

func TestSetupRefspecSecondCloneRoundTrip(t *testing.T) {
	repoA := testutil.NewTestRepo(t)
	bare := newBareRemote(t, repoA, "origin")
	commitFile(t, repoA)
	testutil.RunCmd(t, repoA, "git", "branch", "-M", "main")
	testutil.RunCmd(t, repoA, "git", "update-ref", "refs/etch/sessions/01ROUNDTRIPREF", "HEAD")

	// Machine 1: setup + plain push carries branch and session refs.
	res := testutil.RunBinary(t, repoA, []string{"setup-refspec"}, "")
	if res.ExitCode != 0 {
		t.Fatalf("setup-refspec in repo A failed: %s", res.Stderr)
	}
	testutil.RunCmd(t, repoA, "git", "push", "origin")

	// Machine 2: clone has zero etch refs until the documented setup + fetch.
	repoB := filepath.Join(t.TempDir(), "cloneB")
	testutil.RunCmd(t, filepath.Dir(repoB), "git", "clone", bare, repoB)
	testutil.RunCmd(t, repoB, "git", "config", "user.email", "test@test.local")
	testutil.RunCmd(t, repoB, "git", "config", "user.name", "Test")

	cmd := exec.Command("git", "for-each-ref", "refs/etch/sessions/")
	cmd.Dir = repoB
	out, _ := cmd.Output()
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("fresh clone unexpectedly has etch refs already:\n%s", out)
	}

	res = testutil.RunBinary(t, repoB, []string{"setup-refspec"}, "")
	if res.ExitCode != 0 {
		t.Fatalf("setup-refspec in clone failed: %s", res.Stderr)
	}
	testutil.RunCmd(t, repoB, "git", "fetch", "origin")

	cmd = exec.Command("git", "for-each-ref", "refs/etch/sessions/")
	cmd.Dir = repoB
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("for-each-ref in clone: %v", err)
	}
	if !strings.Contains(string(out), "refs/etch/sessions/01ROUNDTRIPREF") {
		t.Errorf("etch refs did not round-trip to second clone:\n%s", out)
	}

	// Normal push from the clone still pushes the clone's branch.
	commitFile(t, repoB)
	testutil.RunCmd(t, repoB, "git", "push", "origin")
	remote := lsRemote(t, repoB, "origin")
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoB
	head, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	if !strings.Contains(remote, strings.TrimSpace(string(head))) {
		t.Errorf("clone's branch commit did not reach remote after plain push\nls-remote:\n%s", remote)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
