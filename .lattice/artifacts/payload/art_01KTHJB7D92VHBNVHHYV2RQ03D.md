## Code review (own-reviewer fallback) — lifecycle batch, fix/lifecycle-recovery @ 4d62267

**Fallback note:** `lattice code-review --mode single --base origin/main` returned 'Diff is empty' twice — it resolves the repo via LATTICE_ROOT (main checkout, on main), so worktree-branch diffs are invisible to it. Structured self-review performed instead per run rules.

### Scope reviewed
10 commits vs origin/main @ 7d16929 (post PR #21/#22/#23 rebase): reducer unification (f.9), PID liveness (f.1), reuse-guard (f.4), create-only refs (f.3), exactly-once retry (f.8), below-cut (diff -z / archive txn / rune backoff / stat-first scan), docs, two fixture mtime fixes.

### Verified correct (with how)
1. **Reducer parity is structural, not asserted**: recovery calls the same ReduceEvents/FinishSession as Finalize; TestRecoveryParity_HasEnd proves byte-identical JSON for identical streams. The deleted parallel aggregator (wipEvent/flattenHookEvent/applyTokenSnapshot, dead flat decode) has no remaining references (grep clean).
2. **Rebase interactions audited line-by-line**:
   - ETCH-41 (PR #23) strip-before-push now lives INSIDE commitRecord — recovery gets the dual-ref projection for free; TestE2ELocalOnlyCrashRecovery (theirs) passes against my recovery rewrite.
   - Canonical sessions namespace is create-only + upgrade-CAS; refs/etch/local/ deliberately keeps overwrite semantics (its own documented self-healing contract). WriteSessionRefAt is used only for local refs.
   - agent_session_id (PR #21) flows through the reducer (session_start case) and composes with the f.4 reuse-guard (guard fires before any mint/clobber).
   - tokens stay null everywhere (PR #21's v1 contract); reducer never assigns session.Tokens; recovery tests assert nil.
3. **Liveness policy edges**: empty ev.SessionID skips the reuse-guard (LookupMapping returns ""); pid 0 → timeout governs; ps unreadable for a live pid → conservative veto (documented); PID-reuse → start-time mismatch → no veto (tested); cycle-bounded ancestry walk (tested).
4. **ErrRefExists classification**: only when the ref resolves; D/F-conflict/permission failures stay visible (TestCommitRetryViaRecovery would catch regressions — it did catch the original bug).
5. **gitDiffFiles -z**: trailing NUL token handled (TrimSpace does not eat NUL; empty token skipped); R/C two-path consumption bounds-checked; rename emits within the schema's {added,modified,deleted} vocabulary.
6. **Archive txn**: zero-OID guard for fresh quarters sized from the actual SHA length (sha1/sha256 portable); single --stdin transaction is all-or-nothing (interruption test proves no half-state).

### Findings (all minor, addressed or accepted)
- m1 (accepted): ScanOrphaned silently skips entries whose Info() errors — transient stat failure delays recovery to the next scan; acceptable, self-healing.
- m2 (accepted): scanActivityGrace=5min means dead_pid recovery waits ≥5min after crash; deliberate (protects in-flight teardown), noted in code.
- m3 (fixed during review): two upstream test fixtures (density, local-only) wrote 'old' wips with fresh mtimes — updated to Chtimes, committed separately with rationale.
- m4 (accepted): commitRecord's ErrRefExists→success conversion logs at stderr only; the alternative (failing the hook) would re-fail forever on a legitimately-settled ref. Correct trade.

### Verdict
PASS. Gates: build ✓ vet ✓ go test ./... ✓ density ✓ smoke ✓ (all re-run post-rebase).