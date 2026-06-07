#!/usr/bin/env bash
#
# Etch smoke test — end-to-end validation of the install + capture story.
#
# What it proves, step by step:
#   1. The Go binary builds.
#   2. `entire-agent-etch info` emits valid plugin JSON (PATH discovery contract).
#   3. A fresh git repo can be Entire-enabled and etch registers as an agent
#      (this is how Etch hooks into a real repo).
#   4. A simulated agent session — session_start → user_prompt_submit →
#      pre/post_tool_use → session_end, all sharing one session_id — drives the
#      binary the same way Entire's hooks do, by piping hook-event JSON on stdin.
#   5. Exactly one immutable ref appears at refs/etch/sessions/<ULID>.
#   6. That ref's session.json parses and carries schema_version etch.session.v1.
#   7. The Agent Trace blob (agent-trace.json) is emitted alongside it.
#   8. `entire enable --agent etch` registers etch via Entire's external-agent
#      protocol and drives `install-hooks`, wiring etch dispatch entries into
#      .claude/settings.json — coexisting with Entire's own claude-code hooks.
#   9. The INSTALLED hook commands, driven with native Claude Code payloads
#      (the shapes a real session delivers), produce a second session ref.
#  10. That record carries the prompt, native exit reason, and the model
#      backfilled from the transcript JSONL.
#
# The temp repo is removed on exit. The script is re-runnable and self-contained:
# its only external dependency is the real `entire` CLI (and git, go, python3).
#
# Exit 0 only if every step passes.

set -uo pipefail

# --- locate repo root (this script lives in <root>/scripts) ---
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN="$ROOT/bin/entire-agent-etch"

# --- colored status helpers ---
if [ -t 1 ]; then
  GREEN=$'\033[32m'; RED=$'\033[31m'; DIM=$'\033[2m'; RESET=$'\033[0m'
else
  GREEN=""; RED=""; DIM=""; RESET=""
fi
FAILS=0
pass() { printf "  ${GREEN}✓${RESET} %s\n" "$1"; }
fail() { printf "  ${RED}✗${RESET} %s\n" "$1"; FAILS=$((FAILS + 1)); }
step() { printf "\n${DIM}== %s ==${RESET}\n" "$1"; }

TMP=""
cleanup() { [ -n "$TMP" ] && rm -rf "$TMP"; }
trap cleanup EXIT

# --- preflight ---
step "Preflight"
for tool in git go python3 entire; do
  if command -v "$tool" >/dev/null 2>&1; then
    pass "$tool found ($(command -v "$tool"))"
  else
    fail "$tool not found on PATH"
  fi
done
[ "$FAILS" -eq 0 ] || { echo; echo "${RED}Preflight failed — missing dependencies.${RESET}"; exit 1; }

# --- 1. build ---
step "1. Build binary"
if (cd "$ROOT" && go build -o "$BIN" ./cmd/entire-agent-etch); then
  pass "built $BIN"
else
  fail "go build failed"; exit 1
fi

# --- 2. info contract ---
step "2. Plugin info contract"
INFO_JSON="$("$BIN" info 2>/dev/null)"
NAME="$(printf '%s' "$INFO_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("name",""))' 2>/dev/null)"
if [ "$NAME" = "etch" ]; then
  pass "info returns name=etch"
else
  fail "info did not return name=etch (got: $INFO_JSON)"
fi

# --- 3. fresh repo + Entire enable + agent registration ---
step "3. Entire integration in a fresh repo"
TMP="$(mktemp -d)"
(
  cd "$TMP"
  git init -q
  git config user.email smoke@etch.test
  git config user.name "etch smoke"
  printf "init\n" > README.md
  git add README.md
  git commit -q -m "initial commit"
)
# Put our binary first on PATH so Entire discovers it as an external agent.
export PATH="$ROOT/bin:$PATH"

ENABLE_OUT="$(cd "$TMP" && entire enable --agent claude-code --no-github </dev/null 2>&1)"
if printf '%s' "$ENABLE_OUT" | grep -qiE "ready|enabled"; then
  pass "entire enable succeeded"
else
  fail "entire enable did not report success"
  printf '%s\n' "$ENABLE_OUT" | sed 's/^/      /'
fi

# Etch's binary is an Entire external-agent plugin, discovered by its
# entire-agent-<name> filename on PATH. Confirm that discovery contract holds.
if command -v entire-agent-etch >/dev/null 2>&1; then
  pass "entire-agent-etch discoverable on PATH (entire-agent-<name> plugin contract)"
else
  fail "entire-agent-etch not on PATH — Entire cannot discover it"
fi

# Note: in entire v0.6.3 the `entire agent add` roster is a fixed built-in list
# (claude-code, codex, ...); external agents like etch are driven via Entire's
# hook dispatch, which step 4 exercises directly using the same stdin contract.

