# Cairn Output Spec

What Cairn's data looks like when it's working. Reference for building capture code and generating synthetic test data.

## 1. Schema: `session.json`

Every session produces one `session.json` stored inside a git commit at `refs/cairn/sessions/<session_uuid>`. Fields are nullable unless marked **required**.

```jsonc
{
  // ── Identity ──────────────────────────────────────────────
  "schema_version": "cairn.session.v1",          // required
  "session_id": "01JWB8K3XQPNR7TV0ZYM4GD2AH",   // required, ULID
  "parent_session_id": null,                      // ULID of spawning orchestrator session, or null
  "status": "complete",                           // required: "complete" | "incomplete"
  "exit_reason": "normal",                        // "normal" | "token_limit" | "error" | "user_kill" | "timeout" | "crash" | "unknown"

  // ── Agent ─────────────────────────────────────────────────
  "agent": {
    "runtime": "claude-code",                     // required: "claude-code" | "codex" | "gemini-cli" | "opencode" | "cursor" | "kimi" | ...
    "model": "claude-opus-4-7",                   // nullable
    "version": "1.0.33"                           // agent CLI version, nullable
  },

  // ── Prompt ────────────────────────────────────────────────
  "prompt": {
    "text": "Implement the login button component per the design spec in SPEC.md. Branch off origin/main.",
    "source": "c11_send",                         // "interactive" | "c11_send" | "pipe" | "prompt_file" | "unknown"
    "truncated": false                            // true if prompt exceeded 32 KiB capture limit
  },

  // ── Orchestration ─────────────────────────────────────────
  "orchestration": {
    "type": "lattice-orchestrator",               // from CAIRN_ORCHESTRATOR_TYPE; "manual" when absent
    "dispatch_method": "c11_delegator",           // from CAIRN_DISPATCH_METHOD; null when absent
    "ticket_id": "FT-481",                        // from CAIRN_TICKET_ID; null when absent
    "run_id": "01JWB8FGXQPNR7TV0ZYM4GD1AA",      // from CAIRN_RUN_ID; groups sessions into one orchestration run; null when absent
    "role": "implementer",                        // from CAIRN_AGENT_ROLE; null when absent
    "workflow_version": "a3f8c2e",                // from CAIRN_WORKFLOW_VERSION; content-hash or git SHA of the workflow definition; null when absent
    "extra": {}                                   // from CAIRN_ORCHESTRATION_EXTRA (JSON string); open property bag for workflow-specific metadata
  },

  // ── Timing ────────────────────────────────────────────────
  "timing": {
    "started_at": "2026-05-26T14:32:08.441Z",    // required, ISO 8601 UTC
    "ended_at": "2026-05-26T14:47:22.109Z",      // null if crash before session-end hook
    "duration_ms": 913668                         // derived; null if ended_at is null
  },

  // ── Machine ───────────────────────────────────────────────
  "machine": {
    "hostname_hash": "sha256:a1b2c3d4e5f6...",   // SHA-256 of hostname (default)
    "hostname_raw": null,                         // populated only when raw_machine_identity = true in .cairn/settings.json
    "os": "darwin",                               // "darwin" | "linux" | "windows"
    "os_version": "Darwin 25.5.0",
    "arch": "arm64"
  },

  // ── Operator ──────────────────────────────────────────────
  "operator": {
    "git_user": "Atin Woodard <atin@authentic.tech>",
    "os_user": "atin"
  },

  // ── Git state ─────────────────────────────────────────────
  "git_start": {
    "branch": "feat/login-button",
    "head_sha": "e4a9c1f2b3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8",
    "worktree_path": "/Users/atin/Projects/Stage11/code/Lattice-worktrees/feat-login-button",
    "is_worktree": true,
    "repo_root": "/Users/atin/Projects/Stage11/code/Lattice"
  },
  "git_end": {
    "branch": "feat/login-button",
    "head_sha": "b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6",
    "commits_produced": [
      "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0",
      "b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6"
    ]
  },

  // ── Outcome (observed at session end) ─────────────────────
  "outcome": {
    "commits": [
      "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0",
      "b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6"
    ],
    "pr_number": 42,
    "pr_state": "open",                           // "open" | "merged" | "closed" | null
    "ci_status": null                             // "pass" | "fail" | "pending" | null
  },

  // ── Files touched ─────────────────────────────────────────
  "files_touched": [
    { "path": "src/components/LoginButton.tsx", "action": "added" },
    { "path": "src/components/LoginButton.test.tsx", "action": "added" },
    { "path": "src/pages/auth/login.tsx", "action": "modified" }
  ],

  // ── Token usage ───────────────────────────────────────────
  "tokens": {
    "input": 284100,
    "output": 18420,
    "cache_read": 196000,
    "cache_write": 88100,
    "api_calls": 47,
    "estimated_cost_usd": 3.82
  },

  // ── Tool use summary ──────────────────────────────────────
  "tool_use": {
    "total_calls": 132,
    "by_tool": {
      "Read": 41,
      "Edit": 28,
      "Write": 6,
      "Bash": 33,
      "Agent": 3,
      "AskUserQuestion": 2,
      "Search": 19
    }
  },

  // ── Transcript cross-reference ────────────────────────────
  "transcript_ref": {
    "entire_checkpoint_id": "chk_01JWB8KXYZ...",  // Entire CLI checkpoint ID; null if Entire not installed
    "local_path": "~/.claude/projects/-Users-atin-Projects-Stage11-code-Lattice/01JWB8K3XQ.jsonl",
    "available": true                              // false if the checkpoint is missing or unfetched
  },

  // ── c11 context (populated when session ran inside c11) ───
  "c11": {
    "workspace_id": "01JWB7AABC...",
    "surface_id": "01JWB7BBCD...",
    "tab_title": "FT-481 :: Impl :: Claude",
    "pane_lineage": ["FT-481 Orchestrator", "FT-481 :: Impl :: Claude"]
  }
}
```

