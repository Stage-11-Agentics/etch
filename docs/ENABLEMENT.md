# Enablement Modes — design

How a repository opts into Etch capture. Two modes, serving two different
owners. This document is the design spec for **operator mode** (ETCH-47,
ETCH-48) and the reference for how the modes interact.

## The problem this design solves

Claude Code reads hook configuration from `.claude/settings.json` in the
directory a session runs in. When that file is committed (team mode, below),
hook coverage becomes **branch content**: a worktree checked out on a branch
that predates the enablement commit has no hooks, so its sessions are
invisible. Etch's capture layer is already worktree-correct — `gitstate.go`
resolves the git *common dir*, so worktree sessions share one ref store, one
`.wip` buffer directory, one salt. Only hook *dispatch* was branch-entangled.

The fix: enablement state belongs to the **repository**, not to branches.
Git provides three project-scoped, branch-independent homes — repo config,
the shared hooks directory, and `.git/info/exclude` — and operator mode uses
all three.

## Mode 1 — team mode (committed hooks)

What ships today via `install-hooks`: etch's dispatch entries written into
`.claude/settings.json` and committed. The repo owner enables capture for
every contributor — anyone with the binary on PATH is captured, everyone
else's sessions are untouched (hook commands are guarded with
`command -v … || exit 0`).

Right when: a team wants capture as shared repo policy, distributed through
clone/pull like any other repo state.

Known limitation (the reason operator mode exists): coverage rides branch
content. Worktrees and long-lived branches lag until they contain the
enablement commit.

## Mode 2 — operator mode (`etch enable`)

One command, run once per clone. Everything it writes is project-scoped and
branchless; nothing is committed, nothing is pushed, nothing appears in any
diff:

| Component | Where | Why |
|-----------|-------|-----|
| `etch.enabled = true` | `git config` (common dir) | The authoritative switch. Shared by all worktrees, all branches. `etch disable` flips it; the binary fast-exits everywhere, even if stamps linger. |
| Hook stamps | `.claude/settings.local.json` in the repo root and **every existing worktree** | Claude Code merges local settings like project settings, but the file is untracked. |
| Self-propagation | `post-checkout` git hook in the shared hooks dir | Git runs `post-checkout` on `git worktree add`, so every **future** worktree stamps itself at birth — `git worktree add`, lattice orchestrator, Claude Code `EnterWorktree`, all covered with zero per-worktree action. |
| Ignores | `.git/info/exclude` | Keeps `.claude/settings.local.json` (and `.etch/*`, carving out `!.etch/settings.json`) out of `git status` without touching the committed `.gitignore`. Zero committed footprint on public repos. |

### Fast-exit guard (ETCH-47)

Every hook entrypoint checks, before doing anything else: am I in a git
repo, and is `etch.enabled` true? If not → exit 0. Budget: the disabled
path must stay well inside the capture latency budget (SPEC AC #13), since
PreToolUse/PostToolUse fire on every tool call. One `git config --get` is
the ceiling; cheaper short-circuits welcome.

Compatibility rule: a repo with **committed hooks and no git-config key**
keeps capturing (team mode must not require `etch enable`). The guard
treats "key absent + dispatched from a committed entry" as enabled;
`etch.enabled = false` is an explicit off-switch that wins over everything.

### Dedupe — committed entries win (ETCH-48)

A branch containing committed hooks *plus* a local stamp would dispatch
every event twice and corrupt the buffer. Precedence: **committed entries
win; the stamp yields.** The stamped command embeds the guard:

```sh
sh -c 'if grep -qs entire-agent-etch .claude/settings.json; then exit 0; fi;
       if ! command -v entire-agent-etch >/dev/null 2>&1; then exit 0; fi;
       exec entire-agent-etch <event>'
```

(Hook commands run with cwd = the worktree root, so the relative path is
correct.) This guard ships inside every stamp `etch enable` writes — dedupe
does not depend on binary-side state.

### `etch disable`

Sets `etch.enabled = false` (immediate stop via the fast-exit guard), then
best-effort removes stamps and the post-checkout block from all known
worktrees. Stale stamps left behind by a deleted worktree are harmless: the
config key gates them.

### post-checkout etiquette

The hook must be idempotent (re-runs on every checkout/branch switch, not
just worktree creation: check for an existing stamp, exit fast), and must
chain politely with pre-existing post-checkout hooks — append a
marker-delimited block, same discipline as the settings.json merging. If
`core.hooksPath` is set (husky etc.), install into the *effective* hooks
path; `etch doctor` flags the mismatch case.

## Mode interaction summary

- Team mode alone: works, branch-entangled (accepted limitation).
- Operator mode alone: works, per-clone, worktree-proof.
- Both: no double capture (stamp yields to committed entries per worktree);
  `etch.enabled = false` silences both.
- Fresh clone / second machine: operator mode needs one `etch enable` —
  same per-clone cadence as `setup-refspec`; they share a checklist line.

## Edge cases the implementation must test

1. Worktree created after `etch enable` → stamped automatically, session
   captured (the headline acceptance test).
2. Worktree on a branch that later merges main's committed hooks → exactly
   one capture per event (dedupe).
3. `etch disable` → no capture from either mode, in main and in worktrees.
4. Repo with husky-style `core.hooksPath` → propagation still works or
   doctor flags it.
5. Disabled-path latency within budget.
6. `install-hooks` (team mode) in a repo with operator mode active → no
   double capture afterward.

## Relation to existing tickets

- **ETCH-44** (gitignore block): operator mode absorbs it via
  `info/exclude`; team mode keeps the committed `.gitignore` block.
- **ETCH-45** (`install-hooks --local`): superseded — the stamp writer in
  `etch enable` is the same mechanism, done repo-wide.
- **ETCH-46** (`etch doctor`): grows checks — config key state, stamp
  coverage per worktree, hooksPath effectiveness, dedupe sanity.

## Interim measure (2026-06-12, until ETCH-47/48 land)

The c11 pilot's worktrees were hand-stamped with the exact
settings.local.json + guard shape above (no git-config key, no
post-checkout propagation yet), and `.claude/settings.local.json` added to
c11's `info/exclude`. Validated: a worktree session produced a ref in the
shared store. **New worktrees created before the feature lands still need a
manual stamp** — that's the gap ETCH-48 closes.
