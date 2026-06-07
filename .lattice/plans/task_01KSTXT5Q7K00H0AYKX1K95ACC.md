# ETCH-39: Secret scan misses common credential keys (password/passwd/bare token=)

AUDIT ITEM 6 (secondary). The generic-secret pattern only keys off api_key/api_secret/access_token/secret_key. A multiline .env paste line 'DB_PASS=hunter2password' (and password=, passwd=, pwd=, bare token=) is NOT redacted. Lower severity than the structured-key misses (ETCH-25..28) but common in pasted .env files. FIX: extend generic-secret keyword set to include password|passwd|pwd|token|client_secret. Verified empirically in /tmp/etch-custom (custom patterns from settings.json and the anthropic key inside the same multiline paste WERE redacted).

---

**BATCH PLAN POINTER:** This ticket is implemented as part of the redaction completeness batch (ETCH-26/27/28/29/39 + ETCH-40 findings 5 & 7), one PR on branch `fix/redaction-batch`. The authoritative plan lives in the batch primary's plan file: `plans/task_01KSTXRD7W1S9VP2S5PT9KCSS4.md` (ETCH-26).
