# Plan Review: ETCH-46 — etch doctor

## 1. Verdict

**PASS** — Plan is complete, feasible, and aligned. Implementation can proceed.

## 2. Summary

Reviewed the ETCH-46 plan (new `doctor` subcommand: five base health checks plus four operator-mode checks) against the task description, the ticket's two scope-extension comments, and the actual codebase at `origin/main @ b674c82`. The plan is unusually well-grounded: every helper it proposes to reuse exists and does what the plan claims (`enable.parseConfigKey`, `listWorktrees`, `effectiveHooksDir`, `install.areClaudeHooksInstalled`, `testutil.RunBinaryWithEnv`, `config.RecoveryTimeoutHours`, `version.Version`), and the test strategy is verifiably executable with the existing harness. The only findings are minor: one deliberate deviation from the acceptance-criteria wording that should be documented, and a couple of unaddressed edge cases.

Key feasibility claims I verified directly against source:

- `testutil.WriteSession` threads `Timing.EndedAt` → `RefMeta.EndTime` → `GIT_COMMITTER_DATE` in `refs/writer.go:175`, so the stale-session test (test 4) genuinely controls the committer date that `for-each-ref --sort=-creatordate` will read.
- "Shared state root" for the wip check matches the real layout: `capture.RepoContext.StateRoot` is the parent of `--git-common-dir` (`internal/capture/repocontext.go`), and `EnsureDirs` creates `.etch/sessions/` there.
- The test binary is built under the name `entire-agent-etch` (`testutil.go:186`), so putting its directory on PATH via `RunBinaryWithEnv` makes the binary check pass deterministically, and `os.Executable()` will resolve to the same file (no version-exec needed in the healthy path). Go's `exec.Cmd` keeps the last duplicate env key, so a PATH override via the extras map works.
- The PATH binary's `info` subcommand does emit a `version` field (`internal/info/info.go` Response), so the version-mismatch comparison in check 1 is implementable as described.
- The scope extension (checks 2, 7, 8, 9) is real, not scope creep: both ticket comments explicitly fold the ENABLEMENT.md/ETCH-48 operator-mode checks into this ticket ("ETCH-48 adds these checks if doctor exists by then; otherwise they're part of this ticket" — and ETCH-48 shipped first).
- The hook-events list in `install.go` confirms exactly 5 installed events (Stop deliberately excluded), matching the plan's per-event coverage model.

## 3. Issues

**[MINOR] Design / Exit policy — Deliberate deviation from AC wording (`etch.enabled=false` exemption)**
The acceptance criteria say "non-zero when … hooks missing/partial." The plan exempts the case where `etch.enabled=false` (exit 0, info "capture disabled"). This is the right call — an explicitly disabled repo is healthy, not broken — but it is a deviation from the literal AC text.
**Recommendation:** Keep the behavior, but state the deviation explicitly in the PR description and ticket comment so the AC check at review time doesn't read it as a miss. Test 7 already covers it, which helps.

**[MINOR] Design / Checks — Behavior outside a git repo unspecified**
Doctor will be run by operators precisely when things are confusing; running it outside a git repo (or in a bare repo) should produce a clear single-line error rather than a cascade of per-check git failures. The plan doesn't mention this path.
**Recommendation:** Add an up-front "is this a git repo?" guard (reuse the `rev-parse` pattern from `capture.repoContext`) that exits non-zero with one clear message, plus a one-line test.

**[MINOR] Tests / Test 5 — `PATH=/usr/bin` assumes git lives there**
Doctor shells out to git for most checks. `PATH=/usr/bin` holds on macOS and typical Linux CI, but the test silently couples "binary not on PATH" to "git still on PATH."
**Recommendation:** Build the restricted PATH from `filepath.Dir(LookPath("git"))` rather than hardcoding `/usr/bin`, so the test asserts exactly one thing.

**[MINOR] Check 1 — Harden the version-compare exec path**
Check 1 execs the PATH-resolved binary's `info` and parses its JSON. If the PATH binary is an old/broken build (the exact scenario doctor exists to catch), the exec can fail or emit garbage. The plan doesn't say what status that yields.
**Recommendation:** Treat exec failure / unparseable output as warn with the error in detail (it still proves "a different binary is on PATH"), never as a doctor crash. Worth a sentence in the plan or a test variant.

**[MINOR] Base / Dependency note — Uncommitted in-flight schema work on the main checkout**
The main checkout currently carries uncommitted changes adding a `capture` provenance field to `schema.Session` / `capture.Session` (plus `docs/INGESTION.md`). Branching the worktree off `origin/main @ b674c82` correctly isolates doctor from this, and the file sets barely overlap — but if that work merges first, the doctor branch needs a routine rebase before its `--json` schema tests run against the merged main.
**Recommendation:** No plan change needed; just be aware at merge time. (Noting it so the delegator isn't surprised by a rebase.)

## 4. Positive Observations

- **Verified reuse, not aspirational reuse.** Every "reuse, don't duplicate" pointer names a real function that exists today with the claimed semantics. The plan to export small helpers from `internal/enable`/`internal/install` rather than reimplementing settings parsing is exactly the right altitude and avoids the classic doctor-drifts-from-installer failure mode.
- **Exit policy is stated as policy, once, up front** — hard-fail set (binary, hooks) vs warn/info set (age, refspec, stamps, hooksPath, wips) — instead of being scattered per check. This is the part of a doctor command that usually goes wrong; here it's pinned and matches the ticket.
- **The refspec check gets the public-repo posture right** ("local-only capture, correct posture for public repos" as info, never warn), which the ticket explicitly calls out and which matters for this very repo (Etch is public on GitHub with session refs kept local).
- **Test design anticipates the hard determinism problems** — PATH control via `RunBinaryWithEnv`, committer-date control via `WriteSession`, the `--warn-age 100000` counter-test, and the read-only invariant test (snapshot file tree before/after) which encodes "doctor performs zero writes" as a verifiable property rather than a comment.
- **Operator-mode checks 7–9 map one-to-one onto the documented ETCH-48 gaps** (relative `core.hooksPath` defeating self-propagation, non-shell hooks, grep-guard dedupe and its false-positive case), so doctor will diagnose precisely the failure modes the rollout already met in the wild.
- **Docs section knows where each surface lives** — including that `code/platform/etch.md` is outside this repo and gets updated post-merge — and closes the loop on the ROLLOUT.md risk line that motivated the ticket.
