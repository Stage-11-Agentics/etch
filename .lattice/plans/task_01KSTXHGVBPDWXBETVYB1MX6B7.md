# Plan — ETCH-17 Auto-Capture Investigation & Fix (+ ETCH-20 Hook Contract Docs)

**Tickets:** ETCH-17 (critical, primary), ETCH-20 (medium, rides along)
**Worktree:** `/Users/atin/Projects/Stage11/code/Etch-worktrees/auto-capture` (branch `fix/auto-capture`)
**Author:** agent:autocap-w0-planner, 2026-06-07

---

## Confirmed root cause (investigation complete, reproduced + probe-verified)

Entire v0.6.3 (`17720a12`, exact source tag cloned and read) **does** support external
agents via `entire-agent-<name>` binaries on `$PATH`. Discovery calls the binary's
`info` subcommand and requires a **protocol v1 response**
(`cmd/entire/cli/agent/external/types.go`):

```json
{"protocol_version": 1, "name": "...", "type": "...", "description": "...",
 "is_preview": false, "hook_names": [...], "capabilities": {"hooks": true, ...}}
```

Etch never dispatches because of **four independent failures**, in causal order:

1. **`info` doesn't speak the protocol.** Etch emits
   `{"name":"etch","version":"0.01.001","hooks":true,...}` — no `protocol_version`,
   no `capabilities` object. Entire's `external.New()` rejects it
   ("protocol version mismatch: binary reports 0, expected 1") and **silently skips
   the binary** during discovery (debug-level log only). Verified: with a fake
   `entire-agent-probe` emitting protocol-v1 info, `entire enable --agent probe
   --no-github` discovers, registers, and calls `install-hooks` (RC=0, call log
   captured). With etch's current info shape, it does not.
2. **Etch implements none of the install-side protocol.** `entire enable --agent
   etch` → `setupAgentHooksNonInteractive` → `InstallHooks` → runs
   `entire-agent-etch install-hooks` expecting `{"hooks_installed": N}`. Etch has no
   `install-hooks`, `uninstall-hooks`, `are-hooks-installed`, or `detect`
   subcommand — so even with correct `info`, enable would fail. **Nothing ever
   wires hook dispatch into the agent runtime's config; that wiring is the
   plugin's own job in Entire's model.** This is THE missing link: Entire never
   "dispatches to plugins" on its own — the plugin's `install-hooks` must write
   hook entries into the runtime's hook config itself.
3. **`entire agent add` is broken for external agents on 0.6.3** (Entire-side bug):
   `newAgentAddCmd` (`agent_group.go:104`) calls `agent.Get()` without ever running
   external discovery, so `entire agent add etch` can NEVER work on this version.
   `entire enable --agent etch` works (`setup.go:824` calls
   `DiscoverAndRegisterAlways` first) and auto-persists `external_agents: true`.
   README told users a path that cannot succeed.
4. **Hook-payload dialect mismatch (= ETCH-20).** Etch's hook handlers parse
   Entire's *internal* `HookInputJSON` dialect (`user_prompt`, `raw_data.model`,
   `session_ref`) — a shape Entire only uses for `get-session-id`/`read-session`,
   never for hook dispatch. Real Claude Code hooks deliver **native** payloads
   (`prompt`, top-level `model`, `transcript_path`, `hook_event_name`). Unknown
   fields are silently dropped (exit 0, no warning) — exactly the QA finding.

### Architecture decision: direct dispatch, not `entire hooks etch`

Two possible dispatch designs were evaluated against Entire's source:

- **Via Entire** (`entire hooks etch <hook>`): Entire's `executeAgentHook` treats
  the external binary as *the tracked agent runtime* — it calls `ParseHookEvent`
  then runs `DispatchLifecycleEvent`, driving its full checkpoint engine
  (`get-session-id`, `read-session`, `write-session`, transcript chunking…)
  against etch. Etch is a ride-along metadata capturer, not a runtime; this path
  double-tracks every Claude Code session, requires a large unimplemented protocol
  surface, and entangles capture with Entire's strategy engine. **Rejected.**