### Field notes

- **`session_id`**: ULID, not UUID. Lexicographically sortable by creation time. Generated at session start.
- **`parent_session_id`**: Set by `CAIRN_PARENT_SESSION_ID` env var. The orchestrator exports its own session ID so spawned agents inherit it.
- **`prompt.text`**: Captured from `SessionStart` or `UserPromptSubmit` hooks. Capped at 32 KiB; `truncated: true` if exceeded.
- **`orchestration.extra`**: Arbitrary JSON. The workflow author puts whatever is meaningful here (retry count, eval gate results, reviewer model, custom routing logic). Cairn stores it; queries index across it.
- **`transcript_ref`**: Cross-reference only. The session record is valid without the transcript. Graceful degradation.
- **`c11`**: Populated from `C11_WORKSPACE_ID`, `C11_SURFACE_ID` env vars and `c11 get-titlebar-state`. Null when not in c11.
- **`machine.hostname_hash`**: Default. Raw hostname exposed only with explicit opt-in in `.cairn/settings.json`.
- **Immutability**: Once committed to `refs/cairn/sessions/<id>`, the record is never updated. Late-arriving data (PR merge, CI resolution) goes to `refs/cairn/observations/<uuid>`.


## 2. Scenario variants

### 2a. Solo manual session — no orchestration, no parent

An operator typing directly into Claude Code on their laptop. No Lattice, no c11, no parent.

```json
{
  "schema_version": "cairn.session.v1",
  "session_id": "01JWC4R1XQPNR7TV0ZYM4GD5BB",
  "parent_session_id": null,
  "status": "complete",
  "exit_reason": "normal",
  "agent": {
    "runtime": "claude-code",
    "model": "claude-sonnet-4-6",
    "version": "1.0.33"
  },
  "prompt": {
    "text": "Fix the off-by-one error in the pagination logic. The last page shows one duplicate item.",
    "source": "interactive",
    "truncated": false
  },
  "orchestration": {
    "type": "manual",
    "dispatch_method": null,
    "ticket_id": null,
    "run_id": null,
    "role": null,
    "workflow_version": null,
    "extra": {}
  },
  "timing": {
    "started_at": "2026-05-26T09:12:44.330Z",
    "ended_at": "2026-05-26T09:18:02.771Z",
    "duration_ms": 318441
  },
  "machine": {
    "hostname_hash": "sha256:7f2e3d4c5b6a...",
    "hostname_raw": null,
    "os": "darwin",
    "os_version": "Darwin 25.5.0",
    "arch": "arm64"
  },
  "operator": {
    "git_user": "Atin Woodard <atin@authentic.tech>",
    "os_user": "atin"
  },
  "git_start": {
    "branch": "main",
    "head_sha": "d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3",
    "worktree_path": "/Users/atin/Projects/Stage11/code/Lattice",
    "is_worktree": false,
    "repo_root": "/Users/atin/Projects/Stage11/code/Lattice"
  },
  "git_end": {
    "branch": "main",
    "head_sha": "f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7",
    "commits_produced": [
      "f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7"
    ]
  },
  "outcome": {
    "commits": ["f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7"],
    "pr_number": null,
    "pr_state": null,
    "ci_status": null
  },
  "files_touched": [
    { "path": "src/utils/pagination.ts", "action": "modified" },
    { "path": "tests/pagination.test.ts", "action": "modified" }
  ],
  "tokens": {
    "input": 52400,
    "output": 4310,
    "cache_read": 38000,
    "cache_write": 14400,
    "api_calls": 12,
    "estimated_cost_usd": 0.61
  },
  "tool_use": {
    "total_calls": 18,
    "by_tool": {
      "Read": 6,
      "Edit": 4,
      "Bash": 5,
      "Search": 3
    }
  },
  "transcript_ref": {
    "entire_checkpoint_id": null,
    "local_path": "~/.claude/projects/-Users-atin-Projects-Stage11-code-Lattice/01JWC4R1XQ.jsonl",
    "available": true
  },
  "c11": null
}
```

