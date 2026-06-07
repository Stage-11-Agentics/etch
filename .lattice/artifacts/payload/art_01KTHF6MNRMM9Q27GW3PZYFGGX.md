# Plan Review: ETCH-23 (batch: ETCH-23 + ETCH-37 + ETCH-40 f.10)

### 1. Verdict

**FAIL (plan-level)**

The ETCH-23 and ETCH-40 f.10 items are well-scoped, accurate against the code, and ready to implement as written. The plan fails on **ETCH-37**: its core design premise — a salt stored in a *committed* `.etch/settings.json` that "all clones share" — is silently defeated by the repo's `.gitignore`, which hard-ignores the entire `.etch/` directory with an explicit "never as tracked files" comment. The plan never mentions `.gitignore`, and the planned validation gates would pass locally while the stated cross-machine correlation goal breaks in production. A concurrency race on first-use salt generation is a second, independent gap given the project's 60–80+ concurrent-agent target.

### 2. Summary

I reviewed a three-item batch plan against the actual source (`internal/capture/session.go`, `buffer.go`, `machine.go`, `internal/schema/session.go`, `internal/hooks/{session_start,commit}.go`, `internal/recovery/recovery.go`, `internal/config/config.go`, `.gitignore`). The ETCH-23 schema-field work and the ETCH-40 f.10 token-cleanup work are correctly reasoned — every file reference, the `commit.go:34` JSON round-trip bridge, the dead `applyTokenSnapshot` path, and the single non-test `CaptureMachine` caller all check out. The blocking concern is ETCH-37: the salt-persistence design is incompatible with the project's gitignore policy, so the chosen "committed salt → cross-machine correlation survives" property cannot hold as planned.

### 3. Issues

