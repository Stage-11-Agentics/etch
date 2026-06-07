# Plan Review: ETCH-40 — Wave 1 (lifecycle/recovery: f.9, f.1, f.4, f.3, f.8 + below-cut)

## 1. Verdict

**FAIL (plan-level)**

The plan is well-structured, correctly sequenced, and has an excellent adversarial test suite. But the two *load-bearing* design decisions (D1 recovery `git_end`, D2 PID selection) each contain a correctness problem that would undermine the very findings they exist to fix — f.9 (truthful recovery records) and f.1 (don't falsely orphan live sessions). These are cheap to correct now and expensive to discover in code review or production, so the plan should return to planning for two targeted revisions. Everything else passes.

## 2. Summary

Reviewed the Wave 1 lifecycle/recovery batch plan against the 2026-06-04 deep review (the authoritative spec), OUTPUT_SPEC.md, and the current `main` source. I confirmed the plan is correctly scoped to f.9/f.1/f.4/f.3/f.8 + below-cut, that f.2/f.5/f.8-visibility already landed (PRs #17/#18) and the plan accounts for that, and that the shared-reducer approach (D1) is the right structural fix. The blocking concern is that D1's crash-case `git_end` (live-capture at recovery time) contradicts OUTPUT_SPEC.md:474 and reintroduces a falsification risk, and D2's PID-selection heuristic can pick either a transient or an over-durable process — both of which resurrect f.1's data-loss scenario.

## 3. Issues

**[CRITICAL] D1 (recovery step 4) — Live-capturing `git_end` for true crashes contradicts OUTPUT_SPEC and reintroduces falsification**

D1 step 4 sets, for the no-end (true crash) case, `session.GitEnd = capture.CaptureGitEnd(workDir, startSHA)` — shelling live `git` in the workDir *at recovery time* — and justifies it as "satisfying OUTPUT_SPEC's 'git_end reflects the last known git state' without fabricating `git_end == git_start`." This misreads the spec. OUTPUT_SPEC.md:474 says git_end reflects "the last known git state **from the `.wip` file**, not a clean session boundary," and OUTPUT_SPEC.md:475 says files_touched should contain "only files committed before the crash." For a genuine crash the wip contains no end event, so its last known git state *is* `git_start`. Live-capturing HEAD hours later is exactly the "clean session boundary computed later" the spec says it is **not**, and `CaptureGitEnd`'s `rev-list startSHA..HEAD` / `gitDiffFiles startSHA..HEAD` will **attribute any intervening commits made by other sessions in the same checkout to the crashed session** — over-attribution, a fresh falsification in the opposite direction from the one f.9 set out to kill.

Note the real defect behind finding 9(a) is narrower than the plan treats it: the bug is that recovery's `flattenHookEvent` has *no session_end/stop case* (recovery.go:484-487 only handles session_start/prompt/tool), so when the wip legitimately contains an end event (the f.8 retained-wip retry), recovery ignores it and reports `git_end == git_start`. The fix is to make the reducer read end events when present — which `capture.ReduceEvents` already does (buffer.go:206-214) — not to live-capture for the crash case.

**Recommendation:** For `!hasEnd` (true crash), let `git_end` reflect the last git snapshot present in the wip (i.e. `git_start`'s branch/head, empty `commits_produced`) per OUTPUT_SPEC:474 — do **not** shell live git. The truthful enrichment lands automatically in the `hasEnd` case via `ReduceEvents`. If live enrichment is genuinely wanted for crashes, it must (a) be gated to the session's own private worktree (`IsWorktree && workDir == GitStart.WorktreePath`) to prevent cross-attribution, and (b) come with an OUTPUT_SPEC.md update documenting the changed semantics — but the spec-aligned, simpler option is recommended.

**[MAJOR] D2 (PID capture) — "nearest ancestor matching a runtime list" can select a transient OR over-durable PID, both of which resurrect f.1**

The matcher walks the ancestor chain and picks "the nearest ancestor whose command name matches a known agent runtime (claude/codex/gemini/node/entire)." That list mixes processes with opposite lifetimes:
- If it matches a **transient** hook-runner (e.g. a per-invocation `entire` that exits right after the hook), the captured `pid`/`pid_start_time` are dead almost immediately → the next session_start's liveness check sees `dead_pid` → it recovers a **live** session → finding 1's data destruction, reintroduced.
- If it matches an **over-durable** supervisor (a long-lived `node`/`entire` parent that outlives the agent), liveness reports "alive forever" → the session is **never** recovered — the exact failure the plan says it avoids by "recording 0 is strictly safer than a too-durable pid."

The policy's own rationale ("alive wins; record 0 when unsure") is sound, but the selection heuristic doesn't reliably yield the durable *agent session* process it depends on. `node` and `entire` are precisely the ambiguous cases.

**Recommendation:** Pin to the process whose lifetime equals the session's — the agent runtime, not the hook-runner. Concretely: exclude the immediate hook-invocation process, require the chosen ancestor's start-time to predate this session_start (a transient hook-runner won't), and prefer specific runtime names (`claude`/`codex`/`gemini`) over generic `node`/`entire`; if the only match is generic/ambiguous, record `0` (unknown → timeout governs) rather than guess. Add a test that the captured PID survives across two successive hook invocations of the same session.

**[MINOR] D3 — CAS-on-upgrade failure path (concurrent recovery race) unspecified**

The incomplete→complete upgrade uses `update-ref <ref> <new> <existingSHA>`. If two recoveries both read the same `incomplete` existingSHA and both attempt the CAS, the loser's `update-ref` fails (stale old-value). The plan classifies only the *initial* create conflict as `ErrRefExists`; it doesn't say what happens when the **CAS itself** loses the race — that would surface as a generic write error, not the "already committed, clean up the wip" no-op it should be.

**Recommendation:** On CAS failure, re-read the ref; if it now holds a `complete` record, treat as `ErrRefExists` (clean up, return nil). Add this to the create-only/upgrade test alongside the documented incomplete→complete path.

**[MINOR] Recovery-parity test conflates the hasEnd and no-end cases**

The acceptance bullet "same event stream through Finalize vs RecoverSession → identical files_touched/duration/git_end/counts (status/exit_reason aside for the no-end case)" only holds for the **hasEnd** stream (f.8 retained-wip). For a true crash, duration is null, git_end differs, and files_touched is bounded — parity is N/A there, not merely "status/exit_reason aside."

**Recommendation:** Scope the parity assertion explicitly to a wip that contains an end event (the f.8 path). That's where parity is both meaningful and the strongest possible regression guard for the shared reducer.

**[MINOR] PID metadata leakage / silent loss through the schema bridge unstated**

`pid` + `pid_start_time` are added to `SessionStartData` (committed into the wip's session_start event). The plan doesn't state whether they reach the committed record. The capture→schema JSON round-trip in `commitRecord` silently drops fields absent from `schema.Session` (the known lossy-bridge hazard from the cleanup backlog), and OUTPUT_SPEC/schema have no `pid` field — so these will vanish at commit, which is probably intended (pid is recovery-only machine metadata) but should be explicit so it isn't mistaken for a bug, and so nobody adds a schema field expecting it to persist.

**Recommendation:** State that pid/pid_start_time are wip-only recovery metadata, never part of the committed `etch.session.v1` record. Add a one-line assertion in a test that the committed record carries no pid.

**[MINOR] Documentation updates not in the plan**

The batch changes observable recovery semantics — alive-PID sessions are never orphaned even past timeout (D2), and recovered-record `git_end` behavior (D1). OUTPUT_SPEC.md §2c "incomplete records" (lines 469-475) and any SPEC/README description of timeout-based recovery should be reconciled with the new behavior.

**Recommendation:** Add a commit-plan step to update OUTPUT_SPEC.md (recovery semantics) and README if it documents recovery timeout behavior. This is also where the D1 resolution gets recorded.

**[NOTE] Single-PR scope is large but acknowledged**

f.9 (buffer split + recovery rewrite + commitRecord unification) + f.1 + f.4 + f.3 + f.8 + four below-cut items in one PR touches buffer.go, recovery.go, refs/writer.go, session_start.go, session_end.go, gitstate.go, archive.go, user_prompt_submit.go. The plan flags "split only if the diff turns unwieldy" and sequences logical commits well along the dependency spine — acceptable, but hold the split option open; the f.9 reducer + recovery rewrite alone is a reviewable unit and a natural cut line if it grows.

**[NOTE] Known accepted tradeoff — hung-but-alive process leaks its wip**

D2's "alive wins, never recover" means a session whose agent process is alive but permanently hung (never emits session_end) leaves its wip uncommitted indefinitely. This is the correct tradeoff vs. destroying live data, but should be noted as a known limitation (and is a reason the PID must be the agent process, not a supervisor — see the MAJOR above).

## 4. Positive Observations

- **The shared-reducer refactor (D1) is the right call and the strongest part of the plan.** Splitting `Finalize` into `ReduceEvents` / `FinishSession` / `Finalize` and having recovery import `capture` (no cycle — verified `capture` does not import `recovery`) structurally guarantees recovery/normal parity and deletes the entire divergent aggregator (`wipEvent`, `flattenHookEvent`, `applySessionStart`, `applyTokenSnapshot`, the dead flat-decode path). This eliminates the *class* of bug behind f.9/ETCH-33, not just the instance.
- **Excellent, genuinely adversarial test suite.** Every ticket acceptance criterion maps to a concrete test (hook re-delivery via duplicate tool_use_id, idle-timeout false positive via spawned `sleep` + `os.Chtimes`, commit-failure injection via unwritable ref store, duplicate session_start, recovery parity, gitDiffFiles rename/non-ASCII/tab, archive concurrent-repoint, UTF-8 boundary). This directly answers the review's thematic conclusion that these paths were spec'd but never tested.
- **Correctly synchronized with already-landed work.** The plan branches off `eacd4ed`, recognizes f.2 (ResolveRepoContext/StateRoot/WorkDir) and f.8-visibility (printNotOK + wip retention) and f.5 (DeepRedact) already landed, and builds on rather than re-doing them — verified against the current source.
- **D3 create-only refs with a single deliberate incomplete→complete CAS exception** is a precise, well-reasoned immutability model that correctly identifies the one legitimate ref-replacement (premature crash-record upgraded by a real end) and makes f.8's retry exactly-once.
- **D6 reuse-guard + RecoverAll exclude-set** correctly closes the duplicate-session_start split and the resume-vs-recover race in one mechanism, and the plan reasons through the interaction explicitly.
- **Below-cut items are well-specified:** `git diff --name-status -z` rename-aware parsing (verified action vocab is exactly {added, modified, deleted} in OUTPUT_SPEC — renames-as-{deleted,added} is honest), `update-ref --stdin` atomic archive transaction, ScanOrphaned stat-first, and rune-boundary truncation backoff are all technically sound.
- **Tokens handling is correctly deferred** to the operator's f.10 null-in-v1 decision (confirmed in run-state), with the reducer simply never touching `session.Tokens` and a clear rebase note for the schema-privacy lane.
