# Plan Review: ETCH-26 — Redaction Completeness Batch

### 1. Verdict

**PASS**

The plan is complete, technically sound, and aligned with the task. Every claim I spot-checked against the codebase held up. The issues below are refinements and risk callouts, not blockers — implementation can proceed.

### 2. Summary

Reviewed the "Redaction Completeness Batch" plan whose primary ticket is ETCH-26 (a bare 40-char AWS secret access key leaks because `secrets.go` only matches the labeled `aws_secret_access_key=` form). The plan is unusually rigorous: it correctly diagnoses the root cause, adds a structurally-validated bare-secret pattern, *and* fixes the deeper coverage gap (finding 5 — only `Prompt.Text` is redacted, not the rest of the pushed record) via a reflective deep-walk. It even anticipates that tightening the anthropic pattern breaks an existing e2e fixture. The main thing the reviewer and orchestrator should be conscious of is that this is a **6-ticket batch riding under an ETCH-26 review**, and one implementation subtlety (reflection can't mutate map/interface values in place) needs to be handled deliberately.

I verified against the live tree:
- `internal/redact/secrets.go` matches the plan's "current state" description exactly (aws-secret-key is the labeled-only regex; anthropic is the over-broad `sk-ant-[a-zA-Z0-9_-]+`).
- The ETCH-26 repro string `wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY` is **exactly 40 chars** → the `len==40` validator trick works as designed.
- Struct fields the deep-walk targets all exist: `ToolUseSummary.ByTool map[string]int`, `FileEntry.Path`, `Orchestration.Extra map[string]any`, plus `GitState.HeadSHA` (40-hex), `MachineInfo.HostnameHash` (sha256:+64hex), ULID `SessionID` — the negative-test targets are real.
- The e2e fixture at `internal/hooks/e2e_test.go:37` is `sk-ant-abc123456789012345678901234567890` — confirmed it will **not** match the tightened `sk-ant-[a-z]{2,8}[0-9]{2}-…` pattern (no hyphen-delimited tier segment), so the plan's "fixture update required" callout is correct and necessary.
- `TestScanSecretsAnthropic` / `TestScanSecretsMultiple` use `sk-ant-api03-abcdefghijklmnop` (16-char body) — passes the `{16,}` bound, confirming the plan's "keep" note.
- `config.Settings` has no redaction enable/disable flag, so DeepRedact preserving "always redact" introduces no regression.

### 3. Issues

**[MAJOR] Part A — "mutate in place" model is incomplete for map values and interface-wrapped values**
Go reflection cannot `Set` a value obtained from `Value.MapIndex(...)` or from `Value.Elem()` on a `reflect.Interface` — both are non-addressable. `Orchestration.Extra` is `map[string]any`, so the string (or nested map/slice) lives behind an interface inside a map, which is the *worst* case for in-place mutation: you must construct a redacted copy and write it back with `SetMapIndex`, recursing through the interface boxing. The plan says "rebuild" for maps, which is right, but the umbrella framing of `DeepRedact` as a function that "mutates in place" undersells this; a naïve `v.SetString(...)` implementation will panic or silently no-op on exactly the `Extra` field that finding 5 cares about. The planned `walk_test.go` case ("Secret in `Orchestration.Extra` (map[string]any, nested) → redacted") will catch a wrong implementation, which is good — but the implementer should go in knowing the recursive helper needs a value-returning signature (e.g. `redact(reflect.Value) reflect.Value`) for the map/interface cases rather than pure in-place mutation.
**Recommendation:** In the plan, change the Part A description from "mutates in place" to a value-returning recursive walker that writes back at each container boundary (`SetMapIndex` for maps, field/index set for addressable structs/slices). Keep the nested-`Extra` test as the guard.

**[MAJOR] Scope — six tickets bundled into one PR under an ETCH-26 plan review**
This plan resolves ETCH-26/27/28/29/39 plus ETCH-40 findings 5 & 7 on a single `fix/redaction-batch` branch with "one PR for the whole batch." The batching is *defensible* — every pattern change edits the same slice in `secrets.go` and the ordering is interdependent (block-before-header, labeled-before-bare, anthropic-before-openai), so splitting would create guaranteed merge conflicts. But it means per-ticket acceptance is now coupled: a defect in any one sub-fix blocks the entire batch from merging, and ETCH-26's own acceptance is entangled with five siblings. The reviewer/orchestrator should confirm the other five tickets are actually tracked and will be closed by this PR.
**Recommendation:** Keep the single PR (the file coupling justifies it), but require the PR description to map each pattern/table row → its ticket ID, and confirm ETCH-27/28/29/39 + findings 5/7 are linked so none is silently dropped. Note the coupling risk explicitly so a single sub-fix regression doesn't strand ETCH-26.

**[MINOR] Part C — agent-trace.json is a second pushed blob and is not asserted clean**
`refs.WriteSessionRef` commits *two* blobs: `session.json` and `agent-trace.json`. Finding 5's thesis is "every string-bearing field committed into pushed refs leaks," and the trace is pushed too. In both commit boundaries the trace is derived from the already-redacted session (`SessionToAgentTrace(&schemaSession)` runs after redaction), so it inherits redaction *transitively* and is almost certainly safe — but the planned e2e only inspects `session.json`. Closing the loop is cheap.
**Recommendation:** Add one e2e assertion that `git show refs/etch/sessions/<ulid>:agent-trace.json` contains the marker, not the secret. This makes "every pushed blob is clean" a tested invariant rather than an inferred one.

**[MINOR] Part B — bare-AWS pattern only catches isolated 40-char runs (by design)**
The `[A-Za-z0-9/+=]{40,}` + `len==40` validator deliberately rejects any run longer than 40, which is what makes it reject 64-char sha256 and longer base64 blobs. The side effect: a genuine 40-char AWS secret embedded inside a longer contiguous base64 run (len > 40) will *not* be redacted, because the greedy `{40,}` grabs the superset and the validator rejects on length. This is acceptable under the project's "best-effort regex, not exhaustive" convention, but it's a non-obvious limitation.
**Recommendation:** Add a code comment on pattern #13 documenting the "isolated-run only" behavior, and optionally one test asserting the known-miss case so a future reader doesn't file it as a bug.

**[MINOR] Part B — generic-secret loosening raises false-redaction rate; lock the tradeoff into tests**
Lowering the value minimum 16→8 and adding bare `pass`/`token`/`secret` keywords will over-redact some benign audit context (the plan itself cites `compass=abcdef12`). This is the right call for a credential-leak-averse audit tool, but the behavior should be pinned so it reads as intentional. (Note `tokens: 4096` is safely *not* matched, since `token` must be immediately followed by `\s*[:=]` and `tokens:` has an `s` in between — worth keeping that distinction in a test.)
**Recommendation:** Add at least one negative/over-redaction test that documents an accepted false positive (e.g. `compass=...`) and one that confirms `tokens: 4096`-style prose is preserved, so the precision/recall stance is encoded rather than folklore.

### 4. Positive Observations

- **Root cause, not symptom.** The plan doesn't just bolt on a bare-AWS regex; it surfaces the far more serious finding 5 (only `Prompt.Text` was ever redacted — `FilesTouched`, `ByTool` keys, `Orchestration.Extra` were committed verbatim into immutable, push-replicated refs) and fixes it at the commit boundary for both the normal and crash-recovery paths. That's the difference between patching one ticket and closing the leak class.
- **Codebase-grounded.** The "safety audit of structural fields" (ULID, `HeadSHA`, `HostnameHash`) is accurate to the actual struct definitions, and the validator (`!allHex`, requires upper+lower+digit) is specifically engineered to let 40-hex SHAs through — a real FP that a lazier plan would have shipped.
- **Anticipated the fixture breakage.** Catching that the tightened anthropic pattern silently breaks the existing e2e fixture (`sk-ant-abc123…`) *before* implementation is exactly the kind of second-order consequence plan review exists to surface — and the plan found it itself.
- **RE2 awareness.** Correctly notes RE2 has no lookarounds and routes class-composition checks through a code validator and the maximal-run `{40,}` trick instead of trying to write an impossible regex. The `ReplaceAllString` → `ReplaceAllStringFunc` switch (needed for validators, and incidentally killing `$`-template expansion) is the right plumbing.
- **Explicit non-goals.** Naming the refuted non-bugs and `local_only_fields` (ETCH-31) as out of scope keeps the batch from sprawling and pre-empts scope-creep questions.
- **Ordering discipline.** The pattern table's order is semantically load-bearing (block-before-header fallback, labeled-before-bare, specific-before-generic) and the plan commits to documenting it with in-slice comments — the single most likely place for a future regression, flagged proactively.
