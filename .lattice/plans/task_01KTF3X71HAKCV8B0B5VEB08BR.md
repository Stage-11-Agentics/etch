# ETCH-41 Plan — `local_only_fields`: strip-before-push transport for session refs

**Ticket:** ETCH-41 (high) · **Branch:** `feat/local-only-transport` · **Worktree:** `Etch-worktrees/local-only-transport`
**Decision context:** Operator decided 2026-06-06 (ETCH-40 finding 6): implement for real. Configured `local_only_fields` must actually stay off the wire. This plan decides the mechanism; the "whether" is settled.

## 1. Problem

`Settings.LocalOnlyFields` is parsed (`internal/config/config.go:13`) and read nowhere. README documents it as a working privacy control ("field names to strip before a ref is pushed to a remote"). Today a bare `git push` (with the setup-refspec config, `refs/etch/sessions/*:refs/etch/sessions/*`) ships the full record — the promise is false.

**Core promise to implement:** a bare `git push` of `refs/etch/sessions/*` must NOT leak configured fields, silently or otherwise.

## 2. Design decision: projection at WRITE time, not push time

**Chosen mechanism:** when `local_only_fields` is non-empty, the commit boundary writes TWO refs per session:

| Ref | Content | Transport |
|---|---|---|
| `refs/etch/sessions/<ULID>` | **stripped** (pushable) record | pushed/fetched by the existing etch refspec, unchanged |
| `refs/etch/local/<ULID>` | **full-fidelity** record | never named by any etch-configured refspec; stays local |

When `local_only_fields` is empty or absent (the default): exactly today's behavior — one ref, full record, no `local` namespace. Zero behavior change.

### Why this beats the alternatives

The ticket's design space suggested (as an *e.g.*) a parallel public namespace (`refs/etch/public/*`) with setup-refspec pushing only that. All push-time approaches share a fatal flaw against git's transport model:

