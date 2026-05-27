# Standard Plan Review

PLAN_ID=forge-solution
MODEL=Codex

## Executive Summary

The solution is directionally strong. The central move -- use Entire as the capture substrate and put Cairn's durable metadata into one git ref per session -- is the right architectural instinct for the stated problem. It avoids rebuilding the most annoying hook/runtime integration layer, and it correctly treats high-concurrency write isolation as the first-class design constraint rather than a late scaling problem.

The most important caveat is that the plan currently reads more like a confident architecture sketch than an execution-ready build plan. Its strongest claim is "Entire gives us the lifecycle data without forking"; that must be proven immediately. If the external agent protocol is a general callback/plugin surface, the plan holds. If it is primarily an adapter protocol for Entire to support additional agent runtimes, Cairn may not receive the events it assumes and the integration shape changes materially.

My verdict: promising and worth pursuing, but not ready to execute beyond a proof-of-concept until four things are tightened: the actual Entire integration contract, the security/redaction model, the git object/ref scaling model, and the lifecycle for delayed outcome metadata.

## The Plan's Intent vs. Its Execution

The intent is clear: preserve the missing operational context of agent work -- prompts, transcripts, tools, cost, orchestration, machine identity, git state, and outcomes -- in a form that survives across machines and high-density concurrent sessions.

The execution serves that intent well in one critical area: write-path isolation. A single shared metadata branch would recreate the exact failure mode the problem statement says must be avoided. One ref per session is a clean answer: session writers never contend, indexing becomes a read-side concern, and data loss from indexer failure is avoided.

Where the plan drifts is in the word "flat." It says each record is self-contained and has no required parent references, but the table says transcript is an Entire `full.jsonl` cross-reference by checkpoint ID. That is not necessarily wrong, but it is not fully self-contained. If the Entire checkpoint branch is missing, redacted differently, not fetched, or corrupted by the race the evaluation identified, the Cairn record loses a major part of its value.

There is also an unresolved lifecycle mismatch around outcomes. Session end is not the moment when PR state, CI state, merge status, and rework count are known. The plan says "recorded as observed, not computed," which is sensible, but it does not say whether Cairn records only the session-end observation or supports later append/update observations. Without a delayed observation mechanism, the outcome fields will often be null, stale, or misleading.

## Architectural Assessment

The decomposition is mostly right:

- Entire owns runtime hooks and transcript capture.
- Cairn owns metadata, concurrency-safe storage, workflow labels, outcome observations, query, and Agent Trace emission.
- Git owns transport.
- An indexer owns read performance and aggregation.

That separation keeps Cairn from fighting the wrong battle. It also makes the storage layer independent enough that a future Entire fork or replacement is feasible.

The major architectural ambiguity is the shape of a session ref. A ref can point to a commit, a tree, a blob, or a tag. The solution implies a commit containing a JSONL record, but does not specify the object model. This matters because it affects atomic creation, immutability, fetch behavior, garbage collection, and query ergonomics.

I would expect a concrete shape something like:

```text
refs/cairn/sessions/<session-uuid> -> commit
  tree:
    session.json
    events.jsonl
    agent-trace.json
    transcript-ref.json
```

The ref should be create-once, not force-updated. The proposed refspec uses `+refs/cairn/sessions/*`, which enables forced updates. That is useful for repair tooling, but dangerous as the default. Cairn's normal write path should use create-only semantics and reject overwrites. The remote push refspec should not train users or hooks to force-push provenance unless there is a deliberate repair mode.

The indexer also needs a clearer status in the architecture. It is described as Phase 2, but many of Cairn's stated benefits depend on it. Scanning all session refs with `git for-each-ref` plus blob reads is fine for a smoke test, but not for long-lived high-density repos unless ref counts, packed-refs behavior, and incremental indexing are handled early.

## Is This the Move?

Yes, with a proof-first sequence. The plan makes the right big bet: do not rebuild hooks; do not write to a contended shared branch; do not over-model workflows before the raw capture layer works.