### 2b. Lattice delegator session — has parent, has ticket, inside c11

A delegator agent spawned by a Lattice orchestrator into a c11 pane. Working on a ticket in a git worktree. This is the high-density scenario — 19 siblings might exist concurrently.

```json
{
  "schema_version": "cairn.session.v1",
  "session_id": "01JWB8K3XQPNR7TV0ZYM4GD2AH",
  "parent_session_id": "01JWB7MMXQPNR7TV0ZYM4GD0ZZ",
  "status": "complete",
  "exit_reason": "normal",
  "agent": {
    "runtime": "claude-code",
    "model": "claude-opus-4-7",
    "version": "1.0.33"
  },
  "prompt": {
    "text": "You are a Lattice delegator. Ticket FT-481: Implement login button component...",
    "source": "c11_send",
    "truncated": true
  },
  "orchestration": {
    "type": "lattice-orchestrator",
    "dispatch_method": "c11_delegator",
    "ticket_id": "FT-481",
    "run_id": "01JWB7LLXQPNR7TV0ZYM4GD0YY",
    "role": "implementer",
    "workflow_version": "a3f8c2e",
    "extra": {
      "lattice_board": "/Users/atin/Projects/Stage11/code/Lattice",
      "phase": "impl",
      "review_model": "claude-opus-4-7",
      "worktree_branch": "feat/login-button"
    }
  },
  "timing": {
    "started_at": "2026-05-26T14:32:08.441Z",
    "ended_at": "2026-05-26T14:47:22.109Z",
    "duration_ms": 913668
  },
  "machine": {
    "hostname_hash": "sha256:a1b2c3d4e5f6...",
    "hostname_raw": null,
    "os": "darwin",
    "os_version": "Darwin 25.5.0",
    "arch": "arm64"
  },
  "operator": {
    "git_user": "Atin Woodard <atin@authentic.tech>",
    "os_user": "atin"
  },
  "git_start": {
    "branch": "feat/login-button",
    "head_sha": "e4a9c1f2b3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8",
    "worktree_path": "/Users/atin/Projects/Stage11/code/Lattice-worktrees/feat-login-button",
    "is_worktree": true,
    "repo_root": "/Users/atin/Projects/Stage11/code/Lattice"
  },
  "git_end": {
    "branch": "feat/login-button",
    "head_sha": "b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6",
    "commits_produced": [
      "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0",
      "b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6"
    ]
  },
  "outcome": {
    "commits": [
      "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0",
      "b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6"
    ],
    "pr_number": 42,
    "pr_state": "open",
    "ci_status": null
  },
  "files_touched": [
    { "path": "src/components/LoginButton.tsx", "action": "added" },
    { "path": "src/components/LoginButton.test.tsx", "action": "added" },
    { "path": "src/pages/auth/login.tsx", "action": "modified" }
  ],
  "tokens": {
    "input": 284100,
    "output": 18420,
    "cache_read": 196000,
    "cache_write": 88100,
    "api_calls": 47,
    "estimated_cost_usd": 3.82
  },
  "tool_use": {
    "total_calls": 132,
    "by_tool": {
      "Read": 41,
      "Edit": 28,
      "Write": 6,
      "Bash": 33,
      "Agent": 3,
      "AskUserQuestion": 2,
      "Search": 19
    }
  },
  "transcript_ref": {
    "entire_checkpoint_id": "chk_01JWB8KXYZ...",
    "local_path": "~/.claude/projects/-Users-atin-Projects-Stage11-code-Lattice/01JWB8K3XQ.jsonl",
    "available": true
  },
  "c11": {
    "workspace_id": "01JWB7AABC...",
    "surface_id": "01JWB7BBCD...",
    "tab_title": "FT-481 :: Impl :: Claude",
    "pane_lineage": ["FT-481 Orchestrator", "FT-481 :: Impl :: Claude"]
  }
}
```

