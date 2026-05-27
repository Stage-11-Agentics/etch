# Synthesis: Standard Plan Reviews -- Cairn SOLUTION.md

**Pack:** forge-solution-review-pack-2026-05-26T2128
**Models:** Claude, Codex, Gemini
**Date:** 2026-05-26

---

## Executive Summary

All three models endorse the core architecture: use Entire CLI as the capture substrate, isolate writes via per-session git refs (`refs/cairn/sessions/<uuid>`), keep metadata flat, and defer analysis. The per-session ref design is unanimously identified as the plan's single strongest technical decision -- it eliminates write contention by construction rather than by mechanism. No model questions the fundamental direction.

The models diverge on readiness. Gemini rates the plan "ready to execute." Claude rates it "ready to execute with targeted clarifications." Codex rates it "needs revision before full execution; ready for a narrow proof-of-concept." The disagreement is not about the architecture -- all three approve it -- but about how much unspecified operational detail constitutes a blocker versus a Phase 1 implementation detail.

Three cross-cutting concerns appear in all three reviews and should be treated as high-confidence gaps: (1) asynchronous outcome binding (how do CI/PR results reach a session record after the session ends?), (2) ref lifecycle and storage growth (pruning, compaction, remote compatibility at scale), and (3) the Entire plugin protocol's actual data coverage (is the assumed event surface real?).

---

## 1. Points of Agreement (Highest Confidence)

The following findings appear in all three reviews. These are the highest-confidence signals.

1. **Per-session refs are the correct concurrency architecture.** All models identify this as the defining technical insight. It transforms a shared-mutable-state coordination problem into an embarrassingly parallel namespace problem. Claude calls it "correct by construction, not by mechanism." Codex calls it "turning concurrency from a merge problem into a namespace problem." Gemini calls it "the single strongest technical decision."

2. **Using Entire CLI as the capture substrate is the right call.** Rebuilding hook infrastructure across 8+ agent runtimes is months of commodity work for zero differentiation. All models agree the plugin approach is pragmatic and the MIT license provides an adequate fork path.

3. **Flat JSONL records are the right starting schema.** All models agree that premature schema design kills metadata capture systems, and flat self-contained records are the correct Phase 1 choice. Codex and Claude both note that "flat" needs sharper operational definition (see Divergences).

4. **Agent Trace emission is a strategic win.** All models view emitting Agent Trace as low-cost interoperability that positions Cairn within the emerging Cursor/Cognition/Anthropic ecosystem. Codex adds the useful nuance that Agent Trace should be a side-effect serializer, not Cairn's internal schema.

5. **The "What Cairn does NOT build" boundary is valuable.** All models praise the explicit exclusion of dashboards, analysis engines, and hierarchical workflow models. Plans that know their negative space ship better.

6. **Asynchronous outcome data is unresolved.** All three flag that CI status, PR state, merge outcomes, and rework counts are determined after a session ends. The plan does not specify how or when these are bound to the session record. This is the single most consistently identified gap across all reviews.

7. **Ref proliferation and storage growth need a strategy.** All models flag that 60 sessions/day produces thousands of refs per year. Without pruning, compaction, or archival, `git fetch` and ref listing will degrade. The plan acknowledges this implicitly but provides no concrete lifecycle policy.

8. **The indexer is critical but deferred too far.** All models note that Phase 1 queries require scanning all session refs linearly, which becomes unusable at Cairn's target density within days or weeks. Claude suggests a lightweight Phase 1 index (SQLite or flat file). Codex says the indexer "needs a clearer status in the architecture." Gemini says "performance relies entirely on the proposed indexer."

9. **Entire plugin protocol coverage is assumed, not validated.** All models flag that the plan assumes Entire's external agent protocol provides access to prompts, token usage, tool events, and transcript/checkpoint IDs, but does not confirm this. Claude and Codex both recommend validating the protocol's actual event surface before any implementation begins.

---

## 2. Points of Divergence (Disagreement as Signal)

These are areas where models disagree or emphasize different concerns. The disagreement itself is informative.

### 2a. Readiness Level

