# Plan Review: ETCH-37

## 1. Verdict

**FAIL (plan-level)**

## 2. Summary

The "Plan" section is a verbatim copy of the task description — it contains no implementation plan at all. ETCH-37 presents two mutually exclusive fix paths (add a real per-repo salt **or** correct the docs and document the reversibility limitation), and the plan chooses neither, names no files, and proposes no tests. It also misses that hostname hashing exists in *two* code paths, which materially changes the scope of the "add salt" option. The task itself is well-scoped and easy, but there is nothing here to review as a plan.

## 3. Issues

**[CRITICAL] Plan (whole) — No actual plan; task description copied verbatim**
Lines 17–19 of the plan are character-for-character identical to the task description (lines 14). There is no decomposition, no decision, no file list, no test plan, no acceptance criteria. An implementer receiving this has the same ambiguity the ticket started with. This alone blocks implementation.
**Recommendation:** Produce a real plan that (a) picks one of the two fix paths and justifies it, (b) lists exact files and edits, (c) states the test/verification approach, (d) enumerates acceptance criteria.

**[CRITICAL] Decision not made — two mutually exclusive fix paths**
The task explicitly offers an either/or: *"either add a real per-repo salt and keep the 'salted' claim, or correct the README to 'SHA-256 hash' and document the reversibility limitation."* These have wildly different scope and risk. The plan must commit to one.
**Recommendation:** Recommend the **doc-correction path** as the default unless the operator wants real salting. Rationale: the implementation is unsalted SHA-256 by deliberate design in two places, salting is a non-trivial design change with backward-compatibility consequences (see below), and the ticket's primary defect is a *false claim* in the README. The doc fix fully closes the factual-accuracy gap and honestly documents the privacy limitation. If real salting is desired, that should be scoped as its own design-bearing ticket. The plan should state the choice and reasoning so the reviewer/operator can override.

**[MAJOR] Completeness — hostname is hashed in TWO code paths, not one**
The task only cites `capture/machine.go`. There is a second, independent implementation: `internal/redact/hostname.go` (`HashHostname` / `GetHostname`), also producing unsalted `sha256:%x`. Any "add salt" plan that touches only `machine.go` would leave an inconsistent second path and produce divergent hashes for the same hostname. Even the doc-only path benefits from noting both exist.
**Recommendation:** The plan must enumerate both `internal/capture/machine.go:17` and `internal/redact/hostname.go:12`. If salting is chosen, both must share a single salt-derivation function or the hashes won't match.

**[MAJOR] Completeness — fix must span all doc surfaces, not just README line 108**
The "salted" / privacy wording is not confined to README.md. The schema and spec describe the same field:
- `README.md:108` — "stored as a salted hash" (the wrong claim)
- `OUTPUT_SPEC.md:52` / `:149` — "SHA-256 of hostname" (accurate but silent on reversibility)
- `SPEC.md:36` (acceptance item #7) — "hashed (SHA-256 of hostname) by default"
A doc-correction that only edits README.md leaves the privacy-limitation documentation incomplete and the acceptance criterion (#7) unaddressed.
**Recommendation:** Plan should fix README.md line 108 ("salted hash" → "SHA-256 hash") and add a short reversibility/correlation caveat in README and/or OUTPUT_SPEC near the `hostname_hash` description, so SPEC #7 is satisfiable.

**[MAJOR] Risk — "add salt" path has backward-compat and feature implications the plan ignores**
If salting is chosen, the plan must address: (1) where the salt lives and how it's derived (per-repo file in `.etch/`? generated once? committed or gitignored?); (2) existing session refs already carry unsalted hashes — old and new records would no longer compare equal, breaking cross-record correlation that OUTPUT_SPEC relies on (`OUTPUT_SPEC.md:589` "hostname_hash differs from querying machine ⇒ came from elsewhere"; `:874` "2 distinct hostname_hash values"); (3) salt must be stable across machines sharing a repo or cross-machine correlation within a repo breaks. None of this is acknowledged.
**Recommendation:** Either rule out the salt path for this ticket (preferred), or have the plan explicitly design salt storage, migration/coexistence with existing unsalted refs, and confirm the cross-machine-within-repo correlation property is preserved.

**[MAJOR] Acceptance Criteria — none stated or derived**
The ticket has no enumerated acceptance criteria and the plan adds none. There is no testable definition of done.
**Recommendation:** Add explicit criteria, e.g.: README no longer claims "salted"; reversibility/low-entropy + cross-repo correlation limitation is documented; `grep -rn salt README.md` returns the corrected wording only; doc claims are consistent across README/OUTPUT_SPEC/SPEC; `make test` and `make build` stay green.

**[MINOR] Testing — no verification approach, against project policy**
CLAUDE.md mandates tests per ticket and "loopy" self-validation. For a doc-only fix the right move is to state that explicitly (no code change → existing `capture_test.go` / `redact_test.go` hash assertions remain valid; verify with `make test`), rather than silently omitting it. If salting is chosen, new tests asserting salted output and salt stability are required.
**Recommendation:** Plan should state the verification steps even if it concludes no new code tests are needed.

## 4. Positive Observations

The task description itself is excellent and does most of the analytical work a good plan would need: it correctly identifies the exact defect (`README.md:108`), grounds the claim in the real implementation (`sha256.Sum256` with no salt), articulates the concrete security weakness (low-entropy hostnames like "Hyperion"/"Atlas" are brute-forceable; identical hashes enable cross-repo/cross-machine correlation), and pre-stages two viable remediation paths. The defect is real and verified — `machine.go:17` uses an unsalted SHA-256 and `grep salt` confirms no salt anywhere in source. A plan that simply commits to the doc-correction path, lists the three doc surfaces plus both code paths, and states a one-line verification step would pass cleanly; the raw material is all here.
