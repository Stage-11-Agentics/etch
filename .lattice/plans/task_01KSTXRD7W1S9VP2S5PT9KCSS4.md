# Plan — Redaction Completeness Batch (ETCH-26, 27, 28, 29, 39 + ETCH-40 findings 5 & 7)

**Batch primary:** ETCH-26 · **Branch:** `fix/redaction-batch` (worktree `Etch-worktrees/redaction-batch`, based on `origin/main`) · **One PR for the whole batch.**

## Problem summary

Two failure families, per the 2026-06-04 deep code review (authoritative spec):

1. **Coverage (ETCH-40 finding 5):** `redact.Redact` is called only on `Prompt.Text` at the two commit boundaries (`internal/hooks/commit.go:24` and `:105`). `FilesTouched[].Path`, `ToolUse.ByTool` keys, `Orchestration.Extra`, and every other string-bearing field are committed verbatim into immutable, push-replicated refs. SPEC.md §37 promises "Prompt **and tool-use fields** are scanned… before commit."
2. **Pattern gaps (finding 7 + ETCH-26/27/28/39) and over-match (ETCH-29)** in `internal/redact/secrets.go`:
   - Modern OpenAI keys (`sk-proj-…`, `sk-svcacct-…`) die at the first hyphen of `sk-[a-zA-Z0-9]{20,}`.
   - Bare 40-char AWS secret access keys (no `aws_secret_access_key=` label) leak.
   - JWTs have no pattern at all (only caught behind a literal `Bearer ` prefix).
   - Private keys: only the `-----BEGIN … PRIVATE KEY-----` header line is redacted; the base64 key MATERIAL and END line leak — worse than nothing because it implies safety.
   - Common credential keywords (`password=`, `passwd=`, `pwd=`, bare `token=`, `client_secret=`) aren't in the generic-secret keyword set.
   - Over-redaction: `sk-ant-EXAMPLE` doc placeholders are clobbered because the anthropic pattern accepts ANY `sk-ant-` suffix.

**Explicit non-goals:** the review's refuted non-bugs (exit_reason clobber, index races, worktree diff-dir) are NOT touched. `local_only_fields` (finding 6 / ETCH-31) is out of scope.

## Design

### Part A — Finding 5 first: one redaction pass over the whole record at the commit boundary

New file `internal/redact/walk.go`:

