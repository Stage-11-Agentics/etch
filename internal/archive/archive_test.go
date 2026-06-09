package archive_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Stage-11-Agentics/etch/internal/archive"
	"github.com/Stage-11-Agentics/etch/internal/refs"
	"github.com/Stage-11-Agentics/etch/internal/testutil"
)

// fixedNow is the reference "today" used across tests (UTC).
var fixedNow = time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

// makeULID returns a 26-char ULID-shaped string unique per index.
func makeULID(i int) string {
	return fmt.Sprintf("01JWB8K3XQPNR7TV0ZYM4G%04d", i)
}

// writeSession creates a session ref aged to commitTime.
func writeSession(t *testing.T, repo, ulid string, commitTime time.Time) {
	t.Helper()
	sessionJSON := []byte(fmt.Sprintf(`{"schema_version":"etch.session.v1","session_id":%q,"status":"complete","agent":{"runtime":"claude-code","model":"claude-opus-4-7"}}`, ulid))
	traceJSON := []byte(fmt.Sprintf(`{"version":"1.0","traces":[{"agent_id":"claude-code","session_id":%q}]}`, ulid))
	meta := refs.RefMeta{
		Runtime: "claude-code", Model: "claude-opus-4-7", Status: "complete",
		Branch: "feat/x", CommitCount: 1, DurationSecs: 100, EndTime: commitTime,
	}
	if err := refs.WriteSessionRef(repo, ulid, sessionJSON, traceJSON, meta); err != nil {
		t.Fatalf("WriteSessionRef(%s): %v", ulid, err)
	}
}

func refExists(t *testing.T, repo, ref string) bool {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", ref)
	cmd.Dir = repo
	return cmd.Run() == nil
}

func listSessionRefs(t *testing.T, repo string) []string {
	t.Helper()
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname)", "refs/etch/sessions/")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("for-each-ref: %v", err)
	}
	var refs []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(l) != "" {
			refs = append(refs, l)
		}
	}
	return refs
}

func gitShow(t *testing.T, repo, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "show", ref)
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git show %s: %v", ref, err)
	}
	return string(out)
}

func daysAgo(n int) time.Time {
	return fixedNow.AddDate(0, 0, -n)
}

// daysAgoReal computes relative to the real wall clock, for tests that invoke
// the built binary (which uses time.Now(), not fixedNow). Mixing fixedNow into
// those tests makes them time bombs: the gap between fixedNow and the real
// clock grows daily until fabricated "recent" refs cross the binary's cutoff.
func daysAgoReal(n int) time.Time {
	return time.Now().UTC().AddDate(0, 0, -n)
}

