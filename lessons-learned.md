# Lessons learned — Etch

Append-only log. Every failure, point of confusion, or thrash gets an entry. The point is to make the next agent or session pay less for the same problem.

**Format per entry:**
- `## YYYY-MM-DD — <short title>` (one-line header)
- **What happened**: factual one-paragraph
- **Why it bit**: the root cause, not just the symptom
- **Fix applied** (if any): what was done in this run
- **For next time**: what should change in scripts, skills, or process

---

## 2026-05-26 — Phase 0 PoC is Python, production must be Go

**What happened**: The Phase 0 proof-of-concept `entire-agent-cairn` binary was built as a Python script for speed of validation. Entire's plugin ecosystem is Go-native and the production binary must be Go to avoid a Python runtime dependency.

**Why it bit**: Not a failure — intentional design choice. But worth recording so no future agent tries to extend the Python PoC instead of building the Go replacement.

**Fix applied**: Phase 0 PoC stays as `./entire-agent-cairn-poc` (Python) for reference. Ticket ETCH-1 builds the Go replacement.

**For next time**: When building PoCs for validation gates, name them distinctly (e.g., `entire-agent-cairn-poc`) to avoid confusion with the production artifact.

## 2026-05-27 — `lattice review-status` polling loops stall inline-full delegators

**What happened**: 2 of 3 inline-full delegators (ETCH-2, ETCH-7) launched headless `lattice plan-review` or `lattice code-review`, then polled `lattice review-status` in a `while true; do ... sleep 15; done` shell loop. The headless CLI hung indefinitely; the delegator's Claude session blocked on the shell command; cost counter froze; the session stalled until the orchestrator sent a recovery nudge via `c11 send`.

**Why it bit**: The delegator boot prompts specified `timeout 600` on the headless command but also left room for the delegator to interpret the review flow however it saw fit. Both ETCH-2 and ETCH-7 chose to launch the command in background and poll `review-status` — a pattern that circumvents the timeout. The Lattice CLI's worktree-to-root bridge for code-review (and sometimes plan-review) remains fragile; it hangs rather than failing cleanly.

**Fix applied**: Orchestrator sent `c11 send` recovery nudges with explicit instructions to ctrl+c and use the own-reviewer fallback. Both delegators recovered within one tick. ETCH-8's boot prompt was hardened with an explicit "HARD RULE: do NOT poll lattice review-status" warning.

**For next time**: Every inline-full boot prompt must include: (1) `timeout 600` wrapping the headless command directly — not as a separate concern, (2) an explicit prohibition on `lattice review-status` polling loops, (3) the own-reviewer fallback procedure inline. The lattice-orchestrator skill's "HARD RULE: `lattice code-review` 600-second timeout" section already documents this but delegators still independently reinvent the polling pattern. The prohibition needs to be louder in the boot prompt template.

## 2026-05-27 — Additive schema type naming conflict on parallel squash-merges

**What happened**: ETCH-5 and ETCH-6 both added types to `internal/schema/session.go` with different naming conventions (ETCH-5: `Agent`, `Timing`, `Machine`; ETCH-6: `SessionAgent`, `SessionTiming`, `SessionMachine`). After ETCH-4 and ETCH-5 merged to main, ETCH-6's branch became unmergeable. Rebase produced 6 conflict blocks in session.go.

**Why it bit**: Wave 2 tickets were intentionally parallel and independently designed their schema types. The schema package was a shared surface with no upfront type-naming convention. Each delegator named types based on what seemed natural in isolation.

**Fix applied**: Orchestrator took HEAD's (main's) version of session.go and fixed ETCH-6's trace.go + trace_test.go to reference the correct types (pointer fields, unprefixed names). Added `strPtr` helper lost during rebase. All tests passed after resolution.

**For next time**: When multiple Wave 2 tickets will add types to the same package, the BUILDPLAN.md (or the Wave 1 scaffold) should establish the naming convention. For Etch, ETCH-1 could have included a types-only `session.go` with the full struct skeleton and `// TODO: implement` field comments. Parallel tickets would then fill in implementations without conflicting on type names.

## 2026-05-27 — Forgejo API "Please try again later" on rapid sequential merges

