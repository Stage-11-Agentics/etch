# ETCH-47: etch enable/disable — git-config enablement switch + fast-exit guard

Operator-mode enablement, part 1 of 2. Full design: docs/ENABLEMENT.md (read it first — it is the spec).

WHAT: New subcommands 'enable' and 'disable'. 'enable' sets git config etch.enabled=true (common dir — shared by all worktrees/branches, never pushed) and writes the .etch ignore entries (.etch/* with !.etch/settings.json carve-out, plus .claude/settings.local.json) into .git/info/exclude. 'disable' sets etch.enabled=false. Every hook entrypoint gets a fast-exit guard: not a git repo, or etch.enabled=false → exit 0 before any other work.

COMPATIBILITY RULE: a repo with committed hooks and NO config key keeps capturing (team mode must not require 'etch enable'). Key absent = enabled when dispatched; etch.enabled=false is an explicit off-switch that wins over everything.

ACCEPTANCE: (1) enable → key set + excludes written, idempotent on rerun; (2) disable → all capture stops in main checkout AND worktrees, both dispatch modes; (3) team-mode repo with no key captures unchanged; (4) disabled-path latency within SPEC AC #13 budget — measure it; (5) tests via testutil temp repos incl. worktree cases.