The risk is premature confidence in the substrate. Entire is not just a dependency; it is the plan's front door. The correct next move is not to implement the whole Cairn data model. It is to build the smallest `entire-agent-cairn` proof that demonstrates:

1. It receives the events the plan assumes.
2. It can capture prompt/session/tool/file/token data at the right points.
3. It can associate those events with Entire transcript/checkpoint IDs.
4. It works across Claude Code, Codex, Gemini, and at least one unknown/custom runtime.
5. It behaves correctly under 20 concurrent sessions in separate worktrees.

If that proof passes, the plan is a strong path. If it fails, Cairn should pivot to direct hooks, hook chaining, or a thin fork of Entire's hook layer while preserving the per-session ref storage idea.

## Key Strengths

The per-session ref design is the strongest part of the plan. It matches the actual operating constraint: many writers, multiple machines, same repo, normal git transport. It turns concurrency from a merge problem into a namespace problem.

The refusal to build an analysis engine in Phase 1 is also strong. The problem is data survivability. Query and analysis only matter after capture is reliable. Keeping analytics out of the first build reduces the chance of building dashboards over incomplete data.

Using Entire as a substrate is pragmatic. The evaluation shows Entire has real integration work, a friendly license, and an offline capture path. Cairn should spend its effort on the layer Entire does not solve: multi-agent metadata, workflow labeling, outcome observations, cross-machine availability, and standards emission.

Agent Trace emission is a good interoperability choice. It should be treated as a side-effect serializer, not as Cairn's internal schema. Agent Trace does not appear to cover everything Cairn wants, especially prompts and workflow metadata, but emitting it keeps Cairn aligned with the emerging ecosystem.

The "flat metadata" principle is good as a bias. It resists premature workflow ontology design. The plan should keep this principle, but define what "flat" means operationally: denormalized fields, stable IDs, no required parent workflow object, and safe behavior when related records are absent.

## Weaknesses and Gaps

The security model is underdeveloped. Cairn proposes storing prompts, transcripts, tool events, files touched, token/cost data, machine identity, Tailscale identity, operator identity, PR state, and CI state in git refs that may be pushed and fetched. That is a sensitive data lake. Entire's redaction pipeline may not cover Cairn-owned records, and Agent Trace emission may create another copy. This needs to be a Phase 1 requirement, not a later hardening pass.

The plan does not define immutability or update semantics. Session records may need late-arriving data: CI results, PR numbers, review outcomes, merge status, rework count, maybe even final token accounting. Either a session ref is immutable and later observations become separate refs/events, or session refs are mutable with a CAS/update discipline. The plan says "one metadata record per session," but the real world likely needs "one session record plus zero or more observation records."

Ref cardinality is not addressed. One ref per session is great for write isolation, but 60-80 concurrent sessions over months becomes tens of thousands of refs. Git can handle many refs, but operations degrade if the design ignores packed-refs, pruning, indexing, and remote fetch behavior. The plan needs an explicit retention/compaction/index strategy.

The output schema is not specified enough. A field list is not a schema. Cairn needs versioned record types, required vs optional fields, stable IDs, redaction markers, timestamp conventions, machine identity semantics, and failure states. This can remain simple, but it cannot remain informal.

The plan claims "no CAS needed" for per-session refs. That is true only if session UUIDs are globally unique and refs are create-once. The implementation still needs atomic create semantics to prevent accidental overwrite, UUID collision bugs, or retry logic writing divergent records to the same ref. CAS is not needed for normal contention, but compare-and-create is still needed for correctness.

The relationship with Entire's metadata branch is muddy. The solution says Cairn does not depend on Entire for storage, concurrency, or transport, but it still depends on Entire for transcript capture and checkpoint IDs. If Entire's committed branch drops metadata under race, Cairn may preserve its own metadata while losing transcript references. The plan needs to say whether Cairn can capture transcripts directly enough to survive Entire checkpoint loss.

The phrase "Contextual Commits action lines appended to commit messages" is potentially invasive. One of the design strengths is keeping metadata out of the user's working branch. Mutating commit messages re-enters the user-visible code history path and should be opt-in, conservative, and separated from the core capture guarantee.

