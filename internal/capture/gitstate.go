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

// gitDiffFiles returns files changed between startSHA and endSHA with their
// actions. Bounded by the recorded SHAs (never live HEAD) so the result is
// exact even when the diff runs later than the session ended (recovery).
//
// -z parsing: paths are NUL-delimited and never quoted, so renames, paths
// with embedded tabs/newlines, and non-ASCII names (which `--name-status`
// without -z octal-escapes via core.quotePath) come through verbatim.
// Renames and copies carry TWO path tokens; a rename is recorded honestly in
// the schema's action vocabulary as {old: deleted} + {new: added}.
func gitDiffFiles(dir, startSHA, endSHA string) ([]FileEntry, error) {
	out := gitOutput(dir, "git", "diff", "--name-status", "-z", startSHA+".."+endSHA)
	if out == "" {
		return nil, nil
	}
	tokens := strings.Split(out, "\x00")
	var files []FileEntry
	for i := 0; i < len(tokens); {
		status := tokens[i]
		if status == "" {
			i++
			continue
		}
		switch status[0] {
		case 'R', 'C':
			if i+2 >= len(tokens) {
				return files, nil // truncated record — keep what parsed cleanly
			}
			oldPath, newPath := tokens[i+1], tokens[i+2]
			if status[0] == 'R' {
				files = append(files,
					FileEntry{Path: oldPath, Action: "deleted"},
					FileEntry{Path: newPath, Action: "added"})
			} else {
				files = append(files, FileEntry{Path: newPath, Action: "added"})
			}
			i += 3
		default:
			if i+1 >= len(tokens) {
				return files, nil
			}
			action := "modified"
			switch status[0] {
			case 'A':
				action = "added"
			case 'D':
				action = "deleted"
			}
			files = append(files, FileEntry{Path: tokens[i+1], Action: action})
			i += 2
		}
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
