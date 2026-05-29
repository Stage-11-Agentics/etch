# Plan Review: ETCH-7

## 1. Verdict

**PASS** — Plan is complete, feasible, and aligned. Implementation can proceed. A handful of minor inaccuracies in assumed function signatures should be corrected during implementation; none are structural.

## 2. Summary

Reviewed the ETCH-7 end-to-end wiring plan against the actual Go source on all dependency branches (ETCH-1 through ETCH-6). The plan correctly identifies the type mapping challenge between `capture.Session` and `schema.Session`, proposes a sound JSON round-trip solution (verified: all JSON tags match across the two types), and decomposes the wiring into clean, testable steps. The main concerns are minor signature mismatches the plan assumes vs. what the code actually exports, and two capability subcommands (`extract-all-modified-files`, `calculate-total-tokens`) that exist as stubs in `main.go` but aren't addressed in the plan.

## 3. Issues

**[Minor] Section 1 (commit.go), Step 2 — `redact.Redact` signature mismatch**
The plan says "Apply `redact.Redact()` to `session.Prompt.Text`", implying a single-argument call. The actual signature is `func Redact(text string, settings config.Settings) string` — it requires a `config.Settings` value. The plan already loads settings in step 1, so the fix is trivial, but the plan text should reflect the two-argument call to avoid confusion during implementation.
**Recommendation:** Update step 2 to `redact.Redact(session.Prompt.Text, settings)`.

**[Minor] Section 1 (commit.go), Step 8 — `capture.RemoveWip` return type**
The plan implies `RemoveWip` could fail (it's listed as a step in an error-returning function). The actual signature is `func RemoveWip(repoRoot, sessionID string)` — it returns nothing (fire-and-forget `os.Remove`). Similarly, `CleanupMapping` (step 10) has signature `func CleanupMapping(repoRoot, entireSessionID string)` with no return value.
**Recommendation:** Note that steps 8 and 10 are void calls. No error handling needed for them.

**[Minor] Section 1 (commit.go), Step 9 — No exported function to remove `.session.json`**
The plan says "Remove the `.session.json` file (data is now in the ref)." The helper `sessionJSONPath()` is unexported in the `capture` package. The implementer will need to either: (a) add an exported `RemoveSessionJSON(repoRoot, sessionID string)` to capture, or (b) construct the path manually. Both are fine, but the plan should acknowledge this.
**Recommendation:** Add a note that a `RemoveSessionJSON` helper should be added to the capture package, or specify the manual path construction.

**[Minor] Section 5 (commands/) — `extract-all-modified-files` and `calculate-total-tokens` not addressed**
The current `main.go` dispatch includes four stub subcommands: `extract-modified-files`, `calculate-tokens`, `extract-all-modified-files`, and `calculate-total-tokens`. The plan only addresses the first two. The aggregate variants likely iterate across all session refs. If these are part of the Entire plugin protocol, they need implementation.
**Recommendation:** Either add implementations for the aggregate variants, or explicitly note them as out-of-scope with justification. If they're kept as stubs, at minimum update `main.go` to route them to a stub that's clearly marked as deferred, not to the removed `stubs.Run()`.

**[Minor] Section 7 (e2e test) — Push/fetch testing gap**
The BUILDPLAN states ETCH-7 should "Test the full lifecycle: hook fires → buffer → finalize → ref write → **push → fetch on another clone**." The e2e test plan covers everything up to ref verification but does not test push/fetch. This is understandable (it requires a remote or a local bare repo), but a local clone test (push to a bare repo, fetch from another clone) would satisfy the BUILDPLAN requirement without requiring network access.
**Recommendation:** Add an e2e test case that creates a bare repo, pushes the session ref via refspec, and verifies it fetches correctly from a clone of that bare repo. This also exercises `setup-refspec`.

**[Minor] Dependency declaration — ETCH-4 not listed**
The plan integrates crash recovery from ETCH-4 (section 4 calls `recovery.RecoverAll`), but ETCH-7's formal dependency list (in the Lattice task) only includes ETCH-2, ETCH-3, ETCH-5, ETCH-6. In practice this works because the `feat/etch-7-e2e-wiring` branch already contains the ETCH-4 commit, but the missing formal dependency could cause issues if tasks are ever re-ordered.
**Recommendation:** Add ETCH-4 as a formal dependency in the Lattice task.

## 4. Positive Observations

- **JSON round-trip approach is verified sound.** I compared every field across `capture.Session` and `schema.Session` at the JSON tag level. All tags match. The Go type differences (value vs. pointer for `Orchestration`, `Machine`, `Operator`, `Outcome`, `ToolUse`; `string` vs `*string` for `Timing.StartedAt`; `*int64` vs `int64` for token fields) all round-trip correctly through JSON marshaling — Go handles null→zero and value→pointer conversions gracefully.

- **Clean separation between `commitSession` and `cairnRefWriter`.** The plan correctly identifies that crash recovery (via `RefWriter`) starts from `schema.Session` while the normal flow starts from `capture.Session`, and provides distinct code paths that converge at `refs.WriteSessionRef`. This avoids a forced shared abstraction.

- **Error handling philosophy is correct.** Ref-write failures logged but not propagated to the agent runtime matches the project's design principle: metadata capture should never break the agent's workflow. The plan is explicit about this.

- **Comprehensive e2e test coverage.** The test plan covers the full lifecycle, redaction verification, and crash recovery — the three highest-risk integration points. Testing crash recovery within the e2e flow (create orphaned .wip, verify session_start recovers it) is particularly good.

- **Good decomposition.** New files are well-scoped (commit.go for the pipeline, refwriter.go for the adapter, one file per capability subcommand). No premature abstractions or unnecessary helper layers.

- **Risk assessment is honest and accurate.** The plan correctly identifies crash recovery latency as a v1 acceptable tradeoff, and correctly scopes redaction to prompt text (tool-use data in this project contains only tool names and file paths, not raw content).
