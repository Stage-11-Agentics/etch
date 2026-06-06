# ETCH-25: Secret scan: sk-proj- OpenAI keys not redacted

AUDIT ITEM 6 (secret red-team).
REPRO: drive a session whose prompt contains 'sk-proj-AAAA1234BBBB5678CCCC9012DDDD3456EEEE7890FFFF1234' (the CURRENT OpenAI project-key format), finalize, git show <ref>:session.json.
EXPECTED: [REDACTED:openai-api-key]. ACTUAL: key stored verbatim.
ROOT CAUSE: internal/redact/secrets.go openai-api-key regex 'sk-[a-zA-Z0-9]{20,}' breaks at the hyphen after 'sk-proj', so only 4 alphanumerics match before the dash -> no match. The classic 'sk-<48 alnum>' format is caught; the modern 'sk-proj-...' and 'sk-svcacct-...' formats are NOT. This is the dominant OpenAI key format in 2025+. Verified empirically via /tmp/etch-qa/redteam2.py.
