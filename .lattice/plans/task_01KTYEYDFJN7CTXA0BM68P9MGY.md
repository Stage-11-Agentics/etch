# ETCH-47 — etch enable/disable: git-config switch + fast-exit guard

Spec: `docs/ENABLEMENT.md` (operator mode). Part 1 of 2; ETCH-48 builds the
stamping/propagation on top of this.

## Base

Branch `etch-47-enable-disable` off **origin/main @ 36c6121ccf2e1f587dd11783fd2a7abf21dbcdee**,
worktree at `../Etch-worktrees/etch-47-enable-disable`. PR to main, auto-merge
when green (repo policy).

## Design

New package `internal/enable`:

1. **`RunEnable(args)`** — `enable` subcommand.
   - Resolve the git common dir from cwd (`git rev-parse --git-common-dir`);
     error clearly if not in a git repo.
   - `git config etch.enabled true` (local scope writes to the shared common
     config — all worktrees, all branches see it).
   - Write a marker-delimited block into `<commonDir>/info/exclude`:
     `.etch/*`, `!.etch/settings.json`, `.claude/settings.local.json`.
     Idempotent: identical block → file untouched (byte-preserving discipline
     from install.go); markers present with stale content → block replaced;
     foreign content preserved byte-for-byte.
2. **`RunDisable()`** — `git config etch.enabled false`. (ETCH-48 adds
   best-effort stamp/post-checkout removal.)
3. **`Disabled(cwd) bool`** — the fast-exit guard, called by main.go before
   any hook handler runs, before stdin is read:
   - Short-circuit 1 (zero process spawns): walk up from cwd looking for a
     `.git` entry (dir in main checkout, file in worktrees). None → disabled.
   - One `git config --get --type=bool etch.enabled` (the spec's one-spawn
     ceiling). `false` → disabled. `true` or **absent** → enabled
     (compatibility rule: team-mode committed hooks keep capturing with no
     key; `etch.enabled=false` is the explicit off-switch that wins over
     everything).
   - Disabled → exit 0 silently, no stdin read, no filesystem writes.

main.go: add `enable`/`disable` dispatch + usage text; gate all six hook
subcommands (`session_start`, `session_end`, `user_prompt_submit`, `stop`,
`pre_tool_use`, `post_tool_use`) behind the guard.

Behavior change (per spec): hooks invoked outside a git repo now exit 0
silently instead of printing not-ok and exiting 1 — the guard owns that path.

## Tests (testutil temp repos; new `internal/enable/enable_test.go` + binary-level tests)

1. `enable` → key true, exclude block written; rerun → idempotent (exclude
   byte-identical, exit 0).
2. `enable` preserves pre-existing `info/exclude` content; replaces a stale
   etch block without touching neighbors.
3. `enable` from inside a real `git worktree add` worktree → key lands in the
   shared common config; exclude in common dir.
4. `disable` → full hook sequence (session_start → prompt → tool events →
   session_end) produces no refs, no `.wip`, no output — in the main checkout
   AND in a real worktree (acceptance #2).
5. Team-mode compat: repo with committed hooks, NO key → capture works
   unchanged, ref lands (acceptance #3).
6. `etch.enabled=false` wins even with committed hooks present.
7. Hooks in a non-git directory → exit 0, empty stdout.
8. `enable` outside a git repo → non-zero with a clear error.
9. **Latency** (acceptance #4 / SPEC AC #13): run the disabled-path
   `pre_tool_use` repeatedly via the built binary, assert p99 ≤ 50 ms and
   report the measured numbers in the ticket review.

## Out of scope (ETCH-48)

Worktree stamping, post-checkout propagation, dedupe guard inside stamps,
disable's stamp cleanup, doctor scope notes, README/platform-doc updates.
