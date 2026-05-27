package capture

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// CaptureGitState reads the current git state from the given directory.
func CaptureGitState(dir string) *GitState {
	gs := &GitState{}

	gs.Branch = gitOutput(dir, "git", "rev-parse", "--abbrev-ref", "HEAD")
	gs.HeadSHA = gitOutput(dir, "git", "rev-parse", "HEAD")

	// Worktree detection
	toplevel := gitOutput(dir, "git", "rev-parse", "--show-toplevel")
	commonDir := gitOutput(dir, "git", "rev-parse", "--git-common-dir")

	if toplevel != "" {
		gs.WorktreePath = toplevel
	}

	// If --git-common-dir differs from --git-dir, we're in a worktree
	gitDir := gitOutput(dir, "git", "rev-parse", "--git-dir")
	if commonDir != "" && gitDir != "" {
		absCommon := resolvePath(dir, commonDir)
		absGitDir := resolvePath(dir, gitDir)
		gs.IsWorktree = absCommon != absGitDir
	}

	// Repo root is the common dir's parent (strips .git)
	if commonDir != "" {
		absCommon := resolvePath(dir, commonDir)
		gs.RepoRoot = filepath.Dir(absCommon)
	} else if toplevel != "" {
		gs.RepoRoot = toplevel
	}

	return gs
}

// CaptureGitEnd captures git state at session end, including commits produced since startSHA.
func CaptureGitEnd(dir, startSHA string) *GitState {
	gs := CaptureGitState(dir)
	if startSHA != "" && gs.HeadSHA != startSHA {
		gs.CommitsProduced = gitRevList(dir, startSHA)
	}
	return gs
}

func gitRevList(dir, startSHA string) []string {
	out := gitOutput(dir, "git", "rev-list", startSHA+"..HEAD")
	if out == "" {
		return nil
	}
	lines := strings.Split(out, "\n")
	var commits []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			commits = append(commits, l)
		}
	}
	return commits
}

// gitDiffFiles returns files changed between startSHA and HEAD with their actions.
func gitDiffFiles(dir, startSHA string) ([]FileEntry, error) {
	out := gitOutput(dir, "git", "diff", "--name-status", startSHA+"..HEAD")
	if out == "" {
		return nil, nil
	}
	var files []FileEntry
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		action := "modified"
		switch {
		case strings.HasPrefix(parts[0], "A"):
			action = "added"
		case strings.HasPrefix(parts[0], "D"):
			action = "deleted"
		case strings.HasPrefix(parts[0], "M"):
			action = "modified"
		case strings.HasPrefix(parts[0], "R"):
			action = "modified"
		}
		files = append(files, FileEntry{Path: parts[1], Action: action})
	}
	return files, nil
}

func gitOutput(dir string, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func resolvePath(base, rel string) string {
	if filepath.IsAbs(rel) {
		return filepath.Clean(rel)
	}
	return filepath.Clean(filepath.Join(base, rel))
}