## Alternatives Considered

Direct hook implementation instead of Entire: more control and fewer dependency risks, but much slower and likely worse coverage across runtimes. Entire-first is better if the external protocol works as assumed.

Fork Entire immediately: gives full control over write paths and metadata integration, but creates maintenance burden before Cairn has proven its own value. Better as a fallback, not the default.

Sidecar files in the working tree: easier to inspect and push by default, but they pollute the worktree and can create merge conflicts unless every session has its own file. They also change normal repo content, which is not ideal for invisible operational metadata.

Git notes: semantically attractive for metadata, but notes do not push/fetch by default and can become a sync footgun. For Cairn's cross-machine requirement, custom refs with explicit refspecs are clearer.

Single Cairn metadata branch: easier to browse and maybe easier to index, but it recreates the same shared mutable ref problem that the plan correctly rejects.

Local SQLite plus sync service: better query performance and simpler schema evolution, but violates the core git-native, machine-agnostic requirement unless the sync service becomes mandatory. That would be the wrong tradeoff for this problem.

## Readiness Verdict

Needs revision before full execution; ready for a narrow proof-of-concept.

The POC should validate the substrate and storage shape before any query CLI or Agent Trace polish:

1. Confirm what Entire's external agent protocol actually exposes to `entire-agent-cairn`.
2. Capture one real session record with prompt, tool events, token metadata if available, git start/end state, and transcript reference.
3. Write it as a create-once git ref under `refs/cairn/sessions/<uuid>`.
4. Push/fetch it between two machines or two clones using explicit refspecs.
5. Run a 20-session concurrent smoke test and verify no dropped or overwritten records.
6. Verify redaction behavior with fake secrets in prompts, env, files, and tool output.

If those pass, the plan is a good foundation. If the Entire plugin assumption fails, the storage design should survive, but the integration plan must be rewritten.

## Questions for the Plan Author

1. What exact Entire protocol calls does `entire-agent-cairn` receive, and are they callbacks for existing supported agents or only an adapter interface for adding new agents?

2. Can Cairn observe Claude Code, Codex, Gemini, and OpenCode sessions through Entire without replacing Entire's built-in integrations?

3. Does Entire expose raw prompts, tool events, token usage, files touched, and transcript/checkpoint IDs at the times Cairn needs them?

4. If Entire's `entire/checkpoints/v1` branch loses a checkpoint due to the known race, does Cairn still have enough data to preserve the prompt/transcript record?

5. Is the Cairn session ref immutable after creation, or can it be updated when PR/CI/outcome data arrives later?

6. If outcomes arrive later, should they be written as separate observation refs, appended to the same session ref, or materialized only in an index?

7. What is the exact git object layout under `refs/cairn/sessions/<uuid>`?

8. Should the default push refspec use forced updates (`+refs/cairn/sessions/*`), or should normal operation reject overwrites?

9. What is the expected ref count after 1 week, 1 month, and 1 year at normal Atin-density usage, and what git operations become slow at those counts?

10. What data is allowed to leave the machine by default when Cairn refs are pushed?

11. Does Cairn rely on Entire's redaction, implement its own redaction, or both?

12. How are redaction decisions represented in the record so future agents know whether a field is absent, redacted, unavailable, or failed to capture?

13. Should machine identity include hostname and Tailscale identity by default, or should those be hashed/pseudonymized unless explicitly enabled?

14. What is the schema versioning plan for session records and Agent Trace emission?

15. Is "one metadata record per session" still the right invariant once delayed outcomes and indexer observations are included?

16. Who or what runs the periodic indexer, and how does it avoid becoming a new single point of truth?

17. What happens when two machines push different session refs with the same UUID due to a bug or restored VM snapshot?

18. How does Cairn distinguish "manual" orchestration from missing orchestration metadata?

19. Are Contextual Commits action lines core behavior, opt-in behavior, or a future experiment?

20. What is the minimum useful `cairn query` interface for a fresh agent entering a repo, before any dashboard or analytics layer exists?
