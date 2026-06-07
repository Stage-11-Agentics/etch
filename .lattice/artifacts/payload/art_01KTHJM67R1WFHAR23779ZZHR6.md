# ETCH-40 Closeout Audit — Orchestrator (2026-06-07)

Verified finding-by-finding against `reviews/2026-06-04-deep-code-review.md` using merged diffs (PRs #17, #18, #20, #21, #23, #24), per-finding delegator comments, and direct code/test inspection on post-merge main (all gates green: go test ./..., make build, make smoke, make test-density).

| Finding | Disposition | Evidence |
|---|---|---|
| f.1 live-session false orphan | FIXED (PR #24) | PID+ps-start-time at session_start, strict agent allowlist; verified-alive vetoes recovery (incl. past timeout, documented); stat-first scan. Alive-veto + idle-false-positive tests in recovery_test.go. Absorbs ETCH-30/36. |
| f.2 findRepoRoot=CWD | FIXED (PR #18) | ResolveRepoContext (StateRoot=common-dir parent, WorkDir=toplevel) in all six hooks; subdir/worktree/non-git adversarial tests (reporoot_test.go). |
| f.3 refs overwritable | FIXED (PR #24) | Create-only update-ref on canonical namespace, typed ErrRefExists (writer.go:24); single documented incomplete→complete CAS upgrade; refs/etch/local keeps ETCH-41 overwrite contract. writer_test.go +124 lines. |
| f.4 duplicate session_start | FIXED (PR #24) | Mapping reuse-guard; TestDuplicateSessionStartReusesSession (hooks_test.go:49); resumed wip shielded from own recovery pass. |
| f.5 redaction Prompt.Text-only | FIXED (PR #17) | redact.DeepRedact full-record walk at BOTH commit boundaries (commit.go:29); committed-blob e2e proves session.json + agent-trace.json clean. |
| f.6 local_only_fields unimplemented | FIXED (PR #23, ETCH-41) | stripForPush projection (commit.go:49-59): full fidelity refs/etch/local, stripped refs/etch/sessions; bare-remote round-trip e2e; 653 test lines. Follow-up ETCH-43 (low) filed for local-fidelity query flag. |
| f.7 OpenAI key pattern | FIXED (PR #17) | sk-proj-/sk-svcacct-/sk-admin- patterns; placeholders preserved (ETCH-29 negative tests). |
| f.8 commitSession swallowed | FIXED (PR #24) | Visible failure + exactly-once retry via recovery (truthful complete/normal from retained end event); TestCommitFailureVisibleAndRecoverable (reporoot_test.go:293). Bonus: D/F-conflict-swallowed-as-exists bug found by injection test and fixed. |
| f.9 recovery divergence | FIXED (PR #24) | Recovery rides capture.ReduceEvents/FinishSession (recovery.go:207,225); parallel aggregator + dead flat decode deleted (-553 lines recovery.go); byte-identical parity test. Absorbs ETCH-33. |
| f.10 tokens never populated | RESOLVED-BY-DECISION (PR #21) | Operator decision: drop from v1. OUTPUT_SPEC amended null-in-v1/reserved; dead aggregation paths deleted; v2 future work. |
| below-cut: gitDiffFiles renames/non-ASCII | FIXED (PR #24) | -z rename-aware parse. |
| below-cut: archive atomicity | FIXED (PR #24) | Single update-ref --stdin transaction (archive.go:236); TestArchive_ConcurrentRepointAbortsQuarterAtomically. |
| below-cut: ScanOrphaned perf | FIXED (PR #24) | mtime stat-first pre-filter. |
| below-cut: mid-rune truncation | FIXED (PR #24) | Rune-boundary backoff in user_prompt_submit. |
| Refuted non-bugs (exit_reason clobber, index races, worktree diff-dir) | NOT TOUCHED | Verified no delegator "fixed" them — correct per review. |

Adversarial test gate (run-state requirement): hook re-delivery ✓, idle-timeout false positive ✓, commit-failure injection ✓, duplicate session_start ✓ — all present and green on main.

Verdict: PASS. All 10 findings + 4 below-cut items addressed or explicitly resolved by recorded operator decision. Moving ETCH-40 to done.