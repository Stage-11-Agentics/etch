# BUILDPLAN — Etch

> Source spec: [SPEC.md](./SPEC.md)
> Architect: agent:etch-architect
> Date: 2026-05-27

## High-level technical decisions

- **Language: Go 1.22+.** Matches Entire CLI's ecosystem. Single static binary, no runtime dependencies. The Phase 0 PoC is Python — it validated the protocol but the production binary is Go from scratch.
- **No framework.** Entire's plugin protocol is subcommand-based stdin/stdout JSON. The binary is a CLI dispatcher — `cobra` or similar would be overhead. Plain `os.Args` dispatch + `encoding/json`.
- **ULID generation:** `oklog/ulid` (Go). Lexicographic sort = time sort.
- **Git plumbing:** shell out to `git hash-object`, `git mktree`, `git commit-tree`, `git update-ref`. No libgit2 / go-git dependency — the plumbing commands are simpler, faster for this use case, and avoid CGo.
- **Secret scanning:** regex-based pattern matching against common API key formats (AWS, Anthropic, OpenAI, Stripe, generic `sk-`, `key-`, `token-` patterns). Best-effort, not exhaustive.
- **Configuration:** `.cairn/settings.json` at repo root. Minimal surface: `raw_machine_identity`, `local_only_fields[]`, `archive_threshold_days`, `redaction_patterns[]`.

## Architecture

```
entire-agent-cairn (Go binary on $PATH)
├── cmd/          — subcommand dispatch (info, parse-hook, hooks, capabilities)
├── capture/      — session buffer management (.wip files)
├── refs/         — git ref writer (hash-object → mktree → commit-tree → update-ref)
├── schema/       — session.json + agent-trace.json serialization
├── redact/       — secret scanning + hostname hashing
├── config/       — .cairn/settings.json reader
└── recovery/     — orphaned .wip detection + partial record commit
```

Data flow: Entire dispatches hook events via stdin JSON → `entire-agent-cairn` routes to the appropriate handler → handler appends to `.cairn/sessions/<uuid>.wip.jsonl` buffer → on session end, the buffer is finalized into `session.json` + `agent-trace.json` → ref writer creates an orphan commit and points `refs/cairn/sessions/<ULID>` at it → `.wip` file is deleted.

See [CAIRN_PLAN.md](./CAIRN_PLAN.md) for architecture diagrams (Mermaid) and [OUTPUT_SPEC.md](./OUTPUT_SPEC.md) for the full session record schema and scenario variants.

## Tickets

### Wave 1: Foundation (no dependencies)

1. **Go module + binary scaffold** (ETCH-1)
   - What: Initialize Go module, implement `info` and `parse-hook` subcommands, wire up subcommand dispatch. Binary must be discoverable by Entire as `entire-agent-cairn` on PATH. Include `go build` + basic smoke test.
   - Acceptance criteria: SPEC #1 (partial — info + parse-hook subcommands)
   - Depends on: —

### Wave 2: Core capture (depends on ETCH-1)

2. **Session buffer + hook handlers** (ETCH-2)
   - What: Implement all six hook handlers (`session_start`, `session_end`, `user_prompt_submit`, `stop`, `pre_tool_use`, `post_tool_use`). Each handler reads stdin JSON, extracts fields per OUTPUT_SPEC.md schema, and appends to `.cairn/sessions/<uuid>.wip.jsonl`. On `session_start`: read CAIRN_* env vars, git state, machine identity, c11 env vars. On `session_end`/`stop`: read final git state, finalize the session record.
   - Acceptance criteria: SPEC #1, #2, #10, #11
   - Depends on: ETCH-1

3. **Git ref writer** (ETCH-3)
   - What: Given a finalized `session.json` + `agent-trace.json`, create an orphan commit via git plumbing (`hash-object` → `mktree` → `commit-tree` → `update-ref`) and point `refs/cairn/sessions/<ULID>` at it. Human-readable commit message with session summary.
   - Acceptance criteria: SPEC #3
   - Depends on: ETCH-1

4. **Crash recovery** (ETCH-4)
   - What: On session_start, scan `.cairn/sessions/` for orphaned `.wip.jsonl` files (no active process, or last event older than configurable timeout, default 4h). Build partial `session.json` with `status: incomplete`, `exit_reason: crash`. Commit via ref writer and clean up the `.wip` file.
   - Acceptance criteria: SPEC #6
   - Depends on: ETCH-1

5. **Security + redaction** (ETCH-5)
   - What: Hostname hashing (SHA-256 by default, raw opt-in via `.cairn/settings.json`). Secret scanning on prompt text and tool-use fields before commit — regex patterns for common API key formats. Redacted content replaced with `[REDACTED:<pattern-name>]`. Config reader for `.cairn/settings.json`.
   - Acceptance criteria: SPEC #7, #8
   - Depends on: ETCH-1

