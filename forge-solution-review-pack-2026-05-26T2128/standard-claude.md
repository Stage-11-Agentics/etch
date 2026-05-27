# Standard Plan Review: Cairn (SOLUTION.md)

**Plan ID:** forge-solution
**Model:** Claude
**Date:** 2026-05-26

---

## Executive Summary

This is a sharp, disciplined plan that knows what it is and -- more importantly -- what it is not. The decision to stand on Entire CLI as a capture substrate rather than rebuilding hook infrastructure is strategically correct and reflects mature engineering judgment. The per-session ref architecture (`refs/cairn/sessions/<uuid>`) is the single strongest technical choice in the plan: it eliminates the concurrency problem by design rather than by mechanism, which is the right level to solve it. The flat metadata schema and the explicit "no analysis engine" boundary are well-matched to the actual differentiator articulated in the project memory -- capture at density, not modeling.

The single most important thing about this plan: **it correctly identifies that Cairn's value is not in the hooks (commodity) or the analysis (premature) but in the structural choice of how metadata is written and transported at concurrency.** The per-session ref is that structural choice, and it's sound.

My main concern is that the plan underspecifies the Entire CLI plugin integration -- the thing it depends on most -- and leaves several practical questions about the per-session ref lifecycle unanswered (garbage collection, ref proliferation, indexer design). These are tractable problems, but they're the kind that bite during implementation if not addressed upfront.

**Verdict: Ready to execute with targeted clarifications.** The architecture is right. The scope is right. A handful of open questions need answers before Phase 1 is complete, but none of them challenge the fundamental design.

---

## The Plan's Intent vs. Its Execution

**Intent:** Capture flat, self-contained metadata about every agent session in a repository. Store it in git so it travels with the code. Handle 60+ concurrent sessions across multiple machines without contention. Don't build analysis, hierarchy, or dashboards.

**Execution alignment:** Strong. The plan stays remarkably disciplined about what it builds and what it defers. The "What Cairn does NOT build" section is as valuable as the "What Cairn adds" section -- plans that know their negative space tend to ship well.

One place where intent drifts from execution: the plan claims "no hierarchical workflow model" and "records are flat" as core principles, but Phase 3 includes "Contextual Commits action lines appended to commit messages when the session produced explicit decisions/constraints/learnings." That requires Cairn to parse session content for semantic meaning (what counts as a "decision" vs. normal conversation?), which is a foot in the door toward the analysis layer the plan explicitly excludes. This isn't fatal -- it's a thin serializer if Agent Trace/Contextual Commits conventions are well-defined -- but it should be acknowledged as the one place where capture bleeds into interpretation.

A subtler drift: the metadata record includes "Orchestration pattern" sourced from "Environment: Lattice ticket ID, orchestrator type, dispatch method, or 'manual'." This is clean if it's reading environment variables that the orchestration layer already sets. But if Cairn has to *infer* the orchestration pattern (e.g., detecting whether this session was spawned by a Lattice delegator vs. manually launched), it's doing analysis. The plan should clarify: is this field populated by the environment or derived by Cairn?

---

## Architectural Assessment

### The per-session ref: right answer, well-reasoned