// TestArchive_ConcurrentRepointAbortsQuarterAtomically (ETCH-40 below-cut):
// a session ref repointed between plan and apply must abort the WHOLE
// quarter — archive ref unadvanced, no session refs deleted — and a re-run
// against fresh state succeeds cleanly.
func TestArchive_ConcurrentRepointAbortsQuarterAtomically(t *testing.T) {
	repo := testutil.NewTestRepo(t)

	for i := 0; i < 3; i++ {
		writeSession(t, repo, makeULID(i), daysAgo(60))
	}

	// Deterministic interleave: take the plan, repoint one planned ref (a
	// concurrent upgrade/re-commit), then apply the now-STALE plan through
	// the package's own transaction.
	opts := archive.Options{RepoRoot: repo, ThresholdDays: 30, Now: fixedNow}
	plan, err := archive.BuildPlan(opts)
	if err != nil {
		t.Fatal(err)
	}
	if plan.SessionCount() != 3 {
		t.Fatalf("expected 3 planned sessions, got %d", plan.SessionCount())
	}

	// Repoint the second session's ref to a NEW commit (e.g. a concurrent
	// incomplete→complete upgrade) after the plan was taken.
	victim := plan.Quarters[0].Sessions[1]
	cmd := exec.Command("git", "commit-tree", victim.CommitSHA+"^{tree}", "-m", "repointed")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	newSHA, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	repoint := exec.Command("git", "update-ref", victim.Ref, strings.TrimSpace(string(newSHA)))
	repoint.Dir = repo
	if err := repoint.Run(); err != nil {
		t.Fatal(err)
	}

	// Apply the STALE plan through the package's own transaction.
	if err := archive.ApplyQuarterForTest(opts, plan.Quarters[0]); err == nil {
		t.Fatal("stale plan must fail the transaction, got nil error")
	}

	// Atomicity: the archive ref must NOT exist, and ALL session refs must
	// still be present (no half-applied quarter).
	label := plan.Quarters[0].Label
	if refExists(t, repo, "refs/etch/archive/"+label) {
		t.Error("archive ref advanced despite aborted transaction")
	}
	if got := len(listSessionRefs(t, repo)); got != 3 {
		t.Errorf("expected all 3 session refs to survive the abort, got %d", got)
	}

	// A fresh run sees current state and succeeds. The repointed session's
	// new commit is young (committed "now"), so it correctly falls outside
	// the age threshold and stays live — the other two archive.
	applied, err := archive.Archive(opts)
	if err != nil {
		t.Fatalf("fresh archive run after abort: %v", err)
	}
	if applied.SessionCount() != 2 {
		t.Errorf("expected 2 sessions archived on re-run (the repointed one is young again), got %d", applied.SessionCount())
	}
	remaining := listSessionRefs(t, repo)
	if len(remaining) != 1 || remaining[0] != victim.Ref {
		t.Errorf("expected only the repointed ref to remain, got %v", remaining)
	}
}

func TestArchive_OldRefsArchived(t *testing.T) {
	repo := testutil.NewTestRepo(t)

	// 5 old (60 days), 5 recent (5 days).
	for i := 0; i < 5; i++ {
		writeSession(t, repo, makeULID(i), daysAgo(60))
	}
	for i := 5; i < 10; i++ {
		writeSession(t, repo, makeULID(i), daysAgo(5))
	}

	plan, err := archive.Archive(archive.Options{RepoRoot: repo, ThresholdDays: 30, Now: fixedNow})
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if got := plan.SessionCount(); got != 5 {
		t.Fatalf("expected 5 archived, got %d", got)
	}

	// Old refs deleted, recent refs remain.
	remaining := listSessionRefs(t, repo)
	if len(remaining) != 5 {
		t.Fatalf("expected 5 remaining session refs, got %d: %v", len(remaining), remaining)
	}
	for i := 0; i < 5; i++ {
		if refExists(t, repo, "refs/etch/sessions/"+makeULID(i)) {
			t.Errorf("old ref %s should be deleted", makeULID(i))
		}
	}
	for i := 5; i < 10; i++ {
		if !refExists(t, repo, "refs/etch/sessions/"+makeULID(i)) {
			t.Errorf("recent ref %s should still exist", makeULID(i))
		}
	}
}

func TestArchive_GroupedByQuarter(t *testing.T) {
	repo := testutil.NewTestRepo(t)

	// Three distinct quarters, all well older than threshold relative to fixedNow.
	writeSession(t, repo, makeULID(0), time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)) // Q1
	writeSession(t, repo, makeULID(1), time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)) // Q2
	writeSession(t, repo, makeULID(2), time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC)) // Q3

	plan, err := archive.Archive(archive.Options{RepoRoot: repo, ThresholdDays: 30, Now: fixedNow})
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if len(plan.Quarters) != 3 {
		t.Fatalf("expected 3 quarters, got %d", len(plan.Quarters))
	}
	for _, label := range []string{"2025-Q1", "2025-Q2", "2025-Q3"} {
		if !refExists(t, repo, "refs/etch/archive/"+label) {
			t.Errorf("expected archive ref %s", label)
		}
	}
}

