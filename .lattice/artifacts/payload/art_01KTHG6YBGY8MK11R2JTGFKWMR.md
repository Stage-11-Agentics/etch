# Code Review: ETCH-23 (Schema/Privacy Batch — ETCH-23 + ETCH-37 + ETCH-40 f.10)

## 1. Verdict

**PASS** — Implementation is correct, complete, and meets the plan and acceptance criteria.

## 2. Summary

Reviewed the schema/privacy batch commit (`6182c32`), which implements three pre-decided operator items: `agent_session_id` preservation (ETCH-23), per-repo salted hostname hashing (ETCH-37), and reserving `tokens` as null-in-v1 with dead-path removal (ETCH-40 f.10). The implementation matches the plan item-for-item, all callers of the changed signatures (`CaptureMachine`, `HashHostname`, `GetHostname`) are updated, docs (OUTPUT_SPEC, README, SPEC) are consistent with behavior, and the test coverage is strong — unit tests per item plus a genuine end-to-end test (`TestE2ESchemaPrivacyBatch`) asserting against actual committed `session.json` records. `go build ./...` and `go test ./...` are green; the `.gitignore` carve-out verifies correctly (`git check-ignore .etch/settings.json` → exit 1, `.etch/sessions/...` → exit 0).

## 3. Issues

Only two minor, non-blocking observations — neither warrants rework.

**[MINOR] internal/config/config.go:56 — Malformed `.etch/settings.json` permanently degrades to unsalted hashing**
If `settings.json` exists but contains invalid JSON, `readSalt` silently returns `""` (its `json.Unmarshal` error is swallowed), then `EnsureHostnameSalt` re-reads the file and fails on `json.Unmarshal(data, &m)`, returning an error. `session_start` logs and falls back to the unsalted hash on every session — the file is never repaired, so hashing stays permanently unsalted with only a log line. This is an edge case (a user hand-corrupting the file; `config.Load` also errors on the same input) and the degradation is safe, so it's acceptable as-is.
**Fix (optional):** On a JSON parse failure of an existing settings file, either surface a clearer one-time warning or treat the file as replaceable (back it up and rewrite) so salting self-heals.

**[MINOR] internal/redact/hostname.go:25 — `GetHostname` remains production-unused**
`redact.GetHostname` is referenced only by its own tests; no production path calls it (the capture path uses `redact.HashHostname` directly via `CaptureMachine`). This is pre-existing dead code that the plan explicitly acknowledged; the PR correctly wires the salt through it to keep the single-derivation invariant, but the function itself stays unused. Not introduced by this change.
**Fix (optional, separate cleanup):** Remove `GetHostname`/`HostnameResult` if nothing is expected to consume them, or note them as intentional public API surface.

## 4. Positive Observations

- **Single-derivation discipline (Cycle-1 resolution #3 honored).** `redact.HashHostname(salt, hostname)` is the one canonical hostname-hash function; `capture.CaptureMachine` delegates to it rather than re-implementing `sha256`. The duplicate unsalted implementation the plan-review flagged is gone, and the import direction (`capture → redact → config`) stays acyclic.
- **Atomic, field-preserving salt persistence.** `EnsureHostnameSalt` reads existing settings as a generic `map[string]any` (so `raw_machine_identity`, `recovery_timeout_hours`, and unknown user keys survive a rewrite — directly tested in `TestEnsureHostnameSaltPreservesUnknownFields`), writes via temp-file + `os.Rename`, chmods to `0644`, and re-reads after write so concurrent first-use racers converge on the last writer. The accepted residual race window is documented in both code and commit message.
- **Correct `.gitignore` carve-out.** `.etch/*` + `!.etch/settings.json` is the right idiom (a directory-level `.etch/` ignore cannot be negated for a child file). Verified live: settings.json is trackable, session records stay ignored. The temp `settings-*.json.tmp` files land under the ignored glob, so they never pollute status.
- **Null-vs-absent JSON semantics are tested, not assumed.** `agent_session_id` and `tokens` both use non-`omitempty` pointer tags so the keys are always present and marshal as `null`; `TestFinalizeAgentSessionID` and the e2e test assert key-present-and-null explicitly rather than relying on struct shape. This matches the OUTPUT_SPEC canonical example.
- **End-to-end test exercises real records.** `TestE2ESchemaPrivacyBatch` runs the actual binary across two repos, reads committed `git show refs/etch/sessions/<ULID>:session.json`, and asserts (a) upstream id preserved while the minted ULID stays canonical, (b) within-repo hash stability + cross-repo hash difference, and (c) `tokens` null — covering all three items against ground-truth output, not internal state. The comment explaining why the empty-upstream-id case is covered at the Finalize level (empty id also disables end-hook correlation) is a thoughtful note.
- **Clean dead-path removal.** The ETCH-40 f.10 deletion is precisely scoped: `applyTokenSnapshot`, its call site, and `wipEvent`'s token fields are removed; the `Tokens` schema field is retained with a `// Reserved in v1` comment on both `capture` and `schema` structs. `recovery_test.go` is updated to assert `session.Tokens == nil` always. Conflict-avoidance constraints (recovery.go limited to dead token-path removal; README limited to hostname/token claims) were respected.
- **Docs fully aligned.** OUTPUT_SPEC (identity field note, machine hash comment, all scenario `tokens: null` blocks, incomplete-record note, generator narrative), README (settings + privacy sections), and SPEC.md acceptance #7 all match the implemented behavior — including the "commit settings.json to share the salt" caveat surfaced to users.
