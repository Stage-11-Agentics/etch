# Plan Review: ETCH-5 — Security + Redaction

## 1. Verdict

**FAIL (plan-level)** — The plan is empty. It contains only the title "ETCH-5: Security + redaction" with no implementation details, file listing, approach description, or acceptance criteria mapping. Implementation cannot proceed from this plan.

## 2. Summary

The submitted plan for ETCH-5 is a blank document — just the ticket title with zero content. The task itself is well-specified across SPEC.md (criteria #7 and #8), BUILDPLAN.md (§5), and OUTPUT_SPEC.md, with clear requirements for hostname hashing, secret scanning/redaction, and config file reading. But none of that specification has been distilled into an actionable implementation plan. A delegator working from this plan would have to reverse-engineer the entire approach from the spec documents, which defeats the purpose of the planning phase.

## 3. Issues

**[CRITICAL] Entire plan — Plan is empty**
The plan body contains only the title. There is no description of approach, no file listing, no step breakdown, no acceptance criteria mapping, no risk identification. This is not a plan — it's a placeholder.
**Recommendation:** Return to `in_planning`. The plan must cover at minimum:
- **Approach:** How hostname hashing and secret scanning will be implemented (package structure, key functions, data flow).
- **Files:** Which files will be created or modified (e.g., `internal/redact/hostname.go`, `internal/redact/secrets.go`, `internal/config/config.go`, plus test files).
- **Acceptance criteria mapping:** Explicit mapping of SPEC criteria #7 and #8 to implementation steps.
- **Regex patterns:** The specific patterns to be implemented (AWS `AKIA...`, Anthropic `sk-ant-...`, OpenAI `sk-proj-...`, Stripe `sk_live_`/`sk_test_`, generic `sk-`/`key-`/`token-`).
- **Config schema:** The `.cairn/settings.json` structure and how it will be read.
- **Testing approach:** How hostname hashing (default vs raw) and secret redaction will be tested.

**[CRITICAL] Entire plan — No file inventory**
The plan does not identify which files will be created or modified. BUILDPLAN.md references a `redact/` package, but the plan doesn't specify whether this is `internal/redact/`, `pkg/redact/`, or something else, nor does it list the individual source and test files.
**Recommendation:** List every file to be created/modified with a one-line description of its responsibility.

**[CRITICAL] Entire plan — No acceptance criteria coverage**
SPEC criteria #7 (machine identity hashing) and #8 (secret scanning/redaction) have specific, testable requirements. The plan must show how each will be satisfied.
**Recommendation:** Include a criteria-to-implementation mapping table showing which code component satisfies each criterion.

**[MAJOR] Entire plan — No edge case or risk identification**
Security-related code has inherent risks: regex ReDoS, false positives in redaction, config file parsing failures, hostname resolution failures. None of these are addressed.
**Recommendation:** Include a risks section covering at minimum: regex performance (ReDoS), behavior when hostname lookup fails, behavior when `.cairn/settings.json` is missing or malformed, and false positive handling strategy.

## 4. Positive Observations

The task itself is well-scoped in the upstream documents (SPEC.md, BUILDPLAN.md, OUTPUT_SPEC.md). The requirements are clear, the acceptance criteria are testable, and the dependency chain (depends on ETCH-1, blocks ETCH-7) is well-defined. The planning phase just needs to actually produce a plan that synthesizes these inputs into an implementation roadmap.
