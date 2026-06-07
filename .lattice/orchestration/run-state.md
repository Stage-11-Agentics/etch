# Run State — Etch Backlog Completion Prep

Prepared: 2026-06-03
Project root: `/Users/atin/Projects/Stage11/code/Etch`
Status: prep only; no orchestrator or delegators launched.

## Preflight

| Check | Result | Evidence |
|---|---|---|
| Lattice CLI | pass | `lattice, version 0.2.1` |
| Git repo | pass | `main...origin/main` |
| c11 reachable | pass | workspace `workspace:14` |
| Disk headroom | pass | about 1.9 TB free on `/System/Volumes/Data` |
| Current validation baseline | pass | `go test ./...`, `make build`, `make smoke`, `make test-density`, and targeted filtered tests passed |

## Configuration

- **Autonomy level:** Fully Autonomous, matching project `CLAUDE.md`.
- **Concurrent delegator cap (N):** 5 initial cap.
- **Auto-close finished delegator surfaces:** Yes.
- **PR merge policy:** **Auto-merge through to done** (operator decision 2026-06-06, honoring project `CLAUDE.md`; supersedes the earlier `pr_open` setting). The run's own review gates (plan review → code review → fix loop, master validator, closeout audit) are the control.
- **Ticket fidelity:** Existing Lattice backlog tickets are authoritative; delegators should read their task JSON and linked plan artifacts.
- **Master Validator:** On.
- **Closeout audit:** On.
- **Result Validator (Phase 4):** On.
- **Dispatch status:** **APPROVED 2026-06-06.** Operator settled all seven open decisions and approved full-scope dispatch.

## Operator Decisions (2026-06-06) — all decision-first items resolved

| Item | Decision |
|---|---|
| local_only_fields (ETCH-40 f.6 / was ETCH-31) | **Implement** real strip-before-push → spun out as **ETCH-41** (high), lands after refspec batch. README marks feature 'in development' until then. |
| tokens (ETCH-40 f.10 / was ETCH-32) | **Drop from v1 spec**: OUTPUT_SPEC amended to null-in-v1/reserved; delete dead aggregation paths; v2 enrichment is future work. |
| hostname hash (ETCH-37) | **Per-repo salt**: random salt at first init, stored in committed `.etch/settings.json`; hash = SHA-256(salt+hostname). |
| upstream session id (ETCH-23) | **Add** optional `agent_session_id` to the schema from the hook payload's session id. Priority raised to medium. |
| PR merge policy | **Auto-merge all** through to done. |
| ETCH-17 | **Promoted to Wave 0 investigation lane** (priority→critical): root-cause Entire 0.6.3 dispatch failure, implement the real fix, then enable dogfooding on this repo. |
| Run scope | **Everything** — all backlog tickets + ETCH-40 + ETCH-41. |

## Current c11 Topology

| Role | Ref | Notes |
|---|---|---|
| workspace | `workspace:14` | Current Etch workspace |
| main view area | `pane:52` | Current operator/session terminal, surface `surface:217` |
| control surface | `pane:58` | Lattice board browser selected on `surface:107`, URL `http://localhost:55492/` |
| prep/delegate pane | `pane:60` | Orchestrator running in `surface:387` ("Etch Orchestrator") |
| orchestrator surface | `surface:387` | claude-code orchestrator, launched 2026-06-07 |

## Live Lattice Summary

Source of truth:

```bash
lattice list --status backlog --json
```

Current task counts (post 2026-06-04 deep review — see `reviews/2026-06-04-deep-code-review.md`):

| Status | Count |
|---|---:|
| backlog | 19 |
| cancelled | 7 |
| done | 14 |

**2026-06-06 reconciliation:** The 2026-06-04 deep code review (ETCH-40, critical umbrella) superseded ETCH-25, ETCH-30, ETCH-31, ETCH-32, ETCH-33, ETCH-36 (all cancelled, linked `supersedes` from ETCH-40). The batch plan below replaces the original; the review file is the authoritative spec for all ETCH-40 findings (it includes refuted non-bugs — do not re-fix those).

