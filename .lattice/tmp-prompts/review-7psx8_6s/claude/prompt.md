# Plan Review: ETCH-40

You are reviewing a plan before implementation begins. Your job is to evaluate
whether this plan is complete, feasible, and aligned with the task description.
Catching plan-level issues now is far cheaper than discovering them during
code review.

## Context

### Task
**ID:** ETCH-40

### Task Description
Umbrella remediation ticket for the 2026-06-04 deep code review. THE SPEC IS THE REVIEW FILE: reviews/2026-06-04-deep-code-review.md — read it first; it has file:line, verified failure scenarios, and refuted non-bugs (do not re-fix those).

Scope (10 confirmed findings + 4 below-cut):
1. Recovery falsely orphans LIVE idle sessions — capture PID at session_start, wire the liveness check (recovery.go:129; absorbs ETCH-30)
2. findRepoRoot()=os.Getwd() → silent session loss — resolve git common-dir root at the hook boundary (hooks/common.go:39; supersedes-in-part ETCH-34)
3. Session refs silently overwritable — create-only update-ref guard (refs/writer.go:47)
4. Duplicate session_start splits sessions — reuse existing mapping (session_start.go:38)
5. Redaction only covers Prompt.Text — full-record redaction pass at commit boundary (commit.go:24)
6. local_only_fields unimplemented (config.go:13; absorbs ETCH-31)
7. OpenAI key regex misses sk-proj-/sk-svcacct- (secrets.go:28; absorbs ETCH-25)
8. commitSession failure swallowed, printOK lies (session_end.go:62)
9. Recovery records falsified/lossy — share ONE wip→session reducer with Finalize (recovery.go:263; absorbs ETCH-33)
10. tokens never populated — reconcile spec vs Entire payload reality (buffer.go:159; absorbs ETCH-32)
Below-cut: gitDiffFiles rename/quotePath corruption; archive non-atomic quarter (use update-ref --stdin); ScanOrphaned O(N×M) per start (absorbs ETCH-36); mid-rune prompt truncation.

NOT absorbed (still standalone): ETCH-26/27/28/29/39 (distinct secret-scan patterns the review did not cover).

Acceptance: each fix lands with adversarial tests (hook re-delivery, idle-timeout false positive, commit-failure injection, duplicate session_start) — the review's thematic conclusion is that these paths were spec'd but never tested.

### Plan
# ETCH-40: Deep code review remediation (2026-06-04): lifecycle, recovery, redaction, data-quality

Umbrella remediation ticket for the 2026-06-04 deep code review. THE SPEC IS THE REVIEW FILE: reviews/2026-06-04-deep-code-review.md — read it first; it has file:line, verified failure scenarios, and refuted non-bugs (do not re-fix those).

Scope (10 confirmed findings + 4 below-cut):
1. Recovery falsely orphans LIVE idle sessions — capture PID at session_start, wire the liveness check (recovery.go:129; absorbs ETCH-30)
2. findRepoRoot()=os.Getwd() → silent session loss — resolve git common-dir root at the hook boundary (hooks/common.go:39; supersedes-in-part ETCH-34)
3. Session refs silently overwritable — create-only update-ref guard (refs/writer.go:47)
4. Duplicate session_start splits sessions — reuse existing mapping (session_start.go:38)
5. Redaction only covers Prompt.Text — full-record redaction pass at commit boundary (commit.go:24)
6. local_only_fields unimplemented (config.go:13; absorbs ETCH-31)
7. OpenAI key regex misses sk-proj-/sk-svcacct- (secrets.go:28; absorbs ETCH-25)
8. commitSession failure swallowed, printOK lies (session_end.go:62)
9. Recovery records falsified/lossy — share ONE wip→session reducer with Finalize (recovery.go:263; absorbs ETCH-33)
10. tokens never populated — reconcile spec vs Entire payload reality (buffer.go:159; absorbs ETCH-32)
Below-cut: gitDiffFiles rename/quotePath corruption; archive non-atomic quarter (use update-ref --stdin); ScanOrphaned O(N×M) per start (absorbs ETCH-36); mid-rune prompt truncation.

