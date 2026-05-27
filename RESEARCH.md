# Research: Prior Art for Agent Orchestration Metadata in Source Control

**Project:** Cairn (formerly Forge++ — name flipped 2026-05-23; both fine for now)
**Date:** 2026-05-23
**Researcher:** Claude Code research agent (spawned from Forge++/Cairn Design pane)

## Summary

**Agent Trace is the de-facto industry standard for AI code provenance, and that is a win for Forge++.** Released as a Cursor RFC in January 2026, authored by Cognition, and signed by ~17 organizations (Anthropic, OpenAI, Google, Vercel, Cloudflare, and the major AI coding platforms), it covers exactly the layer Forge++ has no interest in owning: the vendor-neutral attribution schema. It is deliberately storage-agnostic (sidecar / git notes / DB all acceptable), explicitly punts on prompt capture (links to external conversation URLs rather than embedding), explicitly punts on quality assessment, and explicitly punts on workflow modeling. In other words: **it standardizes the boring layer, leaves the interesting layer wide open, and gives Forge++ free interop with every tool in the ecosystem for the cost of one serializer.** Emit Agent Trace records as a side-effect of Forge++ capture and the entire ecosystem can read what we produce — no negotiation, no spec war.

**The heaviest competitor is Entire.io**, founded by ex-GitHub-CEO Thomas Dohmke with a $60M seed from Felicis. Their pitch is verbatim Cairn's pitch ("git-compatible database unifying code, intent, constraints, and reasoning… semantic reasoning layer enabling multi-agent coordination"). The open-source `entire/cli` is a real implementation. They claim multi-machine support ("sessions from all your machines push to the same checkpoint branch"), but the docs don't address concurrency under load and they don't model workflows as first-class entities or derive outcomes from CI/PR state. Cairn's defensible space is *upstream* of where Entire is investing.

Beyond Agent Trace and Entire, the field has fragmented into three storage patterns: (a) commit-body trailers (Lore, Contextual Commits), (b) sidecar dotfolders (`.prompts/`, `.sessions/`, `.brain/`), and (c) parallel git branches / git notes (Entire's `entire/checkpoints/v1`; Mesa's Agent Blame uses notes). **Two gaps remain open enough to plant a flag in, and Cairn should plant both:**

1. **Workflow as a versioned artifact bound to derived outcomes.** Nobody is modeling the orchestration itself as a first-class entity that can be versioned, replayed, and correlated to PR-level outcomes. The metadata layer here should stay **flexible and open-ended** — workflows look very different across teams and will evolve fast; lock the schema and we lose. Treat it more like a tag/property bag than a fixed schema.
2. **Multi-agent attribution at high concurrency, across multiple machines.** The single-agent-on-a-laptop assumption is baked into every tool surveyed. Cairn's design target is the **real** Atin pattern: ~20 terminal agents (Claude / Codex / Gemini / OpenCode) running concurrently inside c11 on Hyperion in a single repo, plus ~20 more across three other repos on Hyperion, plus agents on Atlas (the always-on Mac Studio at home, Tailscale-connected) touching some of the same repos via push/pull — 60–80+ concurrent sessions across two machines, all transiting the same git remotes. That density and topology aren't edge cases — they're the shape of multi-agent orchestration in 2026 — and capture/attribution/merge has to work natively across machines on day one. The substrate choice (sidecar files vs. git notes vs. parallel branches vs. per-session refs) is downstream of this requirement, not upstream.

## New competitors / projects

### Standards plays

