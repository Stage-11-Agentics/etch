# Validation Evidence — ETCH-35 (no-git visibility; no silent ok on dropped data)

Branch fix/repo-root-batch @ f869d9d. Run 2026-06-07.

## Automated gates
- TestNonGitDirAllHooksFailVisible — ALL SIX hook subcommands in a non-git dir: exit
  code != 0, stderr explains ("could not resolve a git repository"), stdout contains
  ok:false and never ok:true, NO .etch created. PASS.
- TestCommitFailureVisibleAndRecoverable — deterministic ref-write sabotage at
  session_end: non-zero exit, ok:false on stdout, wip + mapping retained for recovery.
  PASS.
- TestStopAfterSessionEndStaysOK — regression guard for the REFUTED stop-after-end
  finding: second end-hook still exits 0 with ok:true. PASS.
- `go test ./...`, `make build`, `make smoke` — all green.

## Live e2e (real binary, plain temp dir)
session_start and session_end in a non-git dir:
- stderr: "etch: could not resolve a git repository (cwd=...): git rev-parse: fatal:
  not a git repository ...; session capture disabled, no record will be written"
- stdout: {"ok":false,"error":"could not resolve a git repository ..."} — never ok:true
- exit=1; directory afterwards contains NO .etch (zero pollution, nothing orphaned)

## Go/no-go gate (plan cycle-2 resolution #12): GO
- Entire v0.6.3's agent roster is a fixed built-in list — etch is not yet dispatched
  through `entire hooks`; hook events reach the binary directly with the same stdin
  contract (exactly how make smoke exercises it, which PASSES).
- The invoking layer is therefore the agent-runtime hook wrapper. Claude Code hook
  contract: exit 1 is a non-blocking warning (only exit 2 blocks). Empirically verified
  the sh -c wrapper pattern propagates exit 1 (wrapper-exit=1) — the agent session is
  NOT aborted. Etch deliberately exits 1, never 2.
- Fallback (loud-stderr clean no-op) NOT needed.