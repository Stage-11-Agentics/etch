# Evolutionary Review: Cairn (SOLUTION.md)

**Plan ID:** forge-solution
**Model:** Claude
**Reviewer type:** Evolutionary
**Date:** 2026-05-26T21:28

---

## Executive Summary

The biggest opportunity here is not a metadata capture tool. It is a **substrate for institutional memory that compounds through use** — a system where every agent session deposits a trace that makes the next session smarter, cheaper, and faster, without any human curation step. SOLUTION.md describes the plumbing correctly (per-session refs, flat JSONL, Entire as the hook layer), but it stops precisely where the compounding begins. The plan builds a lake. The opportunity is in the river that feeds from the lake back into the work.

If Cairn evolves well, it becomes the first system where AI agents operating at scale leave behind a durable, queryable, machine-readable record of *everything they tried* — and where a fresh agent arriving in the repo can cheaply answer "what has been tried, what worked, what failed, and what does that imply for what I should do now?" That capability, once real, is more valuable than the capture layer underneath it. The capture layer is the cost of entry. The query-and-learn loop is the product.

---

## What's Really Being Built

On the surface: a metadata sidecar for agent sessions, stored in git refs.

Underneath: **an append-only knowledge substrate for multi-agent systems operating on shared codebases.** The real primitive Cairn introduces is not "capture" — it's "durable, distributed, conflict-free agent memory." Every session writes. No session needs to coordinate with any other session to write. The writes are globally visible after the next fetch. The data is structured enough to query but loose enough to survive schema evolution.

This is a CRDT-shaped design applied to agent provenance. The per-session ref architecture isn't just a concurrency fix — it's a statement about the information topology: agents produce knowledge independently, and that knowledge should merge at read time, not write time. That's a more general idea than "fix Entire's race condition." It's an architecture for how multi-agent systems should accumulate institutional memory.

Name it: **Cairn is building a conflict-free replicated provenance log.** That's the underlying capability. The session metadata is the first thing written to it. It won't be the last.

---

## How It Could Be Better

### 1. Make the indexer the product, not an afterthought

The plan says "a periodic indexer... materializes a queryable index" and leaves it at that. This is where the real differentiation lives and it gets one sentence. The indexer is where Cairn becomes more than a write-only log. Concrete improvements:

- **The indexer should produce a materialized view optimized for the next agent's cold start.** Not a generic query index — a view that answers the specific questions a fresh agent needs: "What was tried on this file/module? What prompts produced clean outcomes? What patterns are known-bad here?" Think of it as an automatically-maintained CLAUDE.md supplement derived from session history.
- **The indexer should run on Atlas (the always-on machine) as a background daemon**, not as a post-push hook or on-demand command. It watches for new refs arriving via fetch and rebuilds incrementally. This gives Cairn a "live memory" feel without requiring any agent to wait for indexing.
- **The index itself should be a ref** — `refs/cairn/index/latest` — so it propagates via the same push/fetch mechanism. An agent on Hyperion fetches and gets both raw sessions and the pre-computed index.

### 2. Define the "next agent reads this" interface now, not later

The plan explicitly says "no dashboard" and "no analysis engine." Good — those are premature. But "how does the next agent consume Cairn data?" is not premature. It's the core use case. Without a defined consumption interface, Cairn is a write-only system that nobody reads.

Concretely: the plan should specify a `cairn context` command (or a SKILL.md pattern, or an MCP server) that an agent can invoke at session start to get a structured summary of recent history relevant to its current task. The summary doesn't need to be smart — even "here are the last 10 sessions that touched files in this directory, with their prompts and outcomes" is enormously valuable versus the current state of zero.

### 3. Rethink the Entire dependency as a temporary scaffold, not a permanent substrate

The plan frames Entire as "the capture substrate" with Cairn as "the metadata layer." This is the right pragmatic choice for week 1. But the plan should explicitly state the graduation criteria — the conditions under which Cairn builds its own hook layer and drops the Entire dependency.

Why: Entire's hook layer is the only part of their codebase Cairn actually needs, and hooks are getting commoditized fast. Claude Code, Codex, Gemini CLI, and Cursor all now expose standardized hook points. A thin adapter layer that listens to each runtime's native hooks (no Entire binary needed) is probably 2-3 weeks of work and eliminates a dependency on a $60M-funded competitor's codebase. The plan should include a Phase 4 that says "Cairn-native hooks, Entire optional."

