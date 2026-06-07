# Code Review (own-reviewer fallback) — refspec/sync batch, commit 3860bce vs origin/main

**Reviewer:** agent:refspec-w1-reviewer (self-review fallback — headless `lattice code-review` could not resolve the worktree diff: auto-fired runs resolve the root checkout where HEAD == origin/main, manual worktree run reported empty diff; pivoted per run instructions)

## Verdict: CHANGES REQUESTED — 1 Major, 2 Minor

### [MAJOR] Legacy etch-only push config is not healed — the original ETCH-16 victims stay broken
`configurePush` adds `HEAD` only when the push list was **empty** before the run. A repo configured by the *old* binary (or the old README manual config) has exactly `remote.<name>.push = [etch refspec]` — the precise broken state ETCH-16 describes. Rerunning the new `setup-refspec` there leaves the push list as `[etch]`: bare `git push` remains hijacked, no notice is printed (`headManaged=false`, `hasForeign=false`), and the user reasonably believes they are fixed. The fetch side heals its legacy form (no-`+` → `+`); the push side must heal its legacy form too.
**Required:** when the pre-run push list contains **no foreign refspecs** (entries other than the etch refspec / bare `HEAD`), treat the config as etch-managed: ensure both the etch refspec and `HEAD` are present. Foreign refspecs continue to block `HEAD` injection entirely. Add a regression test: pre-seed `remote.origin.push = [etch only]`, run, assert `HEAD` added and plain push carries the branch.

### [MINOR] `configurePush` comment/flag conflation
The `headManaged` derivation plus its comment ("Report HEAD as etch-managed on reruns…") encodes three states in two booleans through non-obvious boolean algebra. The Major fix collapses this naturally (no-foreign → manage both entries; foreign → etch only). Restructure rather than patch.

### [MINOR] `--remote` flag value can swallow a flag-like token
`--remote --foo` parses `--foo` as a remote name and fails later with `remote "--foo" not found` — confusing but safe and self-explanatory at the error site. Acceptable; no change required.

## Verified sound
- Root-cause analysis correct: `remote.<name>.push` presence replaces git's implicit default; fetch entries are augmentative because clone materializes the default. `HEAD` augmentation validated by passing regression test (plain push carries branch + etch refs) and two-clone round-trip test.
- Remote selection (ETCH-18/38): phantom URL-less remotes never configured, errors actionable, `--remote`/`--remote=` both parse, unknown args rejected. Config sweep catches config-section-only phantoms `git remote` omits.
- Fetch `+` upgrade is exact-match anchored (`--unset-all` with regex-escaped pattern), idempotent, cannot touch the `+` form or non-etch entries.
- No shell-interpolation risk: remote names pass as argv to `exec.Command`.
- README claims each map to a passing test; clone section commands mirror `TestSetupRefspecSecondCloneRoundTrip` exactly.
- Existing `TestE2ESetupRefspec` (substring assertions) unaffected. `go test ./...` green, `make build` clean, rebased onto ec406c7 cleanly with no overlap into the redaction batch's files.