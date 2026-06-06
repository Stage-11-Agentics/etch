# Etch — Adversarial QA Report (Tier 1 + Tier 1.5)

**Auditor:** agent:qa-adversarial (QA Engineer A — hostile auditor)
**Date:** 2026-05-29
**Target:** Etch v0.01.001 @ `main` (after ETCH-15 Cairn→Etch rename)
**Build:** `go build` clean; `go test ./...` all green; `make smoke` PASSED.

> Mode: hostile. Goal was to break the implementation, not validate it. Every
> finding below was reproduced empirically by driving the real binary, not by
> reading code alone. Harness scripts live under `/tmp/etch-qa/`, `/tmp/etch-crash/`,
> `/tmp/etch-lat/`, `/tmp/etch-subdir/`, `/tmp/etch-custom/`, `/tmp/etch-t15/`.

## Verdict

**NOT ready for unconditional use on Stage 11 platform projects (Lattice, c11, Etch
itself) until the secret-redaction criticals (ETCH-25..28) and the silent-config /
silent-data-loss issues (ETCH-34, ETCH-35) are fixed.**

The capture core is genuinely solid — concurrency, immutability, push/fetch,
schema conformance, and Agent Trace all hold up under hostile testing. But the
project's headline safety promise (SPEC #8, "prompts are scanned for secrets
before commit") leaks **4 of the most important credential classes**, and two
silent-failure modes (subdir config ignore, non-repo data loss) mean operators
can believe protection is on when it is off. These are exactly the failures that
matter when you point this at real repos with real keys in agent prompts.

Notably, **every shipped unit test passes** — including the ones covering the
broken behavior. The test suite encodes the bugs (e.g. the private-key test only
asserts the BEGIN marker is redacted; the crash-recovery test injects a PID the
production binary never writes). Green CI here is not evidence of correctness.

## Per-item results