func TestArchive_NothingToArchive(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	for i := 0; i < 3; i++ {
		writeSession(t, repo, makeULID(i), daysAgo(5))
	}
	plan, err := archive.Archive(archive.Options{RepoRoot: repo, ThresholdDays: 30, Now: fixedNow})
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if !plan.Empty() {
		t.Fatalf("expected empty plan, got %d sessions", plan.SessionCount())
	}
	// No archive refs created.
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname)", "refs/etch/archive/")
	cmd.Dir = repo
	out, _ := cmd.Output()
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("expected no archive refs, got: %s", out)
	}
	if len(listSessionRefs(t, repo)) != 3 {
		t.Fatalf("session refs should be untouched")
	}
}

func TestArchive_DryRun(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	for i := 0; i < 4; i++ {
		writeSession(t, repo, makeULID(i), daysAgo(100))
	}
	plan, err := archive.BuildPlan(archive.Options{RepoRoot: repo, ThresholdDays: 30, Now: fixedNow, DryRun: true})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.SessionCount() != 4 {
		t.Fatalf("expected plan of 4, got %d", plan.SessionCount())
	}
	// Nothing modified.
	if len(listSessionRefs(t, repo)) != 4 {
		t.Errorf("dry-run should not delete session refs")
	}
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname)", "refs/etch/archive/")
	cmd.Dir = repo
	out, _ := cmd.Output()
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("dry-run should not create archive refs, got: %s", out)
	}
}

func TestArchive_IncrementalArchive(t *testing.T) {
	repo := testutil.NewTestRepo(t)

	// First batch: 2 sessions in 2025-Q1.
	writeSession(t, repo, makeULID(0), time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC))
	writeSession(t, repo, makeULID(1), time.Date(2025, 2, 10, 0, 0, 0, 0, time.UTC))
	if _, err := archive.Archive(archive.Options{RepoRoot: repo, ThresholdDays: 30, Now: fixedNow}); err != nil {
		t.Fatalf("first Archive: %v", err)
	}

	// Second batch: 2 more sessions in same quarter.
	writeSession(t, repo, makeULID(2), time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC))
	writeSession(t, repo, makeULID(3), time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC))
	if _, err := archive.Archive(archive.Options{RepoRoot: repo, ThresholdDays: 30, Now: fixedNow}); err != nil {
		t.Fatalf("second Archive: %v", err)
	}

	// Archive ref should now contain all 4 ULIDs.
	cmd := exec.Command("git", "ls-tree", "refs/etch/archive/2025-Q1")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ls-tree: %v", err)
	}
	for i := 0; i < 4; i++ {
		if !strings.Contains(string(out), makeULID(i)) {
			t.Errorf("archive missing %s after incremental archival:\n%s", makeULID(i), out)
		}
	}
}

func TestArchive_ContentPreserved(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	ulid := makeULID(0)
	writeSession(t, repo, ulid, time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC))

	wantSession := strings.TrimSpace(gitShow(t, repo, "refs/etch/sessions/"+ulid+":session.json"))
	wantTrace := strings.TrimSpace(gitShow(t, repo, "refs/etch/sessions/"+ulid+":agent-trace.json"))

	if _, err := archive.Archive(archive.Options{RepoRoot: repo, ThresholdDays: 30, Now: fixedNow}); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	gotSession := strings.TrimSpace(gitShow(t, repo, "refs/etch/archive/2025-Q1:"+ulid+"/session.json"))
	if gotSession != wantSession {
		t.Errorf("session.json mismatch:\ngot:  %s\nwant: %s", gotSession, wantSession)
	}
	gotTrace := strings.TrimSpace(gitShow(t, repo, "refs/etch/archive/2025-Q1:"+ulid+"/agent-trace.json"))
	if gotTrace != wantTrace {
		t.Errorf("agent-trace.json mismatch:\ngot:  %s\nwant: %s", gotTrace, wantTrace)
	}
}