- **Direct dispatch** (installed hooks exec `entire-agent-etch <event>` with the
  runtime's native JSON): version-independent of Entire, works even if `entire`
  is absent at runtime, zero double-tracking, and coexists with Entire's own
  claude-code hooks (Claude Code runs all hook entries per event). **Chosen.**

Entire's role lands where it's strong: `entire enable --agent etch` is the
*install* path (discovery → our `install-hooks` does the wiring). At runtime,
Claude Code dispatches straight to the etch binary.

---

## Implementation plan

### 1. Protocol-v1 `info` (`internal/info/info.go`)

Emit `protocol_version: 1`, `name: "etch"`, `type: "etch"`, `description`,
`is_preview: false`, `hook_names: ["session_start","user_prompt_submit",
"pre_tool_use","post_tool_use","stop","session_end"]`,
`capabilities: {"hooks": true}` (all other capabilities `false` — only declare
what is protocol-verified; the old top-level booleans were aspirational and
never matched Entire's schema). Keep `"version"` as an extra field (ignored by
Entire, useful to humans).

### 2. New subcommands (`cmd/entire-agent-etch/main.go` + new `internal/install` pkg)

- `detect` → `{"present": true}` (the binary being invoked is the presence test).
- `install-hooks [--local-dev] [--force]` → writes etch hook entries into
  `.claude/settings.json` at the repo root (same file/shape Entire's claude-code
  installer manages: `hooks.<Event>[].hooks[]` matcher structure). Events wired:
  **SessionStart, UserPromptSubmit, PreToolUse, PostToolUse, SessionEnd** — and
  **deliberately NOT Stop**: etch's `stop` handler finalizes the session, and
  Claude Code fires Stop at every turn end, which would truncate multi-turn
  sessions at turn 1. (`stop` stays available for runtimes without a SessionEnd.)
  Commands are guarded like Entire's own:
  `sh -c 'if ! command -v entire-agent-etch >/dev/null 2>&1; then exit 0; fi; exec entire-agent-etch <event>'`.
  Idempotent (skip if present), `--force` removes etch entries first, preserves
  all non-etch JSON. Returns `{"hooks_installed": N}`.
- `are-hooks-installed` → `{"installed": bool}` (scan `.claude/settings.json` for
  etch commands).
- `uninstall-hooks` → remove etch entries, `{}`.

`install-hooks` is also the **standalone path**: it must work invoked directly
(no Entire involved), so capture can be enabled on any repo with one command.

### 3. Dual-dialect hook payload parsing + visible warnings (ETCH-20 fix)

Extend `internal/hooks/common.go` `StdinEvent` to accept both dialects:

| field | Entire HookInput dialect | Claude Code native |
|---|---|---|
| session id | `session_id` | `session_id` |
| prompt | `user_prompt` | `prompt` |
| model | `raw_data.model` | `model` (top level, SessionStart) |
| transcript | `session_ref` | `transcript_path` |
| tool | `tool_name`, `tool_use_id`, `tool_input` | same |

Precedence: dialect-specific field wins; native fills gaps. Emit a **stderr
warning** (exit still 0 — never break the agent's session) when an event that
should carry data has none, e.g. `user_prompt_submit` with neither `user_prompt`
nor `prompt`: `etch: warning: user_prompt_submit carried no prompt field
(expected "user_prompt" or "prompt"); payload keys: [...]`. This makes the
accepted contract visible instead of silently dropping.

### 4. Smoke test extension (`scripts/smoke.sh`)

After the existing steps, add the previously-broken path:
- `entire enable --agent etch --no-github` in the temp repo → assert RC=0 and
  `.claude/settings.json` contains etch hook entries and `.entire/settings.json`
  has `external_agents: true`.
- Drive the **installed hook commands** (extracted from `.claude/settings.json`)
  with **native Claude Code JSON** payloads → assert a second session ref appears.
- Keep the existing direct-pipe step (Entire-dialect regression coverage).

Plus Go unit tests: install/uninstall/are-installed round-trip on a temp repo
(preserving foreign settings.json content), dual-dialect parsing per event,
warning emission on empty payloads, protocol-v1 info shape.

### 5. Real-thing validation gate (ETCH-17 acceptance)

Fresh temp repo → README configure steps → **real Claude Code session**
(`env -u CLAUDECODE claude -p "..." --dangerously-skip-permissions` with cwd in
the temp repo) → `git for-each-ref refs/etch/sessions/` shows a committed
record; `session.json` parses with correct model/prompt/tool fields. Transcript
captured and attached to the ticket as validation evidence. If the headless
claude run is unavailable, fall back to hook-faithful simulation (exact installed
commands, exact native payloads) and say so explicitly in the evidence.

### 6. Docs

- **README truth pass:** Configure section becomes the tested path —
  `entire enable --agent etch --no-github` (one command; auto-enables
  `external_agents`); note that `entire agent add etch` is broken on Entire
  ≤ 0.6.3 (no discovery in that code path) and `enable --agent` is the supported
  route; document the standalone `entire-agent-etch install-hooks` path; rewrite
  "Usage" to describe what is actually true post-fix.
- **`docs/HOOK_CONTRACT.md` (ETCH-20):** per-event stdin contract, both dialects,
  full JSON example per event (session_start, user_prompt_submit, pre/post_tool_use,
  stop, session_end), field tables, unknown-field behavior (ignored), missing-field
  behavior (stderr warning, exit 0), and the install-side protocol (info/detect/
  install-hooks/uninstall-hooks/are-hooks-installed). README links to it.
- PR body + ETCH-17 comment: dogfooding enablement commands for the main Etch repo
  (run by Orchestrator post-merge, not by this lane).

### Out of scope

- Fixing `entire agent add` upstream (Entire-side; documented + noted in PR).
- Hook wiring for non-Claude-Code runtimes (codex, gemini…) — the installer is
  structured so per-runtime writers can be added later; only claude-code now.
- Declaring transcript_analyzer/token_calculator capabilities to Entire (their
  protocol subcommand shapes don't match etch's existing commands; revisit later).

## Risks / mitigations

- **Claude Code without `model` in SessionStart payload** (older builds): field
  stays null — same as today; record still valid.
- **Hard-killed sessions never fire SessionEnd:** existing `.wip.jsonl` crash
  recovery finalizes on next session_start (already tested).
- **Foreign content in `.claude/settings.json`:** installer parses as
  `map[string]json.RawMessage` and only touches etch entries (same technique as
  Entire's installer); unit test covers preservation.
- **`make build` artifact:** never commit `bin/entire-agent-etch`
  (`git restore bin/` before commit).

## Validation checklist (gates)

- [ ] Real session gate (step 5) — ref exists, transcript attached
- [ ] `make smoke` green incl. new auto-capture steps
- [ ] `go test ./...` green, `make build` green
- [ ] Hook contract documented with correct field names + examples
- [ ] README no longer promises what 0.6.3 can't do; documents tested path
- [ ] PR body carries dogfooding enablement commands

---

## Plan-Review Cycle 1 Resolutions (AUTHORITATIVE — overrides earlier text on conflict)

Review verdict: FAIL (3 MAJOR, 2 MINOR) — reviewer states folding resolutions in
makes it "a clean PASS." All issues resolved below; the two empirical questions
were settled by a live experiment (2026-06-07, Claude Code 2.1.168: temp repo
with stdin-dumping hooks for all six events + real `claude -p` run; dumps at
`/tmp/etch-hookdump-50zg/dump-*.jsonl`).

**MAJOR 1 — `model` is NOT in native Claude Code hook payloads. CONFIRMED.**
The live dump shows no `model` key in any of the six events. Native payloads
carry: `session_id`, `transcript_path`, `cwd`, `hook_event_name`, plus
event-specific fields (`source`; `prompt`; `tool_name`/`tool_input`/
`tool_use_id`/`tool_response`; `reason`; `stop_hook_active`).
**Resolution:** derive model from the transcript JSONL at `transcript_path` —
assistant entries carry `message.model` (verified: `claude-opus-4-8` present in
the test transcript). Implementation: capture `transcript_path` at
session_start; at finalize (session_end), if model is still unset, scan the
transcript for the first `message.model` and backfill. Bounded read, soft-fail
to null with a stderr warning (never break capture). The dialect table's
"`model` (top level, SessionStart)" row is corrected: native dialect has **no**
model field; transcript derivation is the native-path source. `raw_data.model`
remains the Entire-dialect source. Unit test: finalize with a fixture
transcript → model backfilled; missing/unreadable transcript → null + warning.

**MAJOR 2 — Install-side response shapes pinned to Entire v0.6.3 structs.**
All four were in fact read from the v0.6.3 clone (`17720a12`,
`cmd/entire/cli/agent/external/types.go` + `external.go`); the plan now cites
them explicitly:
- `detect` → `DetectResponse{Present bool \`json:"present"\`}` (types.go:33-35)
- `install-hooks` → `HooksInstalledCountResponse{HooksInstalled int \`json:"hooks_installed"\`}` (types.go:63-65)
- `are-hooks-installed` → `AreHooksInstalledResponse{Installed bool \`json:"installed"\`}` (types.go:68-70)
- `uninstall-hooks` → stdout **ignored** by Entire (`external.go:273-276`,
  `_, err := e.run(...)`) — emit `{}` for symmetry; any JSON is safe.
- `info` → `InfoResponse` (types.go:20-30) with `agent.DeclaredCaps`
  (capabilities.go:18-27), `ProtocolVersion == 1` enforced (external.go:48-51).
Smoke asserts `entire enable --agent etch` RC=0 **and** "Installed N hooks for
etch agent" in output **and** etch entries present in `.claude/settings.json`
(proves discovery + install actually ran, not just settings written).

**MAJOR 3 — SessionEnd DOES fire under headless `claude -p`. CONFIRMED.**
The live run produced all six dumps including SessionEnd
(`{"hook_event_name":"SessionEnd","reason":"other",...}`). Stop fires before
SessionEnd, re-confirming the skip-Stop install decision.
**Resolution:** live `claude -p` run stays the primary acceptance gate (now
de-risked by direct evidence); the smoke test's hook-faithful simulation (exact
installed commands, exact captured native payloads) is the deterministic CI
gate. If a future live run yields no ref, the documented finalization fallback
is a second `session_start` (crash recovery) — noted in HOOK_CONTRACT.md.

**MINOR 1 — Version pinning:** all protocol shapes were read from the `v0.6.3`
tag at `17720a12`, which exactly matches the installed binary
(`Entire CLI 0.6.3 (17720a12)`). The PR will state this. Native-dialect shapes
were captured live from Claude Code 2.1.168. Supported range: Entire ≥ 0.6.3
for the `enable --agent etch` path; the standalone `install-hooks` path has no
Entire version dependency at all.

**MINOR 2 — `.claude/settings.json` semantics + coexistence:**
- The file is ordinary committed repo state (Claude Code project settings);
  whether a repo commits or ignores it is the repo's policy — etch follows
  Entire's precedent and does not gitignore it. Documented in README.
- `entire enable --agent etch` is **additive**: setup.go's `--agent` path is "a
  targeted operation: set up this specific agent without affecting other
  agents" (setup.go comment, verified). Etch coexists with Entire's claude-code
  hooks; Claude Code runs all matching hook entries per event (verified live —
  multiple entries all fired). Smoke asserts coexistence: enable claude-code
  first, then etch, assert both `entire hooks claude-code` and
  `entire-agent-etch` entries present and a session ref still appears.
