ETCH-9 self-review (fast-track, inline).

SCOPE: cairn query subcommand. internal/query/{query,filter}.go, cmd wrapper, main dispatch wiring, testutil WriteSession helper, 18 tests.

VERIFIED:
- All required tests pass (AllSessions, FilterByTicket/Runtime/Status/TimeRange/HasFiles, MultipleFilters, JSONOutput, CountOutput, SortStartedAt+reverse, NoMatches, EmptyRepo) plus extras (duration sort, table output, branch, run-id, exit-reason, invalid-sort, empty-json-is-array).
- go build ./..., go vet ./..., full go test ./... all green.
- Manual binary smoke test: empty repo -> empty table/[]/0; seeded ref -> table+count+filter; --repo from a foreign cwd works.

DESIGN NOTES:
- Logic lives in testable internal/query package; cmd/query.go is a 3-line wrapper. Output writers injected via RunTo for testability.
- Filters AND together; unset filters skipped; nil pointer chains (orchestration/git_start) handled defensively. Refs with missing/unparseable session.json are skipped with a stderr warning, not fatal.
- Time filters: inclusive --since/--until on timing.started_at, RFC3339; unparseable times do not match range filters. Default sort started_at DESC, sessions without start time sort last; --reverse flips; --sort duration|session_id supported.
- --has-files matches both the full path glob and the basename, so *.go matches nested paths (friendlier than strict path.Match); documented in code.

RISKS/LIMITATIONS: shells out to git per ref (acceptable for current scale; no batching). No pagination. gofmt flags pre-existing cmd/main_test.go (not touched by this ticket).

VERDICT: Ship. Acceptance criteria met, behavior validated end-to-end.