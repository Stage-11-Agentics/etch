# Plan Review: ETCH-20

## 1. Verdict

**FAIL (plan-level)**

## 2. Summary

I reviewed the plan for ETCH-20 (undocumented hook-event JSON contract + silent dropping of unrecognized fields). The submitted "plan" (`.lattice/plans/task_01KSTXJ6XBQQJREX5D9X06G6PX.md`) is a **verbatim copy of the bug description** — it contains zero implementation content: no approach, no file list, no decision on the open design question the task itself raises, and no test plan. The event log confirms the planner moved `in_planning → planned` in ~2.5 minutes (14:52:21 → 14:54:58) without ever updating the plan file (mtime unchanged from task creation on May 29). There is, in effect, nothing to implement against.

## 3. Issues

**[CRITICAL] Entire plan — No implementation plan exists; the document is a copy of the task description**
The plan file is identical to the `description` field on the task. It restates the problem and ends with "Recommendation: document the event payload contract in README/OUTPUT_SPEC, and/or warn when expected fields are absent." A recommendation is not a plan. None of the review dimensions (completeness, feasibility, alignment, risk, acceptance criteria, architecture) can be evaluated because no concrete steps, files, or decisions are specified. This must return to `in_planning`.
**Recommendation:** Author a real plan. At minimum it must (1) resolve the open design question below, (2) enumerate files to change, (3) specify the documentation content, and (4) define tests. Concrete skeleton:
- **Decision — doc-only vs. doc + warn:** The task phrases the fix as "document … *and/or* warn." This is the central scope decision and the plan must pick one. Recommended: **do both** — docs are the durable fix; a stderr warning closes the silent-failure footgun that cost the QA agent time in the first place. If scope must be minimal, doc-only is defensible, but the plan must say so explicitly rather than leave "and/or" unresolved.
- **Documentation (the core deliverable):** Add an input event-payload contract section. The authoritative field set already exists in code at `internal/parsehook/parsehook.go` (`hookInput`/`rawData` structs) and is exercised in `scripts/smoke.sh:119-120`. Document, per hook type, the exact JSON keys etch reads: `session_id`, `timestamp`, `session_ref`, `raw_data.model` (session_start), `user_prompt` (user_prompt_submit), `tool_name`/`tool_use_id`/`tool_input` (pre/post_tool_use). Include a copy-pasteable example payload for each hook. Decide the home: README's hook section (`README.md:75`) names the hooks but gives no fields; OUTPUT_SPEC documents the *output* record, not the *input* contract — so the input contract likely belongs in README (or a new section) with a cross-link. State which file(s) and where.
- **Warning behavior (if chosen):** Specify exactly when to warn. The natural signal is per-hook: e.g., `session_start` with `raw_data.model` absent/empty, or `user_prompt_submit` with empty `user_prompt`. Specify the channel (stderr, not stdout — stdout carries the parsed JSON result), the message format, and critically that **exit code stays 0** and stdout output is unchanged (Entire's hook dispatch and downstream capture must not break). Note the false-positive risk: some hooks legitimately carry empty fields, so warn only on the fields each hook type is expected to populate.

**[MAJOR] Entire plan — No files identified for change**
The project conventions (CLAUDE.md "Does the plan identify which files will be created or modified?") and the ticket-per-test rule require this. The plan names none.
**Recommendation:** List them explicitly. Likely set: `README.md` (and/or `OUTPUT_SPEC.md`) for docs; `internal/parsehook/parsehook.go` if warnings are added; a new/updated test in `internal/parsehook/` (no `parsehook_test.go` currently exists — see below).

**[MAJOR] Entire plan — No test plan, violating the project's mandatory per-ticket testing rule**
CLAUDE.md is unambiguous: "Every ticket ships with tests. No exceptions." There is currently no `internal/parsehook/parsehook_test.go`. A behavioral change (warnings) with no test is non-compliant; even a doc-only change should add a regression guard that locks the documented field names to actual parser behavior so the docs can't silently drift again.
**Recommendation:** Specify tests. If warnings are added: table test feeding wrong-field payloads (e.g., top-level `{"model":...}`, `{"prompt":...}`) and asserting (a) a warning is emitted to stderr, (b) exit 0, (c) stdout JSON is byte-identical to today's behavior; plus correct-field payloads asserting no warning and correct extraction. A doc-drift guard test that asserts the documented field names match the `hookInput` struct tags is a strong, low-cost addition.

**[MINOR] Entire plan — Does not reconcile with the existing source of truth (`scripts/smoke.sh`)**
The smoke script (`:119-120`) already encodes the correct payloads and is the de-facto contract. The plan should reference it as the authoritative example source so documentation and the script stay in sync, rather than hand-authoring divergent examples.
**Recommendation:** Have the plan note that documented examples must match `scripts/smoke.sh`, and ideally that the smoke test continues to pass as a contract check.

## 4. Positive Observations

The strength here is upstream of the plan, in the **bug report itself**: the QA agent did exemplary diagnostic work — it isolated the silent-drop behavior, identified the exact wrong-vs-correct field names (`model` → `raw_data.model`, `prompt` → `user_prompt`), located the authoritative source (`scripts/smoke.sh`), and correctly concluded "the binary is correct; the gap is documentation + silent dropping." That is a clean, root-caused report that hands the planner everything needed. The failure is purely that the planning step never converted this excellent diagnosis into an actionable plan — the raw material for a strong, fast plan is all present and just needs to be written down with the open `and/or` decision resolved.
