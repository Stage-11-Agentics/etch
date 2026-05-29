# Lattice orchestrator skill integration (ETCH-12)

Etch captures orchestration metadata from `ETCH_*` environment variables at
`session_start` (see `internal/capture/environ.go::CaptureOrchestration` and
`internal/hooks/session_start.go`). For that capture to populate anything other
than `orchestration.type = "manual"`, the layer that spawns agents must export
the vars. The **lattice-orchestrator skill** is that layer for Stage 11
orchestration runs.

## Where the skill change lives

The skill is authored in the **c11 repo**, not here:

- Source: `code/c11/skills/lattice-orchestrator/{SKILL.md,references/orchestrator.md}`
- Commit: `d44c94889c1dec5e1f0be62f5483cccc78c9bace`
  (branch `feat/etch-12-etch-skill-export`, remote
  `github.com/Stage-11-Agentics/c11`)

The change adds the `ETCH_*` export block to all three delegator boot templates
(fast-track, inline-full, sub-agent-full) immediately after `export LATTICE_ROOT=…`,
plus a HARD RULE in the sub-agent boilerplate section documenting the full
contract, and per-sub-agent `ETCH_AGENT_ROLE` / `ETCH_PARENT_SESSION_ID` on the
atomic-cwd launch line.

> Note: the installed copy at `~/.claude/skills/lattice-orchestrator/` is a
> regenerated **copy**, not a symlink to the source. Editing the c11 source is the
> authoritative change; the installed copy is refreshed by c11's skill-install flow.

## The env-var contract (OUTPUT_SPEC.md §3)

| Var | → field | Notes |
|---|---|---|
| `ETCH_ORCHESTRATOR_TYPE` | `orchestration.type` | defaults to `"manual"` when absent |
| `ETCH_DISPATCH_METHOD` | `orchestration.dispatch_method` | `c11_delegator` / `headless_clear` / … |
| `ETCH_TICKET_ID` | `orchestration.ticket_id` | |
| `ETCH_RUN_ID` | `orchestration.run_id` | groups a run; one ULID across all delegators |
| `ETCH_AGENT_ROLE` | `orchestration.role` | `delegator` / `planner` / `impl` / `reviewer` / `fix` |
| `ETCH_WORKFLOW_VERSION` | `orchestration.workflow_version` | git short SHA of the skill |
| `ETCH_ORCHESTRATION_EXTRA` | `orchestration.extra` | JSON string → open property bag |
| `ETCH_PARENT_SESSION_ID` | `session.parent_session_id` | consumed at the hook layer, not inside `Orchestration` |

`CaptureOrchestration()` reads the first seven (the six `orchestration.*` fields
plus `extra`). `ETCH_PARENT_SESSION_ID` is handled in `session_start.go` and lands
on the top-level `Session.parent_session_id`, not on the `Orchestration` struct.

The contract is pinned by `internal/capture/orchestration_lattice_skill_test.go`,
which sets the vars exactly as the skill exports them and asserts every field.
