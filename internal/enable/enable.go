// Package enable implements operator-mode enablement (docs/ENABLEMENT.md):
// the `enable`/`disable` subcommands and the fast-exit guard every hook
// entrypoint runs first. Everything `enable` writes is project-scoped and
// branchless — git config in the common dir and .git/info/exclude — so
// nothing is committed, nothing appears in any diff.
package enable

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Stage-11-Agentics/etch/internal/install"
)

// configKey is the authoritative enablement switch. It lives in the common
// config, shared by all worktrees and all branches.
const configKey = "etch.enabled"

// Marker lines delimiting the etch-managed block in .git/info/exclude.
const (
	excludeBegin = "# >>> etch >>>"
	excludeEnd   = "# <<< etch <<<"
)

// excludeBody is the managed block content: keep operator-mode files out of
// `git status` without touching the committed .gitignore. The
// !.etch/settings.json carve-out keeps the shared per-repo settings (salt)
// commitable.
var excludeBody = []string{
	".etch/*",
	"!.etch/settings.json",
	".claude/settings.local.json",
}

// HooksDisabled is the fast-exit guard. Hook entrypoints call it before
// reading stdin or touching the filesystem; true means exit 0 immediately.
//
// PreToolUse/PostToolUse fire on every tool call, so the budget is tight
// (SPEC AC #13). The common path spawns no processes at all: a directory
// walk finds the .git entry, the common dir resolves through the gitfile /
// commondir indirection, and etch.enabled is parsed straight out of the
// config file. One `git config` spawn remains as the correctness fallback
// for exotic setups (GIT_DIR env override, include directives).
//
// Semantics (ENABLEMENT.md): etch.enabled=false is an explicit off-switch
// that wins over everything. An absent key means enabled — a team-mode repo
// with committed hooks and no key must keep capturing without `etch enable`.
// Operator-mode stamps never fire without the key because `etch enable` is
// what writes both.
func HooksDisabled() bool {
	cwd, err := os.Getwd()
	if err != nil {
		return true
	}
	if os.Getenv("GIT_DIR") != "" {
		// Explicit repo override: the walk below would resolve the wrong
		// repo. Let git answer both questions (one spawn, rare path).
		out, err := gitOutput(cwd, "rev-parse", "--git-common-dir")
		if err != nil || out == "" {
			return true
		}
		return configSaysDisabledViaGit(cwd)
	}

	common, ok := findCommonDir(cwd)
	if !ok {
		return true // not inside a git repo
	}

	val, ok, clean := parseConfigKey(filepath.Join(common, "config"))
	if !clean {
		// Include directives or an unreadable file: the manual parse can't
		// answer authoritatively — fall back to git itself.
		return configSaysDisabledViaGit(cwd)
	}
	if !ok {
		return false // key absent: enabled (compatibility rule)
	}
	return !gitConfigBool(val)
}

// configSaysDisabledViaGit asks git for the effective etch.enabled value.
func configSaysDisabledViaGit(dir string) bool {
	out, err := gitOutput(dir, "config", "--get", "--type=bool", configKey)
	if err != nil {
		return false // key absent (or config unreadable): enabled
	}
	return out == "false"
}

// findCommonDir walks up from dir looking for a .git entry — a directory in
// a main checkout, a `gitdir:` file in a linked worktree — and resolves it
// to the shared common dir (following the worktree `commondir` indirection).
// Pure filesystem, no process spawn.
func findCommonDir(dir string) (string, bool) {
	for {
		gitPath := filepath.Join(dir, ".git")
		// Stat (not Lstat): a .git symlink to a directory is a setup git
		// supports, and must read as a directory here — treating it as a
		// gitfile would silently disable capture.
		if fi, err := os.Stat(gitPath); err == nil {
			gitDir := gitPath
			if !fi.IsDir() {
				// Linked worktree: .git is a file "gitdir: <path>".
				data, err := os.ReadFile(gitPath) //nolint:gosec // repo-walk derived path
				if err != nil {
					return "", false
				}
				target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(data)), "gitdir:"))
				if target == "" {
					return "", false
				}
				if !filepath.IsAbs(target) {
					target = filepath.Join(dir, target)
				}
				gitDir = filepath.Clean(target)
			}
			// Worktree git dirs carry a `commondir` pointer; the main .git
			// doesn't and is its own common dir.
			if data, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
				common := strings.TrimSpace(string(data))
				if !filepath.IsAbs(common) {
					common = filepath.Join(gitDir, common)
				}
				return filepath.Clean(common), true
			}
			return gitDir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// parseConfigKey scans a git config file for etch.enabled without spawning
