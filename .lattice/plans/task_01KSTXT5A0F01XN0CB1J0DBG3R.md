# ETCH-35: no-git-repo: session_end silently drops data, returns ok, orphans .wip

AUDIT ITEM 1 (no git repo edge case). REPRO: run session_start/user_prompt_submit/session_end in a directory that is NOT a git repo.
ACTUAL: session_start exits 0 and creates .etch/ in the non-repo dir; session_end's commit fails ('fatal: not a git repository') but the error is only log.Printf'd in session_end.go (commitSession error swallowed) -- the process still prints {"ok":true} and exits 0. The captured session is NEVER committed, the .wip is orphaned, and .etch/ pollutes the directory. Entire sees success.
IMPACT: silent data loss + no surfaced error if Etch is ever invoked outside a repo (misconfig, wrong cwd). FIX: detect non-repo at session_start and either no-op cleanly or surface a non-zero/error so the failure is visible; don't print ok on commit failure. Verified empirically in /tmp/etch-nogit.
