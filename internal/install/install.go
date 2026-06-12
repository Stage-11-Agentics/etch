// Package install wires Etch's capture hooks into the agent runtime's hook
// configuration. This is the install side of Entire's external-agent protocol:
// when an operator runs `entire enable --agent etch`, Entire discovers this
// binary and invokes `install-hooks` — and it is THIS code, not Entire, that
// writes the dispatch entries. The same subcommand also works standalone
// (no Entire involved): `entire-agent-etch install-hooks`.
//
// Today only Claude Code is wired (.claude/settings.json at the repo root).
// Installed hooks dispatch directly to this binary with the runtime's native
// hook JSON on stdin — Entire is not in the runtime dispatch path.
package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// commandMarker identifies etch-managed hook entries inside foreign config.
const commandMarker = "entire-agent-etch"

// claudeEvent maps a Claude Code hook event to the etch subcommand it drives.
type claudeEvent struct {
	Event      string // Claude Code hooks key, e.g. "SessionStart"
	Subcommand string // etch hook subcommand, e.g. "session_start"
	Matcher    string // Claude Code matcher ("" = all; "*" for tool events)
}

// claudeEvents are the hooks Etch installs for Claude Code.
//
// Stop is deliberately NOT installed: etch's `stop` handler finalizes the
// session, and Claude Code fires Stop at every turn end — installing it would
// truncate multi-turn sessions at the first turn. Claude Code has a real
// SessionEnd, which is the finalizer. (`stop` remains available for runtimes
// that lack a session-end hook.)
var claudeEvents = []claudeEvent{
	{Event: "SessionStart", Subcommand: "session_start", Matcher: ""},
	{Event: "UserPromptSubmit", Subcommand: "user_prompt_submit", Matcher: ""},
	{Event: "PreToolUse", Subcommand: "pre_tool_use", Matcher: "*"},
	{Event: "PostToolUse", Subcommand: "post_tool_use", Matcher: "*"},
	{Event: "SessionEnd", Subcommand: "session_end", Matcher: ""},
}

// hookCommand builds the guarded dispatch command for one etch subcommand.
// The guard mirrors Entire's own installer: if the binary is missing from
// PATH the hook exits 0 so the agent session is never broken.
func hookCommand(subcommand string) string {
	return fmt.Sprintf(
		`sh -c 'if ! command -v entire-agent-etch >/dev/null 2>&1; then exit 0; fi; exec entire-agent-etch %s'`,
		subcommand,
	)
}

// repoRoot resolves the git worktree root, falling back to the cwd.
func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		if root := strings.TrimSpace(string(out)); root != "" {
			return root, nil
		}
	}
	return os.Getwd()
}

// settingsPath returns the Claude Code project settings file for the repo.
func settingsPath() (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", fmt.Errorf("resolving repo root: %w", err)
	}
	return filepath.Join(root, ".claude", "settings.json"), nil
}

// RunInstallHooks implements the `install-hooks` subcommand.
// Flags: --force removes existing etch entries before installing;
// --local-dev is accepted for Entire protocol compatibility and ignored
// (etch has no local-dev dispatch variant).
// Prints {"hooks_installed": N} — the shape Entire's
// HooksInstalledCountResponse unmarshals (v0.6.3 types.go:63-65).
func RunInstallHooks(args []string) error {
	force := false
	for _, a := range args {
		switch a {
		case "--force":
			force = true
		case "--local-dev":
			// accepted, no-op
		}
	}

	path, err := settingsPath()
	if err != nil {
		return err
	}

	n, err := installClaudeHooks(path, force)
	if err != nil {
		return err
	}
	fmt.Printf(`{"hooks_installed":%d}`+"\n", n)
	return nil
}

// RunUninstallHooks implements the `uninstall-hooks` subcommand.
// Entire ignores stdout for this call (v0.6.3 external.go:273-276); `{}` is
// emitted for symmetry.
func RunUninstallHooks() error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	if err := uninstallClaudeHooks(path); err != nil {
		return err
	}
	fmt.Println("{}")
	return nil
}

// RunAreHooksInstalled implements the `are-hooks-installed` subcommand.
// Prints {"installed": bool} — Entire's AreHooksInstalledResponse
// (v0.6.3 types.go:68-70). Installed means ALL required events carry an etch
// entry, so partial installs read false and a re-run repairs them.
func RunAreHooksInstalled() error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	installed, err := areClaudeHooksInstalled(path)
	if err != nil {
		return err
	}
	fmt.Printf(`{"installed":%t}`+"\n", installed)
	return nil
}

// RunDetect implements the `detect` subcommand. Prints {"present": true} —
// Entire's DetectResponse (v0.6.3 types.go:33-35). The binary being invoked
// is itself the presence signal.
func RunDetect() error {
	fmt.Println(`{"present":true}`)
	return nil
}

// --- Claude Code settings.json surgery -------------------------------------
//
// The file is shared, user-owned state. Everything that is not an
// etch-managed hook entry must round-trip byte-preserved, so all parsing is
// map[string]json.RawMessage down to the level we actually modify.

// installClaudeHooks adds etch hook entries, returns how many were added.
func installClaudeHooks(path string, force bool) (int, error) {
	return InstallEntries(path, hookCommand, force)
}

