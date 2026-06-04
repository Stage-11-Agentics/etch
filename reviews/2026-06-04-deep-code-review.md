# Deep Code Review — Full Etch Codebase

**Date:** 2026-06-04
**Scope:** Entire codebase on `main` @ 8589c24 (~9,200 lines Go; ETCH-1 → ETCH-15)
**Method:** /code-review high effort — 7 finder angles (3 correctness, 3 cleanup, 1 altitude) → 42 candidates → dedup → 18 independent verifiers → 15 confirmed, 3 refuted
**Cross-reference:** Adversarial QA pass had already filed ETCH-16–39; overlaps are noted per finding.

## Thematic read

The build is structurally solid, but the failure cluster is unmistakable: **recovery and concurrency invariants were asserted in spec, never adversarially tested.** Findings 1, 3, 4, 8, 9 are all "the crash/duplicate/idle path silently corrupts what the happy path built," and findings 5–7 are all "the security layer covers less than documented." The density test (ETCH-8) exercises happy-path concurrency, not hook re-delivery, idle-timeout false positives, or commit-failure injection.

## Top 10 findings (CONFIRMED, ranked by severity)

### 1. Recovery falsely orphans live idle sessions → data loss + duplicate refs
`internal/recovery/recovery.go:129` — PID is never captured (`SessionStartData` has no pid field; `extractPID` always returns 0), so the dead-PID liveness check at recovery.go:119 is dead code and only the 4h timeout fires.
**Scenario:** an agent idles >4h alive. A sibling's session_start triggers RecoverAll → writes a 'crash' ref for the live session and deletes its .wip (mapping left intact). The live session's next hook recreates the wip from scratch via `O_CREATE` (buffer.go:97), losing all prior events; its real session_end writes a SECOND ref for the same ULID.
**Overlap:** ETCH-30 covers the dead-PID root cause but frames the consequence as "crashes not recovered" — this finding adds the worse consequence (live-session data destruction). *Novel severity.*

### 2. findRepoRoot() = os.Getwd() → sessions silently dropped, unrecoverable
`internal/hooks/common.go:39` — returns raw CWD (doc comment claims git-root resolution); all four hook entry points anchor `.etch/sessions/` wip + ULID mapping to the hook process's CWD.
**Scenario:** hooks for one session fire from different CWDs (subdir, linked worktree). session_end's LookupMapping reads a different `.etch`, returns "", takes the silent `printOK()` path — session dropped, no record. Recovery only scans the current CWD's `.etch`, so the orphan is never recovered. gitstate.go already resolves the true root via `--git-common-dir`; findRepoRoot ignores it.
**Overlap:** ETCH-34 (config-framing, medium). This finding upgrades severity: session loss, not just config miss.