## Worker Batch Plan

The Lattice tickets remain the unit of tracking. ETCH-40 spans multiple file surfaces, so its findings are distributed across workers; ETCH-40 closes only when all its findings have landed (closeout audit verifies against the review file).

| Batch | Tickets / scope | Dispatch shape | Why |
|---|---|---|---|
| Redaction completeness | ETCH-26, ETCH-27, ETCH-28, ETCH-29, ETCH-39 **+ ETCH-40 findings 5, 7** | One inline-full worker | Same surface: `internal/redact` patterns + the commit-boundary redaction pass (finding 5 moves redaction from per-field to whole-record — do this first, then patterns). |
| Repo-root + no-git safety | ETCH-34, ETCH-35 **+ ETCH-40 finding 2** | One inline-full worker | Same root cause: `findRepoRoot()=os.Getwd()`. Fix the boundary once (git common-dir resolution), and no-git behavior falls out of the same code path. |
| Lifecycle/recovery integrity | **ETCH-40 findings 1, 3, 4, 8, 9 + below-cut (scan perf, gitDiffFiles, archive atomicity, utf8)** | One inline-full worker, sequential inside; may split into 2 PRs (lifecycle guards / recovery-parity refactor) | Replaces the old Recovery/perf batch (ETCH-30/33/36 superseded). Depends on repo-root batch landing first. Finding 9's fix (shared wip→session reducer) subsumes ETCH-33's double-count. |
| Auto-capture investigation | **ETCH-17 (Wave 0, critical)** | One inline-full worker: investigate → fix → enable dogfooding on this repo | Existential: zero real sessions ever captured; Entire 0.6.3 never dispatches to the plugin. Once fixed, the rest of the run live-validates itself via dogfooding. ETCH-20 (hook contract docs) rides with it. |
| Refspec/sync | ETCH-16, ETCH-18, ETCH-22, ETCH-24, ETCH-38 | One inline-full worker | One coherent `setup-refspec` and transport story; ETCH-22 is subsumed by ETCH-38. |
| Local-only transport | **ETCH-41** (Wave 2, after refspec lands) | One inline-full worker | Strip-before-push projection (decision: implement). Same setup-refspec surface as the refspec batch; reuses finding 5's record-walking machinery. |
| CLI/docs UX | ETCH-19, ETCH-21 | One fast-track worker | Shared README/CLI discoverability surface. |
| Decided schema/privacy items | ETCH-23 (agent_session_id), ETCH-37 (per-repo salt), ETCH-40 f.10 (drop tokens from v1 spec) | One inline-full worker — decisions are made, see Operator Decisions table | All three touch schema/OUTPUT_SPEC/README; small coordinated PR. |

## Ticket Wave Plan

