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
- **PR merge policy:** Leave at `pr_open` for human review.
- **Ticket fidelity:** Existing Lattice backlog tickets are authoritative; delegators should read their task JSON and linked plan artifacts.
- **Master Validator:** On.
- **Closeout audit:** On.
- **Result Validator (Phase 4):** On.
- **Dispatch status:** Not launched. Await explicit operator approval.

## Current c11 Topology

| Role | Ref | Notes |
|---|---|---|
| workspace | `workspace:14` | Current Etch workspace |
| main view area | `pane:52` | Current operator/session terminal, surface `surface:217` |
| control surface | `pane:58` | Lattice board browser selected on `surface:107`, URL `http://localhost:55492/` |
| prep/delegate pane | `pane:60` | Contains `surface:218` titled `Orchestrator Prep` |
| prepared boot surface | `surface:218` | Prep-only shell surface; no agent running |

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
| Refspec/sync | ETCH-16, ETCH-18, ETCH-22, ETCH-24, ETCH-38 | One inline-full worker | One coherent `setup-refspec` and transport story; ETCH-22 is subsumed by ETCH-38. |
| Hook/docs | ETCH-17, ETCH-20 | Dialogue/product decision first, then inline-full | Auto-capture docs depend on supported Entire versions or manual hook wiring strategy. |
| CLI/docs UX | ETCH-19, ETCH-21 | One fast-track worker | Shared README/CLI discoverability surface; low risk if coordinated with Hook/docs. |
| Product decisions | ETCH-23, ETCH-37, **ETCH-40 findings 6 (local_only_fields) and 10 (tokens)** | Decision first; implement only after decision | Privacy contract, token source, hostname salt, and session-id semantics should not be guessed by delegators. |

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
| ETCH-17 | `task_01KSTXHGVBPDWXBETVYB1MX6B7` | high | 2 | decision: auto-capture path | inline-full grouped | backlog | Hook/docs batch |
| ETCH-20 | `task_01KSTXJ6XBQQJREX5D9X06G6PX` | medium | 2 | decision: hook contract | inline-full grouped | backlog | Hook/docs batch |
| ETCH-19 | `task_01KSTXHTANQNSZQS44241G8EFF` | medium | 2 | — | fast-track grouped | backlog | CLI/docs UX batch |
| ETCH-21 | `task_01KSTXJ70DMDC6V60EB80RF900` | medium | 2 | — | fast-track grouped | backlog | CLI/docs UX batch |
| ETCH-23 | `task_01KSTXJG3MWS6CJWNQV1VVYF6Q` | low | 3 | decision: schema | decision-first | backlog | Preserve upstream agent session ID only if product decision says yes |
| ETCH-37 | `task_01KSTXT5GR056EE9PK26744T01` | medium | 3 | decision: salt vs docs | decision-first | backlog | Implement per-repo salt or correct README limitation |
| _(ETCH-40 f.6)_ | — | high | 3 | decision: privacy contract | decision-first | — | local_only_fields: implement real local-only transport or remove/soften promise (was ETCH-31) |
| _(ETCH-40 f.10)_ | — | medium | 3 | decision: token source | decision-first | — | tokens: hook raw data carries none — decide transcript parse, `calculate-tokens`, or drop from OUTPUT_SPEC (was ETCH-32) |

## Dispatch Guidance

1. Do not dispatch until the operator explicitly approves.
2. Start with Wave 0 after approval: one redaction worker (ETCH-26/27/28/29/39 + ETCH-40 findings 5,7) and one repo-root worker (ETCH-34/35 + ETCH-40 finding 2) can run in parallel without overlapping files.
3. Do not split the redaction tickets across workers.
4. Wave 1 lifecycle/recovery worker (ETCH-40 findings 1,3,4,8,9 + below-cut) MUST wait for the repo-root PR to land — finding 1's recovery fix and finding 2's path anchoring touch the same scan logic.
5. Every ETCH-40 delegator reads `reviews/2026-06-04-deep-code-review.md` first. It contains refuted non-bugs (exit_reason clobber, index races, worktree diff-dir) — do not "fix" those. Acceptance requires adversarial tests: hook re-delivery, idle-timeout false positive, commit-failure injection, duplicate session_start.
6. Treat ETCH-22 and ETCH-38 as overlapping. Prefer one implementation path and close/mark duplicate only with an explicit Lattice note.
7. Treat ETCH-17/ETCH-20, ETCH-23, ETCH-37, and ETCH-40 findings 6/10 as decision-first. For finding 6 (local_only_fields) specifically, choose before coding: implement a real local-only transport model, or remove/soften the README/settings privacy promise until a design exists.
8. Keep PRs at `pr_open` for human review because several tickets change security/privacy behavior.
9. ETCH-40 status: delegators comment per-finding progress on ETCH-40; only the closeout audit moves it to done, after verifying every finding in the review file is addressed or explicitly deferred.

## Handoff Log

- 2026-06-03 — Prep corrected after audit. Current backlog, topology, wave plan, and validation gates recorded. No agents launched.
- 2026-06-06 — Reconciled with the 2026-06-04 deep code review: ETCH-40 (critical umbrella) added and distributed across Wave 0/1 workers; ETCH-25/30/31/32/33/36 superseded→cancelled; batch plan and dispatch guidance rewritten accordingly. No agents launched; operator gate still in effect.
