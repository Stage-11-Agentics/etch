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

## 2026-06-07 — redaction-batch delegator (agent:redact-w0)

**Lattice auto-fires reviews on status transitions — and the auto code-review dies on diff resolution.** Bumping to `planned` auto-fires a plan-review per ticket; bumping to `review` auto-fires a code-review per ticket. The auto plan-review works (headless `claude -p`, no c11 panes). The auto code-review fails instantly with "Could not resolve diff automatically" because it runs from the root repo, not the worktree branch — AND the manual `lattice code-review --base origin/main` run from the worktree ALSO returned "Diff is empty" (LAT-219 root-repo routing applies to the diff computation too). If your work lives on a worktree branch, expect to use the own-reviewer fallback for code review; don't burn cycles retrying.

**`git restore bin/entire-agent-etch` before a validation demo runs the STALE binary.** The binary is checked in; `make build` updates it, and restoring it to keep the tree clean reverts to the pre-fix build. Symptom: validation demo shows secrets leaking that unit tests prove are redacted. Order of operations: build → demo → restore-before-commit. (Cost: one confusing demo run.)

**Boot-prompt status/role names may not exist in the live Lattice.** `in_validation` status and `--role validation` are both rejected by the current CLI (valid statuses end at `review` → `pr_open`; valid roles are plan-review/review/review-individual). Validation evidence attaches fine as `--type note --title "Validation evidence"` while the ticket sits at `review`.

## 2026-06-07 — Repo-root batch delegator (ETCH-34/35): Lattice harness drift vs boot-prompt assumptions

**What happened:** Three boot-prompt prescriptions didn't match the live Lattice instance, each costing a retry cycle: (1) `lattice status <T> in_validation` — no such status exists (valid set: backlog, in_planning, planned, in_progress, review, pr_open, done, blocked, needs_human, cancelled); validation evidence has to ride on `review` → `pr_open`. (2) `lattice attach --role validation` — only `plan-review`, `review`, `review-individual` are valid roles; use `--title "Validation evidence"` with no role instead. (3) Auto-fired code-reviews (status → review) run from the MAIN checkout and cannot resolve a feature-branch diff living in a worktree — ETCH-35's review died with "Could not resolve diff automatically", and even the prescribed worktree re-run with `--base origin/main` reported "Diff is empty". The own-reviewer fallback was the working path. Also: moving `planned → in_planning` (to re-cycle a failed plan review) requires `--force --reason`.

**Lesson:** Verify the live instance's status/role vocabulary (`lattice status <T> <bogus>` errors helpfully, listing the valid set) before relying on a boot prompt's lifecycle names — boot prompts encode the workflow author's memory of Lattice, not the installed version. For worktree-based batches, expect BOTH auto-fired and manual `lattice code-review` to fail diff resolution and budget for the self-review fallback up front. A second ticket in a batch gets its plan-review fired against its OWN stub plan file — either write a pointer-plan into every batch member's plan file BEFORE first transitioning to planned, or expect a vacuous FAIL and a forced re-cycle.

## 2026-06-07 — auto-capture lane (agent:autocap-w0)

**Hung auto-fired code-reviews burned 75 minutes.** `lattice status <T> review`
auto-fires a code-review subprocess; both ETCH-17/ETCH-20 reviewers stalled
(child `claude -p` at ~6s CPU after 75 min wall). Watch `lattice review-status`
with a hard timeout in mind: if elapsed exceeds ~15–20 min with no artifact,
check the child PID's CPU time (`pgrep -P <pid>`, `ps -o time`) — near-zero CPU
means hung, not thorough. Kill and go straight to the own-reviewer fallback;
don't wait politely.

**Entire's external-agent protocol is strict and silent.** Discovery rejects
any `entire-agent-*` binary whose `info` lacks `protocol_version: 1` — at
DEBUG log level, so a wrong info shape looks identical to "no plugin at all".
When integrating against Entire, pin every request/response shape to the
structs in `cmd/entire/cli/agent/external/types.go` at the exact installed tag
(`strings <binary> | grep` is a fast way to confirm a feature exists before
cloning source). Also: `entire agent add <ext>` never runs discovery on 0.6.3;
only `entire enable --agent <ext>` does.

