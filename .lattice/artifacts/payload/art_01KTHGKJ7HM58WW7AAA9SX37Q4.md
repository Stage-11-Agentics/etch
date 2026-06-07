# Self-review — ETCH-21 (agent:clidocs-w2-reviewer)

**Verdict: PASS**

Walked `git diff origin/main...HEAD` (commit 936d873).

## Findings

1. **[FIXED] usage.go comment referenced a nonexistent test name** (`TestHelpListsEverySubcommand`). Corrected to name the real guards (`assertFullListing`, `TestListedSubcommandsAreDispatched`) before commit was cut.
2. **[OK] Exit-code/stream contract** — help/--help/-h → stdout + exit 0; bare → stderr + exit 1 (preserves missing-arg semantics and the pre-existing `TestNoSubcommand`). Unix-conventional.
3. **[OK] Drift guards both directions** — `assertFullListing` (every dispatched name appears in help) + `TestListedSubcommandsAreDispatched` (every advertised name reaches a real dispatch case, not the default). Addresses the auto plan-review's MINOR #3.
4. **[OK] Listing accuracy** — every desc/arg synopsis verified against the implementing package (query flags from internal/query/query.go, index actions, archive flags, install-hooks --force, extract-modified-files/calculate-tokens take <session-id> not stdin).
5. **[NOTE] Hook entry points and stubs are listed under clearly-labeled internal/stub sections** rather than omitted — full discovery without advertising stubs as usable. Matches the auto plan-review's recommendation on ETCH-21.

Note on the auto plan-review FAIL artifact (art_01KTHG7K9FE720B6TNZMTR4GQ5): it raced the plan write — it reviewed the scaffold file before the Plan section was appended. All CRITICAL/MAJOR items it raised (grouping decision, authored one-liners, exit/stream spec, test plan) are addressed in the written plan and in this implementation.

`go test ./...` green, `make build` green, `make smoke` PASSED.