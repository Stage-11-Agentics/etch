# Code Review: ETCH-47 — etch enable/disable + fast-exit guard

> **Reviewer note on the diff under review:** the diff embedded in the review prompt was the
> ticket-creation/docs commit (`.lattice/` metadata, `docs/ENABLEMENT.md`, `docs/ROLLOUT.md` —
> i.e. the content of `36c6121` already on main), **not** the implementation. The actual
> implementation exists on branch `etch-47-enable-disable` (commit `a8b9fd9`, worktree
> `../Etch-worktrees/etch-47-enable-disable`, pushed to origin). This review covers the real
> implementation diff `main...etch-47-enable-disable` (899 insertions across
> `internal/enable/`, `cmd/entire-agent-etch/`, `internal/hooks/reporoot_test.go`).
> The review harness should be checked for why it snapshotted the wrong diff.

## 1. Verdict

**PASS**

Verified independently, not just by reading: `go build ./...` clean; full test suite green
(`internal/enable` 33.6s, `internal/hooks` 113.7s, all other packages ok); measured
disabled-path latency **median 5.57ms / p90 5.83ms / max 6.14ms over 50 runs** — comfortably
inside the 50ms SPEC AC #13 budget even at the max, let alone p99.

## 2. Summary

Reviewed the ETCH-47 implementation: new `internal/enable` package (enable/disable subcommands,
`HooksDisabled()` fast-exit guard), main.go dispatch + `runHook` gating of all six hook
entrypoints, and the deliberate flip of the ETCH-35 fail-visible contract for non-git dirs.
Quality is high — the implementation exceeds the plan (zero-spawn common path via a manual
config-file parse, with a principled fallback to `git config` for include directives and
`GIT_DIR` overrides), test coverage maps one-to-one onto the plan's nine test cases plus extras,
and every acceptance criterion is met with a verifying test. Only minor issues found; none block
merge.

### Acceptance criteria

| AC | Status | Evidence |
|----|--------|----------|
| (1) enable → key set + excludes written, idempotent | ✅ | `TestEnableSetsKeyAndWritesExcludes` (also verifies excludes *function* via `git status`), `TestEnableIsIdempotent` (byte-identical **and** mtime untouched), `TestEnableRefreshesStaleBlock` |
| (2) disable stops all capture, main + worktrees | ✅ | `TestDisableStopsAllCaptureIncludingWorktrees` — real `git worktree add`, committed hooks present, all 6 events, asserts no refs/no wip/no output |
| (3) team mode, no key → captures unchanged | ✅ | `TestTeamModeWithoutKeyStillCaptures` — full start→prompt→end sequence, exactly 1 ref |
| (4) disabled-path latency in budget — measured | ✅ | `TestDisabledPathLatency`: median 5.57ms, p90 5.83ms, max 6.14ms (n=50) vs 50ms budget |
| (5) testutil temp repos incl. worktree cases | ✅ | All binary-level tests use `testutil.NewTestRepo`/`RunBinary`; two real-worktree tests |

## 3. Issues

**[MINOR] internal/enable/enable.go:100 — `.git`-as-symlink resolves to silent capture loss**
`findCommonDir` uses `os.Lstat`, so a `.git` that is a *symlink to a directory* (a setup git
itself supports) reports `IsDir() == false`, gets treated as a gitfile, and `os.ReadFile` on the
directory fails → `HooksDisabled()` returns true → capture silently stops. Silent capture loss
is exactly the failure mode this project's ROLLOUT doc worries about most. Rare setup, but cheap
to fix.
**Fix:** Use `os.Stat` (follows symlinks) for the `IsDir()` decision; reading `commondir`
through the symlinked path works unchanged.

