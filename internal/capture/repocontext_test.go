package capture

import (
	"os"
	"path/filepath"
	"testing"
)

// realPath resolves symlinks so paths from git (physical) compare equal to
// t.TempDir() paths (logical, e.g. /var -> /private/var on macOS).
func realPath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", p, err)
	}
	return r
}

func TestResolveRepoContextAtRoot(t *testing.T) {
	dir := newTestGitRepo(t)

	rc, err := ResolveRepoContext(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := realPath(t, dir)
	if realPath(t, rc.StateRoot) != want {
		t.Errorf("StateRoot: got %s, want %s", rc.StateRoot, want)
	}
	if realPath(t, rc.WorkDir) != want {
		t.Errorf("WorkDir: got %s, want %s", rc.WorkDir, want)
	}
}

func TestResolveRepoContextFromSubdir(t *testing.T) {
	dir := newTestGitRepo(t)
	sub := filepath.Join(dir, "src", "deep", "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	rc, err := ResolveRepoContext(sub)
	if err != nil {
		t.Fatal(err)
	}
	want := realPath(t, dir)
	if realPath(t, rc.StateRoot) != want {
		t.Errorf("StateRoot from subdir: got %s, want %s", rc.StateRoot, want)
	}
	if realPath(t, rc.WorkDir) != want {
		t.Errorf("WorkDir from subdir: got %s, want %s", rc.WorkDir, want)
	}
}

func TestResolveRepoContextLinkedWorktree(t *testing.T) {
	dir := newTestGitRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	gitCmd(t, dir, "worktree", "add", wt, "-b", "feature")

	wantState := realPath(t, dir)
	wantWork := realPath(t, wt)

	// From the worktree toplevel
	rc, err := ResolveRepoContext(wt)
	if err != nil {
		t.Fatal(err)
	}
	if realPath(t, rc.StateRoot) != wantState {
		t.Errorf("StateRoot from worktree: got %s, want main root %s", rc.StateRoot, wantState)
	}
	if realPath(t, rc.WorkDir) != wantWork {
		t.Errorf("WorkDir from worktree: got %s, want %s", rc.WorkDir, wantWork)
	}

	// From a subdir inside the worktree
	sub := filepath.Join(wt, "pkg")
	os.MkdirAll(sub, 0o755)
	rc, err = ResolveRepoContext(sub)
	if err != nil {
		t.Fatal(err)
	}
	if realPath(t, rc.StateRoot) != wantState {
		t.Errorf("StateRoot from worktree subdir: got %s, want main root %s", rc.StateRoot, wantState)
	}
	if realPath(t, rc.WorkDir) != wantWork {
		t.Errorf("WorkDir from worktree subdir: got %s, want %s", rc.WorkDir, wantWork)
	}
}

func TestResolveRepoContextNonGit(t *testing.T) {
	dir := t.TempDir()
	_, err := ResolveRepoContext(dir)
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
}

func TestResolveRepoContextBareRepo(t *testing.T) {
	dir := t.TempDir()
	gitCmd(t, dir, "init", "--bare")
	_, err := ResolveRepoContext(dir)
	if err == nil {
		t.Fatal("expected error for bare repo (no usable worktree)")
	}
}