| Ticket | Task ID | Priority | Wave | Depends on | Workflow mode | Status | Notes |
|---|---|---|---|---|---|---|---|
| ETCH-25 | `task_01KSTXRD4RP1NDF51KN56Y6SPF` | critical | 0 | — | inline-full grouped | backlog | Redaction batch |
| ETCH-26 | `task_01KSTXRD7W1S9VP2S5PT9KCSS4` | critical | 0 | — | inline-full grouped | backlog | Redaction batch |
| ETCH-27 | `task_01KSTXRDB1MKAD1J2P61BTWT57` | critical | 0 | — | inline-full grouped | backlog | Redaction batch |
| ETCH-28 | `task_01KSTXRDE3FGNXR6PMRE0D0SE0` | critical | 0 | — | inline-full grouped | backlog | Redaction batch |
| ETCH-29 | `task_01KSTXRDH5V9Q8DCYBQ1QMSCTK` | high | 0 | — | inline-full grouped | backlog | Redaction batch |
| ETCH-39 | `task_01KSTXT5Q7K00H0AYKX1K95ACC` | low | 0 | — | inline-full grouped | backlog | Redaction batch |
| ETCH-34 | `task_01KSTXS9TY054Y1S6GSPNDQ1TV` | medium | 0 | — | inline-full grouped | backlog | Platform-safety batch |
| ETCH-35 | `task_01KSTXT5A0F01XN0CB1J0DBG3R` | high | 0 | ETCH-34 | inline-full grouped | backlog | Platform-safety batch |
| ETCH-40 | `task_01KTAH0J77Q3Y0G6W517EPKMCB` | critical | 0–1 | findings 5,7 in Wave 0 (redaction worker); finding 2 in Wave 0 (repo-root worker); findings 1,3,4,8,9 + below-cut in Wave 1 (lifecycle worker, after repo-root lands); findings 6,10 in Wave 3 (decision-first) | distributed across workers | backlog | Spec: `reviews/2026-06-04-deep-code-review.md`. Supersedes ETCH-25/30/31/32/33/36. Close only at closeout audit. |
| ETCH-16 | `task_01KSTXH7RCZ6JYQ98W0PQSDC6D` | high | 1 | — | inline-full grouped | backlog | Refspec/sync batch |
| ETCH-18 | `task_01KSTXHT73N4R524WNR0MZEY89` | medium | 1 | — | inline-full grouped | backlog | Refspec/sync batch |
| ETCH-22 | `task_01KSTXJ73G61BENR2PQ2S17HP8` | low | 1 | ETCH-38 | inline-full grouped | backlog | Subsumed by ETCH-38; same refspec/sync batch |
| ETCH-24 | `task_01KSTXJG8Q2ZWH849E12QJKHCV` | low | 1 | — | inline-full grouped | backlog | Refspec/sync batch |
| ETCH-38 | `task_01KSTXT5M07WGKA26GXEYF2E9G` | low | 1 | — | inline-full grouped | backlog | Refspec/sync batch |
| ETCH-17 | `task_01KSTXHGVBPDWXBETVYB1MX6B7` | critical | 0 | — (decision made: investigate→fix→dogfood) | inline-full | backlog | Auto-capture investigation lane |
| ETCH-20 | `task_01KSTXJ6XBQQJREX5D9X06G6PX` | medium | 0 | ETCH-17 (rides with it) | inline-full grouped | backlog | Hook contract docs, same worker as ETCH-17 |
| ETCH-19 | `task_01KSTXHTANQNSZQS44241G8EFF` | medium | 2 | — | fast-track grouped | backlog | CLI/docs UX batch |
| ETCH-21 | `task_01KSTXJ70DMDC6V60EB80RF900` | medium | 2 | — | fast-track grouped | backlog | CLI/docs UX batch |
| ETCH-23 | `task_01KSTXJG3MWS6CJWNQV1VVYF6Q` | medium | 2 | — (decision made: add agent_session_id) | inline-full grouped | backlog | Decided schema/privacy batch |
| ETCH-37 | `task_01KSTXT5GR056EE9PK26744T01` | medium | 2 | — (decision made: per-repo salt) | inline-full grouped | backlog | Decided schema/privacy batch |
| _(ETCH-40 f.10)_ | — | medium | 2 | — (decision made: drop tokens from v1 spec) | inline-full grouped | — | Decided schema/privacy batch (was ETCH-32) |
| ETCH-41 | `task_01KTF3X71HAKCV8B0B5VEB08BR` | high | 2 | ETCH-16 (refspec batch lands first) | inline-full | backlog | local_only_fields strip-before-push (decision made: implement); spawned by ETCH-40 f.6 |

## Dispatch Guidance

