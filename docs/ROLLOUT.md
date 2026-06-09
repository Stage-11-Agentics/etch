# Etch Rollout Plan

**Status: PLAN — capture/enablement steps below are not yet executed.**
Drafted 2026-06-09 against Etch v0.01.001; revised same day after Atin
resolved the open questions. This is the first rollout of Etch beyond its
home repo, so this document doubles as the place where fleet-wide practices
get set. When a step here is executed, the durable version of the practice
moves into `code/platform/etch.md`; this file is the rollout's working plan,
not the permanent reference.

## ⚠️ Migration safety — read before/while moving the Etch repo to GitHub

The Etch repo is being migrated to GitHub as **public open source** (separate
agent, in flight as of 2026-06-09). Three things in the current checkout will
leak session telemetry to the public repo if the migration is done as a naive
"swap origin URL and push":

1. **The local git config has etch push refspecs on `origin`.**
   `remote.origin.push = refs/etch/sessions/*:refs/etch/sessions/*` (plus
   `HEAD`). If `origin` is repointed to GitHub, the very next bare `git push`
   publishes every captured dev-session record — prompts included — to the
   public repo. The migration must either remove the etch fetch+push refspecs
   from the GitHub-pointing remote, or (preferred, see below) keep Forgejo as
   a separate private remote that retains them.
2. **`refs/etch/sessions/*` and `refs/etch/local/*` must not be mirror-pushed.**
   A `git push --mirror` or `git push <remote> 'refs/*:refs/*'` style
   migration carries the session namespaces. Migrate branches + tags only.
3. **`.etch/settings.json` (hostname salt) must stay uncommitted.** It is
   currently untracked — correct for a public repo (a public salt makes
   hostname hashes dictionary-attackable). The earlier plan step to commit it
   is **cancelled**.

**Proposed post-migration shape for the Etch repo itself:** GitHub `origin`
is canonical for code, carries **no** etch refspec; the Forgejo remote
(`s11/etch`) is retained as the private telemetry remote and keeps the
session-ref refspecs (`setup-refspec --remote forgejo`). Code flows public,
session records flow private, dogfooding continues uninterrupted, and the
multi-remote story gets exercised for real. If Forgejo s11/etch is instead
retired, the fallback is c11-style local-only capture on the Etch repo.

## Context

- Etch is fully live on its own repo (Hyperion): binary at
  `~/.local/bin/entire-agent-etch`, hooks committed, refspecs configured,
  session refs synced to Forgejo to date.
- **Decided (Atin, 2026-06-09):** Etch moves to GitHub *now* as a public
  open-source good (handled by a separate agent — out of this plan's scope
  except for the safety section above). The "Forgejo is canonical" doctrine
  stands in general; Etch is a deliberate public exception, like Lattice.
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
   there, or syncs to a *separate private remote* (the post-migration Etch
   repo shape above). `setup-refspec` never writes a `refs/etch/*` wildcard,
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
| 0.1 | Post-migration remote hygiene on the Etch repo: GitHub origin has **no** etch refspecs; Forgejo retained as private telemetry remote (or capture goes local-only). Verify with `git config --get-all remote.<each>.push`. | Replaces the cancelled "commit the salt" step. See the migration-safety section. |
| 0.2 | **GitHub ref-sync smoke test**: scratch *private* GitHub repo (or `c11-private`); `setup-refspec`, push/fetch `refs/etch/sessions/*`, kill-and-recover a session, bare-`git push` semantics | Wave-3 GitHub-private repos depend on this. Custom ref namespaces are supported by GitHub but UI-invisible; confirm no server-side surprises. |
| 0.3 | Write `code/platform/etch.md` | The playbook other agents execute: install, enable checklist, query cheatsheet, the policies above. Stage11 convention — platform docs are the shared brain. |
| 0.4 | Document binary distribution: clone + `make install PREFIX=$HOME/.local` per machine; once the GitHub move lands, `go install github.com/...@latest` and GitHub Releases become available | Good enough for a 2-machine fleet; the public repo improves it for free. |
| 0.5 | File product-gap tickets (non-blocking): `install-hooks` writes the `.etch` gitignore block; `install-hooks --local` → settings.local.json; `etch doctor` (binary on PATH? hooks present? refspec state? age of newest captured session?) | Each found during this planning pass; `doctor` is the answer to "how do we notice silent capture breakage." |

## Wave 1 — pilot (Hyperion, c11 only)

**c11** (GitHub, public, fork of manaflow-ai/cmux) — the load pilot, and the
only wave-1 repo (Atin, 2026-06-09: "just run c11 initially").
`install-hooks`; commit `.claude/settings.json` + gitignore block. **No
refspec, salt stays local** (public-repo rules). What it uniquely tests: high
session volume, many concurrent agents, worktree behavior (worktrees share
the parent's ref store — verify), sibling-clone behavior (six independent
clones = six independent local ref namespaces; committed hooks reach them on
pull; clones that never pull stay uncovered — accepted).

The full-treatment configuration (committed salt + refspec sync on a private
repo) is already proven by the Etch repo's own dogfooding history and, if the
Forgejo-telemetry-remote shape is adopted, continues to be exercised there;
no second wave-1 repo needed.

**Pilot exit bar:** ~1 week of real traffic or ≥50 sessions on c11; zero
agent-visible hook failures; `query`/`index` results sane; at least one
observed `.wip.jsonl` crash recovery; capture latency within the documented
budget (SPEC AC #13).

## Wave 2 — Atlas (cross-machine)

Run the documented fresh-machine path *verbatim* — any deviation is a doc
bug, not an excuse for an ad-hoc fix:

1. Preflight: Go ≥1.22 on Atlas; clone Etch (GitHub once migrated);
   `make install PREFIX=$HOME/.local`; `entire-agent-etch info`.
2. Cross-machine sync test on the Etch repo clone, against the **private
   telemetry remote**: `setup-refspec --remote forgejo`, fetch → Hyperion's
   session refs appear; pull → shared committed salt (private-remote side);
   run a session on Atlas → push → ref visible from Hyperion. (If the Etch
   repo ends up local-only instead, run this test against any private
   Forgejo repo enabled by then.)
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

## Open questions

1. **Post-migration telemetry home for the Etch repo's own sessions** — keep
   the Forgejo remote as the private telemetry remote (recommended above), or
   drop to local-only capture like c11? Decide when the migration lands.
