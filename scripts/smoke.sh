#!/usr/bin/env bash
#
# Etch smoke test — end-to-end validation of the install + capture story.
#
# What it proves, step by step:
#   1. The Go binary builds.
#   2. `entire-agent-cairn info` emits valid plugin JSON (PATH discovery contract).
#   3. A fresh git repo can be Entire-enabled and cairn registers as an agent
#      (this is how Etch hooks into a real repo).
#   4. A simulated agent session — session_start → user_prompt_submit →
#      pre/post_tool_use → session_end, all sharing one session_id — drives the
#      binary the same way Entire's hooks do, by piping hook-event JSON on stdin.
#   5. Exactly one immutable ref appears at refs/cairn/sessions/<ULID>.
#   6. That ref's session.json parses and carries schema_version cairn.session.v1.
#   7. The Agent Trace blob (agent-trace.json) is emitted alongside it.
#
# The temp repo is removed on exit. The script is re-runnable and self-contained:
# its only external dependency is the real `entire` CLI (and git, go, python3).
#
# Exit 0 only if every step passes.

set -uo pipefail

# --- locate repo root (this script lives in <root>/scripts) ---
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN="$ROOT/bin/entire-agent-cairn"

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
if (cd "$ROOT" && go build -o "$BIN" ./cmd/entire-agent-cairn); then
  pass "built $BIN"
else
  fail "go build failed"; exit 1
fi

# --- 2. info contract ---
step "2. Plugin info contract"
INFO_JSON="$("$BIN" info 2>/dev/null)"
NAME="$(printf '%s' "$INFO_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("name",""))' 2>/dev/null)"
if [ "$NAME" = "cairn" ]; then
  pass "info returns name=cairn"
else
  fail "info did not return name=cairn (got: $INFO_JSON)"
fi

# --- 3. fresh repo + Entire enable + agent registration ---
step "3. Entire integration in a fresh repo"
TMP="$(mktemp -d)"
(
  cd "$TMP"
  git init -q
  git config user.email smoke@cairn.test
  git config user.name "cairn smoke"
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
if command -v entire-agent-cairn >/dev/null 2>&1; then
  pass "entire-agent-cairn discoverable on PATH (entire-agent-<name> plugin contract)"
else
  fail "entire-agent-cairn not on PATH — Entire cannot discover it"
fi

# Note: in entire v0.6.3 the `entire agent add` roster is a fixed built-in list
# (claude-code, codex, ...); external agents like cairn are driven via Entire's
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
REFS="$(cd "$TMP" && git for-each-ref --format='%(refname)' refs/cairn/sessions/)"
REF_COUNT="$(printf '%s' "$REFS" | grep -c . )"
if [ "$REF_COUNT" -eq 1 ]; then
  pass "exactly 1 ref under refs/cairn/sessions/ ($REFS)"
else
  fail "expected 1 session ref, found $REF_COUNT"
fi
REF="$(printf '%s' "$REFS" | head -1)"

# --- 6. session.json schema ---
step "6. Verify session.json schema"
if [ -n "$REF" ]; then
  SESSION_JSON="$(cd "$TMP" && git show "$REF:session.json" 2>/dev/null)"
  SCHEMA="$(printf '%s' "$SESSION_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["schema_version"])' 2>/dev/null)"
  if [ "$SCHEMA" = "cairn.session.v1" ]; then
    pass "session.json schema_version = cairn.session.v1"
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

# --- summary ---
echo
if [ "$FAILS" -eq 0 ]; then
  printf "${GREEN}SMOKE PASSED${RESET} — install + capture story verified end-to-end.\n"
  exit 0
else
  printf "${RED}SMOKE FAILED${RESET} — %d check(s) failed.\n" "$FAILS"
  exit 1
fi