| # | Tier 1 audit item | Result | Evidence |
|---|---|---|---|
| 1 | Schema conformance + edge cases | **PARTIAL** | Empty prompt, missing model, zero tools, 1500 tools, no-prompt-event, stop-vs-end all produce schema-valid `etch.session.v1` records with required fields + correct types. FAILS: no-git-repo silently drops the session (ETCH-35); `tokens` always null (ETCH-32); `.etch`/config resolved from CWD not git root (ETCH-34). |
| 2 | Live integration matrix | **PARTIAL** | `make smoke` + `entire enable` succeed; binary discoverable via `entire-agent-<name>`. Full Entire→Etch *dispatch* unverifiable: Entire 0.6.3's agent roster excludes `etch` (`entire agent add etch` → "Unknown agent" — README documents this accurately). Runtime inference (`claude-code` via `CLAUDECODE`) and model capture work. Tokens never captured (ETCH-32). codex/gemini/opencode on PATH but not dispatchable through Entire 0.6.3 either. |
| 3 | Cross-machine push/fetch | **PASS** | `setup-refspec` configures push+fetch; pushed 3 refs to a bare remote, cloned, fetched, `git show <ref>:session.json` content **identical** across origin/clone. Minor: fetch refspec omits `+` and hard-codes `origin` (ETCH-38, low). |
| 4 | Real-concurrency density | **PASS** | 20 concurrent shell-driven full sessions against a shared `.etch/`: 20 distinct refs, 20 unique ULIDs, 0 orphaned `.wip`, 0 leftover mapping/temp files, all schema-valid. No collisions. |
| 5 | Hook latency benchmark | **PARTIAL** | 100 runs/hook (m4 Max). `user_prompt_submit`/`pre`/`post`/`stop`/`session_end` all ~6 ms median (well under 50 ms). **`session_start` 187 ms median / 367 ms p99 / 398 ms max** — 3.7× over the 50 ms target; p99 breaches the 200 ms user-lag bar (ETCH-36). O(N) recovery scan per start → O(N²) in a burst. |
| 6 | Secret-scanning red team | **FAIL (critical)** | 4 credential classes leak verbatim: OpenAI `sk-proj-` (ETCH-25), bare AWS secret (ETCH-26), JWT (ETCH-27), private-key **body** (ETCH-28, only the BEGIN line is redacted). Over-redaction: `sk-ant-EXAMPLE` clobbered (ETCH-29). Passwords/`DB_PASS=` uncovered (ETCH-39). WORKS: anthropic `sk-ant-api03-`, classic `sk-...`, AWS access key, stripe live/test, bearer, generic api_key=, custom patterns from `.etch/settings.json`, multiline `.env` paste, `sk-DOCUMENTATION-NOT-A-KEY` correctly preserved. |
| 7 | Documentation audit | **PARTIAL** | `make install`, `entire-agent-etch info`, `setup-refspec`, settings.json schema, `entire enable`, the `entire agent add etch`→"Unknown agent" note all verified accurate. FAILS: README claims `local_only_fields` keeps fields off the remote — **unimplemented** (ETCH-31, false privacy claim); README says hostname is a **"salted hash"** — it's unsalted SHA-256 (ETCH-37). |
| 1.5a | raw hostname opt-in (PR#9 fix) | **PASS** | `raw_machine_identity:true` in `.etch/settings.json` → `machine.hostname_raw:"Hyperion"`, hash still present. Fix holds. |
| 1.5b | pane_lineage (PR#9 fix) | **PASS** | `ETCH_PANE_LINEAGE='["A","B"]'` + `C11_SURFACE_ID` → `c11.pane_lineage:["A","B"]`. Fix holds. |

## Tickets filed (15)

| short_id | severity | title |
|---|---|---|
| ETCH-25 | critical | Secret scan: `sk-proj-` OpenAI keys not redacted |
| ETCH-26 | critical | Secret scan: bare AWS secret access key not redacted |
| ETCH-27 | critical | Secret scan: JWTs never redacted (no pattern) |
| ETCH-28 | critical | Secret scan: private key **body** leaks; only BEGIN header redacted |
| ETCH-29 | high | Over-redaction: `sk-ant-EXAMPLE` doc placeholder redacted |
| ETCH-30 | high | Crash recovery `dead_pid` path is dead code; crashes not recovered on next invocation (SPEC AC#6) |
| ETCH-31 | high | `local_only_fields` documented but completely unimplemented (false privacy guarantee) |
| ETCH-32 | medium | `tokens` block never populated in any captured session |
| ETCH-33 | medium | Recovered crash sessions double-count tool calls (pre+post vs pre only) |
| ETCH-34 | medium | `.etch`/`settings.json` resolved from CWD not git root; subdir sessions silently ignore config |
| ETCH-35 | high | no-git-repo: `session_end` silently drops data, returns ok, orphans `.wip` |
| ETCH-36 | medium | `session_start` latency ~187 ms median / 367 ms p99 (target 50 ms); O(N) recovery scan per start |
| ETCH-37 | medium | README claims "salted hash" but implementation is unsalted SHA-256 |
| ETCH-38 | low | `setup-refspec` fetch refspec omits leading `+`; hard-codes `remote.origin` |
| ETCH-39 | low | Secret scan misses common credential keys (password/passwd/bare token=) |

Counts: **4 critical, 4 high, 4 medium, 2 low** (15 total; ETCH-29 filed high per the
audit's over-redaction calibration, ETCH-30/31/35 high).

## What held up under hostile testing (credit where due)

- Per-session ref model: zero-contention, immutable, 20-way concurrent with no
  collisions or dropped records.
- Schema conformance across every fabricated edge case (empty/oversized/missing fields).
- Push/fetch portability with byte-identical content across a clone.
- Agent Trace emission (`version 1.0`, well-formed) alongside every session.
- Custom redaction patterns + the *covered* secret classes redact correctly,
  including inside multiline `.env` pastes.
- Both PR#9 fixes (raw hostname, pane lineage) verified holding.

## Highest-leverage fixes before real use

1. **ETCH-25..28** — rewrite the secret patterns. The current set passes its own
   tests while leaking the four credential formats most likely to appear in 2025+
   agent prompts. This is the project's core promise.
2. **ETCH-34** — resolve repo root via `git rev-parse --show-toplevel`. Until then,
   any agent not started at the exact repo root silently runs with default config
   (redaction patterns ignored, recovery timeout ignored).
3. **ETCH-35** — fail loudly outside a git repo instead of returning `{"ok":true}`.
4. **ETCH-30** — write `os.Getpid()` at session_start so prompt crash recovery
   actually works (today it waits 4h), and stop trusting a test that fakes the PID.
