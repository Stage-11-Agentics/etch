# Etch — QA Report: Normal User Flow (QA Engineer B)

**Persona:** Developer who discovered Etch 10 minutes ago, has `entire` v0.6.3 installed, has the repo open, follows the README literally, does not read SPEC/BUILDPLAN.
**Version under test:** Etch v0.01.001 · Entire CLI 0.6.3 · Go 1.26.3 · git 2.50.1 · macOS (zsh)
**Date:** 2026-05-29

---

## Verdict (TL;DR)

**Could a developer realistically get value from Etch within 30 minutes of `brew install`? — Partially, but not by following the README as written.**

The *engine* is genuinely good: install is clean, capture is fast and produces a rich, well-structured `session.json`, `query`/`index`/`archive` all work, and cross-machine push/clone/fetch of session refs works exactly as a git-native tool should. **Once you know the right commands, the product delivers.**

But a naive user following the README **will not** reach that point, for two reasons:
1. **There is no documented working auto-capture path on the Entire version the README was tested against (v0.6.3).** After `entire enable` + `setup-refspec`, nothing dispatches to `entire-agent-etch`. The user does real work, looks for refs, and finds **zero**. The README's "you do nothing — Etch captures invisibly" promise is false in practice here. (ETCH-17)
2. **`setup-refspec` silently sabotages `git push`.** After running it, a plain `git push` pushes *only* etch refs — the user's actual code never leaves the machine, with no error. (ETCH-16)

A normal user would hit one of these, get a frustrated-tweet moment, and likely give up before discovering that `query`/sync actually work great. The gap is almost entirely **docs + first-run UX**, not the core implementation.

---

## Setup transcript (the naive flow, in order)

1. **Env check** — `entire` v0.6.3 at `/opt/homebrew/bin/entire`, go 1.26.3, git 2.50.1, `entire-agent-etch` not yet installed, `~/.local/bin` on PATH. ✅
2. **Install (README first command):** `make install` → **Permission denied** on `/usr/local/bin` (README warned "may need sudo"). Took the documented alternative `PREFIX=$HOME/.local make install` → installed to `~/.local/bin`. ✅
3. **Verify:** `entire-agent-etch info` → matches the README's documented JSON exactly. ✅
4. **Fresh repo:** `git init` + initial commit at `/tmp/etch-qa-userflow/my-project`. ✅
5. **Configure — `entire enable`** (with empty stdin): installed Claude Code subagent + hooks + `.entire/settings.json`, but ended with a scary `telemetry prompt … bubbletea: error opening TTY` (an **Entire** non-interactive bug, not Etch). It enabled the **claude-code** agent by default — **not etch**.
6. **`command -v entire-agent-etch`** → found on PATH. ✅
7. **`entire-agent-etch setup-refspec`** → "etch refspec configured for push and fetch" — but it configured a **phantom `origin` with no URL** (fresh repo had no remote). (ETCH-18)
8. **Reality check:** grepped `.entire/.claude/.git/hooks` for "etch" → **nothing**. The hooks call `entire hooks claude-code …`, never `entire-agent-etch`. **Etch is not wired to capture anything.** (ETCH-17)
9. **Drive work:** couldn't get Entire to auto-dispatch to etch, so simulated sessions by piping hook events directly (as `make smoke` itself does). First naive attempt used guessed field names (`prompt`, top-level `model`) → captured record had **empty prompt and null model** (silent drop). Re-ran with the real fields (`user_prompt`, `raw_data.model`, found in `scripts/smoke.sh`) → **perfect capture**. (ETCH-20)
10. **Inspect:** `git show …:session.json | jq` → rich, valid `etch.session.v1`. `query`, `index build/update/show`, `archive` → **all work** (despite README calling them "coming"). (ETCH-19)
11. **`make smoke`** (README-documented) → **PASSED** end-to-end — but note it verifies capture by *manually piping events*, confirming there's no Entire→etch auto-dispatch.
12. **Cross-machine (Step 6):** pushed refs to a bare remote via explicit refspec ✅; discovered a plain `git push` sends only etch refs, not master (ETCH-16); cloned elsewhere → 0 etch refs by default, explicit fetch refspec brought them over, `query` works in the clone. ✅ (ETCH-24 notes the clone needs setup-refspec rerun.)
13. **Cleanup:** removed `/tmp/etch-qa-userflow`.

