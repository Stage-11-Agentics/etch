# Validation Report — Etch Backlog Completion Run (Phase 4)

Validator: agent:result-validator (fresh-eyes terminal audit)
Date: 2026-06-07
Audited state: `main @ 61d26d0` (post ETCH-40 closeout)
Protocol: `.lattice/orchestration/validation-plan.md`
Prior run's report (2026-05-27, ETCH-8 era) superseded by this one; see git history.
Method: baseline commands re-run (cached AND forced `-count=1`), live temp-repo e2e experiments executed by the validator for every privacy-critical gate (redaction, crash recovery, local_only_fields), three parallel audit agents for PR-diff mapping, cross-machine round-trip, and refspec UX. No delegator evidence taken on faith for privacy rows.

## Verdict: PASS (2 partial, 0 fail)

All 26 in-scope tickets accounted for. All baseline commands green. All six cluster gates pass on live evidence. Two partials: the undocumented session_start latency threshold, and dogfooding running a stale installed binary (operational, not code).

---

## 1. Ticket accounting — PASS

ETCH-16…ETCH-41 are all `done` or explicitly `cancelled` with rationale:

| Disposition | Tickets |
|---|---|
| done | 16, 17, 18, 19, 20, 21, 23, 24, 26, 27, 28, 29, 34, 35, 37, 38, 39, 40, 41 |
| cancelled — subsumed by ETCH-38 (note attached) | 22 |
| cancelled — superseded by ETCH-40 (supersedes links present) | 25, 30, 31, 32, 33, 36 |
| cancelled — historical (pre-run) | 14 |
| cancelled — root-caused (worktree↔root diff bug, documented in lessons-learned) | 42 |
| backlog — NEW follow-up, correctly out of run scope | 43 |

ETCH-40 carries the full closeout audit (verdict PASS, finding-by-finding table, adversarial test gate confirmed) and 12 per-finding comments. Audit trail is complete.

## 2. Baseline commands — PASS

| Command | Result |
|---|---|
| `go test ./...` | exit 0 (also re-run with `-count=1`, fully uncached: exit 0, all 15 packages) |
| `make build` | exit 0 |
| `make smoke` | exit 0 (validates real `entire` + installed-hook native payloads) |
| `make test-density` | exit 0 (20-concurrent, crash-recovery, ref-uniqueness) |
| targeted `Redact\|Recovery\|Refspec\|Root\|Token\|Secret\|Crash` filter | exit 0 |

## 3. Cluster gates

### 3.1 Secret redaction — PASS (validator's own live e2e)

Temp repo, real hook calls, secrets verified against **committed blobs** (`git show <ref>:session.json` and `:agent-trace.json`), not in-memory structs:

