# Code Review: ETCH-34 — Anchor hook state at the git repo root

## 1. Verdict

**PASS** — with one **critical process flag** that must be reconciled before merge (see Issue #1).

The ETCH-34 implementation is correct, complete, and meets every acceptance criterion in the plan, including all Cycle-1 and Cycle-2 review resolutions. It builds, vets clean, is gofmt-clean, and the full suite (including all seven adversarial gate tests) passes. The verdict is PASS on the *implementation*.

The critical caveat is that **the diff supplied to this review does not contain that implementation** — it had to be located on the unmerged feature branch. Whoever consumes this verdict must ensure the correct commit is what actually lands.

## 2. Summary

I was asked to review the ETCH-34 repo-root fix. The diff handed to the review harness contains **only** the archive time-bomb test fix (`internal/archive/archive_test.go`), a large amount of `.lattice/` orchestration churn, and a new `reviews/2026-06-04-deep-code-review.md` — **none of the actual ETCH-34 code**. The genuine implementation lives on the unmerged `fix/repo-root-batch` branch (commit `f869d9d`, one commit on top of current `main` @ `01a2ca4`). I reviewed that commit directly. It is high quality: a clean two-root (`StateRoot`/`WorkDir`) abstraction, all four hook entry points rewired, the ETCH-35 visible-failure paths implemented exactly as specified, and a genuinely adversarial test suite that varies `cmd.Dir` to prove the fix. My only blocking concern is the diff/branch mismatch itself.

## 3. Issues

**[CRITICAL] (review harness / process) — Reviewed diff does not contain the ETCH-34 implementation**
The diff in the prompt diffs `main` against a base at/around `8589c24`, so it shows the recent `main` churn (deep-review doc, lattice metadata, `archive_test.go` time-bomb fix) but **not** the repo-root fix. On `main`, `internal/hooks/common.go:39` still has the buggy `findRepoRoot()` returning raw `os.Getwd()`. The real fix is on branch `fix/repo-root-batch` @ `f869d9d` (worktree `Etch-worktrees/repo-root-batch`), which is **not** reflected in the reviewed diff. A reviewer reading only the supplied diff would conclude ETCH-34 was never implemented.
Why it matters: under this project's "auto-merge through to done" + Fully Autonomous policy, a PASS that triggers a merge of the *reviewed* ref would merge `main` (no fix) and the actual fix on `f869d9d` could be silently dropped. The work is done; the risk is purely that the wrong ref lands.
**Fix:** Before merging, confirm the PR/merge target is `fix/repo-root-batch` @ `f869d9d` (or that `f869d9d` has been pushed and the PR diff shows the 10 files: `repocontext.go`, `repocontext_test.go`, `common.go`, the four hooks, `buffer.go`, `capture_test.go`, `reporoot_test.go`). Re-point the review harness at the feature branch rather than `main` so the diff under review matches the task.

**[MINOR] internal/capture/repocontext.go:38 — extra `git rev-parse` exec per hook invocation**
`ResolveRepoContext` shells out to `git rev-parse --show-toplevel --git-common-dir`, and `CaptureGitState`/`CaptureGitEnd` independently re-resolve the same values. Every hook now pays one extra subprocess. At 60–80 concurrent agents this is measurable but small.
Why it matters: pure overhead, correctness-neutral.
**Fix:** None required here — this is explicitly deferred (Cycle-1 resolution #6) to the `gitexec` consolidation backlog. Noted only for completeness.

**[MINOR] internal/hooks/session_end.go:64-77 — commit-failure recovery is delayed by `recovery_timeout_hours`**
On `commitSession` failure the wip + mapping are correctly retained (verified by `TestCommitFailureVisibleAndRecoverable`), but the retained wip has a fresh mtime, so the next `session_start` recovery sweep won't re-commit it until `recovery_timeout_hours` (~4h default) elapses. There is a multi-hour window where a normally-ended session has no committed ref.
Why it matters: data is not lost, but availability is delayed; additionally, when recovery *does* pick it up, it runs through the divergent recovery reconstruction path (deep-review finding #9 / ETCH-40), so the eventual record may be lossy (fabricated `git_end`, null tokens).
**Fix:** Accepted and documented as "eventual recovery is the contract" (Cycle-2 resolution #13); the lossy-reconstruction half is ETCH-40 scope. No code change in this PR — flagging so the residual window is a known, not a surprise.

**[MINOR] (validation gate, not code) — exit-1-under-real-Entire is unproven by the test suite**
The ETCH-35 design returns a non-zero exit + `{"ok":false}` for non-git/commit-failure. The Go tests prove the binary's behavior, but whether the **real Entire CLI** treats a non-zero `session_start` exit as fatal (aborting the agent session) is asserted, not demonstrated. `make smoke` exercises only the happy path.
**Fix:** Per Cycle-2 resolution #12, the validation phase must run the non-git case against real Entire hook dispatch and confirm the agent session survives; if it doesn't, fall back to loud-stderr clean no-op. This is a go/no-go for the validate phase, not a blocker for the code review.

## 4. Positive Observations

- **The two-root abstraction is exactly right.** Splitting the conflated `repoRoot` into `StateRoot` (common-dir parent — anchors `.etch`, settings, recovery, refs) and `WorkDir` (`--show-toplevel` — anchors git capture/diffs) is the cleanest possible resolution. It deliberately preserves the "diff runs against the session's own checkout" property, keeping the previously-refuted worktree-diff finding refuted. `RepoContext`'s doc comment clearly states the rationale and the documented submodule limitation.
- **Resolution logic reuses the production helper.** `ResolveRepoContext` resolves `--git-common-dir` relative to the CWD via the existing `resolvePath` and `filepath.Dir`, mirroring `gitstate.go`'s `RepoRoot` computation exactly — no second, drifting implementation of the same git-version-sensitive logic.
- **Defensive output parsing.** The two-non-empty-line guard correctly rejects bare repos and degenerate states (`TestResolveRepoContextBareRepo`), and the error wraps git's own stderr so "git missing" is distinguishable from "not a repo" (Cycle-1 #2, #5 honored).
- **The test suite is genuinely adversarial, not smoke.** All seven planned tests are present and *vary `cmd.Dir`* — the exact axis the bug lived on. `TestHooksFromSubdirsProduceOneRecord` proves one coherent record (prompt + tool counts survive, no scattered `.etch`); `TestHooksFromLinkedWorktree` proves state at the main root *and* worktree-aware git capture (`is_worktree`, `worktree_path`, branch, a real worktree commit in `files_touched`); `TestNonGitDirAllHooksFailVisible` covers all six hook subcommands; `TestCommitFailureVisibleAndRecoverable` uses the deterministic nested-ref sabotage (Cycle-1 #4) and asserts wip+mapping retention; `TestStopAfterSessionEndStaysOK` guards the refuted finding. The macOS symlink normalization (`realPath`/`EvalSymlinks`) is a thoughtful touch that prevents flaky `/var`→`/private/var` failures.
- **The detect-first chokepoint is correctly ordered.** Every hook calls `resolveContext()` before any filesystem write, so a non-git invocation creates no `.etch` and orphans nothing (`assertNoEtch` confirms). The stdout/stderr/exit contract (`{"ok":false}` on stdout, warning + `error:` on stderr, exit 1) is implemented precisely once via `printNotOK` + `main.go`'s single error path — no double-printing.
- **Clean blast radius.** `findRepoRoot` is fully deleted, the single `Finalize` caller is updated with the new `(stateRoot, workDir, sessionID)` signature, and the existing `capture_test.go` callers are migrated. Build, vet, gofmt, and the entire test suite are green.
