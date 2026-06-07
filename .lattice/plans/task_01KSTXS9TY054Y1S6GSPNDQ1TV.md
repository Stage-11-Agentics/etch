# Plan — Repo-Root Batch: ETCH-34 + ETCH-35 + ETCH-40 finding 2

**Tickets:** ETCH-34 (primary, this plan file), ETCH-35, ETCH-40 finding 2 (progress comments only — never change ETCH-40 status)
**Branch:** `fix/repo-root-batch` (worktree `Etch-worktrees/repo-root-batch`), one PR off `origin/main`
**Author:** agent:reporoot-w0-planner, 2026-06-07

## Problem

`findRepoRoot()` (`internal/hooks/common.go:39`) returns raw `os.Getwd()` while its doc
comment claims git-root resolution. All four hook entry points (`session_start`,
`session_end`/`stop`, `user_prompt_submit`, `pre/post_tool_use`) anchor **everything** —
`.etch/sessions/` wip buffers, the ULID mapping, `settings.json` resolution, the recovery
scan, and ref writes — to the hook process's CWD.

Consequences (deep review 2026-06-04, finding 2; ETCH-34):

1. Hooks for one session firing from different CWDs (subdir, linked worktree) scatter state
   across multiple `.etch` dirs. `session_end`'s `LookupMapping` reads a different `.etch`,
   returns `""`, takes the silent `printOK()` path — **session dropped, no record, and
   recovery never scans the other CWD's `.etch`, so it's unrecoverable.**
2. `.etch/settings.json` at the repo root is silently ignored for any session not started
   at the exact root — redaction patterns, `raw_machine_identity`, `recovery_timeout_hours`
   all silently stop applying (security-relevant).
3. Crash `.wip` files fragment into per-subdir `.etch` dirs the recovery sweep never visits.

Separately (ETCH-35): in a **non-git** directory, `session_start` happily creates `.etch/`
and exits 0; `session_end`'s `commitSession` fails (`fatal: not a git repository`) but the
error is only `log.Printf`'d (`session_end.go:62-67`) — the process still prints
`{"ok":true}`. Silent data loss, orphaned `.wip`, polluted directory.

## Boundary-of-scope guards (from the deep review's REFUTED list — do NOT "fix")

- *exit_reason clobbered by stop-after-session_end* — refuted; the mapping-miss `printOK()`
  on the second end-hook is **by design** and must keep working.
- *index update races* — refuted; out of scope.
- *Finalize diff runs against wrong checkout in worktrees* — refuted **because** repoRoot
  currently equals the session's CWD. The fix below deliberately preserves this property:
  git state capture and diffs stay anchored to the invoking checkout's toplevel.

## Design

### Two roots, resolved once per hook invocation

The single conflated `repoRoot` becomes two explicit values:

| Value | Resolution | Used for |
|---|---|---|
| **`stateRoot`** | parent of `git rev-parse --git-common-dir` (absolute) — identical logic to `gitstate.go`'s `RepoRoot` field | `.etch/` dirs (wip, mapping, session.json), `settings.json` loads, recovery scan, ref writes |
| **`workDir`** | `git rev-parse --show-toplevel` of the hook's CWD | `CaptureGitState` / `CaptureGitEnd` / `gitDiffFiles` / `CaptureOperator` — anything that must see the session's own checkout |

**Worktree design choice (documented per ticket instruction):** `.etch/` state for linked
worktrees lands at the **main repo root** (common-dir parent), shared by all worktrees.
Rationale:

- *Maximal CWD-drift immunity:* hooks firing from the main root, any subdir, or any linked
  worktree of the same repo all resolve the same `.etch` → one session = one wip = one
  mapping = one record. This is the strongest possible fix for the session-drop scenario.
- *Recovery always finds orphans:* the recovery sweep runs against `stateRoot/.etch/sessions`,
  so a session that crashed in a worktree is recovered by the next session **anywhere** in
  the repo — including after the worktree itself is deleted (worktrees are ephemeral in our
  orchestration pattern; per-worktree state would die with them).
- *Settings apply uniformly:* `.etch/` is gitignored, so a per-worktree `.etch/settings.json`
  would never exist in fresh worktrees — redaction config would silently weaken exactly as in
  ETCH-34. Anchoring at the main root makes one settings file govern all checkouts.
