# Evolutionary Plan Review: Cairn

### Executive Summary
Cairn presents itself as a passive, flat metadata layer for agent session capture, but its structural design—decentralized, contention-free, Git-native storage—positions it to be something much more powerful: a real-time, decentralized "shared brain" and communication bus for autonomous multi-agent organizations. The biggest opportunity isn't just observability for humans; it's enabling true, asynchronous agent-to-agent coordination. If Cairn evolves well, it transforms a repository from a static collection of source files into a self-learning organism where every agent's failure or success compounds the intelligence of the next agent that touches the repo. 

### What's Really Being Built
Beneath the surface of a "flat metadata record per session," Cairn is building the **DNA and immune system of the codebase**. By storing prompts, tool use, dead ends, and outcomes locally in Git, Cairn is constructing a high-fidelity behavioral knowledge graph. It's not just a database of *what* happened; it's a versioned artifact of *intent* and *friction*. You are building the first native temporal memory for AI agents that travels flawlessly across network partitions and machine boundaries without requiring a centralized cloud. 

### How It Could Be Better
The current plan treats Cairn largely as a write-heavy historical archive waiting for a "future query engine" or human operator. To be genuinely better, Cairn must become an **active context provider**. Instead of waiting for Phase 2's `cairn query` CLI meant for humans, Cairn should immediately build an MCP (Model Context Protocol) server or a native context-injection CLI that agents query at the *start* of their session. If Agent B can automatically read Agent A's dead ends from three hours ago without human intervention, Cairn ceases to be just an audit trail and becomes an active velocity multiplier. The data must be consumable by the next AI, not just the next human.

### Mutations and Wild Ideas
*   **The "Ghost" Refactoring Agent:** A background agent that constantly watches `refs/cairn/sessions/*`. It doesn't write code; it mines recent sessions for repeated friction points, abandoned tool-calls, or CI failures, and autonomously generates new `SKILL.md` rules or constraint prompts for the repo. The repo actively heals its own context.
*   **ROI-Driven Orchestration (Bounty Market):** Since Cairn captures token costs, model types, and PR outcomes, it can act as an empirical pricing engine. An orchestrator could query Cairn: "Which model has the highest success rate and lowest cost for modifying the `auth` module?" Cairn dynamically shifts workloads to the most efficient models based on historical success rates.
*   **Git as a Vector Memory Bank:** Instead of just a JSON/table indexer, `cairn index` compiles a local, Git-tracked vector database of past session intents. When a new agent is asked to "fix the login bug," it semantic-searches Cairn to inject the exact prompt and reasoning trace of the last agent who touched the login flow.
*   **Pre-crime CI:** If an agent starts executing a pattern of tool calls that Cairn's historical metadata strongly correlates with a future CI failure, Cairn could interrupt the agent mid-session and inject a warning: "This orchestration pattern failed 4 times yesterday. Re-evaluate."

### What It Unlocks
*   **Asynchronous Swarm Coordination:** 60+ agents across multiple machines can coordinate implicitly through Git fetches. They don't need a centralized Redis or cloud database; they read the state of the swarm via `refs/cairn/sessions/*`.
*   **Resurrection of Abandoned Value:** Often, an agent explores a valid architectural path but abandons it due to token limits or a minor bug. Cairn unlocks the ability to "fork" an agent's reasoning from a specific point in the past and hand it to a more powerful model.
*   **Outcome-Driven Agent Evolution:** By binding workflows to PR/CI outcomes, organizations can mathematically prove which prompting strategies and agent frameworks actually deliver working software, moving the industry from "vibes" to empirical workflow engineering.

### Sequencing and Compounding
The current sequencing defers the consumer of the data to Phase 2 (and limits it to human CLIs). To compound faster, alter the sequence:
1.  **Phase 1: Plugin + Capture + MCP Server.** Ship the capture alongside a basic Model Context Protocol server. Ensure the agents writing the data are also the first consumers of it.
2.  **Phase 2: Outcome Binding & The Flywheel.** Tie the sessions to CI/PR states immediately. This is the crucial step that separates "noise" from "signal."
3.  **Phase 3: Human Tooling & Query CLI.** Humans will find the data useful once it's already structured and refined by the agents themselves.

### The Flywheel
The core self-reinforcing loop:
**Agent writes reasoning to Cairn -> Cairn Indexer correlates reasoning with CI/PR success -> Next Agent queries Cairn for successful patterns / dead ends -> Next Agent ships code faster and cheaper -> Agent writes better reasoning to Cairn.**
To set this spinning, the barrier for an agent to *read* Cairn must be as low as the barrier to *write* to it. The moment an agent uses a Cairn trace to avoid a 5-minute hallucination loop, the flywheel is in motion.

### Concrete Suggestions
1.  **Build `cairn-mcp`:** Expose the index natively to Cursor, Claude Code, and other runtimes via the standard Model Context Protocol. Give them tools like `search_past_dead_ends(file_path)` or `get_recent_intent(module)`.
2.  **Explicit "Abort" Categorization:** When capturing sessions, explicitly instrument *why* it ended if it didn't succeed. Was it a token limit? A human interrupt? A repetitive loop? This explicit metadata is gold for future analysis.
3.  **Compaction Strategy:** 60 agents writing JSONL traces will bloat a Git repo's object store rapidly. Design a `cairn compact` command early that rolls up old sessions into compressed, aggregated outcome summaries while stripping the heavy transcripts, perhaps retaining only Agent Trace schemas for sessions older than 30 days.

### Questions for the Plan Author
1.  How will you manage the inevitable Git repository bloat from thousands of full session transcripts, and what is the lifecycle of a Cairn ref?
2.  Are you planning to make the data easily consumable by the *next* agent, or is the focus strictly on human observability and analytics?
3.  How frequently does the `cairn index` need to run to ensure an agent on Machine B benefits from an agent that just finished on Machine A, given Git's fetch latency?
4.  If Entire.io pivots or breaks their plugin protocol, how tightly coupled is Cairn's internal data model to Entire's `full.jsonl` structure?
5.  Could Cairn emit not just Agent Trace, but also automatically generate and append Contextual Commits based on the agent's reasoning?