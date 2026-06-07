# ETCH-29: Over-redaction: sk-ant-EXAMPLE doc placeholder gets redacted

AUDIT ITEM 6 (negation test). REPRO: prompt 'use sk-ant-EXAMPLE as a placeholder'.
EXPECTED preserved (documentation placeholder); ACTUAL becomes [REDACTED:anthropic-api-key].
ROOT CAUSE: internal/redact/secrets.go anthropic regex 'sk-ant-[a-zA-Z0-9_-]+' matches ANY sk-ant- prefix incl. EXAMPLE/PLACEHOLDER. Real keys are 'sk-ant-api03-' + long body; tighten to require the api-tier segment and a minimum length to avoid clobbering docs. Verified empirically; counterpart 'sk-DOCUMENTATION-NOT-A-KEY' was correctly preserved.

---

**BATCH PLAN POINTER:** This ticket is implemented as part of the redaction completeness batch (ETCH-26/27/28/29/39 + ETCH-40 findings 5 & 7), one PR on branch `fix/redaction-batch`. The authoritative plan lives in the batch primary's plan file: `plans/task_01KSTXRD7W1S9VP2S5PT9KCSS4.md` (ETCH-26).
