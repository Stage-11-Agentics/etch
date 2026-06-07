# SPEC — Etch

## What this is

Etch captures flat metadata about every AI agent session in a repository and stores it as immutable git refs that travel with push/fetch. Built on Entire CLI's hook substrate. Designed for 60–80+ concurrent agents across multiple machines. The primary consumer is the next AI agent starting work in a codebase — not a human opening a dashboard.

Formerly Cairn / Forge++. The binary is `entire-agent-etch` and env vars use the `ETCH_*` namespace.

## Goals

- Capture every agent session's metadata: prompt, transcript ref, orchestration context, agent runtime + model, tool use, files touched, tokens/cost, timing, machine identity, operator, git state, and observed outcomes.
- Store each session as an immutable orphan commit in `refs/etch/sessions/<ULID>` — zero-contention writes at any concurrency level.
- Transport via standard git push/fetch using refspec configuration — no external database, no cloud service.
- Emit Agent Trace (`agent-trace.json`) alongside every session record for free interop with the Cursor/Cognition/Anthropic ecosystem.
- Handle crash recovery: orphaned `.wip` buffer files become partial records on next invocation.
- Support redaction of secrets and hashing of machine identity by default.

## Non-goals

- **No analysis engine.** Etch captures and stores. Correlation queries ("which prompts correlate with clean CI?") are future consumers of the data.
- **No hierarchical workflow model.** Records are flat. Structure emerges from queries via shared identifiers.
- **No dashboard.** Data lives in git. Query with git commands, jq, or future tooling.
- **No hook infrastructure.** Entire CLI handles agent runtime hooks. Etch consumes via the plugin protocol.
- **No outcome binding.** Outcome fields are observed at session end, not computed correlations. Late-arriving outcomes go to `refs/etch/observations/`.
- **Read path (query/index) is not a priority** — focus is on capture quality. Query CLI is a later phase.
- **Lattice skill update comes last**, after everything is tested.

## Acceptance criteria

1. The `entire-agent-etch` Go binary implements all six Entire plugin protocol hooks (`session_start`, `session_end`, `user_prompt_submit`, `stop`, `pre_tool_use`, `post_tool_use`) and the `info`, `parse-hook`, `extract-modified-files`, `calculate-tokens` capability subcommands.
2. Every agent session produces a valid `session.json` conforming to `etch.session.v1` schema (see OUTPUT_SPEC.md for full schema).
3. Each session ref is an orphan commit at `refs/etch/sessions/<ULID>` with a tree containing `session.json` and `agent-trace.json`.
4. 20 concurrent Claude Code sessions on Hyperion each produce a valid, distinct session ref with no collisions or dropped records.
5. Session refs push to Forgejo and GitHub via configured refspecs and fetch cleanly on a second machine (Atlas).
6. A simulated crash (process killed mid-session) produces a recoverable `.wip` file that the next Etch invocation commits as a partial record with `status: incomplete` and `exit_reason: crash`.
7. Machine identity is hashed with a per-repo salt (SHA-256 of salt + hostname; salt auto-generated into committed `.etch/settings.json`) by default; raw hostname is opt-in via `.etch/settings.json`.
8. Prompt and tool-use fields are scanned for common secret patterns (API keys, credential strings) before commit; detected secrets are redacted.
9. `agent-trace.json` is emitted alongside every `session.json` in the Agent Trace RFC format.
10. Orchestration metadata is captured from `ETCH_*` environment variables; absent variables default to `orchestration.type = "manual"`.
11. c11 context (`workspace_id`, `surface_id`, `tab_title`, `pane_lineage`) is captured when `C11_WORKSPACE_ID` / `C11_SURFACE_ID` env vars are present.
12. Ref lifecycle: refs older than a configurable threshold (default 90 days) are compactable into `refs/etch/archive/<YYYY-Q>` archive refs, with individual session refs deleted after archival.

## Constraints & assumptions

- **Entire CLI is installed and configured.** Etch is an Entire external agent plugin — it requires `entire` on PATH and the plugin discovery mechanism (`entire-agent-<name>` binary on PATH).
- **Go toolchain available** for building the binary. Target: Go 1.22+.
- **Git 2.30+** for reliable `update-ref`, `hash-object`, `mktree`, `commit-tree` plumbing.
- **macOS primary target** (Darwin arm64). Linux compatibility is desirable but not blocking for v1.
- **Two machines in scope**: Hyperion (m4 Max MacBook) and Atlas (m2 Ultra Mac Studio), connected via Tailscale.
- **ULID for session IDs** — lexicographically sortable by creation time, generated at session start.
- **No Python runtime dependency** for the production binary. The Phase 0 PoC is Python; the production build is Go.

## Open questions

1. **Exit reason granularity.** Entire's protocol distinguishes `session_end` vs `stop` but doesn't carry `normal` / `token_limit` / `error` / `user_kill` / `timeout`. Etch must infer from agent-native data or degrade to `unknown`. Exact inference heuristics TBD during implementation.
2. **Observation records.** The `refs/etch/observations/<uuid>` namespace for late-arriving data (PR merge, CI resolution) is designed but implementation is deferred — build only if time permits in Phase 1.