### 4. The flat metadata schema needs a few more fields from day one

The field table is solid but missing some high-signal fields that are cheap to capture and expensive to reconstruct later:

- **Parent session ID** — when a session was spawned by an orchestrator (Lattice delegator, Task tool), the parent's session UUID. This is a single optional field, not a hierarchy — but without it, reconstructing the orchestration tree from flat records requires timestamp heuristics. Capture it when the environment provides it (Lattice sets this; Task tool sets this); leave it null otherwise.
- **Exit reason** — did the session end normally, hit a token limit, error out, get killed by the user, time out? A single enum field. Enormously useful for understanding failure patterns at density.
- **Working directory / worktree path** — the plan mentions "worktree path" under git state but it should be a first-class field, not buried. At 20 agents per repo, which worktree each agent is in is essential for disambiguation.
- **Concurrent session count** — how many other sessions were active in the same repo at the time this session ran? A single integer, computed at session start. Captures the density context that makes Cairn's data unique.

### 5. Storage growth needs a strategy now, not later

The 10x.pub reviewer found 3x codebase growth per week with a *single agent*. At 20 concurrent agents, that's potentially 60x per week. The plan mentions no compaction, no retention policy, no tiered storage. This will become an emergency within the first month of real use.

At minimum, the plan should specify: (a) a retention policy for raw session refs (keep N days, archive to a packed ref, or prune after indexing), (b) a compaction strategy for the index (rolling windows, not unbounded growth), (c) a rough estimate of per-session storage at the expected field set.

---

## Mutations and Wild Ideas

### Mutation 1: Cairn as a real-time coordination signal, not just a historical record

What if Cairn refs weren't just written at session end? What if a running session periodically wrote a "heartbeat" ref — `refs/cairn/active/<session-uuid>` — containing its current state (what files it's touching, what it's trying to do, how far along it is)? A fresh agent could fetch active refs and see what's *currently happening* in the repo, not just what happened in the past. This turns Cairn from a provenance log into a lightweight coordination protocol. No message bus, no WebSocket, no external service — just git refs that represent "I'm here, I'm working on X."

This is speculative but the architecture supports it. Per-session refs with no contention means writes are cheap. The fetch interval becomes the coordination granularity. At the always-on-Atlas-as-indexer level, you could even have the indexer maintain a `refs/cairn/active/summary` that lists all currently-active sessions.

**Risk:** git push/fetch latency (seconds to minutes) makes this too slow for real-time coordination. But for the "don't start the same work someone else is doing" use case, seconds is fine.

### Mutation 2: Cairn as the substrate for self-improving orchestration

The plan says "no analysis engine." Fair. But consider: the data Cairn captures is exactly the training signal for improving orchestration patterns. If Cairn captures which orchestration pattern was used and what outcome it produced, a meta-agent could periodically analyze the corpus and suggest modifications to the orchestration itself — "fan-out-of-3 with review gates produces 40% fewer rework cycles than fan-out-of-5 without review gates on this repo."

This doesn't need to be built into Cairn. But the plan should acknowledge that this is the endgame consumer of the data — and design the schema to make it easy. Specifically: the orchestration pattern field should be structured enough to compare across sessions (not just a free-text string), and the outcome fields should be rich enough to compute quality metrics (not just "PR merged" but "PR merged after N review rounds" or "CI passed on first push").

### Mutation 3: Cairn as an open standard for agent-to-agent communication via git

What if Cairn's ref namespace and schema became a convention that other tools could write to? Not just Cairn-produced records, but any tool that wants to leave a machine-readable note for the next agent. A linter that found issues could write to `refs/cairn/signals/<uuid>`. A CI system could write test results. A human reviewer could annotate.

The per-session-ref architecture doesn't actually require sessions. It's a general-purpose append-only log in git. The "session" is just the first record type. If the schema is extensible (a `record_type` field at the top level), Cairn becomes a general-purpose agent signaling substrate that happens to start with session metadata.

### Mutation 4: Sell the density story, not the capture story

Every competitive analysis in the supporting docs says the same thing: Cairn can't out-capture Entire, can't out-fund Entire, can't out-recruit Entire. But Entire has zero multi-agent users. Zero. The density story isn't a feature of Cairn — it could be the entire positioning. "Cairn: agent memory that works at 20 agents per repo." Full stop. Don't even mention capture vs. metadata vs. workflow. Just: "You run many agents. Nothing else works at your scale. Cairn does." That's a cleaner pitch than the current "Entire does hooks, we do metadata" framing, which positions Cairn as a dependent add-on rather than a standalone product.

