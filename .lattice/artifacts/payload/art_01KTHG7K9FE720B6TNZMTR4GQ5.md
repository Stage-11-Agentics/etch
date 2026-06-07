# Plan Review: ETCH-21

## 1. Verdict

**FAIL (plan-level)**

## 2. Summary

I reviewed the proposed plan for ETCH-21 (add subcommand discovery to `entire-agent-etch` for bare invocation and `-h`/`--help`/`help`). The "Plan" section is a verbatim copy of the task description — it contains zero implementation content: no approach, no files identified, no list of which subcommands to surface, no test strategy, and no decisions on the genuine design questions this feature raises. There is effectively no plan to review, and the underlying task has real decisions (which subcommands are user-facing, exit codes, stdout vs stderr) that an implementer would otherwise resolve by guessing. The task itself is small and high-value, but it must return to planning with an actual plan.

## 3. Issues

**[CRITICAL] Plan (lines 17–19) — The plan is just the task description restated**
The entire "Plan" body is identical to the "Task Description." It does not describe *how* the change will be made: no mention of `cmd/entire-agent-etch/main.go` (the file that owns `main()` and the subcommand `switch`), no help-text data structure, no rendering function, no tests. A plan that restates the problem provides nothing to evaluate for feasibility or completeness and gives the implementer no guardrails.
**Recommendation:** Rewrite the plan to specify: (a) the file(s) to modify — `cmd/entire-agent-etch/main.go`, plus README and tests; (b) the mechanism — e.g. an ordered list/table of `{name, description}` and a `printUsage()` helper invoked from `main()`; (c) how the dispatch switch hooks into it.

**[CRITICAL] Plan — No decision on *which* subcommands to list**
The binary currently dispatches ~20 subcommands across distinct audiences: user-facing (`info`, `query`, `index`, `setup-refspec`, `archive`, `restore-archive`), install/setup (`detect`, `install-hooks`, `uninstall-hooks`, `are-hooks-installed`), internal Entire **hook handlers** (`session_start`, `session_end`, `user_prompt_submit`, `stop`, `pre_tool_use`, `post_tool_use`, `parse-hook`), capability helpers (`extract-modified-files`, `calculate-tokens`), and unimplemented **stubs** (`extract-all-modified-files`, `calculate-total-tokens`). The task description names only five (`info/setup-refspec/query/index/archive`) as the discovery target. Dumping all 20 — including hook handlers an end user never types and stubs that error — would be noise and is arguably worse UX than the status quo. The plan makes no decision here.
**Recommendation:** Decide and document the grouping. Recommended: list user-facing + setup commands with one-liners; either omit hook handlers and stubs or place them under a clearly-labeled "Internal (invoked by Entire)" group. Stubs should not be advertised as usable.

**[MAJOR] Plan — One-line descriptions are not authored**
The acceptance target is "a subcommand list with one-line descriptions," yet the plan supplies none of the descriptions and doesn't say where they live. Writing accurate one-liners is the actual substance of this ticket.
**Recommendation:** Enumerate each surfaced subcommand with its one-line description in the plan, so the reviewer/implementer agree on wording (e.g. `info — print plugin metadata for Entire discovery`, `query — search captured session records`, `setup-refspec — configure git refspec for fetching session refs`).

**[MAJOR] Plan — No spec for exit code and stream (stdout vs stderr)**
Today, bare invocation writes usage to **stderr** and exits **1** (`main.go:16–19`); `--help`/`help`/`-h` fall through to `default` → "unknown subcommand" on stderr, exit **1**. Conventional CLI behavior is: explicit `--help`/`-h`/`help` prints to **stdout** and exits **0** (help was requested, not an error), while bare invocation with no args commonly still exits non-zero (misuse) but should now print the command list. The plan addresses none of this, and the distinction is testable behavior.
**Recommendation:** Specify exactly: `-h`/`--help`/`help` → print list to stdout, exit 0. Bare invocation (no args) → print list (stdout or stderr — pick one) and choose exit code deliberately (0 for friendliness, or 1 to preserve "missing required arg" semantics). State the decision so tests can assert it.

**[MAJOR] Testing philosophy — No test plan despite a mandatory-tests policy**
CLAUDE.md states "Every ticket ships with tests. No exceptions," and `main_test.go` already exists. The plan defines no tests.
**Recommendation:** Add test cases to `cmd/entire-agent-etch/main_test.go` (or equivalent) asserting: bare/`-h`/`--help`/`help` each emit a list containing known subcommand names; correct exit code per the decision above; correct stream. Note that `main()` calls `os.Exit`, so the plan should describe the test seam (extract a testable `run()`/`printUsage()` rather than testing `main()` directly, or invoke the built binary as a subprocess as the testutil pattern does).

**[MINOR] Plan — `-h` vs `--help` inconsistency in the source requirements**
The task *title/expected* line says "bare/-h/--help/help" but the description body only lists `--help` and `help`. The plan should resolve all four trigger forms explicitly (`-h`, `--help`, `help`, and bare) so none is silently dropped.
**Recommendation:** Enumerate the exact trigger set the implementation must handle and add a test per trigger.

**[MINOR] Plan — Documentation/keeping-in-sync not addressed**
A hand-maintained subcommand list drifts from the dispatch `switch` as new subcommands are added. The plan should at least note the maintenance expectation, and ideally README should reflect the new discoverability.
**Recommendation:** Keep the description table adjacent to the dispatch switch (same file) with a comment tying them together, and update README's command reference if one exists.

## 4. Positive Observations

The *task* is well-scoped and genuinely high-value: first-run discoverability is a real UX gap, the fix is low-risk and self-contained to `main.go`, and there are no dependency, migration, or backward-compatibility hazards beyond the exit-code decision. The codebase is well-suited to the change — a single dispatch point in `cmd/entire-agent-etch/main.go` and an existing `main_test.go` make both the implementation and its tests straightforward. The failure here is purely that the plan was not written, not that the work is hard; a short revision enumerating surfaced subcommands, descriptions, exit/stream behavior, and tests will make this an easy PASS.
