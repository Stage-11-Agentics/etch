# Standard Plan Review

### Executive Summary
The Cairn plan is an exceptionally pragmatic and architecturally sound approach to solving the invisible agent work problem at high concurrency. It correctly identifies what not to build (the hook infrastructure, which is a tarpit of maintenance) and leverages an existing $60M MIT-licensed project (Entire CLI) for the heavy lifting. The core architectural decision to use isolated per-session git refs (`refs/cairn/sessions/<session-uuid>`) is brilliant—it elegantly sidesteps the exact concurrency failures that Entire CLI suffers from under load. This plan makes smart bets, prioritizes interoperability, and is ready for execution.

### The Plan's Intent vs. Its Execution
The underlying intent is to capture comprehensive agent orchestration metadata at a high density (60-80 concurrent sessions across multiple machines) without breaking under write contention, and to ensure this data travels with the codebase natively. 

The execution aligns perfectly with this intent. By deferring analysis to query time and keeping the metadata flat, the system avoids rigid schemas that would inevitably fail as workflow patterns evolve. The write path (one git ref per session) directly addresses the contention requirement.

### Architectural Assessment
The decomposition of the problem is exactly right:
1. **Capture:** Outsourced to Entire CLI via its plugin protocol.
2. **Metadata Modeling & Storage:** Owned by Cairn via per-session git refs.
3. **Interoperability:** Handled via standard emissions (`agent-trace.json`).

The choice of per-session git refs is the defining structural bet. An alternative framing would be using sidecar dotfolders (`.cairn/<session>.json`) or Git Notes. Sidecar files risk cross-worktree or cross-machine merge conflicts, while Git Notes require explicit, non-default push/fetch configurations that developers will easily forget, breaking cross-machine visibility. Per-session refs solve the concurrency issue while riding naturally on standard Git transports (once configured).

### Is This the Move?
Yes. In the context of 20-80 concurrent agents across multiple machines, trying to build a new capture framework from scratch would be an immense waste of time. Riding Entire CLI's capture layer while building the high-density metadata layer on top is the highest-leverage path forward. The plan makes the right bets: prioritize data survival first, avoid central databases, and emit industry standards (Agent Trace) to stay relevant regardless of Entire's fate.

### Key Strengths
- **Delegation of Toil:** Leveraging `entire-agent-cairn` as a plugin means the project doesn't have to chase API changes in Claude, Codex, Gemini, etc.
- **Write Path Concurrency:** `refs/cairn/sessions/*` inherently cannot race. This is the single strongest technical decision in the plan.
- **Schema Flexibility:** Using flat JSONL and pushing structure to query time accommodates the reality that orchestration workflows are rapidly evolving. 
- **Standards Alignment:** Pledging to emit Agent Trace records provides instant interoperability with tools like Cursor, giving Cairn utility beyond its own ecosystem.

### Weaknesses and Gaps
- **Asynchronous Outcome Binding:** The plan states it captures "PR state, CI status" as observed metadata. However, these outcomes typically resolve *after* an agent session finishes. The plan does not specify if/how a session's git ref is updated asynchronously when CI goes green or a PR merges.
- **Storage Bloat:** Creating a new git ref for every session will eventually result in thousands of refs and bloated repository sizes. Git may struggle to list or fetch these efficiently over time without a compaction or archiving strategy.
- **Indexer Dependency:** Because querying requires reading N refs, performance relies entirely on the proposed indexer. If the indexer is strictly on-demand, queries on large repos will be prohibitively slow. 

### Alternatives Considered
- **Centralized Database (Cloud/SQLite):** This would break the core requirement of "travels with the code" and make cross-machine sync dependent on external network services. Cairn's git-native choice is far superior for offline/distributed environments.
- **Writing to Entire's Checkpoint Branch:** This would inherit Entire's CAS race conditions and silently drop metadata. Cairn's per-session ref strategy is a much safer alternative.

### Readiness Verdict
**Ready to execute.** The architectural foundation is solid. Clarification on asynchronous outcome binding and long-term storage GC should be addressed during Phase 1.

### Questions for the Plan Author
1. **Outcome Timing:** Since CI status and PR state are determined asynchronously *after* an agent session ends, how and when are these outcomes bound to the session record? Do we append to the existing session ref, or is there an outcome-specific ref?
2. **Storage GC:** What is the long-term plan for repository bloat? Are we planning to compact old session refs into a single archival ref after a certain time, or just let them accumulate?
3. **Indexer Execution:** Is the indexer running as a Git hook (e.g., `post-merge` / `post-fetch`), on a cron job on the always-on machine (Atlas), or only synchronously when a user runs `cairn query`?
4. **Configuration Distribution:** How will we ensure all developer machines and CI environments have the correct `push = +refs/cairn/sessions/*...` git configuration? Should this be handled by a Cairn initialization script?
5. **Upstream Drift:** If Entire CLI fundamentally changes or deprecates its external agent plugin protocol before the v1 schema is fully locked, how brittle is our dependency? Should we pin Entire to a specific commit/version?