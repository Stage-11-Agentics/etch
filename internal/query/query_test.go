package query_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"forgejo.stage11.ai/s11/etch/internal/query"
	"forgejo.stage11.ai/s11/etch/internal/schema"
	"forgejo.stage11.ai/s11/etch/internal/testutil"
)

// run executes query.RunTo against repo with the given args and returns
// stdout/stderr.
func run(t *testing.T, repo string, args ...string) (string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	fullArgs := append([]string{"--repo", repo}, args...)
	if err := query.RunTo(fullArgs, &stdout, &stderr); err != nil {
		t.Fatalf("query.RunTo(%v): %v\nstderr: %s", args, err, stderr.String())
	}
	return stdout.String(), stderr.String()
}

// session builds a minimal schema.Session with sane defaults.
func session(id, runtime, ticket, status, startedAt string) schema.Session {
	s := schema.Session{
		SessionID:  id,
		Status:     status,
		ExitReason: "clean",
		Agent:      schema.Agent{Runtime: runtime, Model: testutil.StrPtr("model-x")},
		Timing:     schema.Timing{StartedAt: testutil.StrPtr(startedAt)},
	}
	if ticket != "" {
		s.Orchestration = &schema.Orchestration{TicketID: testutil.StrPtr(ticket)}
	}
	return s
}

func TestQuery_AllSessions(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	for i := 0; i < 5; i++ {
		testutil.WriteSession(t, repo, session(
			fmt.Sprintf("01JWB8K3XQPNR7TV0ZYM4GD20%d", i),
			"claude-code", "ETCH-1", "complete",
			fmt.Sprintf("2026-05-2%dT12:00:00Z", i),
		))
	}
	out, _ := run(t, repo, "--count")
	if got := strings.TrimSpace(out); got != "5" {
		t.Fatalf("expected count 5, got %q", got)
	}
}

func TestQuery_FilterByTicket(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	testutil.WriteSession(t, repo, session("01AAAAAAAAAAAAAAAAAAAAAAA1", "claude-code", "ETCH-1", "complete", "2026-05-20T12:00:00Z"))
	testutil.WriteSession(t, repo, session("01AAAAAAAAAAAAAAAAAAAAAAA2", "claude-code", "ETCH-2", "complete", "2026-05-21T12:00:00Z"))
	testutil.WriteSession(t, repo, session("01AAAAAAAAAAAAAAAAAAAAAAA3", "claude-code", "ETCH-1", "complete", "2026-05-22T12:00:00Z"))

	out, _ := run(t, repo, "--ticket", "ETCH-1", "--count")
	if got := strings.TrimSpace(out); got != "2" {
		t.Fatalf("expected 2 ETCH-1 sessions, got %q", got)
	}
}

func TestQuery_FilterByRuntime(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	testutil.WriteSession(t, repo, session("01BBBBBBBBBBBBBBBBBBBBBBB1", "claude-code", "ETCH-1", "complete", "2026-05-20T12:00:00Z"))
	testutil.WriteSession(t, repo, session("01BBBBBBBBBBBBBBBBBBBBBBB2", "codex", "ETCH-1", "complete", "2026-05-21T12:00:00Z"))
	testutil.WriteSession(t, repo, session("01BBBBBBBBBBBBBBBBBBBBBBB3", "claude-code", "ETCH-1", "complete", "2026-05-22T12:00:00Z"))

	out, _ := run(t, repo, "--runtime", "claude-code", "--json")
	var sessions []schema.Session
	if err := json.Unmarshal([]byte(out), &sessions); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 claude-code sessions, got %d", len(sessions))
	}
	for _, s := range sessions {
		if s.Agent.Runtime != "claude-code" {
			t.Errorf("unexpected runtime %q", s.Agent.Runtime)
		}
	}
}

func TestQuery_FilterByStatus(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	testutil.WriteSession(t, repo, session("01CCCCCCCCCCCCCCCCCCCCCCC1", "claude-code", "ETCH-1", "complete", "2026-05-20T12:00:00Z"))
	testutil.WriteSession(t, repo, session("01CCCCCCCCCCCCCCCCCCCCCCC2", "claude-code", "ETCH-1", "incomplete", "2026-05-21T12:00:00Z"))
	testutil.WriteSession(t, repo, session("01CCCCCCCCCCCCCCCCCCCCCCC3", "claude-code", "ETCH-1", "incomplete", "2026-05-22T12:00:00Z"))

	out, _ := run(t, repo, "--status", "incomplete", "--count")
	if got := strings.TrimSpace(out); got != "2" {
		t.Fatalf("expected 2 incomplete, got %q", got)
	}
	out, _ = run(t, repo, "--status", "complete", "--count")
	if got := strings.TrimSpace(out); got != "1" {
		t.Fatalf("expected 1 complete, got %q", got)
	}
}

