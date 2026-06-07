# Plan Review: ETCH-41 — `local_only_fields` strip-before-push transport

## 1. Verdict

**FAIL (plan-level)**

The core design is strong and the safety argument for write-time projection is genuinely better than the push-time alternatives. But two plan-level issues should be resolved before implementation: (1) a concrete correctness gap where the plan's two commit paths will produce *divergent* JSON for whole-object strips while the plan explicitly claims they produce "the same dual-ref projection," and (2) an unreconciled deviation from the operator-approved direction ("projection layer **at push time**") that carries a real local-queryability regression which the plan treats as settled UX rather than surfacing as a decision. Both are narrow and quick to fix; the core mechanism does not need to change.

## 2. Summary

I reviewed the ETCH-41 plan against the actual codebase: `internal/config/config.go`, `internal/refs/writer.go`, `internal/hooks/commit.go` (both commit paths), `internal/redact/walk.go` + `secrets.go`, `internal/commands/setup_refspec.go`, `internal/schema/session.go`, `internal/capture/session.go`, and the README/OUTPUT_SPEC docs. The plan is well-researched — its diagnosis of the push-time failure modes is accurate, the refspec it cites (`refs/etch/sessions/*:refs/etch/sessions/*`) is real, and the commit-message/trace leak it surfaced during planning is a genuine catch. The blocking concern is that the strip semantics in §4 are specified against a struct shape that differs between the two records the plan runs them on, producing inconsistent output the plan assumes is identical.

## 3. Issues

**[MAJOR] §4 / §5 step 2 & 4 — `capture.Session` and `schema.Session` differ in pointer-ness; whole-object strips will diverge between the two commit paths**

The plan says `StripLocalOnly` "Works on both `*capture.Session` and `*schema.Session`" (step 2) and that "the crash-recovery path produces the same dual-ref projection" (§6). It does not. The normal path (`commitSession`) marshals `*capture.Session` to the ref blob, so it strips `capture.Session`; the recovery path (`etchRefWriter.WriteSessionRef`) strips `*schema.Session`. These structs disagree on the type of major sub-objects:

- `capture.Session`: `Orchestration Orchestration`, `Machine MachineInfo`, `Operator OperatorInfo`, `Outcome Outcome`, `ToolUse ToolUseSummary` are **value** types (session.go:15–24).
- `schema.Session`: the same fields are **pointers** `*Orchestration`, `*Machine`, … (session.go:11–24).

§4 enumerates strip behavior for "Pointer to struct → nil (JSON null)", "Slice/map → nil", "Numeric/bool → zero" — but has **no rule for a non-pointer (value) struct**. So stripping a whole object like `machine` or `orchestration`:
- On the recovery path (schema, pointer) → `nil` → JSON `null`.
- On the normal path (capture, value) → there is no nil; the implementer must zero the struct → JSON `{"hostname_hash":"","os":"","os_version":"","arch":"",...}`.

Two different pushed records for the same configuration, depending on whether the session went through normal commit or crash recovery. The plan's §3 even names `machine` as a canonical whole-object-strip example, so this is on the hot path, not a corner case.

**Recommendation:** Unify the representation that gets stripped. The cleanest fix needs no new struct surface area: in `commitSession`, the code already unmarshals into `schemaSession` for trace generation (commit.go:33–34) — strip `schemaSession` and marshal **that** to the ref blob (instead of the capture-derived `sessionJSON`), so both paths strip `*schema.Session` and emit identical JSON. If the plan instead keeps stripping `capture.Session`, it must (a) add an explicit "value (non-pointer) struct → zeroed" rule to §4 and (b) accept and document that null-vs-empty-object differs by path — but unifying on `schema.Session` is strictly simpler and removes the "works on both structs" requirement entirely.

**[MAJOR] §2 — Plan inverts the operator-approved timing (push-time → write-time) and the resulting local-query regression is asserted as acceptable rather than surfaced as a decision**

The task's operator-approved direction is explicit: "projection layer **at push time** — e.g. a parallel public ref namespace … or a pre-push hook." The plan chooses **write-time** projection and argues (well) that all push-time approaches are unsafe against stale refspec config. The reasoning is sound and the "e.g." framing arguably leaves room. But the chosen design has a consequence the plan under-weights: with write-time projection, `refs/etch/sessions/<ULID>` — the namespace that `etch query`, the index (`internal/index`), `etch archive`, and the documented `git show refs/etch/sessions/<ULID>:session.json` all read — now holds the **stripped** record for the authoring user. The full data lives only in `refs/etch/local/`, which none of those tools read. So a user who sets `local_only_fields` to hide a field from teammates also loses the ability to query/index/aggregate that field **on their own machine**.

This directly tensions with the *currently documented* contract the README states today (verified, README.md:164 and 222): "field names to strip before a ref is pushed to a remote (**kept in the local ref only**)" and "keep selected fields out of any ref that gets pushed." A reasonable reader expects the data to remain locally *usable*, not just locally *present in a ref nothing queries*. The plan calls this "arguably correct UX" and moves on — but it is a behavioral change to a documented promise and a deviation from the approved push-time direction, so it deserves an explicit operator-visible flag, not a one-line accepted-tradeoff aside.