6. **Agent Trace emission** (ETCH-6)
   - What: Serialize each session's data into `agent-trace.json` alongside `session.json` in the commit tree. Format per the Cursor Agent Trace RFC (version 1.0). Include `agent_id`, `model`, `session_id`, `files`, `timestamp`.
   - Acceptance criteria: SPEC #9
   - Depends on: ETCH-1

### Wave 3: Integration (depends on Wave 2 core)

7. **End-to-end wiring + refspec config** (ETCH-7)
   - What: Wire session_end handler to call ref writer after finalization. Implement refspec configuration helper (`cairn setup-refspec` or documented manual config). Test the full lifecycle: hook fires → buffer → finalize → ref write → push → fetch on another clone. Include capability subcommands (`extract-modified-files`, `calculate-tokens`).
   - Acceptance criteria: SPEC #1 (complete), #5
   - Depends on: ETCH-2, ETCH-3, ETCH-5, ETCH-6

### Wave 4: Validation

8. **Density test** (ETCH-8)
   - What: Run 20 concurrent Claude Code sessions on Hyperion with Etch enabled. Verify: all sessions produce valid refs, no collisions, no dropped records, refs push to Forgejo and fetch on Atlas. Include a simulated crash test (kill agent mid-session, verify recovery).
   - Acceptance criteria: SPEC #4, #5, #6 (validated under load)
   - Depends on: ETCH-7

### Deferred (Phase 2+)

9. **cairn query CLI** (ETCH-9)
   - What: Read all `refs/cairn/sessions/*`, filter/aggregate, output JSON or table.
   - Depends on: ETCH-7
   - Phase: 2

10. **cairn index** (ETCH-10)
    - What: Materialize a queryable index from all session refs for large repos.
    - Depends on: ETCH-9
    - Phase: 2

11. **Ref lifecycle + compaction** (ETCH-11)
    - What: Archive refs older than threshold into `refs/cairn/archive/<YYYY-Q>`, delete individual session refs.
    - Acceptance criteria: SPEC #12
    - Depends on: ETCH-7
    - Phase: 3

12. **Lattice skill update** (ETCH-12)
    - What: Update Lattice orchestrator skill to export `CAIRN_*` env vars when dispatching delegators.
    - Depends on: ETCH-8 (tested end-to-end first)
    - Phase: 3

## Dependency graph

```
ETCH-1 (scaffold)
├── ETCH-2 (session buffer + hooks)
├── ETCH-3 (ref writer)
├── ETCH-4 (crash recovery)
├── ETCH-5 (security/redaction)
└── ETCH-6 (agent trace)
        │
        └── ETCH-7 (end-to-end wiring) ← depends on 2,3,5,6
                │
                └── ETCH-8 (density test) ← depends on 7
```

Wave 2 tickets (ETCH-2 through ETCH-6) are fully parallel once ETCH-1 lands. ETCH-7 integrates them. ETCH-8 validates the integrated system.

## Field assignments

| Field | Schema-owner | Writer | Readers |
|---|---|---|---|
| session.json (full record) | ETCH-2 | ETCH-2 (buffer), ETCH-7 (finalize) | ETCH-3, ETCH-8 |
| agent-trace.json | ETCH-6 | ETCH-6 | ETCH-3, ETCH-8 |
| .wip.jsonl buffer | ETCH-2 | ETCH-2 (append per hook) | ETCH-4 (recovery), ETCH-7 (finalize) |
| refs/cairn/sessions/* | ETCH-3 | ETCH-3 (via ETCH-7 wiring) | ETCH-8, ETCH-9, ETCH-10 |
| .cairn/settings.json | ETCH-5 | operator (manual) | ETCH-5, ETCH-2 |
| hostname_hash | ETCH-5 | ETCH-5 | ETCH-2 |
| redacted content | ETCH-5 | ETCH-5 | ETCH-2, ETCH-7 |

## Open questions / deferred decisions

1. **Observation records** (`refs/cairn/observations/<uuid>`) — designed in OUTPUT_SPEC.md but not ticketed for this run. Build if time permits after ETCH-8.
2. **Synthetic data generator** (`cairn synth`) — specified in OUTPUT_SPEC.md §5. Useful for testing query/index but not needed for capture validation. Defer to Phase 2.
3. **`cairn install` CLI** — one-command setup (refspec config + settings scaffold). Could be part of ETCH-7 or its own ticket. Keep it in ETCH-7 for now.
