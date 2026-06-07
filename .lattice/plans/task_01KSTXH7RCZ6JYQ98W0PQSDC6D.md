# Plan — Refspec/Sync Batch (ETCH-16 primary; ETCH-18, ETCH-24, ETCH-38; ETCH-22 subsumed by ETCH-38)

**Branch:** `fix/refspec-batch` (worktree `Etch-worktrees/refspec-batch`) — one PR off `origin/main`.
**Scope:** `internal/commands/setup_refspec.go` + new test file, `cmd/entire-agent-etch/main.go` (call-site/flag plumbing), `README.md` (Configure + new clone/sync section).

## Problem inventory

| Ticket | Defect |
|--------|--------|
| ETCH-16 (high) | `remote.origin.push refs/etch/sessions/*:...` **replaces** git's implicit default-push behavior — bare `git push` pushes ONLY etch refs; user branches silently never leave the machine. |
| ETCH-18 | `setup-refspec` exits 0 with "configured" in a repo with no usable remote (including a phantom URL-less `origin`). |
| ETCH-38 (subsumes ETCH-22) | Fetch refspec lacks leading `+` (README manual-equivalent shows `+`); remote name `origin` is hard-coded with no warning for repos whose remote is named otherwise. |
| ETCH-24 | Fresh clone has zero etch refs until setup-refspec is rerun in the clone — undocumented. |

## Root cause (ETCH-16) — the git config asymmetry

`remote.<name>.fetch` entries written by `--add` are **augmentative**, because `git clone`/`git remote add` materialize the default fetch refspec as an explicit config entry. `remote.<name>.push` has **no default entry** — the default push behavior (`push.default=simple`) is implicit, and the *presence of any* `remote.<name>.push` entry replaces it entirely. So the etch push refspec hijacks bare `git push`.

## Design decision (ETCH-16): augment with `HEAD`, conditionally

When `setup-refspec` writes the etch push refspec into a remote that has **no pre-existing push refspecs**, it also adds `HEAD` as a second push refspec:

```
remote.<name>.push = refs/etch/sessions/*:refs/etch/sessions/*
remote.<name>.push = HEAD
```

