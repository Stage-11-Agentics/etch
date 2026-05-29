# Plan Review: ETCH-12 — Lattice orchestrator skill exports `CAIRN_*` env vars

### 1. Verdict

**PASS**

### 2. Summary

I reviewed the ETCH-12 plan, which (a) edits the c11 `lattice-orchestrator` skill so
delegator/sub-agent boot prompts export the `CAIRN_*` orchestration contract, and (b) adds
a marker doc plus Go tests in the Etch repo proving `CaptureOrchestration()` reads exactly
that contract. I verified the plan's findings against the live source in both repos: the Go
code references (`CaptureOrchestration`, the `Orchestration` schema, the hook-layer
`CAIRN_PARENT_SESSION_ID` handling), the `references/orchestrator.md` template structure
(the three named boot templates at the exact cited line numbers), the `OUTPUT_SPEC.md`
contract, and the copy-not-symlink install detail all check out. The plan is complete,
feasible, and well-scoped; the one substantive thing worth resolving before/while
implementing is how `CAIRN_PARENT_SESSION_ID` actually gets a value.

### 3. Issues

**[MAJOR] Changes → c11 repo — `CAIRN_PARENT_SESSION_ID` has no defined source value**

The plan exports `CAIRN_PARENT_SESSION_ID` into delegator/sub-agent boot prompts, and
correctly notes it is consumed at the hook layer (`internal/hooks/session_start.go:77-78`,
verified). But for parent↔child linkage to actually work, the *spawning* session must put
its own Cairn session ULID into that variable. `OUTPUT_SPEC.md` prescribes the pattern
`export CAIRN_PARENT_SESSION_ID="$MY_CAIRN_SESSION_ID"` (line 625) and states "the
orchestrator exports its own session ID so spawned agents inherit it" (line 144) — yet
nothing in the plan (or, as far as I can see, in Cairn today) defines how a running session
learns its own ULID. Cairn generates the ULID internally at `session_start`
(`ulid.MustNew`) and writes only an Entire-session→ULID mapping; it does not appear to
expose the value back to the shell environment. If `$MY_CAIRN_SESSION_ID` is never
populated, the skill will export an empty string and `parent_session_id` will silently be
null on every orchestrated session — the export becomes a no-op for the one field it most
needs to wire.

This may legitimately be out of ETCH-12's "export the vars" scope, but the plan should not
leave it implicit.

**Recommendation:** State explicitly what concrete value the skill places in
`CAIRN_PARENT_SESSION_ID` (e.g., the orchestrator/delegator's own resolved ULID, and the
mechanism to obtain it), OR explicitly scope it out — "export the var with whatever the
parent has; sourcing the parent's own ULID is tracked separately in <ticket>." Either way,
add a one-line note so a future reader doesn't assume parent linkage is live when it may be
emitting null. The smoke test should assert on a field that *is* sourced (e.g. `type`,
`ticket_id`) rather than implying parent linkage works end-to-end.

---

**[MINOR] Etch repo → tests — 3 of the 4 proposed tests duplicate existing coverage with underscore-twin names**

The plan correctly names the existing tests `TestCaptureOrchestrationDefaults` /
`TestCaptureOrchestrationWithEnv` (verified at `internal/capture/capture_test.go:279,301`).
Reading those, the four proposed tests in a new file
`internal/capture/orchestration_lattice_skill_test.go` largely re-test what already passes:
- `TestCaptureOrchestration_AllAbsent` ≈ existing `TestCaptureOrchestrationDefaults`
  (clears the CAIRN vars; asserts `type=="manual"`, ptr fields nil).
- `TestCaptureOrchestration_LatticeSkillExports` ≈ existing `TestCaptureOrchestrationWithEnv`
  (sets all the vars; asserts the six fields + extra).
- `TestCaptureOrchestration_ExtraJSON` ≈ the extra-parsing already asserted inside
  `TestCaptureOrchestrationWithEnv` (it already checks `extra.phase` string and
  `extra.retry == float64(2)`).
- `TestCaptureOrchestration_OnlyOrchestratorType` is the one genuinely new case (partial
  env set — only `TYPE`), not currently covered.

So the net new coverage is one test, delivered as a second test file whose names differ from
the existing ones only by an underscore. That's a maintainability smell (two
`TestCaptureOrchestration*ExtraJSON` etc. one keystroke apart). Note also the plan's claim
that parent-session capture is "exercised by existing hook tests" checks out —
`internal/hooks/hooks_test.go:178 TestOrchestrationEnvVars` covers that path.

**Recommendation:** Add the genuinely-new partial-set assertion (`OnlyOrchestratorType`) to
the existing `capture_test.go` rather than spinning up a near-duplicate file, OR, if a
separate file is mandated by BUILDPLAN, pick names that don't collide-by-underscore and
explicitly state what each adds beyond the existing two tests so a maintainer isn't left
diffing them.

---

**[MINOR] Findings — Trivial line-number drift**

The plan cites `session_start.go:74` for the `CAIRN_PARENT_SESSION_ID` read; the actual
read is at lines 77-78. Cosmetic, but worth correcting since the plan otherwise cites lines
precisely.

**Recommendation:** Update the citation to `session_start.go:77`.

### 4. Positive Observations

- **Findings are accurately grounded in the live source.** I independently verified: the
  six vars + `CAIRN_ORCHESTRATION_EXTRA` read by `CaptureOrchestration()`
  (`internal/capture/environ.go:11-31`), the `Orchestration` struct field/JSON-tag list
  (`internal/schema/session.go:39-47`), the hook-layer parent-session handling, the three
  boot templates (**Fast-track** `### `@333 / `## Environment`@351 / `export LATTICE_ROOT`@353,
  **Inline-full** @378/@391/@393, **Sub-agent-full** @424) in the 999-line
  `references/orchestrator.md` — the plan's cited "~351 / ~391" are exact — the
  `OUTPUT_SPEC.md` contract (lines 604-612, with `c11_delegator` and `lattice-orchestrator`
  as documented enum examples), and the spelling `CAIRN_ORCHESTRATOR_TYPE` matching the code
  exactly. This level of pre-work investigation is exactly what makes a plan reviewable.
- **The subtle architectural distinction is correct.** Recognizing that
  `CAIRN_PARENT_SESSION_ID` is *not* part of `Orchestration` and is instead consumed at the
  hook layer onto `Session.ParentSessionID` is an easy thing to get wrong; the plan gets it
  right, names the existing tests accurately, and adjusts its assertions accordingly.
- **Clean two-repo scoping.** Separating the load-bearing c11 change from the Etch-side
  proof, committing each in its own repo, capturing the c11 SHA in a marker doc, and
  flagging the copy-not-symlink install quirk (verified: the install is a regenerated copy,
  not a symlink) is the right shape for a cross-repo ticket.
- **Genuine end-to-end validation.** Building the binary, exporting the real `CAIRN_*` block,
  piping a `session_start` event, and asserting the populated `.wip.jsonl` orchestration
  block is real verification, not just "tests pass."
- **Respects the project's skill-writing convention** ("keep template edits timeless").

The plan is sound and implementation can proceed. The MAJOR item is a clarify-or-defer, not
a redesign; the MINOR items are quick corrections.