func TestQuery_FilterByTimeRange(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	days := []string{
		"2026-05-01T12:00:00Z",
		"2026-05-03T12:00:00Z",
		"2026-05-05T12:00:00Z",
		"2026-05-07T12:00:00Z",
	}
	for i, d := range days {
		testutil.WriteSession(t, repo, session(fmt.Sprintf("01DDDDDDDDDDDDDDDDDDDDDDD0%d", i), "claude-code", "ETCH-1", "complete", d))
	}

	// since includes 05-03, 05-05, 05-07
	out, _ := run(t, repo, "--since", "2026-05-03T00:00:00Z", "--count")
	if got := strings.TrimSpace(out); got != "3" {
		t.Fatalf("--since: expected 3, got %q", got)
	}
	// until includes 05-01, 05-03
	out, _ = run(t, repo, "--until", "2026-05-04T00:00:00Z", "--count")
	if got := strings.TrimSpace(out); got != "2" {
		t.Fatalf("--until: expected 2, got %q", got)
	}
	// window 05-03..05-05 inclusive => 05-03, 05-05
	out, _ = run(t, repo, "--since", "2026-05-03T00:00:00Z", "--until", "2026-05-05T23:59:59Z", "--count")
	if got := strings.TrimSpace(out); got != "2" {
		t.Fatalf("window: expected 2, got %q", got)
	}
}

func TestQuery_FilterByHasFiles(t *testing.T) {
	repo := testutil.NewTestRepo(t)

	s1 := session("01EEEEEEEEEEEEEEEEEEEEEEE1", "claude-code", "ETCH-1", "complete", "2026-05-20T12:00:00Z")
	s1.FilesTouched = []schema.FileEntry{{Path: "internal/query/query.go", Action: "modified"}}
	testutil.WriteSession(t, repo, s1)

	s2 := session("01EEEEEEEEEEEEEEEEEEEEEEE2", "claude-code", "ETCH-1", "complete", "2026-05-21T12:00:00Z")
	s2.FilesTouched = []schema.FileEntry{{Path: "README.md", Action: "modified"}}
	testutil.WriteSession(t, repo, s2)

	out, _ := run(t, repo, "--has-files", "*.go", "--json")
	var sessions []schema.Session
	if err := json.Unmarshal([]byte(out), &sessions); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session touching *.go, got %d", len(sessions))
	}
	if sessions[0].SessionID != "01EEEEEEEEEEEEEEEEEEEEEEE1" {
		t.Errorf("wrong session matched: %s", sessions[0].SessionID)
	}
}

func TestQuery_MultipleFilters(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	// Target: runtime=claude-code AND ticket=ETCH-2 AND status=complete
	target := session("01FFFFFFFFFFFFFFFFFFFFFFF1", "claude-code", "ETCH-2", "complete", "2026-05-20T12:00:00Z")
	testutil.WriteSession(t, repo, target)
	// Decoys differing in exactly one dimension.
	testutil.WriteSession(t, repo, session("01FFFFFFFFFFFFFFFFFFFFFFF2", "codex", "ETCH-2", "complete", "2026-05-20T12:00:00Z"))
	testutil.WriteSession(t, repo, session("01FFFFFFFFFFFFFFFFFFFFFFF3", "claude-code", "ETCH-3", "complete", "2026-05-20T12:00:00Z"))
	testutil.WriteSession(t, repo, session("01FFFFFFFFFFFFFFFFFFFFFFF4", "claude-code", "ETCH-2", "incomplete", "2026-05-20T12:00:00Z"))

	out, _ := run(t, repo, "--runtime", "claude-code", "--ticket", "ETCH-2", "--status", "complete", "--json")
	var sessions []schema.Session
	if err := json.Unmarshal([]byte(out), &sessions); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected exactly 1 match for AND filters, got %d", len(sessions))
	}
	if sessions[0].SessionID != "01FFFFFFFFFFFFFFFFFFFFFFF1" {
		t.Errorf("wrong session matched: %s", sessions[0].SessionID)
	}
}

