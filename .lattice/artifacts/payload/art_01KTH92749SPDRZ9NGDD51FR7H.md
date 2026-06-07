# Plan Review: ETCH-39 — Secret scan misses common credential keys

## 1. Verdict

**FAIL (plan-level)**

## 2. Summary

I reviewed the plan to extend the `generic-secret` keyword set in `internal/redact/secrets.go` to cover `password|passwd|pwd|token|client_secret`. The plan body is a verbatim copy of the task description — it contains no implementation steps, no file identification, and no test plan. More importantly, I verified empirically that the proposed keyword change **does not redact the plan's own motivating example** (`DB_PASS=hunter2password`), because the fix ignores two structural properties of the existing regex: the keyword stem and the 16-character minimum value length. The plan needs revision before implementation.

## 3. Issues

**[CRITICAL] Proposed fix — Does not redact the motivating example `DB_PASS=hunter2password`**
The current `generic-secret` regex is:
```
(?i)(api[_-]?key|api[_-]?secret|access[_-]?token|secret[_-]?key)\s*[:=]\s*["']?[a-zA-Z0-9_\-]{16,}["']?
```
All keywords share a single value tail `["']?[a-zA-Z0-9_\-]{16,}["']?` with a **16-character minimum**. I compiled both the current and the proposed regex and ran them against the task's examples:

| Input | Current | Proposed (per plan) |
|---|---|---|
| `DB_PASS=hunter2password` | ❌ | ❌ **still not redacted** |
| `password=hunter2password` | ❌ | ❌ (value is 15 chars) |
| `password=hunter2` | ❌ | ❌ (short password) |
| `pwd=shortpw` | ❌ | ❌ (short password) |
| `passwd=correcthorsebatterystaple` | ❌ | ✅ |
| `token=abc123def456ghi789` | ❌ | ✅ |

Two independent reasons `DB_PASS=hunter2password` fails even after the fix:
1. **Keyword stem mismatch.** `DB_PASS` ends in `PASS`, which none of `password|passwd|pwd` match. The proposed set never matches this line at all.
2. **Length floor.** `hunter2password` is 15 characters, below the `{16,}` minimum, so it would not match even if the keyword matched.

The plan claims this exact line is the problem to solve but the proposed change leaves it unredacted.
**Recommendation:** Add the bare stem `pass` (which also covers `DB_PASS`, `MYSQL_PASS`, `PGPASS`, etc.) and lower the minimum value length for these credential keywords (see next issue). Verify the fix against `DB_PASS=hunter2password` specifically as an acceptance test.

**[MAJOR] Proposed fix — The 16-char minimum is wrong for passwords**
The `{16,}` floor was calibrated for API keys, which are long and high-entropy. Passwords pasted in `.env` files are routinely 8–15 characters. Reusing the shared value tail means the new password/pwd keywords inherit a threshold that excludes most real passwords. The plan does not acknowledge this and would ship a "password redactor" that misses the common case.
**Recommendation:** Either (a) split the credential-keyword group into its own pattern with a lower minimum (e.g. `{6,}` or `{4,}`), accepting more aggressive redaction (the right bias for a redactor), or (b) explicitly state and justify keeping `{16,}` and the resulting coverage gap. Whatever is chosen must be a deliberate decision documented in the plan, not an accident of regex reuse.

**[MAJOR] Proposed fix — Value char class excludes special characters in passwords**
The value tail `[a-zA-Z0-9_\-]` matches only alphanumerics, underscore, and hyphen. Passwords frequently contain `!@#$%^&*` etc. A line like `password=P@ssw0rd!2024` would only partially redact (stopping at the first `@`), leaking part of the secret. This matters more for passwords than for API keys.
**Recommendation:** Broaden the value class for the credential group (e.g. match non-whitespace `\S+` after the delimiter, as the existing `aws-secret-key` pattern already does), so the whole secret is captured.

**[MAJOR] Plan content — No actual plan; just the task description copied verbatim**
Lines 17–19 of the plan are identical to the task description (line 14). There is no identification of the file to change (`internal/redact/secrets.go`, the `generic-secret` entry in `builtinPatterns`), no test plan, and no acceptance criteria. The project's CLAUDE.md mandates unit tests for every ticket ("Every ticket ships with tests. No exceptions"), and `internal/redact/redact_test.go` is the existing home for them.
**Recommendation:** Expand the plan to: (1) name the exact pattern entry and the regex change, (2) list new test cases to add to `redact_test.go` — including the motivating `DB_PASS=hunter2password`, short passwords, symbol-containing passwords, and at least one negative/false-positive guard — and (3) state the empirical verification step (`make test`, and re-checking the `/tmp/etch-custom` paste scenario referenced in the task).

**[MINOR] Proposed fix — Bare `token` / `pwd` may raise false positives**
Adding bare `token` and `pwd` broadens matching beyond the structured `access_token`. In a redactor, over-redaction is the safe direction, so this is acceptable — but it should be a conscious choice and covered by a test asserting that benign prose like `token bucket` (no `=`/`:` delimiter) is left alone.
**Recommendation:** Note the over-redaction bias explicitly in the plan and add a negative test to lock in the delimiter requirement.

## 4. Positive Observations

- The task description is unusually well-grounded: it states the precise gap, names the missing keywords, and references empirical verification in `/tmp/etch-custom` confirming that custom patterns and the `sk-ant-` key in the same paste *were* redacted. That isolates the defect cleanly to the `generic-secret` keyword set.
- The severity triage ("secondary," lower than ETCH-25..28) is appropriate and shows good prioritization relative to the structured-key misses.
- The change is correctly scoped to a single, well-isolated function (`builtinPatterns` in `internal/redact/secrets.go`) with an existing dedicated test file — a small, low-risk surface once the regex details above are corrected.
