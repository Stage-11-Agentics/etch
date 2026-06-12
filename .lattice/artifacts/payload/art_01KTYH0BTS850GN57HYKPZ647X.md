# Plan Review: ETCH-48 — worktree stamping + post-checkout self-propagation + dedupe

## 1. Verdict

**PASS** — Plan is complete, feasible, and aligned. Implementation can proceed,
with the issues below (one major, several minor) folded in during implementation.

## 2. Summary

Reviewed the ETCH-48 plan against the task description, `docs/ENABLEMENT.md`
(the spec), and the actual codebase at origin/main e1ea295 (the ETCH-47
squash-merge). The plan is strong: every claim it makes about existing code was
verified true — the stamp command shape matches the c11 pilot hand-stamps
byte-for-byte (checked against `c11-cmux-catchup/.claude/settings.local.json`),
`internal/enable` exists with the `replaceBlock` discipline the plan reuses, and
`install.go`'s byte-preserving merge generalizes cleanly into the proposed
`InstallEntries`/`RemoveEntries` API. The one real gap is a completeness
omission: the task explicitly assigns the `code/platform/etch.md` Worktrees
update to the implementer, and the plan's Docs section omits it.

## 3. Issues

**[MAJOR] Docs section — platform doc update missing**
The task description says: "the platform doc (code/platform/etch.md, separate
repo) Worktrees section should be updated by the implementer to drop the
interim-recipe framing once this ships." The plan's Docs section lists README,
HOOK_CONTRACT.md, ENABLEMENT.md, and the ETCH-46 ticket comment — but not the
platform doc. This is an explicitly enumerated deliverable; without it in the
plan, it will be forgotten at ship time. It does not affect the architecture,
so it does not warrant returning to planning.
**Recommendation:** Add a bullet to the Docs section (or Out of scope →
post-merge checklist, alongside the c11 pilot validation): after merge, update
`code/platform/etch.md` Worktrees section to replace the interim hand-stamp
recipe with "run `etch enable` once per clone."

**[MINOR] internal/enable additions — worktree enumeration robustness**
`git worktree list --porcelain` can report worktrees whose directories no
longer exist (prunable), are locked, or are on detached HEAD; a bare entry can
also appear. Stamping a missing directory will error and could abort `enable`
mid-run, leaving partial coverage.
**Recommendation:** State the policy in the plan: skip non-existent worktree
paths with a warning, continue stamping the rest, and never let one bad
worktree fail the whole enable. Add a test row for a pruned/missing worktree
path if cheap.

**[MINOR] Post-checkout block — chaining with non-sh pre-existing hooks**
"Append block if file exists" assumes the existing `post-checkout` is a
POSIX-shell script. A hook with a non-shell shebang (`#!/usr/bin/env python3`,
husky's node wrappers in exotic setups) would be corrupted by appending sh
syntax. Also, a pre-existing hook that `exec`s or `exit`s before the appended
block means the block silently never runs.
**Recommendation:** Check the shebang before appending: if the first line is a
recognizable sh-family shebang (or absent), append; otherwise warn and skip
(doctor's hooksPath/coverage checks per ETCH-46 are the named backstop). The
exec/exit-before-block case can stay best-effort, but mention it as a known
limitation in HOOK_CONTRACT.md.

**[MINOR] stamp-worktree — "reuses the guard's zero-spawn config read" needs a
semantics-distinct helper, not reuse of `HooksDisabled`**
`HooksDisabled` treats an absent `etch.enabled` key as *enabled* (the team-mode
compatibility rule). `stamp-worktree` needs the opposite default: absent key →
no stamping (operator mode only), which the plan correctly states. The phrase
"reuses the guard's zero-spawn config read" is fine for the machinery
(`findCommonDir` + `parseConfigKey` are reusable), but the plan should be
explicit that this is a new explicit-true predicate, not a call to
`HooksDisabled` — an implementer who literally reuses the guard would stamp
every team-mode repo on every checkout.
**Recommendation:** Name the new predicate in the plan (e.g.
`OperatorModeEnabled() bool` — true only when the key is present and true) and
note it shares the zero-spawn parsing internals. Test 8 already covers the
behavior; this is about preventing a wrong-default implementation.

**[MINOR] Design — doctor scope handled as ticket comment: correct, but make
the AC#3 mapping explicit**
The task says "extend etch doctor scope (ETCH-46) with checks," and acceptance
criterion 3 reads "propagation works **or** doctor flags it." Since doctor is
not implemented (verified: no `doctor` case in `main.go` dispatch), the plan's
choice — make propagation work under custom `core.hooksPath` (test 4) and
extend the ETCH-46 ticket with the new checks — satisfies the criterion via its
first arm. This is the right call, but the plan should say so explicitly so the
code reviewer doesn't read the missing doctor code as a gap.
**Recommendation:** Add one line under Out of scope: "AC#3's 'doctor flags it'
arm is deferred to ETCH-46 (ticket extended); this ticket satisfies AC#3 via
the 'propagation works' arm, proven by test 4."

## 4. Positive Observations

- **Every factual claim verified true against the codebase.** The stamp command
  is byte-identical to the live c11 pilot hand-stamps, which makes the
  "idempotent upgrade via `matchersContainCommand`" claim real, not aspirational
  — `matchersContainCommand` compares JSON-decoded strings, so the hand-stamps
  will be detected as already-installed exactly as the plan says.
- **Acceptance criteria coverage is complete and traceable.** AC1→test 1,
  AC2→test 2, AC3→test 4, AC4→test 5, AC5→the stated test substrate. The
  dedupe-ships-here requirement is honored in the design section, not deferred.
- **The `install.go` refactor is genuinely minimal.** Generalizing the command
  builder to `cmdFor func(string) string` while keeping team-mode behavior via
  the existing `hookCommand` is the smallest change that unlocks reuse — no
  premature abstraction.
- **Logic-in-Go, thin-shell-shim for post-checkout** is the right architecture:
  the hook block stays trivially idempotent and version-independent while
  `stamp-worktree` carries the real behavior, testable in Go.
- **Risk awareness is built in**: best-effort disable with warnings (stale
  stamps gated by the config key), `replaceBlock` discipline reused for the
  hook file, and the test list includes the foreign-content-preservation and
  pre-seeded-hand-stamp cases that bite real users (the c11 main checkout's
  existing permissions block).
- **Test 1 is a true end-to-end headline test** — real `git worktree add` with
  the binary on PATH, then executing the stamped command and asserting capture
  in shared state — matching the project's testing philosophy exactly.