| Model | Verdict | Rationale |
|-------|---------|-----------|
| Gemini | Ready to execute | Architecture is solid; clarifications can be addressed during Phase 1 |
| Claude | Ready to execute with targeted clarifications | 5 specific items should be resolved before coding begins, but none challenge the design |
| Codex | Needs revision; ready for narrow POC only | The Entire integration is the plan's front door and must be proven before full execution |

**Signal:** The spread reflects different risk tolerances. Codex treats the Entire dependency as a load-bearing assumption that could collapse the plan if wrong, warranting a proof-of-concept gate. Claude treats it as a known risk with a mitigation path (fork). Gemini treats it as settled based on the evaluation. The conservative path (Codex's POC gate) has the best risk/reward -- proving the substrate costs days and removes the largest remaining uncertainty.

### 2b. "Flat" Records and the Foreign Key Question

- **Claude** notes that including "Lattice ticket ID" in the record is technically a foreign key to an external system, making the "no foreign keys" claim slightly overstated. The principle should be restated as "no internal foreign keys."
- **Codex** goes further: the transcript cross-reference to Entire's `full.jsonl` by checkpoint ID means the record is not fully self-contained. If Entire's checkpoint is missing, the Cairn record loses major value.
- **Gemini** does not flag this tension, treating the flat design as straightforwardly correct.

**Signal:** The plan's language about "flat" and "self-contained" is doing more rhetorical work than the actual data model supports. The records have external dependencies (Entire checkpoints, Lattice tickets). This is fine pragmatically but should be acknowledged in the spec.

### 2c. Security and Redaction

- **Codex** treats security as a critical gap, noting that Cairn proposes storing prompts, machine identity, Tailscale identity, operator identity, tool events, and cost data in git refs that may be pushed. Codex calls this "a sensitive data lake" and argues redaction must be a Phase 1 requirement.
- **Claude** does not raise security/redaction at all.
- **Gemini** does not raise security/redaction at all.

**Signal:** Codex is the only model to flag this, but the concern is well-founded. Pushing agent metadata to shared remotes creates a data-exposure surface that the plan does not address. Even if the repos are private, the metadata contains credentials-adjacent information (env vars, machine identity, prompts that may contain secrets). This deserves Phase 1 attention.

### 2d. Immutability and Update Semantics

- **Codex** raises a precise concern: the plan's push refspec uses `+refs/cairn/sessions/*`, which enables forced updates. Normal operation should use create-only semantics. Codex also flags that "one metadata record per session" may be the wrong invariant once delayed outcomes are included -- the real model may be "one session record plus zero or more observation records."
- **Claude** does not address ref mutability.
- **Gemini** asks about outcome appending but does not address the force-push refspec.

**Signal:** The create-once vs. mutable-ref question is architecturally significant. If refs are immutable, late-arriving data (CI, PR merge) needs a separate mechanism (observation refs, index materialization). If refs are mutable, you need CAS discipline and the "no contention" claim weakens. This must be decided before implementation.

### 2e. Contextual Commits as Scope Creep

- **Claude** flags Phase 3's "Contextual Commits action lines" as the one place where capture bleeds into interpretation. Determining what constitutes a "decision" or "learning" from a transcript is analysis, not capture, which the plan explicitly excludes.
- **Codex** flags the same concern from a different angle: mutating commit messages re-enters the user-visible code history path and should be opt-in and conservative.
- **Gemini** does not flag this.

**Signal:** Two of three models see Contextual Commits as a boundary violation against the plan's own principles. The plan should clarify whether this is a thin pass-through (the agent explicitly tags decisions) or actual content analysis, and whether commit-message mutation is opt-in or default.

---

## 3. Unique Insights (Single-Model Findings)

### Claude Only

1. **Git notes as an alternative.** Claude considers git notes (attaching metadata to commits) as a viable alternative and explains why the plan's ref-based approach is superior (handles sessions that produce no commits, avoids notes merge conflicts). This tradeoff analysis is absent from the other two reviews.

2. **Remote ref compatibility as a potential showstopper.** Claude flags that GitHub has historically been restrictive about custom ref namespaces and asks whether `refs/cairn/*` pushes have been validated against target remotes. Neither Codex nor Gemini raises this, but it is a genuine binary risk -- if the remote rejects the refs, the transport layer is dead.

3. **Phase 1 completeness criteria.** Claude asks for a concrete smoke test definition ("20 concurrent Claude Code sessions on Hyperion each produce a session ref that fetches cleanly on Atlas"). Defining "done" before building keeps scope honest.

4. **Multi-repo sessions.** Claude uniquely asks what happens when an agent session touches multiple repositories -- which repo gets the session ref?

### Codex Only

5. **Security and redaction model.** Codex is the only model to identify the sensitivity of the data Cairn captures (prompts, env vars, machine identity, Tailscale identity) and call for a Phase 1 redaction strategy. This includes how redaction decisions are represented in records (absent vs. redacted vs. unavailable vs. failed-to-capture).

6. **Git object layout.** Codex asks for the concrete object model under a session ref (commit pointing to a tree containing `session.json`, `events.jsonl`, `agent-trace.json`, `transcript-ref.json`). No other model asks for this level of specificity, but it directly affects atomic creation, immutability, fetch behavior, and GC.

7. **Create-only vs. force-push semantics.** Codex identifies the `+` prefix in the push refspec as enabling forced updates and recommends create-only semantics for the normal write path. This is a subtle but important correctness concern.

8. **UUID collision and compare-and-create.** Codex notes that "no CAS needed" is only true if UUIDs are globally unique and refs are create-once. Accidental overwrite from UUID collision or retry logic still needs atomic create semantics.

9. **Distinguishing "manual" orchestration from missing metadata.** Codex asks how Cairn tells the difference between "this was a manual session" and "orchestration metadata was not available."

### Gemini Only

10. **Configuration distribution.** Gemini uniquely asks how all developer machines and CI environments will get the correct git push/fetch refspec configuration for `refs/cairn/*`. Should Cairn provide an initialization script? This is a practical deployment concern the other models overlook.

---

## 4. Consolidated Questions for the Plan Author

Deduplicated and ordered by criticality. Source models noted in brackets.

### Substrate Validation (Highest Priority)

1. **Remote ref compatibility:** Have you verified that GitHub, Forgejo, and your target git remotes accept pushes to `refs/cairn/sessions/*`? If they reject custom ref namespaces, the transport layer needs redesign. This should be validated before any code is written. [Claude]

2. **Entire plugin protocol coverage:** Does the external agent protocol actually expose all the data Cairn needs -- specifically prompts, token usage/cost, tool use events with payloads, files touched, and transcript/checkpoint IDs? Are these available as callbacks for existing supported agents, or only as an adapter interface for adding new agents? [Claude, Codex, Gemini]

3. **Entire protocol stability:** Entire is pre-1.0 (v0.6.2) with active storage consolidation. If Entire ships a breaking change to the plugin protocol, how does Cairn respond -- pin to a known-good version, track head, or maintain a compatibility shim? [Claude, Gemini]

### Architecture and Semantics

4. **Ref immutability:** Is a session ref immutable after creation? If outcomes (CI, PR merge) arrive later, are they written as separate observation refs, appended to the same ref, or materialized only in an index? [Codex, Gemini, Claude]

5. **Force-push refspec:** Should the default push refspec use `+refs/cairn/sessions/*` (forced updates) or should normal operation reject overwrites? If create-only, what is the repair/correction mechanism? [Codex]

6. **Git object layout:** What is the concrete object model under `refs/cairn/sessions/<uuid>` -- a commit pointing to what tree structure? This affects atomic creation, immutability, GC, and query ergonomics. [Codex]

7. **Schema versioning:** When Cairn adds or removes fields from the metadata record, how do consumers distinguish record generations? A `schema_version` field is trivial now and expensive to retrofit. [Claude, Codex]

### Operational Concerns

8. **Ref lifecycle and pruning:** At 60 sessions/day (~22,000 refs/year), what is the retention policy? Options include keeping everything, compacting old refs into an indexed branch, or archival after N days. What is the threshold at which the indexer becomes required rather than nice-to-have? [Claude, Codex, Gemini]

9. **Crash handling:** When a session dies without a clean SessionEnd event, what happens to the in-flight buffer and the session ref? Commit a partial record with `status: crashed`? Discard? Keep on disk for recovery? At 60 concurrent sessions, crashes are routine. [Claude]

10. **Indexer execution model:** Is the indexer running as a git hook, on a cron job on Atlas, on-demand when `cairn query` runs, or some combination? Does the index live in a git ref (fetchable) or local-only? [Claude, Codex, Gemini]

11. **Configuration distribution:** How will all developer machines and CI environments get the correct push/fetch refspec configuration? Should Cairn provide an init script (`cairn init`)? [Gemini]

### Security and Data

12. **Redaction model:** Cairn stores prompts, env vars, machine/operator identity, and cost data in refs that may be pushed to shared remotes. Does Cairn rely on Entire's redaction, implement its own, or both? How are redaction decisions represented in the record (absent vs. redacted vs. unavailable vs. failed-to-capture)? [Codex]

13. **Machine identity sensitivity:** Should hostname and Tailscale identity be included by default, or hashed/pseudonymized unless explicitly enabled? [Codex]

14. **Transcript orphaning:** If Entire's checkpoint is missing (crash, not installed, GC'd, or lost to the known race condition), is the Cairn record still useful, or is it an orphan? Can Cairn capture enough transcript data directly to survive Entire checkpoint loss? [Codex, Claude]

