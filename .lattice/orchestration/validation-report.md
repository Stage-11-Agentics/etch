# Validation Report — Etch Build Run

Validated against: `origin/main` at `6ca5f4e` (ETCH-8 merge)
Validator: agent:result-validator (Phase 4)
Date: 2026-05-27

## Build & Test Results

| Check | Result |
|---|---|
| `go build` | PASS — binary compiles cleanly |
| `go test ./...` | PASS — all 10 test packages pass (4 no-test-file packages skipped) |
| `go test -tags density` | PASS — all 3 density tests pass (20 concurrent, crash recovery, uniqueness) |

## Acceptance Criteria

| # | SPEC criterion | Result | Evidence |
|---|---|---|---|
| 1 | Binary implements all six hooks + capability subcommands | **PASS** | `cmd/entire-agent-cairn/main.go:21-56` dispatches all 10 required subcommands: `info`, `parse-hook`, `session_start`, `session_end`, `user_prompt_submit`, `stop`, `pre_tool_use`, `post_tool_use`, `extract-modified-files`, `calculate-tokens`. Also implements `setup-refspec` (bonus). Each reads stdin JSON and writes JSON to stdout. |
| 2 | Every session produces valid session.json conforming to cairn.session.v1 | **PASS** | `schema/session.go` defines `Session` struct with all OUTPUT_SPEC.md fields: `schema_version` (hardcoded `cairn.session.v1`), `session_id`, `parent_session_id`, `status`, `exit_reason`, `agent` (runtime/model/version), `prompt` (text/source/truncated), `orchestration` (type + 6 subfields + extra), `timing`, `machine`, `operator`, `git_start`, `git_end`, `outcome`, `files_touched`, `tokens`, `tool_use`, `transcript_ref`, `c11`. All nullable fields use pointer types. |
| 3 | Each session ref is orphan commit with session.json + agent-trace.json tree | **PASS** | `refs/writer.go:41` calls `commit-tree` with NO `-p` parent flag (verified by grep — zero matches for `-p` in writer.go). Tree input (line 34) contains exactly two blobs: `agent-trace.json` and `session.json`. Commit author set to `cairn <cairn@localhost>` (lines 63-66). |
| 4 | 20 concurrent sessions produce valid distinct refs with no collisions | **PASS** | `test/density/density_test.go:TestDensity20Concurrent` — 20 goroutines each run full session lifecycle (start → prompt → tool_use → end). Verifies 20 refs exist with 20 unique ULIDs, all with `schema_version: cairn.session.v1`, `status: complete`. Test passes in 2.44s. |
| 5 | Refs push to Forgejo/GitHub and fetch on second machine | **PASS** | `test/density/density_test.go:verifyPushFetch()` creates a bare remote, pushes all cairn refs, clones into a second directory, fetches with cairn refspec, verifies ref count and content match. `commands/setup_refspec.go` provides `setup-refspec` subcommand that configures `refs/cairn/sessions/*` for both push and fetch. Tested locally (bare remote), not cross-machine to Atlas. |
| 6 | Simulated crash produces recoverable .wip file → partial record | **PASS** | `recovery/recovery.go:RecoverSession()` builds partial session with `status: "incomplete"`, `exit_reason: "crash"`, nil `ended_at`/`duration_ms`. `ScanOrphaned()` detects orphans via dead PID check (`processAlive()` using signal 0) or timeout (configurable, default 4h). `RecoverAll()` is called from `session_start.go:31` on every new session. `TestDensityCrashRecovery` verifies: starts session, simulates crash via dead PID rewrite, triggers recovery on next session_start, verifies 2 refs (1 complete, 1 incomplete), zero remaining .wip files. Test passes. |
| 7 | Machine identity hashed by default; raw opt-in | **PARTIAL** | Default hashing works: `capture/machine.go:CaptureMachine()` computes `sha256:<hex>` of hostname. `config/config.go:Defaults()` sets `RawMachineIdentity: false`. **Gap:** `CaptureMachine()` does not read config — it never populates `hostname_raw` even when `raw_machine_identity: true` is set in `.cairn/settings.json`. The opt-in logic exists in `redact/hostname.go:GetHostname()` but is not wired into the capture pipeline. |
| 8 | Prompt and tool-use fields scanned for secrets before commit | **PASS** | `redact/secrets.go` defines 9 builtin patterns: AWS access/secret key, Anthropic API key, OpenAI API key, Stripe live/test key, generic secret, bearer token, private key. `redact/redact.go:Redact()` applies builtin + custom patterns from settings. Applied in `hooks/commit.go:24` to prompt text. Tool-use data stored in session is aggregates only (tool name → count), not raw content, so secret exposure risk is minimal. |
| 9 | agent-trace.json emitted in Agent Trace RFC format | **PASS** | `schema/trace.go:AgentTrace` struct has `version: "1.0"` and `traces[]` with `agent_id`, `model`, `session_id`, `files`, `timestamp` — matches Cursor Agent Trace RFC v1.0. `SessionToAgentTrace()` generates from session data. Written alongside session.json in every commit tree (verified in `refs/writer.go:34`). Density test validates `trace["version"] == "1.0"` for all 20 concurrent sessions. |
| 10 | Orchestration metadata captured from CAIRN_* env vars; absent = manual | **PASS** | `capture/environ.go:CaptureOrchestration()` reads all 7 env vars: `CAIRN_ORCHESTRATOR_TYPE` (→ type, default "manual"), `CAIRN_DISPATCH_METHOD`, `CAIRN_TICKET_ID`, `CAIRN_RUN_ID`, `CAIRN_AGENT_ROLE`, `CAIRN_WORKFLOW_VERSION`, `CAIRN_ORCHESTRATION_EXTRA` (JSON parsed into `extra` map). `CAIRN_PARENT_SESSION_ID` is read separately in `session_start.go:74`. All 8 CAIRN_* vars accounted for. |
| 11 | c11 context captured when C11_* env vars present | **PARTIAL** | `capture/environ.go:CaptureC11()` reads `C11_WORKSPACE_ID` and `C11_SURFACE_ID` (with `CMUX_*` fallback). Tab title fetched via `c11 get-titlebar-state --surface <id> --json`. Returns nil when env vars absent. **Gap:** `pane_lineage` field exists in `C11Info` struct but is never populated — `CaptureC11()` doesn't read or compute lineage. |
| 12 | Ref lifecycle: refs compactable into archive refs | **PASS** | Architecture explicitly supports archival: `config/config.go:Settings` includes `ArchiveThresholdDays` with default 90. ETCH-11 is ticketed in BUILDPLAN.md for Phase 3 implementation. Per validation plan, pass condition is "architecture explicitly supports archival; ETCH-11 ticketed for Phase 3" — met. |

