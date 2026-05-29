# ETCH-4: Crash recovery

## Summary

Implement crash recovery for orphaned `.wip.jsonl` files. When a new session starts, scan `.cairn/sessions/` for `.wip.jsonl` files whose writing process is dead or whose last event is older than a configurable timeout (default 4h). Build partial `session.json` with `status: incomplete`, `exit_reason: crash`, commit via ref writer interface (stubbed until ETCH-7 wiring), and clean up the `.wip` file.

## Files to create

- `internal/recovery/recovery.go` — core recovery logic
- `internal/recovery/recovery_test.go` — comprehensive tests
- `internal/schema/session.go` — session record types (shared with future ETCH-2)

## Design

### Package: `internal/recovery`

Three exported functions:

1. **`ScanOrphaned(sessionsDir string, timeout time.Duration) ([]OrphanedWIP, error)`**
   - Lists all `*.wip.jsonl` files in sessionsDir
   - For each file: reads last line to extract timestamp; checks if PID from session_start event is alive
   - Returns files where process is dead OR last event is older than timeout
   - Skips files that can't be read (log warning, continue)

2. **`RecoverSession(wipPath string) (*Session, error)`**
   - Reads the `.wip.jsonl` line by line
   - Builds a partial Session with data from whatever events exist
   - Sets `status: "incomplete"`, `exit_reason: "crash"`
   - Sets `timing.ended_at: null`, `timing.duration_ms: null`
   - Handles partial/corrupt files gracefully (uses what's available)

3. **`CleanupWIP(wipPath string) error`**
   - `os.Remove(wipPath)` after successful commit

### Package: `internal/schema`

Minimal session types needed by recovery:

- `Session` struct matching `cairn.session.v1` schema
- JSON serialization with correct null handling (pointer fields)

### WIP file format

Each line is a JSON object representing a hook event:
```jsonl
{"hook_type":"session_start","session_id":"01JWB...","timestamp":"2026-05-26T14:32:08.441Z","model":"claude-opus-4-7","pid":12345,...}
{"hook_type":"user_prompt_submit","session_id":"01JWB...","timestamp":"2026-05-26T14:32:10.000Z","prompt":"Fix the bug..."}
{"hook_type":"pre_tool_use","session_id":"01JWB...","timestamp":"2026-05-26T14:33:00.000Z","tool_name":"Read",...}
```

### Orphan detection heuristics

1. **PID check (primary):** Extract PID from session_start event. If PID is present and process is not running → orphaned.
2. **Timeout check (fallback):** If PID is absent or check fails, compare last event timestamp against `now - timeout`. If older → orphaned.
3. **Both dead and old:** If PID is dead, mark orphaned regardless of timeout.

### Ref writer interface

Define a `RefWriter` interface that the recovery package calls to commit the partial record. For now, provide a no-op implementation. ETCH-7 wires in the real one.

```go
type RefWriter interface {
    WriteSessionRef(repoDir string, session *Session) error
}
```

### Configuration

Read `recovery_timeout_hours` from `.cairn/settings.json` (default 4). The recovery package accepts timeout as a `time.Duration` parameter — config reading is the caller's concern.

## Test plan

1. **TestScanOrphaned_DetectsOldWIP** — create .wip with old timestamp, verify detected
2. **TestScanOrphaned_IgnoresRecentWIP** — create .wip with recent timestamp and living PID, verify skipped
3. **TestScanOrphaned_DetectsDeadPID** — create .wip with dead PID, verify detected even if recent
4. **TestScanOrphaned_EmptyDir** — no .wip files, returns empty slice
5. **TestScanOrphaned_CorruptFile** — create .wip with garbage content, verify skipped (not crashed)
6. **TestRecoverSession_FullWIP** — .wip with session_start + prompts + tool_use → valid partial Session
7. **TestRecoverSession_MinimalWIP** — .wip with only session_start → partial Session with minimal fields
8. **TestRecoverSession_EmptyFile** — empty .wip → error returned
9. **TestRecoverSession_CorruptLines** — mix of valid and corrupt lines → uses valid lines
10. **TestRecoverSession_SetsIncompleteStatus** — verify status/exit_reason/timing nulls
11. **TestCleanupWIP** — verify file removed
12. **TestIntegration_FullRecoveryFlow** — create .wip, run scan, recover, verify Session, cleanup