### 2c. Crashed/incomplete session — process died mid-work

The agent was killed (OOM, operator Ctrl-C'd c11, machine sleep during long session). The crash recovery sweep found the orphaned `.wip` file and committed it as a partial record.

```json
{
  "schema_version": "cairn.session.v1",
  "session_id": "01JWD2P5XQPNR7TV0ZYM4GD8CC",
  "parent_session_id": "01JWD2N1XQPNR7TV0ZYM4GD8AA",
  "status": "incomplete",
  "exit_reason": "crash",
  "agent": {
    "runtime": "codex",
    "model": "o3",
    "version": "0.2.1"
  },
  "prompt": {
    "text": "Refactor the entire caching layer to use Redis instead of in-memory LRU. See SPEC.md for the new interface.",
    "source": "prompt_file",
    "truncated": false
  },
  "orchestration": {
    "type": "lattice-orchestrator",
    "dispatch_method": "c11_delegator",
    "ticket_id": "FT-503",
    "run_id": "01JWD2M0XQPNR7TV0ZYM4GD8ZZ",
    "role": "implementer",
    "workflow_version": "b4c9d3f",
    "extra": {
      "phase": "impl"
    }
  },
  "timing": {
    "started_at": "2026-05-26T22:10:33.008Z",
    "ended_at": null,
    "duration_ms": null
  },
  "machine": {
    "hostname_hash": "sha256:a1b2c3d4e5f6...",
    "hostname_raw": null,
    "os": "darwin",
    "os_version": "Darwin 25.5.0",
    "arch": "arm64"
  },
  "operator": {
    "git_user": "Atin Woodard <atin@authentic.tech>",
    "os_user": "atin"
  },
  "git_start": {
    "branch": "feat/redis-cache",
    "head_sha": "1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b",
    "worktree_path": "/Users/atin/Projects/Stage11/code/Lattice-worktrees/feat-redis-cache",
    "is_worktree": true,
    "repo_root": "/Users/atin/Projects/Stage11/code/Lattice"
  },
  "git_end": {
    "branch": "feat/redis-cache",
    "head_sha": "3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d",
    "commits_produced": [
      "3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d"
    ]
  },
  "outcome": {
    "commits": ["3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d"],
    "pr_number": null,
    "pr_state": null,
    "ci_status": null
  },
  "files_touched": [
    { "path": "src/cache/redis_client.ts", "action": "added" },
    { "path": "src/cache/lru.ts", "action": "modified" }
  ],
  "tokens": {
    "input": 141200,
    "output": 9800,
    "cache_read": 102000,
    "cache_write": 39200,
    "api_calls": 23,
    "estimated_cost_usd": 1.94
  },
  "tool_use": {
    "total_calls": 56,
    "by_tool": {
      "Read": 18,
      "Edit": 12,
      "Bash": 14,
      "Write": 3,
      "Search": 9
    }
  },
  "transcript_ref": {
    "entire_checkpoint_id": null,
    "local_path": null,
    "available": false
  },
  "c11": {
    "workspace_id": "01JWD2AABC...",
    "surface_id": "01JWD2BBCD...",
    "tab_title": "FT-503 :: Impl :: Codex",
    "pane_lineage": ["FT-503 Orchestrator", "FT-503 :: Impl :: Codex"]
  }
}
```

**What's different about incomplete records:**
- `status: "incomplete"`, `exit_reason: "crash"`
- `timing.ended_at` and `timing.duration_ms` are null — the session-end hook never fired
- `transcript_ref.available: false` — the transcript may be partial or missing
- `tokens` and `tool_use` reflect the last event captured before death (from the `.wip` file)
- `git_end` reflects the last known git state from the `.wip` file, not a clean session boundary
- `files_touched` may be incomplete — only files committed before the crash appear

### 2d. Cross-machine session — captured on Atlas, viewed on Hyperion

A session that ran on Atlas (the always-on Mac Studio) during an overnight orchestration run. Fetched on Hyperion the next morning. The record itself is identical in structure — cross-machine portability is a transport property of the git ref, not a schema property. The distinguishing features are the machine identity and worktree paths.

```json
{
  "schema_version": "cairn.session.v1",
  "session_id": "01JWCR88XQPNR7TV0ZYM4GD7DD",
  "parent_session_id": "01JWCR77XQPNR7TV0ZYM4GD7CC",
  "status": "complete",
  "exit_reason": "normal",
  "agent": {
    "runtime": "claude-code",
    "model": "claude-opus-4-7",
    "version": "1.0.33"
  },
  "prompt": {
    "text": "You are a Lattice delegator. Ticket LAT-219: Add dashboard widget for session cost breakdown...",
    "source": "pipe",
    "truncated": true
  },
  "orchestration": {
    "type": "lattice-orchestrator",
    "dispatch_method": "headless_clear",
    "ticket_id": "LAT-219",
    "run_id": "01JWCR66XQPNR7TV0ZYM4GD7BB",
    "role": "implementer",
    "workflow_version": "c5d0e4a",
    "extra": {
      "phase": "impl",
      "overnight": true,
      "batch_index": 7,
      "batch_total": 12
    }
  },
  "timing": {
    "started_at": "2026-05-26T03:14:22.881Z",
    "ended_at": "2026-05-26T03:38:09.442Z",
    "duration_ms": 1426561
  },
  "machine": {
    "hostname_hash": "sha256:e8f9a0b1c2d3...",
    "hostname_raw": null,
    "os": "darwin",
    "os_version": "Darwin 25.5.0",
    "arch": "arm64"
  },
  "operator": {
    "git_user": "Atin Woodard <atin@authentic.tech>",
    "os_user": "atin"
  },
  "git_start": {
    "branch": "feat/cost-widget",
    "head_sha": "5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f",
    "worktree_path": "/Users/atin/Projects/Stage11/code/Lattice-worktrees/feat-cost-widget",
    "is_worktree": true,
    "repo_root": "/Users/atin/Projects/Stage11/code/Lattice"
  },
  "git_end": {
    "branch": "feat/cost-widget",
    "head_sha": "9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b",
    "commits_produced": [
      "7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b",
      "8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c",
      "9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b"
    ]
  },
  "outcome": {
    "commits": [
      "7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b",
      "8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c",
      "9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b"
    ],
    "pr_number": 87,
    "pr_state": "open",
    "ci_status": "pass"
  },
  "files_touched": [
    { "path": "src/dashboard/widgets/CostBreakdown.tsx", "action": "added" },
    { "path": "src/dashboard/widgets/CostBreakdown.test.tsx", "action": "added" },
    { "path": "src/dashboard/widgets/index.ts", "action": "modified" },
    { "path": "src/dashboard/Dashboard.tsx", "action": "modified" }
  ],
  "tokens": {
    "input": 312000,
    "output": 22100,
    "cache_read": 210000,
    "cache_write": 102000,
    "api_calls": 54,
    "estimated_cost_usd": 4.18
  },
  "tool_use": {
    "total_calls": 148,
    "by_tool": {
      "Read": 44,
      "Edit": 31,
      "Write": 8,
      "Bash": 37,
      "Agent": 5,
      "Search": 23
    }
  },
  "transcript_ref": {
    "entire_checkpoint_id": "chk_01JWCR88XYZ...",
    "local_path": "~/.claude/projects/-Users-atin-Projects-Stage11-code-Lattice/01JWCR88XQ.jsonl",
    "available": false
  },
  "c11": null
}
```

**What's different about cross-machine records:**
- `machine.hostname_hash` differs from the querying machine — this is how you know it came from elsewhere
- `transcript_ref.available: false` — the local transcript path doesn't exist on the fetching machine; the path is still recorded for provenance
- `transcript_ref.local_path` uses the originating machine's filesystem path (useful if you SSH back to that machine)
- `c11: null` — headless sessions on Atlas run without c11 (dispatched via `claude -p`)
- `orchestration.dispatch_method: "headless_clear"` — not `c11_delegator`


## 3. Orchestration method declaration

### Environment variables

The orchestrating layer declares itself to Cairn via environment variables. Cairn reads these at session start and writes them into the `orchestration` block. When all are absent, `orchestration.type` defaults to `"manual"`.

| Variable | Required | Description | Example |
|----------|----------|-------------|---------|
| `CAIRN_ORCHESTRATOR_TYPE` | No | Identifies the orchestration system | `lattice-orchestrator`, `manual`, `custom-bash`, `devin-style` |
| `CAIRN_DISPATCH_METHOD` | No | How this session was spawned | `c11_delegator`, `headless_clear`, `manual_fan_out`, `github_actions` |
| `CAIRN_TICKET_ID` | No | Ticket/task identifier in the orchestration system | `FT-481`, `LAT-219`, `PROJ-42` |
| `CAIRN_RUN_ID` | No | Groups sessions into one orchestration run | ULID: `01JWB7LLXQPNR7TV0ZYM4GD0YY` |
| `CAIRN_AGENT_ROLE` | No | Role of this agent within the orchestration | `implementer`, `reviewer`, `planner`, `validator` |
| `CAIRN_PARENT_SESSION_ID` | No | Cairn session ID of the spawning orchestrator | ULID: `01JWB7MMXQPNR7TV0ZYM4GD0ZZ` |
| `CAIRN_WORKFLOW_VERSION` | No | Version identifier for the workflow definition | Git SHA, content hash, semver |
| `CAIRN_ORCHESTRATION_EXTRA` | No | JSON string — open property bag for workflow-specific metadata | `{"phase":"impl","retry_count":2}` |
| `CAIRN_PANE_LINEAGE` | No | JSON array of ancestor tab titles. The spawning orchestrator exports its own pane_lineage; Cairn appends the current pane's tab title. | `["Orchestrator","FT-481 :: Impl"]` |

### Who sets them

**Lattice orchestrator skill** (`lattice-orchestrator/SKILL.md`) needs to be updated to export these when dispatching delegators. The orchestrator knows all values at dispatch time:

```bash
# In the orchestrator, before launching a delegator via c11:
export CAIRN_ORCHESTRATOR_TYPE="lattice-orchestrator"
export CAIRN_DISPATCH_METHOD="c11_delegator"
export CAIRN_TICKET_ID="FT-481"
export CAIRN_RUN_ID="$RUN_ULID"
export CAIRN_AGENT_ROLE="implementer"
export CAIRN_PARENT_SESSION_ID="$MY_CAIRN_SESSION_ID"
export CAIRN_WORKFLOW_VERSION="$(git -C /path/to/skill rev-parse --short HEAD)"
export CAIRN_ORCHESTRATION_EXTRA='{"phase":"impl","review_model":"claude-opus-4-7"}'

# Then launch via c11 — env vars propagate to the child shell
c11 default-agent launch \
    --in-surface surface:5 \
    --cwd /path/to/worktree \
    --prompt-file /tmp/delegator-prompt.md
```

**c11** does not need to set Cairn variables. c11's own env vars (`C11_WORKSPACE_ID`, `C11_SURFACE_ID`) are read separately by Cairn to populate the `c11` block. c11 is a context provider, not an orchestration system.

**Headless `claude -p` dispatches** (Pattern 2: clear agents) set the env vars inline:

```bash
env -u CLAUDECODE \
    CAIRN_ORCHESTRATOR_TYPE=lattice-orchestrator \
    CAIRN_DISPATCH_METHOD=headless_clear \
    CAIRN_TICKET_ID=FT-503 \
    CAIRN_RUN_ID="$RUN_ULID" \
    CAIRN_PARENT_SESSION_ID="$MY_CAIRN_SESSION_ID" \
    CAIRN_AGENT_ROLE=implementer \
    claude -p "$(cat /tmp/prompt.md)" --dangerously-skip-permissions
```

**Manual sessions** set nothing. All variables absent → `orchestration.type = "manual"`, all other fields null.

**Custom orchestrators** set whatever subset is relevant. The only hard rule: if `CAIRN_ORCHESTRATOR_TYPE` is set, it must be a non-empty string. Everything else is optional.

### Namespace

All variables use the `CAIRN_` prefix. No collision with `LATTICE_*`, `C11_*`, or agent-specific vars. The SOLUTION.md mentioned `LATTICE_TICKET_ID` etc. — this spec narrows to `CAIRN_*` so any orchestrator (not just Lattice) can participate without namespace conflict. The Lattice skill sets `CAIRN_TICKET_ID`, not `LATTICE_TICKET_ID`.


## 4. Git object layout

Each session ref points to a commit whose tree contains the session data. Here's what `git cat-file` and `git show` produce.

### Ref structure

```
refs/cairn/sessions/01JWB8K3XQPNR7TV0ZYM4GD2AH  →  commit abc1234...
```

### The commit

```
$ git cat-file -p refs/cairn/sessions/01JWB8K3XQPNR7TV0ZYM4GD2AH

tree 8a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a9b
author cairn <cairn@localhost> 1748271442 +0000
committer cairn <cairn@localhost> 1748271442 +0000

cairn session 01JWB8K3XQPNR7TV0ZYM4GD2AH
agent: claude-code / claude-opus-4-7
status: complete
branch: feat/login-button
commits: 2
duration: 913s
```

**Commit details:**
- **No parent commit.** Each session ref is a root commit — no DAG entanglement, no merge conflicts, no contention.
- **Author/committer**: `cairn <cairn@localhost>` — fixed identity, not the operator. The operator is inside the record.
- **Timestamp**: session end time (or last-known event time for incomplete sessions).
- **Commit message**: Human-scannable summary. First line is always `cairn session <ULID>`. Remaining lines are key fields for `git log --oneline` readability. Not parsed programmatically — the tree contents are the source of truth.

### The tree

```
$ git cat-file -p 8a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a9b

100644 blob f1e2d3c4b5a6...    session.json
100644 blob a9b8c7d6e5f4...    agent-trace.json
```

Two blobs, always:
- **`session.json`** — the full metadata record (Section 1 of this spec)
- **`agent-trace.json`** — Agent Trace standard format, serialized from the same capture data

### The blobs

```
$ git show refs/cairn/sessions/01JWB8K3XQPNR7TV0ZYM4GD2AH:session.json

{
  "schema_version": "cairn.session.v1",
  "session_id": "01JWB8K3XQPNR7TV0ZYM4GD2AH",
  ...
}
```

```
$ git show refs/cairn/sessions/01JWB8K3XQPNR7TV0ZYM4GD2AH:agent-trace.json

{
  "version": "1.0",
  "traces": [
    {
      "agent_id": "claude-code",
      "model": "claude-opus-4-7",
      "session_id": "01JWB8K3XQPNR7TV0ZYM4GD2AH",
      "files": [
        "src/components/LoginButton.tsx",
        "src/components/LoginButton.test.tsx",
        "src/pages/auth/login.tsx"
      ],
      "timestamp": "2026-05-26T14:47:22.109Z"
    }
  ]
}
```

### Observation refs (late-arriving data)

```
$ git cat-file -p refs/cairn/observations/01JWB9NNXQPNR7TV0ZYM4GD3FF

tree c2d3e4f5a6b7...
author cairn <cairn@localhost> 1748275000 +0000
committer cairn <cairn@localhost> 1748275000 +0000

cairn observation for session 01JWB8K3XQPNR7TV0ZYM4GD2AH
type: ci_status
```

```
$ git show refs/cairn/observations/01JWB9NNXQPNR7TV0ZYM4GD3FF:observation.json

{
  "schema_version": "cairn.observation.v1",
  "observation_id": "01JWB9NNXQPNR7TV0ZYM4GD3FF",
  "session_id": "01JWB8K3XQPNR7TV0ZYM4GD2AH",
  "observed_at": "2026-05-26T15:16:40.000Z",
  "type": "ci_status",
  "data": {
    "ci_status": "pass",
    "pr_state": "merged",
    "merged_at": "2026-05-26T15:14:02.000Z",
    "merge_sha": "d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3"
  }
}
```

### Listing all sessions

```
$ git for-each-ref --sort=-creatordate --format='%(refname:short) %(subject)' refs/cairn/sessions/

01JWD2P5XQPNR7TV0ZYM4GD8CC  cairn session 01JWD2P5XQPNR7TV0ZYM4GD8CC
01JWCR88XQPNR7TV0ZYM4GD7DD  cairn session 01JWCR88XQPNR7TV0ZYM4GD7DD
01JWC4R1XQPNR7TV0ZYM4GD5BB  cairn session 01JWC4R1XQPNR7TV0ZYM4GD5BB
01JWB8K3XQPNR7TV0ZYM4GD2AH  cairn session 01JWB8K3XQPNR7TV0ZYM4GD2AH
```

### Creating a session ref (what `cairn` does internally)

```bash
# 1. Write session.json to a blob
SESSION_BLOB=$(echo '{"schema_version":"cairn.session.v1",...}' | git hash-object -w --stdin)

# 2. Write agent-trace.json to a blob
TRACE_BLOB=$(echo '{"version":"1.0",...}' | git hash-object -w --stdin)

# 3. Build the tree
TREE=$(printf "100644 blob %s\tsession.json\n100644 blob %s\tagent-trace.json\n" \
    "$SESSION_BLOB" "$TRACE_BLOB" | git mktree)

# 4. Create an orphan commit (no parent)
COMMIT=$(git commit-tree "$TREE" -m "cairn session $SESSION_ID
agent: $RUNTIME / $MODEL
status: $STATUS
branch: $BRANCH
commits: $N_COMMITS
duration: ${DURATION_S}s")

# 5. Point the ref at the commit
git update-ref "refs/cairn/sessions/$SESSION_ID" "$COMMIT"
```

No locks. No CAS. No contention. 60 concurrent agents each run this sequence against a different ref name — zero possibility of conflict.


## 5. Synthetic data generator spec

### Purpose

Generate N realistic Cairn session records as git refs in a test repository, matching this schema exactly. Used for testing `cairn query`, `cairn index`, the archival compactor, and any downstream consumers — before real capture data exists.

### CLI

```
cairn synth [OPTIONS]
```

### Parameters

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--sessions` | int | 100 | Number of session records to generate |
| `--time-window` | duration | `7d` | Sessions distributed across this window ending at now |
| `--density` | enum | `medium` | `low` (2-5 concurrent), `medium` (10-20), `high` (40-80) |
| `--orchestration-mix` | string | `auto` | Ratio of orchestrated vs manual: `auto`, `all-manual`, `all-orchestrated`, or `manual:30,lattice:60,custom:10` |
| `--machines` | int | 1 | Number of distinct machine identities (simulates cross-machine) |
| `--crash-rate` | float | 0.05 | Fraction of sessions with `status: incomplete` |
| `--repo` | path | `.` | Target git repository |
| `--seed` | int | random | RNG seed for reproducibility |
| `--with-observations` | bool | false | Also generate observation refs for a subset of sessions |
| `--dry-run` | bool | false | Print session JSON to stdout without creating refs |

### Distribution model

The generator doesn't produce uniformly random records. It simulates realistic patterns:

**Temporal clustering.** Sessions cluster around "work windows" — bursts of activity with quiet gaps. A `high` density run has 3-5 bursts per day, each spawning 10-40 sessions within 5-30 minutes (an orchestration run dispatch). `low` density spaces individual sessions across the day.

**Orchestration runs.** Orchestrated sessions share a `run_id` and a common `parent_session_id`. A run produces 3-15 sessions (tickets), all starting within a few minutes of each other. Each session within a run gets a distinct `ticket_id`, `role`, and branch. The parent session (the orchestrator itself) is also generated, with no parent of its own.

**Agent runtime distribution.** Realistic mix weighted toward the actual Stage 11 pattern:
- `claude-code`: 70%
- `codex`: 15%
- `gemini-cli`: 5%
- `opencode`: 5%
- `kimi`: 5%

**Model distribution.** Per-runtime realistic defaults:
- `claude-code` → `claude-opus-4-7` (60%), `claude-sonnet-4-6` (35%), `claude-haiku-4-5` (5%)
- `codex` → `o3` (70%), `o4-mini` (30%)
- `gemini-cli` → `gemini-2.5-pro` (80%), `gemini-2.5-flash` (20%)
- `opencode` → `claude-opus-4-7` (50%), `o3` (50%)

**Token/cost correlation.** Larger models, longer sessions, and more tool calls correlate with higher token counts. The generator samples from distributions fitted to realistic ranges:
- Short session (< 5 min): 20K-80K input tokens, 2K-8K output, 5-20 tool calls
- Medium session (5-20 min): 80K-300K input, 8K-25K output, 20-150 tool calls
- Long session (20-60 min): 300K-800K input, 25K-60K output, 100-400 tool calls

**Branching patterns.** Solo manual sessions: 60% work on `main`, 40% on feature branches. Orchestrated sessions: always on feature branches, named `feat/<ticket-slug>`. Worktree paths are generated for orchestrated sessions.

**Crash patterns.** Crashes are more likely in longer sessions and during high-density bursts (resource contention). Crash records have null `ended_at` and partial `tool_use`/`tokens` values.

### Output

Running `cairn synth --sessions 200 --density high --machines 2 --time-window 3d` produces:
- 200 session refs at `refs/cairn/sessions/<ulid>`
- Each ref is a proper git commit with `session.json` and `agent-trace.json`
- ~10 crashed/incomplete records (5% default crash rate)
- ~140 orchestrated sessions grouped into ~12 runs
- ~60 manual sessions scattered across the window
- 2 distinct `machine.hostname_hash` values
- Sessions temporally clustered into realistic work bursts
- Optionally, observation refs for ~30% of completed sessions (with `--with-observations`)

### Validation

The generator validates its own output:

```bash
# After generation, verify every ref is readable and schema-valid
cairn synth --sessions 50 && cairn validate --refs refs/cairn/sessions/*
```

`cairn validate` checks:
- Every session ref points to a valid commit with a well-formed tree
- Every `session.json` passes the JSON schema
- ULIDs are unique and temporally ordered
- `parent_session_id` references exist when the parent should have been generated
- `run_id` groups are internally consistent (same orchestrator type, overlapping time windows)
- No two sessions claim the same branch + worktree at overlapping times (the concurrency invariant)
