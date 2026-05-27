# Cairn

**Status:** Pre-implementation. Design crystallized 2026-05-23 after research sweep (see [RESEARCH.md](RESEARCH.md)). v0 scope defined; implementation has not started.

## One-liner

Cairn captures the metadata of agentic work — prompts, sessions, workflow versions, orchestration traces — and binds it to git commits so the next AI agent in a codebase has rich context about how past agents and humans worked there.

## Etymology

A *cairn* is a stack of stones left by past travelers to mark the path for those who follow. Each agent leaves a marker on each commit; future agents read the markers to navigate the territory.

## Philosophy

**The user is the next agent.** Cairn's primary consumer is not a human opening a dashboard. It is the next AI agent starting work in a codebase, reading prior context to inform its decisions. Humans benefit through their agents — the agent surfaces patterns from Cairn during normal conversation.

**Nothing changes in the operator's workflow.** Capture happens via git hooks fired by the operator's existing actions (`git commit`, `git push`, branch creation, merge, rebase). No launcher wrapper, no special invocation, no `cairn-claude` instead of `claude`. The operator does what they already do; Cairn captures invisibly.

**Git is the substrate.** Cairn runs on bare git. No forge required. Forge integrations (PR state, CI outcomes) are deferred — v0 derives everything from git alone.

**Workflow is a first-class artifact.** Most prior art binds metadata to *code* (the diff, the commit). Cairn additionally binds to the *workflow* — which orchestrator at which version produced this run. Workflows iterate; the version that produced a successful PR is recoverable and comparable.

**The workflow slot is open-ended.** Workflows vary wildly across teams (`lattice-orchestrator`, Devin-style, plan→implement→review, RAG-driven, eval-gated, hand-rolled bash) and the shape evolves monthly. Cairn provides the binding slot; the workflow author fills it with whatever they want recorded. Property bag, not fixed schema.

**Dumb CLI, smart consumer.** Cairn's CLI is a data layer that returns structured matches. The agent reading the data does the reasoning. No embedded LLM in Cairn; the consuming agent is the intelligence.

**High concurrency by default.** The design target is 20+ concurrent agent sessions per repo, 60–80 across a developer's machine. This is the operating point, not a stretch goal.

## Standards alignment: ride Agent Trace