**Claude Code native hook payloads carry no model field — anywhere.** The
model only exists inside the transcript JSONL (`message.model` on assistant
entries). Any design that assumes a model field in hook stdin will silently
produce null models. Empirical payload dumps (a hook that `cat >> file`s
stdin, then one real `claude -p` run) settle this class of question in
minutes — do that before trusting any doc or struct tag, including Entire's
(their `sessionInfoRaw` has a `model` tag that Claude Code never populates).

**This Lattice version's vocabulary differs from boot-prompt assumptions:**
no `in_validation` status (valid: backlog … pr_open, done, …), no
`validation` attach role (valid: plan-review, review, review-individual), and
`lattice attach` requires `--actor`. Check `lattice status <T>` error output
early instead of assuming the documented lifecycle.
## 2026-06-07 — Refspec batch: headless `lattice code-review` cannot resolve worktree diffs

**What happened**: All four auto-fired code-reviews (triggered by `lattice status <T> review`) died instantly with `Could not resolve diff automatically. Use --base <ref>` — they resolve against the root checkout (`code/Etch`, on `main`), not the delegator's worktree. The prescribed manual fallback (`cd <worktree> && lattice code-review ETCH-16 --mode single --base origin/main`) then reported `Diff is empty` even though the worktree branch had a real commit — the worktree carries a tracked, stale copy of `.lattice/`, and review diff resolution still landed somewhere HEAD == origin/main. ~15 minutes lost polling reviews that had already failed; `lattice review-status` kept showing the runs as in-flight (stale daemon state, no artifact) long after the process had exited.

**Fix applied**: Pivoted to the own-reviewer fallback per run instructions: self-reviewed the diff adversarially, attached a structured verdict (`lattice attach <T> --type note --role review --inline ...`), ran the fix loop against my own Major finding, attached a cycle-2 APPROVED verdict. The self-review found a real Major the implementation had missed (legacy etch-only push configs not healed), so the fallback carried genuine review value.

**For next time**: (1) Expect auto-fired reviews to fail for any worktree-based delegator — check the daemon log (`.lattice/.daemon/auto-code-review-<task>.log`) immediately after the transition instead of polling `review-status`, which reports stale in-flight state with no completion signal. (2) Budget for the own-reviewer fallback from the start. (3) Upstream fix worth filing: `lattice code-review` should resolve the diff in the invoking cwd (worktree) rather than the lattice root, and `review-status` should reap dead review processes.

## 2026-06-07 — Refspec batch: run-prompt lifecycle states don't all exist in the Lattice CLI

**What happened**: The run instructions said to use `lattice status <T> in_validation` and `lattice attach --role validation`. Neither exists: valid attach roles are `plan-review|review|review-individual`, and `in_validation` is not a status — the lifecycle goes `review → pr_open` directly. Worse, my grep-counted "success" check matched the word inside the *error* message, so four failed transitions read as four successes and the tickets sat in `in_progress` until the `pr_open` transition errored loudly.

**Fix applied**: Validation evidence attached as `--type note --title "Validation evidence"`; lifecycle walked `in_progress → review → pr_open`. Also `lattice cancel` doesn't exist but `lattice status <T> cancelled` works (used for ETCH-22).

**For next time**: Verify transitions with `lattice show <T> | grep Status:` after each bump — never by grepping the transition command's output for the status word, which appears in both success and error messages. Treat run-prompt CLI invocations as advisory; the CLI's own `Next:` hints are ground truth.

## 2026-06-07 — ETCH-41 delegator (agent:localonly-w2)

