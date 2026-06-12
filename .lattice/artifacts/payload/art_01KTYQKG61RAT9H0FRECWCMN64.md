# Code Review: ETCH-46 — etch doctor: capture health check

> **Reviewer note on the diff under review:** the diff embedded in the review prompt was
> **not** the ETCH-46 implementation — it contained no `internal/doctor` at all. It was a
> snapshot of already-merged main content (CI/GoReleaser pipeline, `.lattice/` metadata,
> ETCH-48 stamping code, lessons-learned). This is the same harness failure documented in
> lessons-learned (2026-06-09 / 2026-06-12: auto-fired reviews can't resolve worktree-branch
> diffs). The actual implementation is commit `3c0956c` on branch `etch-46-doctor`
> (worktree `../Etch-worktrees/etch-46-doctor`, **PR #6**, branched off main `b674c82`
> exactly as the plan specified). This review covers the real diff `main...etch-46-doctor`
> (9 files, +1114/−13: `internal/doctor/` new, plus small exported-helper additions to
> `internal/enable`, `internal/install`, `internal/recovery`, `internal/testutil`).

## 1. Verdict

**PASS**

Verified independently, not just by reading: `go build ./...` and `go vet ./...` clean;
`go test ./internal/doctor/` green (18 tests, 112s); full suite green except one
**pre-existing** flake unrelated to this diff (see Issues). All four ticket acceptance
criteria are met with verifying tests, plus the operator-mode checks the plan added from
ETCH-48.

### Acceptance criteria

| AC | Status | Evidence |
|----|--------|----------|
| Exit 0 healthy / non-zero on hard problems (binary off PATH, hooks missing/partial); stale-age + no-refspec are warnings | ✅ | `TestHealthyTeamModeRepo`, `TestBinaryNotOnPathFails`, `TestHooksMissingFails`, `TestPartialHooksFail`, `TestStaleSessionWarns` (warn exits 0) |
| `--json` emits a structured report, a field per check | ✅ | `TestHealthyTeamModeRepo` asserts all 9 check fields present; `healthy`/`warnings` booleans per plan |
| Zero-sessions repo reports "no sessions captured yet", not an error | ✅ | `TestNoSessionsIsHealthy` |
| Temp-repo tests: healthy / hooks-missing / no-sessions / stale-newest | ✅ | All four present, plus operator-mode (stamps, propagation, dedupe, relative `core.hooksPath`), wip orphan-vs-live, disabled-repo-healthy, read-only-snapshot, broken-PATH-binary |

## 2. Summary

Reviewed the new `internal/doctor` package (477 lines, 9 checks), the main.go/usage
dispatch, and the supporting exported helpers added to enable/install/recovery. Quality is
high: the implementation matches the plan's check list and exit policy precisely, reuses
existing internals instead of duplicating them (exactly as the plan directed), is read-only
by construction with a test proving it, and goes beyond the plan in places (the wip check
distinguishes live agent sessions from true orphans via `recovery.WipAgentAlive`, so a
long-running session never false-alarms). Findings are all minor; the only plan-deliverable
gap is the `docs/ROLLOUT.md` wave-0.5 risk-line update, which should be folded in before
merge.

## 3. Issues

**[MINOR] docs/ROLLOUT.md:204 — plan's Docs deliverable not in the diff**
The plan's Docs section commits to three items: README (✅ done, good paragraph),
`code/platform/etch.md` (post-merge, separate repo — correctly deferred), and "ROLLOUT.md:
wave-0.5 risk line gets its ✅". The Risks table row "Capture silently breaks (binary moved,
hooks dropped)" still reads "`etch doctor` ticket; interim: weekly `query --since` spot
check" — and ROLLOUT.md is untouched by this branch. Same-repo, explicitly enumerated, will
be forgotten at ship time otherwise.
**Fix:** One-line edit on the branch before merge: replace the interim framing with
"✅ `entire-agent-etch doctor` (ETCH-46)".

