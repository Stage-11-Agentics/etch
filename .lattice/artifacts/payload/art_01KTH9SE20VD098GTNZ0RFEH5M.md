# Code Review — Redaction Completeness Batch (own-reviewer fallback)

**Reviewer:** agent:redact-w0-reviewer (delegator self-review; lattice code-review returned empty diff from root-repo routing — fallback per run hard rule)
**Diff:** origin/main..HEAD (69961a1), 7 files, +783/−33

## Verdict: PASS — no Critical or Major findings

## Review method
Full read of the diff plus a call-site audit: every `refs.WriteSessionRef` caller in the tree was checked for redaction coverage, and every pattern was traced against the existing test corpus for ordering interactions.

## Findings

**[MINOR] archive/restore.go:31 writes refs without a redaction pass** — acceptable: restore re-commits a previously-committed (already-redacted) archive snapshot, so no new unredacted data can enter. Worth a comment if archive ever ingests external data.

**[MINOR] Map-key marker collision merges by last-write-wins** — two distinct secret map keys collapsing to the same `[REDACTED:…]` marker lose one count in `ByTool`. Documented in walk.go; counts are advisory metadata, secrets never survive. Accepted.

**[MINOR] Bare-AWS pattern misses secrets inside longer contiguous base64 runs** — inherent to the len==40 maximal-run trick that protects git SHAs. Documented in code AND pinned by TestScanSecretsBareAWSSecretKnownMiss so it reads as intentional, not as a bug.

**[INFO] generic-secret keyword loosening (pass/token/secret, value ≥8) raises FP rate** — deliberate, leak-averse direction; pinned by tests (compass= accepted FP, tokens:/max_tokens= preserved).

## Verified
- Both commit boundaries (commitSession + etchRefWriter.WriteSessionRef) call DeepRedact — recovery path cannot commit less-redacted records (e2e TestE2ECrashRecoveryRedaction proves it on the committed blob).
- Plan-review MAJOR 1 honored: walker is value-returning with write-back at container boundaries (SetMapIndex / field set / re-boxing); nested map[string]any Extra covered by test.
- Pattern ordering is semantic and commented (block→header fallback, bearer→jwt, labeled→bare); one deviation from plan table — bearer-token moved BEFORE jwt to preserve the more specific marker for "Bearer <jwt>" (existing test pinned this).
- Structural-field safety: SHA/ULID/sha256-hash/branch survival all unit-tested; validator rejects all-hex and single-case runs.
- ETCH-29 negatives: sk-ant-EXAMPLE, sk-ant-PLACEHOLDER, sk-DOCUMENTATION-NOT-A-KEY, sk-proj-EXAMPLE all preserved.
- Gates: go test ./... green, go vet clean, make build + make smoke green; binary not committed.

## Ticket → change map
- ETCH-26: bare-AWS validator pattern + AWS_SECRET=/SECRET= keywords
- ETCH-27: jwt pattern
- ETCH-28: private-key full block + truncated fallback
- ETCH-29: anthropic tier-segment tightening + placeholder negative tests
- ETCH-39: generic-secret keyword extension
- ETCH-40 f.5: DeepRedact at both commit boundaries + committed-blob e2e
- ETCH-40 f.7: modern OpenAI prefixes