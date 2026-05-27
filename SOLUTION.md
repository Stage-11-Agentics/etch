# The Solution: Cairn

Cairn captures flat metadata about every agent session in a repository and stores it in git so it travels with the code.

## Why not build from scratch

Entire CLI already solves the hardest part of the problem: hooking into 8+ agent runtimes (Claude Code, Codex, Gemini CLI, OpenCode, Cursor, Copilot CLI, Factory, Pi), capturing transcripts, and writing them to a git branch that travels with push/pull. That's 4,400+ commits of integration work across every major coding agent. It's MIT-licensed with no cloud dependencies on the capture path.

Rebuilding that hook infrastructure would be months of work for a problem already solved. The unsolved problems — capture at density, flat metadata beyond transcripts, machine-agnostic session isolation — are above the hook layer.

## What Cairn adds

Cairn installs alongside Entire CLI and extends it via Entire's external agent plugin protocol (`entire-agent-cairn` binary on $PATH). No fork required, but the plugin protocol's coverage of Cairn's metadata requirements must be validated before committing to this integration path (see Phase 0).

### 1. Flat metadata record per session

Every agent session produces a single metadata record containing:

| Field | Source |
|-------|--------|
| Schema version | Hardcoded: `cairn.session.v1` |
| Prompt | Agent hook (SessionStart / UserPromptSubmit) |
| Transcript ref | Entire's captured `full.jsonl` (cross-ref by checkpoint ID; degrades gracefully when absent) |
| Orchestration pattern | Environment variables: `LATTICE_TICKET_ID`, `LATTICE_ORCHESTRATOR_TYPE`, `LATTICE_DISPATCH_METHOD`; defaults to `"manual"` when absent. No inference. |
| Agent runtime + model | Agent hook (e.g., `claude-code` / `claude-opus-4-7`) |
| Tool use events | Agent hooks (PreToolUse / PostToolUse) |
| Files touched | Git diff at session boundaries |
| Token usage + cost | Agent hook metadata (input/output/cache tokens, API call count) |
| Session timing | Start/end timestamps from hooks |
| Exit reason | Agent hook (Stop event): `normal`, `token_limit`, `error`, `user_kill`, `timeout`, `unknown` |
| Machine identity | Hostname, machine fingerprint (hashed by default; raw opt-in) |
| Operator | Git user, OS user |
| Git state (start) | Branch, HEAD SHA, worktree path |
| Git state (end) | Branch, HEAD SHA, commits produced |
| Outcome (observed) | Commit SHAs, PR number/state, CI status — recorded as-is at session end |

Records are JSONL. All records include a `schema_version` field (`cairn.session.v1`) from the first capture so consumers can distinguish record generations as the schema evolves.

Records are self-contained for metadata: each carries enough context to be useful on its own. The transcript field is a cross-reference to Entire's `full.jsonl` by checkpoint ID — if the Entire checkpoint is missing or not fetched, the metadata record remains valid but the full transcript is unavailable. Shared identifiers (repo, timestamp, ticket ID, operator) let a future query engine reconstruct relationships without baking them into the schema.

Session refs are **immutable after creation.** Outcome fields (PR state, CI status) reflect the state observed at session end. Late-arriving outcome data (merge status, CI resolution, review outcomes that arrive after the session closes) is stored as separate observation records in `refs/cairn/observations/<uuid>`, cross-referenced by session UUID. This preserves the append-only, race-free property of the session ref namespace.

### 2. Per-session refs (the concurrency fix)

The core architectural choice. Instead of writing to a single shared branch that N agents race on, Cairn writes each session's metadata to its own git ref:

```
refs/cairn/sessions/<session-uuid>
```

Each ref points to a **commit** whose tree contains `session.json` (the metadata record) and optionally `agent-trace.json`. Refs are create-once, immutable after creation.

One ref per session. No contention. No CAS needed. No flock needed. 60 concurrent sessions produce 60 independent refs that never touch each other.

