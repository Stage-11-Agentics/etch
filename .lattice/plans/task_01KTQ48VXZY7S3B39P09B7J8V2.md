# ETCH-45: install-hooks --local writes to .claude/settings.local.json

Found during the Etch rollout (ROLLOUT wave 0.5). The default install-hooks writes .claude/settings.json, which is committed repo state — the install unit for repos we control. For repos a user can't commit to (third-party clones, repos where the team hasn't opted in), there needs to be an escape hatch that wires capture locally without touching tracked files.

What it does: add an 'install-hooks --local' flag that writes the same guarded etch dispatch entries into .claude/settings.local.json instead of .claude/settings.json. settings.local.json is gitignored by Claude Code convention, so it stays a personal, uncommitted opt-in.

Acceptance criteria:
- 'install-hooks --local' writes the 5 etch hook entries to .claude/settings.local.json.
- are-hooks-installed detects hooks in EITHER settings.json or settings.local.json (so doctor/Entire see a --local install as installed).
- uninstall-hooks removes etch entries from both files.
- The guard (command -v entire-agent-etch || exit 0) is identical to the committed-hooks form.
- Idempotent + --force behave as they do for the committed path.
- Unit test covers the --local write, dual-file detection, and uninstall from local.

Context: ROLLOUT 'Practices being set' #1 names settings.local.json as 'the escape hatch for repos we don't control.' Referenced in code/platform/etch.md's enable checklist.
