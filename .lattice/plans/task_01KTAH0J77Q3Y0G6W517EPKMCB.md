# ETCH-40: Deep code review remediation (2026-06-04): lifecycle, recovery, redaction, data-quality

Umbrella remediation ticket for the 2026-06-04 deep code review. THE SPEC IS THE REVIEW FILE: reviews/2026-06-04-deep-code-review.md — read it first; it has file:line, verified failure scenarios, and refuted non-bugs (do not re-fix those).

Scope (10 confirmed findings + 4 below-cut):
1. Recovery falsely orphans LIVE idle sessions — capture PID at session_start, wire the liveness check (recovery.go:129; absorbs ETCH-30)
2. findRepoRoot()=os.Getwd() → silent session loss — resolve git common-dir root at the hook boundary (hooks/common.go:39; supersedes-in-part ETCH-34)
3. Session refs silently overwritable — create-only update-ref guard (refs/writer.go:47)
4. Duplicate session_start splits sessions — reuse existing mapping (session_start.go:38)
5. Redaction only covers Prompt.Text — full-record redaction pass at commit boundary (commit.go:24)
6. local_only_fields unimplemented (config.go:13; absorbs ETCH-31)
7. OpenAI key regex misses sk-proj-/sk-svcacct- (secrets.go:28; absorbs ETCH-25)
8. commitSession failure swallowed, printOK lies (session_end.go:62)
9. Recovery records falsified/lossy — share ONE wip→session reducer with Finalize (recovery.go:263; absorbs ETCH-33)
10. tokens never populated — reconcile spec vs Entire payload reality (buffer.go:159; absorbs ETCH-32)
Below-cut: gitDiffFiles rename/quotePath corruption; archive non-atomic quarter (use update-ref --stdin); ScanOrphaned O(N×M) per start (absorbs ETCH-36); mid-rune prompt truncation.

NOT absorbed (still standalone): ETCH-26/27/28/29/39 (distinct secret-scan patterns the review did not cover).

Acceptance: each fix lands with adversarial tests (hook re-delivery, idle-timeout false positive, commit-failure injection, duplicate session_start) — the review's thematic conclusion is that these paths were spec'd but never tested.