1. **Public-namespace + refspec rewrite:** safety depends on every clone's refspec config being current. A user who ran `setup-refspec` with an old binary (or before configuring `local_only_fields`) keeps the direct `refs/etch/sessions/*` push refspec — bare `git push` leaks the full record silently. Exactly the failure mode we're closing. It also forces a refspec migration for every existing user and splits the fetch namespace (own sessions vs others' sessions land in different namespaces).
2. **Pre-push hook:** git pre-push hooks can only veto, not rewrite, outgoing refs. Hooks don't survive clones, can collide with user/Entire hooks, and are bypassed by `--no-verify`. Veto-only would break push UX without providing the projection.
3. **`entire-agent-etch push` subcommand:** projection lives in a command nobody is forced to use; bare `git push` with the configured refspec still leaks. Violates the core promise by construction.

**Write-time projection is safe by construction:** the pushable namespace simply never contains the private data. It is robust to *every* config vintage — stale refspecs, hand-written refspecs (`git config --add remote.X.push 'refs/etch/sessions/*:...'` per README), direct `git push origin 'refs/etch/sessions/*'`, mirrors of that namespace. No setup-refspec changes, no migration, no new push flow to steer users into. The fetch round-trip is also clean: your own pushed (stripped) refs fetch back byte-identical to your local `sessions/` refs — no clobber of full-fidelity data, which lives only in `local/`.

**Accepted trade-off:** on the authoring machine, `refs/etch/sessions/<ULID>` shows the stripped record; full fidelity requires `git show refs/etch/local/<ULID>:session.json`. This is arguably correct UX — "what you see in `sessions/` is what the world sees" — and only affects users who opted into hiding. Documented in README.

**Known residual exposure (documented, not closed):** a user who configures a *wildcard* refspec `refs/etch/*:refs/etch/*` by hand would push `local/` too. setup-refspec never writes that; README warns against it.

## 3. Field addressing grammar (dot-paths)

A configured field is a dot-separated path of JSON field names exactly as they appear in `session.json`:

- Each segment matches an object key (struct json tag, or map key for `orchestration.extra.*`).
- **Implicit array fan-out:** when descent meets an array, the remaining path applies to every element — `files_touched.path` strips the `path` of every entry. A path ending at an array strips the whole array (`files_touched`).
- A path ending at an object strips the whole subtree (`machine`, `c11`, `prompt`).
- Paths that match nothing in a given record are no-ops for that record (e.g. `prompt.text` when the session has no prompt). Documented: typos are not detectable — pick paths from OUTPUT_SPEC.
- **Not strippable (skip-listed):** `schema_version`, `session_id`, `local_only_stripped` — structural identity; stripping them would break the record/ref correspondence. Silently skipped.
- No wildcards in v1; array fan-out covers the real cases.

## 4. Strip semantics, markers, schema validity

In-place projection over the record (reflection walk in the `internal/redact` package, same idioms as `walk.go` — pointer/interface deref, json-tag field resolution, settability handling; targeted path descent rather than a second full traversal):

- **String value** (incl. `*string`) → replaced with marker `[LOCAL_ONLY:<path>]` — consistent with the `[REDACTED:<name>]` redaction marker style.
- **Map value** (e.g. `orchestration.extra.foo`, type `any`) → replaced with the marker string.
- **Pointer to struct/slice/map** → `nil` (JSON `null`; these fields are nullable per OUTPUT_SPEC).
- **Slice/map (non-pointer)** → nil (JSON `null`).
- **Numeric/bool** → zero value.
- **Manifest:** every applied path is recorded in a new top-level field `local_only_stripped: []string` (`omitempty`) on the stripped record only — the explicit, type-safe marker for fields whose type can't carry a string marker. Full local record never has it.

Result remains valid `etch.session.v1`: unmarshals cleanly into `schema.Session`, `schema_version`/`session_id` intact. `local_only_stripped` is added to both `capture.Session` and `schema.Session` and documented in OUTPUT_SPEC.

### Commit-message metadata (leak found during planning)

`refs.WriteSessionRef` embeds runtime/model/status/branch/commit-count/duration in the ref's **commit message** (`internal/refs/writer.go:55`). The pushed ref's `RefMeta` must be built from the **stripped** record, not the full one — otherwise `git log refs/etch/sessions/X` leaks a stripped branch name. Implementation: compute the pushed ref's meta after stripping; the local ref's meta from the full record.

### Trace

`agent-trace.json` is in the same pushed tree and derives `files` and `model` from the session. Pushed ref's trace is regenerated from the **stripped** record; local ref's trace from the full record.

## 5. Implementation steps

1. **`internal/schema/session.go` + `internal/capture/session.go`:** add `LocalOnlyStripped []string \`json:"local_only_stripped,omitempty"\`` to both Session structs.
2. **`internal/redact/strip.go`** (new): `StripLocalOnly(v any, fields []string) (applied []string)` — generic reflection path-descent per §3/§4. Works on both `*capture.Session` and `*schema.Session`. Plus `internal/redact/strip_test.go`.
3. **`internal/refs/writer.go`:** generalize ref target — `WriteSessionRefIn(repoPath, refName, ...)` (or namespace param); `WriteSessionRef` keeps its signature delegating to the sessions namespace. Export `LocalRefPrefix = "refs/etch/local/"` next to the sessions prefix.
4. **`internal/hooks/commit.go`** — both commit paths (`commitSession` and recovery's `etchRefWriter.WriteSessionRef`):
   - `DeepRedact` as today → marshal full JSON + full trace.
   - If `len(settings.LocalOnlyFields) > 0`: write full record + full-meta to `refs/etch/local/<ULID>`; then `StripLocalOnly` in place, set `LocalOnlyStripped`, re-marshal, regenerate trace, rebuild meta → write to `refs/etch/sessions/<ULID>`.
   - Else: single write to `sessions/` exactly as today.
5. **Docs:** README settings + Privacy sections describe real semantics (strip-at-commit, `refs/etch/local`, grammar, markers, "applies to sessions recorded after the setting is set; existing refs are immutable", wildcard-refspec warning, remove/replace any 'in development' caveat). OUTPUT_SPEC: `local_only_stripped` in schema section + `refs/etch/local` in git-layout section.

No changes to `setup_refspec.go` — the safety property is independent of refspec config (that's the point). Archive/index/query operate on `sessions/` and are unaffected; archive of the `local/` namespace is explicitly out of scope (noted in PR).

## 6. Tests & validation gates

- **Unit (strip.go):** nested paths; array fan-out; whole-object strip; map-key strip in `orchestration.extra`; missing-path no-op; skip-list enforced; marker format; non-string nulling; `applied` list correctness; idempotence; stripped record round-trips through `schema.Session` unmarshal/marshal (schema-valid).
- **Commit boundary (hooks):** temp repo + `.etch/settings.json` with `local_only_fields` → simulated session: `sessions/` ref stripped (marker present, secret absent, `local_only_stripped` populated), `local/` ref full (secret present, no manifest); commit message of pushed ref carries stripped values; trace stripped. Empty config → no `local/` ref, byte-identical behavior to today.
- **Recovery path:** crash-recovery commit produces the same dual-ref projection.
- **Transport e2e (the gate):** temp repo + temp bare remote → `setup-refspec` → session with a secret in a configured field → bare `git push` → fresh second clone + `setup-refspec` + `git fetch` → cloned `session.json` has marker, not secret; `refs/etch/local/*` absent on remote and clone; original repo's `local/` ref intact with the secret.
- **Suite:** `go test ./...`, `make build`, `make smoke` all green.

## 7. Phases

1. Schema fields + strip.go + unit tests.
2. Writer generalization + commit.go dual-write + hook/recovery tests.
3. Transport e2e test.
4. Docs (README, OUTPUT_SPEC).
5. Full gates; attach validation evidence note; PR.

## Plan-Review Cycle 1 Resolutions (AUTHORITATIVE — overrides earlier text on conflict)

Review verdict: FAIL (2 MAJOR, 4 MINOR) — core mechanism affirmed; all findings accepted and resolved as follows.

**R1 (MAJOR — struct divergence between commit paths): ACCEPTED — unify on `schema.Session`.** The pushed (stripped) blob is always marshaled from `*schema.Session` on BOTH paths. In `commitSession`: the full/local blob stays the capture-derived `sessionJSON` (unchanged from today); when stripping is configured, strip the already-existing `schemaSession`, set `LocalOnlyStripped` on it, and marshal THAT as the pushed `session.json` (plus regenerate trace and meta from it — `buildRefMetaFromSchema` already exists). The recovery path strips its `*schema.Session` identically. Consequences: `LocalOnlyStripped` is added to `schema.Session` ONLY (not `capture.Session` — §5 step 1 amended); `StripLocalOnly`'s canonical input is `*schema.Session`. The strip walker still defines a value-struct rule (zero it) for robustness, but no caller depends on it.

**R2 (MAJOR — deviation from push-time direction + local-query regression): ACCEPTED — keep write-time, surface the deviation loudly.** (a) This plan section + the PR body explicitly flag: the design deviates from the ticket's "projection at push time" example wording because every push-time mechanism is unsafe against stale/hand-written refspecs (§2); operator sign-off happens at PR review. (b) The local-queryability regression is real and explicitly scoped: when `local_only_fields` is set, `etch query`/index/`git show refs/etch/sessions/…` on the authoring machine see the stripped record; full fidelity is only at `refs/etch/local/<ULID>`. Query/index fallback to `local/` is **scoped out** of this ticket and filed as a follow-up backlog ticket at PR time (referenced in the PR body). (c) README rewrite must drop the "kept in the local ref only" singular-ref framing and describe the two-namespace reality precisely, including that local tooling reads the stripped `sessions/` namespace.

**R3 (MINOR — over-claimed schema validity): ACCEPTED.** Validity gate restated as the testable property: the stripped record round-trips through `schema.Session` unmarshal/marshal with `schema_version`/`session_id` intact. Skip-list extended to cover OUTPUT_SPEC **required** fields: `schema_version`, `session_id`, `status`, `agent.runtime`, plus `local_only_stripped`. Skip rule generalized to prefixes: a configured path P is skipped iff some required path R equals P or R starts with P+`.` — so stripping `agent` whole is skipped (it contains required `agent.runtime`), while `agent.model` is strippable. Skipped paths are silently ignored and never listed in `local_only_stripped`; documented in README.

**R4 (MINOR — crash window between the two ref writes): ACCEPTED.** Ordering invariant stated: `refs/etch/local/<ULID>` is written FIRST, `refs/etch/sessions/<ULID>` (the canonical, recovery-tracked ref) LAST, so `.wip` removal coincides with the canonical ref existing. A crash between the writes leaves an orphan `local/` ref; recovery re-runs and rewrites both — content-identical records produce identical orphan commit SHAs (deterministic author/committer dates from the record's `ended_at`), making the rewrite an `update-ref` no-op; records lacking `ended_at` get a fresh timestamp and the ref is simply updated. Self-healing either way.

**R5 (MINOR — no 'in development' caveat exists in README): ACCEPTED.** §5 step 5 reframed: there is nothing to remove; the work is to REWRITE the README `local_only_fields` settings entry (line ~146) and Privacy bullet (line ~200) to describe the two-namespace strip-at-commit behavior per R2(c).

**R6 (MINOR — "same machinery" wording): ACCEPTED.** Stated plainly: `StripLocalOnly` is a NEW targeted path-descent walker sharing `walk.go`'s reflection idioms (pointer/interface deref, json-tag resolution, settability handling), distinct from `DeepRedact`'s exhaustive traversal. This satisfies the ticket constraint's intent (no duplicated full-record traversal machinery; shared idioms and package); the PR body says so explicitly so a literal reading doesn't trip review.