---

## What worked

- **Install** via `PREFIX=$HOME/.local make install` — clean, fast, no surprises (the default-prefix sudo wall is documented).
- **`info` contract** — output matches the README byte-for-byte.
- **Capture fidelity** — with correct event fields, `session.json` is excellent: schema_version, agent runtime/model, prompt, timing (ms), salted hostname hash, git start/end state, `files_touched`, `tool_use` counts, c11 workspace/surface context, and a companion `agent-trace.json`.
- **`query` / `index` / `archive`** — all functional, including `query --runtime` filtering and a real on-disk index with `show` stats.
- **Cross-machine sync** — push → clone → fetch of `refs/etch/sessions/*` works exactly as expected; records parse and `query` works in the clone.
- **`make smoke`** — passes and is a good confidence check.
- **Exit codes** — unknown subcommands correctly exit 1 (my early "exit 0" readings were a `head` pipe artifact, not a bug).

## What didn't work / friction (narrative order)

1. **`make install` default prefix** → Permission denied on `/usr/local/bin`. Documented ("may need sudo"); minor.
2. **`entire enable`** non-interactively spews a `bubbletea: error opening TTY` telemetry error. **Upstream Entire issue**, not Etch — noted, no Etch ticket filed.
3. **`setup-refspec` on a fresh repo** reports success against a non-existent (URL-less) `origin`. (ETCH-18)
4. **No auto-capture wiring** — the headline gap. README Configure never registers etch; nothing dispatches to it on v0.6.3. A naive user gets zero refs. (ETCH-17)
5. **Undocumented event contract + silent field drops** — guessed field names yield empty/null data with no warning. (ETCH-20)
6. **No subcommand discovery** — bare invocation and `--help`/`help` give no command list. (ETCH-21)
7. **README undersells shipped features** — query/index/archive labeled "coming." (ETCH-19)
8. **`git push` sabotage** — after `setup-refspec`, a plain `git push` pushes only etch refs, not the user's code. The scariest finding. (ETCH-16)
9. **Clone doesn't auto-fetch etch refs** until `setup-refspec` is rerun in the clone. (ETCH-24)
10. **Upstream session_id not preserved** — output uses etch's minted ULID; the agent's own session_id isn't stored. (ETCH-23)

> **Aside (not filed):** the README's `git show refs/etch/sessions/<ULID>:session.json` works when you paste a literal ULID, but if you script it with a shell variable in **zsh** (`$ULID:session.json`) you get `bad substitution`. This is a zsh quirk, not an Etch/doc defect, so no ticket — but worth a mental note since macOS defaults to zsh.

---

## Tickets filed

| short_id | severity | title |
|----------|----------|-------|
| ETCH-16 | **high** | setup-refspec silently breaks normal `git push` (pushes only etch refs, not your code) |
| ETCH-17 | **high** | No documented working auto-capture path on tested Entire v0.6.3 — README "Usage" promise false in practice |
| ETCH-18 | medium | setup-refspec reports success against a phantom `origin` with no URL in a fresh repo |
| ETCH-19 | medium | README Status/Usage undersell shipped features (query/index/archive all work) |
| ETCH-20 | medium | Hook-event JSON contract undocumented; wrong field names silently dropped |
| ETCH-21 | medium | No subcommand discovery: bare invocation and `--help`/`help` give no command list |
| ETCH-22 | low | setup-refspec fetch refspec omits leading `+` that README's manual-equivalent shows |
| ETCH-23 | low | Agent's own session_id discarded; output session_id is etch's minted ULID only |
| ETCH-24 | low | Fresh clone doesn't auto-fetch etch refs until setup-refspec rerun in the clone — undocumented |

**Totals:** 2 high · 4 medium · 3 low · 0 critical.

---

## Bottom line

No **critical** (flow-stopping with no path forward) findings — a determined user *can* get all the way through. But the two **high** findings (no real auto-capture on the tested Entire version; `git push` silently dropping the user's code) are exactly the kind that make a first-time user bounce. The engine earns its keep; the README and first-run UX do not yet match it. Fix ETCH-16/17/19/21 and a naive user's 30-minute experience flips from "confused, gave up" to "this just works."
