# Etch Rollout Plan

**Status: IN PROGRESS — Wave 0 (0.1–0.5) ✅ and Wave 1 (c11) ✅ executed
2026-06-09. Pilot soak (≥1 week / ≥50 sessions on c11) is the remaining gate;
Waves 2–3 not yet started.**
Drafted 2026-06-09 against Etch v0.01.001; revised same day after Atin
resolved the open questions. This is the first rollout of Etch beyond its
home repo, so this document doubles as the place where fleet-wide practices
get set. When a step here is executed, the durable version of the practice
moves into `code/platform/etch.md`; this file is the rollout's working plan,
not the permanent reference.

## ✅ Migration — DONE (2026-06-09): full clean cut to public GitHub

The Etch repo migrated to GitHub as **public open source** on 2026-06-09:
`github.com/Stage-11-Agentics/etch`, `main` only. Atin chose a **full clean
cut** — Forgejo is fully retired, not kept as a telemetry remote. What was
done, and why each step mattered:

1. **The etch push refspecs were stripped, not repointed.** The old checkout
   had `remote.origin.push = refs/etch/sessions/*:refs/etch/sessions/*` (plus
   `HEAD`) on Forgejo. A naive "swap origin URL and push" would have published
   every captured dev-session record — prompts included — to the public repo.
   Instead: `origin` repointed to GitHub with a **clean fetch refspec only**,
   no `refs/etch/*` push/fetch. Etch's own repo now captures **local-only**,
   exactly the c11 public-repo posture. The 6 session refs created to date
   remain in the local checkout; they were never pushed anywhere public.
2. **Session namespaces were not mirror-pushed.** Only `main` went to GitHub.
   `refs/etch/sessions/*` / `refs/etch/local/*` stayed local.
3. **`.etch/settings.json` (hostname salt) stays uncommitted** — correct for a
   public repo (a public salt makes hostname hashes dictionary-attackable).
   The earlier plan step to commit it was **cancelled**.
4. **Entire CLI's own session-log push was disabled.** The repo dogfoods
   Entire, whose pre-push hook auto-pushes session logs to an
   `entire/checkpoints/v1` branch. `entire configure --skip-push-sessions`
   (committed `push_sessions:false` in `.entire/settings.json`) keeps Entire's
   capture local too — same privacy guarantee as the etch layer.
5. **Forgejo `s11/etch` was archived** (read-only), and the Go module renamed
   `forgejo.stage11.ai/s11/etch` → `github.com/Stage-11-Agentics/etch`.

**Resulting shape:** GitHub `origin` is the single canonical remote, code-only,
no etch refspec; capture is local-only, matching the c11 public-repo rule. The
multi-remote refspec path is now exercised on the private fleet repos (wave 3),
not on the Etch repo itself.

## Context

- Etch is fully live on its own repo (Hyperion): binary at
  `~/.local/bin/entire-agent-etch`, hooks committed. Session refs synced to
  Forgejo up to the 2026-06-09 move; capture is **local-only** now.
- **Done (Atin, 2026-06-09):** Etch moved to GitHub as a public open-source
  good — a full clean cut, Forgejo retired and archived (see the migration
  section above). The "Forgejo is canonical" doctrine stands in general; Etch
  is a deliberate public exception, like Lattice.
- Repo landscape (Hyperion, `code/`): Forgejo-private — platform, holodeck,
  surety, cell06, taste-tester, xray, maia2. GitHub — everything c11-related
  (one main checkout + six sibling clones), c11-private, Lattice,
  lattice-stage-11-plugin, stage11.ai, cell-zero, backstage, subterra, more.
  **Public:** Etch (imminently), c11, Lattice.
- Out of scope (Atin): the non-git Stage11 monorepo root and other non-repo
  zones. Etch covers git repositories; sessions elsewhere are not its
  problem.

## Practices being set (the part that outlives the rollout)