- `type redactor struct { custom []secretPattern }` — built once per commit via `newRedactor(settings)`; compiles custom patterns ONCE (today they're recompiled per `Redact` call; with a deep walk that would be per-string).
- `func (r *redactor) apply(s string) string` — builtin pass + custom pass (current `Redact` body).
- `func DeepRedact(v any, settings config.Settings)` — reflective walker that mutates in place:
  - `reflect.Ptr`/`reflect.Interface` → deref and recurse (nil-safe).
  - `reflect.Struct` → recurse into every settable field.
  - `reflect.Slice`/`reflect.Array` → recurse per element.
  - `reflect.Map` → rebuild: redact string keys AND recurse into values (covers `ToolUse.ByTool map[string]int` keys and `Orchestration.Extra map[string]any` values).
  - `reflect.String` → replace with `r.apply(...)`.
  - Everything else (ints, bools, floats) → ignore.
- Existing `Redact(text, settings) string` stays as a thin wrapper (other call sites / tests unaffected).

Wiring (`internal/hooks/commit.go`):
- `commitSession`: replace the `session.Prompt.Text`-only call with `redact.DeepRedact(session, settings)` on the full `*capture.Session` before marshaling.
- `etchRefWriter.WriteSessionRef` (recovery path): same swap on `*schema.Session`.

**All strings are redacted, no field exemptions.** Safety audit of structural fields against the new pattern set:
- ULIDs (26 chars, Crockford base32): match nothing.
- `GitState.HeadSHA` (40 lowercase hex): the new bare-AWS validator requires upper+lower+digit and rejects all-hex — explicit regression test added.
- `Machine.HostnameHash` (`sha256:` + 64 hex): 64 ≠ 40, no match.

### Part B — Pattern work in `internal/redact/secrets.go`

Extend `secretPattern` with an optional post-match validator (RE2 has no lookarounds, so class-composition checks must be code):

```go
type secretPattern struct {
    Name     string
    Regex    *regexp.Regexp
    Validate func(match string) bool // nil = always redact
}
```

`ScanSecrets` switches from `ReplaceAllString` to `ReplaceAllStringFunc` (also removes `$`-template expansion semantics, a latent footgun).

New/changed patterns, in scan order:

| # | Name | Regex | Notes |
|---|------|-------|-------|
| 1 | `private-key` | `(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----` | ETCH-28: full block incl. material + END. `[A-Z ]*` also covers OPENSSH/ENCRYPTED variants the old `(RSA \|EC \|DSA )?` missed. |
| 2 | `private-key` | `-----BEGIN [A-Z ]*PRIVATE KEY-----[A-Za-z0-9+/=\s]*` | Fallback for truncated blocks (no END marker): header + trailing base64 material. Runs after #1 so complete blocks are already gone. |
| 3 | `jwt` | `\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{6,}` | ETCH-27: three base64url segments, `eyJ` = base64 of `{"`. Min lengths keep prose like `x.y.z` safe. |
| 4 | `anthropic-api-key` | `sk-ant-[a-z]{2,8}[0-9]{2}-[A-Za-z0-9_-]{16,}` | ETCH-29: require the real tier segment (`api03-`, `oat01-`, `sid01-`) + ≥16-char body. `sk-ant-EXAMPLE` no longer matches. |
| 5 | `openai-api-key` | `sk-(?:proj\|svcacct\|admin)-[A-Za-z0-9_-]{20,}` | Finding 7: modern prefixed keys, body may contain `-`/`_`. Deliberately NOT a generic `sk-[\w-]{20,}` — that would clobber `sk-DOCUMENTATION-NOT-A-KEY` (ETCH-29 counterpart, verified preserved today). |
| 6 | `openai-api-key` | `sk-[a-zA-Z0-9]{20,}` | Legacy keys, unchanged. |
| 7–8 | stripe live/test | unchanged | |
| 9 | `aws-access-key` | `AKIA[0-9A-Z]{16}` | unchanged |
| 10 | `aws-secret-key` | `(?i)aws_secret_access_key\s*[:=]\s*\S+` | unchanged (labeled form) |
| 11 | `generic-secret` | `(?i)(api[_-]?key\|api[_-]?secret\|access[_-]?token\|secret[_-]?key\|client[_-]?secret\|password\|passwd\|pwd\|pass\|token\|secret)\s*[:=]\s*["']?[A-Za-z0-9_\-/+=]{8,}["']?` | ETCH-39 + ETCH-26's `AWS_SECRET=`/`SECRET=` variants. Value class gains `/+=` (base64 values like AWS secrets); min length 16→8 so `DB_PASS=hunter2password` (15) redacts. `pass` covers `DB_PASS=`. |
| 12 | `bearer-token` | `Bearer\s+[a-zA-Z0-9._\-]+` | unchanged |
| 13 | `aws-secret-key` | `[A-Za-z0-9/+=]{40,}` + validator | ETCH-26 bare form. The `{40,}` greedy maximal-run trick replaces lookarounds: validator accepts only `len==40 && hasUpper && hasLower && hasDigit && !allHex`. Rejects git SHAs (no uppercase / all-hex), 64-char base64 blobs (len≠40), and runs glued to `key=` prefixes (len>40 — those are covered by #10/#11). Runs LAST so labeled patterns win their more specific marker names. |

**Precision/recall stance (project convention: "best-effort regex, not exhaustive"):** the loosened generic keywords (`pass`, `token`, `secret`, value ≥8) trade some false-positive risk (e.g. `compass=abcdef12`) for catching pasted .env files — acceptable for an audit-trail tool where a false redaction loses a little context and a false pass leaks a credential into an immutable ref.

### Part C — Test plan (`internal/redact/redact_test.go` + hooks tests)

Positive (every shape, stable marker asserted):
- Modern OpenAI: `sk-proj-<24 chars incl - _>`, `sk-svcacct-…`, `sk-admin-…`; legacy `sk-<24 alnum>` still works.
- Anthropic real-shape: `sk-ant-api03-<long>`, `sk-ant-oat01-<long>`.
- Bare AWS secret: `wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY` bare in prose; `AWS_SECRET=<value>`; existing labeled form.
- JWT: ticket repro `eyJhbGciOi….eyJzdWIiOi….signature_part`; JWT inside larger text.
- Private key: full RSA block → entire block (material + END) gone; OPENSSH/ENCRYPTED variants; truncated block (header + material, no END).
- Credential keywords: `password=`, `passwd:`, `pwd=`, `token =`, `client_secret=`, `DB_PASS=hunter2password`.

Negative (must pass through unchanged):
- `sk-ant-EXAMPLE`, `sk-ant-PLACEHOLDER`, `use sk-ant-EXAMPLE as a placeholder` (ETCH-29 repro).
- `sk-DOCUMENTATION-NOT-A-KEY`.
- `sk-proj-EXAMPLE` (short body).
- 40-char lowercase git SHA; 64-char hex (sha256); 40+ char base64 blob (len≠40); ULID.
- `x.y.z` / semver / dotted hostnames (JWT min-lengths).
- Plain prose, `-----BEGIN CERTIFICATE-----` (not a private key).

DeepRedact unit tests (new `walk_test.go` or in redact_test.go):
- Secret in `FilesTouched[].Path` → redacted.
- Secret as a `ToolUse.ByTool` map KEY → key rewritten, count preserved.
- Secret in `Orchestration.Extra` (map[string]any, nested) → redacted.
- Nil pointers everywhere → no panic.
- Non-string fields untouched; `HeadSHA` survives intact.

End-to-end (extend `internal/hooks/e2e_test.go`, proving the **committed blob** is clean):
- pre/post_tool_use with `tool_name` carrying an embedded secret (e.g. a JWT) → `git show refs/etch/sessions/<ulid>:session.json` contains marker, not secret.
- Adversarial finding-5 shape: a `tool_input.file_path` / files-touched path containing `sk-proj-…` → committed record redacted.
- **Fixture update required:** the existing e2e prompt secret `sk-ant-abc123456789…` no longer matches the tightened anthropic pattern (and matches nothing else) — update to realistic `sk-ant-api03-<long>` or the e2e redaction assertion fails. Same audit for `TestScanSecretsAnthropic`/`TestScanSecretsMultiple` fixtures (16-char bodies — pass with `{16,}`, keep).

## Execution order (single branch, logical commits)

1. Part B patterns + unit tests (ETCH-26/27/28/29/39 + f.7) — `internal/redact/secrets.go`, `redact_test.go`.
2. Part A DeepRedact + wiring (f.5) — `internal/redact/walk.go`, `internal/hooks/commit.go`, walk tests.
3. Part C e2e additions + fixture updates — `internal/hooks/e2e_test.go`.
4. Full gate: `go test ./...`, `make build`, `make smoke`; restore `bin/entire-agent-etch` if build touched it (never commit it).

## Risks / mitigations

- **Deep-walk FPs on structural fields** → explicit negative tests (SHA, ULID, hostname hash); validator on the bare-AWS pattern is the only high-FP-risk pattern and is code-guarded.
- **RE2 limitations** (no lookarounds) → maximal-run `{40,}` + length-equality validator instead.
- **Performance** (walk × patterns × sessions) → compile-once redactor; pattern count stays small (~13); sessions are committed once.
- **Recovery path drift** → both commit boundaries (`commitSession`, `etchRefWriter.WriteSessionRef`) call the same `DeepRedact`.
- **Order sensitivity** in the pattern table is now semantic (block-before-header, labeled-before-bare) — documented with comments in the slice.

## Plan-Review Cycle 1 Resolutions (AUTHORITATIVE — overrides earlier text on conflict)

Review verdict: **PASS** (artifact art_01KTH95C4WXXXS2YDS19Z3E2WY). All five findings ACCEPTED:

1. **[MAJOR] Walker signature** — `DeepRedact` is NOT pure in-place mutation. Implementation uses a **value-returning recursive walker** (`walk(reflect.Value) reflect.Value`-style) that writes back at each container boundary: `SetMapIndex` for map entries (keys AND values; map values and interface-boxed values are non-addressable), field-set for addressable struct fields, index-set for slices, re-boxing for `reflect.Interface` (covers `Orchestration.Extra map[string]any` nesting). The nested-`Extra` walk test is the guard.
2. **[MAJOR] Batch coupling** — single PR stands (same-slice/ordering coupling), but the PR description MUST map each change → ticket ID (table: pattern/feature → ETCH-26/27/28/29/39/40-f5/40-f7) so no sibling is silently dropped.
3. **[MINOR] agent-trace.json** — add an e2e assertion that `git show <ref>:agent-trace.json` is also clean (marker present, secret absent). Trace derives from the post-redaction session, so this pins the transitive guarantee.
4. **[MINOR] Bare-AWS isolated-run limitation** — add an in-code comment on the bare pattern documenting that a 40-char secret embedded in a LONGER contiguous base64 run is deliberately missed (len==40 validator), plus one test asserting the known-miss so it reads as intentional.
5. **[MINOR] Precision/recall pinning** — add tests: `compass=abcdef123` (accepted false positive, documented) and `tokens: 4096` / `max_tokens=8192`-style prose preserved (keyword must be immediately followed by `\s*[:=]`).
