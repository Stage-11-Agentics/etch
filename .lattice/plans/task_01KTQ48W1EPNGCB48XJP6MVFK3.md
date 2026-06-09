# ETCH-46: etch doctor — capture health check

Found during the Etch rollout (ROLLOUT wave 0.5). This is the answer to the rollout risk 'capture silently breaks (binary moved, hooks dropped).' There is currently no single command to tell an operator whether Etch is actually working in a repo; the interim is a weekly 'query --since' spot check.

What it does: add an 'entire-agent-etch doctor' subcommand that reports, for the current repo, a health summary (human table + --json):
  1. Binary on PATH — is entire-agent-etch resolvable, and is it the same build as the one invoked? (path + version)
  2. Hooks present — are the 5 etch hook entries installed in .claude/settings.json (and/or settings.local.json)? Flag partial installs.
  3. Refspec state — is a refs/etch/sessions/* fetch/push refspec configured, and on which remote(s)? For a public repo, NO refspec is the correct state — report it as OK-for-public, not a warning (detecting public vs private is out of scope; just report the facts).
  4. Age of newest captured session — newest refs/etch/sessions/ commit time; warn if older than a threshold (suggest --warn-age, default e.g. 14d) since stale capture is the silent-breakage signal.
  5. Orphaned wip buffers — count .etch/sessions/*.wip.jsonl and the oldest one's age (a growing pile past recovery_timeout_hours indicates recovery isn't firing).

Acceptance criteria:
- 'doctor' exits 0 when healthy, non-zero when a hard problem is found (binary not on PATH, hooks missing/partial). Stale-age and no-refspec are warnings, not failures.
- '--json' emits a structured report with a field per check.
- Works in a repo with zero sessions (reports 'no sessions captured yet', not an error).
- Unit test in the temp-repo harness covers: healthy repo, hooks-missing repo, no-sessions repo, stale-newest-session repo.

Context: named in ROLLOUT wave 0.5 and the Risks table; referenced as the not-yet-shipped health check in code/platform/etch.md's query cheatsheet and gotchas.
