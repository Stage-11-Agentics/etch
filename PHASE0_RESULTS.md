# Cairn Phase 0: Validation Gate Results

**Date:** 2026-05-26
**Status:** Both gates PASS

---

## Gate 1: Remote Ref Compatibility

**Result: PASS — both GitHub and Forgejo accept custom ref namespaces.**

### Test procedure

1. Created a test commit with a `session.json` blob in the `backstage` repo (has both GitHub and Forgejo remotes)
2. Wrote it to `refs/cairn/sessions/phase0-test-6e834de7-e350-41a9-805d-8d003291304a`
3. Pushed to both remotes
4. Deleted the local ref
5. Fetched it back from GitHub
6. Verified content integrity (commit → tree → blob round-tripped bit-for-bit)
7. Cleaned up: deleted test refs from both remotes and local

### Results by remote

| Remote | Type | URL pattern | Push | Fetch | Delete | Notes |
|--------|------|-------------|------|-------|--------|-------|
| GitHub | `origin` | `https://github.com/Stage-11-Agentics/video-call.git` | OK | OK | OK | No restrictions on `refs/cairn/*` namespace |
| Forgejo | `forgejo` | `git@forgejo.stage11.ai:s11/video-call.git` | OK | OK | OK | No restrictions on `refs/cairn/*` namespace |

### Observations

- Both platforms accept arbitrary ref namespaces without special configuration
- The `refs/cairn/sessions/*` namespace does not conflict with any platform-reserved namespace
- Refspec-based push/fetch works: `refs/cairn/sessions/*:refs/cairn/sessions/*`
- Ref deletion via `git push --delete` works on both platforms (needed for the compaction/archival lifecycle)
- No fallback strategy needed — the primary architecture holds

### Recommended refspec config (confirmed working)

```gitconfig
[remote "origin"]
    push = refs/cairn/sessions/*:refs/cairn/sessions/*
    fetch = refs/cairn/sessions/*:refs/cairn/sessions/*
```

---

## Gate 2: Entire Plugin Protocol Coverage

**Result: PASS — protocol covers the hook layer Cairn needs. Gaps exist only in fields Cairn captures directly (git state, machine identity, operator).**

### Protocol overview

Entire's external agent protocol is subcommand-based (stdin/stdout JSON, stateless). Discovery: any `entire-agent-<name>` binary on `$PATH` is auto-registered. Protocol version: 1.

The protocol dispatches six hook types to plugins:

| Hook | Entire constant | Data available |
|------|----------------|----------------|
| `session_start` | `HookSessionStart` | session_id, session_ref (transcript path), timestamp, model (via Event.model) |
| `session_end` | `HookSessionEnd` | session_id, session_ref, timestamp, model |
| `user_prompt_submit` | `HookUserPromptSubmit` | session_id, user_prompt, timestamp |
| `stop` | `HookStop` | session_id, session_ref, timestamp |
| `pre_tool_use` | `HookPreToolUse` | session_id, tool_name, tool_use_id, tool_input (raw JSON), timestamp |
| `post_tool_use` | `HookPostToolUse` | session_id, tool_name, tool_use_id, tool_input, tool_response, timestamp |

### Cairn metadata field → Protocol mapping

