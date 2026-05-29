# Plan: ETCH-6 — Agent Trace emission

## Goal

Add Agent Trace RFC v1.0 serialization to the `internal/schema/` package so that every session record can produce an `agent-trace.json` alongside `session.json`.

## Files to create

### `internal/schema/session.go`
Define the Session struct and its sub-types (Agent, Timing, FileEntry) — the subset needed for the `SessionToAgentTrace` conversion. ETCH-2 will extend this later with the full session capture logic; we define the canonical types here since `schema/` owns session.json serialization per BUILDPLAN.

### `internal/schema/trace.go`
- `AgentTrace` struct with `Version` + `Traces []TraceEntry`
- `TraceEntry` struct with `AgentID`, `Model`, `SessionID`, `Files`, `Timestamp`
- `SessionToAgentTrace(session *Session) *AgentTrace` — maps session fields to trace fields
- Timestamp logic: use `EndedAt` if non-nil, fall back to `StartedAt`
- Files: extract `Path` from each `FileEntry` in `FilesTouched`

### `internal/schema/trace_test.go`
Test cases:
1. Complete session → valid trace with all fields
2. `version` is always `"1.0"`
3. Files list correctly extracted from FilesTouched
4. Empty FilesTouched → empty (not nil) files array
5. Incomplete session (EndedAt nil) → uses StartedAt
6. JSON round-trip: marshal → unmarshal → compare
7. Single trace entry per session (len(Traces) == 1)

## Constraints
- No external dependencies (pure Go stdlib + schema types)
- No changes to existing packages
- ETCH-3 (ref writer) will consume AgentTrace to create the second blob in the tree
