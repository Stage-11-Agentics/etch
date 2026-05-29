# Plan Review: ETCH-2 — Session buffer + hook handlers

## 1. Verdict

**PASS**

## 2. Summary

The plan proposes a clean architecture for replacing six stub hook handlers with real implementations that capture session metadata into `.wip.jsonl` buffer files and finalize them into `session.json`. The package decomposition (`internal/capture/` and `internal/hooks/`) is well-aligned with ETCH-1's established `internal/` pattern and the scope boundaries are correct — the plan doesn't bleed into ETCH-3 (ref writing) or ETCH-5 (redaction/config). However, the plan is silent on several OUTPUT_SPEC fields that must appear in the finalized `session.json`, and the ULID dependency isn't addressed.

## 3. Issues

**[MAJOR] Finalization / session.go — Several OUTPUT_SPEC fields are unaddressed**

The plan claims to produce "valid `cairn.session.v1` session.json" (SPEC #2) but doesn't mention how the following OUTPUT_SPEC fields are handled in the finalized record:

- **`outcome`** (`commits`, `pr_number`, `pr_state`, `ci_status`) — not mentioned anywhere in the plan.
- **`tokens`** (`input`, `output`, `cache_read`, `cache_write`, `api_calls`, `estimated_cost_usd`) — the `calculate-tokens` capability subcommand is still a stub. The plan doesn't state whether token data is captured from hook events or deferred.
- **`transcript_ref`** (`entire_checkpoint_id`, `local_path`, `available`) — the `session_ref` field IS available in the stdin JSON (per `parsehook.go`), but the plan never mentions mapping it to the `transcript_ref` block.
- **`agent.version`** — the plan captures model from stdin but doesn't specify how agent CLI version is determined.

These fields need to be explicitly handled — either populated from available data or set to null with a note that they're populated by a later ticket.

**Recommendation:** Add a "Schema field coverage" section to the plan that maps every top-level OUTPUT_SPEC section to either: (a) the hook/mechanism that populates it, or (b) an explicit "set to null — populated by ETCH-N" deferral. This prevents the implementer from silently dropping fields.

---

**[MAJOR] Session start — `agent.runtime` determination is unspecified**

The plan says session_start "captures env/git/machine state" but doesn't specify how `agent.runtime` (e.g., `"claude-code"`, `"codex"`, `"gemini-cli"`) is determined. The Entire plugin protocol identifies the *plugin* (`cairn`) via `info`, not the calling agent runtime. The runtime must come from somewhere — an env var, the stdin JSON, or process-tree inference — but the plan doesn't say where.

**Recommendation:** Specify the source for `agent.runtime`. Likely candidates: an env var set by Entire (check what Entire provides in session_start stdin), or a `CAIRN_AGENT_RUNTIME` env var, or inference from the process tree/session_ref path pattern.

---

**[MINOR] go.mod — ULID dependency not mentioned**

The plan relies on ULID generation (`oklog/ulid`) but `go.mod` currently has no dependencies. The implementation will need `go get github.com/oklog/ulid/v2`. This is trivial but should be explicit in the plan since it's a new external dependency.

**Recommendation:** Add a "Dependencies" section listing `oklog/ulid/v2`.

---

**[MINOR] git_end — `commits_produced` derivation not detailed**

The plan says "captures final git state" at session end but doesn't specify how `commits_produced` is computed. This requires comparing `git_start.head_sha` against the current HEAD and enumerating commits between them via `git rev-list`. The implementer could miss this or implement it incorrectly (e.g., capturing only HEAD instead of the full list).

**Recommendation:** Add a note to the git state section: "Derive `commits_produced` from `git rev-list <start_head>..HEAD` at session end."

---

**[MINOR] files_touched — `action` field derivation unclear**

The plan says "files touched are extracted from Read/Write/Edit tool inputs" but doesn't specify how the `action` field (`"added"` / `"modified"` / `"deleted"`) is determined. Tool names alone don't map cleanly to file actions (e.g., `Write` could be a new file or an overwrite).

**Recommendation:** Clarify the heuristic. One approach: defer accurate `action` determination to session end via `git diff --name-status <start_sha> HEAD`, which gives authoritative added/modified/deleted classification. Tool-use tracking collects the file *paths*; git diff resolves the *actions*.

---

**[MINOR] Implementation order — Tests listed as final step**

Tests are step 12 of 12 in the implementation order. The CLAUDE.md testing philosophy says "every ticket ships with tests" and the test strategy itself is solid, but listing tests as a separate final step risks them being treated as an afterthought rather than developed alongside each component.

**Recommendation:** Consider interleaving: implement `capture/buffer.go` → test buffer, implement `capture/gitstate.go` → test gitstate, etc. Or at minimum, note that tests should be written per-component, not batched at the end.

## 4. Positive Observations

- **Clean scope boundaries.** The plan correctly defers ref writing to ETCH-3 and full redaction/config to ETCH-5, doing only the minimal hostname hash needed for ETCH-2. No scope creep.
- **Session ID mapping design is well thought out.** Using `.cairn/sessions/.map/<entire_session_id>` to bridge Entire's session ID to the ULID is the right approach for the stateless hook invocation model.
- **Aligns with ETCH-1's conventions.** The plan uses `internal/` packages (matching what ETCH-1 built) rather than the BUILDPLAN's original top-level package layout. This is the correct call — adapt to what was built, not what was originally sketched.
- **Data flow is clear.** The stdin → hooks → buffer → finalize pipeline is easy to follow and matches the BUILDPLAN architecture.
- **Buffer format is sensible.** JSONL with timestamp + hook type + data per line gives crash recovery (ETCH-4) a clean input format.
- **Acceptance criteria mapping is explicit** — SPEC #1, #2, #10, #11 are all covered with clear correspondence.
- **Test strategy covers the critical paths** — unit, integration, and end-to-end, including env var presence/absence and the 32 KiB truncation boundary.
