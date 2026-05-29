# Plan Review: ETCH-4 — Crash Recovery

### 1. Verdict

**PASS**

### 2. Summary

The ETCH-4 plan covers crash recovery for orphaned `.wip.jsonl` files with a clean three-function API, a well-designed orphan detection heuristic (PID check + timeout fallback), and a thorough 12-test plan. The plan is implementable and aligned with the SPEC and BUILDPLAN. Two coordination issues need attention — the PID field dependency and parallel schema ownership with ETCH-2 — but neither blocks implementation; they're resolvable during the build.

### 3. Issues

**[MAJOR] Design — PID field not available in WIP events**
The plan's primary orphan detection heuristic extracts `pid` from the `session_start` event in the `.wip.jsonl` file. However, the existing `parsehook.Result` struct (from ETCH-1) does not include a `pid` field. The WIP file format example in the plan shows `"pid":12345`, but nothing in the current codebase writes this field. ETCH-2 (session buffer + hook handlers) is the ticket that will write `.wip.jsonl` files and could include PID — but ETCH-2 and ETCH-4 are parallel Wave 2 tickets with no ordering guarantee.
**Recommendation:** The plan should explicitly state that ETCH-4 will define the expected WIP event JSON structure it reads (including `pid` as an optional field), and that the timeout fallback is the primary heuristic until ETCH-2 lands with PID capture. Alternatively, ETCH-4 can capture `os.Getpid()` itself if it's the one writing the WIP — but per BUILDPLAN, ETCH-2 owns WIP writes. Either way, the plan needs to acknowledge that PID-based detection gracefully degrades to timeout-only when PID is absent, and the test plan already covers this (`TestScanOrphaned_DetectsOldWIP`).

**[MAJOR] Files to create — `internal/schema/session.go` conflicts with ETCH-2's schema ownership**
The plan creates `internal/schema/session.go` with session record types and notes it's "shared with future ETCH-2." Per BUILDPLAN's field assignments table, ETCH-2 is the schema-owner for `session.json`. Since both tickets are parallel Wave 2, whichever merges second will face a merge conflict on this file. The plan acknowledges the sharing intent but doesn't specify a coordination strategy.
**Recommendation:** ETCH-4 should define a minimal `Session` struct — only the fields recovery actually needs to populate (`schema_version`, `session_id`, `status`, `exit_reason`, `timing`, `agent`, `prompt`). Keep it intentionally minimal so ETCH-2 can extend or replace it when it lands as the schema-owner. Add a code comment marking the struct as provisional. The merge conflict is small and manageable — just call it out explicitly.

**[MINOR] Design — `RefWriter` interface location unspecified**
The plan defines a `RefWriter` interface with `WriteSessionRef(repoDir string, session *Session) error` but doesn't state which package owns it. It can't live in `internal/schema` (would create a dependency from schema → git plumbing concerns). It shouldn't live in `internal/recovery` either if ETCH-3 and ETCH-7 will also need it.
**Recommendation:** Define `RefWriter` in `internal/recovery` for now (it's the only consumer), with the understanding that ETCH-7 may promote it to a shared location during integration wiring. This is the simplest path that avoids premature abstraction.

**[MINOR] Design — No logging approach specified**
`ScanOrphaned` says "log warning, continue" for unreadable files, but the ETCH-1 scaffold includes no logging library and the plan doesn't specify one.
**Recommendation:** Use `log.Printf` to stderr. It's stdlib, zero-dep, and adequate for a CLI binary. Don't introduce a structured logging library for this.

### 4. Positive Observations

- **Clean API decomposition.** Three functions with clear single responsibilities — scan, recover, cleanup — is the right granularity. No god-function that tries to do everything.
- **Robust test plan.** 12 tests covering happy paths, edge cases (empty dir, empty file, corrupt lines, dead PID), and a full integration flow. The corruption-resilience tests (5, 8, 9) are particularly valuable for crash recovery code.
- **Smart interface boundary.** The `RefWriter` interface stubs the ref-writing dependency cleanly, allowing ETCH-4 to be fully testable in isolation without waiting for ETCH-3.
- **Configuration separation.** Accepting timeout as `time.Duration` parameter and leaving config reading to the caller is the right layering. No config package dependency needed.
- **Graceful degradation design.** The orphan detection heuristic chain (PID check → timeout fallback) handles the real-world case where PID may not be available, and "dead PID = orphaned regardless of timeout" is the correct priority.
- **Aligned with OUTPUT_SPEC.** The crashed session fields (`status: incomplete`, `exit_reason: crash`, null `timing.ended_at`/`duration_ms`) match scenario 2c in OUTPUT_SPEC.md exactly.