- *Refs are shared anyway:* `refs/etch/sessions/*` live in the common dir regardless of which
  checkout runs `update-ref`; writing them from `stateRoot` changes nothing observable.
- *Concurrency is safe:* wip files are keyed by per-session ULID, mappings by Entire session
  ID — 60–80 agents appending to distinct files in one shared dir is contention-free.
- *Diff correctness preserved:* git state capture stays on `workDir`, so a worktree session
  records its own branch/SHA and diffs its own checkout (keeps the refuted finding refuted).

Sessions whose recorded `git_state.worktree_path` differs from the main root remain fully
attributable: `is_worktree` and `worktree_path` are already captured fields.

### Resolution implementation

New exported helper in `internal/capture` (it owns git plumbing helpers and `gitstate.go`
already computes the same thing):

```go
// RepoContext anchors hook state and git operations.
type RepoContext struct {
    StateRoot string // main repo root (parent of git common dir): .etch state, settings, refs
    WorkDir   string // toplevel of the invoking checkout (linked-worktree aware): git capture/diffs
}

// ResolveRepoContext resolves both roots from dir (the hook CWD).
// Returns an error when dir is not inside a git repository.
func ResolveRepoContext(dir string) (*RepoContext, error)
```

- One `git -C dir rev-parse --show-toplevel --git-common-dir` call (two output lines).
- Relative `--git-common-dir` output (e.g. `.git`) resolved against the CWD per git's
  relative-to-CWD output semantics, then `filepath.Dir` → `StateRoot` (same logic as
  `gitstate.go:32-38`).
- Non-zero git exit (`fatal: not a git repository`) → `error`. **No fallback to CWD.**
- `hooks.findRepoRoot()` is deleted; each hook calls `capture.ResolveRepoContext(cwd)` once.

Known limitation, documented in the helper's comment: submodules resolve `--git-common-dir`
into the superproject's `.git/modules/<name>`, so `Dir()` is not a checkout root there.
Submodule sessions are out of scope (no current use; behavior is no worse than today).

### Hook threading (all four entry points)

- `session_start`: `EnsureDirs(StateRoot)`; recovery scan over `StateRoot/.etch/sessions`
  with `ReadTimeoutFromSettings(StateRoot)` and `RecoverAll(..., StateRoot, ...)`;
  `config.Load(StateRoot)`; `CaptureGitState(WorkDir)`; `CaptureOperator(WorkDir)`;
  `AppendEvent(StateRoot, ...)`; `WriteMapping(StateRoot, ...)`.
- `user_prompt_submit`, `tool_use`: `LookupMapping(StateRoot, ...)`, `AppendEvent(StateRoot, ...)`.
- `session_end`/`stop` (`runEnd`): `LookupMapping(StateRoot, ...)`; `ReadEvents(StateRoot, ...)`;
  `CaptureGitEnd(WorkDir, startSHA)`; `Finalize` gains the second root (below);
  `commitSession(StateRoot, ...)`.
- `capture.Finalize(repoRoot, sessionID)` → `Finalize(stateRoot, workDir, sessionID)`:
  reads/writes state under `stateRoot`, runs `gitDiffFiles(workDir, ...)`. All callers and
  tests updated.
- `commitSession` / `etchRefWriter`: `config.Load`, `refs.WriteSessionRef`, wip/mapping
  cleanup all take `StateRoot` (ref writes from the main root are always valid).

### ETCH-35: non-git visibility

