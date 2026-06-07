# Plan Review: ETCH-18

## 1. Verdict

**FAIL (plan-level)**

## 2. Summary

I reviewed the plan for ETCH-18 (`setup-refspec` reports success against a phantom `origin` remote that has no URL in a fresh repo). The "Plan" section is a **verbatim copy of the task description** — it contains no implementation approach, no decision on which of the two expected behaviors to adopt, no list of files to touch, and no test plan. There is nothing here to implement against, so the task must return to `in_planning`.

## 3. Issues

**[CRITICAL] Plan (lines 17–19) — The plan is just the task description copied back**
The entire "Plan" body is identical to the "Task Description" body. It restates the problem but proposes zero solution: no described code change, no chosen strategy, no acceptance criteria, no test cases. A reviewer cannot assess feasibility or completeness of an approach that does not exist, and an implementer would have to invent the whole design from scratch. This alone forces a FAIL.
**Recommendation:** Write a real plan. The task itself names two acceptable strategies — the plan must pick one (or define the precedence between them) explicitly:
- **Option A (no-op + guidance):** Before writing any refspec, check `git config --get remote.origin.url`. If empty/absent, print a clear message such as `no "origin" remote with a URL found; add one (git remote add origin <url>) then re-run` and exit non-zero (or exit 0 but with an unambiguous "nothing configured" message — decide and state which).
- **Option B (configure against whatever remote exists):** Enumerate remotes via `git remote`; if exactly one exists, target it; if none exist, fall back to Option A's message; if multiple exist, decide behavior (target `origin` if present, else error asking which).

**[CRITICAL] Plan — No decision on exit-code / success-message semantics**
The bug is fundamentally that the command prints `etch refspec configured for push and fetch` and exits 0 when nothing usable was configured. The plan must explicitly state the new contract: what is printed and what the exit code is in each branch (no remote, remote without URL, remote with URL, already-configured). This is the crux of the ticket and is entirely unaddressed.
**Recommendation:** Define the precise output/exit matrix. Suggested: success message only printed when a real remote URL exists; otherwise a distinct guidance message. Decide whether "no usable remote" is an error (non-zero) or an informational no-op (zero) — the task accepts either, but the plan must commit to one.

**[MAJOR] Plan — No files identified**
The relevant code is `internal/commands/setup_refspec.go` (`RunSetupRefspec` / `addRefspecIfMissing`), dispatched from `cmd/entire-agent-etch/main.go:50`. The plan names none of these.
**Recommendation:** List `internal/commands/setup_refspec.go` as the file to modify and note that the dispatch in `main.go` already exists and is unchanged.

**[MAJOR] Plan — No test plan, violating project testing policy**
CLAUDE.md states "Every ticket ships with tests. No exceptions." There is currently **no test file** for `setup_refspec.go`. The plan defines no tests for the fresh-repo scenario, the bare-`origin`-without-URL scenario (the exact reported bug), the happy-path scenario (real remote URL present), or the idempotent re-run scenario.
**Recommendation:** Add a `setup_refspec_test.go` covering at minimum: (1) `git init` only, no remote → guidance message, no refspec written; (2) `origin` added with a URL → refspec written for push and fetch, success message; (3) re-run when already configured → idempotent, no duplicate entries; (4) the reported repro — verify `git config --get-all remote.origin.fetch/push` does NOT contain the etch refspec when there is no origin URL. Use the `NewTestRepo()` helper described in CLAUDE.md.

**[MINOR] Plan — Edge cases unaddressed**
Several edge cases need a stated position: a remote named something other than `origin`; multiple remotes; a remote that exists in `git remote -v` but with no URL (the literal repro); and whether partial state from a prior buggy run should be cleaned up (the bug already writes `remote.origin.fetch`/`push` entries — should the fix remove these orphaned entries, or leave them?).
**Recommendation:** State explicitly how each is handled, especially whether to clean up the orphaned `remote.origin.{fetch,push}` entries a previously-buggy invocation may have left behind.

## 4. Positive Observations

The underlying task description is excellent: it provides a crisp reproduction, the exact diagnostic commands (`git config remote.origin.url`, `git remote -v`), the observed-vs-expected behavior, and two concrete acceptable resolutions. The bug is real and the existing code confirms it — `addRefspecIfMissing` in `internal/commands/setup_refspec.go` writes `remote.origin.{push,fetch}` and prints unconditional success without ever checking that `origin` has a URL. A plan built on this description should be quick to author; the only failing here is that the plan was never actually written.
