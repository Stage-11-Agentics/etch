# Code Review (own-reviewer fallback) — ETCH-17/ETCH-20 auto-capture fix

**Context:** the auto-fired `lattice code-review` agents for both tickets hung
(75+ min elapsed, child claude process at 6.5s CPU); processes were killed and
this structured self-review executed per the lane's fallback protocol.

**Scope reviewed:** commits 089a027 + a070a7d vs merge-base 01a2ca4
(origin/main) — 14 files, +1433/-58.

## Verdict: PASS (no blocking findings)

## Dimensions

**Correctness / protocol fidelity**
- `info` response pinned field-by-field to Entire v0.6.3 `InfoResponse` /
  `DeclaredCaps` (source tag 17720a12 == installed build). Verified live:
  `entire enable --agent etch --no-github` → RC=0, "Installed 5 hooks",
  `external_agents: true` persisted. Only the `hooks` capability is declared —
  prevents Entire calling unimplemented protocol subcommands.
- Install/uninstall/are-installed responses match `HooksInstalledCountResponse`,
  `AreHooksInstalledResponse`, `DetectResponse`; uninstall stdout is ignored
  by Entire (verified in source).
- `are-hooks-installed` requires ALL five events present → partial installs
  read false and re-enable repairs them.

**Edge cases**
- Foreign `.claude/settings.json` content round-trips untouched (raw-JSON
  surgery; unit test covers foreign matchers, unknown entry fields like
  `timeout`, unknown hook types like `Notification`, permissions/env keys).
- Idempotent install does not rewrite the file (byte-equality tested).
- Malformed settings JSON → explicit error, never a silent clobber.
- Stop hook deliberately NOT installed for Claude Code (would finalize at
  first turn end and truncate multi-turn sessions) — documented in three
  places and locked by a unit test.
- Model backfill soft-fails to null + stderr warning; finalize still commits
  (tested). Transcript reads are line-capped (4MB) — no unbounded memory.
- Missing session_id / prompt / tool_name → stderr warning, exit 0, stdout
  contract unchanged (tested per event).

**Known non-findings (noted for transparency)**
- `parse-hook` still emits its legacy string-typed result, not Entire's
  numeric-`type` eventJSON. It is NOT in the chosen dispatch path (direct
  dispatch; hooks never route through `entire hooks etch …`) and predates
  this change. Filed as future-work note rather than fixed here to keep the
  diff scoped.
- `gofmt -l` flags 8 pre-existing files untouched by this diff; none of the
  files added/modified here are flagged.
- `findRepoRoot()` in hooks returns cwd (pre-existing behavior); installer
  uses `git rev-parse --show-toplevel` with cwd fallback.

**Tests**
- `go test ./...` green (13 packages). New: install round-trip suite
  (7 tests), native-dialect lifecycle, Entire-dialect regression, warning
  contract, model-backfill soft-failure. Updated: info protocol-v1 shape test.
- `make smoke` green incl. new steps 8–10 (enable via Entire → installed-hook
  native-payload session → record assertions: model backfilled, prompt,
  exit reason, coexistence with Entire's claude-code hooks).

**The real thing**
- Fresh temp repo + README configure steps + REAL `claude -p` session →
  `refs/etch/sessions/01KTHA6BQ4T3CQJPYSA8EM02VS` committed with
  model=claude-opus-4-8 (transcript-derived), prompt, tool_use {Read:1},
  duration 9046ms, exit_reason "other". This is the first real auto-captured
  session in project history.

**Docs**
- README configure/usage claims now match verified behavior; HOOK_CONTRACT.md
  documents both dialects with live-captured examples and the v0.6.3 quirks
  (`entire agent add` lacks discovery; external_agents gate).