1. **Committed hooks are the install unit.** `entire-agent-etch
   install-hooks` writes `.claude/settings.json`; that file gets committed —
   on public repos too. The hook commands are guarded (`command -v … || exit
   0`), so contributors and CI without the binary are untouched. This is also
   the general-user install story: one person installs and commits, every
   collaborator who opts in just puts the binary on PATH. The gitignored
   `.claude/settings.local.json` is the escape hatch for repos we don't
   control (ticket: `install-hooks --local`).
2. **Each enabled repo also gets the `.etch` gitignore block** (ignore
   `.etch/*`, carve out `!.etch/settings.json`), copied from the Etch repo's
   own `.gitignore` until `install-hooks` learns to write it (ticket below).
3. **Ref-sync policy: private remotes only — Forgejo *or* GitHub.** Session
   records contain prompt text; best-effort redaction is not a publishing
   gate. **Public repos never get an etch refspec** — capture runs local-only
   there (the posture the Etch repo itself now uses), or syncs to a *separate
   private remote* on repos that have one. `setup-refspec` never writes a
   `refs/etch/*` wildcard,
   and `local_only_fields` projection happens at commit time, so this policy
   is defense-in-depth, not the only line.
4. **Salt (`.etch/settings.json`) is committed only on private repos.** On
   public repos it stays local/untracked. On private repos it MUST be
   committed — cross-machine correlation depends on a shared salt.
5. **Capture posture: Etch defaults.** Full prompts, salted-hash hostname, no
   `local_only_fields`. Per-repo overrides (e.g. a customer deployment repo
   stripping `prompt.text`) are available and documented, not default.
6. **Reversibility.** Rollback per repo = `uninstall-hooks` (+ revert the
   settings commit); refs are inert data and can be deleted independently.
   Cheap to undo — which is why enabling broadly is low-risk once the
   playbook is right.

## Wave 0 — harden home base (Etch repo, Hyperion)

| # | Step | Why |
|---|------|-----|
| 0.1 | ✅ **Done.** Post-migration remote hygiene verified on the Etch repo: GitHub origin has **no** etch refspecs (`git config --get-regexp 'refs/etch'` empty), Forgejo remote removed entirely, capture is local-only. See the migration section. | Replaces the cancelled "commit the salt" step. |
| 0.2 | ✅ **Done (2026-06-09).** GitHub ref-sync smoke test against a private scratch repo (`etch-refsync-smoke`): `setup-refspec` (fetch+push+HEAD), a simulated native Claude Code session, bare `git push` (current branch + etch refs), round-trip into a 2nd clone (byte-identical), and kill-and-recover (2 orphan wips → `status:incomplete, exit_reason:crash`, synced too). **No GitHub surprises** — custom `refs/etch/sessions/*` push/fetch/ls-remote all work; only quirk is UI-invisibility (use `git ls-remote`). Fresh clones get 0 etch refs until `setup-refspec`+`fetch`. (Scratch-repo delete blocked on missing `delete_repo` gh scope — flagged for operator.) | Wave-3 GitHub-private repos depend on this. Custom ref namespaces are supported by GitHub but UI-invisible; confirm no server-side surprises. |
| 0.3 | ✅ **Done (2026-06-09).** Wrote `code/platform/etch.md` — install, per-repo enable checklist, ref-sync privacy policy, query cheatsheet, capture defaults, and gotchas from 0.2. Added to the platform service-docs index. Committed + pushed to Forgejo (canonical) and GitHub (in sync; reconciled a prior 1-commit drift). | The playbook other agents execute: install, enable checklist, query cheatsheet, the policies above. Stage11 convention — platform docs are the shared brain. |
| 0.4 | ✅ **Done (2026-06-09).** Binary distribution documented in `etch.md`: clone + `make install PREFIX=$HOME/.local` per machine, and `go install github.com/Stage-11-Agentics/etch/cmd/entire-agent-etch@latest` (verified working post-migration). Reinstall/version-skew procedure included. | Good enough for a 2-machine fleet; the public repo improves it for free. |
| 0.5 | ✅ **Done (2026-06-09).** Filed three backlog tickets in the Etch project: **ETCH-44** (`install-hooks` writes the `.etch` gitignore block), **ETCH-45** (`install-hooks --local` → `settings.local.json`), **ETCH-46** (`etch doctor` — PATH/hooks/refspec/newest-session-age/orphan-wips, priority high). Tickets only — not implemented. | Each found during this planning pass; `doctor` is the answer to "how do we notice silent capture breakage." |

