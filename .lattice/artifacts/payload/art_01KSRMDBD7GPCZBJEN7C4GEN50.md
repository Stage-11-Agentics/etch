# Plan Review: ETCH-13 — Productization (README, Makefile, install path, rename Python PoC)

### 1. Verdict

**FAIL (plan-level)**

### 2. Summary

I reviewed the submitted plan for ETCH-13 against the task description and the actual
repository state (`go.mod`, `.gitignore`, `cmd/entire-agent-cairn/`, the tracked
`./entire-agent-cairn` Python PoC, and the installed `entire` CLI). The "plan" is a
**verbatim restatement of the task description** — it contains no implementation steps,
no file inventory, no design decisions, and no test/acceptance approach. Worse, the task
as worded collides with the project's *existing* binary-naming convention (`.gitignore`
ignores `entire-agent-cairn-go`, CLAUDE.md builds to `entire-agent-cairn`), and the plan
does not reconcile that conflict — so an implementer following it literally would either
break plugin discovery or leave the repo in an inconsistent state.

### 3. Issues

**[CRITICAL] Entire plan — No actual plan; task description copied verbatim**
The plan body (lines 17–19 of the prompt) is character-for-character identical to the
task description. There are no steps, no list of files to create/modify, no decisions
(e.g., where the Go binary builds to, what the smoke test asserts), and no statement of
how the work will be verified. A review cannot confirm completeness or feasibility of a
plan that contains no plan. For a productization ticket touching build tooling, install
paths, and a git-tracked rename, the absence of a file inventory alone is disqualifying.
**Recommendation:** Return to `in_planning` and produce a concrete plan that, at minimum,
lists: (a) the exact files created (`README.md`, `Makefile`, smoke-test script path,
e.g. `test/smoke.sh` or `scripts/smoke.sh`) and modified (`.gitignore`, CLAUDE.md
Build/test/run block, `git mv` of the PoC); (b) the Makefile target set and what each
runs; (c) the smoke-test's end-to-end assertions; (d) the verification command sequence.

**[CRITICAL] Rename + build-output — Binary-naming conflict left unresolved**
The task says rename `./entire-agent-cairn` (Python PoC) → `./entire-agent-cairn-poc`
"to avoid conflict with the built Go binary." But the repo currently encodes a *different*
convention: `.gitignore` ignores `entire-agent-cairn-go` and its comment reads "the Python
PoC `./entire-agent-cairn` is tracked separately." Meanwhile CLAUDE.md's Build/test/run
block builds with `-o entire-agent-cairn`. So three sources disagree on what the Go binary
is named and what is git-ignored. Entire's plugin discovery additionally *requires* the
production binary be named exactly `entire-agent-cairn`. The plan must pick one coherent
end state and update every reference, or the rename will leave `.gitignore` ignoring a
stale name (`-go`) while the real build artifact (`entire-agent-cairn`) becomes
accidentally committable, or vice versa.
**Recommendation:** The plan should explicitly state the target end state — almost
certainly: Python PoC → `entire-agent-cairn-poc` (via `git mv`, since it is tracked);
Go binary builds to `entire-agent-cairn` (matching plugin-discovery + CLAUDE.md);
`.gitignore` updated to ignore `entire-agent-cairn` and drop the now-obsolete
`entire-agent-cairn-go` entry and its stale comment. Call out each of these edits.

**[MAJOR] Acceptance criteria — None defined or derived**
The task lists five deliverables (README, Makefile, install-path docs, PoC rename,
smoke-test script) but the plan defines no testable acceptance criteria for any of them.
"README is user-facing" and "smoke-test exercises an end-to-end session" are not
verifiable as written.
**Recommendation:** Add explicit criteria, e.g.: `make build` produces `entire-agent-cairn`;
`make test` runs `go test ./...` green; `make install` places the binary in a documented
PATH dir; `git ls-files entire-agent-cairn-poc` shows the rename landed and
`entire-agent-cairn` (Go artifact) is gitignored; the smoke script exits non-zero on
failure and verifies a `refs/cairn/sessions/<ULID>` ref is created end-to-end.

**[MAJOR] Smoke test — "real Entire CLI install" dependency and design unspecified**
The task requires a smoke test "against a real Entire CLI install." `entire` is present on
this machine (`/opt/homebrew/bin/entire`), so it is feasible here, but the plan neither
confirms the dependency nor specifies behavior when `entire` is absent (CI, other
machines), nor what the script actually drives (which hook events, what it asserts about
the resulting ref/`session.json`).
**Recommendation:** Plan should specify: a preflight check that `entire` is on PATH (skip
or fail with a clear message otherwise), the sequence of simulated/real hook events it
fires, and the concrete post-conditions it verifies (ref exists, `session.json` validates
against `cairn.session.v1`). Decide and state whether the smoke test gates CI or is
manual-only.

**[MINOR] go.mod version drift not addressed**
`go.mod` declares `go 1.26.3` while CLAUDE.md's tech stack says "Go 1.22+". Whatever the
Makefile/README state about the toolchain should be consistent with one of these.
**Recommendation:** Have the plan note the documented minimum Go version and ensure
README install instructions match `go.mod` (or intentionally reconcile the two).

**[MINOR] Install-path doc — choice between targets left open**
The task offers "~/.local/bin or /usr/local/bin" and "go install / make install" without
the plan committing to a default. Leaving this to the implementer risks README/Makefile
divergence.
**Recommendation:** Pick a primary documented path (the project already uses
`~/.local/bin` in CLAUDE.md) and have `make install` and the README agree on it, while
mentioning `go install` as the alternative.

### 4. Positive Observations

- The **underlying task is well-scoped and low-risk** — documentation, a Makefile, a
  rename, and a smoke script are independent, reversible artifacts with no architectural
  blast radius. This is a good productization ticket; the problem is purely that the plan
  was not written.
- The repository is **in good shape to support the task**: `entire` is installed locally
  (smoke test is runnable here), the subcommand surface in `cmd/entire-agent-cairn/main.go`
  is clear (`info`, `parse-hook`, the hook events, `setup-refspec`) which makes Makefile
  targets and smoke-test steps easy to define, and `lessons-learned.md` already records the
  exact rename recommendation (`entire-agent-cairn-poc`) — a strong signal the implementer
  can use.
- The task description itself **correctly identifies the binary-name collision** as the
  motivation for the rename, which is the right instinct; the plan just needs to follow
  through and reconcile it across `.gitignore` and CLAUDE.md.
