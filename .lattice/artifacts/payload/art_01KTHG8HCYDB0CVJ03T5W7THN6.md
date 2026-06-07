# Plan Review: ETCH-19

### 1. Verdict

**PASS** — Plan is complete, feasible, and aligned. Implementation can proceed.

### 2. Summary

I reviewed the plan covering ETCH-19 (README feature truth) batched with ETCH-21
(subcommand discovery). I verified every factual claim in the plan against the
actual source: the command table, the `query` flags (`--runtime`, `--ticket`,
`--status`, `--since`/`--until`, `--json`, `--count`, `--repo`), the `index`
actions (`build`/`update`/`show`/`drop`), the `archive --dry-run` flag, and
`restore-archive <ULID>` are all real and match the dispatch switch in
`cmd/entire-agent-etch/main.go`. The plan is accurate and low-risk; the only
thing worth flagging is that it expands scope beyond ETCH-19 by folding in
ETCH-21, which is a deliberate and well-justified batch rather than a defect.

### 3. Issues

**[MINOR] Whole plan — Scope batches ETCH-21 into an ETCH-19 review**
The task under review is ETCH-19 (a docs-only update to README). The plan adds
ETCH-21 (a real code change: new `usage.go`, dispatch changes in `main.go`, new
tests). This is coherent — the README step explicitly references
`entire-agent-etch help`, which is ETCH-21's deliverable, so the two are coupled
and shipping them together avoids documenting a command that doesn't exist yet.
The risk is purely process: a reviewer scoped to ETCH-19 alone might not expect
code/test changes, and a regression in the `main.go` dispatch path (a hook entry
point) is higher-blast-radius than a README edit. It matters because the two
tickets have different rollback profiles.
**Recommendation:** Keep the batch, but make the ETCH-21 commit and the ETCH-19
commit separate within the single PR so each can be reverted independently, and
land ETCH-21's dispatch change first so the README's `help` reference is never
ahead of the binary. No plan rework required.

**[MINOR] ETCH-19 §2 — `archive` has no `--repo` flag (don't let README imply one)**
`query` and `index` both take `--repo PATH`, but `runArchive`/`runRestoreArchive`
operate on `os.Getwd()` and accept no `--repo` flag (verified in
`cmd/entire-agent-etch/archive.go`). The plan's example text already gets this
right — it shows `archive --dry-run` / `archive` *without* `--repo` while showing
`query --repo .` — so this is a guardrail, not a correction.
**Recommendation:** When writing the README examples, keep `--repo .` on `query`
and `index` only; show `archive` / `restore-archive` as run from inside the repo
(`cd repo && entire-agent-etch archive --dry-run`). The plan's "execute every
README example command in a temp repo" validation step will catch any drift here
— make sure that validation actually `cd`s into the temp repo for the archive
examples.

**[MINOR] ETCH-21 §4 — "every name from a canonical subcommand list" needs a guard against silent drift**
The test asserts that `help` output contains every name from a canonical list
"cross-checked against the dispatch switch." There's no compile-time link between
the `usage.go` table, the test's canonical list, and the actual `switch` in
`main.go` — all three are hand-maintained string sets. A future subcommand added
to the switch but not the table would pass tests silently (the test only checks
that listed names appear, not that all dispatched names are listed).
**Recommendation:** Have the test derive its expectation from the `usage.go`
table itself, and add a single assertion that every non-stub, non-hook name in
the table is reachable (e.g. invoking `<name> --help`/bad-args returns something
other than "unknown subcommand"). This makes the table the single source of
truth and turns "forgot to document a new command" into a test failure. Optional
but cheap insurance for a discovery feature whose whole value is completeness.

### 4. Positive Observations

- **Every factual claim checks out.** I verified the command groupings, the
  stub list (`extract-all-modified-files`, `calculate-total-tokens`), the hook
  entry points, and the capability subcommands against `main.go` line-by-line —
  the plan's table matches the dispatch switch exactly. This is the kind of
  pre-grounded plan that doesn't send the implementer chasing phantom flags.
- **Backward-compat awareness is explicit and correct.** The plan calls out
  preserving `TestNoSubcommand` (bare invocation → stderr, exit 1) and keeping
  the unknown-subcommand error path. I confirmed both tests exist and that the
  proposed stdout/stderr + exit-code split satisfies them. Routing `help`/`-h`/
  `--help` to stdout+exit 0 while keeping bare-invocation on stderr+exit 1 is the
  right Unix convention and the right way to avoid breaking the existing test.
- **Additive-only discipline on the README.** Explicitly scoping the change to
  "do not disturb the recently merged redaction / refspec / auto-capture
  sections" and flagging the stale `archive_threshold_days` (ETCH-11) config note
  for cleanup shows the planner read the current README rather than rewriting
  from memory.
- **Validation is concrete and executable.** `go test ./...`, `make build`,
  `make smoke`, `--help` invocation, and running every README example in a temp
  repo — this is a real validation loop, not "looks good to me," and it will
  catch the `--repo` and table-completeness concerns above if executed faithfully.