1. ~~Do not dispatch until the operator explicitly approves.~~ **Dispatch approved 2026-06-06.**
2. Wave 0 runs three parallel lanes: redaction worker (ETCH-26/27/28/29/39 + ETCH-40 findings 5,7), repo-root worker (ETCH-34/35 + ETCH-40 finding 2), and auto-capture investigation (ETCH-17 + ETCH-20 docs).
3. Do not split the redaction tickets across workers.
4. Wave 1 lifecycle/recovery worker (ETCH-40 findings 1,3,4,8,9 + below-cut) MUST wait for the repo-root PR to land — finding 1's recovery fix and finding 2's path anchoring touch the same scan logic.
5. Every ETCH-40 delegator reads `reviews/2026-06-04-deep-code-review.md` first. It contains refuted non-bugs (exit_reason clobber, index races, worktree diff-dir) — do not "fix" those. Acceptance requires adversarial tests: hook re-delivery, idle-timeout false positive, commit-failure injection, duplicate session_start.
6. Treat ETCH-22 and ETCH-38 as overlapping. Prefer one implementation path and close/mark duplicate only with an explicit Lattice note.
7. All decision-first items are RESOLVED — see the Operator Decisions table. Delegators implement the recorded decisions; do not re-litigate them.
8. PRs auto-merge through to done once review gates pass (operator decision 2026-06-06).
9. ETCH-40 status: delegators comment per-finding progress on ETCH-40; only the closeout audit moves it to done, after verifying every finding in the review file is addressed or explicitly deferred.

## Handoff Log

- 2026-06-03 — Prep corrected after audit. Current backlog, topology, wave plan, and validation gates recorded. No agents launched.
- 2026-06-06 — Reconciled with the 2026-06-04 deep code review: ETCH-40 (critical umbrella) added and distributed across Wave 0/1 workers; ETCH-25/30/31/32/33/36 superseded→cancelled; batch plan and dispatch guidance rewritten accordingly. No agents launched; operator gate still in effect.
- 2026-06-06 (later) — Operator settled all seven open decisions (see Operator Decisions table), created ETCH-41, promoted ETCH-17 to Wave 0 critical, switched merge policy to auto-merge, and **approved full-scope dispatch**. Orchestrator launched into `surface:387` (pane:60; the original prep surface:218 had been closed). Launched 2026-06-07.
- 2026-06-07 — Orchestrator (surface:387) booted. Baseline re-verify found `internal/archive` red on clean main: time-bomb tests (`TestArchive_ConfigThreshold`, `TestArchive_FlagOverridesConfig`) mixed hardcoded `fixedNow` (2026-05-28) with the binary's real clock; went red ~2026-06-02 as the gap grew. Fixed inline (`daysAgoReal` helper for binary-invoking tests), committed `01a2ca4` to main, full suite + build + smoke + density green.
- 2026-06-07 — **Wave 0 dispatched** (3 inline-full lanes off `origin/main @ 01a2ca4`): redaction batch (ETCH-26/27/28/29/39 + ETCH-40 f.5,7 → surface:397, fix/redaction-batch), repo-root batch (ETCH-34/35 + ETCH-40 f.2 → surface:398, fix/repo-root-batch), auto-capture investigation (ETCH-17+20 → surface:399, fix/auto-capture). Master Validator spawned (surface:396, pane:52). See agents.md.
- 2026-06-07 (tick 2) — Press-ahead: refspec/sync batch (ETCH-16/18/24/38, ETCH-22 subsumed) dispatched early — no dependencies, 2 free slots under cap 5. surface:402, fix/refspec-batch @ 01a2ca4. 4/5 slots in use; 5th reserved for Wave 1 lifecycle worker (gated on repo-root PR).

## Run-time footguns

- 2026-06-07 — `in_validation` status and `lattice attach --role validation` do not exist in the installed Lattice version; boot prompts must say: validate e2e, attach evidence as a plain note (`--type note`), then go `review → pr_open` directly. (Hit by redact-w0; adapted without thrash.)
- 2026-06-07 — `git restore bin/entire-agent-etch` leaves a STALE binary; delegators demoing the built binary must `make build` AFTER the restore (or demo via `go run`). (Hit by redact-w0 — one confusing demo run.)

