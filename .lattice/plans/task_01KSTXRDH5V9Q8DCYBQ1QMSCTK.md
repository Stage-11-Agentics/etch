# ETCH-29: Over-redaction: sk-ant-EXAMPLE doc placeholder gets redacted

AUDIT ITEM 6 (negation test). REPRO: prompt 'use sk-ant-EXAMPLE as a placeholder'.
EXPECTED preserved (documentation placeholder); ACTUAL becomes [REDACTED:anthropic-api-key].
ROOT CAUSE: internal/redact/secrets.go anthropic regex 'sk-ant-[a-zA-Z0-9_-]+' matches ANY sk-ant- prefix incl. EXAMPLE/PLACEHOLDER. Real keys are 'sk-ant-api03-' + long body; tighten to require the api-tier segment and a minimum length to avoid clobbering docs. Verified empirically; counterpart 'sk-DOCUMENTATION-NOT-A-KEY' was correctly preserved.