NOT absorbed (still standalone): ETCH-26/27/28/29/39 (distinct secret-scan patterns the review did not cover).

Acceptance: each fix lands with adversarial tests (hook re-delivery, idle-timeout false positive, commit-failure injection, duplicate session_start) — the review's thematic conclusion is that these paths were spec'd but never tested.

---

# Lifecycle/Recovery Batch Plan (Wave 1, agent:lifecycle-w1) — f.9, f.1, f.4, f.3, f.8 + below-cut

**Branch:** `fix/lifecycle-recovery` off `origin/main @ eacd4ed`. Inline-full mode. One PR planned (split only if the diff turns unwieldy). Work order matches the dependency spine: f.9 builds the shared reducer everything else leans on, then the lifecycle guards (f.1, f.4, f.3, f.8), then below-cut items.

## Design decisions (the load-bearing ones)

### D1. Shared reducer shape (f.9)
Split `capture.Finalize` into composable pieces in `internal/capture/buffer.go`:

- `ReduceEvents(sessionID string, events []HookEvent) (*Session, bool)` — pure aggregation (no filesystem writes, no git). Second return = "an end event was seen". Carries all of today's Finalize aggregation incl. end-event handling, plus two hardening rules (D5).
- `FinishSession(s *Session, workDir string)` — duration, outcome.commits, files_touched (git diff with tool-path fallback). Exactly today's post-aggregation block.
- `Finalize(repoRoot, workDir, sessionID)` — `ReadEvents` + `ReduceEvents` + `FinishSession` + write session.json. Unchanged contract for hooks.

Recovery (`internal/recovery`) then imports `capture` (no cycle: capture does not import recovery) and `RecoverSession` becomes:
1. `events := capture.ReadEvents(repoRoot, ulid)`
2. `session, hasEnd := capture.ReduceEvents(ulid, events)`
3. workDir := `session.GitStart.WorktreePath` if it still exists on disk, else "" (diff skipped → tool-path fallback).
4. If `!hasEnd` (true crash): `session.GitEnd = capture.CaptureGitEnd(workDir, startSHA)` best-effort when workDir is live — this is the SAME function session_end uses, captured at recovery time instead of end time, satisfying OUTPUT_SPEC's "git_end reflects the last known git state" without fabricating `git_end == git_start`; then `Status="incomplete"`, `ExitReason="crash"`, EndedAt stays nil (spec: duration null for crashes).
5. If `hasEnd` (f.8 retry — session ended normally, commit failed): keep the reducer's complete/normal result verbatim. Recovery commits the truthful record.
6. `capture.FinishSession(session, workDir)`.

This deletes the entire parallel aggregator: `wipEvent`, `flattenHookEvent`, `applySessionStart`, `applyTokenSnapshot`, the dead flat-decode fallback (cleanup backlog), and old ETCH-33's double-count. Tokens: NO extraction anywhere — aligned with the operator's f.10 decision (tokens null in v1; the schema-privacy lane deletes the remaining dead paths; our reducer simply never touches `session.Tokens`).