### Scope and Boundaries

15. **Orchestration pattern field:** Is the "Orchestration pattern" field populated by reading environment variables that Lattice/c11 already set, or does Cairn infer the pattern? If inference, this is analysis, not capture. Which env vars? [Claude, Codex]

16. **Contextual Commits:** Phase 3 includes appending action lines "when the session produced explicit decisions/constraints/learnings." How does Cairn determine what constitutes a "decision"? Is this a thin pass-through (agent explicitly tags decisions) or content analysis? Is commit-message mutation opt-in or default? [Claude, Codex]

17. **"No dashboard" permanence:** Is the exclusion of dashboards a principled stance (forever out of scope) or a phasing decision (not now, maybe later)? The answer affects whether the record format needs to be human-browsable. [Claude]

18. **Multi-repo sessions:** Do any orchestration patterns produce agent sessions touching multiple repos? If so, which repo gets the session ref? [Claude]

19. **Storage budget:** At Cairn's density, what is the acceptable storage overhead per month? 1GB? 10GB? This drives decisions about record granularity (full tool payloads vs. summaries). [Claude]

20. **Phase 1 completeness criteria:** What does "Phase 1 done" look like concretely? A specific smoke test (e.g., "20 concurrent sessions on Hyperion each produce a ref that fetches cleanly on Atlas") prevents scope drift. [Claude]

---

## 5. Overall Readiness Verdict

**Architecture: Approved unanimously.** No model questions the fundamental design. Per-session refs, Entire as substrate, flat JSONL, and Agent Trace emission are the right bets.

**Execution readiness: Conditional.** The plan is an architecture sketch that reads as confident but leaves operationally critical details unspecified. The models disagree on whether these gaps are blocking:

- **Minimum gate before any implementation (all models agree):** Validate that `refs/cairn/*` pushes work on target remotes, and confirm Entire's plugin protocol exposes the assumed event surface. These are binary risks that could invalidate the plan.

- **Should be resolved during Phase 1 (majority agreement):** Ref immutability/update semantics, crash handling, schema versioning, redaction model, and a lightweight query index.

- **Can be deferred past Phase 1 (majority agreement):** Ref pruning/compaction strategy, indexer architecture, Contextual Commits scope, multi-repo sessions.

**Recommended next step:** A narrow proof-of-concept (Codex's recommendation, which Claude's "targeted clarifications" and Gemini's "address during Phase 1" are compatible with) that validates the two binary risks, captures one real session end-to-end, and pushes/fetches the ref between two machines. This costs days, not weeks, and converts the plan's largest assumptions into known facts.