func TestArchive_RestoreRoundTrip(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	ulid := makeULID(0)
	writeSession(t, repo, ulid, time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC))

	wantSession := strings.TrimSpace(gitShow(t, repo, "refs/etch/sessions/"+ulid+":session.json"))
	wantTrace := strings.TrimSpace(gitShow(t, repo, "refs/etch/sessions/"+ulid+":agent-trace.json"))

	if _, err := archive.Archive(archive.Options{RepoRoot: repo, ThresholdDays: 30, Now: fixedNow}); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if refExists(t, repo, "refs/etch/sessions/"+ulid) {
		t.Fatalf("session ref should be gone after archive")
	}

	if err := archive.Restore(repo, ulid, fixedNow); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if !refExists(t, repo, "refs/etch/sessions/"+ulid) {
		t.Fatalf("session ref should be recreated")
	}
	gotSession := strings.TrimSpace(gitShow(t, repo, "refs/etch/sessions/"+ulid+":session.json"))
	if gotSession != wantSession {
		t.Errorf("restored session.json mismatch:\ngot:  %s\nwant: %s", gotSession, wantSession)
	}
	gotTrace := strings.TrimSpace(gitShow(t, repo, "refs/etch/sessions/"+ulid+":agent-trace.json"))
	if gotTrace != wantTrace {
		t.Errorf("restored agent-trace.json mismatch:\ngot:  %s\nwant: %s", gotTrace, wantTrace)
	}
}

func TestArchive_RestoreFromMultipleQuarters(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	q1 := makeULID(0)
	q3 := makeULID(1)
	writeSession(t, repo, q1, time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC))
	writeSession(t, repo, q3, time.Date(2025, 9, 5, 0, 0, 0, 0, time.UTC))

	if _, err := archive.Archive(archive.Options{RepoRoot: repo, ThresholdDays: 30, Now: fixedNow}); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	for _, ulid := range []string{q1, q3} {
		if err := archive.Restore(repo, ulid, fixedNow); err != nil {
			t.Fatalf("Restore(%s): %v", ulid, err)
		}
		if !refExists(t, repo, "refs/etch/sessions/"+ulid) {
			t.Errorf("ref %s not restored", ulid)
		}
	}

	// Restoring an unknown ULID must error.
	if err := archive.Restore(repo, makeULID(99), fixedNow); err == nil {
		t.Error("expected error restoring unknown ULID")
	}
}

func TestArchive_ConfigThreshold(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	writeConfig(t, repo, `{"archive_threshold_days":45}`)

	// One ref at 50 days (older than 45 → archived), one at 40 days (kept).
	// daysAgoReal: this test runs the built binary, which uses the real clock.
	writeSession(t, repo, makeULID(0), daysAgoReal(50))
	writeSession(t, repo, makeULID(1), daysAgoReal(40))

	res := testutil.RunBinary(t, repo, []string{"archive"}, "")
	if res.ExitCode != 0 {
		t.Fatalf("archive exit %d: %s", res.ExitCode, res.Stderr)
	}
	if refExists(t, repo, "refs/etch/sessions/"+makeULID(0)) {
		t.Errorf("50-day ref should be archived under 45-day config")
	}
	if !refExists(t, repo, "refs/etch/sessions/"+makeULID(1)) {
		t.Errorf("40-day ref should be kept under 45-day config")
	}
}

func TestArchive_FlagOverridesConfig(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	writeConfig(t, repo, `{"archive_threshold_days":45}`)

	// Ref at 50 days: archived under config(45) but kept under flag(60).
	// daysAgoReal: this test runs the built binary, which uses the real clock.
	writeSession(t, repo, makeULID(0), daysAgoReal(50))

	res := testutil.RunBinary(t, repo, []string{"archive", "--threshold-days", "60"}, "")
	if res.ExitCode != 0 {
		t.Fatalf("archive exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !refExists(t, repo, "refs/etch/sessions/"+makeULID(0)) {
		t.Errorf("--threshold-days 60 should override config 45 and keep the 50-day ref")
	}
}

func writeConfig(t *testing.T, repo, contents string) {
	t.Helper()
	dir := filepath.Join(repo, ".etch")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
