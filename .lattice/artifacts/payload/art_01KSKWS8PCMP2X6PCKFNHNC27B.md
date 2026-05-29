# Plan Review: ETCH-3 — Git ref writer

## 1. Verdict

**PASS**

## 2. Summary

The plan for ETCH-3 is well-structured, technically sound, and tightly scoped to its acceptance criterion (SPEC #3). The approach — four git plumbing commands sequenced inside a single `WriteSessionRef()` function — directly mirrors the reference implementation in OUTPUT_SPEC.md §4 and is the right design. One critical dependency concern (ETCH-1 has no code yet) and a few minor gaps are noted below, but none require plan revision — they are implementation-time details.

## 3. Issues

**[MAJOR] Dependencies — ETCH-1 scaffold does not exist yet**
The plan states "All tests use `testutil.NewTestRepo(t)`" and depends on ETCH-1 for the Go module, binary scaffold, and testutil package. However, ETCH-1 has been marked done despite no Go source files, `go.mod`, or directory structure existing in the repo. The plan is correct to declare this dependency — but the implementer will be blocked immediately unless ETCH-1 is actually built first, or ETCH-3 bootstraps its own minimal scaffold.
**Recommendation:** Either (a) re-open ETCH-1 and build the scaffold before dispatching ETCH-3, or (b) explicitly note in the plan that the ETCH-3 implementer should create the Go module, `internal/refs/` package, and a minimal `testutil.NewTestRepo()` if they don't exist yet. Option (b) is pragmatic since ETCH-3 only needs `go.mod` + its own package + testutil — it doesn't need the full subcommand dispatch.

**[MINOR] Plan section "Commit message format" — missing trailing newline/format detail**
The commit message in the plan shows the body lines without a blank separator between the subject line and the body. OUTPUT_SPEC.md §4 shows the format as:
```
cairn session <ULID>
agent: ...
status: ...
```
This matches (no blank line between subject and body lines in the actual spec example). The plan is consistent — but the implementer should be aware that `git commit-tree -m` will treat the entire string as the message verbatim, which is the correct behavior here. No action needed, just noting for completeness.

**[MINOR] Plan section "Function signature" — no GIT_DIR / bare-repo consideration**
The `runGit` helper sets `cmd.Dir = repoPath` but doesn't set `GIT_DIR`. This works for normal repos but would fail for bare repos. Since Etch targets working repos with agents running in them, bare repos are out of scope — but a one-line note acknowledging this assumption would make the plan more explicit.
**Recommendation:** Add a brief note: "Assumes a non-bare repository (agents always work in working trees)."

**[MINOR] Plan section "Tests" — no test for invalid/empty inputs**
The six tests cover the happy path thoroughly but don't include edge cases like empty `sessionJSON`, empty `traceJSON`, or an invalid `repoPath`. These aren't critical for the initial implementation but are worth noting.
**Recommendation:** Consider adding a `TestWriteSessionRef_InvalidRepo` test (non-existent path → returns error) to verify error propagation from the `runGit` helper.

**[MINOR] Plan section "Data types" — `RefMeta` is narrower than the commit message fields**
`RefMeta` includes `Runtime`, `Model`, `Status`, `Branch`, `CommitCount`, `DurationSecs`, and `EndTime`. This is exactly what the commit message needs. However, it doesn't carry the session ID — that's a separate parameter, which is fine. Just noting the split is intentional and clean.

## 4. Positive Observations

- **Direct alignment with OUTPUT_SPEC.md §4.** The plan's implementation steps are a 1:1 match with the reference bash commands in the spec. No interpretation gap between plan and spec.

- **Clean scope boundaries.** The "Not in scope" section explicitly names ETCH-2, ETCH-4, and ETCH-5 as out of scope. The function takes `[]byte` inputs — it doesn't know or care where session data comes from. This is the right abstraction boundary.

- **Concurrency test included.** `TestWriteSessionRef_Concurrent` with 20 goroutines directly validates the zero-contention design claim from the spec. This is the most important test for a system designed for 60–80 concurrent agents, and it's good to see it in the plan.

- **Minimal helper surface.** A single `runGit()` helper rather than an abstraction layer over git. Right level of indirection for this use case.

- **Realistic effort estimate.** ~100 lines of implementation + ~150 lines of tests for a function that shells out to four git commands is accurate and honest.

- **Test decomposition is thorough.** Separate tests for blob content, author/committer, timestamp, and commit message format mean each invariant is independently verified. This makes failures easy to diagnose.
