# ETCH-41 Validation Evidence

**Branch:** feat/local-only-transport @ b18703c (rebased onto origin/main 9930ee2, post-#21/#22)
**Merge check:** `git merge-tree --write-tree origin/main feat/local-only-transport` → clean, no conflicts.

## Gates (all green, run on the rebased tree)

- `make build` — built bin/entire-agent-etch
- `go test ./...` — 14/14 packages ok
- `make smoke` — SMOKE PASSED (install + capture story end-to-end against real Entire CLI)

## Ticket-specific gates

Strip walker: 16 unit tests pass (grammar, array fan-out, map/interface descent, protected paths incl. prefix rule, markers, manifest, idempotence, schema round-trip, agent_session_id strippable-by-decision).

E2e (internal/hooks/local_only_test.go), all PASS:
- **TestE2ELocalOnlyTransport — THE gate:** temp repo + bare remote → setup-refspec → session with sensitive value in configured field → bare `git push` → value absent on remote and in a fresh clone+fetch, `[LOCAL_ONLY:prompt.text]` marker present, refs/etch/local/* exists nowhere off-machine, original repo's local ref intact with the value.
- TestE2ELocalOnlyDualRef: sessions ref stripped (session.json + agent-trace.json + ref commit message all rebuilt from stripped record), local ref full fidelity, manifest only on stripped record.
- TestE2ELocalOnlyEmptyConfigNoChange: empty config → byte-identical behavior, no local namespace.
- TestE2ELocalOnlyNoMatchNoLocalRef: configured-but-unmatched paths → full no-op, single ref.
- TestE2ELocalOnlyCrashRecovery: recovery path produces the same dual-ref projection.

## Code-review cycle resolution

Review art_01KTHH4480FW08RTXW250FMTAC: FAIL solely on integration (branch pre-dated #21). Resolved: rebased onto current main, conflicts resolved keeping both batches; agent_session_id consciously kept strippable (nullable transcript join key, not identity — encoded as unit test); MINOR 1 comment softened; MINOR 2 implemented (local ref gated on applied strips). Gates re-run green post-rebase.