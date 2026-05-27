# Evolutionary Review Synthesis: Cairn SOLUTION.md

**Reviewers:** Claude, Codex, Gemini
**Date:** 2026-05-26
**Review type:** Evolutionary / Exploratory

---

## Executive Summary

All three models converge on the same core thesis: Cairn is not a metadata capture tool -- it is a **conflict-free, git-native provenance log** that, if evolved correctly, becomes the institutional memory and coordination substrate for multi-agent development at scale. The plan correctly identifies per-session refs as the foundational primitive but systematically underinvests in the read/consume side. Every reviewer independently flagged the same critical gap: Phase 1 ships capture with no read path, which means the flywheel cannot start spinning. The unanimous prescription is to bundle a minimal query or context-injection interface into Phase 1 so that the first agent to consume Cairn data validates the entire architecture.

Beyond that consensus, the reviews surface a rich set of evolution paths -- from real-time coordination via heartbeat refs, to orchestration A/B testing, to semantic search over past intents, to a "pre-crime CI" that interrupts agents heading toward known failure patterns. The density story (20+ concurrent agents, zero lost records) is unanimously identified as Cairn's sharpest differentiator and the one the plan should lead with.

---

## 1. Consensus Direction

Evolution paths identified by two or more models:

1. **Close the read loop in Phase 1.** All three insist that capture without consumption is a write-only system that produces no feedback signal. Ship a minimal read interface -- `cairn recent`, `cairn context`, or an MCP server -- alongside the first capture code. (Claude, Codex, Gemini)

2. **Cairn is an event-sourced system; name it.** The per-session ref design is an append-only, immutable event log with derived indexes. All three models recommend making this architecture explicit: immutable session facts, disposable derived views, late-arriving observations as separate records. (Claude, Codex)

3. **The Entire dependency must be treated as a swappable adapter, not the architecture.** Define an internal capture interface so Entire is one adapter behind it. State graduation criteria for building native hooks. Plan for the Entire CAS PR being declined. (Claude, Codex, Gemini)

4. **Storage growth is a P0 concern, not a future problem.** At 20-60 agents per day, raw refs accumulate fast. All three call for a compaction/retention strategy in the design phase, not after the first emergency. (Claude, Codex, Gemini)

5. **Privacy and redaction need a Phase 1 boundary.** Cairn records prompts, tool events, machine identity, and operator info. Pushing these to a shared remote without a defined sensitivity classification is a trust liability. (Claude, Codex)

6. **Density is the positioning, not capture.** Entire has $60M and zero multi-agent users. Cairn's pitch should be "agent memory that works at 20 agents per repo" -- not "we do metadata while Entire does hooks." (Claude, Gemini)

7. **Orchestration analytics is the highest-value long-term loop.** Binding workflows to PR/CI outcomes lets Cairn answer "which orchestration pattern actually works?" -- moving from craft to measurable operational practice. (Claude, Codex, Gemini)

8. **Agent Trace emission should be a projection from the canonical record, not the schema foundation.** Emit it once the internal schema stabilizes, to avoid warping Cairn's design toward file-level attribution when its unique value is workflow/session provenance. (Claude, Codex)

---

## 2. Best Concrete Suggestions

Ranked by actionability and impact:

1. **Ship `cairn status` / `cairn recent` in Phase 1.** Zero-argument command that lists the last N sessions with timestamp, agent, prompt summary, and outcome. Proves capture works, doubles as a demo. (Claude)

2. **Build a `cairn-mcp` server.** Expose Cairn data to any MCP-capable runtime (Claude Code, Cursor, etc.) via tools like `get_repo_context(path)`, `search_past_dead_ends(file)`, `get_recent_intent(module)`. This is the most universal consumption interface. (Claude, Gemini)

3. **Define the canonical event envelope now.** Versioned schema (`cairn.session.v1`), stable outer fields, flexible inner fields. Keep it boring. Make early data not become junk. (Codex)

4. **Add missing high-signal fields to the base schema:**
   - `parent_session_id` -- links orchestrator to delegator sessions (Claude)
   - `exit_reason` enum -- normal, token limit, error, killed, timeout (Claude, Gemini)
   - `concurrent_session_count` -- integer at session start; the field that makes data *about* density (Claude)
   - `working_directory` / `worktree_path` as a first-class field (Claude)

