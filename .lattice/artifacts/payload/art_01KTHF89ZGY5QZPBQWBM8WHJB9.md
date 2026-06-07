# Validation Evidence — ETCH-17 real-thing gate: PASS

(Phase note: this Lattice version has no in_validation status / validation
role; evidence attached as a note at review.)

## Gate definition
Fresh temp repo + documented README configure steps + REAL Entire/Claude Code
session ⇒ `git for-each-ref refs/etch/sessions/` shows a committed session
record. Executed twice against this branch; both passed.

## Run 1 (mid-impl, commit 089a027 pre-state)
- /tmp/etch-realgate-yDFx, `entire enable --agent etch --no-github`, then
  `env -u CLAUDECODE claude -p "Read notes.txt and tell me what it says" --dangerously-skip-permissions`
- → refs/etch/sessions/01KTHA6BQ4T3CQJPYSA8EM02VS (commit 2630294)
- model=claude-opus-4-8 (backfilled from transcript — native payloads carry no
  model field), prompt captured, tools {Read:1}, status=complete,
  exit_reason=other (native reason), duration 9046ms

## Run 2 (final HEAD a070a7d, multi-tool)
- /tmp/etch-finalgate-als1, same configure path, prompt: "Read notes.txt, then
  create a file called echo.txt containing the same text"
- enable output: "Installed 5 hooks", "Ready."
- → refs/etch/sessions/01KTHF5WFDQJ19TN5EA2HYCW61 (commit 48a8ac6)
- model=claude-opus-4-8 | runtime=claude-code | status=complete | exit=other
- tools {Read:1, Write:1} | files_touched [notes.txt, echo.txt]
- transcript_ref.available=true (finalize re-stat working) | duration 10233ms

These are the first real auto-captured sessions in project history — Entire
0.6.3 discovery → etch install-hooks → native Claude Code dispatch → committed
ref, with zero manual event piping.

## Other gates
- `make smoke` green, including new steps 8–10 (enable-via-Entire, installed-
  hook native session, record assertions incl. coexistence with Entire's own
  claude-code hooks).
- `go test ./...` green (13 packages; new install + native-dialect + warning
  suites). `make build` green.
- Hook contract documented: docs/HOOK_CONTRACT.md (per-event examples in both
  dialects from live captures; warning semantics; install-side protocol pinned
  to v0.6.3 structs).

## Dogfooding enablement (for the Orchestrator, post-merge)
```bash
cd /Users/atin/Projects/Stage11/code/Etch
make install PREFIX=$HOME/.local        # or: make install (needs sudo)
entire enable --agent etch --no-github  # wires .claude/settings.json
entire-agent-etch are-hooks-installed   # → {"installed":true}
entire-agent-etch setup-refspec         # sync refs with the remote
```
Every subsequent agent session on the Etch repo is then captured to
refs/etch/sessions/* automatically.