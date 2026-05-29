# Plan Review: ETCH-9 — `cairn query` CLI

## 1. Verdict

**PASS** — The plan is complete, feasible, and aligned with the task description. Implementation can proceed. All issues below are minor and can be addressed inline during implementation rather than requiring a return to planning.

## 2. Summary

I reviewed the ETCH-9 plan (a read-only `cairn query` subcommand over `refs/cairn/sessions/*`) against the actual codebase: `internal/schema/session.go`, `internal/refs/writer.go`, `internal/testutil/testutil.go`, and the `cmd/entire-agent-cairn/main.go` dispatch. The plan is technically sound — every schema field it references exists and is correctly typed, `git show <ref>:session.json` is a valid read path because the ref writer stores `session.json` in the commit tree, and the proposed `WriteSession` test helper composes cleanly with `refs.WriteSessionRef`. The only concerns are a small architectural divergence (an unnecessary `cmd/`-level wrapper), an output-format alignment nuance, and a few under-specified semantics (glob, time comparison, scale) that are worth nailing down but don't block the work.

## 3. Issues

**[MINOR] Architecture — `cmd/entire-agent-cairn/query.go` wrapper diverges from the established dispatch convention**
The plan proposes a thin `cmd/entire-agent-cairn/query.go` wrapper (`RunQuery(args) error`) that delegates to `query.Run`. The existing codebase has no per-command file under `cmd/`; `main.go` dispatches directly to internal package entrypoints (`info.Run()`, `parsehook.Run(...)`, `commands.RunX(...)`). The extra wrapper adds an indirection layer that nothing else in the repo uses.
**Recommendation:** Drop the `cmd/`-level file. Wire the dispatch directly: `case "query": err = query.Run(os.Args[2:])`, matching `info`/`parsehook`/`commands`. The logic already lives in the testable `internal/query` package, which is the right call.

**[MINOR] Alignment — default output is a table, but the task says "outputs JSON"**
The task description and BUILDPLAN both phrase ETCH-9 as "outputs JSON," and ETCH-10 (`cairn index`) is specified to "build on top of cairn query." The plan makes a human-readable table the default and puts JSON behind `--json`. A human table is a reasonable CLI affordance, but the explicit deliverable is JSON, and a downstream programmatic consumer (the index) would more naturally expect machine output by default.
**Recommendation:** Either make `--json` the default and gate the table behind a `--table`/`--pretty` flag, or explicitly document the table-as-human-default / `--json`-for-machines split in the plan and ensure the index ticket consumes `--json`. State the choice so it's a decision, not an accident.

**[MINOR] Filter semantics — `--has-files` glob via `path.Match` won't match nested paths intuitively**
The plan specifies `--has-files` matches `files_touched[].path` via `path.Match`. Go's `path.Match` does not let `*` cross `/` separators, so `--has-files '*.go'` will match `main.go` but **not** `src/query/query.go`. Most users will expect `*.go` to match any Go file. This is a silent footgun, not an error.
**Recommendation:** Decide and document the semantics: match against the basename, use a substring/suffix fallback when the pattern has no `/`, or support `**`-style matching. At minimum, the test matrix's `FilterByHasFiles` case should include a nested path to lock in the chosen behavior.

**[MINOR] Time filtering — "RFC3339 compare" is ambiguous and lexicographic compare is fragile**
The plan says since/until compare against `timing.started_at` via "RFC3339 compare." Lexicographic string comparison of RFC3339 timestamps is only correct when both sides are normalized to the same offset (e.g., UTC `Z`). Sessions written from different machines/timezones could compare incorrectly. The schema stores `StartedAt` as `*string`, so the stored format isn't guaranteed normalized.
**Recommendation:** Parse both the filter bound and `started_at` into `time.Time` and compare as instants (the risk section already says malformed values become non-matching, which implies parsing — make it explicit that both sides are parsed, not string-compared).

**[MINOR] Scale — per-ref `git show` subprocess won't scale to the project's stated target**
Etch is explicitly designed for "60–80+ concurrent agents across multiple machines," which implies thousands of session refs accumulating over time. The plan reads each session with a separate `git show <ref>:session.json` invocation (N subprocess spawns per query). This is fine for a Phase 2 query and ETCH-10 (`cairn index`) exists precisely to address scale — but it's worth acknowledging the limit rather than discovering it later.
**Recommendation:** Note the scaling ceiling in the plan, and consider `git cat-file --batch` (feed all ref blob specs to one long-lived process) as a cheap, single-subprocess alternative that reads every `session.json` in one pass. Not required for ETCH-9 to land, but a small change that removes a known cliff.

**[MINOR] Scope — extra filters beyond the task's enumerated criteria**
The task enumerates `ticket_id, agent_runtime, status, time range, run_id`. The plan adds `--exit-reason`, `--has-files`, `--branch`, plus sort/output flags. These are cheap, useful, and map cleanly to existing schema fields, so this is benign scope expansion rather than risky creep — flagged only for visibility.
**Recommendation:** No change needed; keep them. Just confirm the mandated five are the ones covered by acceptance tests (they are, per the test matrix).

## 4. Positive Observations

- **Clean layer separation.** Pushing the matching logic into `internal/query/filter.go` as pure `Match(*schema.Session) bool` functions makes the core logic trivially unit-testable without git I/O — exactly the right boundary, and consistent with the project's "pure git plumbing, comprehensive testing" philosophy.
- **Accurate schema grounding.** Every field the plan references (`orchestration.ticket_id`, `agent.runtime`, `orchestration.run_id`, `timing.started_at`, `files_touched[].path`, `git_start`/`git_end.branch`) exists and is correctly typed in `schema/session.go`, including correct awareness that `Orchestration`, `GitStart`, `GitEnd`, and `Timing.StartedAt` are nilable — and the plan explicitly handles the nil-pointer case (filter does not match when the field's parent is nil).
- **Defensive read path.** Skipping refs whose blob is missing/unparseable and continuing (logging to stderr) is the right call for an immutable, multi-writer ref space where partial/legacy records will exist.
- **Strong, explicit test matrix.** The twelve named test cases (including `NoMatches`, `EmptyRepo`, `--reverse`) cover the meaningful branches, and the `WriteSession` helper is correctly scoped to shared `internal/testutil` infrastructure that composes with the real `refs.WriteSessionRef` signature — tests exercise the production write path rather than hand-rolled fixtures.
- **Good risk awareness.** The risks section anticipates the empty-repo (`for-each-ref` returns empty, not error), missing-blob, and malformed-timestamp cases — the three things most likely to bite a naive implementation.
