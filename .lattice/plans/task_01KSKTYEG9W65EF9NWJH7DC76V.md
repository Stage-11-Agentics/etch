# ETCH-2: Session buffer + hook handlers

## Objective

Replace the six stub hook handlers with real implementations that capture session metadata into `.cairn/sessions/<session_id>.wip.jsonl` buffer files, and finalize them into `session.json` on session end.

## Architecture

### New packages

- **`internal/capture/`** — Session buffer management + finalization
  - `buffer.go` — Create/append/read `.wip.jsonl` files in `.cairn/sessions/`
  - `session.go` — Session record struct (`cairn.session.v1`) + finalization logic (aggregate events → session.json)
  - `gitstate.go` — Git state capture (branch, HEAD, worktree detection, repo root)
  - `machine.go` — Machine identity (hostname hash, OS, arch)
  - `environ.go` — CAIRN_* env var reader, C11 env var reader, operator info

### New hook handlers (replace stubs)

- **`internal/hooks/`** — One function per hook type
  - `session_start.go` — Generates ULID session ID, captures env/git/machine state, writes first `.wip` event
  - `session_end.go` — Captures final git state, finalizes session record, writes `session.json`
  - `stop.go` — Same as session_end (stop = abnormal end)
  - `user_prompt_submit.go` — Captures prompt text (capped at 32 KiB), infers source
  - `pre_tool_use.go` — Increments tool use counters, tracks files touched
  - `post_tool_use.go` — Increments tool use counters, tracks files touched
  - `common.go` — Shared stdin reading, event struct, hook dispatch

## Data flow

```
Entire CLI → stdin JSON → entire-agent-cairn <hook_name>
  → hooks package reads stdin, builds event
  → capture.AppendEvent(sessionID, event) writes to .wip.jsonl
  → (on session_end/stop) capture.Finalize(sessionID) reads .wip, builds session.json, writes it
```

## Key design decisions

1. **Session ID strategy**: On `session_start`, generate a new ULID and write it to `.cairn/sessions/<ULID>.wip.jsonl`. The ULID is derived from the Entire-provided session_id (if present) or freshly generated.

2. **Session ID mapping**: Entire passes a `session_id` in stdin JSON for every hook. We use this to find the right `.wip` file. On `session_start` we create a mapping file `.cairn/sessions/.map/<entire_session_id>` containing the ULID. Subsequent hooks look up the ULID via this map.

3. **Buffer format**: One JSON line per event in `.wip.jsonl`:
   ```jsonl
   {"ts":"2026-...","hook":"session_start","data":{...}}
   {"ts":"2026-...","hook":"user_prompt_submit","data":{"prompt":"..."}}
   ```

4. **Finalization**: On `session_end`/`stop`, read all lines from `.wip`, aggregate into the `cairn.session.v1` schema, write `session.json` to `.cairn/sessions/<ULID>.session.json`. The ref writer (ETCH-3) will consume this.

5. **Hostname hashing**: SHA-256 of hostname by default. ETCH-5 owns the full config/redaction story; we just do the basic hash here.

6. **Prompt truncation**: Cap at 32 KiB, set `truncated: true` if exceeded.

7. **Tool use tracking**: `pre_tool_use` and `post_tool_use` both contribute to the `by_tool` map and `total_calls` counter. Files touched are extracted from Read/Write/Edit tool inputs.

8. **Git state**: Shell out to git plumbing for branch, HEAD sha, worktree detection, repo root. Capture at start and end.

9. **Output on success**: Each hook prints `{"ok":true}` to stdout (matching stub behavior) so Entire doesn't complain.

## Implementation order

1. `internal/capture/session.go` — Session record types (the full `cairn.session.v1` struct)
2. `internal/capture/buffer.go` — Buffer create/append/read/finalize
3. `internal/capture/gitstate.go` — Git state capture
4. `internal/capture/machine.go` — Machine identity capture
5. `internal/capture/environ.go` — CAIRN_*/C11/operator env readers
6. `internal/hooks/common.go` — Shared hook infrastructure (stdin reading, dispatch)
7. `internal/hooks/session_start.go` — First hook to implement
8. `internal/hooks/user_prompt_submit.go`
9. `internal/hooks/pre_tool_use.go` + `post_tool_use.go`
10. `internal/hooks/session_end.go` + `stop.go` — Finalization
11. Wire hooks into `cmd/entire-agent-cairn/main.go` — Replace stubs
12. Tests for each component

## Test strategy

- Unit tests for each `capture/` function (git state, machine, env, buffer)
- Integration tests for each hook handler (simulated stdin → verify .wip content)
- End-to-end test: session_start → prompt → tool_use → session_end → verify session.json
- All tests use `testutil.NewTestRepo()` for isolated git repos
- Test with and without CAIRN_* env vars
- Test with and without C11 env vars
- Test prompt truncation at 32 KiB boundary

## Acceptance criteria mapping

- **SPEC #1**: All six hooks implemented and processing stdin JSON ✓
- **SPEC #2**: Valid `cairn.session.v1` session.json produced ✓
- **SPEC #10**: CAIRN_* env vars captured into orchestration block ✓
- **SPEC #11**: C11 env vars captured when present ✓

## Amendments

### Post plan-review (art_01KSKWVSTEHC30EZEKNCJY5BZ3)

**[MAJOR] Schema field coverage — all OUTPUT_SPEC fields addressed:**
- `outcome` — Set all fields to null/empty. Populated at session_end from git state (commits from rev-list), but PR/CI fields are null (observed later by ETCH-7 or observation records).
- `tokens` — Set to null. The `calculate-tokens` capability is a stub (ETCH-7 wires it up). Token fields remain null in ETCH-2.
- `transcript_ref` — Map from `session_ref` in stdin JSON (session_start event). `entire_checkpoint_id` set to null. `available` checked via file existence.
- `agent.version` — Read from `version.Version` (the compiled-in version constant). This is cairn's version; Entire doesn't expose the calling agent's version in stdin. We use our own version for now.

**[MAJOR] agent.runtime determination:**
- Source: Entire's stdin JSON for session_start includes a `raw_data` field. We check for `raw_data.agent_name` or infer from `session_ref` path patterns (e.g., `.claude/` → claude-code). Fallback: check `CLAUDECODE`, `CODEX_CLI`, `GEMINI_CLI` env vars. Ultimate fallback: `"unknown"`.

**[MINOR] ULID dependency:**
- Add `github.com/oklog/ulid/v2` via `go get`.

**[MINOR] commits_produced:**
- Derive via `git rev-list <start_head>..HEAD` at session end. Empty list if HEADs match.

**[MINOR] files_touched action field:**
- Defer accurate action determination to session end: `git diff --name-status <start_sha> HEAD` gives added/modified/deleted. Tool-use events collect file paths; git diff resolves actions.

**[MINOR] Tests interleaved with implementation:**
- Tests written alongside each component, not batched at end.
