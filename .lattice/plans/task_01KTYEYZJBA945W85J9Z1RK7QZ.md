# ETCH-48: etch enable: worktree stamping + post-checkout self-propagation + dedupe

Operator-mode enablement, part 2 of 2. Builds on ETCH-47. Full design: docs/ENABLEMENT.md (the spec).

WHAT: 'etch enable' additionally (a) stamps guarded hook entries into .claude/settings.local.json of the repo root and EVERY existing worktree (git worktree list --porcelain; merge politely with existing local settings, byte-preserve foreign entries like install.go does); (b) installs a marker-delimited post-checkout git hook into the EFFECTIVE hooks dir (honor core.hooksPath) that stamps any new worktree at creation — idempotent, fast-exits when already stamped, chains politely with pre-existing post-checkout content; (c) 'disable' best-effort removes stamps + the post-checkout block.

DEDUPE (ships in this ticket, not later): stamped commands embed the committed-entries-win guard — if grep -qs entire-agent-etch .claude/settings.json then exit 0 — so a branch that merges committed hooks never double-captures. Exact command shape is in ENABLEMENT.md.

ALSO: extend etch doctor scope (ETCH-46) with checks: config key state, per-worktree stamp coverage, hooksPath mismatch. Update README + docs/HOOK_CONTRACT.md cross-refs; the platform doc (code/platform/etch.md, separate repo) Worktrees section should be updated by the implementer to drop the interim-recipe framing once this ships.

ACCEPTANCE (edge cases enumerated in ENABLEMENT.md): (1) worktree created AFTER enable captures with zero manual steps — the headline test; (2) worktree whose branch merges main's committed hooks → exactly one capture per event; (3) repo with custom core.hooksPath → propagation works or doctor flags it; (4) install-hooks (team mode) on an operator-mode repo → no double capture; (5) tests via testutil temp repos with real git worktree add.
