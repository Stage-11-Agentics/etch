# scripts/

## `smoke.sh`

End-to-end smoke test for the Etch install + capture story. Run it with:

```bash
make smoke        # or: bash scripts/smoke.sh
```

It builds the binary, spins up a throwaway git repo, enables Entire and registers
the `etch` agent against the **real** `entire` CLI, then simulates a full agent
session by piping hook-event JSON through the binary the same way Entire's hooks
do. It asserts that a single immutable ref appears under `refs/etch/sessions/`,
that its `session.json` carries `schema_version: etch.session.v1`, and that the
Agent Trace blob is emitted alongside it. Each step prints a colored `✓`/`✗`; the
script exits non-zero if any check fails and cleans up its temp repo on exit.

Dependencies: `git`, `go`, `python3`, and the `entire` CLI on `$PATH`. The script
is self-contained and re-runnable — each run uses a fresh `mktemp` directory.
