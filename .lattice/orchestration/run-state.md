# Run State — Etch Build

## Configuration

- **Autonomy level:** Fully Autonomous
- **Concurrent delegator cap (N):** 5
- **Auto-close finished delegator surfaces:** Yes
- **PR merge policy:** Auto-merge (squash-merge at pr_open, lattice complete without operator review)
- **Ticket fidelity:** Minimal (tickets reference BUILDPLAN.md sections)
- **Master Validator:** On
- **Closeout audit:** On
- **Result Validator (Phase 4):** On
- **c11:** Not detected — layout regions are conceptual
- **Lattice dashboard port:** TBD (start on orchestrator boot)

## Wave plan

| Ticket | Title | Wave | Depends on | Workflow mode | Status |
|---|---|---|---|---|---|
| ETCH-1 | Go module + binary scaffold | 1 | — | fast-track | backlog |
| ETCH-2 | Session buffer + hook handlers | 2 | ETCH-1 | inline-full | backlog |
| ETCH-3 | Git ref writer | 2 | ETCH-1 | inline-full | backlog |
| ETCH-4 | Crash recovery | 2 | ETCH-1 | fast-track | backlog |
| ETCH-5 | Security + redaction | 2 | ETCH-1 | fast-track | backlog |
| ETCH-6 | Agent Trace emission | 2 | ETCH-1 | fast-track | backlog |
| ETCH-7 | End-to-end wiring + refspec config | 3 | ETCH-2,3,5,6 | inline-full | backlog |
| ETCH-8 | Density test (20 concurrent agents) | 4 | ETCH-7 | inline-full | backlog |

## Workflow mode rationale

- **ETCH-1** (fast-track): Scaffold work, clear scope, single-file-cluster changes.
- **ETCH-2** (inline-full): Core hook logic with complex stdin/stdout protocol — fresh-eyes review catches protocol mismatches.
- **ETCH-3** (inline-full): Git plumbing is subtle — ref writer correctness is load-bearing for the whole system.
- **ETCH-4** (fast-track): Small, well-scoped recovery logic with clear test criteria.
- **ETCH-5** (fast-track): Regex patterns + config reader — straightforward.
- **ETCH-6** (fast-track): Serialization to a known spec — Agent Trace RFC is well-documented.
- **ETCH-7** (inline-full): Integration ticket wiring multiple components — review catches interface mismatches.
- **ETCH-8** (inline-full): Validation ticket needs fresh eyes to confirm test rigor.

## Dispatch plan

1. **Wave 1:** Dispatch ETCH-1 solo. Fast-track, should complete quickly.
2. **Wave 2:** On ETCH-1 completion, dispatch ETCH-2, ETCH-3, ETCH-4, ETCH-5, ETCH-6 in parallel (5 delegators = cap).
3. **Wave 3:** On Wave 2 completion (specifically ETCH-2, ETCH-3, ETCH-5, ETCH-6), dispatch ETCH-7.
4. **Wave 4:** On ETCH-7 completion, dispatch ETCH-8.

## Decision log

- 2026-05-27 — Architect: Set Fully Autonomous per project CLAUDE.md default.
- 2026-05-27 — Architect: Set auto-merge per project CLAUDE.md default. Operator-AFK runs benefit from not queuing PRs.
- 2026-05-27 — Architect: ETCH-4 (crash recovery) does not block ETCH-7 — recovery is independent of the happy-path integration. ETCH-7 depends on ETCH-2, ETCH-3, ETCH-5, ETCH-6 only.
- 2026-05-27 — Architect: Observation records (refs/cairn/observations/*) deferred — not ticketed for this run.

## Handoff log

- 2026-05-27 — Architect → Orchestrator handoff initiated.
