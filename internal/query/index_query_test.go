package query_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Stage-11-Agentics/etch/internal/index"
	"github.com/Stage-11-Agentics/etch/internal/query"
	"github.com/Stage-11-Agentics/etch/internal/schema"
	"github.com/Stage-11-Agentics/etch/internal/testutil"
)

// runStats executes query.RunToWithStats and returns stdout + the stats.
func runStats(t *testing.T, repo string, args ...string) (string, query.QueryStats) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	full := append([]string{"--repo", repo}, args...)
	st, err := query.RunToWithStats(full, &stdout, &stderr)
	if err != nil {
		t.Fatalf("RunToWithStats(%v): %v\nstderr: %s", args, err, stderr.String())
	}
	return stdout.String(), st
}

func TestIndex_QueryUsesIndex(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	testutil.WriteSession(t, repo, session("01QQQQQQQQQQQQQQQQQQQQQQQ1", "claude-code", "ETCH-1", "complete", "2026-05-20T12:00:00Z"))
	testutil.WriteSession(t, repo, session("01QQQQQQQQQQQQQQQQQQQQQQQ2", "codex", "ETCH-1", "complete", "2026-05-21T12:00:00Z"))
	if _, err := index.Build(repo); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Fast path: count by indexed scalar fields → no git show calls.
	out, st := runStats(t, repo, "--runtime", "claude-code", "--count")
	if st.Source != "index" {
		t.Fatalf("expected index source, got %q", st.Source)
	}
	if st.RefShows != 0 {
		t.Fatalf("fast path should do 0 git shows, did %d", st.RefShows)
	}
	if got := strings.TrimSpace(out); got != "1" {
		t.Fatalf("expected count 1, got %q", got)
	}
}

func TestIndex_QueryNoIndexFallback(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	testutil.WriteSession(t, repo, session("01RRRRRRRRRRRRRRRRRRRRRRR1", "claude-code", "ETCH-1", "complete", "2026-05-20T12:00:00Z"))
	// No index built.
	out, st := runStats(t, repo, "--count")
	if st.Source != "refs" {
		t.Fatalf("expected refs source when no index, got %q", st.Source)
	}
	if got := strings.TrimSpace(out); got != "1" {
		t.Fatalf("expected count 1, got %q", got)
	}
}

func TestIndex_NoIndexFlag(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	testutil.WriteSession(t, repo, session("01SSSSSSSSSSSSSSSSSSSSSSS1", "claude-code", "ETCH-1", "complete", "2026-05-20T12:00:00Z"))
	if _, err := index.Build(repo); err != nil {
		t.Fatalf("Build: %v", err)
	}
	// --no-index forces ref-walk even though an index exists.
	out, st := runStats(t, repo, "--count", "--no-index")
	if st.Source != "refs" {
		t.Fatalf("--no-index should force refs source, got %q", st.Source)
	}
	if got := strings.TrimSpace(out); got != "1" {
		t.Fatalf("expected count 1, got %q", got)
	}
}

func TestIndex_StaleHandling(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	testutil.WriteSession(t, repo, session("01TTTTTTTTTTTTTTTTTTTTTTT1", "claude-code", "ETCH-1", "complete", "2026-05-20T12:00:00Z"))
	testutil.WriteSession(t, repo, session("01TTTTTTTTTTTTTTTTTTTTTTT2", "claude-code", "ETCH-1", "complete", "2026-05-21T12:00:00Z"))
	if _, err := index.Build(repo); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Delete one ref out from under the index.
	testutil.RunCmd(t, repo, "git", "update-ref", "-d", "refs/etch/sessions/01TTTTTTTTTTTTTTTTTTTTTTT2")

	// Index still lists 2 entries, but the query must not include the deleted ref.
	_, entries, err := index.Load(repo)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("index should still hold 2 stale entries, got %d", len(entries))
	}

	out, st := runStats(t, repo, "--count")
	if st.Source != "index" {
		t.Fatalf("expected index source, got %q", st.Source)
	}
	if got := strings.TrimSpace(out); got != "1" {
		t.Fatalf("stale ref should be excluded: expected count 1, got %q", got)
	}
}

func TestIndex_BranchParity_StartVsEnd(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	// A session that started on one branch and ended on another. The ref-walk
	// path matches --branch against either; the fast index path must too.
	s := session("01VVVVVVVVVVVVVVVVVVVVVVV1", "claude-code", "ETCH-1", "complete", "2026-05-20T12:00:00Z")
	s.GitStart = &schema.GitState{Branch: "feat/start"}
	s.GitEnd = &schema.GitState{Branch: "feat/end"}
	testutil.WriteSession(t, repo, s)
	if _, err := index.Build(repo); err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, br := range []string{"feat/start", "feat/end"} {
		out, st := runStats(t, repo, "--branch", br, "--count")
		if st.Source != "index" {
			t.Fatalf("--branch %s: expected index source, got %q", br, st.Source)
		}
		if st.RefShows != 0 {
			t.Fatalf("--branch %s: expected fast path (0 shows), got %d", br, st.RefShows)
		}
		if got := strings.TrimSpace(out); got != "1" {
			t.Fatalf("--branch %s on fast index path: expected 1, got %q", br, got)
		}
	}
}

func TestIndex_QueryJSONMatchesRefWalk(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	s := session("01UUUUUUUUUUUUUUUUUUUUUUU1", "claude-code", "ETCH-9", "complete", "2026-05-20T12:00:00Z")
	s.FilesTouched = []schema.FileEntry{{Path: "internal/index/index.go", Action: "modified"}}
	testutil.WriteSession(t, repo, s)
	if _, err := index.Build(repo); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// --json forces the full-record path; output must equal the ref-walk output.
	idxOut, st := runStats(t, repo, "--json")
	if st.Source != "index" {
		t.Fatalf("expected index source, got %q", st.Source)
	}
	if st.RefShows != 1 {
		t.Fatalf("--json should load the full record (1 git show), got %d", st.RefShows)
	}
	refOut, _ := runStats(t, repo, "--json", "--no-index")
	if idxOut != refOut {
		t.Fatalf("index --json differs from ref-walk --json:\nindex=%s\nrefs=%s", idxOut, refOut)
	}

	// has-files also forces the full path and must match.
	var sessions []schema.Session
	hasOut, st := runStats(t, repo, "--has-files", "*.go", "--json")
	if st.Source != "index" || st.RefShows != 1 {
		t.Fatalf("has-files should use index + load full record, got source=%q shows=%d", st.Source, st.RefShows)
	}
	if err := json.Unmarshal([]byte(hasOut), &sessions); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 match for *.go, got %d", len(sessions))
	}
}
