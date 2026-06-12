# Plan Review: ETCH-47 — etch enable/disable + fast-exit guard

### 1. Verdict

**PASS** — Plan is complete, feasible, and aligned with both the ticket and `docs/ENABLEMENT.md`. Implementation can proceed. The issues below are minor hardening notes the implementer should fold in; none require a planning round-trip.

### 2. Summary

Reviewed the ETCH-47 plan (new `internal/enable` package: `RunEnable`, `RunDisable`, `Disabled` guard; main.go dispatch + gating of all six hook subcommands; nine-item test plan) against the ticket, the ENABLEMENT.md spec, SPEC AC #13, and the current code (`cmd/entire-agent-etch/main.go`, `internal/hooks/common.go`, `internal/install/install.go`, `internal/capture/gitstate.go`). The plan is tight, correctly scoped against ETCH-48, maps one-to-one onto the acceptance criteria, and explicitly carries the spec's subtle points (compatibility rule, one-spawn ceiling, byte-preserving exclude discipline). The only substantive concern is a behavioral edge the plan asserts but doesn't test for: exiting 0 *without reading stdin* can EPIPE the dispatching parent.

### 3. Issues

**[MINOR] Design §3 (fast-exit guard) — Exiting without reading stdin may EPIPE the dispatcher**
The guard exits 0 "no stdin read." The dispatching parent (Claude Code / Entire) writes the JSON payload to the child's stdin pipe; if the process has already exited, that write fails with EPIPE, and some dispatchers surface a failed-write as a hook error — which would make the *disabled* path noisier than the enabled one. Today's handlers always `io.ReadAll(os.Stdin)` (`internal/hooks/common.go:79`), so this failure mode has never been exercised.
**Recommendation:** Either cheaply drain stdin before exiting (an `io.Copy(io.Discard, os.Stdin)` is microseconds for these payloads and keeps the 50 ms budget intact), or add a binary-level test that pipes a payload through a shell pipeline exactly as the hook entries do and asserts the writer side doesn't error. Decide deliberately rather than inherit whatever the runtime does.

**[MINOR] Design §3 (short-circuit 1) — Filesystem walk-up can disagree with git's own repo discovery**
The zero-spawn walk-up for a `.git` entry is a good optimization, but it is a second, independent repo-discovery implementation alongside `capture.ResolveRepoContext` (which delegates to `git rev-parse`). Divergence cases: `GIT_DIR` set in the environment, a `.git` entry that exists but isn't a usable repo, or `ceiling`-style oddities. None of these occur under Claude Code hook dispatch (cwd = worktree root, no `GIT_DIR`), so this is acceptable — but the asymmetry should be a conscious choice.
**Recommendation:** Treat the walk-up as a *negative-only* short-circuit (no `.git` found anywhere up to root → disabled), and let the subsequent `git config` spawn be authoritative for everything else. Document that contract in a comment on `Disabled()`. That's what the plan appears to intend; make it explicit.

**[MINOR] Design §3 — Undefined guard behavior for a malformed `etch.enabled` value**
`git config --get --type=bool etch.enabled` exits non-zero both when the key is absent (exit 1, empty output) and when the value can't be canonicalized to bool (e.g. someone hand-writes `etch.enabled = maybe`). The plan defines absent → enabled and `false` → disabled, but not the malformed case. Conflating "absent" with "malformed" via a bare exit-code check would silently fail-open, which is probably the right outcome but should be chosen, not accidental.
**Recommendation:** State the rule: anything other than a clean `false` → enabled (fail-open, consistent with "key absent = enabled" and with capture-must-not-break-sessions). One test with a garbage value locks it in.

**[MINOR] Design §1 (RunEnable) — Common-dir path handling details**
`git rev-parse --git-common-dir` returns a *relative* path (`.git`) when run from the main checkout root, and `<commonDir>/info/` may not exist in minimal repos. Both are one-line fixes but classic trip-ups.
**Recommendation:** Absolutize the common dir against the directory the rev-parse ran from (mirror whatever `gitstate.go` already does at `internal/capture/gitstate.go:18`), and `MkdirAll` the `info/` directory before writing `exclude`. Test 3 (enable from inside a worktree) will catch the relative-path bug only if the assertion checks the file's real location — make it do so.

**[MINOR] Tests §9 — Latency assertion robustness**
Asserting p99 ≤ 50 ms via repeated binary spawns is the right measurement, but a hard p99 assert against a wall-clock budget can flake on a loaded machine, and a flaky latency test gets skipped or deleted — losing the regression guard entirely.
**Recommendation:** Keep the measurement and the report-to-ticket (acceptance #4 demands it), but consider asserting against a budget with headroom logic (e.g., fail only if p99 > 50 ms across a retry, or assert the median strictly and p99 with one re-run allowed). Whatever the choice, print the full distribution so the ticket review gets real numbers, not just pass/fail.

**[MINOR] Tests §4 — "Both dispatch modes" is satisfied by construction; say so**
Acceptance #2 requires capture to stop under *both dispatch modes*, but operator-mode stamps don't exist until ETCH-48 — in ETCH-47 both modes reduce to "the binary gets invoked and the guard fires," which tests 4 and 6 cover. That's correct, but a reviewer of the PR may flag the apparent gap.
**Recommendation:** Add one sentence to the test plan (or PR description) noting that dispatch-mode coverage is binary-side and mode-agnostic in this ticket, with stamp-dispatch tests landing in ETCH-48. Optionally have test 4 invoke the hook through an `sh -c`-wrapped command shaped like a committed entry to make the claim concrete.

### 4. Positive Observations

- **The spec's traps are all pre-caught.** The compatibility rule (key absent = enabled), the one-spawn ceiling, `etch.enabled=false` winning over everything, and config landing in the *common* dir so all worktrees share it — each appears verbatim in the design, not just in the test list. This is a plan written after actually reading ENABLEMENT.md.
- **The behavior change is declared, not smuggled.** Hooks outside a git repo currently print not-ok and exit 1 (`internal/hooks/common.go:131-144`); the plan explicitly states the guard now owns that path and exits 0 silently, per spec. Calling out a behavior change in the plan is exactly where it belongs.
- **Scope discipline against ETCH-48 is clean.** Stamping, post-checkout propagation, dedupe, and disable's cleanup are explicitly fenced out, and `RunDisable` is noted as deliberately minimal. No scope creep, and the seam to part 2 is well-defined.
- **Reuses established codebase discipline.** The marker-delimited, byte-preserving block treatment for `info/exclude` mirrors `install.go`'s settings.json handling — consistent conventions rather than a new pattern.
- **The test plan maps one-to-one onto the acceptance criteria**, including real `git worktree add` worktrees (not simulated ones) and the team-mode no-key compat case, with the latency measurement reported, not just asserted. Pinning the base commit (36c6121) and worktree path is good orchestration hygiene.
