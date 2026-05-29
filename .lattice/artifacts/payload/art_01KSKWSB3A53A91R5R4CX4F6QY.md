# Plan Review: ETCH-6 — Agent Trace emission

## 1. Verdict

**PASS** — Plan is complete, feasible, and aligned. Implementation can proceed.

## 2. Summary

The plan for ETCH-6 proposes a clean, minimal serialization layer: two Go files defining the Agent Trace structs and a `SessionToAgentTrace` conversion function, plus a comprehensive test file with seven test cases. The approach is technically sound, tightly scoped to SPEC #9, and the test coverage is thorough. The one coordination concern — defining Session types here when ETCH-2 is the canonical schema owner per BUILDPLAN — is explicitly acknowledged in the plan and is a reasonable pragmatic choice for a parallel Wave 2 ticket.

## 3. Issues

**[Minor] Files to create / session.go — Session type ownership overlap with ETCH-2**
The BUILDPLAN field assignments table designates ETCH-2 as the schema owner for `session.json`. This plan defines the Session struct in ETCH-6 and states "ETCH-2 will extend this later." Since both are parallel Wave 2 tickets, there's a merge/coordination risk: if ETCH-2's implementer independently defines the same types with different field names, JSON tags, or sub-struct shapes, ETCH-7 integration will require reconciliation work.
**Recommendation:** The plan already acknowledges this. To mitigate further, the implementer should define the *minimum viable* Session struct — only the fields needed for the trace conversion (`SessionID`, `Agent.Runtime`, `Agent.Model`, `Timing.StartedAt`, `Timing.EndedAt`, `FilesTouched[]`) — and add a brief comment noting these types are intentionally minimal and will be superseded by ETCH-2's full definition. This reduces the surface area of potential conflicts.

**[Minor] trace.go — JSON struct tags not explicitly specified**
The plan describes Go struct fields (`AgentID`, `Model`, `SessionID`, etc.) but doesn't mention JSON struct tags. Agent Trace RFC compliance requires exact JSON field names (`agent_id`, `model`, `session_id`, `files`, `timestamp`). This is standard Go practice and any competent implementer will add them, but given that RFC conformance is the entire point of this ticket, the plan would be stronger with an explicit note.
**Recommendation:** Add a one-liner: "All struct fields carry `json:` tags matching the Agent Trace RFC field names exactly (snake_case)."

**[Minor] trace.go — Timestamp format not specified**
The plan says "use EndedAt if non-nil, fall back to StartedAt" for the timestamp value, which is correct. But it doesn't specify the format. OUTPUT_SPEC.md shows ISO 8601 UTC (`"2026-05-26T14:47:22.109Z"`). Go's `time.Time` marshals to RFC 3339 by default (which is a profile of ISO 8601), so this will likely work automatically — but the test cases should verify the exact format includes the `Z` suffix and millisecond precision.
**Recommendation:** Add a test case (or extend test case #6) that verifies the timestamp string format in the marshaled JSON matches ISO 8601 UTC with millisecond precision.

## 4. Positive Observations

- **Tight scope.** The plan does exactly what the task description asks — no scope creep, no premature abstractions. Three files, one conversion function, seven tests.
- **Thoughtful test coverage.** The seven test cases cover the important scenarios: complete session, version pinning, file extraction, empty files (not nil), fallback timestamp, JSON round-trip, and single-trace-per-session invariant. This is exactly the right level of rigor for a serialization layer.
- **Explicit dependency awareness.** The plan clearly states what ETCH-3 will consume and what ETCH-2 will extend, making the integration path visible.
- **No external dependencies.** Pure stdlib + schema types, consistent with the project's "no frameworks" constraint.
- **The decision to define minimal Session types here rather than waiting for ETCH-2 is the right call.** It unblocks ETCH-6 implementation without creating a blocking dependency chain, and the plan is transparent about the tradeoff.
