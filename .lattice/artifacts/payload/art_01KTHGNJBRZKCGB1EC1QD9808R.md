# Validation evidence — CLI/docs batch (commits 936d873 + 4cf8dac)

- `go test ./...` — all packages pass (incl. new TestHelpSubcommands, TestListedSubcommandsAreDispatched, extended TestUnknownSubcommand/TestNoSubcommand).
- `make build` — green.
- `make smoke` — "SMOKE PASSED — install + capture story verified end-to-end."
- `go run ./cmd/entire-agent-etch --help` — prints 4-section listing (Session commands / Install & protocol / Hook entry points / Stubs), all 21 dispatched subcommands with one-line descriptions and arg synopses, aligned columns.
- Exit/stream contract demoed: help/--help/-h → stdout exit 0; bare → listing on stderr exit 1; unknown subcommand → hint at 'entire-agent-etch help'.
- Every README example executed in a temp git repo with a hook-captured session: query (--repo/--runtime/--ticket/--status/--since/--until/--branch/--has-files/--json/--count/--exit-reason/--run-id/--sort/--reverse/--no-index), index build/update/show/drop, archive --dry-run / real / --threshold-days/--quarter, restore-archive; full archive → restore round-trip verified (ref moved to refs/etch/archive/2026-Q2 and back).
- bin/entire-agent-etch NOT committed (rebuilt locally post-restore before demos).