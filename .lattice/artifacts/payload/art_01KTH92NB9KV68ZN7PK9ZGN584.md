# Plan Review: ETCH-35 — no-git-repo silent data loss

## 1. Verdict

**FAIL (plan-level)**

## 2. Summary

I reviewed the ETCH-35 plan against the task description, the live source
(`internal/hooks/session_start.go`, `session_end.go`, `common.go`,
`commit.go`, `internal/capture/gitstate.go`), and an empirical repro in a
non-repo temp directory. The underlying bug is real and reproduces exactly as
described (verified: `session_start` exits 0 and creates `.etch/`;
`session_end` logs `fatal: not a git repository`, then still prints
`{"ok":true}` and exits 0, leaving an orphaned `.wip.jsonl` and an uncommitted
`.session.json`). The blocking problem is that **the "Plan" is not a plan** —
lines 21–23 are a verbatim copy-paste of the task description (lines 14–16).
It contains no implementation strategy, no decision between the two mutually
exclusive remedies the task itself offers, no list of files to touch, and no
test plan. There is nothing here an implementer can execute or that a reviewer
can hold the implementation against.

## 3. Issues

**[CRITICAL] Whole plan — The plan is a duplicate of the task description, not an implementation plan**
The "Plan" section reproduces the task's REPRO / ACTUAL / IMPACT / FIX text
word-for-word and adds nothing. It does not say *what code changes will be
made*, *where*, or *how the fix will be verified*. A plan review exists to
catch design decisions before code is written; there is no design here to
review. This alone makes the plan non-actionable.
**Recommendation:** Return to `in_planning` and author a real plan that names
the files to change (`internal/hooks/common.go`, `session_start.go`,
`session_end.go`, and likely a new repo-detection helper), states the chosen
behavior, and lists the tests to add.

**[CRITICAL] FIX strategy — The task offers two mutually exclusive remedies and the plan picks neither**
The task says: "detect non-repo at session_start and **either** no-op cleanly
**or** surface a non-zero/error." These are very different implementations with
different blast radii:
- *No-op cleanly* — detect non-repo, skip all `.etch/` creation and mapping
  writes, print `{"ok":true}`, exit 0. Nothing is captured (acceptable: there
  is no repo to attach refs to), nothing is orphaned, the directory stays
  clean. This is the safer default for a hook that must never disrupt the host
  agent.
- *Surface an error* — exit non-zero / emit a visible error. This makes
  misconfiguration loud, but in `main.go` any returned error becomes
  `os.Exit(1)` for **every** hook, which Entire may treat as a hook failure
  that disrupts the agent run. The plan must reason about how Entire reacts to
  a non-zero hook exit before choosing this path.
**Recommendation:** Pick one and justify it. Recommended: **no-op cleanly at
detection time** (don't create `.etch/`, don't write the mapping) **plus**
stop printing `{"ok":true}` on a genuine commit failure inside a real repo
(write the error to stderr; decide deliberately on the exit code). Document the
Entire-side consequence of whichever exit-code choice is made.

**[MAJOR] Detection point — `findRepoRoot()` is the real defect and is shared by all six hooks**
`internal/hooks/common.go:39` `findRepoRoot()` is a misnomer: it returns
`os.Getwd()` verbatim — it never verifies a git repo exists and never walks up
to a repo root. Every hook (`session_start`, `user_prompt_submit`,
`pre_tool_use`, `post_tool_use`, `session_end`, `stop`) routes through it. A
fix scoped to only `session_start` + `session_end` leaves the other four
hooks creating/appending to `.etch/` in a non-repo. The plan never mentions
this function or that the fix must be applied at this shared chokepoint.
**Recommendation:** Centralize repo detection in/near `findRepoRoot()` (e.g. a
helper using `git rev-parse --show-toplevel`, reusing the `gitOutput` pattern
already in `internal/capture/gitstate.go:17`) and have every hook short-circuit
to a clean no-op when there is no repo. Specify this in the plan.

**[MAJOR] Swallowed commit error — the `{"ok":true}` lie isn't unique to the non-repo case**
`session_end.go:62-66` logs any `commitSession` failure via `log.Printf` and
then unconditionally calls `printOK()`. The non-repo case is just the most
visible trigger; *any* commit failure inside a real repo (corrupt object store,
permissions, disk full) silently reports success and orphans the `.wip`. The
task's "don't print ok on commit failure" applies here generally. The plan
does not address this code path at all.
**Recommendation:** Have the plan specify the desired behavior when
`commitSession` returns an error in a real repo — at minimum surface to stderr
rather than only `log.Printf` + `printOK()`, and state whether the `.wip` is
retained for recovery (it should be, so the next `session_start` recovery scan
can retry).

**[MINOR] Orphaned-artifact cleanup is unspecified**
The repro leaves `.etch/sessions/<ULID>.wip.jsonl`, a finalized
`<ULID>.session.json`, and a `.map/` entry behind. If the fix is "no-op at
session_start," nothing new is created — good. But the plan should explicitly
state that pre-existing pollution in non-repos is out of scope (acceptable) and
that the no-op path must run *before* `EnsureDirs`/`WriteMapping` so no new
artifacts appear.
**Recommendation:** Add an explicit ordering note: detect → no-op return
*before* any filesystem write in `session_start`.

**[MAJOR] No test plan — violates the project's mandatory per-ticket testing rule**
CLAUDE.md states "Every ticket ships with tests. No exceptions" and points to
`testutil` helpers. The plan names no tests. This bug is trivially testable
(run the hooks in a tmpdir that is *not* a git repo and assert: no `.etch/`
created, no `.wip` orphaned, and the chosen exit/stderr behavior).
**Recommendation:** Add a test plan: a non-repo case asserting clean no-op (no
`.etch/`, correct exit code/stderr), and a real-repo commit-failure case (if
feasible to simulate) asserting the success line is no longer printed on
failure.

**[MINOR] No acceptance criteria restated as verifiable checks**
The plan provides no checklist a reviewer or the result validator can run
against. The task embeds clear criteria (no silent data loss; failure visible;
no `.etch/` pollution; no orphaned `.wip`) but the plan doesn't lift them into
testable assertions.
**Recommendation:** Enumerate acceptance criteria as concrete, checkable
statements derived from the task's IMPACT/FIX lines.

## 4. Positive Observations

The task description that was copied is itself excellent raw material: it
includes a precise repro, the exact failing call site
(`commitSession` error swallowed in `session_end.go`), the observable symptom
(`{"ok":true}` + exit 0), the impact (silent data loss + directory pollution),
and an empirically verified reproduction path (`/tmp/etch-nogit`). I confirmed
all of it reproduces exactly as stated. The bug is real, well-characterized,
and the fix is small and well-scoped once a design decision is made — this is a
strong ticket that simply needs an actual plan written on top of it. The fix
also has a natural, low-risk landing spot (`findRepoRoot()` as the shared
detection chokepoint), so revision should be quick.