**[CRITICAL] Item 2 (ETCH-37) — Committed-salt premise contradicts `.gitignore`; cross-machine correlation silently breaks**
The decision and plan rest on the salt living in a *committed* file: *"stored in **committed** `.etch/settings.json` (all clones share it → cross-machine correlation within a repo keeps working)."* But `.gitignore` line 6 ignores the whole directory:
```
# Session records live in git refs (refs/etch/sessions/), never as tracked files.
.etch/
```
`git ls-files` confirms no `settings.json` is tracked. With `.etch/` ignored, the salt file is never committed by default, so every clone/machine generates its *own* salt and produces a *different* `hostname_hash` for the same physical machine within the same repo — the exact opposite of the design intent. Worse, this is a *silent* failure: the planned validation gates ("cross-repo difference, within-repo stability") both pass in a single local repo because the gitignored file still persists on local disk. The production property (cross-machine, one repo) is not in the gates and would break unnoticed. The plan does not touch `.gitignore` at all.
**Recommendation:** Add a `.gitignore` negation as an explicit plan step (e.g. `.etch/` followed by `!.etch/settings.json`), and verify the carve-out actually un-ignores the file (Git won't re-include a file under an ignored directory unless the directory is first re-included via `!.etch/` then re-excluding contents, or the file is added with `git add -f`; test this concretely). Add a validation gate that asserts a freshly generated `settings.json` is *not* gitignored (`git check-ignore .etch/settings.json` returns non-zero). If the carve-out is deemed undesirable against the "never tracked" policy, escalate the decision rather than shipping a salt design that can't meet its stated goal.

**[MAJOR] Item 2 (ETCH-37) — First-use salt generation races under concurrency; non-atomic write of a committed, every-hook-read config file**
`EnsureHostnameSalt` does read-modify-write on `settings.json`. Etch targets 60–80+ concurrent agents; on a fresh repo, multiple `session_start` hooks fire near-simultaneously, each reads "no salt," each generates a *different* 32-byte salt, and they race to write. Consequences: (a) sessions started in the race window get different salts → unstable `hostname_hash` until it settles; (b) a plain (non-atomic) write that interleaves can corrupt `settings.json`, which `config.Load` reads on *every* hook (`session_start.go:68`, `commit.go:21`, `commit.go:103`) — a corrupt parse degrades all settings, not just the salt. The plan specifies no locking, atomic write, or post-write reconciliation.
**Recommendation:** Write atomically (temp file + `os.Rename` in the same dir) and re-read after writing so a loser adopts the winner's salt (first-writer-wins idempotence). Document the transient race window as accepted, or generate the salt once via an advisory lock. At minimum the write must be atomic given it's a shared, every-invocation config file.

**[MAJOR] Item 2 (ETCH-37) — "All clones share it" requires a human commit Etch never performs**
Even with the gitignore fixed, Etch only ever writes git *refs* (orphan commits); it never stages/commits working-tree files. So the first agent writes `settings.json` into the working tree, but it stays *uncommitted* until a human runs `git add .etch/settings.json && git commit && git push`. Until then, a second machine that clones the repo has no salt and mints its own — so "cross-machine correlation within a repo keeps working" only becomes true *after* someone commits and pushes the file. The plan and README wording present this as automatic.
**Recommendation:** Make the operational requirement explicit in the README change ("commit `.etch/settings.json` to share the salt across machines") and in a PR note. Consider whether the plan should surface a one-time hint/log on salt creation. State plainly in the plan that cross-machine correlation is contingent on the file being committed.

**[MINOR] Cross-cutting — Second, unsalted hostname-hash path left inconsistent (`internal/redact/hostname.go`)**
`redact.HashHostname` / `redact.GetHostname` hash the bare hostname unsalted, duplicating the logic the plan is salting in `capture/machine.go`. These currently have no live callers (`GetHostname` is uncalled; `HashHostname` is called only by `GetHostname`), so it's effectively dead code — but it directly contradicts the README's "salted hash" claim the plan is trying to make true, and is a latent footgun if a future caller wires it in.
**Recommendation:** Either delete the dead `redact` hostname path in this batch (low risk, it's uncalled) or add a one-line plan note acknowledging it's intentionally left and tracked for removal, so the "salted hash" claim isn't quietly false in a live code path later.

**[MINOR] Item 1 (ETCH-23) — Recovery-path records won't carry `agent_session_id`; ensure no gate asserts it there**
The plan correctly scopes the recovery aggregator out, so crash-recovered records (committed via `etchRefWriter.WriteSessionRef`, which marshals from `schema.Session` at `commit.go:109`) will have a null `agent_session_id` until the lifecycle worker reworks recovery. This is acknowledged. Just confirm the validation gate's end-to-end assertion runs the *normal* finalize path (it does — `Finalize` → `commitSession` marshals `capture.Session` at `commit.go:28`), not a recovered session, so the gate doesn't falsely fail or falsely pass.
**Recommendation:** No change required beyond keeping the e2e assertion on the normal path. Optionally add a one-line PR note that recovered records are a known temporary gap.

### 4. Positive Observations

- **Code-accurate references throughout.** The `commit.go:34` JSON round-trip bridge is real, and the plan correctly identifies that the *committed* `session.json` is marshaled from `capture.Session` (`commit.go:28`) — so adding `AgentSessionID` to `capture.Session` + populating it in `Finalize`'s `session_start` case (`buffer.go:163`) is exactly the right insertion point, with the `schema.Session` mirror keeping the trace path consistent. Tag-matching requirement is correctly called out.
- **ETCH-40 f.10 dead-path analysis is precise.** The plan correctly observes that `applyTokenSnapshot`'s `TokensInput==0 && TokensOutput==0` guard always bails because `flattenHookEvent` never extracts token keys from the nested format (the only format `capture.AppendEvent` writes), and that `capture.Session.Tokens` is already never populated — so `"tokens": null` already holds and the cleanup is genuinely dead-code removal, not behavior change. Keeping the field as reserved is the right call for forward compatibility.
- **Blast radius correctly bounded.** `CaptureMachine` has exactly one non-test caller (`session_start.go:74`), so the signature change is low-risk; the plan flags the existing tests that need the new signature.
- **Strong scope discipline.** Conflict-avoidance constraints (recovery.go limited to dead token removal, README limited to hostname/token claims, no other ETCH-40 findings touched) are explicit and respect the parallel workers' lanes. Deliberately-out-of-scope sections are stated rather than left implicit.
- **Good test specificity.** Each item carries concrete, falsifiable assertions (upstream id preserved AND distinct from the minted ULID; salt stability/difference; literal `"tokens": null`).

---

*Reviewer note:* The two non-blocking items (ETCH-23, ETCH-40 f.10) could proceed immediately; the blocking concerns are isolated to ETCH-37 and are fixable with a `.gitignore` carve-out, an atomic/idempotent salt write, and honest README/PR wording about the commit requirement. Recommend returning to `in_planning` to revise ETCH-37, then re-submitting.