`recovery.RefWriter` changes to take `*capture.Session`; `hooks.etchRefWriter` and `commitSession` collapse onto one shared `commitRecord(repoRoot, *capture.Session) error` (redact → schema-bridge → trace → ref write). Recovery cleanup grows: wip + stray `<ulid>.session.json` + mapping-by-ULID reverse scan (new `capture.CleanupMappingByULID` — the review's f.1 scenario shows the stale mapping is what lets a "recovered" live session recreate its wip via O_CREATE).

### D2. PID + liveness policy (f.1)
The Entire payload carries no PID, and the hook's direct parent may be a transient `entire`/shell process. Policy (documented in code):

- **Capture** at session_start: walk the ancestor chain (`ps -o ppid=`), pick the nearest ancestor whose command name matches a known agent runtime (claude/codex/gemini/node/entire); record `pid` + `pid_start_time` (`ps -o lstart=`) in `SessionStartData`. No match → pid 0 (unknown). Recording 0 is strictly safer than recording a too-durable pid (terminal) which would block recovery forever.
- **Liveness check** = pid alive AND start-time matches (kills PID-reuse false-alives).
- **Verified alive → never recovered**, even past the 4h timeout (log the skip for visibility). Rationale: an alive agent can still end normally; recovering it double-records and destroys the live wip — the strictly worse failure. This is the >4h-idle-but-alive policy: alive wins, recovery waits for the process to actually exit.
- **Verified dead → `dead_pid` orphan** (faster-than-timeout recovery preserved, now actually live code).
- **Unknown (pid 0) → timeout governs**, as today.

### D3. Create-only refs with one deliberate exception (f.3)
`refs.WriteSessionRef` uses `git update-ref <ref> <sha> ""` (create-only). On conflict it reads the existing record's `status` (`git show <sha>:session.json`):
- existing `incomplete` + incoming `complete` → **upgrade** via CAS `update-ref <ref> <new> <existingSHA>` (atomic). This is the documented recovery-path interaction: a real session_end racing/following a premature crash-record may legitimately replace it. Never the reverse, never complete→complete.
- anything else → typed `ErrRefExists` (exported sentinel). Callers treat it as "already committed": `commitSession` logs + proceeds to cleanup (returns nil — the data is safe, the wip should die); recovery logs + cleans up the wip/mapping. This is what makes f.8's retry **exactly-once**.

### D4. f.8 — visibility already landed in PR #18; this batch lands the exactly-once retry
Current session_end already does printNotOK + retains wip/mapping. Remaining work is the state machine around it, which D1+D3 deliver: retained wip contains the end event → next session_start's RecoverAll reduces it to a complete/normal record (not 'crash') → create-only ref write (or ErrRefExists no-op) → cleanup. One correct record, no double-finalize divergence.

### D5. Reducer hardening for re-delivery & duplicate ends (feeds f.8/acceptance)
- End-event precedence: first `session_end` is authoritative; a later `stop` never overrides a seen `session_end`; duplicate same-type end events: first wins. (Normal flow unaffected — the refuted non-bug stands; this only matters for retained-wip retries.)
- Tool-call dedup: add `ToolUseID` to `ToolUseData` (capture it in tool_use.go from stdin — parsehook already extracts it); reducer counts a `pre_tool_use` once per tool_use_id when present.

### D6. f.4 — duplicate session_start reuse-guard
In RunSessionStart, before minting: `existing := LookupMapping(StateRoot, ev.SessionID)`; if existing != "" AND its wip exists → duplicate/resume: do NOT mint, do NOT clobber the mapping, do NOT append a second session_start (keeps started_at/duration truthful); exclude `existing` from this invocation's RecoverAll (a resumed-after-crash session's wip must not be crash-committed out from under the resume); log + printOK. Stale mapping (wip gone) → mint fresh as today. `RecoverAll` gains an exclude set.

### D7. Below-cut
- **gitDiffFiles**: `git diff --name-status -z` with rename/copy-aware parsing (R/C consume two NUL-terminated paths). Renames emit `{old, deleted}` + `{new, added}` — spec's action vocabulary is exactly {added, modified, deleted}, and a rename is honestly both. `-z` fixes quotePath octal-escapes and embedded tab/newline corruption for non-ASCII paths. Rename detection is on by default in modern git, so R entries are reachable today.
- **Archive atomicity**: replace archiveQuarter's advance-then-delete sequence with one `git update-ref --stdin` transaction (`update <archiveRef> <new> <old-or-empty>` + `delete <sessionRef> <oldSHA>`...) — all-or-nothing per quarter; a concurrent repoint aborts the whole quarter cleanly and a re-run is consistent (no half-archived state, no snapshot overwrite of an applied quarter).
- **ScanOrphaned perf**: stat-first. Fresh mtime (< activity grace) → skip without opening. Otherwise read only the FIRST line (session_start header: pid/pid_start_time; ULID comes from the filename) — no full event parse during scan. `lastEvent` ≈ file mtime (the file is appended on every event).
- **Prompt truncation**: back off to the previous rune boundary (`utf8.RuneStart` walk) before slicing.

