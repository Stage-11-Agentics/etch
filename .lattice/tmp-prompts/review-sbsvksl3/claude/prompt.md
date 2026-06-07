# Code Review: ETCH-17

You are performing an independent code review. You did NOT write this code —
you are coming in cold with fresh context. Your job is to evaluate the
implementation against the plan and acceptance criteria, surface issues,
and provide a clear verdict.

## Context

### Task
**ID:** ETCH-17

### Task Description
Followed README Configure literally: 'entire enable' + 'setup-refspec'. After that, grep for 'etch' in .entire/.claude/.git/hooks finds NOTHING — the installed Claude Code hooks dispatch to 'entire hooks claude-code ...', never to 'entire-agent-etch'. So real agent sessions are captured by Entire's own engine, not by Etch. README 'Usage' says: 'As an operator you do nothing — once etch is registered, Etch captures every session invisibly.' But the README never gives a working way to REGISTER etch on the version it was tested against: the Entire version note admits 'entire agent add etch' returns 'Unknown agent' on v0.6.3 and that external-agent dispatch 'depends on your Entire version.' Net effect for a naive user: enable, do work, get ZERO refs at refs/etch/sessions/, with no troubleshooting guidance. Even 'make smoke' only proves capture by MANUALLY piping hook-event JSON to the binary (smoke.sh step 4), not by Entire dispatching to etch. Recommendation: README must state plainly that on Entire <= 0.6.3 there is no auto-dispatch, document the exact supported path (which Entire version, or the manual hook-wiring), and stop claiming invisible auto-capture works out of the box.

### Plan
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


### Project Context
# Etch

Flat metadata capture for every AI agent session in a repository, stored as immutable git refs. Built on Entire CLI's hook substrate. Designed for 60–80+ concurrent agents across multiple machines.

- [SPEC.md](./SPEC.md) — requirements and acceptance criteria
- [BUILDPLAN.md](./BUILDPLAN.md) — technical decisions, architecture, ticket breakdown
- [OUTPUT_SPEC.md](./OUTPUT_SPEC.md) — full session record schema and scenario variants
- [PHASE0_RESULTS.md](./PHASE0_RESULTS.md) — Phase 0 validation gate results

**Project home:** `/Users/atin/Projects/Stage11/code/Etch`
**Remote:** `forgejo.stage11.ai:s11/etch`

## Naming

