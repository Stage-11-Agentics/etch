# Code Review (own-reviewer fallback) — ETCH-20 slice of the auto-capture fix

**Context:** auto-fired code-review hung (75+ min, stalled child process);
killed and replaced by this structured self-review per the lane's fallback
protocol. Full review of the joint diff is attached to ETCH-17
(art_01KTHF4RJ3VND96XN3C9ZARK1J); this note covers the ETCH-20 acceptance
surface specifically.

## Verdict: PASS

- **Documented contract:** docs/HOOK_CONTRACT.md specifies per-event stdin
  payloads in BOTH dialects with live-captured examples (Claude Code 2.1.168),
  field mapping table, unknown-field (ignored) and missing-field (stderr warn,
  exit 0) semantics. README links to it.
- **The QA agent's original wrong guesses now work:** top-level
  `{"model":...}` and `{"prompt":...}` are the native dialect and parse
  correctly (dual-dialect StdinEvent). Locked by TestNativeClaudeCodeLifecycle.
- **Silent dropping is gone:** payloads missing expected fields warn visibly
  on stderr naming expected keys + received keys; stdout/exit unchanged.
  Locked by TestWarningsOnMissingFields.
- **Regression safety:** Entire-dialect fields (`user_prompt`,
  `raw_data.model`) still parse (TestEntireDialectStillWorks); smoke's
  original direct-pipe step retained; documented examples match
  scripts/smoke.sh payloads.