## Wave 1 — pilot (Hyperion, c11 only)

**c11** (GitHub, public, fork of manaflow-ai/cmux) — the load pilot, and the
only wave-1 repo (Atin, 2026-06-09: "just run c11 initially").
`install-hooks`; commit `.claude/settings.json` + gitignore block. **No
refspec, salt stays local** (public-repo rules). What it uniquely tests: high
session volume, many concurrent agents, worktree behavior (worktrees share
the parent's ref store — verify), sibling-clone behavior (six independent
clones = six independent local ref namespaces; committed hooks reach them on
pull; clones that never pull stay uncovered — accepted).

✅ **Enabled 2026-06-09** on the c11 main checkout (`code/c11`, not the sibling
clones). 5 guarded hooks installed; `.etch` gitignore block added; **no refspec,
salt `.etch/settings.json` left untracked**; committed directly to `main`
(`a2cfd1106`) and pushed. Post-push safety verified: **0 etch refs on the public
remote.** Validated end-to-end ("I saw it work"): a real headless Claude Code
session (`claude -p`) fired SessionStart→SessionEnd, captured cleanly to
`refs/etch/sessions/` (`status:complete`, `claude-code/claude-opus-4-8`, 3.3s,
model backfilled from transcript, no orphan wip left behind). Captured fields
match OUTPUT_SPEC (`timing`, `git_start/end`, salted hostname with raw=null,
`c11.workspace_id`). Headless sessions **do** fire the hooks — no discrepancy.
Now in the soak window toward the pilot exit bar below.

The full-treatment configuration (committed salt + refspec sync on a private
repo) is already proven by the Etch repo's own pre-move dogfooding history on
Forgejo; the next live exercise of it is the wave-3 private fleet repos. No
second wave-1 repo needed.

