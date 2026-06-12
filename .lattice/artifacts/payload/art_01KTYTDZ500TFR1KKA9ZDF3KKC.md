# Plan Review: ETCH-49 — README rewrite: value prop + DevRel best practices

## 1. Verdict

**PASS** — Plan is complete, feasible, and aligned. Implementation can proceed.

## 2. Summary

Reviewed the two-phase plan (value prop articulation → README rewrite) for ETCH-49.
The plan is unusually strong: the value prop is sharp and differentiated, the
proposed README structure follows DevRel best practice (hero → proof → quickstart
→ depth), and every factual claim in the plan was verified against the actual
repo — `enable`, `doctor`, `query`, `index`, `archive` subcommands all exist;
`docs/HOOK_CONTRACT.md`, `.github/workflows/ci.yml`, and the Apache-2.0 LICENSE
are all present. The only concerns are minor: binary-name precision in shown
commands and a rendering-verification gap.

## 3. Issues

**[MINOR] Phase 2 plan / Quickstart — `etch enable` is not the binary name**
The plan says "lead with operator mode `etch enable`," but the binary is
`entire-agent-etch` (Entire's `entire-agent-<name>` discovery requirement; see
the project's Naming section). There is no `etch` command on PATH. The current
README correctly uses `entire-agent-etch enable`. If the rewrite copies the
plan's shorthand into a runnable command block, the quickstart breaks on the
very first command — the worst possible place for a README to be wrong.
**Recommendation:** Use `entire-agent-etch` verbatim in every command block.
The plan's own verification step ("run every shown command against the binary")
will catch this if executed faithfully — make that check explicit per command,
not just for the query/session.json excerpts.

**[MINOR] Verification — c11 markdown surface is not GitHub rendering**
The plan verifies rendering "in a c11 markdown surface," but the audience reads
this README on github.com. GFM quirks (anchor slug generation for the TOC-style
internal links, badge image rendering, relative-link resolution from the repo
root) differ from local renderers and are exactly the things that silently break.
**Recommendation:** After the PR opens, view the rendered README on the GitHub PR
"Files changed" / branch view and click every relative link and anchor before
enabling auto-merge.

**[MINOR] Task description — no acceptance criteria exist; the plan defines its own**
The Lattice task carries only a title (description is `null`), so the plan's
Phase 1 deliverable is effectively self-authored acceptance criteria. That is
the right move, but it means the reviewer of the eventual PR has no independent
bar to check against beyond this plan file.
**Recommendation:** Paste the Phase 1 value-prop deliverable (or a link to the
plan file) into the PR description so the implementation review has the same
bar this plan review used. Low effort, keeps the loop honest.

**[MINOR] Positioning note — co-launch sensitivity is real but unaddressed in timing**
Project memory records that Entire's Checkpoints overlap is commercially
sensitive (co-launch gated via a DevRel contact). The plan handles the
*content* correctly — position on fleet scale, name the overlap honestly — but
the repo is public, so the comparison section ships the positioning publicly the
moment the PR merges (and merge is automatic under this project's policy).
**Recommendation:** Keep the comparison factual and generous toward Entire (the
plan's "complementary layers" framing does this). No plan change required;
flagging so the implementer doesn't sharpen the comparison into something
adversarial during drafting.

## 4. Positive Observations

- **Phase 1 before Phase 2 is the right decomposition.** Settling the value
  prop, target user, and jobs-to-be-done before touching prose prevents the
  classic README failure mode of structure-without-thesis. The one-sentence
  value prop is genuinely good — concrete, falsifiable, differentiated.
- **"Zero aspirational output" is an excellent discipline.** Generating the
  shown `query` output and `session.json` excerpt against the built binary in a
  temp repo (per HOOK_CONTRACT.md) guarantees the README's proof section can't
  drift from reality at ship time.
- **Scope honesty section ("What Etch is NOT") is verified-accurate** — every
  negative claim matches the codebase (best-effort regex secret scanning, flat
  records, no server/telemetry). Saying what a tool isn't is rare and
  trust-building, and the plan grounds it in PHILOSOPHY.md rather than inventing it.
- **Diff discipline is explicit and necessary.** The main checkout currently has
  unrelated uncommitted work (`internal/capture`, `internal/schema`,
  `docs/INGESTION.md`); the plan names those files and isolates the work in a
  worktree off `origin/main`. This shows real awareness of the working-tree state.
- **The Entire Checkpoints positioning is handled with maturity** — positioning
  on fleet scale rather than "capture exists" is the correct competitive ground,
  and matches the project's recorded positioning strategy.
- **Brand voice calibration is included**, with concrete banned-word examples,
  so the external register is a checklist rather than a vibe.
- **Landing plan cites repo precedent** (closeout commits f4acc37, b674c82),
  which means the lattice lifecycle steps follow established convention rather
  than improvisation.
