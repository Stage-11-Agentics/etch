# ETCH-28: Secret scan: private key BODY leaks; only BEGIN header redacted

AUDIT ITEM 6. REPRO: prompt containing a full '-----BEGIN RSA PRIVATE KEY-----\n<base64 material>\n-----END RSA PRIVATE KEY-----' block.
EXPECTED whole block redacted; ACTUAL only the '-----BEGIN ... PRIVATE KEY-----' line becomes [REDACTED:private-key]; the base64 key MATERIAL and the END line remain in plaintext.
ROOT CAUSE: internal/redact/secrets.go private-key regex matches only the BEGIN marker line, not the block. This is worse than no redaction because the partial redaction implies safety. Match the full block: '(?s)-----BEGIN[^-]*PRIVATE KEY-----.*?-----END[^-]*PRIVATE KEY-----'. Verified empirically (stored text retained MIIEpAIBAAK... material).

---

**BATCH PLAN POINTER:** This ticket is implemented as part of the redaction completeness batch (ETCH-26/27/28/29/39 + ETCH-40 findings 5 & 7), one PR on branch `fix/redaction-batch`. The authoritative plan lives in the batch primary's plan file: `plans/task_01KSTXRD7W1S9VP2S5PT9KCSS4.md` (ETCH-26).