**What happened:** Rebased onto origin/main before implementation, but main moved AGAIN mid-implementation (PR #21/#22 merged while I was writing code). The code-review verdict was FAIL solely on integration — "branch is based on pre-#21 main" — even though the reviewer would have passed the implementation on its merits. Cost: one full review cycle plus a conflict-resolution rebase under review pressure.

**Lesson:** In a high-throughput run with parallel delegators, `git fetch origin && git rebase origin/main` immediately BEFORE `lattice status <ticket> review` — not just before implementation. Main moves on the scale of an implementation phase. A 30-second rebase before requesting review avoids a multi-minute review cycle spent on staleness, and lets the reviewer judge the merged reality (new schema fields, doc sections) instead of a vanished base.

**Also worth knowing:** the auto-fired code-review on `status review` worked this time despite the known "diff is empty" footgun — the reviewer noticed the supplied diff was defective and reviewed the real worktree branch on its own initiative. Don't assume the fallback is always needed; check the artifact before discarding the auto-review.

## 2026-06-07 — Lifecycle batch (agent:lifecycle-w1): two recurring footguns, one fixture pattern

**What happened (1):** `lattice code-review ETCH-40 --mode single --base origin/main` returned "Diff is empty" twice when invoked from the worktree — it resolves the repo via `LATTICE_ROOT` (the main checkout, sitting on `main`), so a worktree branch's diff is invisible no matter the cwd. Pivoted to the own-reviewer fallback per run rules; the structured self-review was genuinely useful (it had earlier caught a real bug — a D/F-conflict create failure misclassified as ErrRefExists — via the commit-failure injection test).

**What happened (2):** Main moved TWICE during the batch (PR #20 pre-impl; #21/#22/#23 mid-flight, landing in my exact files: recovery.go, commit.go, writer.go). The pre-PR rebase rule from the ETCH-41 lesson held up — rebasing immediately before review/PR cost ~20 minutes of careful conflict resolution but produced a clean composition (ETCH-41's strip-before-push projection folded INTO the new unified commitRecord, so crash recovery inherited it for free).

**Fixture pattern worth knowing:** the recovery scan is now stat-first — idleness is judged on wip file **mtime**, not event timestamps. Any test that plants an "old" wip must `os.Chtimes` it to match; writing old `ts` fields with a fresh file silently reads as a live session. Two upstream fixtures (density, local-only) needed this; future tests that fabricate orphaned wips will too.

**For next time:** Don't bother re-running `lattice code-review` from a worktree expecting cwd to matter — go straight to the fallback (or fix the tool: it should resolve the diff in the invoking cwd). And when a sibling lane's PR touches your files mid-flight, read their PR body for the *contract* (e.g. "local refs converge by overwrite") before resolving conflicts — the comment threads encode invariants the diff alone doesn't.

## 2026-06-07 — Orchestrator closeout audit (run: backlog completion)

**Time-bomb tests: fixed clocks + real binaries don't mix.** Two archive tests mixed a hardcoded `fixedNow` with the built binary's `time.Now()` — green for 5 days after authoring, then red on clean main forever after. If a test invokes the real binary, fabricate dates relative to the real clock (`daysAgoReal`); reserve `fixedNow` for library calls that accept an explicit `Now`.
- **Routed to:** lessons-learned.md (fixed at 01a2ca4).

**Auto-fired lattice reviews are this install's #1 friction source.** Status transitions auto-fire plan/code reviews; the code-review path cannot resolve worktree-branch diffs (fails or hangs — one hung 75 min at near-zero child CPU). Every delegator independently hit this. The own-reviewer fallback is the working path; check hung children via `ps -o time` and kill, don't wait. ETCH-42 (auto-filed failure ticket) was this same bug; cancelled with root-cause note.
- **Potential fix:** Lattice repo — code-review worktree↔root bridge (LAT-219 family) + a no-auto-review-on-orchestrator-transitions option. Proposed as Lattice issue.

**Orchestrator status walks fire side effects.** Walking a tracking ticket backlog→done at closeout auto-fires reviews on each transition (one stray reviewer pid to kill). A `lattice complete --force` for tracking-only tickets would remove the noise.
- **Routed to:** proposed Lattice issue.

**c11 PTY wedge mid-run is survivable without restart.** When new-surface PTY init starts failing (created in tree but "Terminal surface not found"), existing surfaces keep working — reuse idle/freed surfaces by sending the launch command into them. Three dispatches completed that way this run.
- **Routed to:** lessons-learned.md; c11 repo already tracks the wedge.