The project is **Etch**. The binary is `entire-agent-etch` (Entire's plugin discovery requires `entire-agent-<name>`). Environment variables use the `ETCH_*` namespace.

## Autonomy default — Fully Autonomous

Lattice orchestrator runs default to Fully Autonomous for this project.

## PR merge policy — auto-merge through to done

## Tech stack

- **Go 1.22+** — single static binary, no runtime dependencies
- **Git plumbing** — `hash-object`, `mktree`, `commit-tree`, `update-ref` via shell exec
- **ULID** — `oklog/ulid` for session IDs
- **No frameworks** — plain subcommand dispatch, `encoding/json`

## Build / test / run

```bash
cd /Users/atin/Projects/Stage11/code/Etch
make build                          # compile ./bin/entire-agent-etch
make test                           # go test ./...
make install PREFIX=$HOME/.local    # install to ~/.local/bin (default PREFIX=/usr/local)
make smoke                          # end-to-end smoke test against the real Entire CLI
make help                           # list all targets
```

See [README.md](./README.md) for the full install + configure guide.

## Key design decisions

- Per-session refs (`refs/etch/sessions/<ULID>`) — zero-contention writes, immutable after creation
- Entire plugin protocol for hook substrate — no need to rebuild 8+ agent runtime integrations
- Agent Trace emission alongside internal format — free interop with Cursor/Cognition ecosystem
- Flat records, no hierarchy — structure emerges from shared identifiers at query time
- Crash recovery via `.wip.jsonl` buffer files — partial records committed on next invocation

## Testing philosophy

Etch is pure git plumbing — every test runs on the filesystem with zero external dependencies. This makes comprehensive testing not just possible but mandatory.

**Unit tests per ticket:** Every ticket ships with tests. No exceptions. A Go binary that touches git refs is trivially testable:
1. Create a temp git repo (`git init` in a tmpdir)
2. Pipe simulated hook events (stdin JSON) to the binary
3. Verify the output: refs exist, session.json is valid, blobs are correct, .wip files behave as expected
4. Clean up

**Test helpers:** Build a shared `testutil` package early (in ETCH-1) that provides:
- `NewTestRepo()` — creates a temp git repo, returns path + cleanup func
- `SimulateHookEvent(subcommand, json)` — runs the binary

### Diff
```
diff --git a/.lattice/events/_lifecycle.jsonl b/.lattice/events/_lifecycle.jsonl
index 1124607..37dd803 100644
--- a/.lattice/events/_lifecycle.jsonl
+++ b/.lattice/events/_lifecycle.jsonl
@@ -32,3 +32,4 @@
 {"actor":"agent:qa-adversarial","data":{"description":"AUDIT ITEMS 3 + 7. commands/setup_refspec.go writes fetch refspec 'refs/etch/sessions/*:refs/etch/sessions/*' WITHOUT the leading '+' that README's manual-equivalent shows ('+refs/etch/sessions/*:...'). Harmless for immutable refs (never force-updated) but a doc/impl divergence. Also it hard-codes remote 'origin'; a repo whose remote is named e.g. 'forgejo' gets no usable refspec and no warning. FIX: align the '+' with README (or fix README), and detect/parameterize the remote name (or warn when origin is absent). Verified empirically in /tmp/etch-refspec (push/fetch/content-match otherwise PASS).","priority":"low","short_id":"ETCH-38","status":"backlog","title":"setup-refspec fetch refspec omits leading '+'; hard-codes remote.origin","type":"bug"},"id":"ev_01KSTXT5M12FD87ZDH2JMDY7XR","schema_version":1,"task_id":"task_01KSTXT5M07WGKA26GXEYF2E9G","ts":"2026-05-29T22:31:24Z","type":"task_created"}
 {"actor":"agent:qa-adversarial","data":{"description":"AUDIT ITEM 6 (secondary). The generic-secret pattern only keys off api_key/api_secret/access_token/secret_key. A multiline .env paste line 'DB_PASS=hunter2password' (and password=, passwd=, pwd=, bare token=) is NOT redacted. Lower severity than the structured-key misses (ETCH-25..28) but common in pasted .env files. FIX: extend generic-secret keyword set to include password|passwd|pwd|token|client_secret. Verified empirically in /tmp/etch-custom (custom patterns from settings.json and the anthropic key inside the same multiline paste WERE redacted).","priority":"low","short_id":"ETCH-39","status":"backlog","title":"Secret scan misses common credential keys (password/passwd/bare token=)","type":"bug"},"id":"ev_01KSTXT5Q8PCNZFVZ3G6KSH6BK","schema_version":1,"task_id":"task_01KSTXT5Q7K00H0AYKX1K95ACC","ts":"2026-05-29T22:31:24Z","type":"task_created"}
 {"actor":{"agent_type":"review","base_name":"Etch-Reviewer","framework":"claude-code","model":"claude-opus-4-8","name":"Etch-Reviewer-1","serial":1,"session":"sess_01KTAH0J43J0KC6GCCMT3DVB8A"},"data":{"complexity":"high","description":"Umbrella remediation ticket for the 2026-06-04 deep code review. THE SPEC IS THE REVIEW FILE: reviews/2026-06-04-deep-code-review.md \u2014 read it first; it has file:line, verified failure scenarios, and refuted non-bugs (do not re-fix those).\n\nScope (10 confirmed findings + 4 below-cut):\n1. Recovery falsely orphans LIVE idle sessions \u2014 capture PID at session_start, wire the liveness check (recovery.go:129; absorbs ETCH-30)\n2. findRepoRoot()=os.Getwd() \u2192 silent session loss \u2014 resolve git common-dir root at the hook boundary (hooks/common.go:39; supersedes-in-part ETCH-34)\n3. Session refs silently overwritable \u2014 create-only update-ref guard (refs/writer.go:47)\n4. Duplicate session_start splits sessions \u2014 reuse existing mapping (session_start.go:38)\n5. Redaction only covers Prompt.Text \u2014 full-record redaction pass at commit boundary (commit.go:24)\n6. local_only_fields unimplemented (config.go:13; absorbs ETCH-31)\n7. OpenAI key regex misses sk-proj-/sk-svcacct- (secrets.go:28; absorbs ETCH-25)\n8. commitSession failure swallowed, printOK lies (session_end.go:62)\n9. Recovery records falsified/lossy \u2014 share ONE wip\u2192session reducer with Finalize (recovery.go:263; absorbs ETCH-33)\n10. tokens never populated \u2014 reconcile spec vs Entire payload reality (buffer.go:159; absorbs ETCH-32)\nBelow-cut: gitDiffFiles rename/quotePath corruption; archive non-atomic quarter (use update-ref --stdin); ScanOrphaned O(N\u00d7M) per start (absorbs ETCH-36); mid-rune prompt truncation.\n\nNOT absorbed (still standalone): ETCH-26/27/28/29/39 (distinct secret-scan patterns the review did not cover).\n\nAcceptance: each fix lands with adversarial tests (hook re-delivery, idle-timeout false positive, commit-failure injection, duplicate session_start) \u2014 the review's thematic conclusion is that these paths were spec'd but never tested.","priority":"critical","short_id":"ETCH-40","status":"backlog","tags":["code-review","remediation"],"title":"Deep code review remediation (2026-06-04): lifecycle, recovery, redaction, data-quality","type":"bug"},"id":"ev_01KTAH0J78TTTNWMEX1SW0G4Q2","schema_version":1,"task_id":"task_01KTAH0J77Q3Y0G6W517EPKMCB","ts":"2026-06-04T23:55:32Z","type":"task_created"}
+{"actor":{"agent_type":"review","base_name":"Etch-Reviewer","framework":"claude-code","model":"claude-opus-4-8","name":"Etch-Reviewer-1","serial":1,"session":"sess_01KTAH0J43J0KC6GCCMT3DVB8A"},"data":{"complexity":"high","description":"DECISION MADE (operator, 2026-06-06): implement for real (not soften docs). Configured local_only_fields must actually stay off the wire.\n\nDesign space (delegator plans, operator-approved direction): projection layer at push time \u2014 e.g. a parallel public ref namespace (refs/etch/public/<ULID>) holding the stripped record, with setup-refspec pushing ONLY the public namespace; or a pre-push hook rewriting outgoing refs. The local ref keeps full fidelity. Immutability holds per-namespace.\n\nConstraints:\n- Coordinates tightly with the refspec/sync batch (ETCH-16/18/24/38) \u2014 same setup-refspec surface; land AFTER that batch.\n- Coordinates with ETCH-40 finding 5 (whole-record redaction pass) \u2014 the projection should reuse the same record-walking machinery.\n- README/settings docs updated to describe the real behavior; until this lands, README marks local_only_fields 'in development' (ETCH-40 finding 6 interim).\nOrigin: ETCH-40 finding 6 / superseded ETCH-31. Review file: reviews/2026-06-04-deep-code-review.md.","priority":"high","short_id":"ETCH-41","status":"backlog","tags":["privacy","refspec"],"title":"Implement local_only_fields: strip-before-push transport for session refs","type":"task"},"id":"ev_01KTF3X71JKMPGXCEC1DZAAC41","schema_version":1,"task_id":"task_01KTF3X71HAKCV8B0B5VEB08BR","ts":"2026-06-06T18:42:43Z","type":"task_created"}
diff --git a/.lattice/events/task_01KSTXHGVBPDWXBETVYB1MX6B7.jsonl b/.lattice/events/task_01KSTXHGVBPDWXBETVYB1MX6B7.jsonl
index 4170636..672e164 100644
--- a/.lattice/events/task_01KSTXHGVBPDWXBETVYB1MX6B7.jsonl
+++ b/.lattice/events/task_01KSTXHGVBPDWXBETVYB1MX6B7.jsonl
@@ -1 +1,3 @@
 {"actor":"agent:qa-userflow","data":{"description":"Followed README Configure literally: 'entire enable' + 'setup-refspec'. After that, grep for 'etch' in .entire/.claude/.git/hooks finds NOTHING \u2014 the installed Claude Code hooks dispatch to 'entire hooks claude-code ...', never to 'entire-agent-etch'. So real agent sessions are captured by Entire's own engine, not by Etch. README 'Usage' says: 'As an operator you do nothing \u2014 once etch is registered, Etch captures every session invisibly.' But the README never gives a working way to REGISTER etch on the version it was tested against: the Entire version note admits 'entire agent add etch' returns 'Unknown agent' on v0.6.3 and that external-agent dispatch 'depends on your Entire version.' Net effect for a naive user: enable, do work, get ZERO refs at refs/etch/sessions/, with no troubleshooting guidance. Even 'make smoke' only proves capture by MANUALLY piping hook-event JSON to the binary (smoke.sh step 4), not by Entire dispatching to etch. Recommendation: README must state plainly that on Entire <= 0.6.3 there is no auto-dispatch, document the exact supported path (which Entire version, or the manual hook-wiring), and stop claiming invisible auto-capture works out of the box.","priority":"high","short_id":"ETCH-17","status":"backlog","title":"No documented working auto-capture path on tested Entire v0.6.3 \u2014 README 'Usage' promise is false in practice","type":"bug"},"id":"ev_01KSTXHGVC7AFXP9MMEMYSMV2R","schema_version":1,"task_id":"task_01KSTXHGVBPDWXBETVYB1MX6B7","ts":"2026-05-29T22:26:40Z","type":"task_created"}
+{"actor":{"agent_type":"review","base_name":"Etch-Reviewer","framework":"claude-code","model":"claude-opus-4-8","name":"Etch-Reviewer-1","serial":1,"session":"sess_01KTAH0J43J0KC6GCCMT3DVB8A"},"data":{"body":"DECISION (operator, 2026-06-06): promoted to Wave 0 as an INVESTIGATION lane, not a docs task. Investigate WHY Entire 0.6.3 never dispatches to entire-agent-etch (plugin discovery? hook registration? config?), determine the real fix \u2014 code, config, or documented manual wiring \u2014 implement it, then ENABLE DOGFOODING on the Etch repo itself. Every other fix in this run is unverifiable in production until capture actually runs; dogfooding is the project's stated integration test."},"id":"ev_01KTF3XHDZR9HXY5XQHPGYVJM3","schema_version":1,"task_id":"task_01KSTXHGVBPDWXBETVYB1MX6B7","ts":"2026-06-06T18:42:54Z","type":"comment_added"}
+{"actor":{"agent_type":"review","base_name":"Etch-Reviewer","framework":"claude-code","model":"claude-opus-4-8","name":"Etch-Reviewer-1","serial":1,"session":"sess_01KTAH0J43J0KC6GCCMT3DVB8A"},"data":{"field":"priority","from":"high","to":"critical"},"id":"ev_01KTF3XHH49FQFVWYJJVKQGA8J","schema_version":1,"task_id":"task_01KSTXHGVBPDWXBETVYB1MX6B7","ts":"2026-06-06T18:42:54Z","type":"field_updated"}
diff --git a/.lattice/events/task_01KSTXJG3MWS6CJWNQV1VVYF6Q.jsonl b/.lattice/events/task_01KSTXJG3MWS6CJWNQV1VVYF6Q.jsonl
index c2bb2d3..90bb01c 100644
--- a/.lattice/events/task_01KSTXJG3MWS6CJWNQV1VVYF6Q.jsonl
+++ b/.lattice/events/task_01KSTXJG3MWS6CJWNQV1VVYF6Q.jsonl
@@ -1 +1,3 @@
 {"actor":"agent:qa-userflow","data":{"description":"I passed session_id='01KSTCORRECTFIELDS00000001' across all hook events. Etch correctly correlated them into ONE session, but the resulting session.json session_id is etch's own minted ULID (e.g. 01KSTXE5N2...). The agent/runtime session_id I provided is not stored in any field I could find. For a tool whose value is correlating agent sessions, dropping the upstream session_id makes cross-referencing with Entire/Claude Code logs harder. May be intentional \u2014 if so, consider adding an 'agent_session_id' field to preserve the upstream id.","priority":"low","short_id":"ETCH-23","status":"backlog","title":"Agent's own session_id is discarded; output session_id is etch's minted ULID only","type":"bug"},"id":"ev_01KSTXJG3S862GSPAFASC0YVER","schema_version":1,"task_id":"task_01KSTXJG3MWS6CJWNQV1VVYF6Q","ts":"2026-05-29T22:27:12Z","type":"task_created"}
+{"actor":{"agent_type":"review","base_name":"Etch-Reviewer","framework":"claude-code","model":"claude-opus-4-8","name":"Etch-Reviewer-1","serial":1,"session":"sess_01KTAH0J43J0KC6GCCMT3DVB8A"},"data":{"body":"DECISION (operator, 2026-06-06): YES \u2014 add optional agent_session_id field to etch.session.v1, populated from the hook payload's session id (already in hand at every hook; it keys the mapping file). Non-breaking schema addition. It is the join key to Claude Code transcripts, c11 surface manifests, and resume flows."},"id":"ev_01KTF3X6N48HV5JR3YETB0J78Z","schema_version":1,"task_id":"task_01KSTXJG3MWS6CJWNQV1VVYF6Q","ts":"2026-06-06T18:42:43Z","type":"comment_added"}
+{"actor":{"agent_type":"review","base_name":"Etch-Reviewer","framework":"claude-code","model":"claude-opus-4-8","name":"Etch-Reviewer-1","serial":1,"session":"sess_01KTAH0J43J0KC6GCCMT3DVB8A"},"data":{"field":"priority","from":"low","to":"medium"},"id":"ev_01KTF3X6R7BVB12PBQ1RPSV1VW","schema_version":1,"task_id":"task_01KSTXJG3MWS6CJWNQV1VVYF6Q","ts":"2026-06-06T18:42:43Z","type":"field_updated"}
diff --git a/.lattice/events/task_01KSTXT5GR056EE9PK26744T01.jsonl b/.lattice/events/task_01KSTXT5GR056EE9PK26744T01.jsonl
index 6d308d0..ef36ff2 100644
--- a/.lattice/events/task_01KSTXT5GR056EE9PK26744T01.jsonl
+++ b/.lattice/events/task_01KSTXT5GR056EE9PK26744T01.jsonl
@@ -1 +1,2 @@
 {"actor":"agent:qa-adversarial","data":{"description":"AUDIT ITEM 7 (doc audit) + SPEC #7. README.md line ~107: 'the hostname is stored as a salted hash.' ACTUAL: capture/machine.go uses sha256.Sum256([]byte(hostname)) with NO salt (grep 'salt' -> none). OUTPUT_SPEC.md says only 'SHA-256 of hostname'. Two problems: (1) README is factually wrong ('salted'). (2) An UNSALTED hash of a low-entropy hostname (e.g. 'Hyperion', 'Atlas') is trivially reversible by brute force/rainbow table, so the privacy protection is weak -- and identical across repos/machines, enabling correlation. FIX: either add a real per-repo salt and keep the 'salted' claim, or correct the README to 'SHA-256 hash' and document the reversibility limitation.","priority":"medium","short_id":"ETCH-37","status":"backlog","title":"README claims 'salted hash' for hostname but implementation is unsalted SHA-256","type":"bug"},"id":"ev_01KSTXT5GSKCKYF608D441MCEX","schema_version":1,"task_id":"task_01KSTXT5GR056EE9PK26744T01","ts":"2026-05-29T22:31:23Z","type":"task_created"}
+{"actor":{"agent_type":"review","base_name":"Etch-Reviewer","framework":"claude-code","model":"claude-opus-4-8","name":"Etch-Reviewer-1","serial":1,"session":"sess_01KTAH0J43J0KC6GCCMT3DVB8A"},"data":{"body":"DECISION (operator, 2026-06-06): implement per-repo salt. Generate a random salt at first init, store in .etch/settings.json (committed so all clones share it \u2014 cross-machine correlation within the repo keeps working), hash = SHA-256(salt + hostname). Makes the README's 'salted hash' claim true rather than editing it to be less private."},"id":"ev_01KTF3X6VBZZ3SYG5DE62WRDK4","schema_version":1,"task_id":"task_01KSTXT5GR056EE9PK26744T01","ts":"2026-06-06T18:42:43Z","type":"comment_added"}
diff --git a/.lattice/events/task_01KTAH0J77Q3Y0G6W517EPKMCB.jsonl b/.lattice/events/task_01KTAH0J77Q3Y0G6W517EPKMCB.jsonl
index 3836161..16ac34d 100644
--- a/.lattice/events/task_01KTAH0J77Q3Y0G6W517EPKMCB.jsonl
+++ b/.lattice/events/task_01KTAH0J77Q3Y0G6W517EPKMCB.jsonl
@@ -7,3 +7,4 @@
 {"actor":{"agent_type":"review","base_name":"Etch-Reviewer","framework":"claude-code","model":"claude-opus-4-8","name":"Etch-Reviewer-1","serial":1,"session":"sess_01KTAH0J43J0KC6GCCMT3DVB8A"},"data":{"note":"Absorbed into umbrella remediation; review file has fuller scenario + fix direction","target_task_id":"task_01KSTXT5D6H65S8TRAADNZYVH3","type":"supersedes"},"id":"ev_01KTAH1C0VR4WT11GZVAWGES39","schema_version":1,"task_id":"task_01KTAH0J77Q3Y0G6W517EPKMCB","ts":"2026-06-04T23:55:59Z","type":"relationship_added"}
 {"actor":{"agent_type":"review","base_name":"Etch-Reviewer","framework":"claude-code","model":"claude-opus-4-8","name":"Etch-Reviewer-1","serial":1,"session":"sess_01KTAH0J43J0KC6GCCMT3DVB8A"},"data":{"artifact_id":"art_01KTAH22XMPVSTR5DZM3DDN0J2"},"id":"ev_01KTAH22XP02GXMD03561JH2B9","schema_version":1,"task_id":"task_01KTAH0J77Q3Y0G6W517EPKMCB","ts":"2026-06-04T23:56:22Z","type":"artifact_attached"}
 {"actor":{"agent_type":"review","base_name":"Etch-Reviewer","framework":"claude-code","model":"claude-opus-4-8","name":"Etch-Reviewer-1","serial":1,"session":"sess_01KTAH0J43J0KC6GCCMT3DVB8A"},"data":{"note":"Finding #2 = same root cause; ETCH-34 kept standalone per board hygiene decision","target_task_id":"task_01KSTXS9TY054Y1S6GSPNDQ1TV","type":"related_to"},"id":"ev_01KTAH2BHD4R17SVS471WFB3CE","schema_version":1,"task_id":"task_01KTAH0J77Q3Y0G6W517EPKMCB","ts":"2026-06-04T23:56:31Z","type":"relationship_added"}
+{"actor":{"agent_type":"review","base_name":"Etch-Reviewer","framework":"claude-code","model":"claude-opus-4-8","name":"Etch-Reviewer-1","serial":1,"session":"sess_01KTAH0J43J0KC6GCCMT3DVB8A"},"data":{"body":"DECISIONS (operator, 2026-06-06) for the two decision-first findings:\n- Finding 6 (local_only_fields): IMPLEMENT real strip-before-push \u2014 spun out as its own ticket (see ETCH-41) since it is a design+impl project touching the refspec story. Interim: this finding is satisfied for ETCH-40 closeout by the new ticket existing and README marking the feature 'in development' until it lands.\n- Finding 10 (tokens): DROP from v1 spec \u2014 amend OUTPUT_SPEC to mark tokens null-in-v1/reserved, delete the dead aggregation paths (capture + recovery), file v2 enrichment as future work. Honest null beats a number that is wrong for 7 of 8 runtimes."},"id":"ev_01KTF3X6YGQ92PVTX7TR4PNQW8","schema_version":1,"task_id":"task_01KTAH0J77Q3Y0G6W517EPKMCB","ts":"2026-06-06T18:42:43Z","type":"comment_added"}
diff --git a/.lattice/events/task_01KTF3X71HAKCV8B0B5VEB08BR.jsonl b/.lattice/events/task_01KTF3X71HAKCV8B0B5VEB08BR.jsonl
new file mode 100644
index 0000000..2889ee7
--- /dev/null
+++ b/.lattice/events/task_01KTF3X71HAKCV8B0B5VEB08BR.jsonl
@@ -0,0 +1,3 @@
+{"actor":{"agent_type":"review","base_name":"Etch-Reviewer","framework":"claude-code","model":"claude-opus-4-8","name":"Etch-Reviewer-1","serial":1,"session":"sess_01KTAH0J43J0KC6GCCMT3DVB8A"},"data":{"complexity":"high","description":"DECISION MADE (operator, 2026-06-06): implement for real (not soften docs). Configured local_only_fields must actually stay off the wire.\n\nDesign space (delegator plans, operator-approved direction): projection layer at push time \u2014 e.g. a parallel public ref namespace (refs/etch/public/<ULID>) holding the stripped record, with setup-refspec pushing ONLY the public namespace; or a pre-push hook rewriting outgoing refs. The local ref keeps full fidelity. Immutability holds per-namespace.\n\nConstraints:\n- Coordinates tightly with the refspec/sync batch (ETCH-16/18/24/38) \u2014 same setup-refspec surface; land AFTER that batch.\n- Coordinates with ETCH-40 finding 5 (whole-record redaction pass) \u2014 the projection should reuse the same record-walking machinery.\n- README/settings docs updated to describe the real behavior; until this lands, README marks local_only_fields 'in development' (ETCH-40 finding 6 interim).\nOrigin: ETCH-40 finding 6 / superseded ETCH-31. Review file: reviews/2026-06-04-deep-code-review.md.","priority":"high","short_id":"ETCH-41","status":"backlog","tags":["privacy","refspec"],"title":"Implement local_only_fields: strip-before-push transport for session refs","type":"task"},"id":"ev_01KTF3X71JKMPGXCEC1DZAAC41","schema_version":1,"task_id":"task_01KTF3X71HAKCV8B0B5VEB08BR","ts":"2026-06-06T18:42:43Z","type":"task_created"}
+{"actor":{"agent_type":"review","base_name":"Etch-Reviewer","framework":"claude-code","model":"claude-opus-4-8","name":"Etch-Reviewer-1","serial":1,"session":"sess_01KTAH0J43J0KC6GCCMT3DVB8A"},"data":{"note":"Finding 6 decision: implement strip-before-push","target_task_id":"task_01KTAH0J77Q3Y0G6W517EPKMCB","type":"spawned_by"},"id":"ev_01KTF3XH7QX84VTF182ZNPWTME","schema_version":1,"task_id":"task_01KTF3X71HAKCV8B0B5VEB08BR","ts":"2026-06-06T18:42:54Z","type":"relationship_added"}
+{"actor":{"agent_type":"review","base_name":"Etch-Reviewer","framework":"claude-code","model":"claude-opus-4-8","name":"Etch-Reviewer-1","serial":1,"session":"sess_01KTAH0J43J0KC6GCCMT3DVB8A"},"data":{"note":"Same setup-refspec surface; land after refspec/sync batch","target_task_id":"task_01KSTXH7RCZ6JYQ98W0PQSDC6D","type":"depends_on"},"id":"ev_01KTF3XHAV2BHAYD5NGR77XR0X","schema_version":1,"task_id":"task_01KTF3X71HAKCV8B0B5VEB08BR","ts":"2026-06-06T18:42:54Z","type":"relationship_added"}
diff --git a/.lattice/ids.json b/.lattice/ids.json
index 9cc4707..dac9dc4 100644
--- a/.lattice/ids.json
+++ b/.lattice/ids.json
@@ -35,6 +35,7 @@
     "ETCH-39": "task_01KSTXT5Q7K00H0AYKX1K95ACC",
     "ETCH-4": "task_01KSKTYEP1BN8JNAVC0E191YHS",
     "ETCH-40": "task_01KTAH0J77Q3Y0G6W517EPKMCB",
+    "ETCH-41": "task_01KTF3X71HAKCV8B0B5VEB08BR",
     "ETCH-5": "task_01KSKTYERZEX3XHEN5WEZM7W2M",
     "ETCH-6": "task_01KSKTYEVX64KKVDAEAE9Q2SV6",
     "ETCH-7": "task_01KSKTYEYXPJRH8DDCB95YP7QE",
@@ -42,7 +43,7 @@
     "ETCH-9": "task_01KSRK9T1CFYCAD4SX8TN7G6SQ"
   },
   "next_seqs": {
-    "ETCH": 41
+    "ETCH": 42
   },
   "schema_version": 2
 }
diff --git a/.lattice/orchestration/orchestrator-boot-prompt.md b/.lattice/orchestration/orchestrator-boot-prompt.md
index cf806b0..19d761c 100644
--- a/.lattice/orchestration/orchestrator-boot-prompt.md
+++ b/.lattice/orchestration/orchestrator-boot-prompt.md
@@ -4,14 +4,13 @@ You are the Etch backlog-completion orchestrator. Work in:
 
 `/Users/atin/Projects/Stage11/code/Etch`
 
-Read these files first:
+Read these files first, in this order:
 
 - `CLAUDE.md`
-- `SPEC.md`
-- `BUILDPLAN.md`
-- `.lattice/orchestration/run-state.md`
+- `.lattice/orchestration/run-state.md` — the authoritative run plan, including the **Operator Decisions (2026-06-06)** table
+- `reviews/2026-06-04-deep-code-review.md` — the spec for all ETCH-40 findings (includes refuted non-bugs; delegators must not "fix" those)
 - `.lattice/orchestration/validation-plan.md`
-- `.lattice/orchestration/next-run-prep.md`
+- `SPEC.md`, `BUILDPLAN.md` (background)
 
 Use live Lattice state as the source of truth for ticket status:
 
@@ -19,25 +18,24 @@ Use live Lattice state as the source of truth for ticket status:
 lattice list --status backlog --json
 ```
 
-Important constraints:
-
-- Do not dispatch delegators until the operator explicitly approves launch.
-- Do not assume older run-state content from ETCH-1 through ETCH-14 is current.
-- Use the 24 backlog tickets ETCH-16 through ETCH-39.
-- Keep the initial concurrent delegator cap at 5 unless the operator changes it.
-- Use the worker batch plan in `run-state.md`; do not blindly spawn one independent worker per ticket when the batch plan groups shared-file tickets.
-- Leave PRs at `pr_open` for human review unless the operator explicitly opts into auto-merge.
-- Avoid reverting existing dirty/untracked Lattice artifacts.
-- Treat ETCH-17/ETCH-20, ETCH-23, ETCH-31, ETCH-32, and ETCH-37 as decision-first tickets before implementation.
-- Treat ETCH-22 as subsumed by ETCH-38 unless the operator wants separate closure.
-
-Before launching, present the operator with:
-
-1. Wave plan and ticket list.
-2. Worker batch plan and workflow mode per batch/ticket.
-3. Dependency/overlap risks.
-4. Product decisions required before coding.
-5. Validation gates.
-6. Final confirmation request.
-
-If the operator approves, spawn delegators according to `.lattice/orchestration/run-state.md`.
+## Launch status
+
+**Dispatch is APPROVED (operator, 2026-06-06).** Do not re-ask for launch confirmation. Begin Wave 0 immediately after orienting.
+
+## Constraints
+
+- Scope: all backlog tickets — ETCH-16 through ETCH-24, ETCH-26 through ETCH-29, ETCH-34/35/37/38/39, ETCH-40, ETCH-41. ETCH-25/30/31/32/33/36 are cancelled (superseded by ETCH-40); ETCH-14 is cancelled history.
+- Use the worker batch plan and wave table in `run-state.md`; do not spawn one worker per ticket when the plan groups shared-file tickets.
+- Wave 0 = three parallel lanes: redaction, repo-root, auto-capture investigation (ETCH-17+20).
+- Wave 1 lifecycle/recovery worker MUST wait for the repo-root PR to land.
+- ETCH-41 (local-only transport) MUST wait for the refspec/sync batch to land.
+- Concurrent delegator cap: 5.
+- **PR merge policy: auto-merge through to done** once a delegator's review gates pass (operator decision 2026-06-06).
+- All formerly decision-first items are RESOLVED — the Operator Decisions table in run-state.md records each decision. Delegators implement them; nobody re-litigates or re-asks.
+- Every delegator touching ETCH-40 findings reads the review file first and comments per-finding progress on ETCH-40. Only the closeout audit moves ETCH-40 to done.
+- ETCH-40 acceptance requires adversarial tests: hook re-delivery, idle-timeout false positive, commit-failure injection, duplicate session_start.
+- Treat ETCH-22 as subsumed by ETCH-38; close with an explicit Lattice note.
+- Avoid reverting existing dirty/untracked artifacts (`bin/entire-agent-etch`, `.claude/scheduled_tasks.lock`).
+- Validation gates per `validation-plan.md`; baseline (test/build/smoke/density) was green at prep time — re-verify before Wave 0 dispatch and after each wave.
+- Dogfooding: once ETCH-17's fix lands, enable capture on this repo so the rest of the run live-validates Etch against itself. Check `git for-each-ref refs/etch/sessions/` periodically thereafter — refs appearing is the success signal.
+- Report status to the c11 sidebar (`set-status`, `set-progress`, `log`) and keep your tab title/description current.
diff --git a/.lattice/orchestration/run-state.md b/.lattice/orchestration/run-state.md
index 7918a9a..2ef554f 100644
--- a/.lattice/orchestration/run-state.md
+++ b/.lattice/orchestration/run-state.md
@@ -19,12 +19,24 @@ Status: prep only; no orchestrator or delegators launched.
 - **Autonomy level:** Fully Autonomous, matching project `CLAUDE.md`.
 - **Concurrent delegator cap (N):** 5 initial cap.
 - **Auto-close finished delegator surfaces:** Yes.
-- **PR merge policy:** Leave at `pr_open` for human review.
+- **PR merge policy:** **Auto-merge through to done** (operator decision 2026-06-06, honoring project `CLAUDE.md`; supersedes the earlier `pr_open` setting). The run's own review gates (plan review → code review → fix loop, master validator, closeout audit) are the control.
 - **Ticket fidelity:** Existing Lattice backlog tickets are authoritative; delegators should read their task JSON and linked plan artifacts.
 - **Master Validator:** On.
 - **Closeout audit:** On.
 - **Result Validator (Phase 4):** On.
-- **Dispatch status:** Not launched. Await explicit operator approval.
+- **Dispatch status:** **APPROVED 2026-06-06.** Operator settled all seven open decisions and approved full-scope dispatch.
+
+## Operator Decisions (2026-06-06) — all decision-first items resolved
+
+| Item | Decision |
+|---|---|
+| local_only_fields (ETCH-40 f.6 / was ETCH-31) | **Implement** real strip-before-push → spun out as **ETCH-41** (high), lands after refspec batch. README marks feature 'in development' until then. |
+| tokens (ETCH-40 f.10 / was ETCH-32) | **Drop from v1 spec**: OUTPUT_SPEC amended to null-in-v1/reserved; delete dead aggregation paths; v2 enrichment is future work. |
+| hostname hash (ETCH-37) | **Per-repo salt**: random salt at first init, stored in committed `.etch/settings.json`; hash = SHA-256(salt+hostname). |
+| upstream session id (ETCH-23) | **Add** optional `agent_session_id` to the schema from the hook payload's session id. Priority raised to medium. |
+| PR merge policy | **Auto-merge all** through to done. |
+| ETCH-17 | **Promoted to Wave 0 investigation lane** (priority→critical): root-cause Entire 0.6.3 dispatch failure, implement the real fix, then enable dogfooding on this repo. |
+| Run scope | **Everything** — all backlog tickets + ETCH-40 + ETCH-41. |
 
 ## Current c11 Topology
 
@@ -33,8 +45,8 @@ Status: prep only; no orchestrator or delegators launched.
 | workspace | `workspace:14` | Current Etch workspace |
 | main view area | `pane:52` | Current operator/session terminal, surface `surface:217` |
 | control surface | `pane:58` | Lattice board browser selected on `surface:107`, URL `http://localhost:55492/` |
-| prep/delegate pane | `pane:60` | Contains `surface:218` titled `Orchestrator Prep` |
-| prepared boot surface | `surface:218` | Prep-only shell surface; no agent running |
+| prep/delegate pane | `pane:60` | Orchestrator running in `surface:387` ("Etch Orchestrator") |
+| orchestrator surface | `surface:387` | claude-code orchestrator, launched 2026-06-07 |
 
 ## Live Lattice Summary
 
@@ -63,10 +75,11 @@ The Lattice tickets remain the unit of tracking. ETCH-40 spans multiple file sur
 | Redaction completeness | ETCH-26, ETCH-27, ETCH-28, ETCH-29, ETCH-39 **+ ETCH-40 findings 5, 7** | One inline-full worker | Same surface: `internal/redact` patterns + the commit-boundary redaction pass (finding 5 moves redaction from per-field to whole-record — do this first, then patterns). |
 | Repo-root + no-git safety | ETCH-34, ETCH-35 **+ ETCH-40 finding 2** | One inline-full worker | Same root cause: `findRepoRoot()=os.Getwd()`. Fix the boundary once (git common-dir resolution), and no-git behavior falls out of the same code path. |
 | Lifecycle/recovery integrity | **ETCH-40 findings 1, 3, 4, 8, 9 + below-cut (scan perf, gitDiffFiles, archive atomicity, utf8)** | One inline-full worker, sequential inside; may split into 2 PRs (lifecycle guards / recovery-parity refactor) | Replaces the old Recovery/perf batch (ETCH-30/33/36 superseded). Depends on repo-root batch landing first. Finding 9's fix (shared wip→session reducer) subsumes ETCH-33's double-count. |
+| Auto-capture investigation | **ETCH-17 (Wave 0, critical)** | One inline-full worker: investigate → fix → enable dogfooding on this repo | Existential: zero real sessions ever captured; Entire 0.6.3 never dispatches to the plugin. Once fixed, the rest of the run live-validates itself via dogfooding. ETCH-20 (hook contract docs) rides with it. |
 | Refspec/sync | ETCH-16, ETCH-18, ETCH-22, ETCH-24, ETCH-38 | One inline-full worker | One coherent `setup-refspec` and transport story; ETCH-22 is subsumed by ETCH-38. |
-| Hook/docs | ETCH-17, ETCH-20 | Dialogue/product decision first, then inline-full | Auto-capture docs depend on supported Entire versions or manual hook wiring strategy. |
-| CLI/docs UX | ETCH-19, ETCH-21 | One fast-track worker | Shared README/CLI discoverability surface; low risk if coordinated with Hook/docs. |
-| Product decisions | ETCH-23, ETCH-37, **ETCH-40 findings 6 (local_only_fields) and 10 (tokens)** | Decision first; implement only after decision | Privacy contract, token source, hostname salt, and session-id semantics should not be guessed by delegators. |
+| Local-only transport | **ETCH-41** (Wave 2, after refspec lands) | One inline-full worker | Strip-before-push projection (decision: implement). Same setup-refspec surface as the refspec batch; reuses finding 5's record-walking machinery. |
+| CLI/docs UX | ETCH-19, ETCH-21 | One fast-track worker | Shared README/CLI discoverability surface. |
+| Decided schema/privacy items | ETCH-23 (agent_session_id), ETCH-37 (per-repo salt), ETCH-40 f.10 (drop tokens from v1 spec) | One inline-full worker — decisions are made, see Operator Decisions table | All three touch schema/OUTPUT_SPEC/README; small coordinated PR. |
 
 ## Ticket Wave Plan
 
@@ -86,28 +99,29 @@ The Lattice tickets remain the unit of tracking. ETCH-40 spans multiple file sur
 | ETCH-22 | `task_01KSTXJ73G61BENR2PQ2S17HP8` | low | 1 | ETCH-38 | inline-full grouped | backlog | Subsumed by ETCH-38; same refspec/sync batch |
 | ETCH-24 | `task_01KSTXJG8Q2ZWH849E12QJKHCV` | low | 1 | — | inline-full grouped | backlog | Refspec/sync batch |
 | ETCH-38 | `task_01KSTXT5M07WGKA26GXEYF2E9G` | low | 1 | — | inline-full grouped | backlog | Refspec/sync batch |
-| ETCH-17 | `task_01KSTXHGVBPDWXBETVYB1MX6B7` | high | 2 | decision: auto-capture path | inline-full grouped | backlog | Hook/docs batch |
-| ETCH-20 | `task_01KSTXJ6XBQQJREX5D9X06G6PX` | medium | 2 | decision: hook contract | inline-full grouped | backlog | Hook/docs batch |
+| ETCH-17 | `task_01KSTXHGVBPDWXBETVYB1MX6B7` | critical | 0 | — (decision made: investigate→fix→dogfood) | inline-full | backlog | Auto-capture investigation lane |
+| ETCH-20 | `task_01KSTXJ6XBQQJREX5D9X06G6PX` | medium | 0 | ETCH-17 (rides with it) | inline-full grouped | backlog | Hook contract docs, same worker as ETCH-17 |
 | ETCH-19 | `task_01KSTXHTANQNSZQS44241G8EFF` | medium | 2 | — | fast-track grouped | backlog | CLI/docs UX batch |
 | ETCH-21 | `task_01KSTXJ70DMDC6V60EB80RF900` | medium | 2 | — | fast-track grouped | backlog | CLI/docs UX batch |
-| ETCH-23 | `task_01KSTXJG3MWS6CJWNQV1VVYF6Q` | low | 3 | decision: schema | decision-first | backlog | Preserve upstream agent session ID only if product decision says yes |
-| ETCH-37 | `task_01KSTXT5GR056EE9PK26744T01` | medium | 3 | decision: salt vs docs | decision-first | backlog | Implement per-repo salt or correct README limitation |
-| _(ETCH-40 f.6)_ | — | high | 3 | decision: privacy contract | decision-first | — | local_only_fields: implement real local-only transport or remove/soften promise (was ETCH-31) |
-| _(ETCH-40 f.10)_ | — | medium | 3 | decision: token source | decision-first | — | tokens: hook raw data carries none — decide transcript parse, `calculate-tokens`, or drop from OUTPUT_SPEC (was ETCH-32) |
+| ETCH-23 | `task_01KSTXJG3MWS6CJWNQV1VVYF6Q` | medium | 2 | — (decision made: add agent_session_id) | inline-full grouped | backlog | Decided schema/privacy batch |
+| ETCH-37 | `task_01KSTXT5GR056EE9PK26744T01` | medium | 2 | — (decision made: per-repo salt) | inline-full grouped | backlog | Decided schema/privacy batch |
+| _(ETCH-40 f.10)_ | — | medium | 2 | — (decision made: drop tokens from v1 spec) | inline-full grouped | — | Decided schema/privacy batch (was ETCH-32) |
+| ETCH-41 | `task_01KTF3X71HAKCV8B0B5VEB08BR` | high | 2 | ETCH-16 (refspec batch lands first) | inline-full | backlog | local_only_fields strip-before-push (decision made: implement); spawned by ETCH-40 f.6 |
 
 ## Dispatch Guidance
 
-1. Do not dispatch until the operator explicitly approves.
-2. Start with Wave 0 after approval: one redaction worker (ETCH-26/27/28/29/39 + ETCH-40 findings 5,7) and one repo-root worker (ETCH-34/35 + ETCH-40 finding 2) can run in parallel without overlapping files.
+1. ~~Do not dispatch until the operator explicitly approves.~~ **Dispatch approved 2026-06-06.**
+2. Wave 0 runs three parallel lanes: redaction worker (ETCH-26/27/28/29/39 + ETCH-40 findings 5,7), repo-root worker (ETCH-34/35 + ETCH-40 finding 2), and auto-capture investigation (ETCH-17 + ETCH-20 docs).
 3. Do not split the redaction tickets across workers.
 4. Wave 1 lifecycle/recovery worker (ETCH-40 findings 1,3,4,8,9 + below-cut) MUST wait for the repo-root PR to land — finding 1's recovery fix and finding 2's path anchoring touch the same scan logic.
 5. Every ETCH-40 delegator reads `reviews/2026-06-04-deep-code-review.md` first. It contains refuted non-bugs (exit_reason clobber, index races, worktree diff-dir) — do not "fix" those. Acceptance requires adversarial tests: hook re-delivery, idle-timeout false positive, commit-failure injection, duplicate session_start.
 6. Treat ETCH-22 and ETCH-38 as overlapping. Prefer one implementation path and close/mark duplicate only with an explicit Lattice note.
-7. Treat ETCH-17/ETCH-20, ETCH-23, ETCH-37, and ETCH-40 findings 6/10 as decision-first. For finding 6 (local_only_fields) specifically, choose before coding: implement a real local-only transport model, or remove/soften the README/settings privacy promise until a design exists.
-8. Keep PRs at `pr_open` for human review because several tickets change security/privacy behavior.
+7. All decision-first items are RESOLVED — see the Operator Decisions table. Delegators implement the recorded decisions; do not re-litigate them.
+8. PRs auto-merge through to done once review gates pass (operator decision 2026-06-06).
 9. ETCH-40 status: delegators comment per-finding progress on ETCH-40; only the closeout audit moves it to done, after verifying every finding in the review file is addressed or explicitly deferred.
 
 ## Handoff Log
 
 - 2026-06-03 — Prep corrected after audit. Current backlog, topology, wave plan, and validation gates recorded. No agents launched.
 - 2026-06-06 — Reconciled with the 2026-06-04 deep code review: ETCH-40 (critical umbrella) added and distributed across Wave 0/1 workers; ETCH-25/30/31/32/33/36 superseded→cancelled; batch plan and dispatch guidance rewritten accordingly. No agents launched; operator gate still in effect.
+- 2026-06-06 (later) — Operator settled all seven open decisions (see Operator Decisions table), created ETCH-41, promoted ETCH-17 to Wave 0 critical, switched merge policy to auto-merge, and **approved full-scope dispatch**. Orchestrator launched into `surface:387` (pane:60; the original prep surface:218 had been closed). Launched 2026-06-07.
diff --git a/.lattice/plans/task_01KTF3X71HAKCV8B0B5VEB08BR.md b/.lattice/plans/task_01KTF3X71HAKCV8B0B5VEB08BR.md
new file mode 100644
index 0000000..08c798e
--- /dev/null
+++ b/.lattice/plans/task_01KTF3X71HAKCV8B0B5VEB08BR.md
@@ -0,0 +1,11 @@
+# ETCH-41: Implement local_only_fields: strip-before-push transport for session refs
+
+DECISION MADE (operator, 2026-06-06): implement for real (not soften docs). Configured local_only_fields must actually stay off the wire.
+
+Design space (delegator plans, operator-approved direction): projection layer at push time — e.g. a parallel public ref namespace (refs/etch/public/<ULID>) holding the stripped record, with setup-refspec pushing ONLY the public namespace; or a pre-push hook rewriting outgoing refs. The local ref keeps full fidelity. Immutability holds per-namespace.
+
+Constraints:
+- Coordinates tightly with the refspec/sync batch (ETCH-16/18/24/38) — same setup-refspec surface; land AFTER that batch.
+- Coordinates with ETCH-40 finding 5 (whole-record redaction pass) — the projection should reuse the same record-walking machinery.
+- README/settings docs updated to describe the real behavior; until this lands, README marks local_only_fields 'in development' (ETCH-40 finding 6 interim).
+Origin: ETCH-40 finding 6 / superseded ETCH-31. Review file: reviews/2026-06-04-deep-code-review.md.
diff --git a/.lattice/sessions/Etch-Reviewer-1.json b/.lattice/sessions/Etch-Reviewer-1.json
index 152b21b..06e98e1 100644
--- a/.lattice/sessions/Etch-Reviewer-1.json
+++ b/.lattice/sessions/Etch-Reviewer-1.json
@@ -2,7 +2,7 @@
   "agent_type": "review",
   "base_name": "Etch-Reviewer",
   "framework": "claude-code",
-  "last_active": "2026-06-04T23:56:31Z",
+  "last_active": "2026-06-06T18:42:54Z",
   "model": "claude-opus-4-8",
   "name": "Etch-Reviewer-1",
   "parent": "human:atin",
diff --git a/.lattice/tasks/task_01KSTXHGVBPDWXBETVYB1MX6B7.json b/.lattice/tasks/task_01KSTXHGVBPDWXBETVYB1MX6B7.json
index 2c27bc7..7412d8a 100644
--- a/.lattice/tasks/task_01KSTXHGVBPDWXBETVYB1MX6B7.json
+++ b/.lattice/tasks/task_01KSTXHGVBPDWXBETVYB1MX6B7.json
@@ -1,7 +1,7 @@
 {
   "assigned_to": null,
   "branch_links": [],
-  "comment_count": 0,
+  "comment_count": 1,
   "complexity": null,
   "created_at": "2026-05-29T22:26:40Z",
   "created_by": "agent:qa-userflow",
@@ -10,10 +10,10 @@
   "done_at": null,
   "evidence_refs": [],
   "id": "task_01KSTXHGVBPDWXBETVYB1MX6B7",
-  "last_event_id": "ev_01KSTXHGVC7AFXP9MMEMYSMV2R",
+  "last_event_id": "ev_01KTF3XHH49FQFVWYJJVKQGA8J",
   "last_status_changed_at": "2026-05-29T22:26:40Z",
   "linked_files": [],
-  "priority": "high",
+  "priority": "critical",
   "relationships_out": [],
   "reopened_count": 0,
   "schema_version": 1,
@@ -22,6 +22,6 @@
   "tags": null,
   "title": "No documented working auto-capture path on tested Entire v0.6.3 \u2014 README 'Usage' promise is false in practice",
   "type": "bug",
-  "updated_at": "2026-05-29T22:26:40Z",
+  "updated_at": "2026-06-06T18:42:54Z",
   "urgency": null
 }
diff --git a/.lattice/tasks/task_01KSTXJG3MWS6CJWNQV1VVYF6Q.json b/.lattice/tasks/task_01KSTXJG3MWS6CJWNQV1VVYF6Q.json
index 2d447a9..a04c5cf 100644
--- a/.lattice/tasks/task_01KSTXJG3MWS6CJWNQV1VVYF6Q.json
+++ b/.lattice/tasks/task_01KSTXJG3MWS6CJWNQV1VVYF6Q.json
@@ -1,7 +1,7 @@
 {
   "assigned_to": null,
   "branch_links": [],
-  "comment_count": 0,
+  "comment_count": 1,
   "complexity": null,
   "created_at": "2026-05-29T22:27:12Z",
   "created_by": "agent:qa-userflow",
@@ -10,10 +10,10 @@
   "done_at": null,
   "evidence_refs": [],
   "id": "task_01KSTXJG3MWS6CJWNQV1VVYF6Q",
-  "last_event_id": "ev_01KSTXJG3S862GSPAFASC0YVER",
+  "last_event_id": "ev_01KTF3X6R7BVB12PBQ1RPSV1VW",
   "last_status_changed_at": "2026-05-29T22:27:12Z",
   "linked_files": [],
-  "priority": "low",
+  "priority": "medium",
   "relationships_out": [],
   "reopened_count": 0,
   "schema_version": 1,
@@ -22,6 +22,6 @@
   "tags": null,
   "title": "Agent's own session_id is discarded; output session_id is etch's minted ULID only",
   "type": "bug",
-  "updated_at": "2026-05-29T22:27:12Z",
+  "updated_at": "2026-06-06T18:42:43Z",
   "urgency": null
 }
diff --git a/.lattice/tasks/task_01KSTXT5GR056EE9PK26744T01.json b/.lattice/tasks/task_01KSTXT5GR056EE9PK26744T01.json
index 626e40a..0fecc69 100644
--- a/.lattice/tasks/task_01KSTXT5GR056EE9PK26744T01.json
+++ b/.lattice/tasks/task_01KSTXT5GR056EE9PK26744T01.json
@@ -1,7 +1,7 @@
 {
   "assigned_to": null,
   "branch_links": [],
-  "comment_count": 0,
+  "comment_count": 1,
   "complexity": null,
   "created_at": "2026-05-29T22:31:23Z",
   "created_by": "agent:qa-adversarial",
@@ -10,7 +10,7 @@
   "done_at": null,
   "evidence_refs": [],
   "id": "task_01KSTXT5GR056EE9PK26744T01",
-  "last_event_id": "ev_01KSTXT5GSKCKYF608D441MCEX",
+  "last_event_id": "ev_01KTF3X6VBZZ3SYG5DE62WRDK4",
   "last_status_changed_at": "2026-05-29T22:31:23Z",
   "linked_files": [],
   "priority": "medium",
@@ -22,6 +22,6 @@
   "tags": null,
   "title": "README claims 'salted hash' for hostname but implementation is unsalted SHA-256",
   "type": "bug",
-  "updated_at": "2026-05-29T22:31:23Z",
+  "updated_at": "2026-06-06T18:42:43Z",
   "urgency": null
 }
diff --git a/.lattice/tasks/task_01KTAH0J77Q3Y0G6W517EPKMCB.json b/.lattice/tasks/task_01KTAH0J77Q3Y0G6W517EPKMCB.json
index adf945f..80824a3 100644
--- a/.lattice/tasks/task_01KTAH0J77Q3Y0G6W517EPKMCB.json
+++ b/.lattice/tasks/task_01KTAH0J77Q3Y0G6W517EPKMCB.json
@@ -1,7 +1,7 @@
 {
   "assigned_to": null,
   "branch_links": [],
-  "comment_count": 0,
+  "comment_count": 1,
   "complexity": "high",
   "created_at": "2026-06-04T23:55:32Z",
   "created_by": {
@@ -24,7 +24,7 @@
     }
   ],
   "id": "task_01KTAH0J77Q3Y0G6W517EPKMCB",
-  "last_event_id": "ev_01KTAH2BHD4R17SVS471WFB3CE",
+  "last_event_id": "ev_01KTF3X6YGQ92PVTX7TR4PNQW8",
   "last_status_changed_at": "2026-06-04T23:55:32Z",
   "linked_files": [],
   "priority": "critical",
@@ -145,6 +145,6 @@
   ],
   "title": "Deep code review remediation (2026-06-04): lifecycle, recovery, redaction, data-quality",
   "type": "bug",
