# Plan Review: ETCH-1 — Go module + binary scaffold

## 1. Verdict

**PASS** — Plan is complete, feasible, and aligned. Implementation can proceed.

## 2. Summary

Reviewed the ETCH-1 implementation plan against the BUILDPLAN ticket definition, SPEC.md acceptance criteria, PHASE0_RESULTS.md protocol details, and OUTPUT_SPEC.md schema. The plan is well-structured for a greenfield scaffold: clear file layout, sensible package decomposition, and a thorough test plan. Minor gaps exist around testutil helper completeness and parse-hook input/output specificity, but none block implementation.

## 3. Issues

**[Minor] Test plan — testutil helper coverage is underspecified**
The project's CLAUDE.md testing philosophy prescribes four helpers to build in ETCH-1: `NewTestRepo()`, `SimulateHookEvent()`, `ReadSessionRef()`, and `MustValidateSchema()`. The plan's test items (8-9) only verify `NewTestRepo` and `RunBinary` (a variant of `SimulateHookEvent`). `ReadSessionRef` and `MustValidateSchema` reference concepts (session refs, full schema) that don't materialize until ETCH-2/3, so deferring them is reasonable — but the plan should acknowledge this deviation explicitly rather than leaving it implicit.
**Recommendation:** Add a note to the plan stating which testutil helpers ship in ETCH-1 (NewTestRepo, SimulateHookEvent/RunBinary) and which are stubs or deferred (ReadSessionRef, MustValidateSchema) to be fleshed out in their respective tickets. This keeps the CLAUDE.md contract visible.

**[Minor] parse-hook section — input/output format underspecified**
The plan says "Reads stdin JSON, extracts `--hook` flag from args. Maps hook name to event fields." but doesn't specify the exact stdin structure Entire sends or the exact JSON output parse-hook must return. PHASE0_RESULTS.md documents the Event fields per hook type (e.g., `session_start` provides `session_id`, `session_ref`, `timestamp`, `model`; `pre_tool_use` provides `tool_name`, `tool_use_id`, `tool_input`). The implementer needs this mapping to get the output right.
**Recommendation:** Either inline the hook→fields mapping table in the plan or explicitly reference PHASE0_RESULTS.md §"Protocol overview" as the source of truth for parse-hook's per-hook output fields. The current description is close but could be misinterpreted.

**[Minor] info subcommand — capabilities not enumerated**
The plan says "capabilities map" without listing the specific capability declarations from PHASE0_RESULTS.md (`hooks: true`, `transcript_analyzer: true`, `compact_transcript: false`, `token_calculator: true`, `text_generator: false`, `hook_response_writer: false`, `subagent_aware_extractor: true`). Listing them prevents the implementer from guessing.
**Recommendation:** Enumerate the capabilities JSON in the plan or cite the exact section of PHASE0_RESULTS.md. This is a static structure — having it in-plan costs little and eliminates ambiguity.

## 4. Positive Observations

- **Stubbing all subcommands in ETCH-1 is a smart scope addition.** The Entire plugin protocol will invoke any declared subcommand — having stubs return `{"ok": true}` means the binary is immediately usable as an Entire plugin after this ticket, not just after ETCH-7. This unblocks dogfooding and live integration testing in parallel with Wave 2 development.
- **Package layout is clean.** Using `internal/` prevents downstream import of unstable APIs. Isolating each subcommand into its own package keeps the dispatch thin and test boundaries clear. This will compose well with the BUILDPLAN's deeper packages (capture/, refs/, schema/) arriving in Wave 2.
- **Test plan covers the right categories.** Output validation (JSON correctness), error paths (unknown subcommand exit code), build verification, and testutil self-tests. The 9-item test plan is proportional to the scope.
- **No unnecessary dependencies.** The plan correctly limits to `oklog/ulid` and avoids framework overhead. Matches the "No frameworks" constraint in BUILDPLAN.
- **Version string uses Stage 11's `0.01.001` convention.** Small detail, correctly handled.
