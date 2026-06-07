package capture

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// RepoContext anchors hook state and git operations for one hook invocation.
//
// StateRoot is the main repo root — the parent of `git rev-parse --git-common-dir` —
// shared by all linked worktrees. All .etch state (wip buffers, ULID mappings,
// settings.json), the recovery scan, and ref writes anchor here so hooks firing from
// any CWD inside the repo (root, subdir, linked worktree) resolve the same state.
//
// WorkDir is the toplevel of the invoking checkout (`git rev-parse --show-toplevel`),
// linked-worktree aware. Git state capture and diffs anchor here so a worktree session
// records its own branch/SHA and diffs its own checkout.
//
// Known limitation: inside a submodule, --git-common-dir points into the superproject's
// .git/modules/<name>, so StateRoot would not be a checkout root there. Submodule
// sessions are out of scope.
type RepoContext struct {
	StateRoot string
	WorkDir   string
}

// ResolveRepoContext resolves both roots from dir (the hook process CWD).
// Returns an error when dir is not inside a usable git repository or git cannot run;
// the error wraps git's own stderr so a missing git binary is distinguishable from a
// non-repo directory.
func ResolveRepoContext(dir string) (*RepoContext, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel", "--git-common-dir")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("git rev-parse: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("running git rev-parse: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 || strings.TrimSpace(lines[0]) == "" || strings.TrimSpace(lines[1]) == "" {
		// Bare repos and other degenerate states yield no toplevel — not usable.
		return nil, fmt.Errorf("git rev-parse returned no usable worktree (bare repo?): %q", strings.TrimSpace(string(out)))
	}

	workDir := strings.TrimSpace(lines[0])
	// Git emits --git-common-dir relative to the CWD (e.g. ".git" at the toplevel).
	commonDir := resolvePath(dir, strings.TrimSpace(lines[1]))

	return &RepoContext{
		StateRoot: filepath.Dir(commonDir),
		WorkDir:   workDir,
	}, nil
}
