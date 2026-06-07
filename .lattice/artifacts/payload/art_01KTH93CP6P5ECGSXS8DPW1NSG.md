# Plan Review: ETCH-28 — Private key BODY leaks; only BEGIN header redacted

### 1. Verdict

**FAIL (plan-level)**

The root cause and the corrective regex are correct and well-diagnosed, but the plan as written is a verbatim copy of the task description with no implementation detail, no test plan, and — most importantly — it overlooks a real regression the proposed regex introduces: a private-key block with no `-----END-----` marker (truncated or streaming capture) goes from *partially redacted* to *not redacted at all*, leaking the entire key body. That is the exact "worse than no redaction" failure the ticket is trying to eliminate, inverted. This needs to be resolved in the plan before implementation.

### 2. Summary

I reviewed the ETCH-28 plan against `internal/redact/secrets.go`, `redact.go`, and the existing `redact_test.go`. The diagnosis is accurate: the `private-key` pattern at `secrets.go:48` matches only the BEGIN marker line, leaving base64 material and the END line in plaintext. The proposed full-block regex is a sound fix for the *complete-block* case, but the plan does not account for incomplete blocks (no END marker), does not mention that the existing private-key tests will break, and includes no test plan despite the project's hard rule that every ticket ships with tests.

### 3. Issues

**[CRITICAL] Proposed regex — truncated key blocks (no END marker) become fully unredacted**
The proposed regex `(?s)-----BEGIN[^-]*PRIVATE KEY-----.*?-----END[^-]*PRIVATE KEY-----` requires a matching `-----END ... PRIVATE KEY-----` to fire at all. If a captured prompt contains a BEGIN marker and key material but the END marker is absent or malformed — which is plausible for this system, given prompts are captured from agent sessions and partial records are committed from `.wip.jsonl` crash-recovery buffers — the new regex matches *nothing*, so the key material is emitted entirely in plaintext. Under the current (buggy) regex, at least the BEGIN line is redacted. The fix as specified therefore regresses the worst case from "partial leak" to "total leak."
**Recommendation:** Make the END marker optional with a fallback to end-of-text, e.g. `(?s)-----BEGIN[^-]*PRIVATE KEY-----.*?(?:-----END[^-]*PRIVATE KEY-----|\z)`. In Go's RE2, `\z` anchors end of text and, with the lazy `.*?`, will consume to EOF when no END marker is present, guaranteeing the body is always redacted once a BEGIN marker appears. The plan should state this explicitly and require a test for the no-END case.

**[MAJOR] Whole plan — no test plan, and existing tests will break**
The project CLAUDE.md states "Every ticket ships with tests. No exceptions." The plan contains no test section. Worse, the existing `TestScanSecretsPrivateKey` (`internal/redact/redact_test.go:133-151`) uses truncated inputs with **no END marker** (`"-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAK..."`). The proposed regex will *fail* all three existing assertions, so the change breaks `make test` on landing. The plan does not acknowledge this.
**Recommendation:** Expand the plan to (a) update the existing assertions to use full BEGIN…material…END blocks, (b) add a positive assertion that the base64 body and the END line are *absent* from the output (the actual bug — current tests only assert the marker is *present*, which is why this shipped), and (c) add cases for: full block redaction, the no-END fallback, multiple blocks in one string (verify the lazy `.*?` doesn't over-match across two keys), and `OPENSSH`/`ENCRYPTED` variants now covered by `[^-]*`.

**[MINOR] Plan — no file/scope statement**
The plan never names the files it will touch. It is inferable (`secrets.go:48`, `redact_test.go`), but an explicit "modify `secrets.go` line 48; update/extend `redact_test.go`" line removes ambiguity and confirms scope is contained to the `redact` package.
**Recommendation:** Add a one-line "Files changed" note. Confirm no other call sites depend on the old BEGIN-only behavior (verified during review: `ScanSecrets` is only reached via `Redact`, called from `internal/hooks/commit.go` — no other dependents, so scope is safe).

### 4. Positive Observations

- **Excellent root-cause diagnosis.** The ticket pinpoints the exact file, the exact failure mode (BEGIN line only), and backs it with empirical evidence (retained `MIIEpAIBAAK...` material). This is the hard part and it's done well.
- **The `(?s)` dotall flag and `[^-]*` generalization are correct.** `(?s)` is required so `.*?` spans newlines, and `[^-]*` correctly broadens coverage beyond the original hardcoded `(RSA |EC |DSA )?` to OPENSSH/ENCRYPTED/etc. variants — a genuine improvement over the status quo.
- **Sequential-replace safety holds.** I confirmed the BEGIN/END markers contain dashes and won't be consumed by the earlier `openai-api-key` (`sk-...`) or `generic-secret` patterns, so the block anchors survive prior passes and the full-block match remains intact. The chosen approach is compatible with the existing `ScanSecrets` pipeline.
