# Plan Review: ETCH-11 — Ref lifecycle + archive compaction

### 1. Verdict

**PASS** — Plan is complete, feasible, and aligned with SPEC #12. Implementation can proceed. The issues below are minor refinements, not blockers.

### 2. Summary

Reviewed the ETCH-11 plan for `cairn archive` / `cairn restore-archive` against the actual codebase (`internal/refs/writer.go`, `internal/config/config.go`, `internal/testutil`, `cmd/entire-agent-cairn/main.go`, SPEC #12, BUILDPLAN). The plan is well-decomposed, technically sound, and reuses established patterns (`os/exec` git plumbing, injected `Now` clock, `testutil.RunBinary` for config/flag wiring). It fully covers the SPEC #12 requirement (threshold-driven archival into `refs/cairn/archive/<YYYY-Q>` with post-archival deletion of session refs). The main thing worth flagging is that `restore-archive` is scope beyond what SPEC #12 / the task description ask for — defensible as a safety net for a destructive op, but worth a conscious confirmation.

### 3. Issues

**[MINOR] restore.go + restore-archive — Scope beyond task / SPEC #12**
The task description and SPEC #12 specify only archival + deletion ("individual session refs deleted after archival"). The plan adds a full `restore.go`, a `restore-archive` subcommand, `main.go` wiring, and two dedicated tests (`TestArchive_RestoreRoundTrip`, `TestArchive_RestoreFromMultipleQuarters`) for forensic recovery that nothing requested. This is reasonable — deleting refs with no recovery path is risky, and the blobs are preserved in the archive tree anyway — but it is added scope. CLAUDE.md notes observation/recovery namespaces were deliberately deferred, so the team has a habit of trimming nice-to-haves.
**Recommendation:** Keep it (the safety argument is strong), but call it out explicitly as an intentional addition in the PR description so reviewers know it wasn't in the ticket. Alternatively, descope `restore-archive` to a follow-up ticket if the orchestrator wants to keep ETCH-11 tight to SPEC #12.

**[MINOR] restore.go step 3 — RefMeta reconstruction is lossy; "round-trip" is content-only**
`refs.WriteSessionRef(repoPath, sessionID, sessionJSON, traceJSON, meta)` stamps the commit author/committer date and message from `RefMeta` (see `commitEnv`/`formatCommitMessage` in `writer.go`). Restoring with "`EndTime=now`" produces a commit with a *different* date, message, and therefore SHA than the original — and, importantly, the restored ref will have a recent commit date, so a subsequent `cairn archive` run will treat it as new and not re-archive it. The plan's `TestArchive_RestoreRoundTrip` asserts "identical content," which holds (the blobs are byte-preserved), but the commit object is not identical.
**Recommendation:** State explicitly in the plan that restore is *content-preserving, not commit-identical*, and prefer deriving `RefMeta.EndTime` (and the other fields) from the archived `session.json` rather than `now` so the restored ref's metadata reflects the original session. Keep `now` only as the fallback when `session.json` lacks the data.

**[MINOR] Archive() — `update-ref` on the archive ref has no old-value guard (concurrent-archiver race)**
Session-ref deletion correctly uses the `update-ref -d <ref> <expected-SHA>` old-value guard, but the plan's `update-ref refs/cairn/archive/<label> <commit>` (step 2) is unguarded. Two concurrent `cairn archive` invocations could both seed from the same pre-existing archive tree and the second `update-ref` would clobber the first, silently dropping sessions the first run added. Session refs are immutable so the hot path is safe, and archive is a manual/maintenance op — but the project explicitly targets 60–80 concurrent agents and a cron-driven archive is plausible.
**Recommendation:** Use the two-argument `git update-ref refs/cairn/archive/<label> <new> <old>` form (old = the SHA read when seeding, or empty/`0{40}` when the ref didn't exist) so a racing archiver fails loudly instead of losing data. At minimum, document that concurrent `archive` runs are unsupported.

**[MINOR] runArchive — `RepoRoot = cwd` is consistent with codebase but breaks from subdirectories**
The plan sets `RepoRoot = cwd`. This matches the existing convention (`hooks.findRepoRoot()` just returns `os.Getwd()`), so it's not a divergence. However, `config.Load(repoRoot)` joins `repoRoot/.cairn/settings.json` literally, so running `cairn archive` from a subdirectory of the repo would miss the config (git plumbing itself walks up to `.git`, so the ref ops would still work — creating an inconsistency where refs are found but config isn't).
**Recommendation:** Resolve the repo root via `git rev-parse --show-toplevel` (already used in `internal/capture/gitstate.go`) before loading config and running git. Low effort, removes a sharp edge. If matching the prevailing cwd convention is preferred, at least note the subdir limitation.

**[MINOR] Config threshold sentinel — `archive_threshold_days: 0` is ambiguous**
`config.Defaults()` sets `ArchiveThresholdDays: 90` and `Load` unmarshals onto a `Defaults()` base, so an omitted field stays 90. But an explicit `"archive_threshold_days": 0` in settings.json yields threshold 0 → cutoff = now → archives essentially everything. The plan's flag uses `-1` as "use config," which is clean, but doesn't address a 0 coming from config.
**Recommendation:** Decide and document the meaning of `0` (archive-all vs. treat-as-unset). A short guard/comment in `runArchive` suffices.

**[MINOR] No usage/help or documentation update for the new subcommands**
`main.go` dispatch has no help text, and the plan doesn't mention documenting `archive` / `restore-archive` (flags, behavior) anywhere (CLAUDE.md, SPEC, or a `--help`). Discoverability of a destructive command matters.
**Recommendation:** Add a one-line summary of each new subcommand and its flags to CLAUDE.md's command list (or wherever subcommands are catalogued), and ensure `flag.FlagSet` usage strings are populated.

### 4. Positive Observations

- **Excellent clock injection.** `Options.Now` plus `RefMeta.EndTime` controlling the committer date means age-based tests need no git time mocking — a clean, deterministic testing story that fits the project's "pure git plumbing, mandatory tests" philosophy.
- **Strong test matrix.** Ten tests cover the real risk surface: age filtering, quarter grouping, nothing-to-archive, dry-run, incremental accretion, content preservation, restore round-trips, and the config-vs-flag precedence path through the real binary (`testutil.RunBinary`). This is exactly the layering CLAUDE.md asks for (core logic direct, wiring via the binary).
- **Right reuse of conventions.** Self-contained `runGit`/`runGitEnv` mirroring `internal/refs/writer.go`, UTC stamping aligned with the writer's `+0000`, and the `--dry-run`/`BuildPlan` separation are all idiomatic to the codebase.
- **Considered design tradeoffs are surfaced.** The plan explicitly justifies archive refs accreting history (vs. the orphan-per-write rule, which is correctly scoped to *session* refs only), the old-value delete guard, deterministic sorting, and the dry-run vs. execute split — good risk awareness.
- **Idempotency thought through.** "New wins on collision" plus incremental seeding from the existing archive tree makes re-running `archive` safe.
