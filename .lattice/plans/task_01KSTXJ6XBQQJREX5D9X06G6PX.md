# Plan — ETCH-20: Hook-event JSON contract documentation + visible warnings

**This ticket rides the auto-capture lane.** The full joint plan (root cause,
architecture decision, implementation steps, tests, validation gates) lives in
the primary ticket's plan: `plans/task_01KSTXHGVBPDWXBETVYB1MX6B7.md` (ETCH-17).
This file scopes the ETCH-20 deliverables out of that plan.

## Resolved design decision

**Do both: document AND warn.** Docs are the durable fix; a stderr warning
closes the silent-failure footgun that cost the QA agent time. (Resolves the
"and/or" left open in the ticket.)

## Scope (ETCH-20 slice of the joint plan)

1. **Dual-dialect payload parsing** (`internal/hooks/common.go`): accept both
   the Entire HookInput dialect (`user_prompt`, `raw_data.model`, `session_ref`)
   and Claude Code native fields (`prompt`, top-level `model`,
   `transcript_path`). This *removes* the wrong-field trap for the most common
   wrong guesses — they're now correct field names.
2. **Visible warnings** (stderr, never stdout; exit stays 0; stdout unchanged):
   warn only on fields each hook type is expected to populate — e.g.
   `user_prompt_submit` with neither `user_prompt` nor `prompt`, `session_start`
   with no model in either dialect. Message names the expected keys and the
   payload keys actually received.
3. **`docs/HOOK_CONTRACT.md`**: per-event stdin contract, both dialects, one
   copy-pasteable JSON example per hook (session_start, user_prompt_submit,
   pre_tool_use, post_tool_use, stop, session_end), field tables,
   unknown-field behavior (ignored), missing-field behavior (warn + exit 0),
   plus the install-side plugin protocol. README links to it; examples must
   match `scripts/smoke.sh` (which stays green as the contract check).

## Files

- `internal/hooks/common.go` (+ handlers reading new fields)
- `internal/hooks/hooks_test.go` (+ new cases)
- `docs/HOOK_CONTRACT.md` (new)
- `README.md` (link + truth pass, shared with ETCH-17)
- `scripts/smoke.sh` (extended; existing direct-pipe step kept as regression)

## Tests

- Table tests per hook: wrong-dialect payloads (top-level `model`, `prompt`)
  now parse correctly (dual-dialect); truly-empty payloads emit a stderr
  warning, exit 0, stdout byte-identical to today.
- Correct-field payloads: no warning, correct extraction (existing tests keep
  passing).
- Smoke: existing Entire-dialect step + new native-dialect step both produce
  valid session refs.

## Acceptance

- Hook contract documented with the correct field names + examples per event.
- Sending the QA agent's original wrong guesses (`{"model":...}`,
  `{"prompt":...}`) now *works* (native dialect) instead of silently dropping.
- A payload with no usable fields produces a visible stderr warning, exit 0.