The industry standardized the *attribution schema* in January 2026 with [Agent Trace](https://github.com/cursor/agent-trace) — authored by Cognition, RFC'd by Cursor, signed by Anthropic, OpenAI, Google, Vercel, Cloudflare, and ~17 partners. Vendor-neutral JSON, storage-agnostic, deliberately punts on prompt capture, quality assessment, and workflow modeling.

Cairn's strategy: **emit Agent Trace records as a side-channel output of every capture.** Whatever Cairn records internally also serializes to `agent-trace.json`. The Cursor / Cognition / Anthropic toolchains can read what Cairn produces for the cost of one serializer. Free interop; zero negotiation.

Cairn owns the layer Agent Trace leaves open: prompt corpus, workflow versioning, derived outcomes, multi-agent attribution. Don't compete with Agent Trace. Ride it.

[Contextual Commits](https://github.com/berserkdisruptors/contextual-commits) (typed action lines in commit bodies: `intent`, `decision`, `rejected`, `constraint`, `learned`) is a second emerging standard at the in-commit-body layer. Cairn supports emitting these as an opt-in per-repo setting — capture is invisible by default; commit-body mutation is not silent.

## Architecture: three surfaces

| Surface | Lives | Fires on | Purpose |
|---|---|---|---|
| **Git hooks** | `.git/hooks/` via `core.hooksPath → .cairn/hooks/` (checked-in) | `post-commit`, `post-checkout`, `pre-push`, `post-merge`, `post-rewrite` | Capture — write bindings, slice sessions |
| **Agent tool-use hooks** | `~/.claude/settings.json` (+ Codex / OpenCode equivalents) | `PreToolUse` / `PostToolUse` matching `git commit` etc. | Attribution — write deterministic session markers |
| **SKILL.md** | `.claude/skills/cairn/` (and per-agent equivalents) | Agent reads at session start | Consumption — query Cairn, surface patterns to operator |

Capture and attribution are passive observability — no agent reasoning, no operator action. Consumption is skill-mediated, in the contract every agent host already supports.

`cairn install` wires all three: sets `git config core.hooksPath`, edits agent settings files, drops the skill into the project.

## Storage

| Tier | Lives | What goes here |
|---|---|---|
| **Working state** | `.cairn/` in the project (checked into git) | bindings, hooks, config, locally-rendered cooked artifacts. Operator-inspectable, version-controlled. |
| **Sync** | `refs/notes/cairn` (git notes namespace) | bindings serialized for cross-machine portability and standard-tool interop. Written on `pre-push`. |
| **Source pointers** | `~/.claude/projects/.../<id>.jsonl`, `~/.codex/sessions/...`, etc. | Raw session JSONLs, untouched, pointed to from bindings. |
| **Pinned (on demand)** | `.cairn/pinned/<sha>/` or a content-addressed store | Archival copies of sessions referenced by long-lived bindings. `cairn pin` triggers. |

Storage is intentionally hybrid: sidecar dotfolder for working state (inspectable, version-controlled) + git notes for sync (standard tool can read). Pointer by default; pin on demand.

## Data model

### Unit of capture: per-commit binding

Every commit (agent-produced or human-typed) gets a binding written by the git hook. Atomic; always works.

```jsonl
{"id":"01K…","ts":"…","commit":"abc123","branch":"feat/x","cwd":"…","session":{"agent":"claude","model":"…","session_id":"…","slice":{"from_idx":42,"to_idx":117}},"workflow":{"name":"lattice-orchestrator","version":"a3f8c2e","phase":"impl","ticket":"FT-481","custom":{"reviewer_model":"gpt-5","retry_count":2}},"actor":"agent"}
{"id":"01K…","ts":"…","commit":"def456","branch":"main","cwd":"…","session":null,"workflow":null,"actor":"human"}
```

Fields populated where derivable; nullable where not. Human commits become `agent=null` bindings so the timeline stays complete.

**The `workflow` field is a property bag.** Cairn does not define its keys. The workflow author records whatever is meaningful — phase, ticket, sub-agent role, model used for review, retry count, eval gate result, arbitrary nested objects. Cairn stores it; queries index across whatever keys exist.

### Aggregation: orchestration run

A run is an aggregation across commits, derived where workflow structure exists. For Stage 11 with Lattice: ticket dispatch → ticket close. For solo dev without a Lattice-equivalent: no runs, just commits. Queries operate on commits directly in that case.

Runs are derived, not declared. Cairn does not own a run lifecycle.

### Outcomes (git-derivable only in v0)

- Number of commits per branch
- Time between first commit and merge to main
- Amend / force-push / reset counts (thrash signal)
- Branch lifetime
- Whether the branch reached main at all

Forge-only outcomes (CI status, PR review cycles, merge timing from PR events) are deferred.

## Attribution: multi-agent at high concurrency

**Design target: 20+ concurrent agent sessions per repo, 60–80 across a developer's machine.** No surveyed tool handles this density. The single-agent-per-laptop assumption is baked into every prior-art tool. Cairn's smoke test is the real Stage 11 pattern: many Claude / Codex / Gemini / OpenCode sessions in c11 panes, across worktrees and shared branches, all touching git concurrently.

When `post-commit` fires, Cairn answers two questions:

1. **Which session(s) produced this commit?**
2. **What slice of that session is relevant?**

**Primary mechanism: agent tool-use hook writes a marker file.**

Before `git commit` runs, the agent's `PreToolUse` hook fires (Claude Code; equivalents on Codex / OpenCode). The hook writes `~/.cairn/active/<session-id>.json` with `session_id`, `cwd`, `branch`, `pid`, and the session's current event-index. The git `post-commit` hook reads the most recent marker for this `cwd` + `branch` and binds.

**Fallback: process-tree walk.**

If no agent hook is installed, the git hook walks parent PIDs (`ps -o ppid` on macOS, `/proc/<pid>/stat` on Linux) until it finds an agent process. Less precise but works.

**Disambiguators:** process tree, `cwd`, `branch`, tool-call event-index, c11 surface metadata when present.

**Marker lifecycle.** Marker files are cheap, but at 60+ concurrent sessions the `~/.cairn/active/` directory needs maintenance. Cleanup triggers:
- Session-end hook removes its own marker
- Periodic prune of markers whose `pid` is no longer alive
- TTL on markers older than 24h

Sub-agents producing commits are attributed to the immediate session. Orchestration parent (the orchestrator that drove the sub-agent) is a query-time inference from c11 lineage or Lattice ticket join — not a capture-time concern.

## Capture details

Git hooks:

| Hook | Use |
|---|---|
| `post-commit` | Write binding; emit `agent-trace.json` sidecar; pin marker to commit SHA |
| `post-checkout` | Branch creation = potential run-start signal |
| `pre-push` | Sync bindings to `refs/notes/cairn` for remote portability |
| `post-merge` | Run-close signal |
| `post-rewrite` | **Move bindings to follow rewritten commits.** Critical for correctness through rebases and amends. |

Session storage:

- Pointer by default. Cairn stores `~/.claude/projects/.../<id>.jsonl` paths, not copies.
- Slice is `(session_id, from_idx, to_idx)` — bounded by `git commit` tool-call events.
- `cairn pin <commit-or-binding>` copies the session JSONL to a content-addressed store for archival.

## Consumption

Agent reads SKILL.md at session start. Skill tells the agent to:

1. Run `cairn context` once at session start; digest the returned structured data.
2. **Selectively surface** relevant patterns to the operator — only when a surfaced pattern would change a decision the operator is about to make. Not chatty. Not silent. *"Cairn noticed prompts shaped like this one tend to take three review cycles — want to rephrase?"* is appropriate. Constant background commentary is not.
3. Query `cairn query <question>` when relevant prior work would inform the current decision.

The CLI is dumb: takes a question or filter, returns structured matches. The agent reasons over the matches.

## Trust and visibility

Provenance tooling that mutates the substrate silently poisons the substrate. (See the VS Code Copilot Co-Author backlash, [Penligent writeup](https://www.penligent.ai/hackinglabs/vs-code-copilot-co-author-when-ai-attribution-becomes-a-supply-chain-trust-problem/).) Cairn's posture:

- **Capture is explicit opt-in at the repo level.** Running `cairn install` is the consent gesture. Until then, nothing fires.
- **Bindings are operator-inspectable.** `.cairn/bindings.jsonl` lives in the repo, version-controlled, readable in any text editor.
- **`cairn inspect [<commit>|<session>|<branch>]`** dumps everything Cairn has recorded about a target.
- **`cairn redact <binding-id>`** removes a binding, with the redaction itself recorded as an event (audit-trail-preserved).
- **No silent commit-message mutation.** Contextual Commits emission requires an explicit per-repo opt-in.
- **No silent ref-push to remote.** `pre-push` sync to `refs/notes/cairn` is on by default but visible in repo config and disabled with one setting.
- **Source pointers, not silent copies.** Session JSONLs stay where the agent put them. `cairn pin` is an explicit operation.

## What v0 ships

- `cairn install` — wires git hooks, agent settings, and the SKILL.md
- Five git hooks with marker-file + process-tree-walk attribution
- `PreToolUse` / `PostToolUse` hooks for Claude Code, Codex, OpenCode
- Per-commit binding format (jsonl in `.cairn/bindings.jsonl`)
- Open-ended `workflow` property bag
- **Agent Trace emission** — every binding serializes to an `agent-trace.json` sidecar
- Sidecar (`.cairn/`) + git-notes-on-push storage
- `cairn context` and `cairn query` CLI subcommands
- `cairn inspect` and `cairn redact` operator-control subcommands
- Git-derivable outcome computation
- SKILL.md template
- Marker lifecycle (session-end cleanup + dead-pid prune + TTL)

## What's deferred

- **Contextual Commits emission** (opt-in commit-body action lines) — defer to v1; design is straightforward but mutates commit messages, wants real-use validation first
- Forge integrations (Forgejo / GitHub / GitLab) — webhook receivers, CI status, PR outcomes
- Polling-based forge enrichment for users without webhook infrastructure
- Per-line attribution (editor-side hooks)
- Voice / SuperWhisper capture
- MCP tool surface (`mcp__cairn__*`)
- Cross-machine portability of pinned sessions
- Meta-feedback loop: Cairn-side analysis that pings the operator with prompt-pattern coaching (v2)
- Run-as-primitive (only emerges as a derived aggregation where structure exists)

## Open questions

1. **Slicing edge cases.** Tool-call boundaries from `git commit` events are the working plan, but `git commit --amend`, partial-staging commits, and orchestrator-driven multi-commit sequences need pinning down.
2. **Workflow version resolution.** A workflow's version needs to be reproducible. SKILL.md content-hash at run-time is the working plan, but skills live in many places (`~/.claude/skills/`, `.claude/skills/<project>/`, c11's installed-skill set). Resolution order needs a spec.
3. **Multi-agent orchestration parent attribution.** Capture-time hook walk vs query-time inference. Current lean: query-time, to keep the hook dumb.
4. **Bindings.jsonl growth strategy at 20+ concurrent sessions over months.** Append-only is fine for a year; eventually needs rotation, indexing, or sharding. Not v0 work but worth tracking.

## Prior art

### Standards (emit, don't compete)

- **[Agent Trace](https://github.com/cursor/agent-trace)** — vendor-neutral attribution schema. Cognition / Cursor / 17 partners (Anthropic, OpenAI, Google, Vercel, Cloudflare, …). Storage-agnostic; punts on prompts, quality, workflow. Cairn emits this.
- **[Contextual Commits](https://github.com/berserkdisruptors/contextual-commits)** — typed action lines (`intent` / `decision` / `rejected` / `constraint` / `learned`) in commit bodies. Reference impl as Claude Code skills. Cairn emits this opt-in.

### Direct competitors

- **[Entire CLI](https://github.com/entireio/cli)** — closest analogue. Multi-agent capture, parallel-branch storage (`entire/checkpoints/v1`). Single-developer flow; no workflow-versioning; no outcome correlation. The branch-as-sidecar trick is worth studying.
- **[Git AI](https://usegitai.com)** ([git-ai-project/git-ai](https://github.com/git-ai-project/git-ai)) — git notes under `refs/notes/ai`, per-line attribution, v3.0.0 spec.
- **[h5i](https://h5i.dev)** ([Koukyosyumei/h5i](https://github.com/Koukyosyumei/h5i)) — Rust, "reasoning versioning" as DAG nodes in `refs/h5i/context`, SessionStart hooks.
- **[promptcellar](https://github.com/dominiek/promptcellar-for-claude-code)** — Claude Code plugin, JSONL logs to `.prompts/`. Single-agent; human-audience; future MCP server planned.
- **[claude-mem](https://github.com/thedotmack/claude-mem)** — persistent compressed-context across sessions. Closest spiritual sibling on the next-AI-agent-as-consumer axis; ignores the git substrate.
- **[claude-code-hooks-multi-agent-observability](https://github.com/disler/claude-code-hooks-multi-agent-observability)** — full Claude Code hook coverage, SQLite + WebSocket dashboard. Human dashboard, not git-bound.

### Adjacent

- **[MLflow Prompt Registry](https://mlflow.org/prompt-registry)** — closest mature "version everything → bind to outcomes" model. Our workflow-version-to-PR-outcome pattern is essentially this applied to git/PRs rather than ML experiments.
- **LangSmith / Langfuse / Helicone / Braintrust / Arize Phoenix** — LLM-runtime observability. Capture prompts, tool calls, costs, evals. None bind traces to commits or PRs.
- **[Cloudflare Artifacts](https://blog.cloudflare.com/artifacts-git-for-agents-beta/)** — "versioned storage that speaks Git." Confirms git as the convergence substrate for agent state.
- **[GitHub Agentic Workflows](https://github.github.com/gh-aw/)** — workflow-as-branch with phase commits as state checkpoints. Convergent with Cairn's "workflow is versioned" thesis.
- **[Memento](https://github.com/cdwilson/memento)** — git extension attaching AI session transcripts as git notes.
- **[blameprompt](https://github.com/Ekaanth/blameprompt)** — git-log-style prompt timeline with cost.
- **[Propel Code](https://propelcode.ai)** — SaaS arguing for session provenance schemas.

### Academic

- **[PROV-AGENT](https://arxiv.org/abs/2508.02866)** (arXiv 2508.02866) — W3C PROV extension for AI agent workflows. Not git-bound; aimed at HPC/cloud.
- **[Reasoning Provenance for Autonomous AI Agents](https://arxiv.org/pdf/2603.21692)** (arXiv 2603.21692, 2026) — *why* not *what* provenance.
- **[Fingerprinting AI Coding Agents on GitHub](https://arxiv.org/pdf/2601.17406)** (arXiv 2601.17406) — empirical: agents leave statistically distinguishable commit signatures.
- **[Promises, Perils, and (Timely) Heuristics for Mining Coding Agent Activity](https://arxiv.org/pdf/2601.18345)** (arXiv 2601.18345) — catalog of visible traces; warns of temporal volatility.
- **[Beyond Resolution Rates: Behavioral Drivers of Coding Agent Success and Failure](https://arxiv.org/pdf/2604.02547)** (arXiv 2604.02547) — trajectory structure distinguishes success from failure. Direct support for workflow-shape → outcome correlation.
- **[Which Agent Causes Task Failures and When?](https://arxiv.org/pdf/2505.00212)** (arXiv 2505.00212) — multi-agent failure attribution.

**Convergence observed:** the field has fragmented into three storage patterns (git notes, sidecar dotfolders, parallel branches). Agent Trace is the standard for *what* to record; the *where* remains pluralistic. Cairn picks sidecar-plus-notes for working state + sync.

## Differentiation hypothesis

What Cairn adds over the prior art:

1. **Workflow as a versioned first-class artifact with an open-ended property bag.** Git AI binds to code; h5i binds to reasoning; Agent Trace binds to attribution. None model the workflow blueprint itself as a versioned, queryable entity with a flexible schema. Cairn does.
2. **Multi-agent attribution at high concurrency.** Marker-file + process-tree walk designed for 20+ concurrent sessions per repo. No surveyed tool targets this density.
3. **Agent-as-consumer as the design center.** Consumption is skill-mediated and selectively proactive — not human-dashboard-first.
4. **Capture works without a forge or an orchestrator.** Hooks fire on bare git; metadata accumulates whether the operator is on GitHub, Forgejo, or no forge at all.
5. **Standards-aligned by default.** Emits Agent Trace alongside its internal model. Free interop with the broader ecosystem.