`HEAD` resolves to the current branch pushed to its same-name remote branch — i.e. bare `git push` behaves like `push.default=current` *plus* etch refs. **Empirically validated** (temp repo + bare remote): plain `git push origin` pushes the current branch AND etch refs; explicit CLI refspecs (`git push origin main`) still override config refspecs; detached HEAD fails loudly (git's implicit default also errors in detached state).

Conditional guard: if the remote **already has** non-etch push refspecs, the user has deliberately configured push behavior — add only the etch refspec and print a notice telling them their existing refspecs remain authoritative. Never inject `HEAD` into a hand-tuned push config.

Semantic delta vs `push.default=simple`, documented in README and echoed in command output:
- `HEAD` pushes the current branch even when no upstream is set (creates the remote branch) — `simple` would error with a `--set-upstream` hint.
- `HEAD` ignores a differently-named upstream; `simple` would refuse.

These are strictly *more permissive*, never silently dropping the user's commits — the failure mode inverted by ETCH-16.

### Alternatives rejected
- `refs/heads/*:refs/heads/*` — pushes ALL local branches on every bare push; far too aggressive for the 60–80-concurrent-agent worktree world.
- Drop the push refspec; push etch refs from hooks at session end — adds network ops/latency/failure modes inside the hook path and contradicts SPEC ("Transport via standard git push/fetch using refspec configuration").
- Mirror the user's `push.default` with an equivalent refspec — no refspec replicates `simple`; over-engineering.

ETCH-41 (strip-before-push transport) builds on this surface next; keeping transport on plain refspec config (not hook-time pushes) leaves that lane clean.

## Design (ETCH-18 + ETCH-38 remote handling): remote selection + validation

New selection logic in `setup_refspec.go`:

1. Enumerate remotes that have a URL (`git remote` + `git config --get remote.<n>.url`). A config-only remote with no URL (the phantom-`origin` repro) is **not usable**.
2. `--remote <name>` flag (parsed from `os.Args[2:]`): use it; error if it doesn't exist or has no URL.
3. No flag: prefer `origin` if usable; else if **exactly one** usable remote exists, use it and say so (`configuring remote "forgejo"`); else (zero, or multiple non-origin) **fail non-zero** with a message listing the remotes found and the fix (`git remote add origin <url>` or `--remote <name>`).
4. Phantom URL-less remote encountered → the error message names it explicitly so the user understands why it was skipped.

No config writes ever happen against an unusable remote. Success output names the remote it configured.

## Design (ETCH-38 / ETCH-22): fetch `+`, upgrade path

- Fetch refspec becomes `+refs/etch/sessions/*:refs/etch/sessions/*` — matches README's manual-equivalent; `+` allows non-fast-forward fetch so stale/re-pointed refs can't stall a fetch.
- Push refspec stays without `+` (refs are immutable; server-side non-FF rejection should stay loud).
- **Upgrade path:** if the legacy no-`+` fetch entry exists, remove that exact value and add the `+` form (read all values, unset the matching one via value-pattern, add new) — rerunning setup-refspec heals old configs without duplicating entries. Idempotent on repeat runs in both fresh and upgraded repos.

## Design (ETCH-24): clone story

- `setup-refspec` success output appends a hint: `run 'git fetch <remote>' to pull existing session refs`.
- New README subsection under Configure — **"Second machine / fresh clone"**: clone → `entire-agent-etch setup-refspec` → `git fetch origin` → `git for-each-ref refs/etch/sessions/`. Every command in the section is exercised by the e2e round-trip test before it's written down (README carries no untested claims).
- README Configure section updated: manual-equivalent block gains the `HEAD` push line + the conditional caveat; explicit paragraph on what bare `git push` does after setup (the `push.default=current`-like semantics).

## Implementation steps

1. Rewrite `internal/commands/setup_refspec.go`:
   - `RunSetupRefspec(args []string)` (signature change; update `main.go` call site to pass `os.Args[2:]`).
   - Remote enumeration/selection/validation per above; all git calls stay shell-exec plumbing, matching house style.
   - Write fetch `+` refspec with legacy-entry upgrade; write push refspec(s) with the conditional-`HEAD` rule.
   - Output: remote name, what was added, push-semantics notice, fetch hint.
2. New `internal/commands/setup_refspec_test.go` via `testutil.NewTestRepo`/`RunBinary` (binary-level, matching house testing philosophy).
3. README edits (Configure + clone section ONLY — auto-capture delegator owns the auto-capture/usage sections; do not touch them).

## Test matrix (all binary-level, temp repos, zero external deps)

1. No remote at all → non-zero exit, clear message, **no** config writes.
2. Phantom `origin` (config entry, no URL) → non-zero exit, message names the phantom; no writes.
3. Normal `origin` → fetch entry `+refs/etch/...`; push entries exactly `[etch, HEAD]`; rerun is idempotent (no duplicates).
4. Single non-`origin` usable remote → configured, output names it.
5. Multiple usable remotes, none `origin` → non-zero, lists candidates, suggests `--remote`.
6. `--remote <name>` → selects that remote; `--remote bogus` → non-zero.
7. Upgrade: pre-seeded legacy no-`+` fetch entry → replaced by `+` form, single entry.
8. Pre-existing custom push refspec → etch refspec added, `HEAD` NOT added, notice printed.
9. **ETCH-16 regression (mandatory gate):** temp repo + bare remote + local etch ref (`git update-ref refs/etch/sessions/<ulid>`) + commit on a branch → `setup-refspec` → plain `git push origin` → remote has BOTH the branch ref and the etch ref.
10. **E2e round-trip (ETCH-24 gate):** repo A (bare remote) pushes session refs via plain push → clone B → `setup-refspec` in B → `git fetch origin` → etch refs present in B and `git push` from B still pushes B's branch.

## Validation gates

- `go test ./...`, `make build`, `make smoke` green.
- Live e2e round-trip (same shape as test 10, run manually, evidence attached per ticket).
- README claims each backed by a passing test.
- `git restore bin/entire-agent-etch` before committing if `make build` touched it.

## Ticket closure choreography

- ETCH-16/18/24/38 ride the full status lifecycle; ETCH-22 gets only a closure comment ("Subsumed by ETCH-38: <what landed, PR #N>") + cancel attempt.

## Plan-Review Cycle 1 Resolutions (AUTHORITATIVE — overrides earlier text on conflict)

Review artifact: `art_01KTH9F68HN1WNSRGHVNQZ67JE` (PASS, 2 MAJOR / 3 MINOR). All five accepted:

1. **[MAJOR] Multi-remote stance (SPEC criterion 5).** Explicit stance: dual-remote sync (Forgejo + GitHub) is achieved by **rerunning `setup-refspec --remote <name>` once per remote** — git's model means one push reaches one remote regardless, so per-remote configuration is the honest unit. The no-flag multiple-remotes-no-origin error message must say exactly that ("multiple remotes found (a, b); run with --remote <name> for each remote you want to sync etch refs with"). README Configure gains a one-liner on per-remote reruns for multi-remote setups. New test: two usable remotes, run `--remote a` then `--remote b` → both remotes carry the etch fetch/push refspecs independently. SPEC criterion 5 is satisfied via this documented per-remote path, not via a single magic invocation.
2. **[MAJOR] Split the HEAD/notice conditions.** Two precise, separate rules: (a) add `HEAD` **iff the remote's push list was empty before this run**; (b) print the "existing push refspecs remain authoritative" notice **only when a push refspec other than the etch refspec and other than a bare `HEAD` exists**. Test 3 additionally asserts the rerun emits **no** spurious notice; test 8 asserts the notice text appears.
3. **[MINOR] Branch auto-create framing.** README + command output state plainly: after setup, bare `git push` also creates remote branches for upstream-less branches (push.default=current semantics) — one operator-facing sentence, not buried.
4. **[MINOR] `--remote` parse format.** Support both `--remote <name>` and `--remote=<name>`; any other unknown argument errors clearly. Test asserts the unknown-flag error.
5. **[MINOR] Gate framing.** The ETCH-16 regression test (matrix 9) and e2e round-trip (matrix 10) are the **authoritative** validation gates; `make smoke` is a non-blocking environment sanity check (depends on live Entire CLI).

## Reset 2026-06-07 by agent:refspec-w1-impl
