# ETCH-48 — etch enable: worktree stamping + post-checkout self-propagation + dedupe

Spec: `docs/ENABLEMENT.md` (operator mode, part 2 of 2). Builds on ETCH-47's
`internal/enable` package.

## Base

Branch `etch-48-worktree-stamping` off **origin/main @ e1ea295** (the ETCH-47
squash-merge), worktree at `../Etch-worktrees/etch-48-worktree-stamping`. PR
to main, auto-merge when green.

## Design

### Stamp command shape (dedupe ships here)

Exactly the interim hand-stamp shape already live in the c11 pilot
(verified byte-for-byte against `c11-cmux-catchup/.claude/settings.local.json`):

```
sh -c 'if grep -qs entire-agent-etch .claude/settings.json; then exit 0; fi; if ! command -v entire-agent-etch >/dev/null 2>&1; then exit 0; fi; exec entire-agent-etch <event>'
```

Committed entries win: the grep guard makes the stamp yield whenever the
worktree's branch carries committed hooks. Because the string matches the
hand-stamps exactly, `install.go`'s `matchersContainCommand` detects them
as already-installed — idempotent upgrade, no duplication.

### internal/install refactor (small, mechanical)

`installClaudeHooks` already does the byte-preserving merge; generalize the
command builder: `InstallEntries(path string, cmdFor func(subcommand string) string, force bool) (int, error)`
and `RemoveEntries(path string) error` exported; team-mode functions keep
their current behavior via `hookCommand`. No behavior change for team mode.

### internal/enable additions

1. **`RunEnable` grows three steps** (after config key + excludes):
   - Enumerate worktrees: `git worktree list --porcelain` from cwd.
   - Stamp each worktree root's `.claude/settings.local.json` via
     `install.InstallEntries` with the guarded command builder.
   - Install a marker-delimited `post-checkout` block into the **effective**
     hooks dir (`git rev-parse --git-path hooks` — honors `core.hooksPath`):
     create file with shebang + block + chmod 0755 if missing; append block
     if file exists (chain politely, preserve foreign content byte-for-byte,
     refresh stale block in place — same `replaceBlock` discipline as
     info/exclude).
2. **Post-checkout block** delegates to the binary (logic stays in Go):
   ```sh
   # >>> etch >>>
   if command -v entire-agent-etch >/dev/null 2>&1; then
     entire-agent-etch stamp-worktree || true
   fi
   # <<< etch <<<
   ```
3. **New subcommand `stamp-worktree`**: stamps the current worktree (cwd
   toplevel). Fast-exits silently unless `etch.enabled` is explicitly true
   (operator mode only — key absent means no stamping). Idempotent: already
   stamped → no write. Runs on every checkout/branch switch, so it reuses
   the guard's zero-spawn config read before any git spawn.
4. **`RunDisable` grows best-effort cleanup**: after setting the key false,
   remove etch entries from every worktree's settings.local.json
   (`install.RemoveEntries`) and remove the post-checkout block (delete the
   file if only the etch block + shebang remain, else splice the block out).
   Failures are warnings, not errors — stale stamps are harmless (config
   key gates them).

### Docs

- README Configure section: operator mode (`etch enable`) alongside team
  mode (`install-hooks`).
- docs/HOOK_CONTRACT.md: cross-ref note that dispatch may come from
  committed entries or local stamps, with the dedupe precedence.
- ENABLEMENT.md: keep current if implementation diverges.
- **code/platform/etch.md** (separate Forgejo repo, post-merge): rewrite the
  Worktrees section — drop the interim hand-stamp recipe, document
  `etch enable` once per clone (plan-review MAJOR finding folded in).
- ETCH-46 (doctor, backlog): comment on the ticket adding the new checks
  (config key state, per-worktree stamp coverage, hooksPath mismatch,
  dedupe sanity) — doctor is not implemented yet, so this is ticket-scope
  extension, not code.

## Tests (testutil temp repos, real `git worktree add`)

1. **Headline**: enable → `git worktree add` (with the test binary on PATH)
   → new worktree has the stamp file with guarded entries, and executing
   the stamped command (`sh -c ...`) in the new worktree captures (wip
   appears in shared state). Zero manual steps.
2. **Dedupe**: repo with committed `.claude/settings.json` etch entries +
   stamp → stamped command exits 0, produces nothing; without committed
   entries → stamped command dispatches and captures. Exactly one capture
   per event when both are present.
3. **Disable cleanup**: enable + worktrees stamped → disable → stamps gone
   from all worktrees (foreign settings.local.json content preserved),
   post-checkout block gone, and no capture from either dispatch shape.
4. **core.hooksPath**: repo with custom hooksPath → post-checkout block
   lands in the effective dir; worktree add still auto-stamps.
5. **install-hooks on an operator-mode repo**: committed entries added →
   stamped command yields (grep guard) → no double capture.
6. **Idempotency**: enable twice → settings.local.json byte-identical;
   post-checkout rerun (plain `git checkout`) adds nothing; hand-stamp
   shape pre-seeded → enable detects it, no duplicate entries.
7. **Foreign-content preservation**: settings.local.json with existing
   permissions block (the real c11 main-checkout shape) → stamp merges in,
   foreign keys byte-preserved.
8. **stamp-worktree without operator mode** (key absent or false) → silent
   no-op, no file created.

## Plan-review amendments (folded in)

- Worktree enumeration is best-effort: missing/pruned paths are skipped with
  a warning, one bad worktree never aborts enable (test added).
- Post-checkout chaining checks the shebang: non-sh-family hooks are warned
  about and left untouched (test added); the exec/exit-before-block case is
  a documented limitation in HOOK_CONTRACT.md.
- The stamp predicate is `explicitlyEnabled` (key present AND true) — a
  deliberately different default from `HooksDisabled`'s absent-means-enabled
  compatibility rule; it shares the zero-spawn parsing internals.
- AC#3 mapping: this ticket satisfies "propagation works or doctor flags it"
  via the first arm (absolute core.hooksPath, proven by test); the
  doctor-flags arm is ETCH-46's (ticket extended with the checks, including
  the relative-hooksPath gap where per-worktree resolution defeats
  propagation).

## Out of scope

`etch doctor` implementation (ETCH-46). Live c11 pilot validation happens
post-merge per the rollout plan.
