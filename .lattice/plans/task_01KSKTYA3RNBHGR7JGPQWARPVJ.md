# ETCH-1 Implementation Plan: Go module + binary scaffold

## Summary

Initialize the Go project, implement `info` and `parse-hook` subcommands with full behavior, stub all remaining subcommands, create test infrastructure (`testutil` package), and write comprehensive tests.

## Files to create

```
go.mod                                    — module forgejo.stage11.ai/s11/etch
go.sum                                    — dependency lock (oklog/ulid)
cmd/entire-agent-cairn/main.go            — entry point, subcommand dispatch
internal/info/info.go                     — info subcommand handler
internal/parsehook/parsehook.go           — parse-hook subcommand handler
internal/stubs/stubs.go                   — all stub subcommand handlers
internal/version/version.go               — version constant
internal/testutil/testutil.go             — shared test helpers
internal/testutil/testutil_test.go        — self-tests for testutil
cmd/entire-agent-cairn/main_test.go       — integration tests (binary smoke tests)
```

## Package layout rationale

Using `internal/` to prevent downstream imports of unstable packages. Each subcommand gets its own package for isolation. Stubs are grouped since they share the same pattern.

## Subcommand dispatch

Plain `os.Args[1]` switch. No flags library. Exit code 1 + stderr message for unknown subcommands. Exit code 0 for all known subcommands.

## `info` subcommand

Returns the capability JSON from PHASE0_RESULTS.md. Fields: name, version (0.01.001), capabilities map. Matches the PoC output structure but adds `version` field per delegator instructions.

## `parse-hook` subcommand

Reads stdin JSON, extracts `--hook` flag from args. Maps hook name to event fields. Returns JSON with hook_type, session_id, timestamp, and hook-specific fields (model for session_start, user_prompt for user_prompt_submit, tool fields for pre/post_tool_use).

## Stub subcommands

Each reads stdin (if present), discards it, returns `{"ok": true}`. List: session_start, session_end, user_prompt_submit, stop, pre_tool_use, post_tool_use, extract-modified-files, calculate-tokens, extract-all-modified-files, calculate-total-tokens.

## Test plan

1. `info` returns valid JSON with all required fields
2. `parse-hook --hook session-start` correctly parses session_start events
3. `parse-hook --hook user-prompt-submit` extracts user_prompt
4. `parse-hook --hook pre-tool-use` extracts tool fields
5. All stub subcommands return valid JSON without error
6. Unknown subcommand returns exit code 1
7. `go build` produces working binary
8. `testutil.NewTestRepo` creates a valid git repo
9. `testutil.RunBinary` builds and runs the binary successfully

## Dependencies

- `github.com/oklog/ulid/v2` — ULID generation (used in stubs, required by later tickets)

## Risks

None significant. This is a greenfield scaffold with well-defined protocol.