func TestQuery_JSONOutput(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	testutil.WriteSession(t, repo, session("01GGGGGGGGGGGGGGGGGGGGGGG1", "claude-code", "ETCH-1", "complete", "2026-05-20T12:00:00Z"))
	testutil.WriteSession(t, repo, session("01GGGGGGGGGGGGGGGGGGGGGGG2", "claude-code", "ETCH-1", "complete", "2026-05-21T12:00:00Z"))

	out, _ := run(t, repo, "--json")
	var sessions []schema.Session
	if err := json.Unmarshal([]byte(out), &sessions); err != nil {
		t.Fatalf("--json not round-trippable: %v\n%s", err, out)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions in JSON, got %d", len(sessions))
	}
	for _, s := range sessions {
		if s.SchemaVersion != schema.SchemaVersion {
			t.Errorf("schema_version not preserved: %q", s.SchemaVersion)
		}
	}
}

func TestQuery_JSONOutput_EmptyIsArray(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	out, _ := run(t, repo, "--json")
	var sessions []schema.Session
	if err := json.Unmarshal([]byte(out), &sessions); err != nil {
		t.Fatalf("empty --json should be a valid array: %v\n%q", err, out)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected empty array, got %d", len(sessions))
	}
}

func TestQuery_CountOutput(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	for i := 0; i < 3; i++ {
		testutil.WriteSession(t, repo, session(fmt.Sprintf("01HHHHHHHHHHHHHHHHHHHHHHH0%d", i), "claude-code", "ETCH-1", "complete", "2026-05-20T12:00:00Z"))
	}
	out, _ := run(t, repo, "--count")
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		t.Fatalf("--count not an integer: %q", out)
	}
	if n != 3 {
		t.Fatalf("expected 3, got %d", n)
	}
}

func TestQuery_SortStartedAt(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	// Insert out of order.
	testutil.WriteSession(t, repo, session("01IIIIIIIIIIIIIIIIIIIIIII2", "claude-code", "ETCH-1", "complete", "2026-05-02T12:00:00Z"))
	testutil.WriteSession(t, repo, session("01IIIIIIIIIIIIIIIIIIIIIII3", "claude-code", "ETCH-1", "complete", "2026-05-03T12:00:00Z"))
	testutil.WriteSession(t, repo, session("01IIIIIIIIIIIIIIIIIIIIIII1", "claude-code", "ETCH-1", "complete", "2026-05-01T12:00:00Z"))

	// Default: descending (newest first).
	out, _ := run(t, repo, "--json")
	var sessions []schema.Session
	if err := json.Unmarshal([]byte(out), &sessions); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	wantDesc := []string{"2026-05-03T12:00:00Z", "2026-05-02T12:00:00Z", "2026-05-01T12:00:00Z"}
	for i, w := range wantDesc {
		if *sessions[i].Timing.StartedAt != w {
			t.Fatalf("desc order: pos %d expected %s, got %s", i, w, *sessions[i].Timing.StartedAt)
		}
	}

	// Reverse: ascending (oldest first).
	out, _ = run(t, repo, "--json", "--reverse")
	if err := json.Unmarshal([]byte(out), &sessions); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	wantAsc := []string{"2026-05-01T12:00:00Z", "2026-05-02T12:00:00Z", "2026-05-03T12:00:00Z"}
	for i, w := range wantAsc {
		if *sessions[i].Timing.StartedAt != w {
			t.Fatalf("asc order: pos %d expected %s, got %s", i, w, *sessions[i].Timing.StartedAt)
		}
	}
}

func TestQuery_NoMatches(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	testutil.WriteSession(t, repo, session("01JJJJJJJJJJJJJJJJJJJJJJJ1", "claude-code", "ETCH-1", "complete", "2026-05-20T12:00:00Z"))

	out, _ := run(t, repo, "--ticket", "DOES-NOT-EXIST", "--count")
	if got := strings.TrimSpace(out); got != "0" {
		t.Fatalf("expected 0 matches, got %q", got)
	}

	out, _ = run(t, repo, "--ticket", "DOES-NOT-EXIST", "--json")
	var sessions []schema.Session
	if err := json.Unmarshal([]byte(out), &sessions); err != nil {
		t.Fatalf("no-match --json should be valid array: %v\n%q", err, out)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected empty, got %d", len(sessions))
	}
}