// git. Returns the raw value, whether the key was found, and whether the
// parse is authoritative (clean=false when the file is unreadable or uses
// include directives the scanner doesn't follow). Handles what git's own
// writer produces — section headers, name = value lines, comments,
// case-insensitive names, last assignment wins.
func parseConfigKey(path string) (value string, found bool, clean bool) {
	data, err := os.ReadFile(path) //nolint:gosec // common-dir derived path
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, true // no config file: key definitively absent
		}
		return "", false, false
	}

	inEtch := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' {
			header, ok := sectionHeader(line)
			if !ok {
				return "", false, false // unparseable header: let git answer
			}
			section := strings.ToLower(header)
			inEtch = section == "[etch]"
			// [include] / [includeIf "..."] pull in files this scanner does
			// not follow — the answer might live there.
			if section == "[include]" || strings.HasPrefix(section, "[includeif ") {
				return "", false, false
			}
			continue
		}
		if !inEtch {
			continue
		}
		name, val, hasEq := strings.Cut(line, "=")
		if !hasEq {
			// Bare key (e.g. just "enabled") means true in git config.
			if strings.ToLower(strings.TrimSpace(name)) == "enabled" {
				value, found = "true", true
			}
			continue
		}
		if strings.ToLower(strings.TrimSpace(name)) != "enabled" {
			continue
		}
		val = strings.TrimSpace(val)
		if i := strings.IndexAny(val, "#;"); i >= 0 {
			val = strings.TrimSpace(val[:i])
		}
		val = strings.Trim(val, `"`)
		value, found = val, true // last assignment wins; keep scanning
	}
	return value, found, true
}

// sectionHeader isolates a `[section]` header from a config line, tolerating
// a trailing comment (`[etch] # managed`) and quoted subsection names that
// may contain `]`. ok=false means the line doesn't parse as a clean header.
func sectionHeader(line string) (string, bool) {
	inQuote := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\\':
			if inQuote {
				i++ // skip the escaped character
			}
		case '"':
			inQuote = !inQuote
		case ']':
			if inQuote {
				continue
			}
			rest := strings.TrimSpace(line[i+1:])
			if rest != "" && rest[0] != '#' && rest[0] != ';' {
				return "", false // trailing junk git itself would reject
			}
			return line[:i+1], true
		}
	}
	return "", false // unterminated header
}

// gitConfigBool interprets a git config boolean: false/no/off/0 are false,
// anything else (true/yes/on/1, or any junk) reads as enabled — capture is
// the safe default for a key only etch itself writes.
func gitConfigBool(v string) bool {
	switch strings.ToLower(v) {
	case "false", "no", "off", "0":
		return false
	}
	return true
}

// RunEnable implements the `enable` subcommand: set the config key in the
// shared common config and write the managed exclude block. Idempotent —
// reruns change nothing and exit 0.
func RunEnable(args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	common, err := gitCommonDir(cwd)
	if err != nil {
		return err
	}

	// Local-scope config writes land in the common config file, which every
	// worktree and branch shares (only `--worktree` writes diverge per
	// worktree, and etch never uses it).
	if _, err := gitOutput(cwd, "config", configKey, "true"); err != nil {
		return fmt.Errorf("setting %s: %w", configKey, err)
	}

	excludePath := filepath.Join(common, "info", "exclude")
	if err := writeExcludeBlock(excludePath); err != nil {
		return err
	}

	// Stamp every existing worktree, then install the post-checkout hook so
	// every future worktree stamps itself at birth (ETCH-48).
	worktrees, err := listWorktrees(cwd)
	if err != nil {
		return err
	}
	// Best-effort per worktree: a pruned/missing path or one unparseable
	// settings file must not abort enable and strand the rest unstamped.
	stamped, already, skipped := 0, 0, 0
	for _, wt := range worktrees {
		if _, err := os.Stat(wt); err != nil {
			fmt.Fprintf(os.Stderr, "etch: warning: skipping missing worktree %s\n", wt)
			skipped++
			continue
		}
		n, err := install.InstallEntries(localSettingsPath(wt), StampCommand, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "etch: warning: could not stamp %s: %v\n", wt, err)
			skipped++
			continue
		}
		if n > 0 {
			stamped++
		} else {
			already++
		}
	}

	hooksDir, err := effectiveHooksDir(cwd)
	if err != nil {
		return err
	}
	propagating, err := installPostCheckout(hooksDir)
	if err != nil {
		return err
	}

	stampLine := fmt.Sprintf("%d worktree(s) newly stamped, %d already stamped", stamped, already)
	if skipped > 0 {
		stampLine += fmt.Sprintf(", %d skipped (see warnings)", skipped)
	}
	hookLine := "post-checkout self-propagation in " + filepath.Join(hooksDir, "post-checkout")
	if !propagating {
		hookLine = "post-checkout NOT chained (existing non-shell hook — see warning; new worktrees need `entire-agent-etch stamp-worktree`)"
	}
	fmt.Printf("etch: enabled\n  %s = true (%s)\n  managed ignore block in %s\n  %s (.claude/settings.local.json)\n  %s\n",
		configKey, filepath.Join(common, "config"), excludePath, stampLine, hookLine)
	return nil
}

