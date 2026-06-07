# Plan — ETCH-35: no-git-repo visibility (member of the Repo-Root Batch)

**Authoritative batch plan:** `plans/task_01KSTXS9TY054Y1S6GSPNDQ1TV.md` (ETCH-34 primary).
ETCH-34, ETCH-35, and ETCH-40 finding 2 share one root cause — `findRepoRoot()` =
`os.Getwd()` in `internal/hooks/common.go:39` — and are fixed at that single boundary in
one PR off `origin/main` (`fix/repo-root-batch`). This file records the ETCH-35-specific
decisions; the full design, threading map, and test matrix live in the batch plan and its
"Plan-Review Cycle 1 Resolutions" section.

## ETCH-35 decisions

1. **Remedy chosen: visible error, NOT silent no-op.** The task offered "no-op cleanly OR
   surface an error"; the run's validation gate mandates visibility ("Non-git invocation
   is visible — does not return `{"ok":true}` while dropping data"). On non-git:
   - stderr: `etch: could not resolve a git repository (cwd=...): <underlying git error>`
     (wraps git's own error so a missing `git` binary is distinguishable from a non-repo)
   - stdout: `{"ok":false,"error":"..."}` — never `{"ok":true}`
   - exit 1 via `main.go`'s existing error path
   Rationale for loud-on-every-hook: a mid-session hook in a non-git dir also drops data,
   so all six hook subcommands short-circuit identically. Entire treats plugin hooks
   observationally; non-zero exits surface in its hook logging without killing the agent.
2. **Detection chokepoint:** `findRepoRoot()` is deleted; every hook calls the new
   `capture.ResolveRepoContext(cwd)` FIRST and fails before any filesystem write —
   no `.etch/` created, no `.wip` written, no mapping. Nothing to orphan, no pollution.
3. **Commit-failure visibility generalized:** `runEnd` no longer swallows `commitSession`
   errors (`session_end.go:62-67`). On failure: error to stderr, non-ok/non-zero result,
   and the `.wip` + mapping are deliberately retained so the next `session_start`
   recovery scan can retry the commit. Applies to any commit failure (corrupt object
   store, permissions, disk full), not just the non-git trigger.
4. **Pre-existing pollution out of scope:** `.etch/` dirs already created in non-repos by
   the old code are not swept (pre-release, dogfood-only; noted in PR body).

## ETCH-35 acceptance criteria (verifiable)

- All four hook types invoked in a non-git directory: exit code != 0, stderr names the
  resolution failure, stdout does NOT contain `"ok":true`, and no `.etch/` exists after.
- `session_end` whose ref write fails in a REAL repo: non-ok/non-zero result; `.wip` and
  mapping still on disk (recoverable).
- Stop-after-session_end for the same Entire session ID still exits 0 with `{"ok":true}`
  (mapping-miss path is by design — refuted finding, must not regress).
- `go test ./...`, `make build`, `make smoke` green.

## Tests (owned by ETCH-35 in the batch test matrix)

- Batch test 5 — non-git gate: all four hooks in a plain temp dir (assertions above).
- Batch test 6 — commit-failure visibility: deterministic ref-write conflict (pre-created
  conflicting ref/lock), assert non-ok result + retained wip/mapping.
- Batch test 7 — stop-after-end regression guard.

## Reset 2026-06-07 by agent:reporoot-w0-planner
