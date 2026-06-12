package enable

// Worktree stamping and post-checkout self-propagation (ETCH-48,
// docs/ENABLEMENT.md). `etch enable` stamps guarded hook entries into every
// existing worktree's .claude/settings.local.json and installs a
// marker-delimited post-checkout block into the effective hooks dir; git
// runs post-checkout on `git worktree add`, so every future worktree stamps
// itself at birth with zero per-worktree action.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Stage-11-Agentics/etch/internal/install"
)

// StampCommand builds the guarded dispatch command operator-mode stamps
// embed. Dedupe ships inside the command, not in binary-side state: when
// the worktree's branch carries committed hooks (settings.json mentions the
// binary), the stamp yields — committed entries win, exactly one capture
// per event. The shape matches the interim hand-stamps byte-for-byte, so
// pre-existing stamps are detected as already installed, never duplicated.
func StampCommand(subcommand string) string {
	return fmt.Sprintf(
		`sh -c 'if grep -qs entire-agent-etch .claude/settings.json; then exit 0; fi; if ! command -v entire-agent-etch >/dev/null 2>&1; then exit 0; fi; exec entire-agent-etch %s'`,
		subcommand,
	)
}

// Marker lines delimiting the etch-managed block in the post-checkout hook.
// Distinct from the exclude markers so a grep for one never matches the
// other's file.
const (
	postCheckoutBegin = "# >>> etch post-checkout >>>"
	postCheckoutEnd   = "# <<< etch post-checkout <<<"
)

// postCheckoutBlock is the managed hook body. It delegates to the binary
// (`stamp-worktree` owns the idempotency and the enabled-state check) and
// never breaks a checkout: missing binary or stamp failure is a silent
// no-op.
const postCheckoutBlock = postCheckoutBegin + `
if command -v entire-agent-etch >/dev/null 2>&1; then
  entire-agent-etch stamp-worktree || true
fi
` + postCheckoutEnd + "\n"

// localSettingsPath returns the operator-mode stamp file for a worktree.
func localSettingsPath(worktreeRoot string) string {
	return filepath.Join(worktreeRoot, ".claude", "settings.local.json")
}

// RunStampWorktree implements the `stamp-worktree` subcommand: stamp the
// current worktree's settings.local.json. Runs on every checkout via the
// post-checkout hook, so it is quiet, idempotent, and never fails the
// checkout: not in a git repo, or operator mode not explicitly on → silent
// exit 0. Errors go to stderr but still exit 0 (the hook block appends
// `|| true` for defense in depth).
func RunStampWorktree() error {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	root, common, ok := findWorktreeRoot(cwd)
	if !ok {
		return nil
	}
	// Stamping requires operator mode explicitly ON — an absent key means
	// team mode (or nothing), where local stamps have no business existing.
	// This is stricter than the capture guard's absent-means-enabled rule.
	if !explicitlyEnabled(cwd, common) {
		return nil
	}
	if _, err := install.InstallEntries(localSettingsPath(root), StampCommand, false); err != nil {
		fmt.Fprintf(os.Stderr, "etch: warning: stamp-worktree: %v\n", err)
	}
	return nil
}

// explicitlyEnabled reports whether etch.enabled is set to true, using the
// same zero-spawn config scan as the capture guard (git fallback when the
// scan can't answer). Absent reads as false here.
func explicitlyEnabled(dir, common string) bool {
	val, found, clean := parseConfigKey(filepath.Join(common, "config"))
	if !clean {
		out, err := gitOutput(dir, "config", "--get", "--type=bool", configKey)
		return err == nil && out == "true"
	}
	return found && gitConfigBool(val)
}

// findWorktreeRoot is findCommonDir plus the directory the .git entry was
// found in — the worktree root the stamp belongs to.
func findWorktreeRoot(dir string) (root, common string, ok bool) {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			c, cok := findCommonDir(dir)
			return dir, c, cok
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", false
		}
		dir = parent
	}
}

// listWorktrees enumerates every worktree root of the repo at dir, the main
// checkout included, via `git worktree list --porcelain`.
func listWorktrees(dir string) ([]string, error) {
	out, err := gitOutput(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("listing worktrees: %w", err)
	}
	var roots []string
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "worktree "); ok {
			roots = append(roots, rest)
		}
	}
	return roots, nil
}

