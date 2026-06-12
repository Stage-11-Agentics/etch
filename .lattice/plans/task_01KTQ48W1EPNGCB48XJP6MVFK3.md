# ETCH-46 — etch doctor: capture health check

The rollout's answer to "capture silently breaks." One command, human table +
`--json`, telling an operator whether Etch is actually working in this repo.
Scope = the ticket's five base checks + the operator-mode checks added from
ENABLEMENT.md/ETCH-48 (ticket comments).

## Base

Branch `etch-46-doctor` off **origin/main @ b674c82**, worktree at
`../Etch-worktrees/etch-46-doctor`. PR to main, auto-merge when green.
Code-review per the lessons-learned recipe: merge first if the auto-reviewer
can't resolve the worktree diff, then `lattice code-review --base <pre-merge
SHA>` from the ff'd main checkout; fix findings forward.

## Design

New package `internal/doctor`; subcommand `doctor` with `--json` and
`--warn-age <days>` (default 14). Each check yields {name, status
ok|warn|fail|info, detail}; humans get an aligned table with a one-line
verdict, `--json` gets one field per check plus `healthy` and `warnings`
booleans.

**Exit policy (per ticket):** non-zero only on hard failures — binary not on
PATH, or hook coverage missing/partial *while capture is not explicitly
disabled*. Everything else (stale age, no refspec, stamp gaps, hooksPath
quirks, orphan wips) is warn/info, exit 0.

### Checks

1. **binary** — `exec.LookPath("entire-agent-etch")`. Not found → FAIL.
   Found at a different file than `os.Executable()` (symlinks resolved) →
   exec its `info`, compare `version` against `version.Version`; mismatch →
   warn (path + both versions in detail).
2. **enablement** — `etch.enabled` true/false/absent (reuse
   `enable.parseConfigKey` semantics via a small exported helper) + implied
   mode: operator / explicitly disabled / team-default (absent).
3. **hooks** — per the 5 events, etch entry present in `.claude/settings.json`
   (committed) and/or `.claude/settings.local.json` (stamp) at the *current
   worktree* root. Full/partial/none per source. Combined coverage none or
   partial → FAIL unless `etch.enabled=false` (then info: "capture disabled").
4. **refspec** — every remote's fetch/push entries matching
   `refs/etch/sessions`. None → info "local-only capture (correct posture for
   public repos)", never a warning. Report per-remote facts.
5. **session age** — newest `refs/etch/sessions/` committer date via
   `git for-each-ref --sort=-creatordate --count=1`. Older than warn-age →
   warn. Zero refs → info "no sessions captured yet" (never an error).
6. **wip buffers** — count `.etch/sessions/*.wip.jsonl` at the shared state
   root; oldest age vs `recovery_timeout_hours` (config.Load). Pile of
   wips older than the timeout → warn (recovery isn't firing); young wips →
   info (live sessions).
7. **worktree stamps** (operator mode on) — every existing worktree's
   settings.local.json carries all 5 guarded entries; gaps → warn (rerun
   `etch enable`). Key false/absent but stamps exist → warn (stamps present
   without operator mode: hand-stamps or leftover state).
8. **post-checkout propagation** (operator mode on) — etch block present in
   the *effective* hooks dir; missing → warn. `core.hooksPath` set to a
   RELATIVE path → warn even if the block exists (per-worktree resolution
   defeats self-propagation — the ETCH-48 documented gap). Non-shell hook →
   warn (block can't chain; stamp-worktree manual path).
9. **dedupe sanity** — stamps whose commands lack the
   `grep -qs entire-agent-etch .claude/settings.json` guard → warn (double
   capture risk on branches with committed hooks). settings.json *mentions*
   the binary without carrying complete etch hook entries → warn (grep-guard
   false positive: stamps would yield with nothing capturing).

Reuse, don't duplicate: export tiny helpers from `internal/enable`
(worktree list, effective hooks dir, config-key read, stamp command shape)
and `internal/install` (per-event entry presence — generalize
`areClaudeHooksInstalled` into an exported per-source report).

main.go dispatch + usage row (Session commands section). Doctor performs
zero writes — read-only by construction.

## Tests (temp-repo harness; PATH controlled via RunBinaryWithEnv so the
binary check is deterministic on CI)

1. Healthy team-mode repo (install-hooks + one fresh session ref) → exit 0,
   binary/hooks/age ok, refspec info, `--json` parses with a field per check.
2. Hooks missing → exit non-zero, hooks=fail. Partial (3 of 5 events) →
   exit non-zero.
3. Zero sessions → "no sessions captured yet", exit 0.
4. Stale newest session (ref with old committer date via testutil
   WriteSession Timing.EndedAt) → warn, exit 0; `--warn-age 100000` → ok.
5. Binary not on PATH (PATH=/usr/bin only) → exit non-zero, binary=fail.
6. Operator-mode repo (enable + worktree) → stamps/propagation/dedupe ok;
   then strip one worktree's stamp → warn; relative core.hooksPath → warn;
   de-guarded stamp command → warn; settings.json mention without hooks →
   warn.
7. `etch.enabled=false` + no hooks anywhere → exit 0 with "capture disabled"
   info (explicit off is healthy).
8. Orphan wip older than recovery timeout → warn; fresh wip → no warn.
9. Doctor never writes: snapshot repo file tree before/after, identical.

## Docs

- README: doctor row in the troubleshooting/Configure area (one short
  paragraph — "is Etch working here?").
- `code/platform/etch.md` (Forgejo, post-merge): replace the "weekly query
  spot check" interim and the "doctor not yet shipped" references in the
  query cheatsheet/gotchas with the real command.
- ROLLOUT.md: wave-0.5 risk line gets its ✅.
