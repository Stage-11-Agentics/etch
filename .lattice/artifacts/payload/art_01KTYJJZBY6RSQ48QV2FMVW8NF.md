# Code Review: ETCH-48 — worktree stamping, post-checkout self-propagation, dedupe

## 1. Verdict

**PASS** — Implementation is correct, matches the plan (including all plan-review amendments), and satisfies the acceptance criteria. The minor issues below are polish items, none of which warrant rework before merge.

## 2. Summary

Reviewed the full diff: `internal/enable/stamp.go` (new, 228 lines), the `RunEnable`/`RunDisable` extensions, the `internal/install` refactor (`InstallEntries`/`RemoveEntries`), the new `stamp-worktree` subcommand, 551 lines of acceptance tests, and the doc updates (README, HOOK_CONTRACT.md, ENABLEMENT.md). Quality is high: the stamp command matches the spec'd shape byte-for-byte (verified against `docs/ENABLEMENT.md:75`), the install refactor is genuinely mechanical with zero team-mode behavior change, and every enumerated edge case from the plan has a real end-to-end test using actual `git worktree add` with the hook firing. I independently ran the full suite (`go test ./...` — all packages pass) and `go vet` (clean). The findings are all minor: a few cosmetic/edge-case gaps in the post-checkout chaining and the enable summary message.

## 3. Issues

**[MINOR] internal/enable/stamp.go:185-203 — `shFamilyHook` treats shebang-less content as chainable, which includes compiled binary hooks**
`shFamilyHook` returns true when content lacks a `#!` prefix. That is correct for plain-text sh fragments (git falls back to sh on ENOEXEC), but a compiled binary hook (Mach-O/ELF, no shebang) also passes the check, and `replaceBlock` will append shell text to it. Trailing bytes corrupt a signed Mach-O on macOS (signature invalidation kills execution), silently breaking the operator's existing hook. Rare setup, but the failure mode is corruption of a foreign file — exactly what the rest of this code is so careful to avoid.
**Fix:** Before chaining, reject content that isn't plausibly text — e.g. `if bytes.IndexByte(existing, 0) >= 0` → emit the same warn-and-skip path as the non-shell-shebang case.

**[MINOR] internal/enable/enable.go:295-296 — Enable summary claims post-checkout self-propagation even when chaining was skipped**
When `installPostCheckout` hits a non-shell shebang it warns on stderr and returns nil, but `RunEnable`'s stdout summary still prints "post-checkout self-propagation in <path>". An operator scanning the success output gets told propagation is in place when it isn't; the stderr warning is easy to miss in a noisy terminal.
**Fix:** Have `installPostCheckout` return an `installed bool` (or sentinel) and vary the summary line ("post-checkout NOT chained (non-shell hook) — see warning above").

**[MINOR] internal/enable/enable.go:295-296 — "rest already stamped" mislabels skipped/failed worktrees**
The `%d/%d worktree(s) stamped (...; rest already stamped)` line counts missing-path skips and stamp failures into "rest already stamped". After a warning like `could not stamp <wt>`, the summary actively contradicts stderr.
**Fix:** Track skipped/failed counts separately, or soften the parenthetical to "rest already stamped or skipped (see warnings)".

**[MINOR] internal/enable/stamp.go:152-157 — Pre-existing empty hook file keeps its old (possibly non-executable) mode**
The `len(existing) == 0` branch handles both "no file" and "empty file" with `os.WriteFile(..., 0o755)`, but WriteFile's mode argument only applies at creation. An existing zero-byte `post-checkout` with mode 0644 stays non-executable, and the hook silently never fires — the exact failure the chmod-repair in the non-empty branch (lines 176-178) exists to prevent.
**Fix:** Apply the same `fi.Mode()&0o111 == 0 → Chmod` repair after the empty-file write, or move that repair out of the branch so it runs unconditionally.

**[MINOR] internal/enable/stamp_test.go — Plan test #6's "plain `git checkout` rerun adds nothing" is not directly exercised**
The plan promised a test that a plain checkout (post-checkout firing again in an already-stamped worktree) changes nothing. Idempotency is covered indirectly — `TestEnableStampingIdempotent` proves `InstallEntries` is a no-op on an already-stamped file, and `RunStampWorktree` is just that call — but no test runs `git checkout` in a stamped worktree and asserts the file is byte-identical (and mtime-stable, which is what proves the no-write path).
**Fix:** Add a short case to the headline test: after the worktree-add stamp, run `gitWithBinary(t, wt, binDir, "checkout", "-b", "again")` and assert the stamp file content is unchanged.

**[MINOR] internal/enable/stamp.go:26-31 / docs — grep guard can false-positive on non-hook mentions of the binary name**
`grep -qs entire-agent-etch .claude/settings.json` yields on *any* mention, not just hook entries — e.g. a committed permissions rule like `"Bash(entire-agent-etch:*)"` in `settings.json` would make every stamp yield with no committed hooks present, silently stopping capture. This is the spec's command shape (pilot-validated, byte-match contract with the hand-stamps), so it is not an implementation defect — but it's an undocumented sharp edge.
**Fix:** No code change here; note the limitation in HOOK_CONTRACT.md's known-limitation paragraph, and fold a "settings.json mentions the binary but carries no etch hook entries" check into the ETCH-46 doctor scope.

## 4. Positive Observations

- **The dedupe contract is enforced end-to-end, not just asserted.** `TestStampYieldsToCommittedEntries` and `TestInstallHooksOnOperatorModeRepoNoDoubleCapture` run *both* installed dispatch shapes through `/bin/sh` with real payloads and count actual `.wip` files in the shared store — they test the behavior Claude Code will produce, not the implementation. Same for the headline test, which fires the real post-checkout hook via real `git worktree add` with the test binary on PATH.
- **The `install` refactor is exactly as scoped: mechanical and behavior-preserving.** `installClaudeHooks`/`uninstallClaudeHooks` become one-line delegates to the exported `InstallEntries`/`RemoveEntries`; team mode's `hookCommand` path is untouched. The byte-for-byte stamp-shape match means `matchersContainCommand` detects the 2026-06-12 hand-stamps for free — and `TestEnableStampingIdempotent` pins this with the *exact* pilot file content as a fixture, which is excellent regression armor.
- **Defensive posture is consistent and correct for each context.** `stamp-worktree` never fails a checkout (silent exit 0 with stderr-only warnings, plus `|| true` in the hook block as a second layer); `enable`'s per-worktree loop is best-effort with warnings so one pruned worktree can't strand the rest (`TestEnableSkipsMissingWorktree` verifies it doesn't resurrect deleted directories); `disable` correctly treats cleanup as best-effort because the config key is the real stop — and the test proves a stale stamp executed post-disable captures nothing.
- **`explicitlyEnabled` vs `HooksDisabled` asymmetry is deliberate and documented.** Stamping requires the key explicitly true while capture treats absent-as-enabled — the comment at stamp.go:71-73 explains why, the plan flagged it as an amendment, and `TestStampWorktreeRequiresOperatorMode` covers both absent and false.
- **The non-shell-shebang guard and `replaceBlock` generalization are clean.** Parameterizing the markers instead of duplicating the splice logic keeps one tested code path for exclude blocks and hook blocks; distinct marker strings (with the substring-collision case actually checked — neither marker is a substring of the other) avoid cross-file confusion.
- **Docs land in the right places**: README gains a concise operator-mode section, HOOK_CONTRACT.md documents the second dispatch source *and* the exec/exit-before-block limitation the plan review demanded, and ENABLEMENT.md's interim-measure section was rewritten as closed rather than deleted, preserving the byte-match rationale.
