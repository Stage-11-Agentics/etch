# ETCH-49 — README rewrite: value prop + DevRel best practices

## Phase 1 deliverable — the fundamental value prop

### One-sentence value prop

**Etch is the flight recorder for AI agent fleets: every agent session in a
repository becomes a permanent, queryable record in plain git — no server, no
database, no account — and the write path holds at 60–80+ concurrent agents
across machines.**

### Target user

The person who feels this pain **today** is the **fleet operator**: someone
running many concurrent CLI coding agents (Claude Code today; the substrate
generalizes) across worktrees, branches, and often multiple machines, who has
no record of what all those sessions actually did. Close behind:

1. **Platform/infra engineers** asked to add observability or provenance to an
   agent swarm without standing up a service, a database, or a dashboard.
2. **Teams that need an audit trail** — "which session touched this file,
   under which ticket, on which machine, and did it finish?" — answerable
   after the fact, with git tooling they already trust.

The deliberate design center (from PHILOSOPHY.md): the *next agent* is the
primary reader, not a human at a dashboard. The README should say this — it's
differentiating and explains the shape of the tool.

### Top 3 jobs-to-be-done in the first 30 seconds

1. **"Tell me what my agents did."** Show a real query and a real captured
   record immediately — proof the answer to "what happened in this repo last
   night?" is one command, not archaeology.
2. **"Convince me it survives my scale."** Per-session refs = zero-contention
   writes at any concurrency; records sync machine-to-machine over ordinary
   push/fetch. This is the differentiated ground vs. every single-agent
   capture tool (including Entire's own Checkpoints).
3. **"Tell me what it costs and what the risk is."** Invisible capture (no
   workflow change), one binary, local-only by default on public repos
   (records contain prompt text — the README is loud about this as a trust
   feature), and one `doctor` command to verify health.

### What Etch is NOT (scope honesty)

- **Not a dashboard or analysis engine.** Etch captures and stores; querying
  intelligence lives in the consumer (usually the next agent).
- **Not session restore / checkpointing.** Entire's Checkpoints rewinds your
  working session; Etch records the fleet's history. Complementary layers.
- **Not a cloud service.** No account, no telemetry, no control plane.
- **Not exhaustive secret redaction.** Best-effort regex scanning; the real
  guarantees are local-only defaults + `local_only_fields` projection.
- **Not a workflow engine.** Records are flat; structure emerges at query time.

### Positioning note (Entire Checkpoints overlap)

Etch builds on Entire CLI's hook substrate, and Entire ships its own
single-session capture (Checkpoints). The README must not position on
"session capture exists" — it positions on **fleet scale**: many agents, many
worktrees, many machines, one queryable record store, sovereign (plain git,
no cloud). The comparison section names this honestly and links out.

## Phase 2 plan — the rewrite

- **Base:** branch `etch-49-readme-rewrite` off `origin/main` @ <SHA recorded
  at fetch time — see PR description>, worktree at
  `../Etch-worktrees/etch-49-readme`.
- **Diff discipline:** `README.md` + this plan file only. Never touch the main
  checkout's unrelated uncommitted work (internal/capture, internal/schema,
  docs/INGESTION.md).
- **Structure:** hero (name, one-liner, why-paragraph) → Why Etch (fleet
  framing, 3 jobs) → show-don't-tell (real `query` output + real
  `session.json` excerpt, generated against the built binary in a temp repo —
  zero aspirational output) → Quickstart (<5 min: install → enable → run →
  see the record; lead with operator mode `etch enable` since the target user
  is a fleet operator on their own clones, team mode immediately after) →
  How it works (brief; link SPEC/BUILDPLAN/OUTPUT_SPEC/HOOK_CONTRACT) →
  Configuration (modes, refspec sync, settings.json) → Querying (query/index/
  archive) → Health (doctor) → Privacy posture (loud) → What Etch is not /
  comparison → Development/Contributing → License.
- **Badges:** CI (ci.yml exists), License Apache-2.0, Go version. No vanity.
- **Verification:** `make build`; run every shown command against the binary
  (temp repo + simulated hook events per docs/HOOK_CONTRACT.md for the
  session.json excerpt); render README in a c11 markdown surface.
- **Landing:** PR titled for ETCH-49; auto-merge when CI green + validated;
  lattice complete with real review; closeout commit on main for lattice
  state (repo precedent: f4acc37, b674c82).

### Brand voice calibration

External register of the Stage 11 voice: standard grammar, declarative,
unhedged, no corporate filler ("leverage", "empower", "streamline" banned),
no startup excitement. Kinetic verbs. Trust the reader. The README sells by
stating what is, plainly, and proving it with real output.
