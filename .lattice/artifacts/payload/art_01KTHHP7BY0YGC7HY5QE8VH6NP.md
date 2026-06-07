## Lifecycle batch validation evidence (agent:lifecycle-w1, 2026-06-07)

Branch fix/lifecycle-recovery @ f60239d (8 commits off origin/main @ 3dc67d4).

### Gates
- `make build` ✓  ·  `go vet ./...` ✓  ·  `go test ./...` ✓ (all 13 packages, -count=1)
- `make test-density` ✓ (incl. TestDensityCrashRecovery — fixture needed an mtime backdate for the stat-first scan, committed separately)
- `make smoke` ✓ (real Entire CLI; 10 stages incl. native Claude Code hook dialect, recovery, redaction)

### Adversarial acceptance suite (all new, all passing)
- **Hook re-delivery**: TestRecoverSession_ReDeliveredToolUse (same tool_use_id twice → counts once); reducer end-event precedence covered by TestStopRetryAfterFailedSessionEnd.
- **Idle-timeout false positive**: TestScanOrphaned_AlivePIDVetoesPastTimeout + TestRecoverAll_SkipsActiveSession (unit) + TestE2ELiveIdleSessionNotRecovered (binary): 30h-idle wip with verified-alive PID → no ref, wip intact.
- **PID reuse**: TestScanOrphaned_PIDReuseDoesNotVeto (alive PID, mismatched start time → recovered).
- **Commit-failure injection**: TestCommitRetryViaRecovery (sabotage → visible failure → heal → sibling start recovers exactly one complete/normal ref, all state cleaned) + TestStopRetryAfterFailedSessionEnd (stop retry keeps session_end's truth — exit_reason normal, not unknown).
- **Duplicate session_start**: TestDuplicateSessionStartReusesSession (one ULID, one mapping, one ref) + TestResumeAfterCrashContinuesSession (crashed-looking wip shielded from recovery during its own resume).
- **Recovery parity**: TestRecoveryParity_HasEnd — identical event stream through Finalize vs RecoverSession → byte-identical session JSON (real git repo, real diff).
- **Ref immutability**: TestWriteSessionRef_CreateOnly / _NeverDowngrade / _UpgradeIncompleteToComplete / _ConcurrentSameULID (10 racers → exactly 1 winner, 9 ErrRefExists).
- **gitDiffFiles**: TestGitDiffFiles_RenameAndNonASCII (R100 → {old:deleted}+{new:added}; héllo wörld.txt verbatim; tab-in-name verbatim; no octal escapes).
- **Archive atomicity**: TestArchive_ConcurrentRepointAbortsQuarterAtomically (stale plan → whole quarter aborted, zero half-state; fresh run converges, repointed ref correctly stays live).
- **UTF-8 truncation**: TestPromptTruncationRuneBoundary (4-byte rune straddling 32KiB cut → dropped whole, valid UTF-8, no U+FFFD).

### Bonus finding during testing
The commit-failure injection test caught a live bug in the new create-only guard: a D/F-conflict create failure (no resolvable ref) was misclassified as ErrRefExists and swallowed. Fixed in 83284b5 — the exists/upgrade flow now only engages when the ref actually resolves.