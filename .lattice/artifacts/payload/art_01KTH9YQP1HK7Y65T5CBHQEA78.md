# Validation Evidence — Redaction Batch (da6e7f8)

**Full gate (worktree, fresh run):**
- `go test ./...` — 12/12 packages ok (includes 20+ new redaction tests)
- `go vet ./...` — clean
- `make build` — ok
- `make smoke` — SMOKE PASSED (real Entire CLI, end-to-end)

**Live e2e demonstration (real binary, scratch repo, committed blob inspected via `git show`):**
Simulated session_start → user_prompt_submit (secrets embedded) → pre_tool_use (JWT in tool_name) → git commit of a file named `sk-proj-….txt` → session_end. Then inspected `refs/etch/sessions/01KTH9VSXVVAJMQTDJRDEWQQPB`:

- prompt: `deploy with key [REDACTED:anthropic-api-key] and aws secret [REDACTED:aws-secret-key] plus jwt [REDACTED:jwt] and DB_[REDACTED:generic-secret] but keep sk-ant-EXAMPLE as doc placeholder`
- by_tool (map KEY redacted): `{'Bash [REDACTED:jwt]': 1}`
- files_touched (finding-5 adversarial shape — secret in committed file path): `[REDACTED:openai-api-key].txt`
- git_start.head_sha INTACT (40-hex survives the bare-AWS validator)
- Leak scan over BOTH committed blobs (session.json + agent-trace.json) for all five planted secrets: **0 leaked lines**
- ETCH-29 negative: `sk-ant-EXAMPLE` preserved verbatim

**Observed + pinned accepted FP:** the internal path run `etch/sessions/<ULID>` is exactly 40 base64-ish chars and gets over-redacted as aws-secret-key (safe direction; pinned by TestScanSecretsBareAWSSecretPathFP).

Per-ticket coverage: ETCH-26 bare-AWS validator pattern; ETCH-27 jwt; ETCH-28 full PEM block + truncated fallback; ETCH-29 anthropic tier tightening + placeholder negatives; ETCH-39 credential keywords; ETCH-40 f.5 DeepRedact at both commit boundaries; f.7 modern OpenAI prefixes.