# Roadmap

*What Etch could grow into. Forward-looking; evolves often. For what's being built right now, see the Lattice board. For why Etch exists and what it refuses to become, see [PHILOSOPHY.md](./PHILOSOPHY.md).*

---

## Consumption — how a human (or agent) reads the substrate

Etch's first principle is that *the reader is not human* — every record is written for the next agent. That stays true. But the operator still needs to consume the substrate, and the philosophy already names the shape of the answer: capture is the contribution; **analysis, dashboards, and digests are downstream consumers built on top of the honest record, never folded into the capture core.**

**Direction (operator, 2026-06-15):** short term, **agent-mediated query only** — no new surface, just ask an agent and let it run `query`. Target: **build toward generated digests rendered into a browsable wiki** (the Substrate-wiki instinct). The wiki is the thing to design toward; the live dashboard is explicitly not the near-term path.

Options, roughly in order of leverage:

- **Agent-mediated query (have today).** Ask an agent "what happened on c11 this week / on this ticket / in this file's history" and it runs `entire-agent-etch query` and synthesizes. Flexible, conversational, zero new surface. This is the baseline and likely stays the primary path.
- **Generated digests → a browsable wiki.** A periodic rollup (per-week, per-ticket, per-repo) rendered from `query` output into durable, linkable markdown — the same instinct as the Substrate wiki. Fits "permanence over surveillance": the record is permanent, so a digest *of* it can be too. Decision needed: is this an Etch-shipped generator, or a downstream consumer that merely depends on `query --json`? (Philosophy leans hard toward the latter.)
- **Live dashboard.** An HTML view of recent sessions, à la the Lattice dashboard. Tempting, but PHILOSOPHY.md is explicit that a screen-to-watch-agents is the category error Etch refuses. Any dashboard is a *downstream* artifact, opt-in, never part of capture.

The throughline: keep Etch the substrate; let consumption layers proliferate against `query --json` without the core taking a position on presentation.

## Backfill the load pilot

The c11 pilot has ~188 importable sessions on disk (overwhelmingly Codex, which has no live-hook path) that capture never recorded. `entire-agent-etch import` ingests them idempotently (dedupe on `agent_session_id`, hooks always win). Running it converts the pilot from "live-Claude-only" coverage to true every-session coverage. Pending operator go-ahead — it's a one-time bulk write of immutable refs.

## Binary currency check in `doctor` — ✅ delivered

The pilot silently ran a binary that predated the two-path-ingestion release: live Claude hooks worked, but `import` and `capture.method` provenance were missing, so Codex sessions were invisible *and nothing flagged it*. **Shipped:** the binary now carries its build identity — `make build` stamps `Commit` + `BuildDate` via ldflags (with a `runtime/debug` VCS fallback for `go install`), `info` exposes `commit`/`build_date`, and `doctor`'s new `currency` check surfaces version + commit + build date + age prominently and **warns** when the build is older than ~7 days, dirty, or has unknown identity. `doctor`'s `binary` check also compares commit (not just the static version) so a stale PATH binary at the same version no longer looks identical. "The installed binary lags source" is now a routine, visible doctor warning rather than an audit-only finding.

*Nice-to-have not yet built:* a precise "N commits behind origin/main" comparison (would need to locate the Etch source checkout). Deferred to keep the check dependency-free; the age/dirty/unknown signals cover the silent-staleness case.

## Orchestration provenance

The c11 Mailbox audit found `orchestration.ticket_id`/`role`/`run_id` and `outcome.pr_number` empty on every session — the Lattice orchestrator never exported the `ETCH_*` vars, and capture had no fallback, so "who did what" was external inference rather than queryable fact. **Shipped:** orchestration capture is now smart, flexible, and honest. (1) **Flexible namespace** — any `ETCH_META_<key>` var is harvested into `orchestration.extra`, so new provenance dimensions need no Etch code. (2) **Auto-detection** — when no explicit var set `ticket_id`/`role`, Etch infers them from signals it already captures (git branch → ticket, c11 tab title/lineage → role), so capture no longer goes blank just because an orchestrator forgot to wire env vars; runs in the import path too. (3) **Provenance-of-provenance** — every inferred field is tagged in `orchestration.extra._sources`, so declared and inferred values are distinguishable.

*Still open (the other half):* the **orchestrator should still export explicit `ETCH_TICKET_ID`/`ETCH_RUN_ID`/`ETCH_AGENT_ROLE`** as the authoritative source — auto-detection is a floor, not a replacement. That's a lattice-orchestrator skill change (c11 repo). And `outcome.pr_number` / `tokens` remain unpopulated — capture-time doesn't know the eventual PR; a post-hoc enrichment pass (PR by branch, tokens from transcript) is future work.

## Provenance everywhere

`capture.method` / `capture.fidelity` (shipped in source) make "how was this captured" queryable — a repo whose Claude sessions are all `import` signals live hooks silently broke. Ensure every consumer (digest, dashboard, agent prompt) reads provenance so degraded capture is loud, not silent. Records written by pre-provenance binaries carry empty provenance; a one-time provenance backstamp (or simply accept the gap as pre-history) is an open question.