**What happened**: After merging PR #4 (ETCH-4), immediately attempting to merge PRs #3 and #2 returned `{"message":"Please try again later"}`. PR #3 re-merged on retry; PR #2 needed a rebase because main had moved.

**Why it bit**: Forgejo's merge endpoint has a brief lock/recompute window after each squash-merge. Sequential merge attempts within ~1-2 seconds hit this window.

**Fix applied**: Retried PR #3 after a brief pause (succeeded). PR #2 required a full rebase (main had diverged with the schema conflict).

**For next time**: When auto-merging multiple PRs in the same tick, add a 5-second pause between merge API calls, or merge sequentially with a mergeability re-check between each. Don't fire all merge calls in parallel.

## 2026-05-28 — `git stash` silently rewound 8 Lattice tickets to backlog

**What happened**: Between the original run completing and the Wave 5 dispatch, the orchestrator ran `git stash` to clean up an in-flight branch. The stash captured uncommitted modifications to `.lattice/events/*.jsonl` and `.lattice/tasks/*.json` for ETCH-1 through ETCH-8 — the complete event history of all 8 squash-merged tickets, including every `in_planning → planned → in_progress → review → pr_open → done` transition. Because Lattice rebuilds ticket state from the on-disk event log on every read, the post-stash `lattice list` reported ETCH-1 through ETCH-8 as `backlog` — matching the only events that had actually been committed to git (the Phase 2 ticket-creation events). The operator caught it on the Lattice Board ("a bunch of these items are in backlog. I think Lattice is out of sync with the codebase").

**Why it bit**: `.lattice/events/` and `.lattice/tasks/` are checked into git, and the project's git policy doesn't auto-commit Lattice transitions. The orchestrator's `lattice status` / `lattice complete` calls during the run wrote to the working copy of these files but those writes were never committed. From git's perspective they were uncommitted local modifications — which is exactly what `git stash` captures. The stash was applied implicitly during a branch swap, and the working copy reverted to whatever was on the committed history of `main` (which was the Phase 2 snapshot showing every ticket at `backlog`).

**Fix applied**: Inspected the stash with `git stash show`, confirmed it held the missing event entries, then restored only the `.lattice/events/`, `.lattice/tasks/`, and `.lattice/plans/` files from the stash via `git checkout stash@{0} -- <paths>`. Lattice immediately reported correct state. The stash was then dropped.

**For next time**: Two safeguards. (1) **The orchestrator commits Lattice state to git after every meaningful status change** — at minimum after `lattice complete` and at every closeout. A single `git add .lattice/ && git commit -m "Lattice state checkpoint"` per merge would have prevented the rewind. (2) **Before running `git stash`, always check `git status` for `.lattice/` modifications.** If any appear, either commit them first or restore them explicitly after the stash. Better: avoid `git stash` entirely when the working tree has Lattice state — use `git checkout -- <specific-paths>` to scope the cleanup. The same risk applies to any branch swap when `.lattice/` has uncommitted modifications. Consider proposing a Lattice CLI option to write a sentinel file or auto-stage on every event so accidental `git stash` doesn't hide the state.
## 2026-05-28 — ETCH-13: Hook subcommands are underscore_case, not hyphen-case

**What happened**: Writing the smoke test, I drove the binary with hyphenated
subcommands (`session-start`, `session-end`, ...) by analogy to the `info`-style
names. Every call exited **1** with `unknown subcommand: session-start` — but a
naive smoke harness that only checks for a created ref (not per-call exit codes)
would have reported "no ref" with no hint why. The real dispatch names are
underscore_case: `session_start`, `user_prompt_submit`, `pre_tool_use`,
`post_tool_use`, `session_end`, `stop` (see `cmd/entire-agent-cairn/main.go`).

**Fix applied**: smoke.sh uses the underscore names and asserts each hook call
exits 0, so a future name drift fails loudly at the offending step.

**For next time**: the stdin envelope is the `parsehook.HookInput` shape
(`session_id`, `raw_data.model`, `user_prompt`, `tool_name`, `tool_use_id`,
`tool_input`), not a flat `{cwd, agent, model}`. Model arrives via `raw_data.model`.
When simulating sessions, mirror `test/density/density_test.go::runFullSession`.