// InstallEntries adds one etch hook entry per Claude Code event to the
// settings file at path, with the dispatch command built by cmdFor. This is
// the shared merge engine for team mode (committed settings.json, plain
// guard) and operator mode (settings.local.json stamps, dedupe guard) —
// idempotent, and everything that is not an etch entry round-trips
// byte-preserved. Returns how many entries were added.
func InstallEntries(path string, cmdFor func(subcommand string) string, force bool) (int, error) {
	settings, hooks, err := readSettings(path)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, ce := range claudeEvents {
		matchers, err := parseMatchers(hooks[ce.Event])
		if err != nil {
			return 0, fmt.Errorf("parsing hooks.%s: %w", ce.Event, err)
		}
		if force {
			matchers = removeEtchEntries(matchers)
		}
		cmd := cmdFor(ce.Subcommand)
		if matchersContainCommand(matchers, cmd) {
			continue
		}
		entry, err := marshalNoEscape(map[string]any{
			"matcher": ce.Matcher,
			"hooks": []map[string]string{
				{"type": "command", "command": cmd},
			},
		})
		if err != nil {
			return 0, err
		}
		matchers = append(matchers, entry)
		count++

		raw, err := marshalNoEscape(matchers)
		if err != nil {
			return 0, err
		}
		hooks[ce.Event] = raw
	}

	if count == 0 && !force {
		return 0, nil // nothing changed; don't rewrite the file
	}
	return count, writeSettings(path, settings, hooks)
}

// uninstallClaudeHooks removes every etch-managed entry.
func uninstallClaudeHooks(path string) error {
	return RemoveEntries(path)
}

// RemoveEntries removes every etch-managed hook entry from the settings
// file at path, preserving foreign content byte-for-byte. A missing file is
// a no-op.
func RemoveEntries(path string) error {
	settings, hooks, err := readSettings(path)
	if err != nil {
		return err
	}
	if len(hooks) == 0 {
		return nil
	}
	changed := false
	for event, raw := range hooks {
		if !bytes.Contains(raw, []byte(commandMarker)) {
			continue
		}
		matchers, err := parseMatchers(raw)
		if err != nil {
			return fmt.Errorf("parsing hooks.%s: %w", event, err)
		}
		cleaned := removeEtchEntries(matchers)
		if len(cleaned) == len(matchers) {
			continue
		}
		changed = true
		if len(cleaned) == 0 {
			delete(hooks, event)
			continue
		}
		out, err := marshalNoEscape(cleaned)
		if err != nil {
			return err
		}
		hooks[event] = out
	}
	if !changed {
		return nil
	}
	return writeSettings(path, settings, hooks)
}

// areClaudeHooksInstalled reports whether every required event has an etch entry.
func areClaudeHooksInstalled(path string) (bool, error) {
	_, hooks, err := readSettings(path)
	if err != nil {
		return false, err
	}
	for _, ce := range claudeEvents {
		matchers, err := parseMatchers(hooks[ce.Event])
		if err != nil || !matchersContainCommand(matchers, hookCommand(ce.Subcommand)) {
			return false, nil
		}
	}
	return true, nil
}

func readSettings(path string) (map[string]json.RawMessage, map[string]json.RawMessage, error) {
	settings := map[string]json.RawMessage{}
	data, err := os.ReadFile(path) //nolint:gosec // repo-root derived path
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return nil, nil, fmt.Errorf("parsing %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, nil, err
	}

	hooks := map[string]json.RawMessage{}
	if raw, ok := settings["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return nil, nil, fmt.Errorf("parsing hooks in %s: %w", path, err)
		}
	}
	return settings, hooks, nil
}

func writeSettings(path string, settings, hooks map[string]json.RawMessage) error {
	if len(hooks) == 0 {
		delete(settings, "hooks")
	} else {
		raw, err := marshalNoEscape(hooks)
		if err != nil {
			return err
		}
		settings["hooks"] = raw
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(settings); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644) //nolint:gosec // shared project settings file
}

// marshalNoEscape is json.Marshal without HTML escaping, so shell commands
// with ">" and "&" stay readable in the user-owned settings file.
func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// parseMatchers decodes one event's matcher list, preserving each matcher
// object as raw JSON.
func parseMatchers(raw json.RawMessage) ([]json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var matchers []json.RawMessage
	if err := json.Unmarshal(raw, &matchers); err != nil {
		return nil, err
	}
	return matchers, nil
}

// matchersContainCommand reports whether any matcher carries the exact
// command. Comparison is on decoded strings, so JSON escaping differences
// (e.g. ">" vs ">") don't matter.
func matchersContainCommand(matchers []json.RawMessage, cmd string) bool {
	for _, m := range matchers {
		var obj struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		}
		if json.Unmarshal(m, &obj) != nil {
			continue
		}
		for _, h := range obj.Hooks {
			if h.Command == cmd {
				return true
			}
		}
	}
	return false
}

// removeEtchEntries filters etch-managed hook entries out of a matcher list.
// Foreign matchers (and foreign entries inside mixed matchers) are preserved
// byte-for-byte; only matchers we touch are re-serialized.
func removeEtchEntries(matchers []json.RawMessage) []json.RawMessage {
	var out []json.RawMessage
	for _, m := range matchers {
		if !bytes.Contains(m, []byte(commandMarker)) {
			out = append(out, m)
			continue
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(m, &obj); err != nil {
			out = append(out, m) // unparseable: leave untouched
			continue
		}
		var entries []json.RawMessage
		if err := json.Unmarshal(obj["hooks"], &entries); err != nil {
			out = append(out, m)
			continue
		}
		var kept []json.RawMessage
		for _, e := range entries {
			if !bytes.Contains(e, []byte(commandMarker)) {
				kept = append(kept, e)
			}
		}
		if len(kept) == 0 {
			continue // matcher held only etch entries: drop it
		}
		raw, err := marshalNoEscape(kept)
		if err != nil {
			out = append(out, m)
			continue
		}
		obj["hooks"] = raw
		rebuilt, err := marshalNoEscape(obj)
		if err != nil {
			out = append(out, m)
			continue
		}
		out = append(out, rebuilt)
	}
	return out
}