**[MINOR] internal/enable/enable.go:72 — fast path can't see `etch.enabled=false` set outside the common config**
The zero-spawn parse reads only `<commonDir>/config`. A `false` set in global (`~/.gitconfig`),
system, or `config.worktree` (under `extensions.worktreeConfig`) is invisible to the fast path
— it reports "key absent → enabled" — while the `git config` fallback (taken only when includes
are present) *would* honor it. ENABLEMENT.md promises "`etch.enabled = false` is an explicit
off-switch that wins over everything"; a machine-wide `git config --global etch.enabled false`
is a plausible operator move and would be silently ignored. Within designed usage (etch only
ever writes local scope) behavior is correct, so this is a documented-semantics gap, not a bug.
**Fix:** Cheapest: one line in ENABLEMENT.md and the usage text stating the switch is per-repo
local config only. Alternatively, also scan global/system config files on the fast path (still
zero spawns), or treat "key absent locally" as not-clean — but that reintroduces a spawn per
event and is probably not worth it.

**[MINOR] internal/enable/enable.go:158 — section header with trailing comment misses the match**
Git permits comments after a section header (`[etch] # managed`). The lowercased-whole-line
comparison against `"[etch]"` fails for that form, so a hand-edited `enabled = false` underneath
is missed and the fast path fails *open* (capture continues despite an explicit false). Fail
direction is the safe one, but it contradicts "false wins over everything" for hand-edited
configs.
**Fix:** Strip `#`/`;` comments after the closing `]` before comparing, or return
`clean = false` for any section-header line that isn't an exact match after trimming.

**[MINOR] internal/enable/enable_test.go:285-288 — plan said p99 assertion; test asserts median + p90**
The plan committed to "assert p99 ≤ 50 ms"; the test hard-asserts median and p90 only, with a
comment arguing the tail measures the parallel test scheduler. The reasoning is sound and the
measured **max** (6.1ms) is 8× inside the budget, so the relaxation has no practical effect
today — but it's a deviation from the written plan worth acknowledging in the PR.
**Fix:** Either keep as-is and note the deviation in the PR body, or assert max ≤ 50ms with a
generous comment (at 6ms measured max there's huge headroom).

## 4. Positive Observations

- **Implementation exceeds the plan's performance design.** The plan budgeted one `git config`
  spawn per disabled event; the implementation achieves **zero spawns** on the common path
  (manual gitfile/`commondir`/config-file resolution) while keeping correctness via an explicit
  `clean` flag that falls back to real git for include directives and `GIT_DIR` overrides. The
  5.6ms median is mostly process spawn of the binary itself.
- **The stdin drain in `runHook` (main.go:97) is a thoughtful deviation.** The plan said "no
  stdin read," but draining to `io.Discard` keeps the dispatcher from ever seeing EPIPE on its
  payload write — the contract stays uniform with the enabled path, and the comment explains
  exactly why. This is the kind of deviation a reviewer wants to find documented in place.
- **Fail-open semantics are consistently reasoned and tested.** Malformed values
  (`TestMalformedEnabledValueFailsOpen`), absent keys, and unreadable configs all resolve to
  "capture" — the right default for a key only etch writes — and `gitConfigBool` matches git's
  own false-set (`false/no/off/0`).
- **Tests verify behavior, not implementation.** `TestEnableSetsKeyAndWritesExcludes` doesn't
  just grep the exclude file — it writes the operator-mode files and asserts `git status`
  actually hides them while preserving the `.etch/settings.json` carve-out.
  `TestEnableIsIdempotent` checks mtime to prove the no-op rerun never rewrites the file
  (the byte-preserving discipline from install.go, honored exactly).
- **The ETCH-35 contract flip is handled with care:** `TestNonGitDirAllHooksFastExit` rewrites
  the old fail-visible gate with a comment chain (ETCH-35 → ETCH-47, ENABLEMENT.md) explaining
  ownership transfer, and explicitly notes loud failure remains the contract for in-repo errors.
- **Internal parser tests are thorough:** 12 table cases covering subsections, case folding,
  last-assignment-wins, bare keys, quoted values, trailing comments, and both include-directive
  forms forcing the fallback.
