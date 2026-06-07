# Plan — CLI/Docs UX batch: ETCH-21 (subcommand discovery) + ETCH-19 (README feature truth)

One PR off `origin/main`, branch `docs/cli-discoverability`, worktree `Etch-worktrees/cli-docs`.

## ETCH-21 — subcommand discovery

`cmd/entire-agent-etch/main.go` currently prints a bare one-line usage with no
subcommand listing, and `help`/`--help`/`-h` fall through to "unknown subcommand"
(exit 1).

1. Add a `usage.go` with a structured command table — `[]struct{name, args, desc}`
   grouped into sections: **Operational** (query, index, archive, restore-archive,
   setup-refspec), **Install / capability** (info, detect, install-hooks,
   uninstall-hooks, are-hooks-installed, parse-hook, extract-modified-files,
   calculate-tokens), **Hook entry points** (session_start, session_end,
   user_prompt_submit, stop, pre_tool_use, post_tool_use), **Stubs**
   (extract-all-modified-files, calculate-total-tokens).
2. `printUsage(w io.Writer)` renders the table deterministically (stable order,
   aligned columns) so tests can assert on it.
3. Dispatch changes:
   - bare invocation → full listing to **stderr**, exit 1 (preserves
     `TestNoSubcommand`).
   - `help`, `--help`, `-h` → full listing to **stdout**, exit 0.
   - unknown subcommand → keep error, add "run 'entire-agent-etch help'" hint.
4. Tests in `main_test.go`: help/-h/--help exit 0 and contain every name from
   a canonical subcommand list (cross-checked against the dispatch switch);
   bare invocation exits non-zero but stderr contains the listing.

## ETCH-19 — README feature truth

README Status still says query/index/archive are "in progress" and Usage says
"Richer querying is coming." All three shipped in v0.01.001.

1. **Status** — fold query/index/archive into the shipped bullet; drop the
   "In progress" framing for them.
2. **Usage** — replace the "Richer querying is coming" paragraph with a real
   "Query, index, archive" subsection: `entire-agent-etch query --repo .`
   (+ key filters: --runtime, --ticket, --status, --since/--until, --json,
   --count), `index build|update|show|drop`, `archive --dry-run` /
   `archive`, `restore-archive <ULID>`.
3. Mention `entire-agent-etch help` as the discovery entry point (new in this PR).
4. Fix any remaining stale "(ETCH-11)" / "coming" claims (e.g. the
   `archive_threshold_days` config note). Additive only — do not disturb the
   recently merged redaction / refspec / auto-capture sections.

## Validation

`go test ./...`, `make build`, `make smoke`; run `go run ./cmd/entire-agent-etch --help`;
execute every README example command in a temp repo.

---

*(Original ticket description below, preserved.)*

README Status says 'etch query (ETCH-9), etch index (ETCH-10), etch archive (ETCH-11) are in progress' and Usage says 'Richer querying is coming.' But all three are fully functional in v0.01.001: 'entire-agent-etch query' prints a session table (with --runtime filter), 'index build/update/show' build and report a real index, 'archive' runs ('nothing to archive' on fresh sessions). A naive user trusting the docs won't even try these commands. Update README to document query/index/archive as available, with usage examples and flags.