- Every hook resolves the context **first**. On `ResolveRepoContext` error:
  - print a clear line to **stderr**: `etch: not inside a git repository (cwd=...): session capture disabled, no record will be written`
  - print **no** `{"ok":true}`; return the error → `main.go` prints `error: ...` and exits 1
    (the Entire contract treats hook stdout/exit observationally — a non-zero plugin exit
    surfaces in Entire's hook logging without killing the agent; this is the "non-ok result
    where the contract permits").
  - **No `.etch/` is created, no `.wip` is written** — nothing to orphan, no pollution.
- `runEnd`: `commitSession` failure is no longer swallowed (ticket ETCH-35 fix clause:
  "don't print ok on commit failure"). On error: keep wip + mapping on disk (recovery can
  still commit later — cleanup ordering already guarantees this), log to stderr, **return
  the error instead of `printOK()`**.
- Mapping-miss in `runEnd` stays `printOK()` (required by the refuted stop-after-end
  behavior) but gains a one-line stderr `log.Printf` so a genuinely dropped session is
  no longer invisible.

## Tests (adversarial, per validation-plan gates)

All via the existing `testutil.RunBinary` harness (real binary, real temp git repos,
`cmd.Dir` controls the hook CWD):

1. **Subdir scattering (gate):** `git init` repo with `src/deep/nested`; `session_start` at
   root, `user_prompt_submit` + `tool_use` from `src/`, `session_end` from `src/deep/nested`.
   Assert: exactly one `.etch/` (at root), no `.etch` in subdirs, one committed ref whose
   session.json contains the prompt and tool counts (ONE coherent record).
2. **Settings honored from subdir:** `.etch/settings.json` at root with a custom
   `redaction_patterns` entry; full hook sequence fired from a subdir; assert the committed
   prompt is redacted (proves `config.Load(StateRoot)`).
3. **Linked worktree (gate):** `git worktree add`; full hook sequence fired from inside the
   worktree (and a worktree subdir). Assert: state lands under the MAIN root's `.etch/`;
   committed `git_state` records the worktree's branch/`worktree_path`/`is_worktree:true`;
   diff reflects a commit made in the worktree (not the main checkout).
4. **Worktree orphan recovery:** simulate a crash in a worktree session (write wip, skip
   end), then run `session_start` from the main root with timeout 0 — assert the orphan is
   recovered into a ref.
5. **Non-git (gate, ETCH-35):** all four hooks in a plain temp dir. Assert: exit code != 0,
   stderr mentions "not inside a git repository", stdout does NOT contain `"ok":true`, and
   no `.etch/` exists afterward.
6. **Commit-failure visibility (ETCH-35):** valid repo, full sequence, but sabotage ref
   writing before `session_end` (e.g. make `.git` read-only or pre-create a conflicting
   lock). Assert: non-ok/non-zero result, wip + mapping still on disk.
7. **Stop-after-end unchanged (regression guard for the refuted finding):** session_end then
   stop for the same Entire session ID → second call still exits 0 with `{"ok":true}`.
8. Existing suites (`hooks_test`, `e2e_test`, `capture_test`, `recovery_test`) updated for
   the `Finalize` signature and any CWD assumptions; `go test ./...`, `make build`,
   `make smoke` green.

## Step sequence

1. `capture.ResolveRepoContext` + unit tests (root repo, subdir, linked worktree, worktree
   subdir, non-git, relative common-dir resolution).
2. Rewire the four hooks + `commitSession` + `Finalize` signature; delete `findRepoRoot`.
3. ETCH-35 error paths (non-git early exit; commitSession propagation; mapping-miss log).
4. Adversarial tests 1–7; fix existing tests.
5. Full gates; live e2e per validation plan (real `git init` temp repo + subdir + non-git).
6. Commit (never `bin/entire-agent-etch`), push, PR via Forgejo REST.

## Risks

- **Hook tests that fired from a repo root keep passing silently while behavior changed for
  subdirs** — mitigated by tests 1–4 explicitly varying `cmd.Dir`.
- **`--git-common-dir` relative-path quirks across git versions** — mitigated by resolving
  relative output against the CWD exactly as `gitstate.go` already does in production.
- **Entire treating exit-1 hooks as fatal** — `make smoke` exercises the real Entire CLI
  path; non-git is a misconfiguration state where loud failure is the explicit requirement.

## Plan-Review Cycle 1 Resolutions (AUTHORITATIVE — overrides earlier text on conflict)

Two reviews ran: ETCH-34 (this plan) → **PASS** with 6 minor items; ETCH-35 → **FAIL**,
but that review was against ETCH-35's own plan file, which was still the auto-generated
ticket-description stub — the batch plan (this file) is authoritative per the run boot
prompt. ETCH-35's plan file is being replaced with a real pointer-plan and re-reviewed.
All substantive points from both reviews are resolved here:

1. **Ticket→test mapping in PR/validation (ETCH-34 minor #1) — ACCEPTED.** PR body and
   validation evidence will map tests to owning tickets explicitly:
   tests 1–4 + 7 → ETCH-34 / ETCH-40 f.2; tests 5–6 → ETCH-35.
2. **"Not a git repository" message conflates git-missing with non-repo (ETCH-34 minor #2)
   — ACCEPTED.** `ResolveRepoContext` wraps the underlying git error: message becomes
   `etch: could not resolve a git repository (cwd=...): <underlying error / git stderr>`.
   A clean `fatal: not a git repository` exit and an exec failure are thereby
   distinguishable from the surfaced text.
3. **Pre-existing scattered `.etch` state not migrated (ETCH-34 minor #3) — ACCEPTED,
   explicitly out of scope.** Etch is pre-release/dogfood-only; legacy per-subdir `.etch`
   dirs from the buggy resolver are abandoned, not swept. Manual cleanup
   (`find . -name .etch -type d`) noted in the PR body. No code.
4. **Test 6 sabotage mechanism (ETCH-34 minor #4) — ACCEPTED.** Primary sabotage is a
   deterministic ref-write conflict (pre-create `refs/etch/sessions/<ulid>` as a directory
   or a stale `.lock`), not read-only `.git`.
5. **Defensive guard on `rev-parse` two-line output (ETCH-34 minor #5) — ACCEPTED.**
   `ResolveRepoContext` validates exactly two non-empty lines (toplevel may be absent in
   bare repos → treated as "not a usable repo" error, same path as non-git).
6. **`ResolveRepoContext`/`CaptureGitState` duplicate rev-parse execs (ETCH-34 minor #6)
   — DEFERRED.** Correctness-neutral; folding the two resolutions together belongs to the
   `gitexec` consolidation already on the cleanup backlog. Not in this PR.
7. **ETCH-35 remedy choice (ETCH-35 critical #2) — DECIDED: visible error, not silent
   no-op.** The run's validation gate mandates it: "Non-git invocation is visible — does
   not return `{"ok":true}` while dropping data." Implementation: stderr line +
   `{"ok":false,"error":"..."}` on stdout + exit 1 (via the existing `main.go` error
   path). All six hook subcommands take this path — a mid-session hook in a non-git dir
   also drops data, so it must also be non-ok. Entire treats plugin hooks observationally;
   a non-zero exit surfaces in Entire's hook logging without killing the agent session,
   and non-git is a misconfiguration where loud is the requirement. `make smoke` guards
   the real-Entire happy path.
8. **Detection chokepoint shared by all six hooks (ETCH-35 major #1) — ALREADY IN PLAN.**
   `findRepoRoot()` is deleted; every hook resolves `ResolveRepoContext` first and
   short-circuits with the visible error before any filesystem write (ordering note from
   ETCH-35 minor #4: detect → fail before `EnsureDirs`/`WriteMapping`).
9. **Swallowed `commitSession` error generalizes beyond non-git (ETCH-35 major #2) —
   ALREADY IN PLAN.** `runEnd` propagates the error (stderr + non-zero, no `printOK`);
   wip + mapping are retained so the next `session_start` recovery scan can retry.
10. **Test plan + acceptance criteria for ETCH-35 (ETCH-35 major #3, minors) — ALREADY IN
    PLAN** (tests 5–6; gates restated in "Tests" section). ETCH-35's plan file now lifts
    its acceptance criteria explicitly.

### Cycle 2 (ETCH-35 re-review PASS) resolutions

11. **Stdout contract pinned (cycle-2 major #1).** Committed choice: machine-readable
    `{"ok":false,"error":"..."}` IS printed on stdout, via a new `printNotOK` helper the
    hooks call explicitly before returning the error; the `main.go` error path then adds
    `error: ...` on stderr and exits 1. The earlier "via the existing main.go path"
    phrasing was wrong about the mechanism and is superseded by this item. Net behavior:
    stdout = `{"ok":false,...}`, stderr = warning + `error:` line, exit 1.
12. **Exit-1-under-real-Entire is a validation gate, not an assumption (cycle-2 major
    #2).** The validate phase will run the non-git case against the real Entire CLI's
    hook dispatch (alongside `make smoke`) and confirm the agent session is not aborted.
    Go/no-go: if real Entire treats a non-zero `session_start` exit as fatal, fall back
    to loud-stderr clean no-op (exit 0, still no `.etch/` created, stdout still never
    `{"ok":true}` on the data-dropping end path) and record the pivot here.
13. **Commit-failure recovery is deferred, not immediate (cycle-2 minor).** A retained
    wip from a failed commit has a fresh mtime; the recovery sweep re-commits it only
    after `recovery_timeout_hours` elapses. Accepted and documented — eventual recovery
    is the contract.
