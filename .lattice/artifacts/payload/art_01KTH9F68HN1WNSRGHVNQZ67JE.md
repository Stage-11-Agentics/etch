# Plan Review: ETCH-16 (refspec/sync batch)

### 1. Verdict

**PASS** — The plan is technically sound, empirically grounded, and well-decomposed. The core ETCH-16 fix is correct and verified. Two MAJOR issues (a SPEC acceptance-criterion conflict introduced by the bundled ETCH-18 selection logic, and an under-specified conditional rule that will emit a spurious notice on rerun) should be resolved during implementation, but neither requires a return to planning — both are addressable additively without redesigning the core fix.

### 2. Summary

I reviewed the four-ticket refspec/sync batch plan (ETCH-16 primary, with ETCH-18/24/38, ETCH-22 subsumed) against the live codebase (`internal/commands/setup_refspec.go`, `cmd/entire-agent-etch/main.go`, `README.md`, `internal/testutil`, `SPEC.md`). I independently reproduced the ETCH-16 bug (bare `git push origin` pushed only the etch ref, branch silently stayed local) and validated the proposed `HEAD`-augmentation fix (bare push then carried both the branch and the etch ref; detached HEAD failed loudly as claimed). The plan's root-cause analysis of git's push/fetch config asymmetry is accurate. The key concern is that the bundled remote-selection logic (ETCH-18) can regress SPEC acceptance criterion 5 (refs push to **both** Forgejo and GitHub) without acknowledging it.

### 3. Issues

**[MAJOR] Design (ETCH-18 + ETCH-38) — multi-remote sync (SPEC criterion 5) is unaddressed and may regress**
SPEC acceptance criterion 5 reads "Session refs push to Forgejo **and** GitHub via configured refspecs." The plan's selection logic configures exactly one remote and, per test matrix item 5, **fails non-zero when multiple usable remotes exist and none is named `origin`**. For the exact dual-remote scenario the SPEC calls out, no-flag `setup-refspec` now errors out — arguably worse for that user than today's behavior. Even when `origin` exists, only `origin` gets the etch refspec; the second remote (e.g. `github`) is never configured. Git's model means a single `git push` only reaches one remote anyway, so dual-sync needs either per-remote reruns, a multi-push-URL remote, or explicit documentation. The plan never mentions criterion 5, so this reads as a silent scope decision on a documented acceptance criterion.
**Recommendation:** Explicitly state the multi-remote stance in the plan. Minimum: document that `setup-refspec --remote <name>` must be rerun per remote to sync to both Forgejo and GitHub, and add a test/README note for it. If dual-remote is out of scope for this batch, say so and confirm criterion 5 is tracked elsewhere — don't leave the "fail on multiple remotes" path silently colliding with the SPEC.

**[MAJOR] Design decision (ETCH-16) — conditional `HEAD`/notice rule is under-specified; spurious notice on rerun**
Line 46 gates the behavior on whether "the remote **already has** non-etch push refspecs." After a first successful run the push list is `[etch, HEAD]` — and `HEAD` *is* a non-etch push refspec. Taken literally, the second (idempotent) run would see a "non-etch refspec" and print the "your existing refspecs remain authoritative" notice on every rerun of an etch-only config. Test matrix item 3 only asserts the final entries (`[etch, HEAD]`), not the output, so it would not catch this. The decision to **add `HEAD`** and the decision to **print the caveat notice** are two different conditions and the plan conflates them.
**Recommendation:** Specify two precise, separate rules: (a) add `HEAD` **iff the push list was empty before this run** (clean for fresh add, idempotent on rerun, never added when a user push refspec pre-exists); (b) print the "existing refspecs remain authoritative" notice **only when a push refspec other than the etch refspec and other than the etch-managed bare `HEAD` exists**. Add an assertion on stdout (no spurious notice) to test 3, and assert the notice text in test 8.

**[MINOR] Design decision (ETCH-16) — repo-wide behavior change: bare push now auto-creates remote branches**
The `HEAD` refspec makes bare `git push` behave like `push.default=current`: a branch with no upstream gets **created** on the remote rather than git refusing with a `--set-upstream` hint. The plan documents this as "strictly more permissive," which is true, but in the 60–80-concurrent-agent worktree world this means every agent's bare push silently spawns a remote branch — potential remote-branch sprawl. This is a deliberate, arguably desirable tradeoff, but it's a repo-wide side effect of an "etch refspec setup" command and deserves explicit operator-facing framing.
**Recommendation:** Keep the design, but make the README/command-output notice state plainly that after setup, bare `git push` will create remote branches for upstream-less branches (not just sync etch refs). One sentence so an operator isn't surprised by new remote branches appearing.

**[MINOR] Implementation steps — `--remote` flag format unspecified**
Step 1 parses `--remote <name>` from `os.Args[2:]` but doesn't say whether `--remote=name` (single-token) is also accepted. House style is plain dispatch with no flag library, so this is a hand-rolled parse.
**Recommendation:** Pick one form (or support both) and add a test asserting the unsupported form errors clearly rather than silently misparsing.

**[MINOR] Validation gates — `make smoke` depends on a live Entire CLI**
Listing `make smoke` green as a gate is good, but per CLAUDE.md it runs against the real `entire` binary and is environment-dependent; it can't validate the refspec change in isolation. This is fine as a final gate but shouldn't be the primary evidence for this batch.
**Recommendation:** Treat the ETCH-16 regression test (item 9) and the e2e round-trip (item 10) as the authoritative gates; keep `make smoke` as a non-blocking sanity check and note that framing.

### 4. Positive Observations

- **Empirically validated, not asserted.** The plan's claims about `HEAD` augmentation, CLI-refspec override, and detached-HEAD failure are correct — I reproduced all three independently. Grounding a git-plumbing fix in actual temp-repo experiments is exactly right and rare.
- **Accurate root-cause analysis.** The push/fetch config asymmetry (fetch is materialized by `git remote add`/`clone` and thus augmentative; push has no default entry, so any explicit entry replaces the implicit default) is precisely correct and explains the bug at the right level.
- **Caught the README's own latent trap.** The plan noticed the README manual-equivalent already shows `+` on fetch (which the current code omits) and itself reproduces the push-only bug — and folds the README fix into the same change so docs and behavior converge.
- **Strong test matrix with explicit regression gates.** Items 9 (ETCH-16 regression) and 10 (e2e round-trip) are named as mandatory gates with concrete setup; idempotency and the legacy-`+` upgrade path are both covered. Aligns with the project's binary-level, zero-dependency testing philosophy and the existing `testutil` (`NewTestRepo`, `RunBinary`, `RunCmd`) surface.
- **Disciplined scope and rejected-alternatives section.** Rejecting `refs/heads/*:refs/heads/*` (too aggressive for the worktree world) and hook-time pushes (contradicts the SPEC transport constraint) shows the author weighed the design against project realities rather than picking the first option. Clear file inventory, idempotency awareness, upgrade-path healing, and the `git restore bin/` house-rule reminder all reflect real care.
