# Plan Review: ETCH-17 — Auto-Capture Investigation & Fix (+ ETCH-20 Hook Contract Docs)

## 1. Verdict

**FAIL (plan-level)**

The plan is excellent in its diagnosis and architecture — the root cause is correct and independently verified against Entire's source. But two MAJOR gaps directly threaten whether the fix actually works and whether the headline output field (`model`) can be captured at all under the chosen "native payload" approach. Both touch the stated acceptance gate. They are specific and quickly resolvable; fold them into the "Plan-Review Cycle 1 Resolutions" section and this is a clean PASS.

## 2. Summary

I reviewed the ETCH-17/ETCH-20 plan, independently verifying its root-cause claims against the Entire CLI source and against Etch's current `info.go`, `hooks/common.go`, `hooks/session_start.go`, `cmd/.../main.go`, `README.md`, and `scripts/smoke.sh`. The investigation is strong and accurate: the four-failure causal chain (info protocol mismatch → silent discovery skip, missing install-side subcommands, broken `entire agent add`, hook-dialect mismatch) all check out, and the "direct dispatch, not via `entire hooks etch`" architecture decision is well-reasoned and correct. The key concerns are (a) the plan assumes Claude Code delivers `model` as a native top-level hook field, which it almost certainly does not — model lives in the transcript JSONL, so the acceptance gate's "correct model field" may be silently unmet; and (b) the install-side protocol response shapes for `detect`/`are-hooks-installed`/`uninstall-hooks` are plan-invented rather than pinned to Entire's actual response structs, risking a silent re-creation of the very enable failure this ticket is about.

## 3. Issues

**[MAJOR] §3 (dual-dialect) + §5 (validation gate) — `model` is not a native top-level Claude Code hook field**

The dialect table maps model to `model (top level, SessionStart)` for the native Claude Code dialect, and the risk section treats a missing model as an edge case affecting only "older builds" (`field stays null — same as today`). This is very likely a misdiagnosis, not an edge case. Claude Code's native hook payloads (SessionStart, UserPromptSubmit, PreToolUse, PostToolUse, Stop, SessionEnd) carry `session_id`, `transcript_path`, `cwd`, `hook_event_name`, and event-specific fields (`source`, `prompt`, `tool_name`, `tool_input`, `tool_response`, `stop_hook_active`) — but **not** `model`. The model identifier lives inside the transcript JSONL referenced by `transcript_path`. Today's capture works only because Entire's *internal* dialect synthesizes `raw_data.model` (see `internal/hooks/session_start.go:45-59`). Under direct dispatch with native payloads, that source disappears and `agent.Model` will be `nil` for **every real session** — yet the acceptance gate (step 5) explicitly requires `session.json` to parse "with correct model/prompt/tool fields," and a null-model record still validates, so the smoke/unit gates would pass green while the headline field is silently empty. This is the exact silent-drop failure mode the ticket exists to kill, just relocated.
**Recommendation:** Confirm where `model` actually appears in Claude Code's native hook payloads (it is not top-level). Plan to derive it from the transcript at `transcript_path` (parse the first/last assistant message's `model`), or explicitly descope model capture for native dispatch and adjust the step-5 acceptance gate so it does not assert a field the contract cannot deliver. Either way the plan must stop treating missing model as an "older builds" rarity.

**[MAJOR] §2 (new subcommands) — Install-side response shapes are invented, not pinned to Entire's structs**

Only `install-hooks` → `{"hooks_installed": N}` is verified against Entire (`HooksInstalledCountResponse`). The plan asserts `detect` → `{"present": true}`, `are-hooks-installed` → `{"installed": bool}`, and `uninstall-hooks` → `{}` as if designed by Etch, but these are *consumed* by Entire's external-agent client during `entire enable --agent etch`. If any response key/type does not match the struct Entire unmarshals into, discovery/enable fails — silently or with a confusing error — which re-creates ETCH-17's core symptom (enable succeeds-ish, zero capture, no guidance). The whole point of this ticket is that an unverified protocol assumption produced a silent no-op.
**Recommendation:** Pin each install-side subcommand's expected response to the exact struct in Entire's `cmd/entire/cli/agent/external/types.go` (e.g., the response types Entire decodes for `detect`, `are-hooks-installed`, `uninstall-hooks`), cite the type names/fields in the plan the same way `info`'s protocol-v1 shape is cited, and add an explicit unit/smoke assertion that `entire enable --agent etch` returns RC=0 *and* that Entire logged successful discovery+install (not just that settings.json got written).

