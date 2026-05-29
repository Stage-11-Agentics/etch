# ETCH-11 Plan — Ref lifecycle + archive compaction

Archive refs older than threshold (default 90 days, configurable in
`.cairn/settings.json` `archive_threshold_days`) into `refs/cairn/archive/<YYYY-Q>`
archive refs, delete individual session refs after archival. Implements SPEC #12.
See BUILDPLAN.md ETCH-11.

## Goal

Implement `cairn archive` and `cairn restore-archive` subcommands that compact old
per-session refs (`refs/cairn/sessions/<ULID>`) into quarterly archive refs
(`refs/cairn/archive/<YYYY-Q>`), deleting the originals, and support forensic
recovery back to individual session refs.

## Design

### New package: `internal/archive/`

All git plumbing via `os/exec` (`git`), matching `internal/refs/writer.go` style.
A small private `runGit`/`runGitEnv` helper (mirroring the refs writer) lives in the
package to keep it self-contained.

**`archive.go`** — core archival logic, unit-testable:

- `type Options struct { RepoRoot string; ThresholdDays int; Now time.Time; DryRun bool; Since, Until *time.Time; Quarter string }`
  - `Now` is injected so tests can fast-forward time (no global clock).
- `type Plan struct { Quarters []QuarterPlan }`, `QuarterPlan{ Label string; Sessions []SessionEntry }`,
  `SessionEntry{ ULID, Ref, CommitSHA string; CommitTime time.Time }`.
- `func BuildPlan(opts Options) (Plan, error)`:
  1. `git for-each-ref --format='%(refname) %(objectname) %(committerdate:unix)' refs/cairn/sessions/`
  2. Parse ULID (last path segment), commit SHA, commit time per ref.
  3. Cutoff = `opts.Now.AddDate(0,0,-ThresholdDays)`; keep refs with `commitTime < cutoff`.
  4. Apply `--since`/`--until` window filter if set; apply `--quarter` filter if set.
  5. Group surviving refs by `quarterLabel(commitTime)`.
  6. Deterministic sort (by label, then ULID).
- `func quarterLabel(t time.Time) string` → `YYYY-Qn`, `n = (month-1)/3+1`, UTC.
- `func Archive(opts Options) (Plan, error)`:
  1. `BuildPlan`.
  2. Per quarter group, build merged tree for `refs/cairn/archive/<label>`:
     - For each ULID, read its two blob SHAs from the source session ref via
       `git ls-tree <ref>`, `mktree` a subtree → `<ULID>` -> subtree SHA.
     - If archive ref exists, seed top-level entries from its tree (incremental);
       new ULIDs added, collisions: new wins (idempotent).
     - `mktree` the top-level tree from `<ULID> tree <sha>` lines.
     - `commit-tree` with cairn identity + deterministic message; parent = previous
       archive commit if it existed (archive refs accrete history, not orphan-per-write).
     - `update-ref refs/cairn/archive/<label> <commit>`.
     - Per archived session: `git update-ref -d <ref> <expected SHA>` (old-value guard).
  3. Return executed `Plan`.

**`restore.go`** — `func Restore(repoRoot, ulid string, now time.Time) error`:
  1. Find the `refs/cairn/archive/*` ref containing `<ulid>/session.json`.
  2. Read both blobs via `git cat-file blob <ref>:<ulid>/...`.
  3. Recreate `refs/cairn/sessions/<ulid>` via `internal/refs.WriteSessionRef`
     (meta derived minimally from session.json, else `EndTime=now`).
  4. Error if ULID not in any archive ref.

### Command layer: `cmd/entire-agent-cairn/archive.go` (package main)

- `runArchive(args)` — `flag.NewFlagSet`: `--dry-run`, `--threshold-days N`
  (default -1 = use config), `--since`, `--until`, `--quarter`. RepoRoot = cwd.
  Config threshold default; flag overrides when `>= 0`. Dry-run → `BuildPlan` + print;
  real → `Archive` + print summary; nothing to archive → message, exit 0.
- `runRestoreArchive(args)` — positional `<ULID>` → `archive.Restore`, print recreated ref.

### Wire `main.go`

```go
case "archive":         err = runArchive(os.Args[2:])
case "restore-archive": err = runRestoreArchive(os.Args[2:])
```

## Quarter grouping

Q1=Jan–Mar, Q2=Apr–Jun, Q3=Jul–Sep, Q4=Oct–Dec. Label `YYYY-Qn` from committer
timestamp in UTC (matches the `+0000` stamping in `internal/refs/writer.go`).

## Tests (mandatory — `internal/archive/archive_test.go`)

Helper: create session refs at controlled ages via `refs.WriteSessionRef` with
`meta.EndTime` set (writer stamps author+committer dates from `EndTime`).
`archive.Options.Now` controls "today" — no git time mocking needed.

- `TestArchive_OldRefsArchived` — 10 mixed-age refs, threshold=30; old archived+deleted, recent untouched.
- `TestArchive_GroupedByQuarter` — 3 quarters → 3 archive refs.
- `TestArchive_NothingToArchive` — all recent → no archive ref, empty plan.
- `TestArchive_DryRun` — BuildPlan path; nothing modified.
- `TestArchive_IncrementalArchive` — archive, add same-quarter sessions, archive again → extended.
- `TestArchive_ContentPreserved` — `git show refs/cairn/archive/<Q>:<ULID>/session.json` == original.
- `TestArchive_RestoreRoundTrip` — archive then restore → identical content.
- `TestArchive_RestoreFromMultipleQuarters` — restore across quarters.
- `TestArchive_ConfigThreshold` — `.cairn/settings.json` `archive_threshold_days:45` honored (via `testutil.RunBinary`).
- `TestArchive_FlagOverridesConfig` — `--threshold-days 60` overrides config 45 (via `testutil.RunBinary`).

Config/flag tests use `testutil.RunBinary` (real command wiring). Core tests call
`archive.Archive`/`BuildPlan` directly with injected `Now`.

## Risks / notes

- `update-ref -d` old-value guard avoids deleting a ref that changed under us.
- Archive refs accrete (have parents) for clean incremental merges + history.
- All times UTC. Empty repo / no session refs → empty plan, clean exit.
