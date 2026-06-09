package index_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/Stage-11-Agentics/etch/internal/index"
	"github.com/Stage-11-Agentics/etch/internal/query"
	"github.com/Stage-11-Agentics/etch/internal/schema"
	"github.com/Stage-11-Agentics/etch/internal/testutil"
)

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

// seedN writes n sessions with deterministic 26-char ULID-shaped ids.
func seedN(t testing.TB, repo string, n int) []string {
	t.Helper()
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("01TEST%020d", i)
		ids[i] = id
		testutil.WriteSession(t, repo, session(id, "claude-code", "ETCH-10", "complete",
			fmt.Sprintf("2026-05-%02dT12:00:00Z", (i%27)+1)))
	}
	return ids
}

func TestIndex_BuildFromEmpty(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	res, err := index.Build(repo)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if res.Total != 0 || res.Parsed != 0 {
		t.Fatalf("expected empty build, got %+v", res)
	}
	header, entries, err := index.Load(repo)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if header.Schema != index.SchemaVersion {
		t.Errorf("schema marker: got %q want %q", header.Schema, index.SchemaVersion)
	}
	if header.SessionCount != 0 || len(entries) != 0 {
		t.Fatalf("expected 0 entries, got header=%d entries=%d", header.SessionCount, len(entries))
	}
}

func TestIndex_BuildFromN(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	seedN(t, repo, 20)

	res, err := index.Build(repo)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if res.Total != 20 || res.Parsed != 20 {
		t.Fatalf("expected 20 parsed+total, got %+v", res)
	}
	header, entries, err := index.Load(repo)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if header.SessionCount != 20 || len(entries) != 20 {
		t.Fatalf("expected 20 entries, got header=%d entries=%d", header.SessionCount, len(entries))
	}
	for _, e := range entries {
		if e.Runtime != "claude-code" || e.Status != "complete" || e.TicketID != "ETCH-10" {
			t.Fatalf("entry not projected correctly: %+v", e)
		}
	}
}

func TestIndex_Update_Incremental(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	seedN(t, repo, 10)

	if _, err := index.Build(repo); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Add 5 more sessions, then update.
	for i := 10; i < 15; i++ {
		id := fmt.Sprintf("01TEST%020d", i)
		testutil.WriteSession(t, repo, session(id, "codex", "ETCH-11", "complete",
			fmt.Sprintf("2026-06-%02dT12:00:00Z", (i%27)+1)))
	}

	res, err := index.Update(repo)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if res.Parsed != 5 {
		t.Errorf("update should have read exactly 5 new blobs, got Parsed=%d", res.Parsed)
	}
	if res.Skipped != 10 {
		t.Errorf("update should have skipped the first 10, got Skipped=%d", res.Skipped)
	}
	if res.Total != 15 {
		t.Errorf("index should hold 15 total, got %d", res.Total)
	}

	_, entries, err := index.Load(repo)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 15 {
		t.Fatalf("expected 15 entries after update, got %d", len(entries))
	}
}

func TestIndex_Update_NoExistingIndexBuildsFresh(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	seedN(t, repo, 4)
	res, err := index.Update(repo) // no index yet → equivalent to Build
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if res.Total != 4 || res.Parsed != 4 {
		t.Fatalf("expected fresh build of 4, got %+v", res)
	}
}

func TestIndex_Show(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	seedN(t, repo, 3)
	if _, err := index.Build(repo); err != nil {
		t.Fatalf("Build: %v", err)
	}
	st, err := index.Show(repo)
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if st.Count != 3 {
		t.Errorf("count: got %d want 3", st.Count)
	}
	if st.SizeBytes <= 0 {
		t.Errorf("size should be positive, got %d", st.SizeBytes)
	}
	if st.BuiltAt == "" {
		t.Errorf("built_at should be set")
	}
	if st.Oldest == "" || st.Newest == "" {
		t.Errorf("expected oldest/newest, got %q/%q", st.Oldest, st.Newest)
	}
	if st.Oldest > st.Newest {
		t.Errorf("oldest %q should be <= newest %q", st.Oldest, st.Newest)
	}
}

func TestIndex_Drop(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	seedN(t, repo, 2)
	if _, err := index.Build(repo); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !index.Exists(repo) {
		t.Fatal("index should exist after build")
	}
	if err := index.Drop(repo); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	if index.Exists(repo) {
		t.Fatal("index should be gone after drop")
	}
	if _, err := os.Stat(index.IndexPath(repo)); !os.IsNotExist(err) {
		t.Fatalf("index file still present: %v", err)
	}
	// Dropping again is a no-op, not an error.
	if err := index.Drop(repo); err != nil {
		t.Fatalf("second Drop should be no-op: %v", err)
	}
}

func TestIndex_SchemaVersion(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	seedN(t, repo, 1)
	if _, err := index.Build(repo); err != nil {
		t.Fatalf("Build: %v", err)
	}
	header, _, err := index.Load(repo)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if header.Schema != "etch.index.v1" {
		t.Fatalf("schema marker: got %q want etch.index.v1", header.Schema)
	}
}

// BenchmarkQueryWithIndex and BenchmarkQueryWithoutIndex demonstrate the index
// speedup. Setup (seeding N refs) is outside the timed loop and only runs under
// -bench.
const benchN = 1000

func benchRepo(b *testing.B) string {
	b.Helper()
	repo := testutil.NewTestRepo(b)
	seedN(b, repo, benchN)
	return repo
}

func BenchmarkIndexBuild(b *testing.B) {
	repo := benchRepo(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := index.Build(repo); err != nil {
			b.Fatalf("Build: %v", err)
		}
	}
}

func BenchmarkQueryWithIndex(b *testing.B) {
	repo := benchRepo(b)
	if _, err := index.Build(repo); err != nil {
		b.Fatalf("Build: %v", err)
	}
	args := []string{"--repo", repo, "--status", "complete", "--runtime", "claude-code", "--count"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		st, err := query.RunToWithStats(args, &discard{}, &discard{})
		if err != nil {
			b.Fatalf("query: %v", err)
		}
		if st.Source != "index" {
			b.Fatalf("expected index path, got %q", st.Source)
		}
	}
}

func BenchmarkQueryWithoutIndex(b *testing.B) {
	repo := benchRepo(b)
	args := []string{"--repo", repo, "--status", "complete", "--runtime", "claude-code", "--count", "--no-index"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := query.RunToWithStats(args, &discard{}, &discard{}); err != nil {
			b.Fatalf("query: %v", err)
		}
	}
}

// discard is an io.Writer that drops everything.
type discard struct{}

func (*discard) Write(p []byte) (int, error) { return len(p), nil }