The move from "shared mutable branch" (Entire's approach) to "one immutable ref per session" is the correct architectural insight. It transforms a coordination problem (N writers racing on one ref) into an embarrassingly parallel write problem (N writers each writing their own ref). This is the same pattern that makes append-only logs, event sourcing, and CRDTs work well under concurrency -- avoid shared mutable state entirely.

The tradeoff is read-time complexity: scanning N refs to answer a query is O(N) without an index, versus O(1) on a single branch. The plan acknowledges this with the indexer, but the indexer is Phase 2. During Phase 1, every query is a linear scan of all session refs. At 60 sessions/day across two machines, that's ~1,800 refs after a month. Git can handle this, but the plan should state the expected ref growth rate and the point at which the indexer becomes necessary rather than nice-to-have.

### Flat records: right for now, watch the edges

The "no foreign keys, no parent references" principle is correct for a capture layer. But the plan includes "Lattice ticket ID" as a field -- which *is* a foreign key to an external system. This is fine pragmatically (it's an opaque string, not a join), but the plan's language about "no foreign keys" is slightly stronger than what it actually does. Minor, but worth being precise about: the principle is "no *internal* foreign keys" (no session-to-session references), not "no references to external systems."

### Entire CLI dependency: correctly bounded

The dependency on Entire is well-structured: Cairn uses Entire for hooks and transcript capture, owns its own storage and transport, and has a clean fork path if Entire becomes uncooperative. The external agent plugin protocol (`entire-agent-cairn` binary) is a good integration surface -- it's the Unix philosophy of composable tools.

One structural risk the plan doesn't address: **what happens when Entire updates its plugin protocol?** The plan treats the protocol as stable, but Entire is pre-1.0 (v0.6.2) with active v2-to-v1 storage consolidation. If Entire ships a breaking change to the external agent protocol, Cairn's capture path breaks. The plan should specify: does Cairn pin to a specific Entire version? Does it have a compatibility layer? Or does it accept the coupling and track Entire's releases?

### Alternative framing: would I arrive here from scratch?

If I were designing a system to capture agent metadata at this density, I would arrive at something very similar. The key decisions I'd make the same way:

1. **Don't rebuild hooks.** Correct. The hook layer is commodity work that Entire, Claude Code's native hooks, and other runtimes are converging on. Building hooks is building a depreciating asset.
2. **Per-session refs over shared branch.** Correct. This is the only architecture that's provably race-free at N concurrent writers.
3. **Git-native transport.** Correct. The cross-machine sync requirement makes git the obvious transport -- it's already there, already authenticated, already understood.
4. **Flat records.** Correct for Phase 1. Structure should emerge from queries over real data, not from a schema designed before you have data.

The one thing I'd do differently: I'd consider **git notes** as an alternative to custom refs. Git notes attach metadata to existing objects (commits, trees) and travel with push/fetch when configured. The advantage is that a session's metadata is directly attached to the commit(s) it produced, rather than floating in a separate ref namespace that requires cross-referencing. The disadvantage is that notes don't handle sessions that produce *no* commits (aborted sessions, exploration, review-only sessions). The plan's ref-based approach handles all session types uniformly, which is probably the better default. But the tradeoff should be acknowledged.

---

## Is This the Move?

Yes. Here's why, evaluated against common failure patterns in projects like this:

**"Build everything before shipping anything" -- avoided.** The three-phase structure (capture, query, emit) is correctly ordered by dependency and value. Phase 1 alone is useful. This is the right sequencing.

**"Over-specify the schema" -- avoided.** Flat JSONL with self-contained records is the right starting point. Premature schema design is the #1 killer of metadata capture systems -- you lock in a structure before you understand the data.

**"Build the analysis layer into the capture layer" -- mostly avoided.** The plan stays disciplined here, with the minor exceptions noted above (Contextual Commits parsing, orchestration pattern inference).

**"Underestimate the boring parts" -- partially present.** The plan focuses on the elegant architectural choices (per-session refs, flat records) but underspecifies the operational concerns that will dominate implementation time:
- How does `cairn query` handle a repo with 10,000 session refs?
- What's the ref pruning/archival strategy?
- How large do individual session records get, and what's the impact on `git fetch` time?
- How does Cairn handle sessions that crash (no clean SessionEnd event)?
- What happens when `git push` fails mid-push with 60 new refs?

These aren't reasons to change the architecture -- they're reasons to add an "Operational Concerns" section before starting Phase 1.

**"Depend on an external project's goodwill" -- mitigated.** The MIT license, the fork path, and the clean separation of concerns make this a well-bounded dependency. The plan is realistic about what happens if Entire pivots. Good.

---

## Key Strengths

**1. The concurrency architecture is correct by construction, not by mechanism.**
Per-session refs can't race because they don't share state. This is fundamentally different from (and superior to) "add a lock/CAS/retry to a shared ref." Mechanism-based concurrency solutions have failure modes (lock contention, retry exhaustion, silent data loss on CAS failure). Construction-based solutions don't have those failure modes because they eliminate the shared state that causes them. This is the plan's strongest technical choice.

**2. The dependency on Entire is well-structured.**
The plan draws a clean line: Entire owns hooks and transcripts, Cairn owns metadata and transport. The plugin protocol is the interface surface. If Entire changes, Cairn's core (refs, records, transport) is unaffected. This is good software architecture -- dependencies should be narrow and replaceable.

**3. The "What Cairn does NOT build" section exists and is specific.**
Plans that enumerate what they *won't* build are more likely to ship than plans that only enumerate what they will. This section prevents scope creep by making the boundaries explicit. Particularly good: "No analysis engine" and "No dashboard" -- both are attractive nuisances that would consume months without advancing the core mission.

**4. Agent Trace emission is a strategic win with minimal engineering cost.**
Emitting Agent Trace gives Cairn free interop with the Cursor/Cognition/Anthropic ecosystem. It positions Cairn on the winning side of a standards line that Entire has chosen to sit outside. This costs a serializer and gains an ecosystem. Excellent risk/reward.

**5. The plan is honest about Entire's strengths.**
"Rebuilding that hook infrastructure would be months of work for a problem already solved" -- this kind of candid assessment of build-vs-reuse tradeoffs is rare and valuable. The plan doesn't pretend Entire is bad; it identifies what Entire does well and builds above it.

---

## Weaknesses and Gaps

**1. The Entire plugin integration is underspecified.**
The plan says Cairn installs as an external agent plugin via `entire-agent-cairn` binary on $PATH, but doesn't specify:
- Which of the ~15 protocol subcommands does Cairn actually need to implement?
- What data flows from Entire to Cairn at each lifecycle event? The metadata record table lists sources like "Agent hook (SessionStart / UserPromptSubmit)" but doesn't map these to specific Entire plugin protocol events.
- Does the plugin protocol give Cairn access to all the fields in the metadata record? Specifically: token usage, cost, tool use events -- are these available through the plugin protocol, or would Cairn need to read Entire's internal data structures?
- What's the contract for error handling? If `entire-agent-cairn` crashes, does Entire retry, skip, or also crash?

This is the most critical integration surface in the system. It needs more detail before Phase 1 implementation begins.

**2. Ref lifecycle management is absent.**
The plan creates a ref per session but never discusses:
- **Pruning:** When (if ever) are old refs deleted? A repo producing 60 sessions/day accumulates ~22,000 refs/year. Git can handle this, but `git fetch` with 22,000 custom refs on every clone is non-trivial.
- **Archival:** Should old refs be compacted into a single indexed branch after some time horizon?
- **Partial fetch:** Can a new clone fetch only recent session refs (e.g., last 30 days) rather than the full history?
- **Forge/remote compatibility:** Do GitHub/GitLab/Forgejo allow arbitrary custom refs to be pushed? GitHub has historically been restrictive about non-standard ref namespaces. This is a showstopper if the remote rejects `refs/cairn/*`.

**3. Session crash handling is unaddressed.**
The plan assumes a clean session lifecycle: start, capture events, commit record on session end. But agent sessions crash, get killed, lose network, or hang indefinitely. What happens to:
- The in-progress JSONL buffer?
- The session ref (is it committed partially, or not at all)?
- The metadata record (is a partial record better or worse than no record)?

At 60 concurrent sessions, crashes aren't edge cases -- they're Tuesday.

**4. Storage growth is mentioned but not quantified.**
The ENTIRE_EVAL.md notes that Entire produces ~3x codebase size per week per the 10x.pub reviewer. Cairn adds metadata on top of that. At 60 sessions/day, each producing a JSONL record + potentially cross-referencing Entire's transcript, what's the expected storage growth per month? The plan should state a target storage budget and a strategy for staying within it.

**5. The indexer is critical but Phase 2.**
The plan positions the indexer as Phase 2, but any query over session data depends on it at even moderate volume. In Phase 1, `cairn query` would have to scan all session refs -- which means reading git objects for every session ever captured. This is fine for a prototype with dozens of sessions but unusable at Cairn's target density after the first week. Consider whether a lightweight local index (SQLite, even just a flat file listing session UUIDs + timestamps) should be Phase 1 rather than Phase 2.

**6. The metadata record has no schema versioning.**
The JSONL records are self-contained, but the plan doesn't specify how the schema evolves. When Cairn adds a new field in v2, old records won't have it. When Cairn removes or renames a field, old records become partially incompatible. A `schema_version` field in each record (or a version marker in the ref path) would make backward compatibility manageable. This is cheap to add now and expensive to retrofit later.

---

## Alternatives Considered

### Per-session refs vs. git notes

**Plan's choice:** Custom refs under `refs/cairn/sessions/<uuid>`.
**Alternative:** Git notes attached to the commit(s) produced by the session.

Git notes are purpose-built for attaching metadata to git objects. They travel with push/fetch (when configured), they're inspectable with standard git tools (`git notes show`), and they're directly associated with the objects they describe.

**Why the plan's choice is better:** Notes attach to *existing objects*. Sessions that produce no commits (aborted, exploratory, review-only) have nothing to attach to. Custom refs handle all session types uniformly. Notes also have their own merge conflict story when multiple machines add notes to the same commit -- less severe than the shared-branch problem but not zero. The plan's approach is simpler and more general.

### Entire plugin vs. independent hook layer

**Plan's choice:** Depend on Entire for hooks, consume via plugin protocol.
**Alternative:** Build Cairn's own hook layer (Claude Code hooks, Codex hooks, Gemini hooks, etc.) independently.

**Why the plan's choice is better:** Hook integration across 8+ runtimes is commodity work with a long tail of edge cases (agent updates, protocol changes, platform quirks). Entire has 4,400+ commits of this work. Rebuilding it would consume months for zero differentiation. The plugin protocol provides a clean interface. The MIT license provides a fork path. The risk/reward strongly favors reuse.

### Flat JSONL vs. structured schema (protobuf, JSON Schema, etc.)

**Plan's choice:** Self-contained JSONL records with implicit schema.
**Alternative:** A formal schema (JSON Schema, protobuf) with explicit versioning and validation.

**Why the plan's choice is probably right for Phase 1:** The schema will evolve rapidly as Cairn encounters real data at density. A formal schema at this stage is premature optimization -- it constrains iteration speed without providing much value when you're the only consumer. However, as noted in Weaknesses, a `schema_version` field should be added from day one so that future consumers can distinguish record generations without parsing and guessing.

### Single binary vs. Entire plugin + standalone daemon

**Plan's choice:** `entire-agent-cairn` binary that runs within Entire's plugin lifecycle.
**Alternative:** A standalone Cairn daemon that watches for agent activity independently (via filesystem watchers, git hooks, or agent-specific hooks) and doesn't depend on Entire at all.

**Why the plan's choice is better for now:** The daemon approach is more complex (process management, crash recovery, resource usage) and rebuilds what Entire already provides. But it's worth noting that the daemon approach is the natural fallback if the Entire plugin protocol proves insufficient or if Entire's roadmap diverges. The plan's fork path implicitly includes this, but it's not spelled out.

---

## Readiness Verdict

**Ready to execute with targeted clarifications.**

The architecture is sound. The scope is disciplined. The dependency structure is well-reasoned. The plan makes the right bets on the right problems.

Before Phase 1 implementation begins, the following need to be resolved:

1. **Validate that `refs/cairn/*` can be pushed to the target git remotes** (GitHub, Forgejo, whatever Cairn targets). This is a potential showstopper.
2. **Specify the Entire plugin protocol mapping** -- which protocol events feed which metadata fields, and whether all required data is accessible through the protocol.
3. **Add a `schema_version` field to the record format.** Trivial now, painful later.
4. **Decide on Phase 1 crash handling** -- what happens to in-flight sessions that die without a clean end event.
5. **Quantify expected ref growth** and state the threshold at which the Phase 2 indexer becomes required.

None of these challenge the fundamental architecture. They're implementation details that the plan should address before coding begins, not reasons to rethink the design.

---

## Questions for the Plan Author

1. **Remote ref compatibility:** Have you verified that GitHub, Forgejo, and/or your target git remotes accept pushes to `refs/cairn/sessions/*`? GitHub has historically restricted custom ref namespaces. If they reject it, the entire transport layer needs redesign. This should be validated before any code is written.

2. **Entire plugin protocol coverage:** Does the external agent plugin protocol actually expose all the data Cairn needs? Specifically: token usage/cost, tool use events with payloads, and the full prompt text. The ENTIRE_EVAL.md mentions the protocol supports ~15 subcommands including `read-transcript`, but token/cost data isn't explicitly confirmed as available through the plugin interface vs. being internal to Entire.

3. **Orchestration pattern field -- environment or inference?** The metadata record includes "Orchestration pattern" sourced from "Environment: Lattice ticket ID, orchestrator type, dispatch method, or 'manual'." Is this reading environment variables that Lattice/c11 already set, or does Cairn need to infer the orchestration pattern? If the former, which env vars? If the latter, this is analysis, not capture.

4. **Crash recovery:** At 60 concurrent sessions, agent crashes are routine. When a session dies without a clean SessionEnd event, what should Cairn do? Options: (a) commit a partial record with a `status: crashed` field, (b) discard the session entirely, (c) keep the buffer on disk for later recovery. Each has tradeoffs. What's the preference?

5. **Ref pruning strategy:** 60 sessions/day = ~22,000 refs/year. What's the plan for ref lifecycle? Options: (a) keep everything forever (simple, growing fetch cost), (b) compact old refs into an indexed branch after N days (adds complexity), (c) let the indexer handle archival (couples read-path to write-path lifecycle). Is there a target retention policy?

6. **Indexer location and trigger:** The plan mentions the indexer running "on the always-on machine, or as a post-push hook, or on-demand." These have very different operational profiles. Is Atlas the expected indexer host? Does the index live in a git ref too (making it fetchable by other machines), or is it local-only?

7. **Schema versioning:** When Cairn adds a field to the metadata record in three months, how do consumers distinguish v1 records from v2 records? A `schema_version` field is trivial to add now. Is there a reason to defer it?

8. **Entire version pinning:** Entire is pre-1.0 (v0.6.2) and actively consolidating their storage format (v2 to v1). If Entire ships a breaking change to the external agent plugin protocol, how does Cairn respond? Pin to a known-good version? Track head? Maintain a compatibility shim?

9. **Phase 1 completeness criteria:** What does "Phase 1 done" look like concretely? Is it "20 concurrent Claude Code sessions on Hyperion each produce a session ref that fetches cleanly on Atlas"? Defining the smoke test before building helps keep scope honest.

10. **Contextual Commits scope:** Phase 3 includes appending Contextual Commits action lines "when the session produced explicit decisions/constraints/learnings." How does Cairn determine what constitutes a "decision" or "learning"? If this requires parsing the transcript for semantic content, it's an analysis task -- which the plan explicitly excludes from Cairn's scope. Is this a thin pass-through (the agent explicitly tags decisions) or actual content analysis?

11. **Transcript cross-referencing:** The metadata record cross-references Entire's `full.jsonl` by checkpoint ID. What happens when Entire's checkpoint is missing (e.g., Entire crashed, Entire wasn't installed, or the checkpoint was garbage-collected)? Is the Cairn record still useful without the transcript, or is it an orphan?

12. **Multi-repo sessions:** Do any of your orchestration patterns produce agent sessions that touch multiple repos? If so, which repo gets the session ref? Or does Cairn produce one ref per repo touched?

13. **Storage budget:** The plan acknowledges storage growth but doesn't set a target. At Cairn's density, what's the acceptable overhead per month? Is 1GB/month acceptable? 10GB? This affects decisions about what to include in the record (e.g., full tool call payloads vs. tool call summaries).

14. **Is the "no dashboard" line permanent or Phase 1?** The plan says "No dashboard. The data is in git. Query it with git commands, jq, or a future tool." Is this a principled stance (dashboards are out of scope for Cairn forever) or a phasing decision (not now, maybe later)? The answer affects whether the record format needs to be human-browsable or only machine-readable.
