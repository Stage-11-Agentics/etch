# Plan Review: ETCH-14 — Fix SPEC #7 and #11 gaps

### 1. Verdict

**FAIL (plan-level)**

### 2. Summary

The plan correctly identifies both PARTIAL criteria and proposes a structurally
reasonable, well-tested fix for Gap 1 (raw hostname opt-in). However, the Gap 2
approach for `pane_lineage` contradicts the project's own `OUTPUT_SPEC.md`, which
already prescribes the exact capture mechanism — and the plan invents a different
one that produces semantically different data. Additionally, Gap 1 ignores the
task's explicit instruction to wire `redact/hostname.go:GetHostname()`, opting to
duplicate the hash logic inline instead. Both should be corrected before
implementation.

### 3. Issues

**[CRITICAL] Gap 2 — `pane_lineage` approach contradicts OUTPUT_SPEC.md**

The plan declares "c11 exposes **no orchestration-ancestry API**" and derives
`pane_lineage` from the *pane's surface stack* via `c11 tree --all --json`, ordered
by `index_in_pane`. The plan itself concedes this yields "the sibling-agent stack"
— surfaces co-located in one pane.

But `OUTPUT_SPEC.md` already defines this field, and it means something different:

- Line 612: `CAIRN_PANE_LINEAGE` — "JSON array of ancestor tab titles. The spawning
  orchestrator exports its own pane_lineage; Cairn appends the current pane's tab
  title." Example: `["Orchestrator","FT-481 :: Impl"]`.
- Example records (lines 136, 360, 464) all show
  `["FT-481 Orchestrator", "FT-481 :: Impl :: Claude"]` — a parent→child
  **orchestration ancestry** chain, not surfaces sharing a pane.

So the spec already specifies the mechanism: read the `CAIRN_PANE_LINEAGE` env var
(exported by the spawning orchestrator) and append the current tab title. The
plan's `c11 tree` pane-stack derivation would populate the field with the wrong
semantics (co-located siblings instead of orchestration ancestors) and ignores the
documented contract entirely. SPEC #11 only lists the field name; OUTPUT_SPEC
defines its meaning, and the plan diverges from it.

**Recommendation:** Drop the `c11 tree`/`c11Cmd`/`index_in_pane` design. Implement
Gap 2 per OUTPUT_SPEC.md line 612: parse `CAIRN_PANE_LINEAGE` (JSON array; ignore
on parse error), append the current `tab_title`, and fall back to a single-element
`[tab_title]` for solo sessions. Update the test plan accordingly (parent-chain +
current, solo single-element, malformed-JSON-ignored). Reference OUTPUT_SPEC.md
explicitly in the plan so the contract is the source of truth.

**[MAJOR] Gap 1 — Plan ignores the task's explicit instruction to wire `GetHostname()` and duplicates logic**

The task description says: "Wire `redact/hostname.go:GetHostname()` into the
capture pipeline." `GetHostname(settings config.Settings) HostnameResult` already
exists and does exactly what's needed — always returns the hash, returns the raw
hostname only when `RawMachineIdentity` is true. The validation report (line 44)
also calls this out: "call `redact.GetHostname()` instead of inline SHA-256."

The plan instead re-implements `os.Hostname()` + `sha256.Sum256` inline inside
`CaptureMachine`. This (a) does not follow the task's explicit wiring instruction,
and (b) duplicates config-aware hashing that already lives in `redact`, leaving
`GetHostname()` as effectively dead code (referenced only by its own tests). Two
copies of the hash/raw decision will drift.

**Recommendation:** Have `CaptureMachine` call `redact.GetHostname(settings)` and
map the `HostnameResult` onto `MachineInfo` (hash → `HostnameHash`; non-empty raw →
`&raw` on `HostnameRaw`). This satisfies the task verbatim and removes the
duplication.

**[MINOR] Gap 1 — Pointer signature `*config.Settings` with nil-handling is unnecessary**

The plan proposes `CaptureMachine(settings *config.Settings)` with a "nil settings
→ defaults" branch and a dedicated `TestCaptureMachine_NilSettings`. Since the
caller already loads config (falling back to `config.Defaults()` on error), there's
no need for a nilable parameter — a value type `config.Settings` with the zero/
default value is simpler and removes a self-imposed nil contract. (The existing
`config_test.go` and the rest of the codebase already pass `config.Settings` by
value.)

**Recommendation:** Use `CaptureMachine(settings config.Settings)` by value; drop
the nil branch and the nil-settings test, replacing it with a defaults test
(`config.Defaults()` → hashed-only).

### 4. Positive Observations

- **Accurate diagnosis.** Both gaps are correctly traced to the right functions
  (`CaptureMachine`, `CaptureC11`) and the root cause (config never read; lineage
  never populated) matches the validation report.
- **Strong test thinking for Gap 1.** The hash-set/raw-nil, raw-opt-in, and
  end-to-end `session_start` integration test (write `settings.json`, run hook,
  read ref, assert `hostname_raw`) is exactly the right coverage and aligns with
  the project's "every ticket ships with tests" rule.
- **Good caller awareness.** The plan correctly identifies `session_start.go` as
  the wiring point and specifies non-fatal config loading with a `Defaults()`
  fallback — the right resilience posture for a capture hook.
- **Testability instinct on Gap 2.** Even though the chosen mechanism is wrong, the
  instinct to add a CLI indirection var for fakeability is a good one; the
  env-var-based approach is even easier to test (`t.Setenv`) and the plan can carry
  that testability intent forward.

---

**Bottom line:** The plan needs to return to `in_planning` to (1) re-base Gap 2 on
the `CAIRN_PANE_LINEAGE` mechanism documented in OUTPUT_SPEC.md, and (2) wire
`redact.GetHostname()` for Gap 1 rather than duplicating the hash logic. Both are
specified by existing artifacts (OUTPUT_SPEC.md line 612, the task description, and
the validation report), so the corrections are well-bounded.
