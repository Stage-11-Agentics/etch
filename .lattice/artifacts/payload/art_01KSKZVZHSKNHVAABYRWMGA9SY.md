# Plan Review: ETCH-8 — Density Test (20 concurrent agents)

## 1. Verdict

**PASS**

## 2. Summary

The plan for ETCH-8 proposes three focused test functions validating concurrent session creation, crash recovery under load, and ULID uniqueness — all using goroutine-launched binary invocations against a shared temp git repo with a bare remote. The plan is well-structured, covers all three SPEC acceptance criteria (#4, #5, #6), and makes sound architectural choices (goroutines over shell processes, build-tagged isolation, local bare remote). Two implementation-level concerns around goroutine error handling and binary cache warming should be addressed during implementation but don't require a plan revision.

## 3. Issues

**[Major] TestDensity20Concurrent — Goroutine error handling not specified**
`RunBinary` calls `t.Fatal` / `t.FailNow` internally (for binary build failures and potentially for exec errors). In Go, calling `t.Fatal` from a goroutine other than the test goroutine calls `runtime.Goexit` on the wrong goroutine — the test continues running and may pass even though a goroutine failed. The plan says "launch 20 goroutines" with `sync.WaitGroup` but doesn't specify how errors are collected and surfaced to the test goroutine.
**Recommendation:** Use `golang.org/x/sync/errgroup` or a channel-based error collector. Each goroutine should return errors rather than calling `t.Fatal`. Assertions should run in the main test goroutine after all goroutines complete. Alternatively, use `t.Run` sub-tests (which get their own goroutine where `t.Fatal` is safe) with `t.Parallel()`.

**[Minor] TestDensity20Concurrent — Binary cache warming strategy implicit**
The plan correctly identifies the race risk on `cachedBinaryPath` and says "buildBinary is called before goroutine launch," but doesn't specify the mechanism. The `RunBinary` API doesn't expose a standalone build step — the cache is warmed as a side effect of the first `RunBinary` call.
**Recommendation:** Call `RunBinary` once in test setup (e.g., a no-op invocation like `RunBinary(t, repoDir, []string{"info"}, "")`) before launching goroutines to warm the cache. Make this explicit in the plan so the implementer doesn't miss it.

**[Minor] TestDensityRefUniqueness — Overlaps with TestDensity20Concurrent**
TestDensity20Concurrent already verifies "exactly 20 refs" with "unique session_id" per ref. TestDensityRefUniqueness re-runs 20 sessions to check the same uniqueness property plus lexicographic ordering. The uniqueness check is redundant; only the temporal-ordering assertion is additive.
**Recommendation:** Either fold the ULID ordering check into TestDensity20Concurrent (it's one extra assertion) or rename TestDensityRefUniqueness to TestDensityULIDOrdering to clarify it tests ordering, not just uniqueness. Avoids running 20 extra sessions for a mostly-covered property.

## 4. Positive Observations

- **Goroutines over shell processes** is the right call. It keeps assertions structured, avoids shell quoting/escaping, and is faster — while still spawning real `exec.Command` binary invocations that test actual process-level concurrency on the git object store.
- **Build tag isolation** (`//go:build density`) is clean. Density tests stay out of `go test ./...` and run explicitly with `-tags density`. Good operational hygiene.
- **Shared temp repo is intentional and correct.** This is the whole point — testing concurrent writes to a single git repo's object store and refspace. The plan calls this out explicitly in the Risks section, which shows awareness of the design.
- **Push/fetch via local bare remote** is a pragmatic substitution for the SPEC's Forgejo/Atlas requirement. It validates the same ref transport mechanics (refspec push, clone, fetch) without network dependencies, making the test CI-runnable and deterministic.
- **Crash recovery test** correctly separates "leave .wip without session_end" from "start new session to trigger recovery" — matching the actual recovery flow in `internal/recovery/recovery.go` where `RecoverAll` runs at session_start time.
- **Single file, no modifications to existing code.** The plan correctly scopes this as additive test-only work with zero risk to production code.
- **Risk identification** is honest and specific — calling out the binary cache race and shared-repo contention rather than generic hand-waving.