### Mutation 5: The "replay" use case nobody's articulated

If Cairn captures prompts, git state at session start, and the outcome, it has everything needed to *replay* a session — not in the claude-replay "watch the transcript" sense, but in the "re-run this prompt against this git state and see if the outcome is the same" sense. This is the agent equivalent of a reproducible build. Nobody in the research survey is doing this. It requires almost no new infrastructure — just a `cairn replay <session-uuid>` command that checks out the start-state, feeds the prompt to the recorded agent runtime, and compares the result.

Why this matters: it's the foundation for evaluating whether a model upgrade or prompt change actually improves outcomes. Without replay, you're A/B testing in production. With it, you can do controlled experiments. This is the MLflow pattern applied to coding agents — and the RESEARCH.md explicitly calls that out as an opportunity nobody's seized.

---

## What It Unlocks

### Immediate (ships with Phase 1-2)

1. **Cross-session deduplication of exploration.** An agent can query Cairn before exploring a dead end that a previous agent already mapped. This alone justifies the entire project at the density Cairn targets.
2. **Prompt-outcome corpus.** For the first time, someone has a structured dataset of {prompt, agent, context, outcome} tuples from real multi-agent work. This is training data for prompt optimization — not in the "fine-tune a model" sense, but in the "learn which prompt patterns produce clean outcomes" sense.
3. **Attribution at density.** When a regression ships, Cairn can answer "which sessions touched this file in the last hour, what were their prompts, and what was their orchestration context?" That's currently impossible.
4. **Machine-to-machine continuity.** Work started on Hyperion is visible on Atlas after the next fetch. No Slack, no handoff doc, no "let me check what happened on the other machine."

### Medium-term (months, with the indexer and query layer)

5. **Orchestration pattern evaluation.** With enough data, compare orchestration patterns head-to-head: does lattice-orchestrator with review gates outperform manual fan-out on this codebase? Cairn is the only system that would have the data to answer this.
6. **Cost attribution per deliverable.** Token usage tied to session, tied to outcome. "This PR cost $47 in API calls across 8 agent sessions, of which 3 were rework." Nobody has this today.
7. **Agent selection signal.** Over time, Cairn data reveals which agent runtimes perform better on which task types in which codebases. "Claude Code on refactoring tasks in this repo has a 3x higher clean-first-push rate than Codex." That's actionable.

### Long-term (the flywheel kicks in)

8. **Self-improving agent workflows.** The orchestration layer reads Cairn data and adjusts its own behavior — fewer agents when density is degrading outcomes, different agent selection based on task type, better prompts based on what worked before. This is the meta-learning loop that nobody in the space is building toward because nobody has the data substrate.

---

## Sequencing and Compounding

The current phasing is:

1. Plugin + capture
2. Query
3. Emit (Agent Trace, Contextual Commits)

Suggested reordering:

### Phase 1: Capture + Minimal Read (weeks 1-3)

Ship `entire-agent-cairn`, the per-session refs, and a bare-minimum `cairn recent` command that lists the last N sessions with their prompts and outcomes. Don't wait for a full query engine. The "list recent sessions" view is enough to validate that capture is working and to start getting value from cross-session visibility. **This is the critical change: don't ship capture without *any* read path.** A write-only system produces no feedback signal for iteration.

### Phase 2: Agent Trace emission (week 4)

Move this up. Agent Trace emission is almost free once you have the session data, and it's the strongest interop signal Cairn can send. Emitting Agent Trace from day one says "Cairn is part of the ecosystem" rather than "Cairn is another proprietary format." It also gives you a validation surface — if Agent Trace consumers can read your output, your schema is correct.

### Phase 3: Indexer + Query (weeks 5-8)

Now build the real query engine. The indexer on Atlas, the materialized views, the `cairn query` CLI with filters and aggregation. By this point you have weeks of real capture data to test against.

### Phase 4: Consumption interface (weeks 9-12)

The "next agent reads this" interface. A SKILL.md pattern, an MCP server, or a `cairn context` command that produces a structured summary for the current task. This is where the compounding loop closes — agents start benefiting from the captured data, which makes the capture more valuable, which makes more people want to capture.

### Phase 5: Native hooks (weeks 13+)

Cairn-native hook adapters for Claude Code, Codex, Gemini CLI, OpenCode. Entire becomes optional. Cairn stands alone.