// effectiveHooksDir resolves where git actually looks for hooks — honors
// core.hooksPath (husky etc.); shared by all worktrees otherwise.
func effectiveHooksDir(dir string) (string, error) {
	out, err := gitOutput(dir, "rev-parse", "--git-path", "hooks")
	if err != nil || out == "" {
		return "", fmt.Errorf("resolving hooks dir: %w", err)
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(dir, out)
	}
	return filepath.Clean(out), nil
}

// installPostCheckout writes or refreshes the managed block in the
// post-checkout hook. A fresh file gets a shebang and the executable bit; a
// pre-existing hook is chained with politely — the block is appended (or
// refreshed in place), foreign content byte-preserved, and the file is left
// executable. Returns installed=false when the existing hook can't be
// chained (non-shell or binary content) — the caller's summary must not
// claim propagation is in place.
func installPostCheckout(hooksDir string) (installed bool, err error) {
	path := filepath.Join(hooksDir, "post-checkout")

	existing, err := os.ReadFile(path) //nolint:gosec // hooks-dir derived path
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("reading %s: %w", path, err)
	}
	if len(existing) == 0 {
		if err := os.MkdirAll(hooksDir, 0o755); err != nil {
			return false, err
		}
		content := "#!/bin/sh\n" + postCheckoutBlock
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil { //nolint:gosec // git hook must be executable
			return false, err
		}
		return true, ensureExecutable(path)
	}

	// Appending sh syntax into a non-shell hook (python, node wrappers, a
	// compiled binary) would corrupt it. Leave it alone and tell the
	// operator; doctor's coverage check (ETCH-46) is the standing backstop.
	if !shFamilyHook(existing) {
		fmt.Fprintf(os.Stderr, "etch: warning: %s is not an sh-family hook; not chaining the etch block — new worktrees will need `entire-agent-etch stamp-worktree` (or run it from your hook)\n", path)
		return false, nil
	}

	updated, changed := replaceBlock(existing, postCheckoutBlock, postCheckoutBegin, postCheckoutEnd)
	if changed {
		if err := os.WriteFile(path, updated, 0o755); err != nil { //nolint:gosec // git hook must be executable
			return false, err
		}
	}
	return true, ensureExecutable(path)
}

// ensureExecutable repairs a hook file left without an exec bit —
// os.WriteFile's mode only applies at creation, so a pre-existing
// non-executable file (even an empty one) would silently never run.
func ensureExecutable(path string) error {
	if fi, err := os.Stat(path); err == nil && fi.Mode()&0o111 == 0 {
		return os.Chmod(path, fi.Mode()|0o755)
	}
	return nil
}

// shFamilyHook reports whether hook content is safe to chain sh syntax
// into: text with no shebang (git execs it via sh on ENOEXEC), or an
// sh-family interpreter (sh, bash, zsh, dash, ksh — directly or via env).
// Binary content (a compiled hook) is never chainable — appending bytes to
// a Mach-O/ELF corrupts it.
func shFamilyHook(content []byte) bool {
	if bytes.IndexByte(content, 0) >= 0 {
		return false
	}
	if !bytes.HasPrefix(content, []byte("#!")) {
		return true
	}
	first, _, _ := strings.Cut(string(content[2:]), "\n")
	fields := strings.Fields(first)
	if len(fields) == 0 {
		return false
	}
	interp := filepath.Base(fields[0])
	if interp == "env" && len(fields) > 1 {
		interp = filepath.Base(fields[1])
	}
	switch interp {
	case "sh", "bash", "zsh", "dash", "ksh":
		return true
	}
	return false
}

// removePostCheckout splices the managed block out of the post-checkout
// hook. If only the shebang (and whitespace) remains afterwards the file is
// removed entirely; foreign hook content is preserved byte-for-byte.
func removePostCheckout(hooksDir string) error {
	path := filepath.Join(hooksDir, "post-checkout")
	existing, err := os.ReadFile(path) //nolint:gosec // hooks-dir derived path
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if !bytes.Contains(existing, []byte(postCheckoutBegin)) {
		return nil // no etch block present (replaceBlock would append otherwise)
	}
	updated, _ := replaceBlock(existing, "", postCheckoutBegin, postCheckoutEnd)

	rest := strings.TrimSpace(string(updated))
	if rest == "" || rest == "#!/bin/sh" {
		return os.Remove(path)
	}
	return os.WriteFile(path, updated, 0o755) //nolint:gosec // git hook must be executable
}
