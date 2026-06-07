# Plan — Schema/Privacy Batch (ETCH-23 + ETCH-37 + ETCH-40 f.10)

**Worker:** agent:schema-w2 (inline-full delegator, worktree `Etch-worktrees/schema-privacy`, branch `fix/schema-privacy`)
**Scope:** One PR off `origin/main` implementing three pre-decided items (run-state.md Operator Decisions, 2026-06-06). Decisions are final; this plan implements, it does not re-litigate.

> Original ticket description (ETCH-23): upstream session_id passed across hook events is correlated correctly but not stored anywhere in the output record; only Etch's minted ULID appears. Decision: add optional `agent_session_id`.

## Item 1 — ETCH-23: add `agent_session_id` (decision: Add)

The upstream hook payload's `session_id` (Entire/agent runtime's own id, `StdinEvent.SessionID` in `internal/hooks/common.go`) is currently used only to key the `.map/` mapping file and is then discarded. Etch's minted ULID stays canonical for refs; we add an **optional** `agent_session_id` field that preserves the upstream id as the join key to Claude Code transcripts, c11 surface manifests, and resume flows.

### Changes

1. **`internal/capture/session.go`**
   - `Session`: add `AgentSessionID *string` with json tag `agent_session_id` (after `SessionID`).
   - `SessionStartData`: add the same field with `omitempty`.