5. **Make late-arriving facts first-class.** Use `refs/cairn/observations/<uuid>` for PR merge, CI status, revert events. Do not force mutable outcome truth into immutable session-end records. (Codex)

6. **Add a `cairn doctor sync` command.** Verifies push/fetch refspecs, remote support, missing refs, divergent indexes, unpushed local sessions. (Codex)

7. **Add loud failure semantics.** Every capture attempt ends in one of: captured, partially captured, redacted, skipped, failed. Silent absence destroys trust. (Codex)

8. **Set `gc.auto=0` via `cairn init`.** Prevents git auto-GC from corrupting loose objects created by high-frequency ref writes. Provide `cairn gc` for safe garbage collection. (Claude)

9. **Publish a Cairn ref spec.** One-page document specifying namespace, schema, transport expectations. Makes Cairn an interoperable standard, not just a tool. (Claude)

10. **Run a 48-hour Entire protocol validation spike.** Write a minimal plugin that logs every event. Confirm the protocol delivers prompts, session lifecycle, tool events, transcript refs -- everything the schema needs. Validate before committing. (Claude)

---

## 3. Wildest Mutations

The most creative and ambitious ideas, ordered from nearest to furthest horizon:

1. **Heartbeat refs for real-time coordination.** Running sessions periodically write `refs/cairn/active/<uuid>` with current state (files being touched, task description, progress). Fresh agents fetch active refs to see what is *currently happening* -- not just history. Git push/fetch latency (seconds) is fast enough for "don't duplicate work" coordination. No message bus required. (Claude)

2. **"Ghost" refactoring agent.** A background daemon watches `refs/cairn/sessions/*`, mines recent sessions for repeated friction points and CI failures, and autonomously generates new SKILL.md rules or constraint prompts. The repo actively heals its own context. (Gemini)

3. **Pre-crime CI.** If an agent starts executing a pattern of tool calls that Cairn's historical metadata correlates with future CI failure, interrupt the agent mid-session with a warning: "This orchestration pattern failed 4 times yesterday. Re-evaluate." (Gemini)

4. **ROI-driven orchestration (bounty market).** An orchestrator queries Cairn: "Which model has the highest success rate and lowest cost for modifying the auth module?" Workloads dynamically shift to the most cost-efficient model based on historical success rates. (Gemini)

5. **Session replay.** `cairn replay <session-uuid>` checks out the start-state, feeds the recorded prompt to the same runtime, and compares results. The agent equivalent of a reproducible build. Foundation for evaluating model upgrades and prompt changes without A/B testing in production. (Claude)

6. **Git as a vector memory bank.** `cairn index` compiles a local, git-tracked vector database of past session intents. New agents semantic-search for relevant prior reasoning when starting a task. (Gemini)

7. **Resurrection of abandoned value.** An agent explores a valid path but abandons it (token limits, minor bug). Cairn lets you fork that agent's reasoning from a specific point and hand it to a more powerful model. (Gemini)

8. **Incident forensics.** `cairn incident <bad-sha>` returns the sessions, prompts, tools, models, files, and CI transitions most likely responsible for a regression. Flight recorder for software development. (Codex)

9. **Agent curriculum / house style.** Derive lessons from the corpus: prompts that worked, prompts that caused rework, files that need human review, models that struggle in certain directories. Auto-generate `AGENTS.learned.md` or `cairn memory export`. (Codex)

10. **Trust layer for AI-authored code.** Attach the full provenance chain (prompt, tool use, model, machine, operator, review workflow, CI result) as a supply-chain artifact on PRs and releases. (Codex)

---

## 4. Flywheel Opportunities

### Primary Flywheel (all three models)

```
More agents captured
  -> Richer query results
    -> Better-informed next agents
      -> Fewer wasted sessions / avoided dead ends
        -> More trust in agentic density
          -> More agents deployed
            -> More agents captured
```

**Key insight (Claude):** The limiting factor on density today is trust, not compute or cost. Cairn attacks the trust deficit. Every captured session is evidence of visibility and accountability. The flywheel does not start until agents *consume* data -- capture is the pedal, read-back is the chain.

### Standards Adoption Flywheel (Codex)

```
Cairn emits Agent Trace + Contextual Commits
  -> External tools can read Cairn artifacts
    -> Users get value outside the Cairn CLI
      -> Cairn becomes safer to adopt (data not trapped)
        -> More sessions captured
```

