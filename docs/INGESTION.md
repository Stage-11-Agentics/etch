# Etch Ingestion — Two Paths to One Record

Etch promises a record for **every** AI agent session in a repository. Agent
runtimes don't share a hook mechanism — Claude Code has rich declarative hooks,
OpenCode has a plugin event bus, Codex exposes only a coarse `notify` callback,
and some runtimes expose nothing live at all. A single hook integration cannot
deliver "every session." So etch ingests through **two paths** that converge on
one engine.

```
                 ┌──────────────────────────────────────────────┐
   live hooks ──▶│                                              │
  (Claude Code,  │   capture.Session  ──▶  commitRecord()       │──▶ refs/etch/sessions/<ULID>
   OpenCode)     │   the single record shape + commit boundary  │     redact · trace · CAS dedup-on-ULID
                 │                                              │
   import     ──▶│   transcript parsers build the same Session, │
  (any runtime   │   then ride the same boundary                │
   with a log)   └──────────────────────────────────────────────┘
```

The two paths are **ingestion front-ends only**. They both produce a
`capture.Session` and hand it to `commitRecord` — the function the live finalize
path and crash recovery already share. There is exactly one record schema
(`etch.session.v1`), one redaction pass, one trace emitter, one ref writer. A
new ingestion path is a parser, never a second engine.

## The two paths

### 1. Live hooks — the preferred path

Low latency, captures things that never reach a transcript (process identity for
crash recovery, c11 surface/tab context, session-start environment, real-time
tool events), and gets `.wip.jsonl` crash-recovery semantics. This is the
premium experience and it is worth a bespoke per-runtime installer **only where
the runtime's hook surface earns it**.

- **Claude Code** — `install-hooks` writes guarded entries into
  `.claude/settings.json`. Full fidelity. Shipped.
- **OpenCode** — a TypeScript plugin (`.opencode/plugins/etch.ts`, written by
  `entire-agent-etch install-opencode`) subscribes to OpenCode's message/tool/
  session events and shells out to the binary with the same stdin contract. Full
  fidelity, different integration shape (a plugin file, not a settings writer).
  Shipped. See [OpenCode plugin](#opencode-first-class-plugin).

### 2. Import — the universal floor

Post-hoc. Reads whatever transcript/session log the runtime already wrote to
disk, parses it into a `Session`, and commits it. This is the path that earns
the word "every": any runtime that persists a session log can be imported,
including ones with no usable live hook surface (Codex, Gemini, Cursor).

```
entire-agent-etch import [--runtime claude-code|codex] [--repo PATH] [--since RFC3339] [--dry-run]
```

Import is **idempotent** and **never competes with hooks** (see
[Dedup](#dedup-hooks-always-win)).

## Provenance: every record says how it was captured

Because fidelity varies by path and runtime, provenance is **in the record**,
not just in docs. Every `etch.session.v1` record carries a `capture` block:

```json
"capture": {
  "method": "hooks",          // "hooks" | "import"
  "fidelity": "full",         // "full" | "session_only"
  "source": "claude-code-transcript"   // optional: where an import came from
}
```

- `method` — which path produced this record.
- `fidelity` — `full` when tool-level events were captured; `session_only` when
  only session boundaries were available (e.g. a Codex `notify`-driven record,
  or a transcript that carried no tool calls).
- `source` — free-form origin tag for imports (e.g. the parser name).

This makes the honesty queryable: `query --capture-method import` shows exactly
which sessions came from the floor rather than live hooks, and a repo whose
Claude sessions are all `import` is a signal that live hooks silently broke.

Live hook records (and crash-recovered records) are stamped `hooks` / `full` by
the reducer, so the field is universal — no record is ever missing it.

## Dedup: hooks always win

Claude Code (and OpenCode) have **both** a live hook path and a transcript on
disk. Without dedup, an import run would double-record every session a hook
already captured. The rule:

> **Import only fills sessions that have no existing record. Hooks win.**

The join key is `agent_session_id` — the upstream runtime's own session id,
which every hook record already preserves (ETCH-23) and which the transcript
also carries. Before importing, `import` enumerates the `agent_session_id` of
every committed session ref (`refs/etch/sessions/*` and `refs/etch/local/*`) and
skips any transcript whose upstream id is already present. Because imported
records are themselves written with `agent_session_id`, re-running `import` is
naturally idempotent — the second run sees the first run's records and skips
them.

Dedup is enforced from the first import commit, never bolted on later: a poisoned
(double-counted) dataset is far more expensive to clean than to prevent.

## Runtime support matrix

| Runtime | Live path | Import path | Fidelity | Status |
|---|---|---|---|---|
| Claude Code | hooks (`.claude/settings.json`) | transcript (`~/.claude/projects/**/*.jsonl`) | full | hooks shipped; import shipped |
| OpenCode | plugin (`.opencode/plugins/etch.ts`) → binary | `storage/` session+message JSON | full | live shipped; import planned |
| Codex | none usable (`notify` is coarse) | rollout JSONL (`~/.codex/sessions/**/*.jsonl`) | full via import | import shipped |
| Gemini CLI | TBD (extension hooks) | transcript (when format confirmed) | TBD | planned |
| anything else | — | transcript import (needs a parser) | full/session-only | per-runtime |

"Every session on a supported runtime" — and the import path is what keeps the
list of supported runtimes from being gated on hook-surface richness. The honest
scope is published here rather than implied by a Claude-only installer.

## Import parser contract

Each runtime's import parser implements one interface: given a transcript file,
produce a `*capture.Session` with `capture.method = import` and the correct
`agent_session_id`, `agent.runtime`, model, timing, prompt, tool-use summary,
and files-touched it can recover from the log. Everything downstream — redaction,
trace, dedup, ref write — is shared. Fidelity is `full` when the parser found
tool calls, `session_only` otherwise. Machine/operator/git identity is captured
from the **current** machine, which is correct because a transcript on disk here
means the session ran here.

Adding a runtime to the import path is: write a parser, register it, done. No
schema change, no engine change.

## OpenCode first-class plugin

OpenCode has no declarative hook file, but it has a real plugin/event system.
The etch integration (`internal/install/opencode/etch.ts`, embedded in the
binary) is a small TypeScript plugin that maps OpenCode's `chat.message`,
`tool.execute.before/after`, and session-lifecycle events to
`entire-agent-etch <subcommand>` with the same stdin JSON contract the Claude
Code hooks use (see `docs/HOOK_CONTRACT.md`). The plugin is the dispatch; etch is
unchanged. `entire-agent-etch install-opencode` drops it into
`.opencode/plugins/etch.ts` (committable repo state), the OpenCode analog of
`install-hooks` writing `.claude/settings.json`. The plugin no-ops without the
binary on PATH, so committing it never forces capture on a collaborator.

A session is finalized on `session.deleted` and, for any still-open session, on
the plugin's `dispose` (OpenCode shutdown); anything missed is picked up by
etch's `.wip` crash recovery. `session.idle` is deliberately not a finalizer —
it fires every turn and would truncate multi-turn sessions (the same reason the
Claude installer skips `Stop`).

Two OpenCode-specific gotchas, recorded so they aren't rediscovered: the plugin
directory is `.opencode/plugins/` (plural — distinct from the `opencode plugin`
CLI subcommand), and Bun's shell stdin redirect must be fed a `Buffer`
(`< ${string}` is treated as a *filename*, silently capturing nothing).

This is the one ingestion artifact that lives partly outside the Go binary (a TS
plugin file). It earns the cost because OpenCode is a first-class target where
live, full-fidelity capture matters; runtimes that don't clear that bar get the
import floor instead.