- 2026-06-07 (tick 6) — PR #17 (redaction batch) auto-merged @ ec406c7; ETCH-26/27/28/29/39 → done; main re-verified green. surface:397 closed, worktree removed. ETCH-40 f.5+f.7 landed (per-finding comments on ETCH-40).
- 2026-06-07 (tick 6) — Schema/privacy batch (ETCH-23/37 + ETCH-40 f.10) dispatched to surface:406, fix/schema-privacy @ ec406c7 (post-redaction main). Boot prompt corrected for the no-in_validation footgun. 4/5 slots used; 5th still reserved for lifecycle worker.
- 2026-06-07 (tick 7) — PR #18 (repo-root batch) auto-merged @ eacd4ed; ETCH-34/35 → done; main green; redaction work verified intact post-merge. surface:398 closed, worktree removed. **Wave 1 lifecycle/recovery worker dispatched** (ETCH-40 f.1,3,4,8,9 + below-cut → surface:409, fix/lifecycle-recovery @ eacd4ed). 5/5 slots used: refspec (review), auto-capture (impl), schema/privacy (planned), lifecycle (booting) + 1 done-pending-close.
- 2026-06-07 (tick 8, ~12:40) — Wakeup fired late (~75 min gap). PR #19 (refspec) auto-merged @ 876c0cb; ETCH-16/18/24/38 → done; ETCH-22 cancelled by delegator (subsumed, note attached). Local run-state checkpoint committed (c717161) + lessons-learned union-merged (c0be772); main green, pushed. **ETCH-41 dispatched** (surface:411, feat/local-only-transport). PR #20 (auto-capture) has conflicts — delegator nudged to rebase. ETCH-42 (auto-filed review-failure investigation) root-caused (worktree↔root diff bug, documented in lessons-learned), commented, needs_human cleared, cancelled. In flight: auto-capture (rebase), schema/privacy (impl), lifecycle (plan), ETCH-41 (boot).
- 2026-06-07 (tick 9, ~12:55) — PR #20 (auto-capture) auto-merged after delegator rebase; ETCH-17/20 → done; main green. **Dogfooding ENABLED on main repo** (make install → entire enable --agent etch → hooks installed:true → setup-refspec). 0 session refs yet — next fresh agent session is the first live capture test. surface:399 closed. **CLI/docs UX batch (ETCH-19/21, fast-track) dispatched** → surface:413, docs/cli-discoverability. In flight: schema/privacy (impl), lifecycle (plan/impl), ETCH-41 (planning), CLI/docs (boot).
- 2026-06-07 (tick 9b) — **c11 PTY wedge hit**: three consecutive new-surface PTY init failures (surfaces 412/413 on pane:60, 414 on pane:58 — created in tree but "Terminal surface not found" on read/send). Existing surfaces unaffected. Routed around by reusing idle surface:103 (pane:58) for the CLI/docs delegator — launch verified. NOTE FOR OPERATOR: new-surface creation is wedged; a c11 restart at a convenient moment will clear it (session persistence + conversation resume should recover the delegators). Remaining dispatches will reuse idle/freed surfaces.
- 2026-06-07 (tick 10, ~12:58) — **DOGFOODING CONFIRMED LIVE**: 3 session refs captured (first: 01KTHFX12CHB9NSKFVWH9QVVG2, commit 6039f16, agent claude-code/claude-opus-4-8, status complete). Etch went from zero-real-sessions-ever to recording the agents building it. In flight: CLI/docs (impl), ETCH-41 (impl), schema/privacy (code-review), lifecycle (impl).
- 2026-06-07 (tick 11, ~13:05) — PR #21 (schema/privacy) auto-merged; ETCH-23/37 → done; ETCH-40 f.10 landed; main green, pushed. surface:406 closed, worktree removed. 15/20 done. Remaining: ETCH-19/21 (self-review), ETCH-41 (impl), ETCH-40 (closeout-gated).
- 2026-06-07 (tick 12, ~13:10) — PR #22 (CLI/docs) auto-merged; ETCH-19/21 → done; main green, pushed. 17/20 done. Remaining: ETCH-41 (code-review phase) + ETCH-40 (closeout-gated). surface:103 left open (borrowed operator surface), worktree removed.