# --- 4. simulate an agent session by piping hook events ---
step "4. Simulate a captured session"
SID="smoke-session-1"
emit() {
  # $1 = hook subcommand, $2 = stdin JSON
  if printf '%s' "$2" | (cd "$TMP" && "$BIN" "$1") >/dev/null 2>"$TMP/.err.$1"; then
    pass "$1"
  else
    fail "$1 (stderr: $(cat "$TMP/.err.$1"))"
  fi
}
emit session_start      "{\"session_id\":\"$SID\",\"raw_data\":{\"model\":\"claude-opus-4-7\"}}"
emit user_prompt_submit "{\"session_id\":\"$SID\",\"user_prompt\":\"run the smoke test\"}"
emit pre_tool_use       "{\"session_id\":\"$SID\",\"tool_name\":\"Read\",\"tool_use_id\":\"tu-1\",\"tool_input\":{\"file_path\":\"/tmp/example.go\"}}"
emit post_tool_use      "{\"session_id\":\"$SID\",\"tool_name\":\"Read\",\"tool_use_id\":\"tu-1\",\"tool_input\":{\"file_path\":\"/tmp/example.go\"}}"
emit session_end        "{\"session_id\":\"$SID\"}"

# --- 5. exactly one session ref ---
step "5. Verify session ref"
REFS="$(cd "$TMP" && git for-each-ref --format='%(refname)' refs/etch/sessions/)"
REF_COUNT="$(printf '%s' "$REFS" | grep -c . )"
if [ "$REF_COUNT" -eq 1 ]; then
  pass "exactly 1 ref under refs/etch/sessions/ ($REFS)"
else
  fail "expected 1 session ref, found $REF_COUNT"
fi
REF="$(printf '%s' "$REFS" | head -1)"

# --- 6. session.json schema ---
step "6. Verify session.json schema"
if [ -n "$REF" ]; then
  SESSION_JSON="$(cd "$TMP" && git show "$REF:session.json" 2>/dev/null)"
  SCHEMA="$(printf '%s' "$SESSION_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["schema_version"])' 2>/dev/null)"
  if [ "$SCHEMA" = "etch.session.v1" ]; then
    pass "session.json schema_version = etch.session.v1"
  else
    fail "unexpected schema_version: '$SCHEMA'"
  fi
  STATUS="$(printf '%s' "$SESSION_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("status",""))' 2>/dev/null)"
  if [ "$STATUS" = "complete" ]; then
    pass "session status = complete"
  else
    fail "unexpected status: '$STATUS'"
  fi
else
  fail "no ref to inspect"
fi

# --- 7. Agent Trace blob ---
step "7. Verify Agent Trace emission"
if [ -n "$REF" ]; then
  TRACE_VER="$(cd "$TMP" && git show "$REF:agent-trace.json" 2>/dev/null | python3 -c 'import json,sys; print(json.load(sys.stdin).get("version",""))' 2>/dev/null)"
  if [ -n "$TRACE_VER" ]; then
    pass "agent-trace.json present (version $TRACE_VER)"
  else
    fail "agent-trace.json missing or unparseable"
  fi
fi

# --- 8. entire enable --agent etch (the auto-capture install path) ---
step "8. Auto-capture install via entire enable --agent etch"
ENABLE_ETCH_OUT="$(cd "$TMP" && entire enable --agent etch --no-github </dev/null 2>&1)"
if printf '%s' "$ENABLE_ETCH_OUT" | grep -qiE "ready|enabled"; then
  pass "entire enable --agent etch succeeded"
else
  fail "entire enable --agent etch did not report success"
  printf '%s\n' "$ENABLE_ETCH_OUT" | sed 's/^/      /'
fi
if printf '%s' "$ENABLE_ETCH_OUT" | grep -q "Installed 5 hooks"; then
  pass "Entire drove install-hooks (5 hooks installed)"
else
  fail "Entire did not report installing etch hooks"
fi
if grep -q "entire-agent-etch session_start" "$TMP/.claude/settings.json" 2>/dev/null; then
  pass "etch dispatch entries present in .claude/settings.json"
else
  fail "etch dispatch entries missing from .claude/settings.json"
fi
# Coexistence: Entire's own claude-code hooks (installed in step 3) survive.
if grep -q "entire hooks claude-code" "$TMP/.claude/settings.json" 2>/dev/null; then
  pass "coexists with Entire's claude-code hooks"
else
  fail "Entire's claude-code hooks were clobbered"
fi
if grep -q '"external_agents": *true' "$TMP/.entire/settings.json" 2>/dev/null; then
  pass "external_agents auto-enabled in .entire/settings.json"
else
  fail "external_agents not set in .entire/settings.json"
fi
INSTALLED_JSON="$(cd "$TMP" && entire-agent-etch are-hooks-installed 2>/dev/null)"
if [ "$INSTALLED_JSON" = '{"installed":true}' ]; then
  pass "are-hooks-installed reports installed"
else
  fail "are-hooks-installed: got '$INSTALLED_JSON'"
