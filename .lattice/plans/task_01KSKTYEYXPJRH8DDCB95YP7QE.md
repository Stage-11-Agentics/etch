# ETCH-7: End-to-end wiring + refspec config

## Summary

Wire all Wave 2 components into a working end-to-end flow: hook fires → buffer → finalize → redact → trace → ref write → cleanup. Add capability subcommands and refspec configuration.

## Current State

- `session_end`/`stop` handlers call `capture.Finalize()` which writes `.session.json` to disk, but **does not** write a git ref, generate agent-trace.json, apply redaction, or clean up the .wip file.
- `session_start` does **not** call crash recovery.
- `extract-modified-files`, `calculate-tokens` are stubs returning `{"ok":true}`.
- No `setup-refspec` subcommand exists.

## Type Mapping Challenge

Two parallel Session types exist: `capture.Session` (used by buffer/finalize) and `schema.Session` (used by recovery/trace). They share identical JSON field names but differ in Go types (value vs pointer for Orchestration, Machine, Operator, Outcome, ToolUse; `string` vs `*string` for Timing.StartedAt; `*int64` vs `int64` for token fields). **Solution:** JSON round-trip conversion — marshal `capture.Session` → unmarshal into `schema.Session`. This works because the JSON tags are identical.

## Implementation Plan

### 1. New file: `internal/hooks/commit.go`

Central wiring that orchestrates the session→ref pipeline:

```go
// commitSession takes a finalized capture.Session, applies redaction,
// generates trace, writes the git ref, and cleans up temp files.
func commitSession(repoRoot string, session *capture.Session, entireSessionID string) error
```

Steps inside `commitSession`:
1. Load settings via `config.Load(repoRoot)`
2. Apply `redact.Redact()` to `session.Prompt.Text` if non-nil
3. Marshal the redacted `capture.Session` to JSON → `sessionJSON`
4. JSON round-trip: unmarshal `sessionJSON` into `schema.Session`
5. Call `schema.SessionToAgentTrace()` → marshal to JSON → `traceJSON`
6. Build `refs.RefMeta` from session fields
7. Call `refs.WriteSessionRef(repoRoot, session.SessionID, sessionJSON, traceJSON, meta)`
8. Call `capture.RemoveWip(repoRoot, session.SessionID)`
9. Remove the `.session.json` file (data is now in the ref)
10. Call `capture.CleanupMapping(repoRoot, entireSessionID)`

### 2. New file: `internal/hooks/refwriter.go`

Implement `recovery.RefWriter` interface for crash recovery:

```go
type cairnRefWriter struct {
    repoDir string
}

func (w *cairnRefWriter) WriteSessionRef(repoDir string, session *schema.Session) error
```

This adapter:
1. Loads settings, applies redaction to prompt
2. Marshals session to JSON
3. Generates agent-trace.json
4. Builds RefMeta
5. Calls `refs.WriteSessionRef()`

### 3. Modify `internal/hooks/session_end.go`

In `runEnd()`, after `capture.Finalize()` succeeds:
- Call `commitSession(repoRoot, session, ev.SessionID)`
- On error, log but still print OK (don't break the agent's workflow)

### 4. Modify `internal/hooks/session_start.go`

After `capture.EnsureDirs()`, before generating the ULID:
- Read timeout from settings
- Call `recovery.RecoverAll(sessionsDir, repoRoot, timeout, &cairnRefWriter{repoRoot})`
- Log recovered count, don't fail on recovery errors

### 5. New package: `internal/commands/`

**`extract_modified_files.go`:**
- Takes session ID from `os.Args[2]` (or reads from stdin)
- Reads `session.json` from `refs/cairn/sessions/<id>` via `git show <ref>:session.json`
- Parses JSON, outputs `files_touched` as JSON array

**`calculate_tokens.go`:**
- Same ref-reading pattern
- Outputs token counts as JSON object

**`setup_refspec.go`:**
- Runs `git config --get-all remote.origin.push` to check existing refspecs
- If `refs/cairn/sessions/*` not present, adds it via `git config --add`
- Same for fetch refspec
- Prints confirmation

### 6. Modify `cmd/entire-agent-cairn/main.go`

Replace stub cases with real command handlers:
```go
case "extract-modified-files":
    err = commands.RunExtractModifiedFiles(os.Args[2:])
case "calculate-tokens":
    err = commands.RunCalculateTokens(os.Args[2:])
case "setup-refspec":
    err = commands.RunSetupRefspec()
```

### 7. End-to-end test: `internal/hooks/e2e_test.go`

Full lifecycle test that:
1. Creates a temp repo with an initial commit
2. Simulates: session_start → user_prompt_submit (with secret in text) → pre_tool_use → post_tool_use → session_end
3. Verifies: .wip file does NOT exist after session_end (cleaned up)
4. Verifies: `refs/cairn/sessions/<ULID>` exists via `git show-ref`
5. Reads `session.json` from the ref via `git show`, validates schema fields
6. Reads `agent-trace.json` from the ref, validates structure
7. Verifies: secret in prompt text is redacted
8. Verifies: session_start triggers crash recovery (create a fake orphaned .wip, then session_start → verify it got committed)

### 8. Tests for capability subcommands

- Test `extract-modified-files` reads from a ref and returns correct files
- Test `calculate-tokens` reads from a ref and returns correct counts
- Test `setup-refspec` adds refspec config idempotently

## File Changes Summary

| File | Action |
|------|--------|
| `internal/hooks/commit.go` | **New** — session→ref pipeline |
| `internal/hooks/refwriter.go` | **New** — recovery.RefWriter adapter |
| `internal/hooks/session_end.go` | **Modify** — call commitSession after Finalize |
| `internal/hooks/session_start.go` | **Modify** — call crash recovery |
| `internal/commands/extract_modified_files.go` | **New** |
| `internal/commands/calculate_tokens.go` | **New** |
| `internal/commands/setup_refspec.go` | **New** |
| `cmd/entire-agent-cairn/main.go` | **Modify** — wire new subcommands |
| `internal/hooks/e2e_test.go` | **New** — end-to-end integration tests |

## Risk Assessment

- **Type conversion via JSON round-trip**: Safe — field names match, Go handles null→zero gracefully.
- **Redaction scope**: Only prompt text for now. Tool-use data doesn't contain raw content in our capture (just tool names and file paths). 
- **Error handling in session_end**: Ref-write failures should be logged but NOT propagate as errors to the agent runtime. A failed ref write is metadata loss, not a session-breaking event.
- **Crash recovery timing**: Running recovery in session_start adds latency. For repos with many orphaned .wip files this could be slow. Acceptable for v1; can be made async later.