**Why this order:** Each phase produces data or capabilities that make the next phase's work easier and its output more valuable. Capture without read (the current Phase 1) is the weakest possible start because it provides no feedback. Capture + minimal read gives immediate signal. Agent Trace emission early builds ecosystem credibility. The indexer benefits from having weeks of data to work with. The consumption interface benefits from a mature query engine. Native hooks are the last thing you need because Entire works fine as a scaffold.

---

## The Flywheel

There is a latent flywheel in this plan that the plan doesn't name or optimize for:

**More agents captured -> richer query results -> better-informed next agents -> fewer wasted sessions -> more trust in agentic density -> more agents deployed -> more agents captured.**

This is real. The limiting factor on agentic density today is not compute or cost — it's trust. Operators don't run 20 agents because they can't see what those agents are doing and whether they're stepping on each other. Cairn directly attacks that trust deficit. Every session captured is evidence that the system is visible and accountable. As confidence grows, operators push the density higher. Higher density produces more data. More data produces better insights. Better insights produce more confidence. The flywheel spins.

**To accelerate it:**

1. **Close the read loop early.** The flywheel doesn't start until agents consume Cairn data. Write-only capture is the pedal; read-back is the chain that connects the pedal to the wheel. Phase 1 must include a minimal read path.
2. **Make the "freshly arrived agent" experience magical.** The first time an agent queries Cairn and avoids a dead end that a predecessor already mapped, the operator sees the value viscerally. Optimize for this moment. Make it happen in week 3, not month 6.
3. **Surface the density metric.** Cairn should report "N concurrent sessions active in this repo" somewhere visible. This normalizes high density as a feature, not a risk. Operators who see "12 agents, all captured, all visible" feel in control. That feeling is what drives them to go to 20.

**A second, subtler flywheel:** Cairn's data, aggregated across repos, becomes a training corpus for orchestration improvement. Better orchestration produces better outcomes. Better outcomes justify higher density. Higher density produces more data. This is the meta-learning flywheel — it's further out but potentially more powerful because it improves the quality of work, not just the visibility.

---

## Concrete Suggestions

### 1. Ship a `cairn status` command in Phase 1

Before `cairn query`, ship `cairn status` — a zero-argument command that prints:
- How many sessions have been captured in this repo
- The last 5 sessions (timestamp, agent, prompt first line, outcome)
- How many refs exist, total storage used
- Whether the indexer has run and when

This is trivial to build (enumerate `refs/cairn/sessions/*`, read each, format). It gives immediate feedback that capture is working and doubles as a debugging tool. It's also the command you demo to show people what Cairn does.

### 2. Design a `cairn context` MCP server

An MCP server that exposes Cairn data to any MCP-capable agent (Claude Code, Cursor, etc.). One tool: `get_repo_context` takes a file path or directory and returns recent session history relevant to that area of the codebase. This is the "next agent reads this" interface in its most universal form — any MCP client gets it for free.

### 3. Add a `concurrent_session_count` field

At session start, count active `refs/cairn/active/*` refs (if heartbeats exist) or estimate from recent session timestamps. This single integer contextualizes every record — a session that ran while 19 others were active is a fundamentally different data point than a session that ran solo. It's the field that makes Cairn's data *about density*, not just *captured at density*.

### 4. Publish a Cairn ref spec

Write a one-page document that specifies the ref namespace (`refs/cairn/sessions/*`, `refs/cairn/index/*`, future `refs/cairn/active/*`), the JSONL schema (field names, types, required vs. optional), and the transport expectations (push/fetch refspecs). This is how Cairn becomes an interoperable standard rather than a tool — other tools can write Cairn-compatible refs without depending on the Cairn binary. It's the Agent Trace move applied to provenance storage.

### 5. Test the Entire plugin protocol before committing to it

The plan assumes the external agent protocol gives Cairn everything it needs. Validate this with a weekend spike: write a minimal `entire-agent-cairn` that logs every event it receives. Run it for 48 hours at real density. Confirm: Does it get session start/end? Does it get prompts? Does it get tool-use events? Does it get transcript refs? Does it get enough to populate every field in the metadata schema? The ENTIRE_EVAL.md lists what the protocol *should* provide, but "should" and "does" diverge constantly in practice.

### 6. Plan for the Entire CAS PR to be declined