func TestQuery_EmptyRepo(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	out, stderr := run(t, repo, "--count")
	if got := strings.TrimSpace(out); got != "0" {
		t.Fatalf("expected 0 for empty repo, got %q (stderr: %s)", got, stderr)
	}
}

func TestQuery_SortDuration(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	mk := func(id string, dur int64) schema.Session {
		s := session(id, "claude-code", "ETCH-1", "complete", "2026-05-20T12:00:00Z")
		s.Timing.DurationMS = testutil.Int64Ptr(dur)
		return s
	}
	testutil.WriteSession(t, repo, mk("01KKKKKKKKKKKKKKKKKKKKKKK1", 1000))
	testutil.WriteSession(t, repo, mk("01KKKKKKKKKKKKKKKKKKKKKKK2", 5000))
	testutil.WriteSession(t, repo, mk("01KKKKKKKKKKKKKKKKKKKKKKK3", 3000))

	out, _ := run(t, repo, "--sort", "duration", "--json")
	var sessions []schema.Session
	if err := json.Unmarshal([]byte(out), &sessions); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	want := []int64{5000, 3000, 1000}
	for i, w := range want {
		if *sessions[i].Timing.DurationMS != w {
			t.Fatalf("duration sort: pos %d expected %d, got %d", i, w, *sessions[i].Timing.DurationMS)
		}
	}
}

func TestQuery_TableOutput(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	testutil.WriteSession(t, repo, session("01LLLLLLLLLLLLLLLLLLLLLLL1", "claude-code", "ETCH-7", "complete", "2026-05-20T12:00:00Z"))

	out, _ := run(t, repo)
	if !strings.Contains(out, "SESSION") || !strings.Contains(out, "STATUS") {
		t.Fatalf("table missing header: %q", out)
	}
	if !strings.Contains(out, "01LLLLLL") {
		t.Fatalf("table missing short session id: %q", out)
	}
	if !strings.Contains(out, "ETCH-7") {
		t.Fatalf("table missing ticket: %q", out)
	}
}

func TestQuery_InvalidSortKey(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	var stdout, stderr bytes.Buffer
	err := query.RunTo([]string{"--repo", repo, "--sort", "bogus"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for invalid --sort key")
	}
}

func TestQuery_FilterByBranch(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	s1 := session("01MMMMMMMMMMMMMMMMMMMMMMM1", "claude-code", "ETCH-1", "complete", "2026-05-20T12:00:00Z")
	s1.GitStart = &schema.GitState{Branch: "feat/alpha"}
	testutil.WriteSession(t, repo, s1)
	s2 := session("01MMMMMMMMMMMMMMMMMMMMMMM2", "claude-code", "ETCH-1", "complete", "2026-05-21T12:00:00Z")
	s2.GitEnd = &schema.GitState{Branch: "feat/beta"}
	testutil.WriteSession(t, repo, s2)

	out, _ := run(t, repo, "--branch", "feat/beta", "--count")
	if got := strings.TrimSpace(out); got != "1" {
		t.Fatalf("expected 1 on feat/beta, got %q", got)
	}
}

func TestQuery_FilterByRunIDAndExitReason(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	s1 := session("01NNNNNNNNNNNNNNNNNNNNNNN1", "claude-code", "ETCH-1", "complete", "2026-05-20T12:00:00Z")
	s1.Orchestration.RunID = testutil.StrPtr("run-123")
	s1.ExitReason = "clean"
	testutil.WriteSession(t, repo, s1)
	s2 := session("01NNNNNNNNNNNNNNNNNNNNNNN2", "claude-code", "ETCH-1", "complete", "2026-05-21T12:00:00Z")
	s2.Orchestration.RunID = testutil.StrPtr("run-999")
	s2.ExitReason = "crash"
	testutil.WriteSession(t, repo, s2)

	out, _ := run(t, repo, "--run-id", "run-123", "--count")
	if got := strings.TrimSpace(out); got != "1" {
		t.Fatalf("--run-id: expected 1, got %q", got)
	}
	out, _ = run(t, repo, "--exit-reason", "crash", "--count")
	if got := strings.TrimSpace(out); got != "1" {
		t.Fatalf("--exit-reason: expected 1, got %q", got)
	}
}
