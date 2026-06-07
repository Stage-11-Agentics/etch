# Etch Hook Contract

This is the authoritative contract for everything that crosses the
`entire-agent-etch` process boundary: the hook events that drive capture, and
the install-side plugin protocol Entire uses to register etch.

Verified against **Entire CLI v0.6.3** (source tag `17720a12`) and
**Claude Code 2.1.168** (live payload captures). `scripts/smoke.sh` exercises
both dialects end-to-end and is the executable form of this contract.

---

## 1. Hook events (capture side)

Each hook subcommand reads **one JSON object on stdin** and exits 0:

```
entire-agent-etch session_start
entire-agent-etch user_prompt_submit
entire-agent-etch pre_tool_use
entire-agent-etch post_tool_use
entire-agent-etch stop
entire-agent-etch session_end
```

Two payload dialects are accepted. **Dialect-specific fields win; native
fields fill gaps.** Unknown fields are ignored. A payload missing a field the
event is expected to carry produces a **warning on stderr** — stdout stays
`{"ok":true}` and the exit code stays 0, so a malformed payload can never
break the agent's session, but it is no longer silent.

| concept | Entire HookInput dialect | Claude Code native dialect |
|---|---|---|
| session id | `session_id` | `session_id` |
| prompt | `user_prompt` | `prompt` |
| model | `raw_data.model` | *(not present in any event — derived from the transcript, see below)* |
| transcript | `session_ref` | `transcript_path` |
| tool | `tool_name`, `tool_use_id`, `tool_input` | same |
| exit reason | *(defaulted)* | `reason` |

### Model derivation (native dialect)

Native Claude Code hook payloads carry **no model field in any event**. The
model lives in the transcript JSONL referenced by `transcript_path`:
assistant entries carry `message.model`. Etch backfills the model at finalize
(`session_end`/`stop`), when the transcript is fully written. If the
transcript is missing or carries no model, the record commits with
`agent.model: null` and a stderr warning.

### Per-event examples

**session_start** — native (what a real Claude Code session sends):

```json
{
  "session_id": "c7a2d1b2-fbcd-4928-8f8d-c7f9cad7b3db",
  "transcript_path": "/Users/me/.claude/projects/-repo/c7a2d1b2.jsonl",
  "cwd": "/path/to/repo",
  "hook_event_name": "SessionStart",
  "source": "startup"
}
```

Entire dialect equivalent:

```json
{"session_id": "abc-123", "session_ref": "/tmp/transcript.jsonl", "raw_data": {"model": "claude-opus-4-8"}}
```

**user_prompt_submit** — native:

```json
{
  "session_id": "c7a2d1b2-fbcd-4928-8f8d-c7f9cad7b3db",
  "transcript_path": "/Users/me/.claude/projects/-repo/c7a2d1b2.jsonl",
  "cwd": "/path/to/repo",
  "hook_event_name": "UserPromptSubmit",
  "prompt": "Read the file f.txt and tell me its contents"
}
```

Entire dialect: `{"session_id": "abc-123", "user_prompt": "..."}`

**pre_tool_use / post_tool_use** — native (identical field names in both
dialects; `post_tool_use` adds `tool_response`, which etch ignores):

```json
{
  "session_id": "c7a2d1b2-fbcd-4928-8f8d-c7f9cad7b3db",
  "transcript_path": "/Users/me/.claude/projects/-repo/c7a2d1b2.jsonl",
  "hook_event_name": "PreToolUse",
  "tool_name": "Read",
  "tool_input": {"file_path": "/path/to/repo/f.txt"},
  "tool_use_id": "toolu_01NcaFyQAdLSuC3k8ZtJor7X"
}
```

**session_end** — native (finalizes the session into a ref):

```json
{
  "session_id": "c7a2d1b2-fbcd-4928-8f8d-c7f9cad7b3db",
  "transcript_path": "/Users/me/.claude/projects/-repo/c7a2d1b2.jsonl",
  "cwd": "/path/to/repo",
  "hook_event_name": "SessionEnd",
  "reason": "other"
}
```

