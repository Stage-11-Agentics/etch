# Code Review (own-reviewer fallback): ETCH-35 — no-git visibility

## Verdict: PASS

**Process note:** the auto-fired code-review and the prescribed worktree re-run both
aborted with "Diff is empty" (the harness could not resolve the feature-branch diff).
Per the run boot prompt this is the own-reviewer fallback: a structured self-review of
the ETCH-35-relevant hunks of `fix/repo-root-batch` @ f869d9d against origin/main.
ETCH-34's independent code-review (art_01KTH9ZEKTFN8F7MWCM6SWN7RN, PASS) reviewed the
SAME commit in full, including the ETCH-35 visible-failure paths, so this fallback is a
second pass over already-reviewed code, not the only review of it.

## Scope reviewed
`internal/hooks/common.go` (resolveContext/printNotOK), `session_end.go` (runEnd error
paths), `session_start.go`/`user_prompt_submit.go`/`tool_use.go` (detect-first ordering),
`internal/capture/repocontext.go` (error wrapping), `reporoot_test.go` tests 5–7.

## Findings

- **Detect-before-write holds for all six hook subcommands.** `resolveContext()` is the
  first call after stdin parse in every entry point; in non-git dirs no `.etch/`, wip, or
  mapping is ever created (verified by TestNonGitDirAllHooksFailVisible + live e2e).
- **Output contract is exactly the cycle-2 pinned one:** stderr warning, machine-readable
  `{"ok":false,"error":...}` on stdout via printNotOK, exit 1 via main.go. No path prints
  ok:true while dropping data. Exit code is 1, never 2 (2 would block at the Claude Code
  hook layer).
- **commitSession failure de-swallowed:** non-ok + non-zero; wip + mapping + session.json
  retained (cleanup in commit.go runs only after WriteSessionRef succeeds — retention is
  correct by construction). Verified by TestCommitFailureVisibleAndRecoverable with
  deterministic nested-ref sabotage.
- **Refuted-finding guard intact:** mapping-miss in runEnd still printOK()s (stop after
  session_end), now with a stderr log line. TestStopAfterSessionEndStaysOK covers it.
- **[MINOR, accepted]** `AppendEvent`/`Finalize` failures inside runEnd return the error
  (stderr + exit 1) without a printNotOK stdout line — stdout is empty, not ok:true, so
  the no-silent-ok contract holds; the machine-readable line is only guaranteed on the
  resolveContext and commitSession paths. Cosmetic inconsistency, not data-dropping.
- **[MINOR, accepted/documented]** Retained wip after a commit failure re-commits only
  after recovery_timeout_hours and via the recovery reconstruction path (lossy — ETCH-40
  finding 9 scope). Eventual recovery is the documented contract (plan cycle-2 #13).

## Gates
`go test ./...` green (incl. 7 adversarial batch tests), `make build` green, `make smoke`
PASS, live non-git e2e: exit 1, loud stderr, ok:false, zero pollution.