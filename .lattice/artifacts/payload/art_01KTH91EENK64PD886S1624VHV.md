# Plan Review: ETCH-27 — Secret scan: JWTs never redacted

### 1. Verdict

**PASS** — with caveats. The change is trivial, fully specified by the task itself, and aligned with the existing architecture. Implementation can proceed, but the implementer must honor the items flagged below (test coverage in particular is non-negotiable per project convention).

### 2. Summary

I reviewed the ETCH-27 plan against the actual code in `internal/redact/secrets.go` and its tests in `internal/redact/redact_test.go`. The plan correctly identifies a real, verified gap — `builtinPatterns` has no standalone JWT pattern, so a bare `eyJ...` token is only redacted when it follows a literal `Bearer ` prefix. The fix (add one regex to the pattern slice) is sound and low-risk. The key concern is that the "plan" is a verbatim copy of the task description: it states the root cause and a candidate regex but commits to no implementation steps, no test, and no file list — which for a mandatory-test project is the one thing worth pinning down before coding.

### 3. Issues

**[MAJOR] Plan (whole) — No test commitment, despite a hard project rule**
The plan does not mention adding a test, yet this project's CLAUDE.md states: *"Every ticket ships with tests. No exceptions."* The existing suite (`redact_test.go`) has a `TestScanSecrets*` test per pattern; a new `jwt` pattern without a matching test would be an obvious gap and an inconsistency with the established convention.
**Recommendation:** The plan must add a `TestScanSecretsJWT` case asserting that the exact repro string from the task (`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature_part`) is replaced with `[REDACTED:jwt]` (or the chosen pattern name) and that the original token no longer appears in the output. Add at least one negative case to guard against false positives (e.g., a normal three-part dotted string that does *not* start with `eyJ`).

**[MINOR] Plan (whole) — Pattern name and placement unspecified**
`ScanSecrets` labels each redaction with the pattern's `Name` (`[REDACTED:<name>]`), and patterns run in slice order. The plan names neither the pattern nor where it goes in `builtinPatterns`. The test assertion string depends on the chosen name, so it should be decided up front.
**Recommendation:** Specify `Name: "jwt"` and append it to `builtinPatterns`. Ordering is not correctness-critical here (a `Bearer eyJ...` input is fully redacted under either the existing `bearer-token` rule or the new one, and neither produces a broken double-redaction), but stating it removes ambiguity for the implementer and reviewer.

**[MINOR] Plan (whole) — Regex correctness / false-positive surface not analyzed**
The proposed regex `eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+` is correct: `eyJ` is base64url for `{"` and is a reliable JWT-header marker, the `\.` segment separators are properly escaped, and `[A-Za-z0-9_-]` is the right base64url character class with `-` correctly positioned as a literal at the end of the class. The plan does not surface this reasoning, nor that anchoring on `eyJ` is what keeps the false-positive rate low. Worth a sentence so the reviewer doesn't have to re-derive it.
**Recommendation:** Note in the plan that the `eyJ` prefix is the intentional precision anchor (a generic "three dot-separated base64url tokens" pattern would over-match), and that the signature segment may legitimately be empty in `alg=none` JWTs — decide whether the third `+` (one-or-more) is acceptable (it is, for the stated repro and for any signed JWT) or whether `*` is wanted for completeness. Recommend keeping `+` to avoid matching `eyJxxx.yyy.` trailing-dot noise.

### 4. Positive Observations

- **Accurate, empirically-verified root cause.** The plan correctly pinpoints `internal/redact/secrets.go` and the exact gap — confirmed against the source: the only JWT coverage today is incidental via the `bearer-token` pattern (`Bearer\s+...`), so an unprefixed JWT slips through verbatim.
- **The provided regex is actually correct.** Unlike many one-line "just add a pattern" tasks, the candidate regex here is well-formed and would compile and work as written — no escaping or character-class bugs.
- **Right-sized scope.** A single additive pattern with no change to `ScanSecrets`/`Redact` control flow, no API change, and no backward-compatibility risk (redaction only ever *adds* matches). This is appropriately minimal — no scope creep, no premature abstraction.
- **Architecturally clean fit.** The fix slots into the existing `builtinPatterns` slice exactly as every other secret type does, fully respecting the established pattern in the file.
