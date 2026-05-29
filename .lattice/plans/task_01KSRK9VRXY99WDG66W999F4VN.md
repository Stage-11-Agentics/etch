# ETCH-10: cairn index — Implementation Plan

## Goal

Materialize a queryable index from all `refs/cairn/sessions/*` so `cairn query` answers
common questions without `git show`-ing every ref. ETCH-9 walks every ref per query —
fine for hundreds, painful at thousands/week from 60-80 concurrent agents.

## Design decisions

### Index format — JSON Lines (`.cairn/index/sessions.idx`)
- Line 1: versioned header `{"_schema":"cairn.index.v1","_built_at":"<RFC3339>","_session_count":N}`
- Lines 2..N+1: one flat entry per session.
- Append-friendly, scannable with `bufio.Scanner`, swappable to SQLite later without API change.

### Entry fields (flat dict)
`session_id, ts (started_at), runtime, model, ticket_id, run_id, role, status,
exit_reason, branch, duration_ms, files_count, input_tokens, output_tokens, cost`.

These cover every scalar `cairn query` filter (ticket, runtime, status, exit-reason,
run-id, since/until via ts, branch) plus sort keys (started_at, duration, session_id)
and table-render fields. The full session.json stays in the ref; the index is a
fast pre-filter + render source.

### Incremental update — dedup by session_id
The ULID *is* the session_id and equals the last path component of the ref name, so
`for-each-ref` yields known session_ids **without reading any blob**. Update reads the
existing index, builds a set of indexed session_ids, then `git show`s session.json
**only** for refs not already indexed. This is the measurable "did NOT re-read the first
10": `BuildResult.Parsed` counts blob reads — update over 10-existing + 5-new yields
Parsed=5. (Committer-date filtering is unreliable here because EndTime defaults to a
fixed epoch in test fixtures; session_id-set dedup is robust regardless of timestamps.)

### Query integration — index as pre-filter, with cheap existence check
`cairn query` path selection:
- `useIndex = !--no-index && index file exists` (default: always use if present).
- Index path: load entries → one `for-each-ref` to get the live session_id set →
  drop entries whose ref no longer exists (handles `TestIndex_StaleHandling`) →
  apply scalar filters on entry fields.
  - Fast case (no `--has-files`, no `--json`): build a partial `schema.Session` from
    each surviving entry; reuse existing `sortSessions`/`writeTable`/count. **Zero**
    `git show` calls — this is the <50ms path.
  - Full case (`--json` OR `--has-files` set): `git show` session.json per surviving
    candidate (narrowed set, not all refs), unmarshal, apply remaining filters, emit
    full records — output identical to the ref-walk path.
- Fallback path (no index / `--no-index`): existing `loadSessions` ref-walk, unchanged.

### Testability sentinel
Add `query.RunToWithStats(args, stdout, stderr) (QueryStats, error)` returning
`{Source string /* "index"|"refs" */, RefShows int}`. `RunTo` delegates and discards
stats. Tests assert `Source=="index"` and `RefShows==0` on the fast path, `Source=="refs"`
under `--no-index`. No stderr pollution, no env-var hacks.

## File plan

- **`internal/index/index.go`** — `Header`, `Entry`, `Stats`, `BuildResult`, schema const,
  `DefaultRelPath`, path resolution, `EntryFromSession`, `EntryToPartialSession`,
  `Load`, `Write`, `Exists`, `Drop`, `Show`.
- **`internal/index/build.go`** — git helpers (`listSessionRefs`, `gitShow`, `runGit`),
  `Build(repo)`.
- **`internal/index/update.go`** — `Update(repo)` (incremental, session_id dedup).
- **`internal/index/index_test.go`** — all mandatory index-level tests + benchmark.
- **`cmd/entire-agent-cairn/index.go`** — `RunIndex(args)` dispatch for
  `build|update|show|drop`.
- **Modify `internal/query/query.go`** — add `--no-index`, `RunToWithStats`, index path.
- **Modify `cmd/entire-agent-cairn/main.go`** — wire `case "index"`.
- **Modify `internal/testutil/testutil.go`** — widen `*testing.T` → `testing.TB` so
  benchmarks can reuse `NewTestRepo`/`WriteSession` (TB has Helper/Fatalf/TempDir/Cleanup).
- **`internal/query/index_query_test.go`** — query+index integration tests
  (uses-index / fallback / no-index-flag / stale).

## Mandatory tests (mapping)

| Test | Location |
|------|----------|
| BuildFromEmpty / BuildFromN | index_test.go |
| Update_Incremental (Parsed==5) | index_test.go |
| Show / Drop / SchemaVersion | index_test.go |
| QueryUsesIndex / QueryNoIndexFallback / NoIndexFlag / StaleHandling | index_query_test.go |
| BenchmarkQueryWithIndex vs WithoutIndex | index_test.go (or query bench) |

## Performance target
1000 sessions: `index build` < 5s; indexed `query --status --runtime` < 50ms vs 500ms+
ref-walk. Benchmark proves the speedup. Functional tests use small N (≤20); benchmark
setup (larger N) only runs under `-bench`.

## Risks / mitigations
- **Branch field**: index stores one branch (GitStart else GitEnd); partial session sets
  `GitStart.Branch` so `matchesBranch` works. Acceptable fidelity loss.
- **has-files**: not indexable without bloating; forces full-record load for candidates.
  Documented; correctness preserved.
- **Stale entries**: never trusted for existence — live `for-each-ref` set gates output.
