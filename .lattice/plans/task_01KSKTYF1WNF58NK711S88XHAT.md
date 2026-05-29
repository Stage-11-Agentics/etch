# Plan: ETCH-8 — Density Test (20 concurrent agents)

## Goal

Validate that 20 concurrent sessions produce 20 valid, distinct session refs with no collisions, no dropped records, correct push/fetch via a local bare remote, and crash recovery under load.

## Approach

Single Go test file at `test/density/density_test.go` with build tag `//go:build density`. Three test functions:

### TestDensity20Concurrent

1. Create temp git repo with initial commit, add bare remote
2. Build `entire-agent-cairn` binary once
3. Launch 20 goroutines, each simulating a full session lifecycle:
   - `session_start` (unique Entire session ID per goroutine, e.g. `density-NN`)
   - `user_prompt_submit`
   - `pre_tool_use` (Read)
   - `post_tool_use` (Read)
   - `session_end`
4. All goroutines start concurrently via `sync.WaitGroup`
5. After all complete, verify:
   - `git for-each-ref refs/cairn/sessions/` returns exactly 20 refs
   - Each ref's `session.json` has unique `session_id`, `schema_version = cairn.session.v1`, `status = complete`
   - Each ref's `agent-trace.json` has `version = 1.0`
6. Push refs to bare remote, clone into second temp dir, fetch, verify 20 refs with matching content

### TestDensityCrashRecovery

1. Create temp git repo with initial commit
2. Start a session (`session_start` + `user_prompt_submit`) — leave the .wip file without `session_end`
3. Start a new session (`session_start`) — this triggers crash recovery
4. End the new session
5. Verify: 2 refs exist — one `complete`, one `incomplete` with `exit_reason: crash`

### TestDensityRefUniqueness

1. Run 20 sessions and collect all session IDs from refs
2. Verify all 20 are distinct ULIDs (no duplicates)
3. Verify lexicographic ordering matches temporal ordering

## Key decisions

- Use goroutines (not shell processes) for concurrency — faster, structured assertions, single Go test binary
- Build tag `density` keeps this out of `go test ./...` — run via `go test ./test/density/ -v -timeout 120s -tags density`
- Use `testutil.RunBinary` for each hook invocation (builds binary once, caches path)
- Local bare repo as remote (no network dependency)
- The density test operates against a single shared temp repo — this is the concurrency stress test

## Files to create/modify

- **Create:** `test/density/density_test.go`
- No existing files modified

## Risks

- The binary caching in testutil uses a package-level var — safe for goroutines since `buildBinary` is called before goroutine launch
- All 20 sessions share one git repo — this is intentional (tests real concurrency on the object store)
