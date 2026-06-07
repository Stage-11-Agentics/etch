# Plan Review: ETCH-35 — no-git-repo visibility

## 1. Verdict

**PASS**

The plan is complete, feasible, and aligned with the task. Implementation can
proceed, but the implementer should resolve one self-contradictory statement
about the hook stdout contract (Issue 1) before committing to an output format,
and treat the real-Entire exit-1 behavior (Issue 2) as a validation requirement,
not an assumption.

## 2. Summary

I reviewed the ETCH-35 pointer-plan together with its authoritative batch plan
(`task_01KSTXS9TY054Y1S6GSPNDQ1TV.md`) and verified every claim against the
current source (`common.go:38-42`, `session_start.go`, `session_end.go:62-67`,
`user_prompt_submit.go`, `tool_use.go`, `commit.go`, `gitstate.go`,
`cmd/entire-agent-etch/main.go`). The plan correctly identifies the single root
cause (`findRepoRoot()` = `os.Getwd()`), chooses the right remedy (visible error
over silent no-op), detects non-git before any filesystem write, and de-swallows
the `commitSession` error — all matching the task's FIX clause. The one real
defect is an internally contradictory description of what lands on stdout on
failure; it's a clarity defect with a trivial fix, not a structural gap.

## 3. Issues

**[MAJOR] ETCH-35 decisions #1 + batch Resolution #7 — stdout contract is self-contradictory and misdescribes the mechanism**
The pointer-plan decision #1 and the authoritative batch Resolution #7 both
state the failure output is `{"ok":false,"error":"..."}` on stdout, "via the
existing `main.go` error path." But the existing error path
(`cmd/entire-agent-etch/main.go`) does the opposite: on a returned error it
prints `error: %v` to **stderr** and exits 1 — it emits **nothing** to stdout.
Meanwhile the batch-plan *body* (lines 121-127) describes the correct behavior
for that path: "print **no** `{"ok":true}`; return the error → main.go prints
`error: ...` and exits 1" — with no stdout JSON at all. So the plan says two
different things, and the section declared authoritative ("overrides earlier
text on conflict") is the one that's wrong about the mechanism: you cannot get
`{"ok":false}` onto stdout *through the existing main.go path*. An implementer
following Resolution #7 literally has an impossible instruction; they'll either
add a new stdout print (which is not "the existing path") or skip it (which
contradicts the stated output). This governs the hook output contract, so it
should be pinned down before coding.
**Recommendation:** Pick one and make the plan say only that. The testable
acceptance criteria only require "stdout does NOT contain `"ok":true`", which
both interpretations satisfy, so the simplest resolution is to drop the
`{"ok":false,"error":"..."}` stdout claim and keep "return err → stderr
`error:` + exit 1" (matches existing `main.go` with zero new code). If a
machine-readable `{"ok":false}` on stdout is actually wanted, say so explicitly
and add a small `printErr`-style helper the hooks call before returning — and
stop describing it as "the existing main.go path." Either is fine; the plan must
commit to one.

**[MAJOR] batch Risks + "Tests" — the new exit-1 behavior is asserted Entire-safe but not actually validated against real Entire**
The whole remedy rests on the claim that "Entire treats plugin hooks
observationally; a non-zero exit surfaces in Entire's hook logging without
killing the agent." Making **all six** hook subcommands exit 1 in a non-git dir
is a meaningful behavior change, and if that assumption is wrong, a misconfigured
(non-git) cwd would now break agent sessions rather than silently dropping data.
The plan offers `make smoke` as mitigation, but `make smoke` exercises the
**happy path in a real repo** — it does not run a hook in a non-git dir against
the real Entire CLI, so the one new, risky behavior (exit 1 from a hook) is the
exact thing `make smoke` does not cover. The assertion about Entire's tolerance
is stated as fact with no cited evidence.
**Recommendation:** Add an explicit validation step that runs at least
`session_start` (and ideally `session_end`) via the real Entire CLI in a non-git
temp dir and confirms the agent session is not aborted by the exit-1 — i.e.
extend `make smoke` or the step-6 live e2e to include the non-git case. If real
Entire *does* treat a non-zero `session_start` exit as fatal, fall back to the
task's other sanctioned option (clean no-op with a loud stderr line, still no
`.etch/` created) for `session_start` specifically. Capture this as a go/no-go
gate, not an afterthought.

**[MINOR] Batch coupling — ETCH-35 cannot land independently**
ETCH-35 ships inside one PR with ETCH-34 and ETCH-40 finding 2 off
`fix/repo-root-batch`. This is a deliberate, well-justified choice (shared root
cause), but it means ETCH-35's merge is gated on the entire batch being correct,
and a problem in the ETCH-34 threading rework blocks the ETCH-35 fix. Worth
naming as an explicit risk.
**Recommendation:** No change required — the coupling is sound given the shared
boundary. Just ensure the PR body's ticket→test mapping (already promised in
Resolution #1: tests 5-6 → ETCH-35) is present so ETCH-35's acceptance is
independently auditable within the combined PR.

**[MINOR] Commit-failure recovery timing not discussed**
On a `commitSession` failure, `runEnd` now appends the `session_end` event to
the wip, then retains wip + mapping (verified: `commit.go` calls `RemoveWip`/
`CleanupMapping` only after `WriteSessionRef` succeeds — so retention is correct).
But the retained wip's mtime is fresh, and the recovery sweep applies
`ReadTimeoutFromSettings` — so the orphan won't be re-committed by the next
`session_start` until the recovery timeout elapses. That's acceptable (eventual
recovery), but it's an unstated implication of "recovery can still commit later."
**Recommendation:** Add one sentence noting recovery of a commit-failed session
is *deferred until the recovery timeout*, so reviewers/operators don't expect
immediate re-commit on the next session. No code change implied.

## 4. Positive Observations

- **Correct root-cause diagnosis, verified.** Every concrete claim checks out
  against source: `findRepoRoot()` returning `os.Getwd()` (`common.go:39`), the
  swallowed error at `session_end.go:62-64`, the `EnsureDirs`-before-detection
  ordering in `session_start.go`, and the post-success cleanup ordering in
  `commit.go` that makes wip/mapping retention on failure correct by
  construction. The plan is grounded in the actual code, not a guess.
- **Right remedy, well-argued.** Choosing visible error over silent no-op is
  tied directly to the run's validation gate, and the rationale for short-
  circuiting *all six* hooks (not just the three named in the task) is sound —
  any mid-session hook in a non-git dir also drops data.
- **Detect-before-write is the correct chokepoint.** Resolving context first and
  failing before `EnsureDirs`/`WriteMapping` directly eliminates the task's
  ".etch/ pollutes the directory" and "orphaned .wip" impacts.
- **Generalizes the fix beyond the trigger.** De-swallowing `commitSession`
  covers any commit failure (corrupt object store, permissions, disk full), not
  just non-git — addressing the deeper bug the task surfaced.
- **Strong, adversarial test matrix.** Tests 5-7 map cleanly to the acceptance
  criteria, the regression guard for the refuted stop-after-end finding is
  explicit, and the sabotage mechanism for the commit-failure test was made
  deterministic (pre-created conflicting ref/lock) rather than flaky.
- **Boundary discipline.** The plan explicitly preserves the three REFUTED
  findings (worktree diff anchoring, stop-after-end `printOK`, index races),
  which is exactly the kind of scope hygiene that prevents a fix from
  regressing adjacent behavior.