- **Agent Trace** — [cursor/agent-trace](https://github.com/cursor/agent-trace) — **clear industry winner on attribution schema.** Authored by Cognition, RFC'd by Cursor, ~17 partner orgs (Anthropic / OpenAI / Google / Vercel / Cloudflare and others). Vendor-neutral JSON trace records connecting code ranges to conversations and contributors. Storage-agnostic by design. **Does not capture prompts** (links to external conversation URLs). **Does not capture workflows.** Single-agent-per-trace model — multi-conversation support exists, but no workflow-as-artifact concept. **Forge++ should emit Agent Trace as a side-channel output**: every captured session ships its `agent-trace.json` for free interop with Cursor / Cognition / Anthropic tooling. Zero cost to Forge++, maximal interop. Don't compete with it; ride it.
- **Contextual Commits** — [berserkdisruptors/contextual-commits](https://github.com/berserkdisruptors/contextual-commits) — open spec by Veselin Dimitrov, [HN launch March 2026](https://news.ycombinator.com/item?id=47354263). Extends Conventional Commits with five typed action lines embedded directly in commit message bodies (no infra, no separate store):
  - `intent(scope): <goal>` — e.g. `intent(notifications): batch emails instead of per-event`
  - `decision(scope): <choice + rationale>` — e.g. `decision(queue): SQS over RabbitMQ for managed scaling`
  - `rejected(scope): <option + why discarded>` — e.g. `rejected(queue): RabbitMQ — requires self-managed infra`
  - `constraint(scope): <hard boundary>` — e.g. `constraint(api): max 5MB payload, 30s timeout`
  - `learned(scope): <gotcha or non-obvious fact>` — e.g. `learned(stripe): presentment ≠ settlement currency`

  Reference implementation ships as two Claude Code skills (`contextual-commit` auto-writes lines from session context; `recall` reconstructs prior decisions via `/recall`, `/recall <scope>`, or `/recall <action>(<scope>)`). Installable via `npx skills add berserkdisruptors/contextual-commits`. Claims compatibility with Claude Code, Copilot, Cursor, Gemini CLI, and 26+ agent platforms. Trivial commits require zero lines — convention is minimal/incremental. Complementary to Agent Trace (decision recovery, in-commit-body) rather than competing. **Possible Forge++ emit target alongside Agent Trace**: action lines are cheap to generate from captured session reasoning and slot naturally into the commit Forge++ is already hooked into.

### Capture tools (direct competitors)

- **Entire CLI / Entire.io** — [entireio/cli](https://github.com/entireio/cli), [entire.io](https://entire.io), [docs.entire.io](https://docs.entire.io/core-concepts) — **the most serious competitor in this space, by a wide margin.** Founded by **Thomas Dohmke** (former GitHub CEO). **$60M seed** led by Felicis with Madrona and M12 ([Hello Entire World](https://entire.io/blog/hello-entire-world)). Their pitch is verbatim Cairn's pitch: *"a git-compatible database unifying code, intent, constraints, and reasoning"* plus *"a semantic reasoning layer enabling multi-agent coordination"* plus *"an AI-native software development lifecycle"* — "the assembly line for the era of agents." CLI is open source / MIT; commercial layer is the entire.io platform that the checkpoints branch pushes to.

  **Architecture (what's documented):** hooks into Claude Code / Codex / Cursor / Gemini / Copilot CLI / Factory / Pi / OpenCode. Captures full session (transcript, prompts, files touched, token usage, tool calls). Stores as checkpoints on a permanent shared branch `entire/checkpoints/v1`. Code commits stay on user branches; Entire never writes to the active branch. Nested sessions (sub-agents from `Task` tool) preserved as hierarchy. **Multi-machine claim:** *"Sessions from all your machines push to the same checkpoint branch."* Push to entire.io for the hosted view.

  **What's still not in the docs (and matters for Cairn):**
  - **Concurrency story under load is unverified.** A single shared mutable branch with N concurrent pushers from different sessions is the classic merge-churn / last-pusher-wins surface. No documentation of a fan-shaped per-session ref scheme, an indexer process, or a merge strategy. At Atin's 20-agents-per-repo density this would need stress testing; I can't tell from docs alone whether it works or quietly drops checkpoints.
  - **Workflow is not a first-class entity.** Their model is *session → commit*, not *workflow-version → session(s) → commit(s) → outcome*.
  - **Outcome derivation absent.** Nothing about CI status, PR state, rework count, or correlating sessions to landed-quality.
  - **Commercial moat is the cloud view, not the local capture.** The CLI is MIT; the differentiation sits in entire.io's hosted index.

  **Reading this honestly:** Entire CLI is the closest existing system to Cairn's intent and has real heavyweight backing. The multi-machine claim is real; the "20 concurrent agents pushing to one branch" claim is unverified. Cairn's defensible space is *upstream* of where Entire is investing — workflow-as-artifact, outcome correlation, and verified-under-load multi-agent merge.
- **promptcellar** — [dominiek/promptcellar-for-claude-code](https://github.com/dominiek/promptcellar-for-claude-code) — Claude Code plugin, JSONL logs to `.prompts/YYYY/MM/DD/<session-id>.jsonl`. Uses SessionStart / UserPromptSubmit / PostToolUse / Stop hooks; snapshots git state, captures tokens/cost. Audience is humans; future MCP server (M5) for AI consumption is on the roadmap. Single-agent. No commit binding (uses git-state matching).
- **claude-mem** — [thedotmack/claude-mem](https://github.com/thedotmack/claude-mem) — persistent compressed-context across sessions, multi-agent (Claude / OpenClaw / Codex / Gemini / Hermes / Copilot / OpenCode). 5 lifecycle hooks, SQLite + Chroma hybrid (keyword + vector), Bun HTTP worker on localhost:37777. Not git-bound; **the goal is to feed the next session, not the next agent in the same repo.** Closest in spirit to Forge++'s "next-AI-agent-as-consumer" axis but ignores the git substrate.
- **claude-code-hooks-multi-agent-observability** — [disler/claude-code-hooks-multi-agent-observability](https://github.com/disler/claude-code-hooks-multi-agent-observability) — full coverage of all 12 Claude Code hooks, SQLite + WebSocket dashboard, multi-agent via `source-app` + session-id keys. Human dashboard, not git-bound, not agent-consumable. Real-time monitoring rather than persistent provenance.
- **Promptcellar / claude-mem / Entire** all share the same single-agent assumption: "this machine, this terminal, this human → these prompts." Nobody handles the "Atin + 3 agents working in parallel git worktrees on the same repo" case cleanly.

### Adjacent capture/spec tools mentioned in survey listicles (verify before relying)

- **Tessl** — [tessl.io](https://tessl.io/) — Guy Podjarny's (ex-Snyk) agent-enablement platform. Spec-driven, not provenance-focused. Spec Registry (10k+ pre-built library specs) + Framework (spec-as-long-term-memory). Adjacent: aims to *prevent* drift rather than capture it.
- **Augment Code, Kiro, GitHub Spec Kit, OpenSpec, BMAD, Google Antigravity** — all shipped spec-driven flows in 2025–2026. None capture session/workflow provenance as a first-class concern.

## Academic / research

- **PROV-AGENT: Unified Provenance for Tracking AI Agent Interactions in Agentic Workflows** — [arXiv 2508.02866](https://arxiv.org/abs/2508.02866) — extends W3C PROV, leverages MCP, captures agent prompts/responses/decisions within broader workflow context. Open-source, "near real-time." **No git/source-control binding.** Aimed at edge/cloud/HPC, not coding workflows specifically.
- **Reasoning Provenance for Autonomous AI Agents** — [arXiv 2603.21692](https://arxiv.org/pdf/2603.21692) (March 2026) — proposes "reasoning provenance" — *why* not *what*, beyond execution traces. References LangGraph/LangSmith integration. Behavioral analytics across populations. Not git-bound.
- **Fingerprinting AI Coding Agents on GitHub** — [arXiv 2601.17406](https://arxiv.org/pdf/2601.17406) — empirical study showing AI coding agents leave distinguishable signatures in commits. Codex: 67.5% feature importance from multiline commit patterns; Claude Code: 27.2% from conditional structure. **Implication:** even without explicit metadata, statistical fingerprinting works — but multi-agent attribution on the same commit remains unsolved.
- **Promises, Perils, and (Timely) Heuristics for Mining Coding Agent Activity** — [arXiv 2601.18345](https://arxiv.org/pdf/2601.18345) — catalogs visible traces (`.claude/`, `.cursor/`, `.aider.conf.yml`, co-author trailers, branch/PR naming). Warns of false positives, attribution challenges, and the temporal volatility of heuristics. Useful as a baseline for "what we can derive without explicit capture."
- **Understanding Code Agent Behaviour: An Empirical Study of Success and Failure Trajectories** — [arXiv 2511.00197](https://arxiv.org/pdf/2511.00197)
- **Beyond Resolution Rates: Behavioral Drivers of Coding Agent Success and Failure** — [arXiv 2604.02547](https://arxiv.org/pdf/2604.02547) — finds trajectory *structure* distinguishes success from failure. Direct support for "workflow shape → outcome" correlation, which is exactly Forge++'s pitch.
- **Which Agent Causes Task Failures and When?** — [arXiv 2505.00212](https://arxiv.org/pdf/2505.00212) — multi-agent failure attribution. Relevant for the "which workflow phase broke this PR" analytics.

## Adjacent solutions worth knowing

- **MLflow Prompt Registry / Version Tracking** — [mlflow.org/prompt-registry](https://mlflow.org/prompt-registry) — closest mature model for "version everything → bind to outcomes." Prompts, agents, app code all versioned and linked to traces/evals/metrics. Cloud or self-hosted. Coding-agent-agnostic. **Forge++'s "workflow-version → PR-outcome" binding is essentially MLflow's pattern applied to git/PRs rather than ML experiments.**
- **LangSmith / Langfuse / Helicone / Braintrust / Arize Phoenix** — full LLM-observability stack. Capture prompts, tool calls, latencies, costs, evals. Multi-framework. **None bind traces to git commits or PRs** — they're application-runtime observability, not source-control provenance. Forge++ could ingest from these via webhook/SDK for the prompt-level data without building its own capture pipeline.
- **Moda (YC W26)** — [ycombinator.com/companies/moda](https://www.ycombinator.com/companies/moda) — "Datadog for AI agents." Production agent reliability, surfaces hallucination/laziness/tool-call-failure patterns. Aimed at *deployed agent products*, not coding agents. Useful as proof of "agent observability is a fundable market."
- **Cloudflare Artifacts** — [blog.cloudflare.com/artifacts-git-for-agents-beta](https://blog.cloudflare.com/artifacts-git-for-agents-beta/) — "versioned storage that speaks Git," sold as a primitive for agent workflows. Confirms git as the convergence substrate for agent state.
- **GitHub Agentic Workflows** ([gh-aw](https://github.github.com/gh-aw/)) — workflow-as-branch pattern with phase commits as state checkpoints. Adjacent and convergent with Forge++'s "workflow is versioned" thesis.
- **claude-replay** ([anthropics/claude-replay](https://github.com/anthropics/claude-replay)) — turns transcripts into step-by-step playback. Visualization layer, not provenance layer.

## Convergence patterns

- **Storage substrate:** three live patterns — (1) git notes (`refs/notes/...`), (2) sidecar dotfolders (`.prompts/`, `.sessions/`, `.brain/`, `.provenance/`), (3) parallel git branch (Entire's `entire/checkpoints/v1`). Agent Trace explicitly leaves choice to implementers. No clear winner; sidecar dotfolders are simplest and most-deployed, git notes are most theoretically clean, parallel branches scale best.
- **Schema substrate:** Agent Trace is the front-runner for vendor-neutral attribution. Contextual Commits / Lore for in-commit-body decision records. They aren't direct competitors — different layers. A serious tool emits both.
- **Hook substrate:** SessionStart / UserPromptSubmit / PreToolUse / PostToolUse / Stop are now standardized across Claude Code, Codex CLI, Cursor (via `.cursor/hooks.json`), Gemini (`.gemini/settings.json`), OpenCode. Per-agent hook capture is solved, multi-agent merge is not.
- **Consumer:** the field is bifurcating. Most capture tools (promptcellar, Entire, Memento, claude-replay) target humans. claude-mem, MLflow, and Cursor's Memories/MCP integrations target the next AI session. Almost nobody targets *another, different* AI agent in the same repo — that's Forge++'s lane.
- **Spec-driven adjacent:** Tessl / Kiro / GitHub Spec Kit / Cursor / Claude Code skills are converging on `SKILL.md` / spec-as-prompt-context. Forge++'s SKILL.md-tells-agent-to-query-context-CLI pattern fits this exact convergence.
- **Trust:** VS Code's Copilot Co-Author backlash ([Penligent writeup](https://www.penligent.ai/hackinglabs/vs-code-copilot-co-author-when-ai-attribution-becomes-a-supply-chain-trust-problem/)) established the norm: provenance must be explicit opt-in, accurate, and visible. Silent metadata mutation poisons the substrate.

## Gaps / differentiation opportunities

### 🚩 Plant the flag here

1. **Workflow as a versioned artifact bound to derived outcomes — PRIMARY.** No existing tool captures *the workflow itself* as a versioned first-class entity (e.g., "lattice-orchestrator v3.2 → ticket FT-481 → PR #1340 → merged on first push, CI green, no rework"). MLflow does this for ML training runs, not for coding-agent workflows. Agent Trace records contributions per file; it doesn't model the workflow as a first-class entity. Tessl/Kiro/Spec Kit model the *spec*, not the *workflow that consumes the spec*. This is genuinely empty space.

   **Design implication:** keep the metadata layer **flexible and open-ended** — closer to a tag / property-bag / extensible-key-value than a fixed schema. Workflows look wildly different across teams (lattice-orchestrator, Devin-style, plan→implement→review, RAG-driven, eval-gated, hand-rolled bash) and the shape evolves monthly. A rigid schema dies on contact with reality. Let the workflow author declare what's worth recording.

2. **Multi-agent attribution at high concurrency, across multiple machines — PRIMARY.** Single-agent capture is saturated. The unsolved problems are (a) many agents on one machine and (b) agents fanned out across machines all touching the same repo. Cairn's design target:
   - ~20 terminal agents running concurrently inside c11 on **Hyperion** (MacBook) in a single repo (mix of Claude Code, Codex, Gemini CLI, OpenCode, etc., across worktrees and shared branches)
   - ~20 more agents across three other repos at the same time on Hyperion
   - Plus agents running on **Atlas** (Mac Studio at home, always-on, Tailscale) touching some of the same repos via push/pull
   - Total: 60–80+ concurrent agent sessions, distributed across two machines, multiple agents per repo, multiple agent types per repo, all touching git through different working trees and possibly the same remote

   **The cross-machine dimension changes the substrate calculus.** A provenance record produced by an agent on Atlas needs to be visible to an agent that next picks up the repo on Hyperion — and vice versa. That implies the provenance store has to ride git's existing sync mechanisms (push/fetch), not live in a local SQLite that never crosses the network. Concretely:

   - **Sidecar dotfolders in the working tree** (promptcellar pattern) ride normal `git push`/`git pull` for free, but per-machine concurrent writes to the same file produce silent corruption — only viable with strict per-session-keyed filenames (`.prompts/YYYY/MM/DD/<session-id>.jsonl` style).
   - **Git notes** (Agent Blame, Memento pattern) travel with git as first-class objects but **do not push or fetch by default** — `refs/notes/*` requires explicit `git push origin refs/notes/*` and `git fetch origin refs/notes/*`. This is a footgun: cross-machine sync silently doesn't happen unless every Cairn install configures it. Workable but opinionated setup required.
   - **Parallel git branch** (Entire's `entire/checkpoints/v1` pattern) syncs naturally with regular pushes but introduces a single mutable shared ref with N concurrent writers across machines. Without a fan-shaped per-session ref scheme + indexer, this is a merge-churn surface waiting to fire under load.
   - **Likely right answer for Cairn:** per-session append-only refs (one ref per session, e.g. `refs/cairn/sessions/<session-uuid>`) that push/pull cleanly without ever colliding, plus a periodic indexer (running on a designated machine — Atlas is the obvious always-on candidate) that derives the queryable view into a separate ref. Git-native, machine-agnostic, race-free at the write surface, survives `git push --mirror`, and the indexer's failure doesn't lose data — it just delays the read view.

   **Identity & causality across machines:** session IDs must be globally unique (UUIDs, not per-machine counters). Records need wall-clock timestamps *and* a logical clock (Lamport or vector) if Cairn ever wants to reconstruct cross-machine causal order. Hostname, machine fingerprint, and operator identity should all be captured as separate fields — never collapsed into one "user" string.

   **No surveyed tool handles this combination natively.** Entire CLI claims multi-machine push to a shared branch but doesn't document the concurrency story under load. [claude-presence MCP / dux / Daytona-style worktree-isolation patterns](https://dev.to/sahil_kat/coordinate-multiple-claude-code-sessions-on-a-shared-repo-1dh4) coordinate single-machine collisions but not attribution/provenance. The [fingerprinting paper](https://arxiv.org/pdf/2601.17406) attacks attribution statistically but admits it stays noisy. **Cairn should make "Hyperion + Atlas + 20 agents in c11" the smoke test, not the stretch goal.**

   Also relevant for this lane:
   - **Agent Blame (Mesa.dev)** — [github.com/mesa-dot-dev/agentblame](https://github.com/mesa-dot-dev/agentblame), [Mesa deep-dive](https://www.mesa.dev/blog/agentblame-deep-dive) — line-level attribution stored in git notes, with SHA256 exact + normalized matching at post-commit time and a GitHub Actions step that transfers attribution through squash/rebase. Different architectural bet from Entire (notes vs branch). Confirms "provenance must be inspectable wherever the code is clonable" as a design principle worth keeping.
   - **The Chronicles of Foundation AI for Forensics of Multi-Agent Provenance** — [arXiv 2504.12612](https://arxiv.org/pdf/2504.12612) — chronological post-hoc attribution of multi-agent contributions from content alone, no internal memory required. Useful fallback when explicit capture is missing or corrupted.
   - **When Only the Final Text Survives: Implicit Execution Tracing for Multi-Agent Attribution** — [arXiv 2603.17445](https://arxiv.org/pdf/2603.17445) — same theme, more recent.

### Secondary

3. **Prompt-language analytics correlated with PR outcomes.** Lots of prompt-engineering surveys; lots of trajectory-outcome papers ([Beyond Resolution Rates](https://arxiv.org/pdf/2604.02547)). Nobody is mining a team's prompt corpus to surface "your prompt style X correlates with rework rate Y" for the *next agent* to learn from. This combines the prompt-capture work (promptcellar/claude-mem) with the outcome-derivation work (CI status / time-to-PR / rework count) — neither side currently bridges.
4. **Outcome derivation pipeline.** All capture tools record inputs richly and outcomes thinly. Outcome is usually "session ended." Deriving from git + forge (CI-on-first-push, time-to-PR, rework count, ticket-close-status) is conceptually obvious but absent in shipped tools. Forge++ should treat this as a P0 capability.
5. **SKILL.md → context-CLI bootstrap.** Cursor Memories / claude-mem / Basic Memory all do session-to-session continuity, but none do *fresh-agent walks-into-repo-and-queries-history*. SKILL.md plus a context CLI at session start is the right shape, and nobody is shipping it cleanly. Adjacent: Anthropic's [memory tool](https://platform.claude.com/docs/en/agents-and-tools/tool-use/memory-tool) and MCP servers like Basic Memory are close but not git-bound.
6. **Emit Agent Trace (and probably Contextual Commits).** Not a gap to fill — free interop wins. Whatever Forge++ records internally, also emit `agent-trace.json` blobs so the Cursor / Cognition / Anthropic toolchains can read them, and append Contextual Commits action lines to the commit body when the session produced explicit decisions / rejections / constraints / learnings. Standards alignment for the cost of two serializers, and we land naturally on whichever convention an adopting team already runs.

## Sources cited

- https://github.com/cursor/agent-trace
- https://cognition.ai/blog/agent-trace
- https://www.contextgraph.tech/learn/agent-trace
- https://www.infoq.com/news/2026/02/agent-trace-cursor/
- https://github.com/entireio/cli
- https://github.com/dominiek/promptcellar-for-claude-code
- https://github.com/disler/claude-code-hooks-multi-agent-observability
- https://github.com/thedotmack/claude-mem
- https://docs.claude-mem.ai/introduction
- https://agent-wars.com/news/2026-03-14-contextual-commits-open-standard-ai-agent-decision-context-git-history
- https://blakecrosley.com/blog/session-is-the-commit-message
- https://www.gmfoster.com/writing/ai-provenance
- https://examples.tely.ai/software-development-devops/10-best-developer-tools-for-capturing-ai-session-context-in-git/
- https://www.penligent.ai/hackinglabs/vs-code-copilot-co-author-when-ai-attribution-becomes-a-supply-chain-trust-problem/
- https://www.ycombinator.com/companies/moda
- https://www.buildmvpfast.com/blog/yc-w26-batch-agent-infrastructure-boom
- https://blog.cloudflare.com/artifacts-git-for-agents-beta/
- https://github.github.com/gh-aw/
- https://www.infoq.com/news/2026/02/github-agentic-workflows/
- https://tessl.io/
- https://tessl.io/blog/tessl-launches-spec-driven-framework-and-registry/
- https://mlflow.org/prompt-registry
- https://mlflow.org/docs/latest/genai/version-tracking/
- https://www.getmaxim.ai/articles/top-5-prompt-orchestration-platforms-for-ai-agents-in-2026/
- https://langfuse.com/docs/observability/overview
- https://www.langchain.com/langsmith/observability
- https://code.claude.com/docs/en/hooks
- https://developers.openai.com/codex/hooks
- https://code.claude.com/docs/en/memory
- https://platform.claude.com/docs/en/agents-and-tools/tool-use/memory-tool
- https://arxiv.org/abs/2508.02866 (PROV-AGENT)
- https://arxiv.org/pdf/2603.21692 (Reasoning Provenance for Autonomous AI Agents)
- https://arxiv.org/pdf/2601.17406 (Fingerprinting AI Coding Agents on GitHub)
- https://arxiv.org/pdf/2601.18345 (Promises, Perils, and Timely Heuristics)
- https://arxiv.org/pdf/2511.00197 (Understanding Code Agent Behaviour)
- https://arxiv.org/pdf/2604.02547 (Beyond Resolution Rates)
- https://arxiv.org/pdf/2505.00212 (Which Agent Causes Task Failures)
- https://github.com/anthropics/claude-code/issues/28300 (multi-agent collaboration request)
- https://dev.to/sahil_kat/coordinate-multiple-claude-code-sessions-on-a-shared-repo-1dh4
- https://entire.io
- https://entire.io/blog/hello-entire-world (Entire $60M seed, Thomas Dohmke)
- https://docs.entire.io/core-concepts
- https://www.mager.co/blog/2026-02-10-entire-cli/
- https://github.com/mesa-dot-dev/agentblame
- https://www.mesa.dev/blog/agentblame-deep-dive
- https://arxiv.org/pdf/2504.12612 (Chronicles of Foundation AI for Forensics of Multi-Agent Provenance)
- https://arxiv.org/pdf/2603.17445 (When Only the Final Text Survives)
- https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents
- https://docs.basicmemory.com/integrations/cursor
- https://github.com/vanzan01/cursor-memory-bank
- https://hackernoon.com/cursor-levels-up-with-10-release-adding-mcp-support-and-persistent-memory