**Recommendation:** Keep write-time projection (the safety argument justifies it), but call out the deviation and the local-query regression explicitly as a decision point in the plan/PR for operator awareness. Either (a) confirm the operator accepts that locally-hidden fields become locally-unqueryable, or (b) scope in a follow-up so `etch query`/index can fall back to `refs/etch/local/<ULID>` when present (the plan currently scopes `local/` archival out — be explicit that querying is scoped out too, since the README implies otherwise). At minimum, the README rewrite (step 5) must drop the "kept in the local ref only" / "local ref" singular framing and describe the two-namespace reality precisely, including that local tooling reads the stripped `sessions/` namespace.

**[MINOR] §6 / §4 — "remains valid `etch.session.v1`" rests on Go-unmarshalability, but whole-object/array strips emit `null` for fields OUTPUT_SPEC may treat as required/array**

OUTPUT_SPEC.md:7 states "Fields are nullable unless marked **required**" and shows `files_touched` as an array. Stripping `files_touched` (whole) yields `null`, and stripping `machine` yields `null`/empty-object. The plan's validity claim is "unmarshals cleanly into `schema.Session`" — which is true (all the relevant schema fields are pointers/slices) — but that's weaker than "valid per OUTPUT_SPEC's required-field annotations." There is no JSON-schema validator in the tree (validation == Go round-trip), so this is unlikely to actually break, but the plan should not over-claim.

**Recommendation:** Restate the validity gate as "round-trips through `schema.Session` unmarshal/marshal" (the real, testable property) and add one line confirming that stripping a field OUTPUT_SPEC marks **required** (e.g. `session_id` is already skip-listed; check `schema_version`, `status`, `agent.runtime`) is either skip-listed or explicitly permitted. The skip-list in §3 covers `schema_version`/`session_id`/`local_only_stripped` but not other required fields like `status` or `agent.runtime` — decide whether those are strippable.

**[MINOR] §5 step 4 — crash window between the `local/` write and the `sessions/` write is unspecified**

Today `commitSession` writes one ref then `RemoveWip` (commit.go:46–50). The plan adds a `local/` write *before* the `sessions/` write. If the process dies after `local/` but before `sessions/`, you get an orphan `refs/etch/local/<ULID>` with no canonical `sessions/` ref. The `.wip` is still present (RemoveWip runs last), so recovery re-runs and rewrites both — content-identical orphan commits, so it self-heals — but the plan doesn't state the ordering invariant or confirm idempotence.

**Recommendation:** Write `sessions/` (the canonical, recovery-tracked ref) as the last update-ref so the `.wip`-removal point coincides with the canonical ref existing; note that re-running on the same finalized record is idempotent (same tree → same orphan SHA → `update-ref` no-op). One sentence in §5.

**[MINOR] §5 step 5 — there is no existing "in development" caveat in README to remove**

The plan says to "remove/replace any 'in development' caveat (ETCH-40 finding 6 interim)." Verified: README.md currently documents `local_only_fields` as a *working* feature (lines 164, 222) with no in-development marker — the interim caveat was apparently never applied. Harmless, but the step as written implies an edit that has no target.

**Recommendation:** Reframe step 5 as "rewrite the README `local_only_fields` settings + Privacy entries to describe the two-namespace strip-at-commit behavior" (no removal needed), so the implementer doesn't go hunting for a caveat that isn't there.

**[MINOR] §4 — "reuse the same record-walking machinery" (ticket constraint) is satisfied by parallel idioms, not literal reuse; worth stating plainly**

The ticket asks the projection to "reuse the same record-walking machinery" as ETCH-40 finding 5. `DeepRedact`/`walkValue` (walk.go) is an *exhaustive* walk applying regex to every string; strip needs *targeted path descent*. These are genuinely different traversals, so a new walker in `internal/redact` sharing walk.go's deref/json-tag/settability idioms is the right call — but it is "same idioms," not "same machinery." This is fine; just don't let a later reviewer read the ticket literally and object.

**Recommendation:** No design change. One sentence in §4 acknowledging that strip is a new targeted-descent walker sharing walk.go's reflection idioms (pointer/interface deref, json-tag resolution, settability), distinct from `DeepRedact`'s exhaustive pass, satisfies the constraint's intent.

## 4. Positive Observations

- **The push-time-vs-write-time analysis is the plan's standout.** The §2 enumeration of why public-namespace, pre-push hook, and `push` subcommand each fail against git's transport model is accurate and well-reasoned — git pre-push hooks genuinely can only veto, and the stale-refspec leak is the exact failure mode being closed. Choosing safety-by-construction over config-dependent safety is the correct instinct.
- **Real leaks found during planning.** The commit-message metadata leak (`refs.WriteSessionRef` embeds branch/model/status in the commit message via `formatCommitMessage`, writer.go:55–58) and the `agent-trace.json` derivation leak are both genuine — verified in the code — and easy to miss. Catching them at plan time is exactly the value of this gate.
- **Zero-behavior-change default is preserved and explicitly gated** on `len(LocalOnlyFields) > 0`, with a byte-identical-to-today test. This keeps blast radius minimal.
- **`local_only_stripped` manifest** is a clean solution to the "non-string fields can't carry a marker" problem and gives auditability of what was stripped per record.
- **The dot-path grammar with implicit array fan-out** (§3) is well-specified — skip-list for structural-identity fields, no-op for non-matching paths, no wildcards in v1 — and matches the real OUTPUT_SPEC shape (`files_touched.path`, `orchestration.extra.*`).
- **Test plan is thorough and correctly identifies the transport e2e as *the* gate** (bare `git push` → fresh clone → assert marker-not-secret and `local/` absent on remote). That end-to-end assertion is what actually proves the core promise.