Claude Code reasons observed: `clear`, `logout`, `prompt_input_exit`, `other`.
Entire dialect: `{"session_id": "abc-123"}` (reason defaults to `normal`).

**stop** — same shape as session_end. `stop` also **finalizes** the session;
it exists for runtimes that have no session-end hook. For Claude Code —
which fires Stop at every turn end and has a real SessionEnd — the installer
deliberately does **not** wire Stop (it would truncate multi-turn sessions at
the first turn).

### Behavior summary

- **Unknown fields:** ignored.
- **Missing expected fields:** stderr warning naming the expected keys and the
  payload keys received; exit 0; stdout unchanged.
- **Unknown `session_id`** (no mapping from a prior session_start): the event
  is a graceful no-op (`{"ok":true}`).
- **Hard-killed sessions** (no session_end ever fires): the `.wip.jsonl`
  buffer is finalized by crash recovery on the next `session_start` in the
  repo, after the configured `recovery_timeout_hours`.

---

## 2. Install-side plugin protocol (Entire external-agent contract)

Entire discovers `entire-agent-<name>` binaries on `$PATH` and talks to them
with JSON-over-stdout subcommands. Etch implements the install-relevant
subset, pinned to Entire v0.6.3's structs
(`cmd/entire/cli/agent/external/types.go`):

| subcommand | response | Entire struct |
|---|---|---|
| `info` | protocol v1 object (below) | `InfoResponse` |
| `detect` | `{"present": true}` | `DetectResponse` |
| `install-hooks [--local-dev] [--force]` | `{"hooks_installed": N}` | `HooksInstalledCountResponse` |
| `uninstall-hooks` | `{}` (stdout ignored by Entire) | — |
| `are-hooks-installed` | `{"installed": bool}` | `AreHooksInstalledResponse` |

`info` must report `protocol_version: 1` — Entire **silently skips** binaries
that don't (debug-level log only):

```json
{
  "protocol_version": 1,
  "name": "etch",
  "type": "etch",
  "description": "Etch — flat session metadata capture into refs/etch/sessions/*",
  "is_preview": false,
  "protected_dirs": [],
  "protected_files": [],
  "hook_names": ["session_start", "user_prompt_submit", "pre_tool_use", "post_tool_use", "stop", "session_end"],
  "capabilities": {"hooks": true, "...": false},
  "version": "0.01.001"
}
```

Only the `hooks` capability is declared: the other capabilities' Entire
protocol subcommand shapes are not implemented by this binary.

### What install-hooks actually does

In Entire's model the **plugin wires its own dispatch**. `install-hooks`
writes guarded entries into the repo's `.claude/settings.json` (Claude Code
project hook config — ordinary, committable repo state):

```
sh -c 'if ! command -v entire-agent-etch >/dev/null 2>&1; then exit 0; fi; exec entire-agent-etch session_start'
```

wired for `SessionStart`, `UserPromptSubmit`, `PreToolUse` (`*`),
`PostToolUse` (`*`), and `SessionEnd` — **not** `Stop` (see above). The
install is idempotent, `--force` re-installs, and everything that is not an
etch-managed entry round-trips untouched. Claude Code runs every hook entry
registered for an event, so etch coexists with Entire's own
`entire hooks claude-code …` entries.

At runtime the installed hooks dispatch **directly to the etch binary** with
Claude Code's native JSON — Entire is not in the dispatch path and is not
required on the machine for capture to work.

### Known Entire v0.6.3 quirks

- `entire agent add etch` fails with "Unknown agent": that code path never
  runs external-agent discovery. Use **`entire enable --agent etch`**, which
  does (and auto-persists `external_agents: true` in `.entire/settings.json`).
- External-agent discovery is gated on `external_agents: true` in
  `.entire/settings.json` for most commands; `entire enable --agent <name>`
  bypasses the gate and sets it for everything else.