## Commit plan (logical units, in order)
1. f.9 reducer: buffer.go split + recovery rewrite onto it + commitRecord unification + parity tests.
2. f.1 PID capture + liveness + policy + ScanOrphaned stat-first (they share the scan loop) + live-session false-positive tests.
3. f.4 reuse-guard + exclude-set + duplicate session_start tests.
4. f.3 create-only/upgrade refs + tests.
5. f.8 exactly-once retry tests (commit-failure injection; mostly testing what 1+4 built; any session_end glue).
6. Below-cut: gitDiffFiles -z, archive transaction, rune backoff + tests.

## Adversarial acceptance suite (mandatory, from run-state)
- Hook re-delivery: same pre_tool_use (same tool_use_id) and same session_end delivered twice → one record, correct counts.
- Idle-timeout false positive: alive-PID idle wip (spawned `sleep` proc, aged mtime via os.Chtimes) + sibling session_start → NOT orphaned, no ref, wip intact.
- Commit-failure injection: unwritable ref store at session_end → visible failure (non-ok), wip+mapping retained; restore + next session_start → exactly one complete/normal ref.
- Duplicate session_start: same upstream id twice → one ULID, one wip, one mapping; after end → one ref, no 'crash' sibling.
- Recovery parity: same event stream through Finalize vs RecoverSession → identical files_touched/duration/git_end/counts (status/exit_reason aside for the no-end case).
- gitDiffFiles: rename + non-ASCII + tab-in-name cases. Archive: concurrent-repoint interruption → quarter fully unapplied, re-run clean. UTF-8: multibyte boundary truncation stays valid.
- Gates: `go test ./...`, `make build`, `make smoke`, `make test-density` green.

## Plan-Review Cycle 1 Resolutions (AUTHORITATIVE — overrides earlier text on conflict)

Review artifact: `art_01KTHFS99K08CS79ASA0F4W3ZK` (verdict: FAIL plan-level, two targeted revisions). All findings accepted; resolutions below are binding over D1–D7 where they conflict.

**R1 (CRITICAL, revises D1 step 4) — NO live git capture for true crashes.** For `!hasEnd`, recovery does NOT shell live git. `git_end` reflects the last git snapshot present in the wip per OUTPUT_SPEC:474: a copy of `git_start`'s branch/head with empty `commits_produced` — now deliberate and documented, not accidental fabrication (the real f.9(a) defect was the missing end-event case, which `ReduceEvents` fixes). `files_touched` for true crashes: NO live diff (it would attribute other sessions' intervening commits); tool-tracked-paths fallback only. Implementation: recovery passes `workDir=""` for `!hasEnd`; `FinishSession` explicitly skips git diff when `workDir == ""` (guard added — `gitOutput` with empty dir would otherwise run in CWD).

**R1b (corollary, strengthens D1 for hasEnd) — diff bounded by the recorded end SHA.** `FinishSession` diffs `GitStart.HeadSHA..GitEnd.HeadSHA` (not `..HEAD`). Identical result in the normal hook path (HEAD == end SHA at end time), exact in the recovery path (no attribution of commits made after the recorded end). Diff requires both SHAs non-empty; error → tool-path fallback.

**R2 (MAJOR, revises D2) — strict agent-name allowlist; ambiguity records 0.** The ancestry walk matches ONLY specific agent-runtime names ({claude, claude-code, codex, gemini} — NOT generic node/entire, which have ambiguous lifetimes in both directions). The hook's own process is excluded by construction (walk starts at ppid). No specific match → record pid 0 (unknown → timeout governs) — never guess. Selection logic is a pure function over an injectable process-table reader, unit-tested against fabricated ancestry tables (transient hook-runner chain, supervisor chain, direct-agent chain, no-agent chain). Liveness/veto tests use explicit pid fixtures in wips (spawned `sleep` for alive, unused pid for dead) — independent of capture-time matching. Known limitation documented: hung-but-alive agent leaks its wip indefinitely (correct tradeoff vs destroying live data).

