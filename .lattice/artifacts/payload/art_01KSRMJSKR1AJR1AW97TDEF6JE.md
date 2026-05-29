# Plan Review: ETCH-10 — cairn index

## 1. Verdict

**FAIL (plan-level)**

The plan is well-researched and most of it is sound and directly implementable. It fails on a single but consequential correctness gap: as designed, the default indexed query path **silently under-reports sessions created since the last `index build`/`update`** — the exact failure mode that matters for a feature whose entire justification is "thousands/week from 60–80 concurrent agents." This needs a plan-level decision before implementation. The remaining issues are minor and easily folded into the revision.

## 2. Summary

I reviewed the ETCH-10 plan against the live codebase (`internal/query/query.go`, `filter.go`, `schema/session.go`, `testutil/testutil.go`, `cmd/.../main.go`, and the ref layout in `internal/refs/writer.go`). The plan's technical foundations check out: the session_id *is* the last path component of `refs/cairn/sessions/<ULID>`, so `for-each-ref` yields the indexed-set without reading blobs; `EndTime` genuinely defaults to `time.Unix(1700000000, 0)` in `testutil.WriteSession`, so the plan is right to reject committer-date dedup in favor of session_id-set dedup; and every scalar filter, sort key, and table-render field is reconstructable from the proposed flat entry. The key concern is index freshness on the default query path.

## 3. Issues

**[CRITICAL] Query integration — default indexed path silently drops new (un-indexed) sessions**
The plan's query path computes the live session_id set via `for-each-ref`, then *drops* index entries whose ref no longer exists (stale-deletion handling). It never handles the inverse: refs present in the live set but **absent from the index** — i.e., sessions created after the last `index build`/`update`. With `useIndex = !--no-index && index file exists` defaulting on, a query run against an index that is even seconds out of date will return *fewer* results than the ref-walk would. In the stated environment (60–80 concurrent agents writing refs continuously), the index is essentially always behind, so the default `cairn query` would routinely omit the most recent sessions with no error or warning. That is a correctness regression versus ETCH-9, hidden behind a performance win, and it contradicts the task's "fast lookup at scale" intent — the sessions you most want at scale are the recent ones.
**Recommendation:** Decide and document the freshness contract at plan level. The cleanest option reuses logic the plan already has: on the fast path, compute `live − indexed`; if non-empty, `git show` those few new refs (incremental, same cost model as `Update`), merge them, and proceed — keeping correctness while preserving the speedup for the bulk of already-indexed refs. Acceptable alternatives: (a) auto-run the incremental `Update` before serving a query, or (b) if the index is intentionally a point-in-time cache, emit a stderr warning like `index is N sessions behind; run 'cairn index update' or pass --no-index` and add it to the `QueryStats` sentinel so tests can assert it. Whatever the choice, add a test that asserts an indexed query over "10 indexed + 5 newly-written refs" returns all 15 (or warns) — the mirror of the existing `Update_Incremental` test.

**[MINOR] Entry fields — nullable `duration_ms` / `started_at` must round-trip nil, not zero**
`writeTable` renders `-` when `Timing.DurationMS == nil` and `sortSessions` sorts sessions with no parseable `StartedAt` *last*. If the index stores `duration_ms`/`ts` as non-pointer zero values, a session that genuinely has no duration/start will reconstruct as `DurationMS=&0` (renders `0s`) and `StartedAt=&""` (changes sort placement), making the fast-path output diverge from the ref-walk output the plan promises is "identical."
**Recommendation:** Store these as omittable/nullable in the entry and reconstruct as nil pointers when absent, so `EntryToPartialSession` reproduces `-` and last-place sort ordering exactly. Add an assertion to the QueryUsesIndex test that a no-duration / no-start session renders identically on both paths.

**[MINOR] Index format — concurrent/atomic write not addressed**
At the target concurrency, two `index build`/`update` invocations (or a reader during a write) could observe a truncated `sessions.idx`. The plan describes the format but not how it's written.
**Recommendation:** Write to a temp file in `.cairn/index/` and `os.Rename` into place (atomic on same filesystem). One sentence in the design and a note that readers tolerate a missing file (already the fallback path) is enough. Storing under the gitignored `.cairn/` dir is the right call and consistent with existing convention (`internal/capture/buffer.go`, `internal/config/config.go`) — no change needed there.

**[MINOR] Benchmark — exclude index-build cost from the timed loop**
`BenchmarkQueryWithIndex` must build the index in setup and call `b.ResetTimer()` before the query loop, or the "<50ms" claim is polluted by one-time build cost. Not stated.
**Recommendation:** Note `b.ResetTimer()` after setup in the plan's benchmark step.

## 4. Positive Observations

- **Correct, blob-free dedup grounded in the actual ref layout.** The plan verified that the ULID equals the last path component of the ref and that `EndTime` defaults to a fixed epoch in fixtures — both true in the code — and chose session_id-set dedup over the naive committer-date approach for exactly the right reason. This is the kind of fixture-aware reasoning that prevents a whole class of flaky tests.
- **`RunToWithStats` sentinel is the right testability primitive.** Returning `{Source, RefShows}` and having `RunTo` delegate-and-discard avoids env-var hacks and stderr pollution, and gives tests a clean assertion for "fast path took zero `git show` calls." Good instinct.
- **Clean path partitioning.** Routing `--has-files`/`--json` to a full-record load over a *narrowed* candidate set (rather than all refs) preserves correctness for the un-indexable glob filter while keeping the common case fast, and the fallback to the unchanged `loadSessions` ref-walk keeps existing query behavior and tests intact.
- **`testutil` `*testing.T` → `testing.TB` widening is correct and low-risk.** Every method the helpers use (`Helper`, `Fatalf`, `TempDir`, `Cleanup`) is on the `TB` interface, and all existing `*testing.T` call sites satisfy `TB` unchanged — so benchmarks can reuse `NewTestRepo`/`WriteSession` with a purely mechanical change.
- **Honest risk section.** The branch-fidelity tradeoff and the has-files non-indexability are surfaced rather than hidden, and the file plan maps cleanly onto the existing package structure (`internal/index`, `cmd/.../index.go`, `case "index"` in `main.go`).

---

Once the freshness contract (CRITICAL) is decided and the three minors are folded in, this is a strong, implementation-ready plan. The gap is in the default-behavior correctness, not the architecture.
