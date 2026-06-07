# Validation Evidence — ETCH-20: PASS

- docs/HOOK_CONTRACT.md documents the per-event stdin contract with correct
  field names and copy-pasteable examples per hook (both dialects: Entire
  HookInput and Claude Code native, captured live from Claude Code 2.1.168).
- The QA report's original wrong guesses (top-level model, prompt) are now the
  accepted native dialect — they parse correctly instead of silently dropping.
  Locked by TestNativeClaudeCodeLifecycle.
- Missing expected fields now warn visibly on stderr (exit 0, stdout
  unchanged): verified by TestWarningsOnMissingFields and manually via
  `echo '{"session_id":"x"}' | entire-agent-etch user_prompt_submit` →
  'etch: warning: user_prompt_submit carried no prompt...'.
- Entire-dialect regression locked (TestEntireDialectStillWorks + original
  smoke step 4 unchanged and green).