**Pilot exit bar:** ~1 week of real traffic or ≥50 sessions on c11; zero
agent-visible hook failures; `query`/`index` results sane; at least one
observed `.wip.jsonl` crash recovery; capture latency within the documented
budget (SPEC AC #13).

### Pilot finding (2026-06-12): worktree coverage gap → enablement redesign

Three days into the soak, captured volume was far below Atin's real c11
activity. Root cause: most c11 work happens in **worktrees** on feature
branches, and committed hooks are *branch content* — branches predating the
enablement commit have no `.claude/settings.json`, so their sessions never
dispatch. The capture layer itself was already worktree-correct (common-dir
resolution; shared ref store, buffers, and salt) — only hook dispatch was
branch-entangled.

- **Interim patch (applied + validated 2026-06-12):** all 11 c11 worktrees
  hand-stamped with guarded hooks in `.claude/settings.local.json`
  (committed-entries-win dedupe guard included); stamps ignored via the
  shared `.git/info/exclude`. A real session in a stamped worktree produced
  ref #3 in the shared store. Caveat: worktrees created before the feature
  lands still need a manual stamp.
- **Long-term fix ✅ SHIPPED (2026-06-12):** operator-mode enablement —
  `etch enable` writes the `git config etch.enabled` switch, stamps all
  worktrees, and installs a self-propagating `post-checkout` hook so future
  worktrees stamp themselves. Per-project, branchless, nothing committed.
  Full design: [ENABLEMENT.md](./ENABLEMENT.md). ETCH-47 (PR #2, `e1ea295`)
  + ETCH-48 (PR #3, `dd7dcc3`), both merged + done. Supersedes ETCH-45;
  absorbs ETCH-44 for operator mode; doctor checks moved to ETCH-46's scope.
  **Live-validated on the c11 pilot the same day:** `enable` upgraded the 11
  hand-stamps idempotently (2 newly stamped incl. the main checkout), a
  brand-new `git worktree add` auto-stamped with zero manual steps, and a
  real headless session in it landed ref #4 in the shared store (dedupe
  held — the branch also carries committed hooks; exactly one ref). The
  manual-stamp caveat above is closed.
- **Soak-bar implication:** captured volume before 2026-06-12 undercounts
  real activity; judge the ≥50-session bar from the stamp date onward.

## Wave 2 — Atlas (cross-machine)

Run the documented fresh-machine path *verbatim* — any deviation is a doc
bug, not an excuse for an ad-hoc fix:

1. Preflight: Go ≥1.22 on Atlas; clone Etch from GitHub
   (`github.com/Stage-11-Agentics/etch`); `make install PREFIX=$HOME/.local`;
   `entire-agent-etch info`.
2. Cross-machine sync test against a **private** repo (the Etch repo itself is
   local-only and can't exercise this — use any private Forgejo repo enabled
   by then, e.g. platform): `setup-refspec`, fetch → Hyperion's session refs
   appear; pull → shared committed salt; run a session on Atlas → push → ref
   visible from Hyperion.
3. Enable Atlas's working repos under the same policies. Hermes sessions are
   captured wherever they run inside enabled repos.

## Wave 3 — fleet

- **Forgejo-private** (platform, holodeck, surety, cell06, taste-tester,
  xray, maia2): full treatment — hooks + salt committed, `setup-refspec`.
- **GitHub-private** (c11-private, cell-zero, backstage, subterra,
  lattice-stage-11-plugin — verify each repo's visibility first): full
  treatment, refspec included, leaning on the wave-0 GitHub smoke test.
- **Public** (Lattice, stage11.ai if public): capture-only, after the c11
  pilot pattern. Lattice explicitly waits (Atin, 2026-06-09).
- Enablement order within the wave: by session traffic, busiest first.

## Risks

| Risk | Mitigation |
|------|------------|
| Session data leaks to a public remote | Migration-safety section above; no-refspec-on-public rule; commit-time `local_only_fields` projection; `setup-refspec` never writes wildcards. |
| Hook latency degrades agent UX under c11 load | Latency budget documented (SPEC AC #13); pilot watches it explicitly. |
| Capture silently breaks (binary moved, hooks dropped) | `etch doctor` ticket; interim: weekly `query --since` spot check. |
| Version skew Hyperion vs Atlas | Schema is versioned (`etch.session.v1`); records interop; reinstall procedure documented in platform doc. |
| GitHub rejects/throttles custom refs | Wave-0 smoke test before anything depends on it. |

## Decision log

2026-06-09, with Atin (planning interview): pilot = c11; Hyperion first,
Atlas as wave 2; sync to private remotes only, Forgejo *and* GitHub; fleet
default = full capture + hashed hostname; hooks committed even on public
repos.

2026-06-09, with Atin (open-questions round): (1) Etch repo moves to GitHub
immediately, as public open source — separate agent executes the move. (2)
"Forgejo is canonical" doctrine stands generally; Etch is a deliberate public
exception. (3) Wave 1 is c11 only; Lattice and others wait. (4) Non-repo
zones (Stage11 root) are out of Etch's scope entirely.

2026-06-09, move executed (Atin directive: "full clean move"): Etch repo
migrated to `github.com/Stage-11-Agentics/etch` (public, main-only). Forgejo
**fully cut** — remote removed locally, `s11/etch` archived, no telemetry-
remote retention. Etch's own sessions capture **local-only**. Go module
renamed to the GitHub path; Entire session-log push disabled on the public
repo. This resolves open question #1 in favor of local-only over the earlier
"keep Forgejo as private telemetry remote" recommendation.

## Open questions

*(none open — the one below was resolved at the move.)*

1. ~~**Post-migration telemetry home for the Etch repo's own sessions**~~ —
   **RESOLVED (Atin, 2026-06-09): local-only capture, full clean cut.** Forgejo
   is not retained as a telemetry remote; the repo captures local-only like c11.