**[MINOR] internal/doctor/doctor.go:303 — swallowed config error makes the wip warning lie about the timeout**
`settings, _ := config.Load(stateRoot)` ignores the error. `config.Load` returns
zero-value `Settings{}` (not defaults) on a malformed or unreadable `.etch/settings.json`,
so `RecoveryTimeoutHours` becomes 0 and every orphan instantly warns with "past the 0h
recovery timeout". The fail direction is safe (warn, not silence), but the message
misstates the configured timeout, and a corrupt `.etch/settings.json` is itself a health
finding doctor should surface rather than mask.
**Fix:** On `config.Load` error, fall back to `config.Defaults()` for the threshold and
append "(.etch/settings.json unreadable — using default)" to the detail.

**[MINOR] internal/doctor/doctor.go:341 — "stamps present in 0 worktree(s)" for partial hand-stamps**
In the stamps-without-key branch, the detail interpolates `stampedCount`, which counts only
*fully covered* worktrees. A hand-stamp covering a subset of events (e.g. only
`SessionStart`, as in `TestStampsWithoutKeyWarn`) yields "stamps present in 0 worktree(s)
but etch.enabled is not set" — internally contradictory wording. The status is correct;
only the count is misleading.
**Fix:** Count worktrees where `covered > 0` for this message (or just say "stamps
present"), keeping `stampedCount` for the operator-mode branches where full coverage is the
bar.

**[MINOR] internal/enable/enable_test.go:324 (pre-existing, not this diff) — latency flake materialized under full-suite load**
`go test ./...` on this branch failed `TestDisabledPathLatency` (p90 61ms vs the 50ms
budget; median 30ms) while the whole suite ran in parallel; it passes in isolation in 1.9s.
This diff doesn't touch the guard path (only renames-to-exported and new read-only helpers
in `internal/enable`), so it's the load-sensitivity the ETCH-47 plan review predicted —
now demonstrated, and CI runs `go test ./...` on shared runners, so PR #6's CI can flake on
it.
**Fix:** Out of scope for this ticket's code, but worth a follow-up (or fold into the
review-fixes PR): gate the hard assert on an isolation-friendly condition — e.g. retry the
measurement once before failing, or assert the median strictly and the tail with one re-run
allowed, per the original review's recommendation.

## 4. Positive Observations

- **Reuse over duplication, exactly as planned.** Doctor adds zero new git-parsing logic:
  `enable.RepoDirs`/`KeyState`/`ListWorktrees`/`EffectiveHooksDir`/`HasPostCheckoutBlock`,
  `install.EtchEntries`/`EventNames`, and `recovery.WipAgentAlive` are thin exported
  wrappers over existing internals. `KeyState` keeps the zero-spawn config scan with the
  git fallback for exotic configs — the ETCH-47 discipline carried forward intact.
- **The wip check is smarter than the ticket asked for.** Instead of "count wips and warn
  on age", it probes each buffer's recorded agent PID (start-time-verified, so PID reuse
  can't fool it): live long-running sessions never warn, and the orphan warning is judged
  against the *configured* recovery timeout. `TestOrphanWipWarnsLiveWipDoesNot` proves both
  arms, using the PID-reuse signature trick documented in lessons-learned.
- **Read-only is proven, not asserted.** `TestDoctorIsReadOnly` fingerprints every file
  (path, size, mtime — `.git` and `.etch` included) before and after both output modes.
- **PATH determinism is handled properly.** Every test pins PATH via `RunBinaryWithEnv`,
  with `gitOnlyPath` derived from where git actually lives rather than assuming `/usr/bin`
  — the binary check asserts exactly one thing on both dev machines and CI. The new
  `testutil.BinaryPath` export is the minimal enabler.
- **`TestBrokenPathBinaryWarns` covers the exact field scenario doctor exists for:** a
  stale/broken build shadowing the real one on PATH — doctor warns with the facts and never
  crashes, exit 0.
- **Exit policy matches the ticket to the letter**, including the subtle cases: explicit
  `etch.enabled=false` turns missing hooks into info ("coverage is moot") and the repo is
  healthy (`TestDisabledRepoIsHealthy`); no-refspec is info with the "correct posture for
  public repos" framing; version mismatch is warn (hooks will use the PATH build), only
  *absence* is fail.
- **Human output is operator-grade:** stable row order (`checkOrder` fixes the map
  iteration), glyphs per status, and every warn/fail detail names the remediation command
  (`re-run etch enable`, `install-hooks`, `stamp-worktree`).