| Cairn field | Protocol source | Coverage |
|-------------|----------------|----------|
| Schema version | Hardcoded by Cairn (`cairn.session.v1`) | N/A — not from protocol |
| **Prompt** | `HookInput.user_prompt` from `user_prompt_submit` hook; `Event.prompt` from `parse-hook` | **FULL** |
| **Transcript ref** | `HookInput.session_ref` (transcript path) — available on every hook | **FULL** |
| Orchestration pattern | Cairn reads env vars (`LATTICE_TICKET_ID`, etc.) directly | N/A — not from protocol |
| **Agent runtime + model** | `info` → agent name/type; `Event.model` from `parse-hook` on session_start | **FULL** — Claude Code sends model in SessionStart payload |
| **Tool use events** | `pre_tool_use` / `post_tool_use` hooks: tool_name, tool_use_id, tool_input, tool_response | **FULL** |
| **Files touched** | `extract-modified-files` (transcript_analyzer capability); `AgentSession.modified_files/new_files/deleted_files` | **FULL** — but Cairn should also diff git at session boundaries for ground truth |
| **Token usage + cost** | `calculate-tokens` (token_calculator capability): input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, api_call_count, subagent_tokens | **FULL** |
| **Session timing** | `HookInput.timestamp` on session_start and session_end/stop | **FULL** |
| **Exit reason** | Event type from `parse-hook` (SessionEnd=5, TurnEnd=3); no explicit exit_reason enum | **PARTIAL** — protocol distinguishes session_end vs stop but doesn't carry `normal`/`token_limit`/`error`/`user_kill`/`timeout`. Cairn must infer from context or read agent-native data |
| Machine identity | Not in protocol | **CAIRN-DIRECT** — hostname, fingerprint from OS |
| Operator | Not in protocol | **CAIRN-DIRECT** — git user.name/email, OS username |
| Git state (start) | Not in protocol | **CAIRN-DIRECT** — branch, HEAD SHA, worktree path from git |
| Git state (end) | Not in protocol | **CAIRN-DIRECT** — branch, HEAD SHA, commits produced from git |
| Outcome (observed) | Not in protocol | **CAIRN-DIRECT** — commit SHAs, PR number, CI status from git/gh |

### Protocol capabilities Cairn should declare

```json
{
  "hooks": true,
  "transcript_analyzer": true,
  "compact_transcript": false,
  "token_calculator": true,
  "text_generator": false,
  "hook_response_writer": false,
  "subagent_aware_extractor": true
}
```

- `hooks: true` — Cairn needs all six lifecycle hooks
- `transcript_analyzer: true` — enables `extract-modified-files`, `extract-prompts`, `extract-summary`
- `token_calculator: true` — enables `calculate-tokens` for per-session token accounting
- `subagent_aware_extractor: true` — enables `extract-all-modified-files` and `calculate-total-tokens` across subagents (critical at Stage 11 density)
- `compact_transcript: false` — Cairn doesn't need to produce Entire's compact format; it writes its own refs

### Gaps and mitigations

| Gap | Impact | Mitigation |
|-----|--------|------------|
| No explicit `exit_reason` in protocol | Cairn can't distinguish `token_limit` from `normal` from `user_kill` purely from protocol events | Read Claude Code's native transcript for the stop event reason; degrade gracefully to `unknown` for agents that don't expose it |
| No `ToolResponse` in `pre_tool_use` HookInput | Expected — tool hasn't run yet | Use `post_tool_use` for tool result data |
| No cost/pricing data | Token counts are available but not dollar amounts | Cairn applies its own pricing table to token counts; this is better as a Cairn concern anyway since pricing changes independently of the agent |
| `extract-modified-files` reads from transcript, not git | May miss files modified outside the agent's tool calls | Supplement with `git diff` at session boundaries (already planned) |

### PoC binary

A working `entire-agent-cairn` PoC is at `./entire-agent-cairn`. It:
- Implements all required and capability-gated subcommands
- Logs every invocation (subcommand, args, stdin, env) to `/tmp/cairn-poc-events.jsonl`
- Returns valid protocol-conforming JSON for every subcommand
- Tested with simulated `session_start`, `user_prompt_submit`, and `pre_tool_use` events

The PoC is a Python script for speed; the production binary should be Go (matching Entire's ecosystem and avoiding a Python runtime dependency).

### Key finding: the protocol is a good substrate

The Entire plugin protocol covers the agent-runtime-specific data (hooks, transcripts, tokens, tool events) that would be expensive to replicate. The data it doesn't cover (git state, machine identity, operator, outcomes) is all stuff Cairn captures directly from the environment — these don't need to come through the agent hook layer at all. No direct agent hooks (bypassing Entire) are needed for any Cairn metadata field.

---

## Decision: Proceed to Phase 1

Both validation gates pass. The architecture holds as designed:
- `refs/cairn/sessions/*` works on all target remotes (GitHub, Forgejo)
- Entire's plugin protocol covers the hook layer; gaps are in fields Cairn owns directly
- No fallback strategies needed; no architectural changes required
