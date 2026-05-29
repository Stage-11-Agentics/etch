# ETCH-9: cairn query CLI — Plan

## Goal
Add a `cairn query` subcommand (`entire-agent-cairn query`) that reads all
`refs/cairn/sessions/*` refs, parses each `session.json`, applies AND-combined
filters, and emits a table / JSON / count, sorted by start time.

## Architecture
- **`internal/query/query.go`** — `Run(args []string) error` entrypoint; flag
  parsing, ref enumeration (`git for-each-ref`), session loading
  (`git show <ref>:session.json`), sorting, and output rendering (table / `--json`
  / `--count`).
- **`internal/query/filter.go`** — `Filters` struct + `Match(*schema.Session)
  bool`, plus glob/time helpers. Pure functions, easy to unit test.
- **`internal/query/query_test.go`** — full test matrix (see below).
- **`cmd/entire-agent-cairn/query.go`** — thin wrapper `RunQuery(args) error`
  delegating to `query.Run`. (Kept thin so logic stays in the testable package.)
- Wire `case "query":` into `cmd/entire-agent-cairn/main.go` dispatch.

## Data flow
1. Determine repo: `--repo <path>` flag or cwd (passed as `git -C <path>` dir).
2. `git for-each-ref refs/cairn/sessions/ --format='%(refname:short)'` → ref list.
3. For each ref: `git show <ref>:session.json` → unmarshal into `schema.Session`.
   Skip refs whose blob is missing/unparseable (defensive; log to stderr, continue).
4. Apply `Filters.Match` (AND semantics). Cheap pre-filters where possible.
5. Sort by key (`started_at` default, `duration`, `session_id`), descending by
   default; `--reverse` flips.
6. Render per output mode.

## Flags
`--repo`, `--ticket`, `--runtime`, `--status`, `--exit-reason`, `--run-id`,
`--since`, `--until`, `--has-files`, `--branch`, `--json`, `--count`,
`--sort`, `--reverse`. Parsed with stdlib `flag.FlagSet`.

## Filter semantics
- ticket → `orchestration.ticket_id`
- runtime → `agent.runtime`
- status → `status`
- exit-reason → `exit_reason`
- run-id → `orchestration.run_id`
- since/until → `timing.started_at` (RFC3339 compare; inclusive since, inclusive until)
- has-files → any `files_touched[].path` matches glob (`path.Match`)
- branch → `git_start.branch` OR `git_end.branch`
- Nil pointers (orchestration, git_start, etc.) → filter for that field does not
  match (unless filter unset). Unset filters are skipped.

## Output
- Table (default): header + rows `SESSION  RUNTIME/MODEL  TICKET  DURATION  STATUS`.
  session_id short = first 8 chars. Aligned with tabwriter.
- `--json`: `json.MarshalIndent` of `[]schema.Session`. Empty → `[]`.
- `--count`: integer + newline.

## Sorting
- Comparable keys; nil started_at sorts last in descending. Stable sort.

## Tests (mandatory, all from supplement)
AllSessions, FilterByTicket, FilterByRuntime, FilterByStatus,
FilterByTimeRange, FilterByHasFiles, MultipleFilters, JSONOutput, CountOutput,
SortStartedAt (+reverse), NoMatches, EmptyRepo.

## testutil extension
Add `WriteSession(t, repo, session schema.Session)` helper that marshals a
`schema.Session`, calls `refs.WriteSessionRef` with a minimal trace + derived
meta, so tests can seed realistic sessions. Shared infra → lives in
`internal/testutil`.

## Risks
- `git show` for missing blob errors → handle gracefully, continue.
- Empty repo / no refs → `for-each-ref` returns empty, not error → empty result.
- Time parsing of malformed `started_at` → treat as non-matching for range filters.