### 3. Session refs are silently overwritable — immutability violated
`internal/refs/writer.go:47` — bare `git update-ref <ref> <sha>`, no create-only guard (empty old-value arg), no existence check.
**Scenario:** recovery re-processes a wip whose CleanupWIP failed (or finding #1 fires): an already-committed COMPLETE record is overwritten by an 'incomplete'/'crash' record for the same ULID. Violates OUTPUT_SPEC.md:150 ("once committed… never updated") with no detection. *Novel.*

### 4. Duplicate session_start splits one session across two records
`internal/hooks/session_start.go:38` + `internal/capture/buffer.go:54` — session_start unconditionally mints a fresh ULID; WriteMapping is a plain `os.WriteFile` that clobbers any existing mapping. No LookupMapping reuse-guard, though every other hook uses LookupMapping.
**Scenario:** session_start fires twice for one Entire session ID (resume, duplicate delivery under load). The map repoints to ULID#2; ULID#1's wip is orphaned → recovered later as a truncated 'crash' record. One logical session → two refs. *Novel.*

### 5. Redaction only covers Prompt.Text — tool-use fields committed unredacted
`internal/hooks/commit.go:24` (and :105) — `redact.Redact` is called only on `session.Prompt.Text`. `FilesTouched[].Path` and `ToolUse.ByTool` keys are committed verbatim. Violates SPEC.md:37 ("Prompt **and tool-use fields** are scanned… before commit").
**Scenario:** a secret embedded in a file path reaches an immutable, push-replicated git ref — unredactable after the fact.
**Deeper fix:** one redaction pass over all string-bearing fields of the finalized record at the commit boundary, instead of per-field calls authors must remember. *Novel.*

### 6. local_only_fields documented but completely unimplemented
`internal/config/config.go:13` — `Settings.LocalOnlyFields` parsed, read nowhere. README.md:109/163 document it as a working privacy control.
**Overlap:** = ETCH-31 (already filed, high).

### 7. OpenAI key pattern can't match modern keys
`internal/redact/secrets.go:28` — `sk-[a-zA-Z0-9]{20,}` excludes `-`/`_`; `sk-proj-…` dies at the hyphen after 4 chars. No other builtin pattern catches a bare key (anthropic requires `sk-ant-`; generic-secret requires a `key=` label). Legacy keys still match.
**Overlap:** = ETCH-25 (already filed, critical).

### 8. commitSession failure is swallowed — printOK lies, state left ambiguous
`internal/hooks/session_end.go:62-67` — on commitSession error: only `log.Printf`, then `printOK()` and `return nil`. Wip + mapping survive (cleanup correctly ordered after ref write in commit.go:45-54).
**Scenario:** transient git failure at session_end. A later stop hook re-resolves the intact mapping, appends another event, re-finalizes, re-writes the ref (double-finalize, no guard); or recovery later commits the session as Status:'incomplete'/ExitReason:'crash' though it ended normally. *Novel.*

### 9. Crash-recovered records are systematically falsified/lossy
`internal/recovery/recovery.go:263` (+350-489) — recovery's parallel event→session aggregator diverges from Finalize:
- (a) GitEnd fabricated as a copy of the session_start SHA (recovery.go:263-266); no session_end/stop case in flattenHookEvent → recovered records always show `git_end == git_start`, zero commits_produced.
- (b) flattenHookEvent never extracts token fields from the nested format actually written (buffer.go:86-90), so applyTokenSnapshot's zero-check (recovery.go:290-292) always bails → tokens:null. Violates OUTPUT_SPEC.md:473.
- (c) files_touched, duration_ms, outcome.commits never computed (Finalize does all three, buffer.go:220-246).
**Deeper fix:** one shared wip→session reducer — recovery calls the Finalize path with status/exit_reason overrides.
**Overlap:** ETCH-33 (recovery double-counts tool calls) is the same divergence family. *Mostly novel; supersedes/absorbs ETCH-33.*

### 10. tokens never populated in any code path
`internal/capture/buffer.go:159` — Finalize's switch handles four hook types, never assigns session.Tokens. Root cause is upstream: the Entire payload (parsehook.go:11-24) carries no token counts at all. OUTPUT_SPEC.md:101-108's promise is currently unmeetable — implementation and spec need reconciling.
**Overlap:** = ETCH-32 (already filed, medium).

## Below the cut (confirmed, outranked)

- **gitDiffFiles parsing corrupts renames and non-ASCII paths** — `internal/capture/gitstate.go:70-95`. `--name-status` without `-z`/`--no-renames`: `R100\told\tnew` → Path="old\tnew" (embedded tab) with action "modified"; quotePath octal-escapes stored verbatim; embedded-newline paths split across lines. Fix: `-z` + rename-aware parse, or `--no-renames`. *Novel.*
- **Archive quarter half-applies under concurrency** — `internal/archive/archive.go:217-226`. Archive ref advances FIRST, then guarded per-ref deletes; a concurrent repoint mid-archive fails the delete after the advance, aborting remaining quarters with session both live and archived; re-runs overwrite the archived snapshot. Fix: `git update-ref --stdin` atomic transaction. *Novel.*
- **ScanOrphaned fully JSON-parses every wip (incl. live ones) on every session_start** — `internal/recovery/recovery.go:91`. O(sessions × events) per start at 60-80 agents. Fix: mtime stat pre-filter. **Overlap:** = ETCH-36.
- **Prompt truncation slices mid-rune** — `internal/hooks/user_prompt_submit.go:25`. `prompt[:maxPromptBytes]` with no rune backoff; json degrades trailing bytes to U+FFFD. Low severity. *Novel.*

## Refuted candidates (verified non-bugs, for the record)

- *exit_reason clobbered by stop-after-session_end* — REFUTED: both end-hooks route through runEnd which finalizes and tears down wip+mapping immediately; two end-events never co-aggregate (session_end.go:18-68).
- *index update races lose sessions permanently* — REFUTED: query filters stale entries against the live ref set (query.go:143-152), and the known-set is recomputed from the index file each run so dropped-but-live refs re-add on next Update (self-healing).
- *Finalize diff runs against wrong checkout in worktrees* — REFUTED: repoRoot = CWD flows into cmd.Dir, so the diff runs in the session's own worktree (the CWD anchoring problem is finding #2, not this).

## Cleanup backlog (valid, no tickets warranted yet — fold into touched code)

- `runGit`/`runGitEnv` duplicated byte-for-byte: refs/writer.go:77 ↔ archive.go:271; a third variant in index/build.go:168 ↔ query/query.go:234; commands/common.go:14 re-implements gitShow. → one `internal/gitexec` home.
- `commitEnv` ↔ `archiveCommitEnv` duplicate the etch author identity.
- Two parallel Session types (capture.Session ↔ schema.Session) bridged by a lossy JSON round-trip in commit.go:32 — fields present in one but not the other vanish silently. Collapse to schema.Session.
- Dead flat `wipEvent` decode path in recovery.go:35/336 (only nested format is ever written).
- Settings read 3× per session (ReadTimeoutFromSettings + config.Load ×2).
- CaptureGitState shells out 5× where one batched `git rev-parse` returns all values; loadViaRefs execs `git show` per session instead of reusing batchReadSessions' `cat-file --batch`.
- Test-repo setup duplicated in capture_test/hooks_test instead of testutil.
