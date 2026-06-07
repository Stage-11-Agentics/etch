# Plan Review: ETCH-34

### 1. Verdict

**PASS** — Plan is complete, feasible, and aligned. Implementation can proceed.

### 2. Summary

Reviewed the "Repo-Root Batch" plan that fixes `findRepoRoot()` returning raw
`os.Getwd()` instead of the git toplevel. I verified every structural claim against the
actual codebase (`internal/hooks/common.go:39`, all four hook entry points,
`capture.Finalize`'s conflated `repoRoot` at `buffer.go:236`, the swallowed commit error
at `session_end.go:62-64`, `main.go`'s error→exit-1 contract, and the recovery path).
The plan is technically sound, the root cause is correctly identified and empirically
verified, and the two-roots design (`stateRoot` vs `workDir`) is the right shape. The
only items worth flagging are a deliberate scope expansion beyond the single task under
review and a handful of minor edge cases the plan does not yet mention.

### 3. Issues

**[MINOR] Scope / "Boundary-of-scope" — Plan bundles three tickets into one PR for a single-ticket review**
The task under review is ETCH-34 (subdir `.etch` placement). The plan also implements
ETCH-35 (non-git silent failure) and touches ETCH-40 finding 2. This is defensible —
ETCH-34 and ETCH-35 share the exact same root cause and fix site (`findRepoRoot` →
context resolution), so splitting them would be artificial — but it means the resulting
PR and the result-validation pass will cover more than the ETCH-34 acceptance criteria.
The plan already correctly guards "never change ETCH-40 status," which is the main risk.
**Recommendation:** Keep the batch, but make the PR description and validation plan map
each test (1–7) to its owning ticket explicitly, so the validator can confirm ETCH-34's
criteria independently of the ETCH-35 work riding along.

**[MINOR] ETCH-35 error path — "not inside a git repository" message will mislead when git is simply unavailable**
`ResolveRepoContext` maps any non-zero `git rev-parse` exit to a "not a git repository"
error. If `git` is missing from `PATH` or the exec itself fails (not a `fatal: not a git
repository` exit), the user-facing stderr line will wrongly claim the directory isn't a
repo. For a tool whose whole premise is "pure git plumbing," conflating "git can't run"
with "not in a repo" is a small but real diagnostic trap.
**Recommendation:** Distinguish an exec/launch failure from a clean non-zero git exit in
the helper, or soften the message to "could not resolve a git repository (cwd=…): …"
including the underlying git stderr.

**[MINOR] Migration gap — pre-existing scattered `.etch` state becomes permanently unrecoverable after the fix**
Sessions that already crashed under the buggy `findRepoRoot` left `.wip` buffers in
per-subdir `.etch/` dirs. After this fix, `session_start` recovery scans only
`stateRoot/.etch/sessions`, so those legacy orphans are never swept and the stale subdir
`.etch` dirs linger. This is almost certainly acceptable (Etch is pre-release and
dogfood-only), but the plan doesn't acknowledge it.
**Recommendation:** Add one line stating this is accepted (pre-release, no migration), or
note a one-time manual `find . -name .etch` cleanup for already-dogfooded repos. No code
change required — just make the decision explicit so it isn't mistaken for an oversight.

**[MINOR] Test 6 (commit-failure visibility) — read-only `.git` is a flaky sabotage mechanism**
The plan offers two ways to force a commit failure: make `.git` read-only, or pre-create
a conflicting ref lock. The read-only approach is platform- and privilege-dependent (a
test runner as root bypasses it) and can interfere with the test's own cleanup. The ref
lock is deterministic and self-contained.
**Recommendation:** Prefer pre-creating `refs/etch/sessions/<ulid>.lock` (or an
equivalent ref-write conflict) as the primary sabotage; drop the read-only variant or keep
it only as a documented fallback.

**[MINOR] Helper robustness — single multi-flag `rev-parse` assumes exactly two clean output lines**
The design relies on `git rev-parse --show-toplevel --git-common-dir` emitting two
parseable lines in flag order. In degenerate states (bare repo, detached/odd checkouts)
`--show-toplevel` can be empty, yielding fewer lines than expected. The plan scopes
submodules and bare repos out, but a defensive guard is cheap.
**Recommendation:** Validate the line count / non-empty toplevel before indexing, and
return the same "not a usable repo" error rather than panicking or silently producing an
empty `WorkDir`.

**[MINOR] Cosmetic duplication — `ResolveRepoContext` and `CaptureGitState` both compute toplevel + common-dir parent**
After the fix, both `ResolveRepoContext` and the existing `CaptureGitState`
(`gitstate.go:17-38`) shell out to resolve the same two values. Not a correctness problem
— they serve different call sites — but it's redundant git exec per hook invocation.
**Recommendation:** Optional. If trivial, have `CaptureGitState` accept or reuse the
already-resolved `WorkDir`/common-dir rather than re-running `rev-parse`. Safe to defer.

### 4. Positive Observations

- **Root cause correctly identified and empirically verified.** The plan pins the bug to
  `common.go:39` and confirms the fix in `/tmp/etch-subdir`. I independently confirmed the
  conflation extends into `capture.Finalize` (`buffer.go:236` runs `gitDiffFiles(repoRoot,
  …)` against the same root used for state) — the plan caught this and correctly splits
  `Finalize(stateRoot, workDir, sessionID)`.
- **The two-roots model is the right abstraction.** Separating `stateRoot` (state,
  settings, refs, recovery) from `workDir` (git capture/diffs) is precisely what's needed
  to fix CWD drift without regressing worktree diff correctness, and it reuses the
  established common-dir-parent logic already living in `gitstate.go`.
- **It actively guards the REFUTED findings.** Explicitly preserving the by-design
  mapping-miss `printOK()` and keeping diffs anchored to the invoking checkout prevents a
  reviewer from "re-fixing" non-bugs — an unusually mature touch that saves a future churn
  cycle.
- **Worktree design choice is thoroughly justified.** Anchoring `.etch` at the main root
  (CWD-drift immunity, recovery-after-worktree-deletion, uniform gitignored settings,
  shared refs, ULID-keyed concurrency safety) is well-reasoned and matches the ephemeral-
  worktree orchestration pattern this project actually uses.
- **Test matrix is adversarial and gate-mapped.** Tests vary `cmd.Dir` to exercise the
  exact failure (subdir, worktree, worktree subdir, non-git), and the recovery test is
  sound — I confirmed recovery reconstructs from captured wip events
  (`recovery.go:553-562`) rather than re-running live git, so a deleted-worktree orphan
  recovers correctly without needing `workDir`.
- **All threading surfaces are accounted for.** Every consumer I checked — the four hooks,
  `commitSession`, `config.Load`, `RecoverAll`/`ReadTimeoutFromSettings`, and `Finalize` —
  appears in the rewire list with the correct root assignment, and the `main.go`
  error→exit-1 contract makes the ETCH-35 "loud failure" mechanism work as described.
