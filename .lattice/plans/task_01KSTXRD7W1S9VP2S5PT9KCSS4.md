# ETCH-26: Secret scan: bare AWS secret access key not redacted

AUDIT ITEM 6. REPRO: prompt containing bare 'wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY' (no key=value prefix).
EXPECTED redacted; ACTUAL stored verbatim.
ROOT CAUSE: internal/redact/secrets.go aws-secret-key regex only matches 'aws_secret_access_key\s*[:=]\s*\S+'. A bare 40-char AWS secret, or one assigned to a differently-named var (AWS_SECRET=, SECRET=), leaks. AWS secrets have a recognizable shape (40 chars base64) that should be matched structurally. Verified empirically.
