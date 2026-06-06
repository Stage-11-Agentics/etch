# ETCH-27: Secret scan: JWTs never redacted (no pattern)

AUDIT ITEM 6. REPRO: prompt containing 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature_part'.
EXPECTED [REDACTED]; ACTUAL stored verbatim.
ROOT CAUSE: internal/redact/secrets.go has NO JWT pattern. A bare JWT (eyJ... three base64url segments) is a common bearer credential and is captured in plaintext. Only matched today if it follows the literal 'Bearer ' prefix. Add a pattern like 'eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+'. Verified empirically.