2. **`internal/schema/session.go`** — `Session`: add the same `AgentSessionID *string` / `agent_session_id` field (capture→schema bridge is a JSON round-trip in `commit.go:34`; tags must match).
3. **`internal/hooks/session_start.go`** — populate `data.AgentSessionID = &ev.SessionID` when `ev.SessionID != ""`.
4. **`internal/capture/buffer.go` (Finalize)** — in the `session_start` case, copy `d.AgentSessionID` → `session.AgentSessionID`.
5. **`OUTPUT_SPEC.md`** — add `agent_session_id` to the canonical record example (comment: upstream runtime's own session id; null when the runtime supplied none) and to the identity field-notes near `session_id`.

### Deliberately NOT in scope
- **`internal/recovery/recovery.go` aggregator**: crash-recovered records will not carry `agent_session_id` yet. The lifecycle/recovery worker is about to replace the recovery aggregator with a shared wip→session reducer (ETCH-40 findings 1/3/4/8/9); once recovery rides the Finalize path, the field flows for free. Dispatch note explicitly restricts this batch's recovery.go edits to dead token-path removal. Recorded as a PR note.
- Agent Trace mapping (`trace.go`) — unchanged.

### Tests
- Hooks e2e (temp repo, simulated stdin events): pass `session_id: "upstream-id-123"` through session_start → session_end; assert committed `session.json` contains `"agent_session_id": "upstream-id-123"` AND `session_id` is an Etch ULID ≠ upstream id.
- Finalize unit test: session_start data with `agent_session_id` set → finalized Session carries it; absent → field is null.

## Item 2 — ETCH-37: per-repo hostname salt (decision: Per-repo salt)

`internal/capture/machine.go` hashes the bare hostname (`sha256.Sum256([]byte(hostname))`) — unsalted, trivially brute-forceable for low-entropy hostnames, and identical across repos (cross-repo correlation). Decision: random salt generated at first use, stored in **committed** `.etch/settings.json` (all clones share it → cross-machine correlation within a repo keeps working); `hostname_hash = sha256:hex(SHA-256(salt + hostname))`. README's "salted hash" claim becomes true.

### Changes

1. **`internal/config/config.go`**
   - `Settings`: add `HostnameSalt string` / json `hostname_salt`.
   - New `EnsureHostnameSalt(repoRoot string) (string, error)`:
     - Read `.etch/settings.json` **as a generic `map[string]any`** (preserve unknown/user fields on rewrite).
     - If `hostname_salt` present and non-empty → return it (no write).
     - Else generate 32 random bytes (`crypto/rand`), hex-encode, set the key, `MkdirAll(.etch)`, write back (indented), return salt.
     - Covers both first-init (no settings.json) and the no-salt-yet path (older settings.json): generate + persist on first use.
2. **`internal/redact/hostname.go`** — single derivation function (see Plan-Review Resolutions #3): `HashHostname(salt, hostname string)` → `sha256:hex(SHA-256(salt + hostname))`; `GetHostname(settings config.Settings, salt string)` updated to pass it through.
3. **`internal/capture/machine.go`** — `CaptureMachine(settings config.Settings, salt string)` delegates to `redact.HashHostname(salt, hostname)`. Empty salt (EnsureHostnameSalt error fallback) degrades to the old unsalted hash rather than failing the hook.
4. **`internal/hooks/session_start.go`** — `salt, err := config.EnsureHostnameSalt(repoRoot)` (log on error, proceed with ""), pass into `CaptureMachine`.
5. **README.md** (touch ONLY hostname-hash + token/schema claims per dispatch)
   - Settings docs: document `hostname_salt` — auto-generated at first session, committed so all clones share it; per-repo so hashes don't correlate across repos.
   - Privacy section: "hashed by default" → salted-hash wording; line ~107 "salted hash" claim is now true and stays.
6. **`OUTPUT_SPEC.md`** line ~52: `// SHA-256 of hostname` → `// SHA-256 of (per-repo salt + hostname); salt in committed .etch/settings.json`; line ~149 field note gains per-repo-salt mention.
7. **`SPEC.md`** line 36 (acceptance #7): "hashed (SHA-256 of hostname)" → salted per-repo wording.

### Tests
- `EnsureHostnameSalt`: fresh repo → 64-hex salt generated + persisted; second call returns identical salt; existing settings.json with other fields → fields preserved; existing salt → returned verbatim, no rewrite.
- `CaptureMachine`: same hostname + two salts → different hashes; same salt → stable hash; `sha256:` prefix retained.
- Cross-repo e2e: two temp repos → different `hostname_hash` in committed records; two sessions in one repo → same hash.
- Update existing `TestCaptureMachine` / `TestCaptureMachineRawOptIn` for the new signature.

## Item 3 — ETCH-40 f.10: tokens = null-in-v1 / reserved (decision: Drop from v1 spec)

Review finding 10: no code path ever populates `tokens` — the Entire payload carries no token data, so OUTPUT_SPEC's promise is unmeetable. Decision: spec amended to null-in-v1/reserved, dead aggregation paths deleted, schema field kept as reserved. v2 enrichment is future work.

### Changes

1. **`OUTPUT_SPEC.md`**
   - Canonical example (~101–108): `"tokens": null` + comment — reserved in v1, always null; v2 enrichment future work (upstream payload carries no token data).
   - All scenario examples with populated `tokens` blocks (~225, ~331, ~437, ~560): → `"tokens": null`.
   - Incomplete-record note (~473): tokens is always null in v1 (drop "reflects last event" claim).
   - Density/generator narrative (~857–864): align so synthetic-data prose doesn't promise live token capture.
2. **`internal/recovery/recovery.go`** — delete ONLY the dead token paths (per dispatch note):
   - `wipEvent` token fields (~82–88).
   - `applyTokenSnapshot` + its call site in `RecoverSession` (its zero-check always bails because `flattenHookEvent` never extracts tokens from the nested format — the only format ever written).
   - `flattenHookEvent` has no token extraction to delete (that absence IS the finding); no other recovery edits.
3. **`internal/capture/session.go` + `internal/schema/session.go`** — KEEP `Tokens` field + type; add `// Reserved in v1 — always null...` comment on both.
4. **`internal/capture/buffer.go` (Finalize)** — never had token-assignment code; nothing to delete.
5. **Left intentionally untouched**: `calculate-tokens` command (read path — correctly reports zeros for null), `internal/index` token columns (nil-tolerant), README (no token-capture claims exist; verified by grep).

### Tests
- Update `TestRecoverSession_FullWIP` / `TestRecoverSession_MinimalWIP`: remove token fixture fields; assert `session.Tokens == nil` always.
- E2e: committed `session.json` contains literal `"tokens": null`.

## Plan-Review Cycle 1 Resolutions (AUTHORITATIVE — overrides earlier text on conflict)

ETCH-37's auto-fired plan-review (art_01KTHAQATYPCX15X6G4QPQ0DV9) evaluated the stale per-ticket plan stub (a verbatim ticket-description copy at `plans/task_01KSTXT5GR056EE9PK26744T01.md`), not this batch plan — the batch plan lives at ETCH-23's plan path per dispatch. The stub is now replaced with a pointer here. Triage of its findings against THIS plan:

1. **[CRITICAL] "No actual plan" — RESOLVED**: artifact of reviewing the stub. This document is the plan (files, tests, acceptance criteria all enumerated).
2. **[CRITICAL] "Decision not made; recommend doc-only path" — OVERRIDDEN BY OPERATOR DECISION**: the operator decided 2026-06-06 (run-state.md Operator Decisions): **per-repo salt**, salt committed in `.etch/settings.json`. The reviewer lacked that context. Not re-litigated.
3. **[MAJOR] "hostname hashed in TWO code paths" — ACCEPTED, plan amended**: `internal/redact/hostname.go` (`HashHostname`/`GetHostname`) is a second unsalted SHA-256 implementation (production-unused — only its own tests call it — but divergence risk is real). Resolution: make `redact.HashHostname(salt, hostname string)` the **single derivation function**; `capture.CaptureMachine` delegates to it (no import cycle — redact only imports config); `redact.GetHostname(settings, salt)` updated to match; its tests updated for salt behavior.
4. **[MAJOR] "fix must span all doc surfaces" — ACCEPTED, plan amended**: in addition to README + OUTPUT_SPEC:52, update `SPEC.md:36` acceptance item #7 ("SHA-256 of hostname" → salted per-repo wording) and `OUTPUT_SPEC.md:149` field note (add per-repo-salt mention). A short cross-repo non-correlation note rides with the README settings docs.
5. **[MAJOR] "salt path backward-compat implications" — ADDRESSED BY DECISION + noted**: (1) salt lives in committed `.etch/settings.json` — generated once at first use; (2) committed → stable across machines/clones of the repo, so cross-machine-within-repo correlation (OUTPUT_SPEC:589, :874) is preserved; (3) pre-salt records in a repo carry unsalted hashes that won't equal new ones — accepted consequence of the decision (Etch has zero real captured sessions to date — ETCH-17 — so the migration surface is nil). PR description will note it.
6. **[MAJOR] "no acceptance criteria" — RESOLVED**: this plan's Validation gates + per-item Tests sections are the criteria.
7. **[MINOR] "no verification approach" — RESOLVED**: per-item Tests sections + gates.

ETCH-23's auto-fired plan-review crashed (agent exit 1, no artifact); re-run manually in headless mode — triaged below.

## Plan-Review Cycle 2 Resolutions (AUTHORITATIVE — overrides earlier text on conflict)

Review of THIS batch plan (art_01KTHF6MNRMM9Q27GW3PZYFGGX). Verdict FAIL on ETCH-37 specifics — all fixable in-plan; the operator's per-repo-salt decision itself is not in question. Resolutions:

1. **[CRITICAL] `.gitignore` ignores all of `.etch/` → salt could never be committed — ACCEPTED, plan amended.** Verified: `git check-ignore .etch/settings.json` → ignored by `.gitignore:6` (`.etch/`). Fix: change the pattern to ignore contents-but-not-the-file:
   ```
   .etch/*
   !.etch/settings.json
   ```
   (Git can't re-include a file under an ignored *directory*; `.etch/*` ignores contents instead, so the negation works.) New validation gate: `git check-ignore .etch/settings.json` exits non-zero after the change, AND `.etch/sessions/...` remains ignored. The "never as tracked files" comment refers to session records — those stay ignored; the settings file is config, not a record.
2. **[MAJOR] First-use salt race / non-atomic write — ACCEPTED, plan amended.** `EnsureHostnameSalt` writes atomically (temp file in `.etch/` + `os.Rename`) and **re-reads after write**, returning whatever salt is then in the file — racers converge on the last writer immediately and on all subsequent calls. The residual window (a racer that re-reads before the final overwrite mints one session with a losing salt) is accepted and noted in the PR. No locking — every-hook lock cost isn't warranted for a once-per-repo event.
3. **[MAJOR] "All clones share it" requires a human commit — ACCEPTED, plan amended.** Etch only writes refs, never stages working-tree files. On salt *creation*, log a one-time hint (`log.Printf`: commit .etch/settings.json to share the salt across clones). README wording states cross-machine correlation within a repo holds *once the file is committed*; until then each clone salts independently. PR note included.
4. **[MINOR] redact hostname path — already resolved in Cycle 1 (#3):** `redact.HashHostname(salt, hostname)` becomes the single canonical derivation; not deleted, wired in.
5. **[MINOR] ETCH-23 recovery gap — already acknowledged:** e2e gate asserts on the normal Finalize→commitSession path; recovered-record gap noted in PR.

Status handling: plan revised in place per resolutions above; reviewer's "return to in_planning" is satisfied by this authoritative triage section (single-cycle revise-and-proceed, consistent with the batch dispatch's fix-loop model).

## Sequencing

1. ETCH-37 (config + machine + session_start) — self-contained.
2. ETCH-23 (schema fields + session_start + Finalize).
3. ETCH-40 f.10 (recovery deletions + spec/README).
4. Docs pass covering all three; tests throughout; one PR.

## Validation gates (from dispatch)

- Upstream session id preserved end-to-end into committed session.json (temp repo, simulated hooks, inspect actual `git show refs/etch/sessions/<ULID>:session.json`).
- Salt behavior: cross-repo difference, within-repo stability.
- Tokens null in committed records; spec consistent.
- `go test ./...`, `make build`, `make smoke` green.
- Never commit `bin/entire-agent-etch`.

## Conflict-avoidance constraints (from dispatch)

- recovery.go: ONLY dead token-path removal (lifecycle worker owns aggregator rework).
- README: ONLY hostname-hash and token/schema claims (refspec worker is editing README).
- No other ETCH-40 findings touched; refuted non-bugs left alone.