**[MAJOR] §5 (validation gate) — A single headless `claude -p` run may produce zero refs even when everything works**

Finalization happens on `session_end`; if Claude Code's `SessionEnd` does not fire (or fires unreliably) in headless one-shot `claude -p` mode, the `.wip.jsonl` buffer is only finalized by crash recovery on the **next** `session_start`. So a single headless run could legitimately end with nothing under `refs/etch/sessions/` despite a fully working install — and the gate would read as a failure for an environmental reason, or worse, a second run masks it. The plan's fallback to "hook-faithful simulation" is good, but the primary gate's success condition is fragile.
**Recommendation:** Either (a) verify `SessionEnd` reliably fires under `claude -p --dangerously-skip-permissions` before relying on it, or (b) make the gate fire a second `session_start` (or invoke crash recovery) to force finalization, or (c) make simulation the primary gate and the live run corroborating evidence. State the chosen finalization trigger explicitly.

**[MINOR] §"Confirmed root cause" — Verification version vs. the version users actually run**

The plan states it cloned the exact 0.6.3 tag (`17720a12`). My independent verification matched the protocol claims, but against Entire's current `main` (`ab2c6169`), not `17720a12`. The README/QA were tested against 0.6.3, which is what users have. A protocol-shape drift between `main` and 0.6.3 would invalidate the install-side shapes silently.
**Recommendation:** Pin all protocol shapes (info + install-side) against the `0.6.3` tag specifically, and note in the plan/PR which Entire version each shape was read from. If Etch must support a range of Entire versions, say which.

**[MINOR] §2 (install-hooks target) — `.claude/settings.json` location and coexistence semantics**

Writing etch entries into repo-root `.claude/settings.json` means hooks become committed repo state (matching Entire's behavior, fine) — but the plan should state whether that file is intended to be committed/gitignored, and clarify the interaction with `entire enable --agent etch` making **etch** the tracked runtime: does that suppress Entire's own claude-code capture, and is that intended? The smoke test today uses `--agent claude-code`; the plan switches to `--agent etch`. Spell out the resulting end state so a user running both Entire-for-claude-code and Etch isn't surprised.
**Recommendation:** Add one sentence on settings-file commit semantics and one on whether enabling `--agent etch` is meant to replace or coexist with `--agent claude-code`. Confirm Claude Code runs *all* matching hook entries (the plan asserts this — keep the smoke assertion that proves coexistence).

## 4. Positive Observations

- **Root cause is real and verified, not guessed.** The four-failure causal chain is accurate against Entire's source (protocol-v1 `info` requirement, silent debug-level skip on mismatch, `install-hooks`→`{"hooks_installed":N}`, `entire agent add` not running discovery, native-vs-internal dialect). I independently confirmed each. The current `internal/info/info.go` indeed emits no `protocol_version`/`capabilities`, exactly as claimed.
- **The architecture decision is the right one and is justified against alternatives.** Rejecting `entire hooks etch` (which would make etch a double-tracked runtime driving Entire's full checkpoint engine) in favor of direct dispatch is correct — it's version-independent, avoids double-tracking, and coexists with Entire's own hooks. The reasoning is laid out with the cost of each path.
- **Scope is coherent and bounded.** Info fix → install subcommands → dual-dialect parsing → smoke extension → real-session gate → docs is a complete, logically ordered chain, and the "Out of scope" section correctly fences off the upstream `entire agent add` bug and non-claude-code runtimes.
- **Good silent-failure instincts elsewhere.** The ETCH-20 change to emit a stderr warning (exit 0) when an event carries no usable prompt directly attacks the "unknown fields silently dropped" QA finding — the same class of bug the model issue above falls into.
- **Strong risk register and hygiene.** Foreign-content preservation in `settings.json` via `map[string]json.RawMessage`, the never-commit-`bin/` note, and the `.wip` crash-recovery fallback all show the author has internalized the project's failure modes.

---
*Reviewed by claude (plan reviewer). Root-cause claims verified against Entire source and Etch working tree on 2026-06-07.*
