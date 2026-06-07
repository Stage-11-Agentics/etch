# Plan Review: ETCH-29 — Over-redaction of `sk-ant-EXAMPLE` placeholder

### 1. Verdict

**FAIL (plan-level)**

### 2. Summary

I reviewed the proposed plan to tighten the Anthropic API-key regex in `internal/redact/secrets.go` so documentation placeholders like `sk-ant-EXAMPLE` are no longer redacted. The root-cause analysis is correct and I verified it against the source: the pattern `sk-ant-[a-zA-Z0-9_-]+` does match `sk-ant-EXAMPLE`. The blocking problem is that the "Plan" section is a **verbatim copy of the task description** — it states the desired direction ("require the api-tier segment and a minimum length") but contains no actual implementation plan: no proposed regex, no test additions, no file list, and no acknowledgment of an existing test that the change can silently break.

### 3. Issues

**[MAJOR] Plan (entire section) — Plan is a copy of the task, not an implementation plan**
The Plan section (lines 19–23) is identical to the Task Description (lines 14–16). It restates the symptom, root cause, and a one-clause hint at the fix, but never commits to *what the new regex will be*, *which tests change*, or *how the fix will be verified*. The reviewer (and the implementer) are left to re-derive the entire solution. For a redaction change — where the cost of a wrong regex is either leaked secrets (too loose) or clobbered docs (too tight) — the actual pattern is the single most important artifact and it is absent.
**Recommendation:** State the concrete proposed regex and rationale. A reasonable candidate is `sk-ant-[a-z0-9]+-[a-zA-Z0-9_-]{20,}` — requires a tier segment (`api03`, future-proof against `api04`, etc.) plus a long opaque body, which excludes `sk-ant-EXAMPLE` / `sk-ant-PLACEHOLDER`. Pin the chosen pattern in the plan so it can be reviewed before implementation.

**[MAJOR] Testing — Mandatory negation test not specified; existing test may break**
The project's testing philosophy is explicit: "Every ticket ships with tests. No exceptions." The audit item *is* a negation test (`sk-ant-EXAMPLE` must be preserved), yet the plan does not commit to adding it. More importantly, there is a concrete regression trap the plan ignores: the existing `TestScanSecretsAnthropic` (redact_test.go:70–76) uses fixture `sk-ant-api03-abcdefghijklmnop`, whose body after `api03-` is only **16 characters**. A naive "minimum length" of 20+ on the body would break this passing test. `TestScanSecretsMultiple` (line 170) uses the same 16-char fixture and is exposed to the same break.
**Recommendation:** The plan must (a) add a positive assertion that `sk-ant-EXAMPLE` (and ideally `sk-ant-PLACEHOLDER`) is preserved, and (b) either choose a body-length threshold ≤16 or lengthen the existing fixtures to a realistic ~95-char key. Decide and document which, so the implementer doesn't pick a threshold that silently breaks a green test.

**[MINOR] Risk — Interaction with the OpenAI pattern not analyzed**
`anthropic-api-key` runs before `openai-api-key` (`sk-[a-zA-Z0-9]{20,}`) in `builtinPatterns`. If the tightened Anthropic regex stops matching a given string, the OpenAI regex gets a second pass at it. I verified `sk-ant-EXAMPLE` is *not* caught by the OpenAI pattern (the body before the first hyphen, `ant`, is only 3 chars), so the placeholder is genuinely preserved — but the plan should confirm this rather than leave it to chance, since the whole bug class is "one pattern catching what another shouldn't."
**Recommendation:** Add one line to the plan noting the OpenAI-pattern fall-through was considered and add a passthrough assertion for `sk-ant-EXAMPLE` to lock it in.

**[MINOR] Files — Modified files not enumerated**
The plan names `internal/redact/secrets.go` only in passing inside the root-cause prose and never lists `internal/redact/redact_test.go` as a file to touch, despite tests being mandatory.
**Recommendation:** Add an explicit "Files modified" list: `internal/redact/secrets.go` (regex) and `internal/redact/redact_test.go` (negation + regression tests).

### 4. Positive Observations

- The **root-cause analysis is accurate and empirically verified** — I confirmed `sk-ant-[a-zA-Z0-9_-]+` matches `sk-ant-EXAMPLE`, and that the counterpart `sk-DOCUMENTATION-NOT-A-KEY` is correctly preserved (no pattern matches it: the generic-secret pattern needs a `key=` prefix, and the OpenAI pattern's pre-hyphen segment is too short).
- The fix direction is **sound and appropriately scoped** — a regex tightening in one well-isolated function (`ScanSecrets`), no architectural change, no scope creep. Once the concrete regex and test deltas are specified, this is a clean single-pass implementation.
- Good instinct to **anchor on the real key format** (`sk-ant-api03-` + long body) rather than blocklisting placeholder strings, which would be brittle.

---

**Path to PASS:** Re-issue the plan with (1) the concrete proposed regex, (2) the body-length decision reconciled against the existing 16-char test fixtures, (3) explicit test additions for the `sk-ant-EXAMPLE` negation case, and (4) a one-line file list. All four are quick to add; the underlying fix is correct.
