# Adversarial Plan Review: Cairn (TP-forge-solution-Gemini)

### Executive Summary
This plan correctly identifies the fatal flaw in Entire CLI's concurrency model and sidesteps it elegantly with per-session git refs. However, it completely underestimates the operational hazard of coupling deeply to a well-funded, fast-moving startup's open-source layer, and it handwaves the most difficult technical challenge: reading the data back out. The plan designs a write-optimized data lake inside a Git repository without a realistic strategy for querying it or managing the inevitable repository bloat. It successfully solves the capture problem but guarantees the data will be ignored by engineers because the read path is left as an exercise for a "future tool."

### How Plans Like This Fail
- **The "Just Data" Graveyard:** Plans that focus exclusively on capturing data flatly and promise "queries and structure will emerge later" typically fail. If the data isn't immediately actionable or easily queryable from day one, developers will ignore it. 
- **Parasitic Coupling:** Relying on Entire CLI as a "capture substrate" means Cairn is at the mercy of Entire's product decisions. Entire is a $60M VC-backed company; they will move fast, change their plugin protocol, or aggressively pivot. The "we can always fork" fallback is a massive underestimation of the maintenance burden required to support 8+ agent runtimes.
- **Git as a Database:** Using Git to store tens of thousands of flat JSONL records and isolated refs at high concurrency. Git is built for source code, not high-frequency operational telemetry. The plan ignores the physics of Git under this specific type of load (loose ref explosion, packfile churn, push contention on the remote).

### Assumption Audit
- **Assumption:** *Entire CLI's plugin protocol will remain stable and open.* (Load-bearing, low likelihood). Entire is incentivized to build a moat, not serve as a dumb pipe for Cairn.
- **Assumption:** *Per-session refs won't choke Git.* (Load-bearing, moderate likelihood). 60-80 sessions a day equals thousands of refs per month. Unless aggressively packed, `git fetch` and `git push` times will degrade severely due to ref advertisement limits and loose object traversal.
- **Assumption:** *Developers will tolerate `refs/cairn/sessions/*` cluttering their remotes.* (Cosmetic, high likelihood).
- **Assumption:** *We don't need a schema, just flat metadata.* (Load-bearing, low likelihood). By refusing to model the workflow, the burden of reconstructing the DAG is pushed to every single query.
- **Assumption:** *Cairn's secret redaction is covered by Entire.* (Invisible, highly dangerous). The plan says Entire handles redaction, but Cairn hooks into the agent at `SessionStart / UserPromptSubmit`. Is Cairn capturing the prompt *before* Entire's redaction layer processes it? If so, Cairn is leaking secrets directly into its own refs.

### Blind Spots
- **Ref Garbage Collection:** There is absolutely no mention of a retention policy or garbage collection. What happens after 6 months and 15,000 sessions? Who deletes the refs? If no one does, cloning the repository becomes prohibitively slow.
- **Remote Push Contention:** While the *local* writes are contention-free (per-session refs), pushing 60 refs concurrently to GitHub/GitLab will hit rate limits or pack-lock contention on the upstream server.
- **The Indexer's Architecture:** "A periodic indexer... reads all refs and materializes a queryable index." Where does this index live? If it lives in Git, we are back to write contention. If it lives locally, every developer's machine burns CPU to rebuild it. If it's a hosted service, Cairn just lost its "Git-native, no cloud" advantage.
- **Outcome Binding:** The plan explicitly punts on outcome binding ("Outcome... recorded as-is, not computed"). But the problem statement specifically highlights the need to correlate prompts to clean CI and PR states. Without a built-in mechanism to do this, the tool provides no actionable insights.

### Challenged Decisions
- **Decision:** *Relying on Entire CLI.*
  - *Counterargument:* Entire is dead weight and a risk. If Cairn is already building its own refs, its own metadata schema, and Agent Trace emission, it should just write the 5-6 essential hooks itself (Claude, Cursor, Gemini) rather than babysitting a massive upstream dependency that might hostile-fork.
- **Decision:** *Using `refs/cairn/sessions/<uuid>`.*
  - *Counterargument:* Why not use Git Notes (`refs/notes/cairn`) attached directly to the commits produced by the session? Git Notes are designed exactly for out-of-band metadata tied to commits. Per-session refs create orphaned data if the underlying commits are rebased or dropped.
- **Decision:** *Flat metadata instead of a hierarchical workflow schema.*
  - *Counterargument:* The problem is orchestration. A lattice orchestrator has a clear DAG. Flattening this into independent JSONL files throws away the structural context that is the primary differentiator between Cairn and Entire.

### Hindsight Preview
- In two years, we will regret the decision to couple with Entire.io when they deprecate the `entire/checkpoints/v1` format or lock the CLI behind an auth wall, forcing us to maintain a legacy fork of 4,000+ commits just to keep our capture pipeline alive.
- We will regret not implementing a strict schema. The "query engine" will become a nightmare of nested `jq` scripts because every agent version emits slightly different flat metadata.
- **Early Warning Sign:** cloning the repository takes 2x longer after a month of Cairn usage due to ref bloat.

### Reality Stress Test
1. **Entire releases v2.0:** They remove the external agent plugin protocol to drive adoption of their own hosted dashboard. Cairn's capture pipeline instantly breaks for all upgraded users.
2. **The "Mega-Ref" problem:** A single busy repository hits 10,000 `refs/cairn/sessions/*` in a month. GitHub rate-limits pushes from the build server because every `git push` advertises thousands of loose refs.
3. **Secret Leak Incident:** An operator pastes a production API key into a Claude Code prompt. Entire CLI redacts it in the checkpoint branch, but Cairn's `SessionStart` hook grabs the raw prompt and pushes it in plaintext to `refs/cairn/sessions/*`.

### The Uncomfortable Truths
- The plan acts like avoiding Entire's metadata branch solves everything, but it just trades a local concurrency problem (flock/CAS) for a global Git repository bloat problem.
- Cairn is building a write-only black hole. Without a dedicated dashboard or built-in analysis engine, the data will sit in Git and rot. No one is going to write bespoke `jq` queries against thousands of JSONL files to figure out why a PR failed.
- "We can always fork the MIT code" is a developer fantasy. Maintaining a 4,400-commit capture tool that constantly chases vendor API changes is a full-time job for a whole team, not a trivial fallback.

### Hard Questions for the Plan Author
1. How exactly are secrets redacted from the prompt if Cairn captures it at the `SessionStart / UserPromptSubmit` hook *before* Entire's redaction engine runs?
2. What is the garbage collection strategy for `refs/cairn/sessions/*`? At what volume do these refs get packed or deleted?
3. Where does the "periodic indexer" store its materialized index, and how does it sync across multiple developers without re-introducing the exact write-contention you designed this architecture to avoid?
4. If you aren't building an analysis engine or a workflow model, why will any developer actually use this data day-to-day instead of just looking at the git diff?
5. What happens when GitHub/GitLab throttles the repository because 60 concurrent agents are spamming `git push` with thousands of new refs every hour?