// RunDisable implements the `disable` subcommand. The config key is the
// immediate, repo-wide stop: the fast-exit guard silences every dispatch
// path — committed hooks and stamps alike — in the main checkout and all
// worktrees.
func RunDisable() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := gitCommonDir(cwd); err != nil {
		return err
	}
	if _, err := gitOutput(cwd, "config", configKey, "false"); err != nil {
		return fmt.Errorf("setting %s: %w", configKey, err)
	}

	// Best-effort cleanup: the config key above is the real stop (the
	// fast-exit guard gates every dispatch path); stale stamps left behind
	// are harmless, so removal failures are warnings, not errors.
	if worktrees, err := listWorktrees(cwd); err == nil {
		for _, wt := range worktrees {
			if err := install.RemoveEntries(localSettingsPath(wt)); err != nil {
				fmt.Fprintf(os.Stderr, "etch: warning: could not unstamp %s: %v\n", wt, err)
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, "etch: warning: could not list worktrees for unstamping: %v\n", err)
	}
	if hooksDir, err := effectiveHooksDir(cwd); err == nil {
		if err := removePostCheckout(hooksDir); err != nil {
			fmt.Fprintf(os.Stderr, "etch: warning: could not remove post-checkout block: %v\n", err)
		}
	}

	fmt.Printf("etch: disabled (%s = false; all capture stops everywhere in this repo)\n", configKey)
	return nil
}

// writeExcludeBlock installs or refreshes the marker-delimited etch block in
// info/exclude. Foreign content is preserved byte-for-byte; an up-to-date
// block means the file is not rewritten at all.
func writeExcludeBlock(path string) error {
	desired := excludeBegin + "\n" + strings.Join(excludeBody, "\n") + "\n" + excludeEnd + "\n"

	existing, err := os.ReadFile(path) //nolint:gosec // common-dir derived path
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	updated, changed := replaceBlock(existing, desired, excludeBegin, excludeEnd)
	if !changed {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, updated, 0o644) //nolint:gosec // git ignore-rules file
}

// replaceBlock splices the desired marker-delimited block into content:
// replaces an existing block in place, otherwise appends. Returns changed =
// false when the block is already exactly present.
func replaceBlock(content []byte, desired, beginMarker, endMarker string) ([]byte, bool) {
	begin := []byte(beginMarker)
	end := []byte(endMarker)

	bi := bytes.Index(content, begin)
	if bi >= 0 {
		rest := content[bi:]
		ei := bytes.Index(rest, end)
		if ei >= 0 {
			// Existing block spans bi .. bi+ei+len(end), plus its trailing
			// newline if present.
			stop := bi + ei + len(end)
			if stop < len(content) && content[stop] == '\n' {
				stop++
			}
			current := content[bi:stop]
			if bytes.Equal(current, []byte(desired)) {
				return content, false
			}
			var out bytes.Buffer
			out.Write(content[:bi])
			out.WriteString(desired)
			out.Write(content[stop:])
			return out.Bytes(), true
		}
		// Begin marker without end: fall through and append a clean block.
	}

	var out bytes.Buffer
	out.Write(content)
	if len(content) > 0 && !bytes.HasSuffix(content, []byte("\n")) {
		out.WriteByte('\n')
	}
	out.WriteString(desired)
	return out.Bytes(), true
}

// gitCommonDir resolves the absolute git common dir for dir, the
// branch-independent home shared by every worktree (same resolution
// capture/gitstate.go builds on).
func gitCommonDir(dir string) (string, error) {
	out, err := gitOutput(dir, "rev-parse", "--git-common-dir")
	if err != nil || out == "" {
		return "", fmt.Errorf("not inside a git repository (cwd=%s)", dir)
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(dir, out)
	}
	return filepath.Clean(out), nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