A periodic indexer (running on the always-on machine, or as a post-push hook, or on-demand) reads all `refs/cairn/sessions/*` and materializes a queryable index. The indexer's failure doesn't lose data — it just delays the read view.

This is the same insight Entire partially applied to shadow branches (per-worktree namespacing) but failed to apply to their committed branch (`entire/checkpoints/v1` uses bare `SetReference` with no CAS — a race at 20 concurrent agents). Cairn sidesteps the problem by design.

**Ref lifecycle.** At 60 sessions/day, the repo accumulates ~22,000 refs/year. Git can handle many refs, but wildcard refspec fetch performance degrades beyond ~10,000 refs and `git push` advertises the full ref list. Mitigation: after indexing, session refs older than a configurable threshold (default 90 days) are compacted into archival commits on a `refs/cairn/archive/<YYYY-Q>` ref and the individual session refs are deleted. New clones fetch only recent session refs by default; archival refs are available on demand.

### 3. Git-native transport

`refs/cairn/sessions/*` push and fetch like any other ref. Configure once:

```
[remote "origin"]
    push = refs/cairn/sessions/*:refs/cairn/sessions/*
    fetch = refs/cairn/sessions/*:refs/cairn/sessions/*
```

Session refs are immutable, so force-push (`+` prefix) is not the default. A `cairn repair` command can force-push individual refs when needed, but the default refspec does not enable overwrites.

A session captured on Hyperion is visible on Atlas after the next fetch. No external sync mechanism, no database, no cloud service.

**Remote compatibility caveat.** Not all git hosting platforms accept pushes to arbitrary ref namespaces. GitHub, GitLab, Forgejo, and Gitea may behave differently. Phase 0 validates this against all target remotes before implementation begins.

### 4. Agent Trace emission

Every session Cairn captures also produces an `agent-trace.json` blob — the Cursor RFC standard signed by Anthropic, OpenAI, Google, and ~14 others. This is a free interop win: any tool that reads Agent Trace can read Cairn's output. Entire has not adopted Agent Trace; Cairn should be on the other side of that line.

## Security and redaction

Cairn captures prompts, tool events, machine identity, operator identity, file paths, and potentially secrets — all pushed to git refs on shared remotes. Entire's redaction pipeline covers Entire's own checkpoint data but not Cairn's metadata records.

- **Machine identity and operator fields** are hashed by default (hostname fingerprint, not raw hostname). Raw values are opt-in via configuration.
- **Prompt and tool-use fields** may contain secrets (API keys, passwords). Cairn runs its own lightweight redaction pass over prompt/tool-use content before committing, scanning for common secret patterns (API key formats, credential strings). This is best-effort — it complements, not replaces, upstream secret management.
- **Sensitive fields can be marked local-only.** A `.cairn/settings.json` can designate specific fields (e.g., `machine_identity`, `operator`) as local-only, excluded from push.
- **Accidental secret remediation.** If a secret is captured and pushed, `cairn redact --session <uuid>` rewrites the session ref's commit tree with the offending content replaced, then force-pushes the single affected ref. This is damage control, not prevention.

## What Cairn does NOT build

- **No analysis engine.** Cairn captures and stores. "Which prompts correlate with clean CI?" is a question for a future consumer of the data.
- **No hierarchical workflow model.** Records are flat. Structure emerges from queries, not schema.
- **No outcome binding.** Outcome metadata (PR state, CI status) is recorded as observed fields on the session record, not as a computed correlation. Late-arriving outcomes are stored as separate observation records.
- **No dashboard.** The data is in git. Query it with git commands, jq, or a future tool.
- **No hook infrastructure.** Entire handles the hooks. Cairn consumes via the plugin protocol.

## Entire CLI's role

Entire is the capture substrate. Cairn is the metadata layer.

| Concern | Owner |
|---------|-------|
| Hook into agent runtimes | Entire |
| Capture transcripts | Entire |
| Write to `entire/checkpoints/v1` | Entire |
| Redact secrets (transcript layer) | Entire |
| Redact secrets (metadata layer) | Cairn |
| Flat metadata record | Cairn |
| Per-session git refs | Cairn |
| Concurrency-safe writes at density | Cairn |
| Machine-agnostic sync via git | Cairn |
| Agent Trace emission | Cairn |

