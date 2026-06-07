# Plan Review: ETCH-24

## 1. Verdict

**FAIL (plan-level)**

## 2. Summary

I reviewed the plan submitted for ETCH-24 (fresh clones don't auto-fetch `refs/etch/sessions/*`; the README should document the post-clone step). The submitted "Plan" is a **verbatim copy of the task description** — it contains no implementation approach, no list of files to change, no test strategy, and no acceptance criteria. As an artifact to implement against, it is not a plan. The redeeming fact is that a genuine, well-structured plan for this exact ticket already exists in the repo as part of the consolidated **Refspec/Sync Batch** (`.lattice/plans/task_01KSTXH7RCZ6JYQ98W0PQSDC6D.md`), which covers ETCH-24 thoroughly; this ticket should be executed against that plan rather than the empty one submitted here.

## 3. Issues

**[CRITICAL] Whole plan — Submitted plan is a verbatim restatement of the task description, not an implementation plan**
The "Plan" block is byte-for-byte identical to the "Task Description" block. It states the problem but proposes no solution. Against the review checklist it fails the basics: it does not identify which files are created/modified, does not describe the README change concretely, does not say whether `setup-refspec` command output should change, and defines no tests or verification. Implementing directly from this would force the delegator to re-derive the entire approach, defeating the purpose of a plan-review gate.
**Recommendation:** Replace the placeholder with a real plan, or — better — explicitly adopt the ETCH-24 portion of the existing consolidated batch plan at `.lattice/plans/task_01KSTXH7RCZ6JYQ98W0PQSDC6D.md` (sections "Design (ETCH-24): clone story" and test 10). That plan already specifies: (a) a new README subsection under Configure — "Second machine / fresh clone" (clone → `entire-agent-etch setup-refspec` → `git fetch origin` → `git for-each-ref refs/etch/sessions/`), (b) a `setup-refspec` success-output hint (`run 'git fetch <remote>' to pull existing session refs`), and (c) an e2e round-trip test as the ETCH-24 gate. Point this ticket at that plan and close the gap.

**[MAJOR] Scope/coordination — ETCH-24 is already absorbed into the refspec batch; the standalone plan ignores that**
`run-state.md` and the consolidated plan both show ETCH-24 bundled with ETCH-16/18/38 into a single `setup-refspec`/transport worker on branch `fix/refspec-batch`. The submitted plan makes no mention of this, creating a real risk of duplicate or conflicting work (two agents editing `setup_refspec.go` and the README Configure section). The current code confirms the batch work is genuinely pending: `internal/commands/setup_refspec.go` still hard-codes `origin`, prints only `"etch refspec configured for push and fetch"` (no fetch hint), and the README has no clone/"Second machine" section at all.
**Recommendation:** State the dependency explicitly. Either (a) execute ETCH-24 as part of the batch PR so the README clone section and the `setup-refspec` fetch hint land coherently with the ETCH-16/18/38 behavior changes, or (b) if ETCH-24 is split out as docs-only, scope it to README-only and note that the command-output hint belongs to the batch — so the two PRs don't both touch the same files.

**[MINOR] Documentation accuracy — a README "one-liner" alone is incomplete for the actual user flow**
The task asks for "a one-liner: after cloning, run `setup-refspec`." But `setup-refspec` only *configures* the fetch refspec; the user still needs an explicit `git fetch` afterward to pull the *already-existing* session refs (the refspec only affects *future* fetches). A bare one-liner risks repeating the original confusion — the user runs `setup-refspec`, sees "configured," and still has zero refs until they fetch.
**Recommendation:** Ensure the documented flow is the full sequence (`setup-refspec` → `git fetch <remote>` → verify with `git for-each-ref refs/etch/sessions/`), not a single command. The consolidated plan already gets this right; the standalone plan's "one-liner" framing does not.

**[MINOR] Verification — no acceptance/verification criteria stated**
The submitted plan defines nothing testable. Documentation tickets still need a verification bar (in this project's "no untested README claims" spirit), otherwise the README can drift from real behavior.
**Recommendation:** Adopt the batch plan's ETCH-24 gate: an e2e round-trip test (repo A with bare remote → clone B → `setup-refspec` in B → `git fetch` → etch refs present in B), and require that every command written in the new README section is exercised by that test before it's documented.

## 4. Positive Observations

- The **underlying ticket is correct and valuable**: the finding (custom ref namespaces aren't fetched by default git, so a naive `git pull` on machine 2 yields zero etch data) is accurate git behavior and a real first-run UX trap worth documenting.
- Although the *submitted* plan is empty, the **project already contains an excellent plan** for this work in the consolidated refspec batch — clear problem inventory, root-cause analysis, explicit file scope, a 10-case binary-level test matrix with ETCH-24 called out as test 10, and validation gates. The fix here is simply to route ETCH-24 to that plan rather than author from scratch.
- The codebase is in a clean, verifiable state for this change: the gap (no README clone section, no fetch hint in `setup-refspec` output) is concrete and easy to confirm, which makes the eventual implementation low-risk.