- `sk-proj-…`, `sk-svcacct-…` → `[REDACTED:openai-api-key]` ✓
- JWT → `[REDACTED:jwt]` ✓
- Full PEM block (body included) → `[REDACTED:private-key]`, zero key-body bytes in blob ✓
- `password=` / `client_secret=` / `token=` / `passwd=` → `[REDACTED:generic-secret]` ✓
- Bare 40-char AWS secret keys (both random mixed-class and the canonical `wJalrXUtnFEMI/…EXAMPLEKEY` doc key) → `[REDACTED:aws-secret-key]` ✓
- `sk-ant-EXAMPLE` placeholder **preserved** (no over-redaction) ✓
- Secret embedded in a **tool-input file path** → redacted in `files_touched[].path` of the committed blob (finding 5's commit-boundary pass works beyond Prompt.Text) ✓
- `agent-trace.json` blob equally clean ✓

Note (not a failure): the AWS validator is strictly 40-char — a 41+-char near-key string escapes. Deliberate false-positive tradeoff; acceptable.

### 3.2 Refspec / remote UX — PASS (audit agent, empirical)

- No-remote repo: `setup-refspec` exits 1 with actionable message; **no phantom remote created** ✓
- Non-origin remote (`upstream`): auto-detected and configured; `--remote` flag also works; origin untouched ✓
- Normal pushes unaffected: both `git push origin main` and bare `git push` verified to advance the branch on the remote after setup-refspec (augment-not-replace, ETCH-16) ✓
- Fetch refspec carries leading `+` (ETCH-38/22) ✓
- README fresh-clone/multi-remote/stale-config-upgrade claims each tested empirically and match behavior (ETCH-24/18) ✓

### 3.3 Capture / recovery correctness — PASS (validator's own live e2e)

Drove real hooks under a fake agent process (`comm` = "claude") that exited without `session_end` — a true dead-PID crash, not a fabricated wip:

- Wip header records agent PID + ps start time at session_start ✓
- Recovery **vetoed while wip is fresh** (stat-first 5-min activity grace — by design) and **vetoed for a live agent PID even past the grace window**; the live session's wip survived untouched ✓
- After aging past grace, next `session_start` recovered the dead session promptly (`dead_pid` path — the dead code of old ETCH-30 now demonstrably live) ✓
- Recovered record truthful: `status: incomplete`, `exit_reason: crash`, prompt preserved, `tool_use.total_calls: 1` (exactly-once counting — ETCH-33's double-count gone), `files_touched` via tool-path fallback, `git_end: null` per OUTPUT_SPEC crash policy, agent-trace emitted ✓
- Duplicate `session_start` for an active session: explicit log "reusing session … (no new ULID minted)", no second wip ✓ (finding 4)
- Repo-root anchoring: all temp-repo state landed in `<git-root>/.etch` regardless of invocation shape; non-git behavior covered by reporoot tests (PR #18 diff audit) ✓

### 3.4 Token / session identity — PASS

- `agent_session_id` captured from the hook payload's session id on current binary (verified live: `crash-sess`, `redact-e2e-1`) ✓ (ETCH-23)
- Etch ULID remains the canonical ref/session id ✓
- `tokens: null` in all records; OUTPUT_SPEC amended to null-in-v1/reserved across canonical + all scenario examples; dead aggregation paths deleted (PR #21 diff audit) ✓ (f.10)

### 3.5 CLI / docs discoverability — PASS

- Bare invocation, `help`, `--help`, `-h` all print the full grouped subcommand listing with per-command usage ✓ (ETCH-21)
- README documents shipped `query`/`index`/`archive` with examples; no "missing feature" understatement ✓ (ETCH-19)
- `docs/HOOK_CONTRACT.md` documents working field names (`user_prompt`, `raw_data.model`, `tool_name`/`tool_use_id`/`tool_input`, shared `session_id`) and the model-backfill-from-transcript path ✓ (ETCH-20)
- Hostname claim now accurate: README documents `SHA-256(salt + hostname)` with per-repo auto-generated salt — matches `config.EnsureHostnameSalt` + `redact.HashHostname` ✓ (ETCH-37)

### 3.6 Privacy contract (local_only_fields) — PASS (validator's own live e2e)

Settings `{"local_only_fields": ["prompt.text", "operator"]}`, live session:

- Pushable `refs/etch/sessions/<ULID>` record: `prompt.text` → `[LOCAL_ONLY:prompt.text]`, `operator` → null, `local_only_stripped` manifest lists both ✓
- `refs/etch/local/<ULID>` keeps full fidelity ✓
- After `setup-refspec` + bare `git push`: session ref arrives on remote, **zero `refs/etch/local/*` on remote** — safe by construction ✓ (ETCH-41 / f.6)

No false privacy guarantee remains; README documents the implemented behavior.

## 4. Cross-machine read path + archive integrity — PASS (audit agent)

Repo A → bare B → clone C round-trip: session refs absent on plain clone (expected), present after `setup-refspec` + `git fetch`; `session.json`/`agent-trace.json` parse on the second machine; `query`/`index build`/`index show` work in the clone; archive moves sessions to `refs/etch/archive/2026-Q2`, `restore-archive` brings one back **byte-identical** to the original blob (SPEC AC #5, #12) ✓

## 5. Phase 4 checklist

- **PR-diff-to-ticket mapping**: all eight PRs (#17–#24) audited hunk-level against claimed scope — 8/8 CONFIRMED, tests ship in every PR, no unexplained scope creep ✓
- **Naming**: no stale Cairn in README/OUTPUT_SPEC/SPEC/code; the only `CAIRN_*` occurrences are deliberate ETCH-15 cutover-guard tests asserting legacy vars are ignored ✓
- **`git status --short --branch`**: `M bin/entire-agent-etch` (intentional), `M .claude/scheduled_tasks.lock` (intentional, though plan said "deletion" — it is a modification), `?? .claude/settings.json` and `?? .entire/` are **dogfooding-enablement artifacts** not on the intentional-dirt list → operator should decide commit-vs-ignore (see recommendations) — minor
- **Dogfooding**: 5 refs under `refs/etch/sessions/`, all parse as valid `etch.session.v1` with consistent machine identity and `tokens: null` ✓ — but see Partial P2

## 6. Partials

### P1 — session_start latency threshold undocumented (minor)

Fresh benchmark, 100 temp-repo invocations with wip count growing 0→99: **median 170.9 ms, p90 175.8, p99 178.3, max 179.0**. The flat curve proves the stat-first scan removed the O(N)-per-start growth (old: 186.9 median / 366.8 p99 with growth). p99 meets the <200 ms target; the 50 ms median target is not met and **no new accepted threshold was documented** (the plan offered "or document new accepted thresholds"). The floor is per-invocation process + git-plumbing cost, not scan cost; per-prompt hooks remain ~6 ms, so the once-per-session 170 ms is operationally benign — but the documentation step is owed.

### P2 — dogfooding runs a stale installed binary (operational)

`~/.local/bin/entire-agent-etch` was installed at 12:50, **before** PRs #21–#24 merged. Proof: all 5 live records' `hostname_hash` equals **unsalted** `SHA-256("Hyperion")` exactly, `agent_session_id` is absent, and no `.etch/settings.json` salt file exists in the repo. The live records are valid v1 but demonstrate pre-#21 behavior; none of the salt, agent_session_id, or lifecycle fixes are exercised by current live capture. Not a code defect — every behavior verified against the **current** binary in temp repos — but the flagship "Etch records the agents building it" loop is running a stale build.

## 7. Drift from BUILDPLAN architecture

All deliberate and documented; no silent drift found:

- `refs/etch/local/<ULID>` namespace (ETCH-41 dual-ref projection) — new beyond the original single-namespace design; documented in OUTPUT_SPEC/README; overwrite contract explicitly scoped to local
- `capture.RepoContext` (StateRoot vs WorkDir split) replaced `findRepoRoot()` — threading verified through all six hooks
- Shared wip→session reducer (`ReduceEvents`/`FinishSession`) — recovery and finalize now one code path (f.9's deeper fix)
- Tokens dropped from v1 (operator decision) — schema field reserved, spec amended

## 8. Unresolved risks

1. **Stale dogfooding binary** (P2) — live capture silently lags merged behavior until reinstall; there is no version/staleness check between repo and installed binary.
2. **Salt distribution**: per-repo salt is generated lazily and must be committed to correlate across clones; until `.etch/settings.json` is committed (it currently doesn't exist in the live repo due to P2), each machine salts independently.
3. **AWS pattern strictness**: 40-char-exact validator misses keys embedded in longer tokens — accepted FP tradeoff, documenting here for the record.
4. **Cosmetics**: `archive`/`restore-archive` don't accept `--repo` (inconsistent with `query`/`index`); `session_end` after `stop` logs a benign "no session mapping" warning.

## 9. Recommendations

1. **Reinstall the dogfooding binary now**: `make install PREFIX=$HOME/.local`, then after the next session confirm a record with salted hash + `agent_session_id`, and **commit `.etch/settings.json`** once generated (the salt is designed to be shared).
2. Document the accepted session_start latency threshold (e.g., "median ≤ 250 ms, p99 ≤ 400 ms; per-event hooks ≤ 15 ms" in BUILDPLAN or OUTPUT_SPEC) or file a low-priority optimization ticket.
3. Decide `.claude/settings.json` + `.entire/` tracking (commit as repo config vs .gitignore) — they are now load-bearing for auto-capture.
4. Consider a `--repo` flag for `archive`/`restore-archive` for CLI consistency (could fold into ETCH-43-adjacent cleanup).
5. ETCH-43 (local-fidelity query fallback) is correctly parked in backlog; no action.