Cairn depends on Entire for hook infrastructure and transcript capture. Cairn does not depend on Entire for storage, concurrency, or transport — those are Cairn's own refs.

If Entire disappears tomorrow, Cairn loses the hook layer and has to rebuild or replace it. The metadata records and refs are fully independent. MIT license means Cairn can fork the hook layer if necessary.

## Implementation shape

### Phase 0: Validation gates

Before implementation begins, validate two binary assumptions the architecture depends on:

1. **Remote ref compatibility.** Push `refs/cairn/sessions/<uuid>` to every target remote (GitHub, Forgejo, or whatever Cairn targets). Document results. If any target rejects custom ref namespaces, design a fallback (e.g., per-session files on a single branch, or a different ref namespace like `refs/notes/cairn/*`).

2. **Entire plugin protocol coverage.** Build a minimal `entire-agent-cairn` binary that logs every event it receives from Entire's external agent protocol. Map each metadata field (from the table above) to the specific protocol subcommand that provides it. Confirm data availability. If the protocol doesn't surface token usage, tool-use events, or prompt text, document what's missing and determine whether direct agent hooks (bypassing Entire) are needed for those fields.

Both gates are pass/fail. If either fails, the plan adjusts before implementation starts — not after.

### Phase 1: Plugin + capture

1. `entire-agent-cairn` binary implementing Entire's external agent protocol (informed by Phase 0 findings)
2. On each session lifecycle event, append to a session-local JSONL buffer persisted to a temp file (survives process death)
3. On session end, commit the metadata record to `refs/cairn/sessions/<uuid>`. On abnormal termination (crash, kill, timeout), commit a partial record with `exit_reason` set accordingly and `status: incomplete`
4. Push refs on the next `git push` (via refspec config or a Cairn post-push hook)

**Crash recovery.** A session-local temp file (`.cairn/sessions/<uuid>.wip.jsonl`) persists in-progress capture. If the process dies, the next Cairn invocation in the same repo detects orphaned `.wip` files, commits them as partial records with `status: incomplete` and `exit_reason: crash`, and cleans up. A configurable timeout (default 4 hours) marks sessions whose last event is stale.

**Phase 1 completion criteria.** Phase 1 is complete when: (a) 20 concurrent Claude Code sessions on Hyperion each produce a valid session ref, (b) the refs fetch cleanly on Atlas, (c) no records are dropped or overwritten, (d) a simulated crash produces a recoverable partial record, and (e) all metadata fields are populated (or explicitly marked unavailable with a reason).

### Phase 2: Query

5. `cairn query` CLI — read all `refs/cairn/sessions/*`, filter/aggregate, output JSON or table
6. `cairn index` — materialize a queryable index from all session refs (for large repos where scanning refs is slow)

### Phase 3: Emit

7. Agent Trace emission per session
8. Contextual Commits action lines (opt-in, not default) — when the agent explicitly tags decisions/constraints/learnings in a structured format, Cairn copies those tags to the commit message. This is a pass-through of agent-declared metadata, not content analysis of the transcript.

## Why this wins

- **Zero-contention writes.** Per-session refs can't race. The architecture that Entire got wrong on their committed branch, Cairn gets right by not sharing a ref.
- **Git-native transport.** No cloud, no database, no sync daemon. Push/fetch works.
- **Flat metadata.** The schema doesn't prescribe how you organize your agents. It records what happened. Structure is a query-time concern.
- **Density-tested from day one.** The smoke test is 20 concurrent agents in one repo across two machines. That's the operating reality, not a stretch goal.
- **Interoperable.** Agent Trace emission means every tool in the Cursor/Cognition/Anthropic ecosystem can read Cairn's output. Entire can't say that.
- **No lock-in.** Cairn's data is plain JSONL in git refs. No proprietary format, no required service, no account. Walk away and the data stays in the repo.