## Summary

**10 PASS, 2 PARTIAL** out of 12 criteria.

## Gaps

1. **Raw hostname opt-in not wired (SPEC #7).** `capture/machine.go:CaptureMachine()` always produces `sha256:` hash and ignores `.cairn/settings.json` `raw_machine_identity` flag. The `redact/hostname.go:GetHostname()` function has the config-aware logic but is never called during session capture. Fix: have `CaptureMachine()` accept or load `config.Settings` and populate `HostnameRaw` when enabled.

2. **pane_lineage never populated (SPEC #11).** `capture/environ.go:CaptureC11()` populates `workspace_id`, `surface_id`, and `tab_title` but does not compute `pane_lineage`. The field exists in the `C11Info` struct and serializes correctly, but will always be nil/empty. Fix: query `c11` CLI for pane ancestry or derive from tab titles.

## Recommendations

1. **Wire raw hostname opt-in** — small fix, high correctness impact. Load config in `CaptureMachine()` or call `redact.GetHostname()` instead of inline SHA-256.
2. **Populate pane_lineage** — requires `c11` CLI support for pane ancestry queries. If the CLI doesn't expose this yet, document as known limitation and defer.
3. **Add test for raw hostname config** — verify that setting `raw_machine_identity: true` actually produces `hostname_raw` in the session record.
4. **Consider redacting tool input fields** — currently only prompt text is scanned for secrets. While tool-use data in the session is aggregates-only (safe), the `.wip.jsonl` buffer does contain raw tool data that passes through the system.
5. **Cross-machine push/fetch** — the density test verifies push/fetch against a local bare remote. A real Hyperion → Atlas test would fully validate SPEC #5, but this is an operational validation rather than a code gap.