**R3 (MINOR, extends D3) — CAS-loser convergence.** If the incomplete→complete upgrade CAS fails (concurrent recovery race), re-read the ref: if it now holds a `complete` record → treat as `ErrRefExists` (already committed; clean up, return nil). Covered in the create-only/upgrade tests.

**R4 (MINOR) — parity test scoped to hasEnd.** The Finalize-vs-recovery parity assertion applies to a wip containing an end event (the f.8 retained-wip path) — that's where parity is meaningful. True-crash records are asserted against their own contract (duration null, git_end == git_start snapshot, files from tool paths, status incomplete/crash).

**R5 (MINOR) — pid/pid_start_time are wip-only recovery metadata.** They live in `SessionStartData` (wip lines) and are never part of the committed `etch.session.v1` record (capture.Session has no pid field; the field-by-field session_start copy in the reducer does not propagate them). A test asserts the committed session.json contains no `"pid"` key.

**R6 (MINOR) — docs in the commit plan.** OUTPUT_SPEC.md §2c (incomplete-record semantics: git_end/files_touched provenance for crashes) and README/SPEC recovery-timeout text get reconciled with the alive-veto policy and R1 semantics. Added as commit 7.

**Note:** single-PR scope held open per review NOTE — the f.9 reducer+recovery rewrite is the natural cut line if the diff grows unwieldy.

## Risks / cautions
- Refuted non-bugs stay untouched (exit_reason clobber in the NORMAL flow, index races, worktree diff-dir).
- schema-privacy lane may land mid-flight (touches recovery token paths + session_start plumbing): rebase carefully; our reducer already never assigns Tokens, so their deletions should compose.
- Never commit `bin/entire-agent-etch`.


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

## Review Checklist

Evaluate the plan against each category. For every issue found, state the
section of the plan, severity (critical / major / minor), and a concrete
recommendation.

### Completeness
- Does the plan address every requirement in the task description?
- Are all acceptance criteria covered by the proposed approach?
- Are there implicit requirements (error handling, logging, documentation) that the plan should address?
- Does the plan identify which files will be created or modified?

### Feasibility
- Is the proposed approach technically sound?
- Are there known limitations, library constraints, or API restrictions that would block the approach?
- Is the scope realistic for a single implementation pass?
- Are the proposed changes compatible with the existing codebase architecture?

### Alignment
- Does the plan solve what the task description asks for — not more, not less?
- Are there any scope creep risks (unnecessary features, premature abstractions)?
- Does the plan respect existing patterns and conventions in the codebase?

### Risk Identification
- What could go wrong during implementation?
- Are there edge cases the plan doesn't address?
- Are there dependency risks (other tasks, external services, data migrations)?
- Does the plan identify any breaking changes or backward-compatibility concerns?

### Acceptance Criteria Coverage
- For each acceptance criterion in the task description, is there a clear corresponding step in the plan?
- Are the criteria testable and verifiable?
- Are there missing acceptance criteria that should be added?

### Architectural Concerns
- Does the plan introduce new patterns that diverge from existing conventions?
- Are module boundaries and layer separations respected?
- Will the proposed changes create technical debt?
- Are there simpler alternatives that achieve the same goal?

## Output Format

Write your review as a structured markdown document with these sections:

### 1. Verdict

One of:
- **PASS** — Plan is complete, feasible, and aligned. Implementation can proceed.
- **FAIL (plan-level)** — Plan has significant gaps or issues that need to be addressed before implementation. The task should return to `in_planning` for revision.

### 2. Summary

2-3 sentences: what was reviewed, overall assessment of plan quality, and the key concern (if any).

### 3. Issues

Ordered by severity (critical first). For each issue:

```
**[SEVERITY] Plan section — Short description**
Description of the concern and why it matters.
**Recommendation:** Concrete suggestion for how to improve the plan.
```

If no issues found, write "No issues found."

### 4. Positive Observations

What the plan does well — clarity, thoroughness, good decomposition, risk awareness.
Acknowledge strong planning to reinforce the practice.

---

Write your review to: <write output here>
