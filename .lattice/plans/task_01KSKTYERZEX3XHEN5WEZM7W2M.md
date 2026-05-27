# ETCH-5: Security + redaction

## Summary

Add two new internal packages (`config`, `redact`) that provide configuration reading from `.cairn/settings.json` and security features: hostname hashing and secret scanning/redaction.

## Files to create

- `internal/config/config.go` — Settings struct, defaults, JSON reader
- `internal/config/config_test.go` — Tests for config reader
- `internal/redact/hostname.go` — SHA-256 hostname hashing with raw opt-in
- `internal/redact/secrets.go` — Regex-based secret pattern detection and redaction
- `internal/redact/redact.go` — Main redaction pipeline combining all scanners
- `internal/redact/redact_test.go` — Comprehensive tests for all redaction behaviors

## Design

### config package
- `Settings` struct with JSON tags matching `.cairn/settings.json` fields
- `Load(repoRoot string) (Settings, error)` — reads `<repoRoot>/.cairn/settings.json`
- Missing file returns zero-value Settings with defaults applied (no error)
- Malformed JSON returns an error
- `Defaults()` function returns Settings with default values

### redact package
- `HashHostname(hostname string) string` → `sha256:<hex>`
- `GetHostname(settings config.Settings) (hash string, raw string)` — returns hash always, raw only when `RawMachineIdentity` is true
- Named patterns slice with compiled regexes, each carrying a pattern name
- `ScanSecrets(text string) string` — replaces all matches with `[REDACTED:<name>]`
- `Redact(text string, settings config.Settings) string` — runs built-in + custom patterns
- Case-insensitive matching on key names in generic patterns

### Test coverage
- Config: valid JSON, missing file (defaults), malformed JSON (error), partial JSON (merge with defaults)
- Hostname: consistent SHA-256, raw opt-in
- Secrets: each pattern detects its test string, multiple secrets in one string, non-secrets pass through, custom patterns
