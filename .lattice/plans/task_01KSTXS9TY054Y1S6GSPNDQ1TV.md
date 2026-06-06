# ETCH-34: .etch dir + settings.json resolved from CWD not git root; subdir sessions ignore config

AUDIT ITEMS 1/6/7. REPRO: in a repo, run the hook sequence from a nested subdir (src/deep/nested). RESULT: .etch/ is created at src/deep/nested/.etch, NOT at the repo root.
IMPACT: (1) .etch/settings.json at the repo root is SILENTLY IGNORED for any session not started at the exact repo root -> custom redaction_patterns, raw_machine_identity, recovery_timeout_hours all stop applying (security-relevant: redaction silently weakens). (2) crash .wip files scatter into per-subdir .etch dirs; the recovery sweep (which scans <cwd>/.etch/sessions) won't find orphans created under a different cwd. (3) buffer fragmentation across the tree.
ROOT CAUSE: hooks/common.go findRepoRoot() returns os.Getwd() and never walks up. FIX: use 'git rev-parse --show-toplevel'. Verified empirically in /tmp/etch-subdir.
