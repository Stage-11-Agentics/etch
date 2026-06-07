# Plan Review: ETCH-38

## 1. Verdict

**FAIL (plan-level)**

## 2. Summary

I reviewed the proposed plan for ETCH-38 (setup-refspec omits the leading `+`
on the fetch refspec and hard-codes `remote.origin`). The underlying bug is
real and I confirmed it against the actual source — but the submitted "Plan" is
a **verbatim copy of the task description**, not an implementation plan. It
contains no approach, resolves none of the either/or decisions the task itself
poses, names no files, and proposes no tests. There is nothing here to review
as a plan, so it must return to `in_planning`.

## 3. Issues

**[CRITICAL] Whole plan — No actual plan was submitted**
The "Plan" section (lines 17–19 of the prompt) is byte-for-byte identical to the
"Task Description" (line 14), minus the closing sentence. It restates the
*problem* but never states the *solution*: no description of what code changes
will be made, which behavior is chosen at each fork in the road, what files are
touched, or how the fix is verified. A plan that only re-states the ticket gives
the implementer zero guidance and gives review nothing to validate against.
**Recommendation:** Author a real plan. At minimum it must contain: (a) the
chosen resolution for the `+` divergence, (b) the chosen resolution for the
hard-coded remote, (c) the exact files to modify, and (d) the test additions.
See the issues below for the specific decisions that must be nailed down.

**[MAJOR] Decision unresolved — `+` divergence: fix code or fix README?**
The task offers two mutually exclusive directions ("align the `+` with README
**or** fix README") and the plan picks neither. I confirmed the current state:
`internal/commands/setup_refspec.go:10` defines a single
`etchRefspec = "refs/etch/sessions/*:refs/etch/sessions/*"` (no `+`) and applies
it to **both** push and fetch (lines 13–16). `README.md:88-89` shows fetch
**with** `+` and push **without**. So push already matches README; only fetch
diverges. Because both directions are "harmless for immutable refs," the choice
is a real judgment call that the plan must make explicitly — otherwise the
implementer guesses and review can't tell intent from accident.
**Recommendation:** State the decision. Recommended: add `+` to the **fetch**
refspec only (matches README and standard git convention that fetch refspecs are
force-able), leaving push without `+`. That means the code can no longer share
one `etchRefspec` constant across both directions — call this out, since
`addRefspecIfMissing` currently relies on a single constant for both the
idempotency check (line 32) and the `--add` (line 37).

**[MAJOR] Decision unresolved — remote handling: detect, parameterize, or warn?**
The task offers three options ("detect/parameterize the remote name **or** warn
when origin is absent") and the plan commits to none. The current code
unconditionally writes `remote.origin.<dir>` (`setup_refspec.go:24`); on a repo
whose only remote is `forgejo` (which is literally this project's own remote per
CLAUDE.md / README), the command silently configures a remote that does not
exist and exits 0 — the exact failure the ticket describes. The chosen behavior
materially changes the surface area (a new CLI flag and arg parsing vs. a
read-only warning) and the test matrix.
**Recommendation:** State the decision and its scope. A reasonable minimal
approach that satisfies the ticket without scope creep: (1) detect the remote —
if exactly one remote exists use it; if multiple exist, prefer `origin`,
otherwise require an explicit choice; (2) if the resolved remote does not exist
(e.g. no `origin` and none specified), **warn loudly and exit non-zero** rather
than writing dead config. Decide explicitly whether a `--remote <name>` flag is
in scope or deferred; if deferred, say so.

**[MAJOR] No test plan — violates project testing mandate**
CLAUDE.md states "Every ticket ships with tests. No exceptions," yet there is no
test plan and I confirmed there is currently **no `setup_refspec_test.go`** in
`internal/commands/`. A fix that changes refspec strings and remote-resolution
logic is trivially testable with a temp git repo (the project's own documented
pattern), so its absence from the plan is a gap that will surface at code review.
**Recommendation:** The plan must enumerate tests: (a) fetch refspec is written
with `+` and push without; (b) idempotency still holds — re-running does not
duplicate entries and correctly matches the now-`+`-prefixed fetch line; (c)
single non-`origin` remote is configured correctly; (d) no remote / missing
`origin` produces a warning + non-zero exit and writes no config; (e)
multiple-remote behavior matches the chosen rule.

**[MINOR] Idempotency check will break silently if `+` is added naively**
Worth pre-empting in the plan: `addRefspecIfMissing` compares each existing
config line against the single `etchRefspec` constant (line 32). If fetch starts
emitting `+refs/...` but the comparison constant is not updated in lockstep, the
"already present" check stops matching and re-running the command appends a
duplicate fetch line every invocation.
**Recommendation:** Have the plan note that the match string and the written
string must be the same per-direction value, and cover it with test (b) above.

## 4. Positive Observations

The ticket it is built on is excellent: the bug is precisely diagnosed, the
"harmless for immutable refs" caveat shows correct understanding of why this is a
divergence rather than a live break, and the fix was **empirically verified**
(`/tmp/etch-refspec`, push/fetch/content-match PASS) before being filed. That
empirical grounding is exactly the rigor this project asks for. The problem is
purely that this strong *diagnosis* was submitted in the plan slot without being
turned into a *plan* — once the three decisions above are resolved in writing,
this should be a quick, low-risk pass.