-  "updated_at": "2026-06-04T23:56:31Z",
+  "updated_at": "2026-06-06T18:42:43Z",
   "urgency": null
 }
diff --git a/.lattice/tasks/task_01KTF3X71HAKCV8B0B5VEB08BR.json b/.lattice/tasks/task_01KTF3X71HAKCV8B0B5VEB08BR.json
new file mode 100644
index 0000000..a27d768
--- /dev/null
+++ b/.lattice/tasks/task_01KTF3X71HAKCV8B0B5VEB08BR.json
@@ -0,0 +1,69 @@
+{
+  "assigned_to": null,
+  "branch_links": [],
+  "comment_count": 0,
+  "complexity": "high",
+  "created_at": "2026-06-06T18:42:43Z",
+  "created_by": {
+    "agent_type": "review",
+    "base_name": "Etch-Reviewer",
+    "framework": "claude-code",
+    "model": "claude-opus-4-8",
+    "name": "Etch-Reviewer-1",
+    "serial": 1,
+    "session": "sess_01KTAH0J43J0KC6GCCMT3DVB8A"
+  },
+  "custom_fields": {},
+  "description": "DECISION MADE (operator, 2026-06-06): implement for real (not soften docs). Configured local_only_fields must actually stay off the wire.\n\nDesign space (delegator plans, operator-approved direction): projection layer at push time \u2014 e.g. a parallel public ref namespace (refs/etch/public/<ULID>) holding the stripped record, with setup-refspec pushing ONLY the public namespace; or a pre-push hook rewriting outgoing refs. The local ref keeps full fidelity. Immutability holds per-namespace.\n\nConstraints:\n- Coordinates tightly with the refspec/sync batch (ETCH-16/18/24/38) \u2014 same setup-refspec surface; land AFTER that batch.\n- Coordinates with ETCH-40 finding 5 (whole-record redaction pass) \u2014 the projection should reuse the same record-walking machinery.\n- README/settings docs updated to describe the real behavior; until this lands, README marks local_only_fields 'in development' (ETCH-40 finding 6 interim).\nOrigin: ETCH-40 finding 6 / superseded ETCH-31. Review file: reviews/2026-06-04-deep-code-review.md.",
+  "done_at": null,
+  "evidence_refs": [],
+  "id": "task_01KTF3X71HAKCV8B0B5VEB08BR",
+  "last_event_id": "ev_01KTF3XHAV2BHAYD5NGR77XR0X",
+  "last_status_changed_at": "2026-06-06T18:42:43Z",
+  "linked_files": [],
+  "priority": "high",
+  "relationships_out": [
+    {
+      "created_at": "2026-06-06T18:42:54Z",
+      "created_by": {
+        "agent_type": "review",
+        "base_name": "Etch-Reviewer",
+        "framework": "claude-code",
+        "model": "claude-opus-4-8",
+        "name": "Etch-Reviewer-1",
+        "serial": 1,
+        "session": "sess_01KTAH0J43J0KC6GCCMT3DVB8A"
+      },
+      "note": "Finding 6 decision: implement strip-before-push",
+      "target_task_id": "task_01KTAH0J77Q3Y0G6W517EPKMCB",
+      "type": "spawned_by"
+    },
+    {
+      "created_at": "2026-06-06T18:42:54Z",
+      "created_by": {
+        "agent_type": "review",
+        "base_name": "Etch-Reviewer",
+        "framework": "claude-code",
+        "model": "claude-opus-4-8",
+        "name": "Etch-Reviewer-1",
+        "serial": 1,
+        "session": "sess_01KTAH0J43J0KC6GCCMT3DVB8A"
+      },
+      "note": "Same setup-refspec surface; land after refspec/sync batch",
+      "target_task_id": "task_01KSTXH7RCZ6JYQ98W0PQSDC6D",
+      "type": "depends_on"
+    }
+  ],
+  "reopened_count": 0,
+  "schema_version": 1,
+  "short_id": "ETCH-41",
+  "status": "backlog",
+  "tags": [
+    "privacy",
+    "refspec"
+  ],
+  "title": "Implement local_only_fields: strip-before-push transport for session refs",
+  "type": "task",
+  "updated_at": "2026-06-06T18:42:54Z",
+  "urgency": null
+}
diff --git a/internal/archive/archive_test.go b/internal/archive/archive_test.go
index c9de8df..9743add 100644
--- a/internal/archive/archive_test.go
+++ b/internal/archive/archive_test.go
@@ -75,6 +75,14 @@ func daysAgo(n int) time.Time {
 	return fixedNow.AddDate(0, 0, -n)
 }
 