### Orchestration Learning Flywheel (all three models)

```
Lattice runs produce Cairn records
  -> Cairn correlates workflow variants with outcomes
    -> Operators adjust dispatch patterns
      -> Better dispatch produces cleaner outcomes
        -> Those outcomes train future orchestration choices
```

**Acceleration levers:**
- Close the read loop in Phase 1 (starts the primary flywheel weeks earlier)
- Make the "freshly arrived agent avoids a dead end" moment happen in week 3, not month 6 (Claude)
- Surface the density metric ("12 agents, all captured, all visible") to normalize high concurrency as a feature (Claude)
- Make the read barrier as low as the write barrier (Gemini)

---

## 5. Strategic Questions for the Plan Author

### Identity and Positioning

1. What is Cairn primarily -- a capture tool, a fresh-agent memory layer, or an orchestration analytics platform? The Phase 1 success metric changes depending on the answer. (Codex, Gemini)

2. Is the long-term differentiator "flat metadata at density" or "workflow-to-outcome learning"? The plan says the former; all three reviewers see the bigger opportunity in the latter. (Codex)

3. Is Cairn a standalone tool, a Lattice plugin, or potentially a Lattice feature? The organizational boundary matters for adoption and maintenance. (Claude)

### Architecture

4. Should session refs be immutable forever? If so, how should late-arriving outcomes (PR merge, CI status, revert) be represented -- separate observation refs, index-only fields, or something else? (Codex)

5. Are transcripts stored in Cairn records, copied from Entire, compressed, referenced by checkpoint ID, or omitted by default? This is the dominant storage cost driver. (Codex)

6. Should `refs/cairn/sessions/*` auto-push on normal `git push`, or require explicit `cairn sync` until trust and redaction are proven? (Codex)

7. At 60-80 sessions/day producing 2000+ refs/month, at what point does ref count become a git performance problem? What is the mitigation -- packing, periodic archival, compaction? (Claude)

### Privacy and Trust

8. What is the minimum record that is safe to push to a shared remote by default? Which fields are always captured vs. opt-in vs. hashed vs. local-only? (Codex)

9. How much machine/operator identity should be raw versus hashed or configurable? (Codex)

10. Does Cairn run its own prompt redaction, or does it rely entirely on Entire's pipeline? Cairn captures prompts directly from hooks -- some will contain API keys or proprietary logic. (Claude)

### Dependencies and Risk

11. Is the Entire dependency a temporary scaffold or a permanent architecture? Under what conditions do you proactively build native hooks rather than waiting for Entire to disappear or break? (Claude)

12. What are the deal-breakers for the Entire dependency? If the plugin protocol does not deliver prompt text, or hooks drop silently at density, or a breaking change ships -- at what point do you pivot, and is the exit plan written down before you start? (Claude)

13. If Entire pivots or breaks their protocol, how tightly coupled is Cairn's internal data model to Entire's `full.jsonl` structure? (Gemini)

### Users and Market

14. Who is the first user besides Atin? If Cairn is only useful at 20+ agents, the addressable market is tiny today. Does the architecture work (and justify itself) at 2-3 concurrent agents? (Claude)

15. Should Cairn capture human sessions too? A human committing without an agent session is a gap in the "what happened in this repo" record. Should there be a lightweight git-hook-only mode? (Claude)

16. Is there a competitive moat, or is session provenance a feature of every agent runtime in 12 months? If every runtime builds its own layer, Cairn's defensible position is "cross-runtime, cross-machine" -- does every design decision reinforce that? (Claude)

### Consumption and Indexing

17. What is the first query that should save real time for an agent entering an active repo? Define the acceptance test: a fresh agent asks what happened recently on this branch and gets a useful answer in under a few seconds. (Codex)

18. Should the indexer produce a CLAUDE.md-shaped output (auto-generated `.cairn/context.md`) rather than a generic query index? This closes the read loop without requiring a new consumption interface. (Claude, Codex)

19. How frequently must `cairn index` run to ensure an agent on Machine B benefits from an agent that just finished on Machine A, given git fetch latency? (Gemini)

20. Should workflow identity be a loose property bag or a versioned object with its own schema? Structured enough to compare across sessions, or free-form? (Codex)