The plan says "open a PR for the metadata-branch CAS fix." This is good citizenship. But Entire is a $60M-funded company with their own roadmap priorities, and a PR from a competitor's project fixing a concurrency bug they may not consider important (they have no multi-agent users) could easily be deprioritized or declined. The plan should not have any critical-path dependency on this PR being accepted. Cairn's per-session refs sidestep the issue entirely — which is the right call — but the plan should state this explicitly: "We're offering the fix. If they take it, everyone wins. If they don't, Cairn's architecture doesn't need it."

### 7. Consider a "cairn init" that sets up gc.auto=0

The ENTIRE_EVAL.md notes that git auto-GC can corrupt worktree indexes when go-git creates loose objects. At Cairn's density, you're creating many loose objects (one commit per session ref). `cairn init` should set `gc.auto 0` on the repo and provide a `cairn gc` command that runs a Cairn-safe garbage collection pass. This is a small operational detail that prevents data loss — exactly the kind of thing that builds trust.

---

## Questions for the Plan Author

1. **What is the first read-back use case?** The plan defers all consumption ("no analysis engine, no dashboard, no query interface" in Phase 1). What is the *minimum viable read* that makes capture worth doing? When does the first agent actually benefit from Cairn's data, and what does that interaction look like?

2. **What's the storage budget?** At 20 agents x N sessions/day x the planned field set, how many MB/day of Cairn refs accumulate? What's the repo size tolerance before it becomes a problem? Is there a retention/compaction policy, or do refs grow indefinitely?

3. **Is the Entire dependency a temporary scaffold or a permanent architecture?** The plan says "if Entire disappears, Cairn loses the hook layer and has to rebuild or replace it." Under what conditions do you proactively build the replacement rather than waiting for Entire to disappear? Is there a Phase 4 that graduates Cairn to standalone?

4. **Should Cairn capture human sessions too?** The plan is agent-centric. But humans also write code, and the "what happened in this repo" question includes human work. If a human commits without an agent session, is that a gap in Cairn's record? Should Cairn have a lightweight git-hook-only mode that captures git state changes even when no agent is running?

5. **What about private/sensitive prompts?** At Cairn's density, some prompts will contain sensitive information (API keys pasted into prompts, proprietary business logic, personal instructions). Cairn refs push to the remote. Is there a redaction strategy? The plan mentions consuming from `entire/checkpoints/v1` (which runs through Entire's redaction pipeline) for transcripts, but Cairn's own metadata record captures prompts directly from hooks — does it run its own redaction?

6. **Is the per-session ref model the right granularity?** At 60-80 sessions/day, that's 60-80 new refs per day, 2000+ per month. Git handles many refs but some operations (fetch, GC, ref advertisement) scale linearly with ref count. At what point does this become a performance problem, and what's the mitigation? Packing refs? Periodic archival to a combined ref?

7. **Who is the first user besides Atin?** The plan is designed for Atin's exact density and topology. Is there a second user in mind? If Cairn is only useful at 20+ agents per repo, the addressable market is vanishingly small today. If it's useful at 2-3 agents (which is where most teams are), the plan should validate that the architecture doesn't over-engineer for density at the cost of simplicity for lighter users.

8. **What's the relationship between Cairn and Lattice?** Lattice is the orchestrator. Cairn captures what the orchestrator did. Should Cairn be a Lattice plugin? Should Lattice write Cairn refs directly? Is there a world where Cairn *is* a Lattice feature rather than a standalone tool? The organizational boundary here matters for adoption and maintenance.

9. **Should the indexer produce a CLAUDE.md-shaped output?** Rather than a generic query index, what if the indexer produced a machine-readable summary in a format that agents already know how to consume — like an auto-generated section in CLAUDE.md or a `.cairn/context.md` that agents read at session start? This would close the read-back loop without requiring any new consumption interface.

10. **What would make you abandon the Entire dependency in week 2?** You're about to stake Cairn's capture path on a third party's binary. What are the deal-breakers? If the plugin protocol doesn't deliver prompt text? If hooks drop silently at density? If Entire ships a breaking change to the external agent protocol? Having the exit criteria written down before you start prevents sunk-cost attachment.

11. **Is there a competitive moat, or is this a feature of every agent runtime in 12 months?** Claude Code already has `/rewind` and session history. Cursor has parallel agents with built-in coordination. If every runtime builds its own provenance layer, what does Cairn offer that's distinct? The answer is probably "cross-runtime, cross-machine" — but the plan should state this explicitly as the defensible position and ensure every design decision reinforces it rather than optimizing for single-runtime capture.