+// daysAgoReal computes relative to the real wall clock, for tests that invoke
+// the built binary (which uses time.Now(), not fixedNow). Mixing fixedNow into
+// those tests makes them time bombs: the gap between fixedNow and the real
+// clock grows daily until fabricated "recent" refs cross the binary's cutoff.
+func daysAgoReal(n int) time.Time {
+	return time.Now().UTC().AddDate(0, 0, -n)
+}
+
 func TestArchive_OldRefsArchived(t *testing.T) {
 	repo := testutil.NewTestRepo(t)
 
@@ -296,8 +304,9 @@ func TestArchive_ConfigThreshold(t *testing.T) {
 	writeConfig(t, repo, `{"archive_threshold_days":45}`)
 
 	// One ref at 50 days (older than 45 → archived), one at 40 days (kept).
-	writeSession(t, repo, makeULID(0), daysAgo(50))
-	writeSession(t, repo, makeULID(1), daysAgo(40))
+	// daysAgoReal: this test runs the built binary, which uses the real clock.
+	writeSession(t, repo, makeULID(0), daysAgoReal(50))
+	writeSession(t, repo, makeULID(1), daysAgoReal(40))
 
 	res := testutil.RunBinary(t, repo, []string{"archive"}, "")
 	if res.ExitCode != 0 {
@@ -316,7 +325,8 @@ func TestArchive_FlagOverridesConfig(t *testing.T) {
 	writeConfig(t, repo, `{"archive_threshold_days":45}`)
 
 	// Ref at 50 days: archived under config(45) but kept under flag(60).
-	writeSession(t, repo, makeULID(0), daysAgo(50))
+	// daysAgoReal: this test runs the built binary, which uses the real clock.
+	writeSession(t, repo, makeULID(0), daysAgoReal(50))
 
 	res := testutil.RunBinary(t, repo, []string{"archive", "--threshold-days", "60"}, "")
 	if res.ExitCode != 0 {
diff --git a/internal/capture/buffer.go b/internal/capture/buffer.go
index b90fe3d..fd8672e 100644
--- a/internal/capture/buffer.go
+++ b/internal/capture/buffer.go
@@ -131,7 +131,9 @@ func ReadEvents(repoRoot, sessionID string) ([]HookEvent, error) {
 }
 
 // Finalize reads the .wip file, aggregates events into a Session, and writes session.json.
-func Finalize(repoRoot, sessionID string) (*Session, error) {
+// State (wip, session.json) lives under repoRoot; git diffs run in workDir — the
+// session's own checkout, which differs from repoRoot for linked worktrees.
+func Finalize(repoRoot, workDir, sessionID string) (*Session, error) {
 	events, err := ReadEvents(repoRoot, sessionID)
 	if err != nil {
 		return nil, err
@@ -233,7 +235,7 @@ func Finalize(repoRoot, sessionID string) (*Session, error) {
 
 	// files_touched: defer accurate actions to git diff at session end
 	if session.GitStart != nil && session.GitEnd != nil && session.GitStart.HeadSHA != "" {
-		files, err := gitDiffFiles(repoRoot, session.GitStart.HeadSHA)
+		files, err := gitDiffFiles(workDir, session.GitStart.HeadSHA)
 		if err == nil {
 			session.FilesTouched = files
 		}
diff --git a/internal/capture/capture_test.go b/internal/capture/capture_test.go
index 108a742..a5084a5 100644
--- a/internal/capture/capture_test.go
+++ b/internal/capture/capture_test.go
@@ -157,7 +157,7 @@ func TestFinalize(t *testing.T) {
 		ExitReason: "normal",
 	})
 
-	session, err := Finalize(dir, sessionID)
+	session, err := Finalize(dir, dir, sessionID)
 	if err != nil {
 		t.Fatal(err)
 	}
@@ -237,7 +237,7 @@ func TestFinalizeEmpty(t *testing.T) {
 	dir := t.TempDir()
 	EnsureDirs(dir)
 
-	_, err := Finalize(dir, "01EMPTY")
+	_, err := Finalize(dir, dir, "01EMPTY")
 	if err == nil {
 		t.Error("expected error for empty/missing wip file")
 	}
diff --git a/internal/capture/repocontext.go b/internal/capture/repocontext.go
new file mode 100644
index 0000000..6971b5a
--- /dev/null
+++ b/internal/capture/repocontext.go
@@ -0,0 +1,58 @@
+package capture
+
+import (
+	"fmt"
+	"os/exec"
+	"path/filepath"
+	"strings"
+)
+
+// RepoContext anchors hook state and git operations for one hook invocation.
+//
+// StateRoot is the main repo root — the parent of `git rev-parse --git-common-dir` —
+// shared by all linked worktrees. All .etch state (wip buffers, ULID mappings,
+// settings.json), the recovery scan, and ref writes anchor here so hooks firing from
+// any CWD inside the repo (root, subdir, linked worktree) resolve the same state.
+//
+// WorkDir is the toplevel of the invoking checkout (`git rev-parse --show-toplevel`),
+// linked-worktree aware. Git state capture and diffs anchor here so a worktree session
+// records its own branch/SHA and diffs its own checkout.
+//
+// Known limitation: inside a submodule, --git-common-dir points into the superproject's
+// .git/modules/<name>, so StateRoot would not be a checkout root there. Submodule
+// sessions are out of scope.
+type RepoContext struct {
+	StateRoot string
+	WorkDir   string
+}
+
+// ResolveRepoContext resolves both roots from dir (the hook process CWD).
+// Returns an error when dir is not inside a usable git repository or git cannot run;
+// the error wraps git's own stderr so a missing git binary is distinguishable from a
+// non-repo directory.
+func ResolveRepoContext(dir string) (*RepoContext, error) {
+	cmd := exec.Command("git", "rev-parse", "--show-toplevel", "--git-common-dir")
+	cmd.Dir = dir
+	out, err := cmd.Output()
+	if err != nil {
+		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
+			return nil, fmt.Errorf("git rev-parse: %s", strings.TrimSpace(string(ee.Stderr)))
+		}
+		return nil, fmt.Errorf("running git rev-parse: %w", err)
+	}
+
+	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
+	if len(lines) != 2 || strings.TrimSpace(lines[0]) == "" || strings.TrimSpace(lines[1]) == "" {
+		// Bare repos and other degenerate states yield no toplevel — not usable.
+		return nil, fmt.Errorf("git rev-parse returned no usable worktree (bare repo?): %q", strings.TrimSpace(string(out)))
+	}
+
+	workDir := strings.TrimSpace(lines[0])
+	// Git emits --git-common-dir relative to the CWD (e.g. ".git" at the toplevel).
+	commonDir := resolvePath(dir, strings.TrimSpace(lines[1]))
+
+	return &RepoContext{
+		StateRoot: filepath.Dir(commonDir),
+		WorkDir:   workDir,
+	}, nil
+}
diff --git a/internal/capture/repocontext_test.go b/internal/capture/repocontext_test.go
new file mode 100644
index 0000000..b3e4f23
--- /dev/null
+++ b/internal/capture/repocontext_test.go
@@ -0,0 +1,106 @@
+package capture
+
+import (
+	"os"
+	"path/filepath"
+	"testing"
+)
+
+// realPath resolves symlinks so paths from git (physical) compare equal to
+// t.TempDir() paths (logical, e.g. /var -> /private/var on macOS).
+func realPath(t *testing.T, p string) string {
+	t.Helper()
+	r, err := filepath.EvalSymlinks(p)
+	if err != nil {
+		t.Fatalf("EvalSymlinks(%s): %v", p, err)
+	}
+	return r
+}
+
+func TestResolveRepoContextAtRoot(t *testing.T) {
+	dir := newTestGitRepo(t)
+
+	rc, err := ResolveRepoContext(dir)
+	if err != nil {
+		t.Fatal(err)
+	}
+	want := realPath(t, dir)
+	if realPath(t, rc.StateRoot) != want {
+		t.Errorf("StateRoot: got %s, want %s", rc.StateRoot, want)
+	}
+	if realPath(t, rc.WorkDir) != want {
+		t.Errorf("WorkDir: got %s, want %s", rc.WorkDir, want)
+	}
+}
+
+func TestResolveRepoContextFromSubdir(t *testing.T) {
+	dir := newTestGitRepo(t)
+	sub := filepath.Join(dir, "src", "deep", "nested")
+	if err := os.MkdirAll(sub, 0o755); err != nil {
+		t.Fatal(err)
+	}
+
+	rc, err := ResolveRepoContext(sub)
+	if err != nil {
+		t.Fatal(err)
+	}
+	want := realPath(t, dir)
+	if realPath(t, rc.StateRoot) != want {
+		t.Errorf("StateRoot from subdir: got %s, want %s", rc.StateRoot, want)
+	}
+	if realPath(t, rc.WorkDir) != want {
+		t.Errorf("WorkDir from subdir: got %s, want %s", rc.WorkDir, want)
+	}
+}
+
+func TestResolveRepoContextLinkedWorktree(t *testing.T) {
+	dir := newTestGitRepo(t)
+	wt := filepath.Join(t.TempDir(), "wt")
+	gitCmd(t, dir, "worktree", "add", wt, "-b", "feature")
+
+	wantState := realPath(t, dir)
+	wantWork := realPath(t, wt)
+
+	// From the worktree toplevel
+	rc, err := ResolveRepoContext(wt)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if realPath(t, rc.StateRoot) != wantState {
+		t.Errorf("StateRoot from worktree: got %s, want main root %s", rc.StateRoot, wantState)
+	}
+	if realPath(t, rc.WorkDir) != wantWork {
+		t.Errorf("WorkDir from worktree: got %s, want %s", rc.WorkDir, wantWork)
+	}
+
+	// From a subdir inside the worktree
+	sub := filepath.Join(wt, "pkg")
+	os.MkdirAll(sub, 0o755)
+	rc, err = ResolveRepoContext(sub)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if realPath(t, rc.StateRoot) != wantState {
+		t.Errorf("StateRoot from worktree subdir: got %s, want main root %s", rc.StateRoot, wantState)
+	}
+	if realPath(t, rc.WorkDir) != wantWork {
+		t.Errorf("WorkDir from worktree subdir: got %s, want %s", rc.WorkDir, wantWork)
+	}
+}
+
+func TestResolveRepoContextNonGit(t *testing.T) {
+	dir := t.TempDir()
+	_, err := ResolveRepoContext(dir)
+	if err == nil {
+		t.Fatal("expected error for non-git directory")
+	}
+}
+
+func TestResolveRepoContextBareRepo(t *testing.T) {
+	dir := t.TempDir()
+	gitCmd(t, dir, "init", "--bare")
+	_, err := ResolveRepoContext(dir)
+	if err == nil {
+		t.Fatal("expected error for bare repo (no usable worktree)")
+	}
+}
diff --git a/internal/hooks/commit.go b/internal/hooks/commit.go
index d17de59..7c1829a 100644
--- a/internal/hooks/commit.go
+++ b/internal/hooks/commit.go
@@ -20,9 +20,10 @@ import (
 func commitSession(repoRoot string, session *capture.Session, entireSessionID string) error {
 	settings, _ := config.Load(repoRoot)
 
-	if session.Prompt != nil {
-		session.Prompt.Text = redact.Redact(session.Prompt.Text, settings)
-	}
+	// One redaction pass over every string-bearing field of the finalized
+	// record — prompt text, file paths, tool names, orchestration extras —
+	// not just Prompt.Text (ETCH-40 finding 5).
+	redact.DeepRedact(session, settings)
 
 	sessionJSON, err := json.MarshalIndent(session, "", "  ")
 	if err != nil {
@@ -101,9 +102,9 @@ type etchRefWriter struct{}
 func (w *etchRefWriter) WriteSessionRef(repoDir string, session *schema.Session) error {
 	settings, _ := config.Load(repoDir)
 
-	if session.Prompt != nil {
-		session.Prompt.Text = redact.Redact(session.Prompt.Text, settings)
-	}
+	// Same full-record redaction pass as commitSession — the crash-recovery
+	// path must not commit less-redacted records than the normal path.
+	redact.DeepRedact(session, settings)
 
 	sessionJSON, err := json.MarshalIndent(session, "", "  ")
 	if err != nil {
diff --git a/internal/hooks/common.go b/internal/hooks/common.go
index 92a34b8..bd021fa 100644
--- a/internal/hooks/common.go
+++ b/internal/hooks/common.go
@@ -5,6 +5,8 @@ import (
 	"fmt"
 	"io"
 	"os"
+
+	"forgejo.stage11.ai/s11/etch/internal/capture"
 )
 
 // StdinEvent is the JSON structure Entire sends on stdin for hook invocations.
@@ -35,8 +37,28 @@ func printOK() {
 	fmt.Println(`{"ok":true}`)
 }
 
-// findRepoRoot returns the git repo root for the current directory.
-func findRepoRoot() string {
-	dir, _ := os.Getwd()
-	return dir
+// printNotOK emits a non-ok result on stdout so Entire never sees success for an
+// invocation that dropped data.
+func printNotOK(msg string) {
+	out, _ := json.Marshal(map[string]any{"ok": false, "error": msg})
+	fmt.Println(string(out))
+}
+
+// resolveContext resolves the repo context for the hook process CWD. Every hook calls
+// this FIRST, before any filesystem write. On failure (non-git directory, unusable
+// repo, git missing) it prints a clear warning to stderr and a non-ok result to stdout,
+// then returns an error — capture is disabled, nothing is created, nothing is orphaned.
+func resolveContext() (*capture.RepoContext, error) {
+	cwd, err := os.Getwd()
+	if err != nil {
+		return nil, fmt.Errorf("getting cwd: %w", err)
+	}
+	rc, err := capture.ResolveRepoContext(cwd)
+	if err != nil {
+		msg := fmt.Sprintf("could not resolve a git repository (cwd=%s): %v", cwd, err)
+		fmt.Fprintf(os.Stderr, "etch: %s; session capture disabled, no record will be written\n", msg)
+		printNotOK(msg)
+		return nil, fmt.Errorf("%s", msg)
+	}
+	return rc, nil
 }
diff --git a/internal/hooks/e2e_test.go b/internal/hooks/e2e_test.go
index fa04d55..e2b8993 100644
--- a/internal/hooks/e2e_test.go
+++ b/internal/hooks/e2e_test.go
@@ -33,8 +33,8 @@ func TestE2EFullLifecycle(t *testing.T) {
 	wipBase := filepath.Base(wipFiles[0])
 	sessionULID := strings.TrimSuffix(wipBase, ".wip.jsonl")
 
-	// 2. user_prompt_submit (include a secret to verify redaction)
-	promptInput := `{"session_id":"` + entireSessionID + `","user_prompt":"fix the bug, key is sk-ant-abc123456789012345678901234567890"}`
+	// 2. user_prompt_submit (include a real-shape secret to verify redaction)
+	promptInput := `{"session_id":"` + entireSessionID + `","user_prompt":"fix the bug, key is sk-ant-api03-AbCd1234efGh5678IjKl9012MnOp"}`
 	r = testutil.RunBinary(t, dir, []string{"user_prompt_submit"}, promptInput)
 	assertOK(t, r, "user_prompt_submit")
 
@@ -98,7 +98,7 @@ func TestE2EFullLifecycle(t *testing.T) {
 		t.Fatal("expected non-nil prompt")
 	}
 	promptText, _ := prompt["text"].(string)
-	if strings.Contains(promptText, "sk-ant-abc123456789") {
+	if strings.Contains(promptText, "sk-ant-api03-AbCd1234") {
 		t.Error("secret was NOT redacted from prompt text")
 	}
 	if !strings.Contains(promptText, "[REDACTED:") {
@@ -131,6 +131,97 @@ func TestE2EFullLifecycle(t *testing.T) {
 	}
 }
 
+// ETCH-40 finding 5: secrets that reach the record via tool names and file
+// paths (not just prompt text) must be redacted in the COMMITTED blobs —
+// both session.json and agent-trace.json.
+func TestE2ECommitBoundaryRedaction(t *testing.T) {
+	dir := testutil.NewTestRepo(t)
+	commitInitial(t, dir)
+
+	const secretKey = "sk-proj-AbCdEf123456_789-abcdefGHIJKL"
+	const secretJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature_part"
+
+	entireSessionID := "e2e-redact-001"
+	startInput := `{"session_id":"` + entireSessionID + `","raw_data":{"model":"claude-opus-4-7"}}`
+	r := testutil.RunBinary(t, dir, []string{"session_start"}, startInput)
+	assertOK(t, r, "session_start")
+
+	wipFiles := findWipFiles(t, dir)
+	sessionULID := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")
+
+	// Tool name carrying a JWT → lands in tool_use.by_tool as a map KEY.
+	toolInput := `{"session_id":"` + entireSessionID + `","tool_name":"Bash ` + secretJWT + `","tool_use_id":"tu-1","tool_input":{"file_path":"/tmp/x"}}`
+	r = testutil.RunBinary(t, dir, []string{"pre_tool_use"}, toolInput)
+	assertOK(t, r, "pre_tool_use")
+	r = testutil.RunBinary(t, dir, []string{"post_tool_use"}, toolInput)
+	assertOK(t, r, "post_tool_use")
+
+	// A committed file whose NAME contains a secret → lands in files_touched.
+	if err := os.WriteFile(filepath.Join(dir, secretKey+".txt"), []byte("x"), 0o644); err != nil {
+		t.Fatal(err)
+	}
+	run(t, dir, "git", "add", "-A")
+	run(t, dir, "git", "commit", "-m", "add file")
+
+	endInput := `{"session_id":"` + entireSessionID + `"}`
+	r = testutil.RunBinary(t, dir, []string{"session_end"}, endInput)
+	assertOK(t, r, "session_end")
+
+	// Inspect the actual committed blobs, not in-memory structs.
+	refName := "refs/etch/sessions/" + sessionULID
+	for _, blob := range []string{"session.json", "agent-trace.json"} {
+		data := gitShow(t, dir, refName+":"+blob)
+		if strings.Contains(data, secretKey) {
+			t.Errorf("%s: file-path secret leaked into committed blob", blob)
+		}
+		if strings.Contains(data, "eyJhbGciOi") {
+			t.Errorf("%s: JWT leaked into committed blob", blob)
+		}
+	}
+	sessionData := gitShow(t, dir, refName+":session.json")
+	if !strings.Contains(sessionData, "[REDACTED:openai-api-key]") {
+		t.Errorf("expected openai marker in committed session.json:\n%s", sessionData)
+	}
+	if !strings.Contains(sessionData, "[REDACTED:jwt]") {
+		t.Errorf("expected jwt marker in committed session.json:\n%s", sessionData)
+	}
+}
+
+// The crash-recovery commit boundary must redact exactly like the normal
+// path: a recovered wip with secrets in prompt text ends up clean in the ref.
+func TestE2ECrashRecoveryRedaction(t *testing.T) {
+	dir := testutil.NewTestRepo(t)
+	commitInitial(t, dir)
+
+	sessionsDir := filepath.Join(dir, ".etch", "sessions")
+	os.MkdirAll(filepath.Join(sessionsDir, ".map"), 0o755)
+
+	orphanedID := "01TESTORPHANREDACT00000000"
+	wipPath := filepath.Join(sessionsDir, orphanedID+".wip.jsonl")
+
+	ts := time.Now().Add(-5 * time.Hour).UTC().Format(time.RFC3339Nano)
+	wipContent := `{"ts":"` + ts + `","hook":"session_start","data":{"session_id":"` + orphanedID + `","agent":{"runtime":"claude-code","model":"claude-opus-4-7"},"orchestration":{"type":"manual","extra":{}},"machine":{"hostname_hash":"sha256:test","os":"darwin","os_version":"Darwin 25.5.0","arch":"arm64"},"operator":{"git_user":"Test <test@test.local>","os_user":"test"},"git_state":{"branch":"main","head_sha":"abc123"}}}` + "\n"
+	wipContent += `{"ts":"` + ts + `","hook":"user_prompt_submit","data":{"prompt":"key is sk-ant-api03-AbCd1234efGh5678IjKl9012MnOp","source":"interactive","truncated":false}}` + "\n"
+
+	if err := os.WriteFile(wipPath, []byte(wipContent), 0o644); err != nil {
+		t.Fatal(err)
+	}
+
+	// Trigger recovery via a fresh session_start
+	startInput := `{"session_id":"e2e-recovery-redact-001","raw_data":{"model":"claude-opus-4-7"}}`
+	r := testutil.RunBinary(t, dir, []string{"session_start"}, startInput)
+	assertOK(t, r, "session_start (recovery trigger)")
+
+	refName := "refs/etch/sessions/" + orphanedID
+	sessionData := gitShow(t, dir, refName+":session.json")
+	if strings.Contains(sessionData, "sk-ant-api03-AbCd1234") {
+		t.Error("secret leaked through the crash-recovery commit path")
+	}
+	if !strings.Contains(sessionData, "[REDACTED:anthropic-api-key]") {
+		t.Errorf("expected anthropic marker in recovered session.json:\n%s", sessionData)
+	}
+}
+
 func TestE2EStopHookWritesRef(t *testing.T) {
 	dir := testutil.NewTestRepo(t)
 	commitInitial(t, dir)
diff --git a/internal/hooks/reporoot_test.go b/internal/hooks/reporoot_test.go
new file mode 100644
index 0000000..25c18c7
--- /dev/null
+++ b/internal/hooks/reporoot_test.go
@@ -0,0 +1,342 @@
+package hooks_test
+
+// Adversarial tests for the repo-root batch (ETCH-34, ETCH-35, ETCH-40 finding 2):
+// hooks must anchor .etch state at the main repo root regardless of the CWD they fire
+// from (subdir, linked worktree), and non-git / commit-failure paths must be visible —
+// never {"ok":true} while dropping data.
+
+import (
+	"encoding/json"
+	"os"
+	"os/exec"
+	"path/filepath"
+	"strings"
+	"testing"
+	"time"
+
+	"forgejo.stage11.ai/s11/etch/internal/capture"
+	"forgejo.stage11.ai/s11/etch/internal/testutil"
+)
+
+// realPath resolves symlinks so git-reported (physical) paths compare equal to
+// t.TempDir() (logical) paths on macOS.
+func realPath(t *testing.T, p string) string {
+	t.Helper()
+	r, err := filepath.EvalSymlinks(p)
+	if err != nil {
+		t.Fatalf("EvalSymlinks(%s): %v", p, err)
+	}
+	return r
+}
+
+func assertNoEtch(t *testing.T, dir string) {
+	t.Helper()
+	if _, err := os.Stat(filepath.Join(dir, ".etch")); !os.IsNotExist(err) {
+		t.Errorf("unexpected .etch dir in %s", dir)
+	}
+}
+
+// ETCH-34 gate: session_start at the repo root + remaining hooks from nested subdirs
+// land in ONE record under one .etch at the root.
+func TestHooksFromSubdirsProduceOneRecord(t *testing.T) {
+	dir := testutil.NewTestRepo(t)
+	commitInitial(t, dir)
+
+	nested := filepath.Join(dir, "src", "deep", "nested")
+	if err := os.MkdirAll(nested, 0o755); err != nil {
+		t.Fatal(err)
+	}
+
+	sid := "subdir-batch-001"
+
+	r := testutil.RunBinary(t, dir, []string{"session_start"}, `{"session_id":"`+sid+`","raw_data":{"model":"claude-opus-4-8"}}`)
+	assertOK(t, r, "session_start at root")
+
+	wipFiles := findWipFiles(t, dir)
+	if len(wipFiles) != 1 {
+		t.Fatalf("expected 1 wip file at root, got %d", len(wipFiles))
+	}
+	ulid := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")
+
+	r = testutil.RunBinary(t, filepath.Join(dir, "src"), []string{"user_prompt_submit"}, `{"session_id":"`+sid+`","user_prompt":"refactor the parser"}`)
+	assertOK(t, r, "user_prompt_submit from src/")
+
+	r = testutil.RunBinary(t, filepath.Join(dir, "src", "deep"), []string{"pre_tool_use"}, `{"session_id":"`+sid+`","tool_name":"Edit","tool_use_id":"tu-1","tool_input":{"file_path":"/tmp/x.go"}}`)
+	assertOK(t, r, "pre_tool_use from src/deep/")
+
+	r = testutil.RunBinary(t, nested, []string{"session_end"}, `{"session_id":"`+sid+`"}`)
+	assertOK(t, r, "session_end from src/deep/nested/")
+
+	// No scattered .etch dirs
+	assertNoEtch(t, filepath.Join(dir, "src"))
+	assertNoEtch(t, filepath.Join(dir, "src", "deep"))
+	assertNoEtch(t, nested)
+
+	// One coherent committed record at the root
+	data := readRefBlob(t, dir, "refs/etch/sessions/"+ulid+":session.json")
+	var session capture.Session
+	if err := json.Unmarshal(data, &session); err != nil {
+		t.Fatalf("invalid session JSON: %v", err)
+	}
+	if session.Prompt == nil || session.Prompt.Text != "refactor the parser" {
+		t.Errorf("prompt lost across CWDs: %+v", session.Prompt)
+	}
+	if session.ToolUse.ByTool["Edit"] != 1 {
+		t.Errorf("tool use lost across CWDs: %+v", session.ToolUse.ByTool)
+	}
+	if session.Status != "complete" {
+		t.Errorf("status: got %s, want complete", session.Status)
+	}
+
+	// Buffer state fully cleaned up at the root
+	if len(findWipFiles(t, dir)) != 0 {
+		t.Error("wip file should be cleaned up after session_end")
+	}
+}
+
+// ETCH-34: .etch/settings.json at the repo root applies to sessions whose hooks fire
+// from a subdir (custom redaction must not silently weaken).
+func TestRootSettingsApplyFromSubdir(t *testing.T) {
+	dir := testutil.NewTestRepo(t)
+	commitInitial(t, dir)
+
+	if err := os.MkdirAll(filepath.Join(dir, ".etch"), 0o755); err != nil {
+		t.Fatal(err)
+	}
+	settings := `{"redaction_patterns":["SECRETTOKEN[0-9]+"]}`
+	if err := os.WriteFile(filepath.Join(dir, ".etch", "settings.json"), []byte(settings), 0o644); err != nil {
+		t.Fatal(err)
+	}
+
+	sub := filepath.Join(dir, "src")
+	os.MkdirAll(sub, 0o755)
+
+	sid := "subdir-settings-001"
+	r := testutil.RunBinary(t, sub, []string{"session_start"}, `{"session_id":"`+sid+`","raw_data":{}}`)
+	assertOK(t, r, "session_start from subdir")
+
+	wipFiles := findWipFiles(t, dir)
+	if len(wipFiles) != 1 {
+		t.Fatalf("expected wip at root for subdir session_start, got %d", len(wipFiles))
+	}
+	ulid := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")
+
+	r = testutil.RunBinary(t, sub, []string{"user_prompt_submit"}, `{"session_id":"`+sid+`","user_prompt":"deploy with SECRETTOKEN12345 now"}`)
+	assertOK(t, r, "user_prompt_submit from subdir")
+
+	r = testutil.RunBinary(t, sub, []string{"session_end"}, `{"session_id":"`+sid+`"}`)
+	assertOK(t, r, "session_end from subdir")
+
+	data := readRefBlob(t, dir, "refs/etch/sessions/"+ulid+":session.json")
+	var session capture.Session
+	json.Unmarshal(data, &session)
+	if session.Prompt == nil {
+		t.Fatal("expected non-nil prompt")
+	}
+	if strings.Contains(session.Prompt.Text, "SECRETTOKEN12345") {
+		t.Error("root settings.json redaction_patterns ignored for subdir session")
+	}
+	if !strings.Contains(session.Prompt.Text, "[REDACTED") {
+		t.Errorf("expected redaction marker, got %q", session.Prompt.Text)
+	}
+}
+
+// ETCH-34 / ETCH-40 f.2 gate: hooks fired from a linked worktree anchor state at the
+// MAIN repo root, while git capture reflects the worktree's own checkout.
+func TestHooksFromLinkedWorktree(t *testing.T) {
+	dir := testutil.NewTestRepo(t)
+	commitInitial(t, dir)
+
+	wt := filepath.Join(t.TempDir(), "wt")
+	run(t, dir, "git", "worktree", "add", wt, "-b", "feature")
+
+	sid := "worktree-batch-001"
+	r := testutil.RunBinary(t, wt, []string{"session_start"}, `{"session_id":"`+sid+`","raw_data":{"model":"claude-opus-4-8"}}`)
+	assertOK(t, r, "session_start in worktree")
+
+	// State must land at the main root, never inside the worktree
+	assertNoEtch(t, wt)
+	wipFiles := findWipFiles(t, dir)
+	if len(wipFiles) != 1 {
+		t.Fatalf("expected 1 wip at MAIN root for worktree session, got %d", len(wipFiles))
+	}
+	ulid := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")
+
+	r = testutil.RunBinary(t, wt, []string{"user_prompt_submit"}, `{"session_id":"`+sid+`","user_prompt":"work in the worktree"}`)
+	assertOK(t, r, "user_prompt_submit in worktree")
+
+	// Produce a commit in the worktree so the diff has content
+	os.WriteFile(filepath.Join(wt, "feature.go"), []byte("package feature"), 0o644)
+	run(t, wt, "git", "add", "feature.go")
+	run(t, wt, "git", "commit", "-m", "worktree commit")
+
+	// End the session from a SUBDIR of the worktree
+	sub := filepath.Join(wt, "pkg")
+	os.MkdirAll(sub, 0o755)
+	r = testutil.RunBinary(t, sub, []string{"session_end"}, `{"session_id":"`+sid+`"}`)
+	assertOK(t, r, "session_end from worktree subdir")
+
+	assertNoEtch(t, wt)
+	assertNoEtch(t, sub)
+
+	data := readRefBlob(t, dir, "refs/etch/sessions/"+ulid+":session.json")
+	var session capture.Session
+	if err := json.Unmarshal(data, &session); err != nil {
+		t.Fatalf("invalid session JSON: %v", err)
+	}
+
+	// Git capture must reflect the worktree's own checkout
+	if session.GitStart == nil {
+		t.Fatal("git_start should not be nil")
+	}
+	if !session.GitStart.IsWorktree {
+		t.Error("git_start.is_worktree: got false, want true")
+	}
+	if realPath(t, session.GitStart.WorktreePath) != realPath(t, wt) {
+		t.Errorf("git_start.worktree_path: got %s, want %s", session.GitStart.WorktreePath, wt)
+	}
+	if session.GitStart.Branch != "feature" {
+		t.Errorf("git_start.branch: got %s, want feature", session.GitStart.Branch)
+	}
+	if session.GitEnd == nil || len(session.GitEnd.CommitsProduced) != 1 {
+		t.Errorf("git_end.commits_produced: got %+v, want exactly 1", session.GitEnd)
+	}
+
+	// Diff ran against the worktree's checkout, not the main one
+	foundFeature := false
+	for _, f := range session.FilesTouched {
+		if f.Path == "feature.go" {
+			foundFeature = true
+			if f.Action != "added" {
+				t.Errorf("feature.go action: got %s, want added", f.Action)
+			}
+		}
+	}
+	if !foundFeature {
+		t.Errorf("files_touched missing worktree change feature.go: %+v", session.FilesTouched)
+	}
+
+	if session.Prompt == nil || session.Prompt.Text != "work in the worktree" {
+		t.Errorf("prompt lost for worktree session: %+v", session.Prompt)
+	}
+}
+
+// ETCH-34: an orphan that crashed under one checkout is recovered by a session_start
+// fired from a different checkout (the linked worktree) — the sweep anchors at the
+// shared state root.
+func TestOrphanRecoveredFromWorktreeSessionStart(t *testing.T) {
+	dir := testutil.NewTestRepo(t)
+	commitInitial(t, dir)
+
+	wt := filepath.Join(t.TempDir(), "wt")
+	run(t, dir, "git", "worktree", "add", wt, "-b", "recovery-feature")
+
+	// Plant an orphaned wip at the main root (>4h idle = past the default timeout)
+	sessionsDir := filepath.Join(dir, ".etch", "sessions")
+	os.MkdirAll(filepath.Join(sessionsDir, ".map"), 0o755)
+	orphanID := "01REPOROOTORPHAN0000000000"
+	ts := time.Now().Add(-5 * time.Hour).UTC().Format(time.RFC3339Nano)
+	wip := `{"ts":"` + ts + `","hook":"session_start","data":{"session_id":"` + orphanID + `","agent":{"runtime":"claude-code","model":"claude-opus-4-8"},"orchestration":{"type":"manual","extra":{}},"git_state":{"branch":"main","head_sha":"abc123"}}}` + "\n"
+	if err := os.WriteFile(filepath.Join(sessionsDir, orphanID+".wip.jsonl"), []byte(wip), 0o644); err != nil {
+		t.Fatal(err)
+	}
+
+	// Trigger recovery from INSIDE the worktree
+	r := testutil.RunBinary(t, wt, []string{"session_start"}, `{"session_id":"recovery-trigger-wt","raw_data":{}}`)
+	assertOK(t, r, "session_start in worktree (recovery trigger)")
+
+	refCheck := exec.Command("git", "show-ref", "--verify", "refs/etch/sessions/"+orphanID)
+	refCheck.Dir = dir
+	if err := refCheck.Run(); err != nil {
+		t.Fatalf("orphan was not recovered by worktree session_start: %v", err)
+	}
+	if _, err := os.Stat(filepath.Join(sessionsDir, orphanID+".wip.jsonl")); !os.IsNotExist(err) {
+		t.Error("orphan wip should be cleaned up after recovery")
+	}
+}
+
+// ETCH-35 gate: every hook in a non-git directory fails visibly — non-zero exit,
+// stderr explanation, no {"ok":true}, and no .etch pollution.
+func TestNonGitDirAllHooksFailVisible(t *testing.T) {
+	dir := t.TempDir()
+
+	hooks := []string{"session_start", "user_prompt_submit", "pre_tool_use", "post_tool_use", "session_end", "stop"}
+	for _, hook := range hooks {
+		input := `{"session_id":"nogit-001","user_prompt":"hi","tool_name":"Read"}`
+		r := testutil.RunBinary(t, dir, []string{hook}, input)
+
+		if r.ExitCode == 0 {
+			t.Errorf("%s: expected non-zero exit in non-git dir, got 0", hook)
+		}
+		if !strings.Contains(r.Stderr, "could not resolve a git repository") {
+			t.Errorf("%s: stderr should explain the failure, got: %s", hook, r.Stderr)
+		}
+		if strings.Contains(r.Stdout, `"ok":true`) {
+			t.Errorf("%s: must never print ok:true in a non-git dir, got: %s", hook, r.Stdout)
+		}
+		if !strings.Contains(r.Stdout, `"ok":false`) {
+			t.Errorf("%s: expected machine-readable ok:false on stdout, got: %s", hook, r.Stdout)
+		}
+	}
+
+	assertNoEtch(t, dir)
+}
+
+// ETCH-35: a commit failure at session_end is visible (non-ok, non-zero) and retains
+// the wip + mapping for later recovery.
+func TestCommitFailureVisibleAndRecoverable(t *testing.T) {
+	dir := testutil.NewTestRepo(t)
+	commitInitial(t, dir)
+
+	sid := "commit-fail-001"
+	r := testutil.RunBinary(t, dir, []string{"session_start"}, `{"session_id":"`+sid+`","raw_data":{}}`)
+	assertOK(t, r, "session_start")
+
+	wipFiles := findWipFiles(t, dir)
+	if len(wipFiles) != 1 {
+		t.Fatalf("expected 1 wip, got %d", len(wipFiles))
+	}
+	ulid := strings.TrimSuffix(filepath.Base(wipFiles[0]), ".wip.jsonl")
+
+	// Deterministic ref-write sabotage: a ref nested UNDER the session's ref name
+	// makes update-ref fail with a directory/file conflict.
+	run(t, dir, "git", "update-ref", "refs/etch/sessions/"+ulid+"/block", "HEAD")
+
+	r = testutil.RunBinary(t, dir, []string{"session_end"}, `{"session_id":"`+sid+`"}`)
+	if r.ExitCode == 0 {
+		t.Error("session_end with failing commit should exit non-zero")
+	}
+	if strings.Contains(r.Stdout, `"ok":true`) {
+		t.Errorf("session_end must not print ok:true on commit failure, got: %s", r.Stdout)
+	}
+	if !strings.Contains(r.Stdout, `"ok":false`) {
+		t.Errorf("expected ok:false on stdout, got: %s", r.Stdout)
+	}
+
+	// wip + mapping retained so recovery can retry later
+	if len(findWipFiles(t, dir)) != 1 {
+		t.Error("wip must be retained after commit failure (recovery needs it)")
+	}
+	mapDir := filepath.Join(dir, ".etch", "sessions", ".map")
+	entries, _ := os.ReadDir(mapDir)
+	if len(entries) != 1 {
+		t.Errorf("mapping must be retained after commit failure, got %d entries", len(entries))
+	}
+}
+
+// Regression guard for the REFUTED "exit_reason clobber" finding: a stop arriving
+// after session_end already finalized must still exit 0 with {"ok":true}.
+func TestStopAfterSessionEndStaysOK(t *testing.T) {
+	dir := testutil.NewTestRepo(t)
+	commitInitial(t, dir)
+
+	sid := "stop-after-end-001"
+	r := testutil.RunBinary(t, dir, []string{"session_start"}, `{"session_id":"`+sid+`","raw_data":{}}`)
+	assertOK(t, r, "session_start")
+
+	r = testutil.RunBinary(t, dir, []string{"session_end"}, `{"session_id":"`+sid+`"}`)
+	assertOK(t, r, "session_end")
+
+	r = testutil.RunBinary(t, dir, []string{"stop"}, `{"session_id":"`+sid+`"}`)
+	assertOK(t, r, "stop after session_end (mapping-miss path is by design)")
+}
diff --git a/internal/hooks/session_end.go b/internal/hooks/session_end.go
index eaf519f..3a6a98c 100644
--- a/internal/hooks/session_end.go
+++ b/internal/hooks/session_end.go
@@ -2,6 +2,7 @@ package hooks
 
 import (
 	"encoding/json"
+	"fmt"
 	"log"
 
 	"forgejo.stage11.ai/s11/etch/internal/capture"
@@ -21,16 +22,23 @@ func runEnd(hookName, defaultExitReason string) error {
 		return err
 	}
 
-	repoRoot := findRepoRoot()
-	sessionID := capture.LookupMapping(repoRoot, ev.SessionID)
+	rc, err := resolveContext()
+	if err != nil {
+		return err
+	}
+
+	sessionID := capture.LookupMapping(rc.StateRoot, ev.SessionID)
 	if sessionID == "" {
+		// By design: a stop arriving after session_end already finalized takes this
+		// path. Log so a genuinely dropped session is visible on stderr.
+		log.Printf("etch: %s: no session mapping for %q under %s (already finalized, or session_start never ran)", hookName, ev.SessionID, rc.StateRoot)
 		printOK()
 		return nil
 	}
 
 	// Read start SHA from the existing wip to compute commits_produced
 	var startSHA string
-	events, _ := capture.ReadEvents(repoRoot, sessionID)
+	events, _ := capture.ReadEvents(rc.StateRoot, sessionID)
 	for _, e := range events {
 		if e.Hook == "session_start" {
 			var d capture.SessionStartData
@@ -41,26 +49,32 @@ func runEnd(hookName, defaultExitReason string) error {
 		}
 	}
 
-	gitEnd := capture.CaptureGitEnd(repoRoot, startSHA)
+	gitEnd := capture.CaptureGitEnd(rc.WorkDir, startSHA)
 
 	data := capture.SessionEndData{
 		GitState:   gitEnd,
 		ExitReason: defaultExitReason,
 	}
 
-	if err := capture.AppendEvent(repoRoot, sessionID, hookName, data); err != nil {
+	if err := capture.AppendEvent(rc.StateRoot, sessionID, hookName, data); err != nil {
 		return err
 	}
 
 	// Finalize the session
-	session, err := capture.Finalize(repoRoot, sessionID)
+	session, err := capture.Finalize(rc.StateRoot, rc.WorkDir, sessionID)
 	if err != nil {
 		return err
 	}
 
-	// Write git ref, apply redaction, generate trace, clean up
-	if err := commitSession(repoRoot, session, ev.SessionID); err != nil {
-		log.Printf("etch: failed to commit session %s: %v", sessionID, err)
+	// Write git ref, apply redaction, generate trace, clean up.
+	// A failure here must be visible — never print ok while dropping data. The wip and
+	// mapping are deliberately left on disk so the next session_start recovery scan can
+	// retry the commit.
+	if err := commitSession(rc.StateRoot, session, ev.SessionID); err != nil {
+		msg := fmt.Sprintf("failed to commit session %s (wip retained for recovery): %v", sessionID, err)
+		log.Printf("etch: %s", msg)
+		printNotOK(msg)
+		return fmt.Errorf("%s", msg)
 	}
 
 	printOK()
diff --git a/internal/hooks/session_start.go b/internal/hooks/session_start.go
index adce5e0..94ae35f 100644
--- a/internal/hooks/session_start.go
+++ b/internal/hooks/session_start.go
@@ -21,15 +21,19 @@ func RunSessionStart() error {
 		return err
 	}
 
-	repoRoot := findRepoRoot()
-	if err := capture.EnsureDirs(repoRoot); err != nil {
+	rc, err := resolveContext()
+	if err != nil {
+		return err
+	}
+
+	if err := capture.EnsureDirs(rc.StateRoot); err != nil {
 		return err
 	}
 
 	// Recover any orphaned .wip files from crashed sessions
-	sessionsDir := filepath.Join(repoRoot, ".etch", "sessions")
-	timeout := recovery.ReadTimeoutFromSettings(repoRoot)
-	if n, err := recovery.RecoverAll(sessionsDir, repoRoot, timeout, &etchRefWriter{}); err != nil {
+	sessionsDir := filepath.Join(rc.StateRoot, ".etch", "sessions")
+	timeout := recovery.ReadTimeoutFromSettings(rc.StateRoot)
+	if n, err := recovery.RecoverAll(sessionsDir, rc.StateRoot, timeout, &etchRefWriter{}); err != nil {
 		log.Printf("etch: recovery scan failed: %v", err)
 	} else if n > 0 {
 		log.Printf("etch: recovered %d orphaned session(s)", n)
@@ -61,15 +65,15 @@ func RunSessionStart() error {
 	v := version.Version
 	agent.Version = &v
 
-	settings, _ := config.Load(repoRoot)
+	settings, _ := config.Load(rc.StateRoot)
 
 	data := capture.SessionStartData{
 		SessionID:     sessionID,
 		Agent:         agent,
 		Orchestration: capture.CaptureOrchestration(),
 		Machine:       capture.CaptureMachine(settings),
-		Operator:      capture.CaptureOperator(repoRoot),
-		GitState:      capture.CaptureGitState(repoRoot),
+		Operator:      capture.CaptureOperator(rc.WorkDir),
+		GitState:      capture.CaptureGitState(rc.WorkDir),
 		C11:           capture.CaptureC11(),
 		TranscriptRef: capture.CaptureTranscriptRef(ev.SessionRef),
 	}
@@ -78,12 +82,12 @@ func RunSessionStart() error {
 		data.ParentSessionID = &parentID
 	}
 
-	if err := capture.AppendEvent(repoRoot, sessionID, "session_start", data); err != nil {
+	if err := capture.AppendEvent(rc.StateRoot, sessionID, "session_start", data); err != nil {
 		return err
 	}
 
 	// Write mapping from Entire session ID to our ULID
-	if err := capture.WriteMapping(repoRoot, ev.SessionID, sessionID); err != nil {
+	if err := capture.WriteMapping(rc.StateRoot, ev.SessionID, sessionID); err != nil {
 		return err
 	}
 
diff --git a/internal/hooks/tool_use.go b/internal/hooks/tool_use.go
index 427a8a8..7bc5b22 100644
--- a/internal/hooks/tool_use.go
+++ b/internal/hooks/tool_use.go
@@ -20,8 +20,12 @@ func runToolUse(hookName string) error {
 		return err
 	}
 
-	repoRoot := findRepoRoot()
-	sessionID := capture.LookupMapping(repoRoot, ev.SessionID)
+	rc, err := resolveContext()
+	if err != nil {
+		return err
+	}
+
+	sessionID := capture.LookupMapping(rc.StateRoot, ev.SessionID)
 	if sessionID == "" {
 		printOK()
 		return nil
@@ -36,7 +40,7 @@ func runToolUse(hookName string) error {
 		data.FilePath = extractFilePath(ev.ToolName, ev.ToolInput)
 	}
 
-	if err := capture.AppendEvent(repoRoot, sessionID, hookName, data); err != nil {
+	if err := capture.AppendEvent(rc.StateRoot, sessionID, hookName, data); err != nil {
 		return err
 	}
 
diff --git a/internal/hooks/user_prompt_submit.go b/internal/hooks/user_prompt_submit.go
index d4aacce..b6e72b2 100644
--- a/internal/hooks/user_prompt_submit.go
+++ b/internal/hooks/user_prompt_submit.go
@@ -12,8 +12,12 @@ func RunUserPromptSubmit() error {
 		return err
 	}
 
-	repoRoot := findRepoRoot()
-	sessionID := capture.LookupMapping(repoRoot, ev.SessionID)
+	rc, err := resolveContext()
+	if err != nil {
+		return err
+	}
+
+	sessionID := capture.LookupMapping(rc.StateRoot, ev.SessionID)
 	if sessionID == "" {
 		printOK()
 		return nil
@@ -32,7 +36,7 @@ func RunUserPromptSubmit() error {
 		Truncated: truncated,
 	}
 
-	if err := capture.AppendEvent(repoRoot, sessionID, "user_prompt_submit", data); err != nil {
+	if err := capture.AppendEvent(rc.StateRoot, sessionID, "user_prompt_submit", data); err != nil {
 		return err
 	}
 
diff --git a/internal/redact/redact.go b/internal/redact/redact.go
index 78c2fe4..9f06a5a 100644
--- a/internal/redact/redact.go
+++ b/internal/redact/redact.go
@@ -1,18 +1,9 @@
 package redact
 
 import (
-	"fmt"
-
 	"forgejo.stage11.ai/s11/etch/internal/config"
 )
 
 func Redact(text string, settings config.Settings) string {
-	text = ScanSecrets(text)
-
-	custom := compileCustomPatterns(settings.RedactionPatterns)
-	for _, p := range custom {
-		text = p.Regex.ReplaceAllString(text, fmt.Sprintf("[REDACTED:%s]", p.Name))
-	}
-
-	return text
+	return newRedactor(settings).apply(text)
 }
diff --git a/internal/redact/redact_test.go b/internal/redact/redact_test.go
index 7ac041b..287ca2e 100644
--- a/internal/redact/redact_test.go
+++ b/internal/redact/redact_test.go
@@ -150,6 +150,266 @@ func TestScanSecretsPrivateKey(t *testing.T) {
 	}
 }
 
+// ETCH-28: the whole PEM block — material and END line — must be redacted,
+// not just the BEGIN header.
+func TestScanSecretsPrivateKeyFullBlock(t *testing.T) {
+	material1 := "MIIEpAIBAAKCAQEA7examplekeymaterial1234567890abcdefghijklmnopqr"
+	material2 := "stuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789exampleexampleexamp"
+	tests := []struct {
+		name  string
+		input string
+	}{
+		{"RSA block", "-----BEGIN RSA PRIVATE KEY-----\n" + material1 + "\n" + material2 + "\n-----END RSA PRIVATE KEY-----"},
+		{"PKCS8 block", "-----BEGIN PRIVATE KEY-----\n" + material1 + "\n-----END PRIVATE KEY-----"},
+		{"OPENSSH block", "-----BEGIN OPENSSH PRIVATE KEY-----\n" + material1 + "\n-----END OPENSSH PRIVATE KEY-----"},
+		{"ENCRYPTED block", "-----BEGIN ENCRYPTED PRIVATE KEY-----\n" + material1 + "\n-----END ENCRYPTED PRIVATE KEY-----"},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			out := ScanSecrets("before\n" + tt.input + "\nafter")
+			if !strings.Contains(out, "[REDACTED:private-key]") {
+				t.Fatalf("block not redacted: %s", out)
+			}
+			if strings.Contains(out, material1) || strings.Contains(out, material2) {
+				t.Errorf("key MATERIAL leaked: %s", out)
+			}
+			if strings.Contains(out, "-----END") {
+				t.Errorf("END line leaked: %s", out)
+			}
+			if !strings.Contains(out, "before") || !strings.Contains(out, "after") {
+				t.Errorf("surrounding text clobbered: %s", out)
+			}
+		})
+	}
+}
+
+// Truncated block (no END marker): header plus material lines still redact.
+func TestScanSecretsPrivateKeyTruncated(t *testing.T) {
+	material := "MIIEpAIBAAKCAQEA7examplekeymaterial1234567890abcdefghijklmnopqr"
+	input := "-----BEGIN RSA PRIVATE KEY-----\n" + material + "\nand then prose continues"
+	out := ScanSecrets(input)
+	if !strings.Contains(out, "[REDACTED:private-key]") {
+		t.Fatalf("truncated block not redacted: %s", out)
+	}
+	if strings.Contains(out, material) {
+		t.Errorf("truncated key material leaked: %s", out)
+	}
+	if !strings.Contains(out, "and then prose continues") {
+		t.Errorf("prose after truncated block clobbered: %s", out)
+	}
+}
+
+// ETCH-27: bare JWTs (three base64url segments) must be redacted.
+func TestScanSecretsJWT(t *testing.T) {
+	tests := []struct {
+		name  string
+		input string
+	}{
+		{"ticket repro", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature_part"},
+		{"in prose", "the token eyJhbGciOiJSUzI1NiIsImtpZCI6ImtleTEifQ.eyJpc3MiOiJodHRwczovL2V4YW1wbGUifQ.dGhpc2lzYXNpZ25hdHVyZQ ended the line"},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			out := ScanSecrets(tt.input)
+			if !strings.Contains(out, "[REDACTED:jwt]") {
+				t.Errorf("JWT not redacted: %s", out)
+			}
+			if strings.Contains(out, "eyJ") {
+				t.Errorf("JWT content leaked: %s", out)
+			}
+		})
+	}
+}
+
+func TestScanSecretsJWTNegative(t *testing.T) {
+	inputs := []string{
+		"x.y.z",
+		"version 1.2.3 released",
+		"see docs.example.com today",
+		"eyJshort.x.y", // segments below minimum length
+	}
+	for _, input := range inputs {
+		out := ScanSecrets(input)
+		if out != input {
+			t.Errorf("non-JWT clobbered: %q → %q", input, out)
+		}
+	}
+}
+
+// ETCH-40 finding 7: modern prefixed OpenAI keys.
+func TestScanSecretsOpenAIModern(t *testing.T) {
+	tests := []struct {
+		name  string
+		input string
+	}{
+		{"project key", "key is sk-proj-AbCdEf123456_789-abcdefGHIJKL"},
+		{"service account key", "sk-svcacct-AbCdEf123456_789-abcdefGHIJKL"},
+		{"admin key", "sk-admin-AbCdEf123456_789-abcdefGHIJKL"},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			out := ScanSecrets(tt.input)
+			if !strings.Contains(out, "[REDACTED:openai-api-key]") {
+				t.Errorf("modern OpenAI key not redacted: %s", out)
+			}
+			if strings.Contains(out, "AbCdEf123456") {
+				t.Errorf("key body leaked: %s", out)
+			}
+		})
+	}
+}
+
+// ETCH-29: documentation placeholders must NOT be redacted.
+func TestScanSecretsPlaceholdersPreserved(t *testing.T) {
+	inputs := []string{
+		"use sk-ant-EXAMPLE as a placeholder",
+		"sk-ant-PLACEHOLDER",
+		"sk-DOCUMENTATION-NOT-A-KEY",
+		"sk-proj-EXAMPLE", // body too short to be real
+		"sk-ant-yourkeyhere",
+	}
+	for _, input := range inputs {
+		out := ScanSecrets(input)
+		if out != input {
+			t.Errorf("placeholder clobbered: %q → %q", input, out)
+		}
+	}
+}
+
+// Real-shape anthropic keys (tier segment + long body) still redact.
+func TestScanSecretsAnthropicRealShapes(t *testing.T) {
+	tests := []struct {
+		name  string
+		input string
+	}{
+		{"api key", "ANTHROPIC_API_KEY=sk-ant-api03-AbCd1234efGh5678IjKl9012MnOp"},
+		{"oauth token", "sk-ant-oat01-AbCd1234efGh5678IjKl9012MnOp"},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			out := ScanSecrets(tt.input)
+			if !strings.Contains(out, "[REDACTED:anthropic-api-key]") {
+				t.Errorf("anthropic key not redacted: %s", out)
+			}
+			if strings.Contains(out, "AbCd1234efGh") {
+				t.Errorf("key body leaked: %s", out)
+			}
+		})
+	}
+}
+
+// ETCH-26: bare AWS secret access keys (no key= label) must be redacted.
+func TestScanSecretsBareAWSSecret(t *testing.T) {
+	tests := []struct {
+		name  string
+		input string
+	}{
+		{"ticket repro bare", "the key wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY appeared"},
+		{"start of string", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			out := ScanSecrets(tt.input)
+			if !strings.Contains(out, "[REDACTED:aws-secret-key]") {
+				t.Errorf("bare AWS secret not redacted: %s", out)
+			}
+			if strings.Contains(out, "wJalrXUtnFEMI") {
+				t.Errorf("secret leaked: %s", out)
+			}
+		})
+	}
+}
+
+// Structural fields that flow through DeepRedact must NOT trip the bare-AWS
+// pattern: git SHAs, sha256 hashes, ULIDs, long base64 blobs.
+func TestScanSecretsBareAWSSecretNegative(t *testing.T) {
+	inputs := []string{
+		"01a2ca4f3e8b9d2c5a7f1e6b4d8c3a9f2e5b7d1c",                          // 40-char lowercase git SHA
+		"sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", // 64-hex
+		"01ARZ3NDEKTSV4RRFFQ69G5FAV",                                        // ULID (26 chars)
+		"dGhpc2lzYWxvbmdlcmJhc2U2NGJsb2J0aGF0Z29lc29uYW5kb24=",              // base64 blob ≠ 40 chars
+		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",                          // 40 chars but no lower/digit
+	}
+	for _, input := range inputs {
+		out := ScanSecrets(input)
+		if out != input {
+			t.Errorf("structural value clobbered: %q → %q", input, out)
+		}
+	}
+}
+
+// Documented, accepted miss: a 40-char secret embedded inside a LONGER
+// contiguous base64 run is not caught (the maximal run fails len==40).
+// This pins the best-effort tradeoff so it reads as intentional.
+func TestScanSecretsBareAWSSecretKnownMiss(t *testing.T) {
+	input := "prefix00" + "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" // one 48-char run
+	out := ScanSecrets(input)
+	if out != input {
+		t.Errorf("expected known miss to pass through, got: %q", out)
+	}
+}
+
+// Documented, accepted false positive: a PATH whose base64-ish run (slashes
+// included) is exactly 40 chars trips the bare-AWS shape — observed with
+// "etch/sessions/<26-char ULID>" (14+26=40). Over-redaction of path metadata
+// is the safe failure direction; this pins the behavior as known.
+func TestScanSecretsBareAWSSecretPathFP(t *testing.T) {
+	input := ".etch/sessions/01KTH9VSXVVAJMQTDJRDEWQQPB.wip.jsonl"
+	out := ScanSecrets(input)
+	if !strings.Contains(out, "[REDACTED:aws-secret-key]") {
+		t.Logf("note: exact-40 path FP no longer occurs (acceptable precision improvement): %q", out)
+	}
+}
+
+// ETCH-39: common credential keywords.
+func TestScanSecretsCredentialKeywords(t *testing.T) {
+	tests := []struct {
+		name  string
+		input string
+	}{
+		{"password equals", "password=SuperSecret123"},
+		{"passwd colon", "passwd: SuperSecret123"},
+		{"pwd equals", "pwd=SuperSecret123"},
+		{"bare token", "token = abcdef1234567890"},
+		{"client_secret", "client_secret=abcdef1234567890"},
+		{"env DB_PASS", "DB_PASS=hunter2password"},
+		{"env AWS_SECRET", "AWS_SECRET=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"},
+		{"env SECRET", "SECRET=abcdef1234567890"},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			out := ScanSecrets(tt.input)
+			if !strings.Contains(out, "[REDACTED:") {
+				t.Errorf("credential not redacted: %s", out)
+			}
+		})
+	}
+}
+
+// Precision/recall pinning: keyword must be immediately followed by : or =,
+// so token-count prose survives; compass= is a documented, accepted FP.
+func TestScanSecretsCredentialKeywordsNegative(t *testing.T) {
+	preserved := []string{
+		"tokens: 4096",
+		"max_tokens=8192",
+		"passwords are stored hashed", // no [:=] after keyword
+		"pwd is /home/user",           // value too short
+	}
+	for _, input := range preserved {
+		out := ScanSecrets(input)
+		if out != input {
+			t.Errorf("prose clobbered: %q → %q", input, out)
+		}
+	}
+
+	// Documented accepted false positive: a keyword suffix inside a longer
+	// identifier still matches (best-effort regex, leak-averse direction).
+	fp := "compass=abcdef123456"
+	if out := ScanSecrets(fp); out == fp {
+		t.Logf("note: compass= no longer a false positive (acceptable)")
+	}
+}
+
 func TestScanSecretsPassthrough(t *testing.T) {
 	inputs := []string{
 		"just a normal string",
diff --git a/internal/redact/secrets.go b/internal/redact/secrets.go
index 7a8d614..10c90de 100644
--- a/internal/redact/secrets.go
+++ b/internal/redact/secrets.go
@@ -6,24 +6,59 @@ import (
 )
 
 type secretPattern struct {
-	Name    string
-	Regex   *regexp.Regexp
+	Name  string
+	Regex *regexp.Regexp
+	// Validate, when non-nil, decides whether a regex match is really a
+	// secret. RE2 has no lookarounds, so class-composition checks (e.g.
+	// "must contain upper+lower+digit") live in code instead.
+	Validate func(match string) bool
 }
 
+// Pattern order is semantic — do not reorder casually:
+//   - the full private-key block runs before the header-only fallback
+//   - bearer-token runs before jwt so "Bearer <jwt>" keeps its more
+//     specific marker name
+//   - labeled secrets run before the bare 40-char AWS form
 var builtinPatterns = []secretPattern{
 	{
-		Name:  "aws-access-key",
-		Regex: regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
+		// Full PEM block: header, key material, and END line (ETCH-28).
+		// [A-Z ]* covers RSA/EC/DSA plus OPENSSH/ENCRYPTED variants.
+		Name:  "private-key",
+		Regex: regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
 	},
 	{
-		Name:  "aws-secret-key",
-		Regex: regexp.MustCompile(`(?i)aws_secret_access_key\s*[:=]\s*\S+`),
+		// Truncated-block fallback (no END marker): the header plus any
+		// following long base64 lines. Material lines are 40+ chars, so
+		// ordinary prose after a lone header is left alone.
+		Name:  "private-key",
+		Regex: regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----(?:\s*[A-Za-z0-9+/=]{40,})*`),
+	},
+	{
+		Name:  "bearer-token",
+		Regex: regexp.MustCompile(`Bearer\s+[a-zA-Z0-9._\-]+`),
+	},
+	{
+		// Bare JWT: three base64url segments; eyJ is base64 of `{"`
+		// (ETCH-27). Minimum segment lengths keep x.y.z prose safe.
+		Name:  "jwt",
+		Regex: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{6,}`),
 	},
 	{
+		// Real Anthropic keys carry a tier segment (api03-, oat01-,
+		// sid01-) and a long body; doc placeholders like sk-ant-EXAMPLE
+		// must pass through (ETCH-29).
 		Name:  "anthropic-api-key",
-		Regex: regexp.MustCompile(`sk-ant-[a-zA-Z0-9_-]+`),
+		Regex: regexp.MustCompile(`sk-ant-[a-z]{2,8}[0-9]{2}-[A-Za-z0-9_-]{16,}`),
 	},
 	{
+		// Modern prefixed OpenAI keys (ETCH-40 finding 7). Deliberately
+		// not a generic sk-[\w-]{20,}: that would clobber doc strings
+		// like sk-DOCUMENTATION-NOT-A-KEY.
+		Name:  "openai-api-key",
+		Regex: regexp.MustCompile(`sk-(?:proj|svcacct|admin)-[A-Za-z0-9_-]{20,}`),
+	},
+	{
+		// Legacy unprefixed OpenAI keys.
 		Name:  "openai-api-key",
 		Regex: regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`),
 	},
@@ -36,22 +71,73 @@ var builtinPatterns = []secretPattern{
 		Regex: regexp.MustCompile(`sk_test_[a-zA-Z0-9]+`),
 	},
 	{
-		Name:  "generic-secret",
-		Regex: regexp.MustCompile(`(?i)(api[_-]?key|api[_-]?secret|access[_-]?token|secret[_-]?key)\s*[:=]\s*["']?[a-zA-Z0-9_\-]{16,}["']?`),
+		Name:  "aws-access-key",
+		Regex: regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
 	},
 	{
-		Name:  "bearer-token",
-		Regex: regexp.MustCompile(`Bearer\s+[a-zA-Z0-9._\-]+`),
+		Name:  "aws-secret-key",
+		Regex: regexp.MustCompile(`(?i)aws_secret_access_key\s*[:=]\s*\S+`),
 	},
 	{
-		Name:  "private-key",
-		Regex: regexp.MustCompile(`-----BEGIN (RSA |EC |DSA )?PRIVATE KEY-----`),
+		// Generic credential assignments (ETCH-39 + ETCH-26's AWS_SECRET=
+		// variants). The keyword must be immediately followed by : or =,
+		// so prose like "tokens: 4096" or "max_tokens=8192" is untouched.
+		// The value class includes /+= for base64-shaped values.
+		Name:  "generic-secret",
+		Regex: regexp.MustCompile(`(?i)(api[_-]?key|api[_-]?secret|access[_-]?token|secret[_-]?key|client[_-]?secret|password|passwd|pwd|pass|token|secret)\s*[:=]\s*["']?[A-Za-z0-9_\-/+=]{8,}["']?`),
 	},
+	{
+		// Bare AWS secret access key (ETCH-26): exactly 40 base64 chars
+		// with mixed character classes. {40,} grabs the maximal run, so
+		// the len==40 validator rejects 40-hex git SHAs (single-case /
+		// all-hex), sha256 hex (64 chars), and longer base64 blobs.
+		// Known, accepted miss: a real 40-char secret embedded INSIDE a
+		// longer contiguous base64 run is not redacted (best-effort).
+		// Runs last so labeled patterns win their specific marker names.
+		Name:     "aws-secret-key",
+		Regex:    regexp.MustCompile(`[A-Za-z0-9/+=]{40,}`),
+		Validate: looksLikeBareAWSSecret,
+	},
+}
+
+// looksLikeBareAWSSecret reports whether a maximal base64 run has the
+// shape of a bare AWS secret access key: exactly 40 chars, mixed case,
+// at least one digit, and not a plain hex string (which would be a SHA).
+func looksLikeBareAWSSecret(m string) bool {
+	if len(m) != 40 {
+		return false
+	}
+	var hasUpper, hasLower, hasDigit, nonHex bool
+	for _, c := range m {
+		switch {
+		case c >= 'A' && c <= 'Z':
+			hasUpper = true
+			if c > 'F' {
+				nonHex = true
+			}
+		case c >= 'a' && c <= 'z':
+			hasLower = true
+			if c > 'f' {
+				nonHex = true
+			}
+		case c >= '0' && c <= '9':
+			hasDigit = true
+		default: // '/', '+', '='
+			nonHex = true
+		}
+	}
+	return hasUpper && hasLower && hasDigit && nonHex
 }
 
 func ScanSecrets(text string) string {
 	for _, p := range builtinPatterns {
-		text = p.Regex.ReplaceAllString(text, fmt.Sprintf("[REDACTED:%s]", p.Name))
+		marker := fmt.Sprintf("[REDACTED:%s]", p.Name)
+		text = p.Regex.ReplaceAllStringFunc(text, func(m string) string {
+			if p.Validate != nil && !p.Validate(m) {
+				return m
+			}
+			return marker
+		})
 	}
 	return text
 }
diff --git a/internal/redact/walk.go b/internal/redact/walk.go
new file mode 100644
index 0000000..1b02359
--- /dev/null
+++ b/internal/redact/walk.go
@@ -0,0 +1,153 @@
+package redact
+
+import (
+	"reflect"
+
+	"forgejo.stage11.ai/s11/etch/internal/config"
+)
+
+// redactor applies the builtin patterns plus custom patterns compiled once,
+// so a deep walk doesn't recompile the custom set per string.
+type redactor struct {
+	custom []secretPattern
+}
+
+func newRedactor(settings config.Settings) *redactor {
+	return &redactor{custom: compileCustomPatterns(settings.RedactionPatterns)}
+}
+
+func (r *redactor) apply(text string) string {
+	text = ScanSecrets(text)
+	for _, p := range r.custom {
+		text = p.Regex.ReplaceAllString(text, "[REDACTED:"+p.Name+"]")
+	}
+	return text
+}
+
+// DeepRedact applies redaction to every string-bearing field reachable from
+// v: struct fields, slice elements, map keys and values, pointers, and
+// interface-boxed values. This is the single commit-boundary pass — callers
+// redact the whole finalized record instead of remembering per-field calls
+// (ETCH-40 finding 5). v must be a non-nil pointer; anything else is a no-op.
+func DeepRedact(v any, settings config.Settings) {
+	if v == nil {
+		return
+	}
+	rv := reflect.ValueOf(v)
+	if rv.Kind() != reflect.Ptr || rv.IsNil() {
+		return
+	}
+	walkValue(rv, newRedactor(settings))
+}
+
+// walkValue returns a (possibly new) value with all reachable strings
+// redacted. Map values and interface-boxed values are not addressable, so
+// the walker returns results and containers write them back (SetMapIndex,
+// field/index Set) rather than relying on pure in-place mutation.
+func walkValue(v reflect.Value, r *redactor) reflect.Value {
+	switch v.Kind() {
+	case reflect.String:
+		s := v.String()
+		red := r.apply(s)
+		if red == s {
+			return v
+		}
+		nv := reflect.New(v.Type()).Elem() // preserves named string types
+		nv.SetString(red)
+		return nv
+
+	case reflect.Ptr:
+		if v.IsNil() {
+			return v
+		}
+		elem := v.Elem()
+		res := walkValue(elem, r)
+		if elem.CanSet() {
+			elem.Set(res)
+		}
+		return v
+
+	case reflect.Interface:
+		if v.IsNil() {
+			return v
+		}
+		// The dynamic value behind an interface is not addressable:
+		// copy it out, walk the copy, and re-box.
+		dyn := v.Elem()
+		cp := reflect.New(dyn.Type()).Elem()
+		cp.Set(dyn)
+		res := walkValue(cp, r)
+		out := reflect.New(v.Type()).Elem()
+		out.Set(res)
+		return out
+
+	case reflect.Struct:
+		sv := v
+		if !sv.CanAddr() {
+			cp := reflect.New(v.Type()).Elem()
+			cp.Set(v)
+			sv = cp
+		}
+		for i := 0; i < sv.NumField(); i++ {
+			f := sv.Field(i)
+			if !f.CanSet() { // unexported field
+				continue
+			}
+			f.Set(walkValue(f, r))
+		}
+		return sv
+
+	case reflect.Slice:
+		if v.IsNil() {
+			return v
+		}
+		for i := 0; i < v.Len(); i++ {
+			el := v.Index(i)
+			el.Set(walkValue(el, r))
+		}
+		return v
+
+	case reflect.Array:
+		av := v
+		if !av.CanAddr() {
+			cp := reflect.New(v.Type()).Elem()
+			cp.Set(v)
+			av = cp
+		}
+		for i := 0; i < av.Len(); i++ {
+			el := av.Index(i)
+			if el.CanSet() {
+				el.Set(walkValue(el, r))
+			}
+		}
+		return av
+
+	case reflect.Map:
+		if v.IsNil() {
+			return v
+		}
+		for _, k := range v.MapKeys() {
+			val := v.MapIndex(k)
+			cv := reflect.New(val.Type()).Elem()
+			cv.Set(val)
+			nv := walkValue(cv, r)
+
+			nk := k
+			if k.Kind() == reflect.String {
+				redK := r.apply(k.String())
+				if redK != k.String() {
+					nk = reflect.New(k.Type()).Elem()
+					nk.SetString(redK)
+					// Two distinct secret keys can collapse onto the
+					// same marker; last write wins (best-effort).
+					v.SetMapIndex(k, reflect.Value{}) // delete old key
+				}
+			}
+			v.SetMapIndex(nk, nv)
+		}
+		return v
+
+	default:
+		return v
+	}
+}
diff --git a/internal/redact/walk_test.go b/internal/redact/walk_test.go
new file mode 100644
index 0000000..fc49cbb
--- /dev/null
+++ b/internal/redact/walk_test.go
@@ -0,0 +1,180 @@
+package redact
+
+import (
+	"strings"
+	"testing"
+
+	"forgejo.stage11.ai/s11/etch/internal/config"
+	"forgejo.stage11.ai/s11/etch/internal/schema"
+)
+
+const (
+	walkSecret = "sk-proj-AbCdEf123456_789-abcdefGHIJKL"
+	walkJWT    = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature_part"
+)
+
+func strptr(s string) *string { return &s }
+
+// ETCH-40 finding 5: secrets in FilesTouched paths must be redacted.
+func TestDeepRedactFilesTouchedPath(t *testing.T) {
+	s := &schema.Session{
+		FilesTouched: []schema.FileEntry{
+			{Path: "/Users/x/keys/" + walkSecret + ".pem", Action: "modified"},
+			{Path: "internal/redact/walk.go", Action: "modified"},
+		},
+	}
+	DeepRedact(s, config.Defaults())
+	if strings.Contains(s.FilesTouched[0].Path, walkSecret) {
+		t.Errorf("secret in file path not redacted: %s", s.FilesTouched[0].Path)
+	}
+	if !strings.Contains(s.FilesTouched[0].Path, "[REDACTED:openai-api-key]") {
+		t.Errorf("expected marker in file path: %s", s.FilesTouched[0].Path)
+	}
+	if s.FilesTouched[1].Path != "internal/redact/walk.go" {
+		t.Errorf("clean path clobbered: %s", s.FilesTouched[1].Path)
+	}
+}
+
+// Secrets as ToolUse.ByTool map KEYS must be rewritten, counts preserved.
+func TestDeepRedactToolUseKeys(t *testing.T) {
+	s := &schema.Session{
+		ToolUse: &schema.ToolUse{
+			TotalCalls: 7,
+			ByTool: map[string]int{
+				"Bash " + walkJWT: 3,
+				"Read":            4,
+			},
+		},
+	}
+	DeepRedact(s, config.Defaults())
+	if s.ToolUse.TotalCalls != 7 {
+		t.Errorf("non-string field changed: %d", s.ToolUse.TotalCalls)
+	}
+	if s.ToolUse.ByTool["Read"] != 4 {
+		t.Errorf("clean key clobbered: %v", s.ToolUse.ByTool)
+	}
+	for k, v := range s.ToolUse.ByTool {
+		if strings.Contains(k, "eyJ") {
+			t.Errorf("secret survived in map key: %s", k)
+		}
+		if strings.HasPrefix(k, "Bash ") {
+			if !strings.Contains(k, "[REDACTED:jwt]") {
+				t.Errorf("expected jwt marker in key: %s", k)
+			}
+			if v != 3 {
+				t.Errorf("count lost on key rewrite: %s=%d", k, v)
+			}
+		}
+	}
+	if len(s.ToolUse.ByTool) != 2 {
+		t.Errorf("expected 2 keys, got %v", s.ToolUse.ByTool)
+	}
+}
+
+// Secrets nested inside Orchestration.Extra (map[string]any — interface-boxed,
+// non-addressable) must be redacted, including nested maps and slices.
+func TestDeepRedactOrchestrationExtra(t *testing.T) {
+	s := &schema.Session{
+		Orchestration: &schema.Orchestration{
+			Type: "delegator",
+			Extra: map[string]any{
+				"note":  "key is " + walkSecret,
+				"count": 42,
+				"nested": map[string]any{
+					"deep": "token " + walkJWT + " here",
+				},
+				"list": []any{"clean", "jwt: " + walkJWT},
+			},
+		},
+	}
+	DeepRedact(s, config.Defaults())
+	extra := s.Orchestration.Extra
+	if note := extra["note"].(string); strings.Contains(note, walkSecret) {
+		t.Errorf("secret in Extra value not redacted: %s", note)
+	}
+	if extra["count"].(int) != 42 {
+		t.Errorf("non-string Extra value changed: %v", extra["count"])
+	}
+	nested := extra["nested"].(map[string]any)
+	if deep := nested["deep"].(string); strings.Contains(deep, "eyJ") {
+		t.Errorf("nested secret not redacted: %s", deep)
+	}
+	list := extra["list"].([]any)
+	if list[0].(string) != "clean" {
+		t.Errorf("clean list element clobbered: %v", list[0])
+	}
+	if strings.Contains(list[1].(string), "eyJ") {
+		t.Errorf("secret in list not redacted: %v", list[1])
+	}
+}
+
+// Prompt text still redacts through the deep walk (the original behavior).
+func TestDeepRedactPromptText(t *testing.T) {
+	s := &schema.Session{
+		Prompt: &schema.Prompt{Text: "fix it, key " + walkSecret, Source: "user_prompt_submit"},
+	}
+	DeepRedact(s, config.Defaults())
+	if strings.Contains(s.Prompt.Text, walkSecret) {
+		t.Errorf("prompt secret not redacted: %s", s.Prompt.Text)
+	}
+	if s.Prompt.Source != "user_prompt_submit" {
+		t.Errorf("prompt source clobbered: %s", s.Prompt.Source)
+	}
+}
+
+// Structural fields survive intact: SHAs, ULIDs, hostname hashes.
+func TestDeepRedactStructuralFieldsSurvive(t *testing.T) {
+	sha := "01a2ca4f3e8b9d2c5a7f1e6b4d8c3a9f2e5b7d1c"
+	hash := "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
+	s := &schema.Session{
+		SchemaVersion: schema.SchemaVersion,
+		SessionID:     "01ARZ3NDEKTSV4RRFFQ69G5FAV",
+		Status:        "complete",
+		ExitReason:    "normal",
+		GitStart:      &schema.GitState{Branch: "fix/redaction-batch", HeadSHA: sha},
+		Machine:       &schema.Machine{HostnameHash: hash, OS: "darwin"},
+	}
+	DeepRedact(s, config.Defaults())
+	if s.GitStart.HeadSHA != sha {
+		t.Errorf("git SHA clobbered: %s", s.GitStart.HeadSHA)
+	}
+	if s.Machine.HostnameHash != hash {
+		t.Errorf("hostname hash clobbered: %s", s.Machine.HostnameHash)
+	}
+	if s.SessionID != "01ARZ3NDEKTSV4RRFFQ69G5FAV" {
+		t.Errorf("ULID clobbered: %s", s.SessionID)
+	}
+	if s.GitStart.Branch != "fix/redaction-batch" {
+		t.Errorf("branch clobbered: %s", s.GitStart.Branch)
+	}
+}
+
+// Nil pointers everywhere must not panic; pointer-to-string fields redact.
+func TestDeepRedactNilSafety(t *testing.T) {
+	DeepRedact(nil, config.Defaults())
+	DeepRedact((*schema.Session)(nil), config.Defaults())
+	DeepRedact(42, config.Defaults()) // non-pointer: no-op
+
+	s := &schema.Session{
+		ParentSessionID: strptr("parent " + walkSecret),
+		Agent:           schema.Agent{Runtime: "claude-code", Model: nil},
+	}
+	DeepRedact(s, config.Defaults())
+	if strings.Contains(*s.ParentSessionID, walkSecret) {
+		t.Errorf("pointer-to-string field not redacted: %s", *s.ParentSessionID)
+	}
+}
+
+// Custom settings patterns apply through the deep walk too.
+func TestDeepRedactCustomPatterns(t *testing.T) {
+	s := &schema.Session{
+		FilesTouched: []schema.FileEntry{{Path: "docs/MYTOKEN-ABCD1234.txt", Action: "added"}},
+	}
+	DeepRedact(s, config.Settings{RedactionPatterns: []string{`MYTOKEN-[A-Z0-9]{8}`}})
+	if strings.Contains(s.FilesTouched[0].Path, "MYTOKEN-ABCD1234") {
+		t.Errorf("custom pattern not applied in walk: %s", s.FilesTouched[0].Path)
+	}
+	if !strings.Contains(s.FilesTouched[0].Path, "[REDACTED:custom-0]") {
+		t.Errorf("expected custom marker: %s", s.FilesTouched[0].Path)
+	}
+}

```

## Review Checklist

Evaluate the diff against each category. For every issue found, state the
file, line number, severity (critical / major / minor), and a concrete fix.

### Correctness
- Does the implementation match the plan and acceptance criteria?
- Are there logic errors, off-by-one errors, or missing edge cases?
- Are error paths handled correctly?
- Are return values and types correct?
- Are all branches reachable and necessary?

### Security
- Any injection vectors (command injection, SQL injection, XSS, path traversal)?
- Secrets or credentials exposed or logged?
- Input validation at system boundaries?
- Unsafe deserialization or file operations?

### Quality
- Does the code follow existing patterns and conventions in this codebase?
- Are names clear, consistent, and idiomatic?
- Is complexity appropriate — no over-engineering, no under-engineering?
- Is the code readable without excessive comments?
- Are imports, dependencies, and module boundaries clean?

### Testing
- Are changes covered by tests?
- Do tests verify behavior, not implementation details?
- Are edge cases and error paths tested?
- Are test names descriptive of what they verify?

### Performance
- Any obvious performance issues (N+1 queries, unbounded loops, unnecessary allocations)?
- Are there concurrency concerns (race conditions, deadlocks)?
- Are resources properly cleaned up (file handles, connections)?

## Output Format

Write your review as a structured markdown document with these sections:

### 1. Verdict

One of:
- **PASS** — Implementation is correct and meets acceptance criteria.
- **FAIL (implementation-level)** — Plan is sound but implementation has issues that need fixing. The task should return to `in_progress` for rework.
- **FAIL (plan-level)** — The approach itself is flawed. The task should return to `in_planning` for a revised plan.

### 2. Summary

2-3 sentences: what was reviewed, what the overall quality is, and the key finding (if any).

### 3. Issues

Ordered by severity (critical first). For each issue:

```
**[SEVERITY] file:line — Short description**
Description of the problem and why it matters.
**Fix:** Concrete suggestion for how to resolve it.
```

If no issues found, write "No issues found."

### 4. Positive Observations

What was done well. This keeps reviews balanced and acknowledges good work.
Mention specific patterns, test coverage, or design decisions that are noteworthy.

---

Write your review to: <write output here>