fi

# --- 9. drive the INSTALLED hooks with native Claude Code payloads ---
step "9. Native Claude Code session through installed hooks"
SID2="smoke-native-1"
TRANSCRIPT="$TMP/transcript.jsonl"
printf '%s\n%s\n' \
  '{"type":"user","message":{"role":"user","content":"hi"}}' \
  '{"type":"assistant","message":{"role":"assistant","model":"claude-opus-4-8","content":[]}}' \
  > "$TRANSCRIPT"

# Extract each installed command from settings.json and run it with the
# native payload a real Claude Code session delivers.
run_installed_hook() {
  # $1 = etch subcommand, $2 = native JSON payload
  local cmd
  cmd="$(python3 -c "
import json, sys
settings = json.load(open('$TMP/.claude/settings.json'))
for matchers in settings.get('hooks', {}).values():
    for m in matchers:
        for h in m.get('hooks', []):
            if 'entire-agent-etch $1' in h.get('command', ''):
                print(h['command']); sys.exit(0)
sys.exit(1)
")" || { fail "$1: no installed command found"; return; }
  if printf '%s' "$2" | (cd "$TMP" && eval "$cmd") >/dev/null 2>"$TMP/.err.native.$1"; then
    pass "installed hook: $1"
  else
    fail "installed hook: $1 (stderr: $(cat "$TMP/.err.native.$1"))"
  fi
}

run_installed_hook session_start      "{\"session_id\":\"$SID2\",\"transcript_path\":\"$TRANSCRIPT\",\"cwd\":\"$TMP\",\"hook_event_name\":\"SessionStart\",\"source\":\"startup\"}"
run_installed_hook user_prompt_submit "{\"session_id\":\"$SID2\",\"transcript_path\":\"$TRANSCRIPT\",\"hook_event_name\":\"UserPromptSubmit\",\"prompt\":\"run the native smoke\"}"
run_installed_hook pre_tool_use       "{\"session_id\":\"$SID2\",\"transcript_path\":\"$TRANSCRIPT\",\"hook_event_name\":\"PreToolUse\",\"tool_name\":\"Read\",\"tool_use_id\":\"toolu_1\",\"tool_input\":{\"file_path\":\"$TMP/f.txt\"}}"
run_installed_hook post_tool_use      "{\"session_id\":\"$SID2\",\"transcript_path\":\"$TRANSCRIPT\",\"hook_event_name\":\"PostToolUse\",\"tool_name\":\"Read\",\"tool_use_id\":\"toolu_1\",\"tool_input\":{\"file_path\":\"$TMP/f.txt\"}}"
run_installed_hook session_end        "{\"session_id\":\"$SID2\",\"transcript_path\":\"$TRANSCRIPT\",\"hook_event_name\":\"SessionEnd\",\"reason\":\"other\"}"

# --- 10. verify the native-capture record ---
step "10. Verify native-capture record"
REFS2="$(cd "$TMP" && git for-each-ref --format='%(refname)' refs/etch/sessions/)"
REF2_COUNT="$(printf '%s\n' "$REFS2" | grep -c . )"
if [ "$REF2_COUNT" -eq 2 ]; then
  pass "2 refs under refs/etch/sessions/ (manual + native)"
else
  fail "expected 2 session refs after native session, found $REF2_COUNT"
fi
NATIVE_REF="$(printf '%s\n' "$REFS2" | grep -v "^$REF\$" | head -1)"
if [ -n "$NATIVE_REF" ]; then
  NATIVE_JSON="$(cd "$TMP" && git show "$NATIVE_REF:session.json" 2>/dev/null)"
  N_MODEL="$(printf '%s' "$NATIVE_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["agent"]["model"] or "")' 2>/dev/null)"
  if [ "$N_MODEL" = "claude-opus-4-8" ]; then
    pass "model backfilled from transcript ($N_MODEL)"
  else
    fail "model not backfilled (got '$N_MODEL')"
  fi
  N_PROMPT="$(printf '%s' "$NATIVE_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["prompt"]["text"])' 2>/dev/null)"
  if [ "$N_PROMPT" = "run the native smoke" ]; then
    pass "native prompt captured"
  else
    fail "native prompt missing (got '$N_PROMPT')"
  fi
  N_REASON="$(printf '%s' "$NATIVE_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["exit_reason"])' 2>/dev/null)"
  if [ "$N_REASON" = "other" ]; then
    pass "native exit reason captured"
  else
    fail "native exit reason missing (got '$N_REASON')"
  fi
else
  fail "no native ref to inspect"
fi

# --- summary ---
echo
if [ "$FAILS" -eq 0 ]; then
  printf "${GREEN}SMOKE PASSED${RESET} — install + capture story verified end-to-end.\n"
  exit 0
else
  printf "${RED}SMOKE FAILED${RESET} — %d check(s) failed.\n" "$FAILS"
  exit 1
fi
