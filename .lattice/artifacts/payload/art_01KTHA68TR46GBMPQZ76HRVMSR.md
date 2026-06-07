# Validation Evidence — ETCH-34 (.etch anchored at git root; subdir/worktree coherence)

Branch fix/repo-root-batch @ f869d9d. All gates from validation-plan.md run 2026-06-07.

## Automated gates
- `go test ./...` — green, all packages (incl. new repocontext units: root, subdir,
  linked worktree, worktree subdir, non-git, bare).
- `make build` — green. `make smoke` — PASS end-to-end against real install story.
- Adversarial tests (vary cmd.Dir, the axis the bug lived on), all PASS:
  - TestHooksFromSubdirsProduceOneRecord — session_start at root, prompt from src/,
    tool_use from src/deep/, session_end from src/deep/nested/ → ONE .etch (root only),
    ONE committed ref, prompt + tool counts intact.
  - TestRootSettingsApplyFromSubdir — root .etch/settings.json redaction_patterns applied
    to a session whose hooks all fired from a subdir (security gate: redaction no longer
    silently weakens).
  - TestHooksFromLinkedWorktree — state at MAIN root, .etch never created in the
    worktree; record has is_worktree=true, worktree_path, branch=feature, the worktree
    commit in commits_produced, and files_touched diffed against the WORKTREE checkout.
  - TestOrphanRecoveredFromWorktreeSessionStart — orphan planted at main root is
    recovered by a session_start fired from inside a linked worktree.

## Live e2e (real git init temp repo, real binary)
session_start at root → prompt from src/ → tool from src/deep/ → session_end from
src/deep/nested/: `find` shows exactly one .etch (repo root); one ref
refs/etch/sessions/01KTHA1QGER3PC6C4HACDMRCZ7; session.json: status=complete,
prompt="live e2e validation", tools={Edit:1}, exit=normal. ONE coherent record. PASS.

## Design choice (documented per ticket instruction)
Linked-worktree state anchors at the MAIN repo root (git common-dir parent), shared by
all worktrees: maximal CWD-drift immunity, recovery finds worktree orphans even after
worktree deletion, one gitignored settings.json governs all checkouts. Git capture/diffs
stay anchored to the invoking checkout's toplevel (WorkDir) — keeps the deep review's
refuted "wrong checkout diff" finding refuted.