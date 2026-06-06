# ETCH-28: Secret scan: private key BODY leaks; only BEGIN header redacted

AUDIT ITEM 6. REPRO: prompt containing a full '-----BEGIN RSA PRIVATE KEY-----\n<base64 material>\n-----END RSA PRIVATE KEY-----' block.
EXPECTED whole block redacted; ACTUAL only the '-----BEGIN ... PRIVATE KEY-----' line becomes [REDACTED:private-key]; the base64 key MATERIAL and the END line remain in plaintext.
ROOT CAUSE: internal/redact/secrets.go private-key regex matches only the BEGIN marker line, not the block. This is worse than no redaction because the partial redaction implies safety. Match the full block: '(?s)-----BEGIN[^-]*PRIVATE KEY-----.*?-----END[^-]*PRIVATE KEY-----'. Verified empirically (stored text retained MIIEpAIBAAK... material).
