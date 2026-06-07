package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tmpSettings(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "settings.json")
	if content != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestInstallFromScratch(t *testing.T) {
	path := tmpSettings(t, "")

	n, err := installClaudeHooks(path, false)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if n != 5 {
		t.Errorf("hooks installed: got %d, want 5", n)
	}

	installed, err := areClaudeHooksInstalled(path)
	if err != nil || !installed {
		t.Errorf("are-installed after install: got %v err %v, want true", installed, err)
	}

	data, _ := os.ReadFile(path)
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "SessionEnd"} {
		if !strings.Contains(string(data), event) {
			t.Errorf("settings missing event %s", event)
		}
	}
	// Stop must NOT be installed: etch's stop handler finalizes the session
	// and Claude Code fires Stop at every turn end.
	if strings.Contains(string(data), `"Stop"`) {
		t.Error("Stop hook must not be installed for Claude Code")
	}
	// Commands must be readable: the escape sequence \u003e must not appear
	// in place of ">".
	if strings.Contains(string(data), `\u003e`) {
		t.Error("settings should not HTML-escape command strings")
	}
}

func TestInstallIdempotent(t *testing.T) {
	path := tmpSettings(t, "")
	if _, err := installClaudeHooks(path, false); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)

	n, err := installClaudeHooks(path, false)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if n != 0 {
		t.Errorf("second install added %d hooks, want 0", n)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Error("idempotent install must not rewrite the file")
	}
}

func TestInstallPreservesForeignContent(t *testing.T) {
	foreign := `{
  "permissions": {"allow": ["Bash(ls:*)"]},
  "env": {"FOO": "bar"},
  "hooks": {
    "SessionStart": [
      {"matcher": "", "hooks": [{"type": "command", "command": "echo hello", "timeout": 30}]}
    ],
    "Notification": [
      {"matcher": "", "hooks": [{"type": "command", "command": "say ping"}]}
    ]
  }
}`
	path := tmpSettings(t, foreign)

	if _, err := installClaudeHooks(path, false); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}
	s := string(data)
	for _, want := range []string{`"Bash(ls:*)"`, `"FOO": "bar"`, `echo hello`, `"timeout": 30`, `say ping`, `"Notification"`} {
		if !strings.Contains(s, want) {
			t.Errorf("foreign content lost: %s", want)
		}
	}
	// SessionStart now carries the foreign matcher AND the etch matcher.
	hooks := m["hooks"].(map[string]any)
	ss := hooks["SessionStart"].([]any)
	if len(ss) != 2 {
		t.Errorf("SessionStart matchers: got %d, want 2", len(ss))
	}

	// Uninstall removes only etch entries; foreign survives untouched.
	if err := uninstallClaudeHooks(path); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	s = string(data)
	if strings.Contains(s, commandMarker) {
		t.Error("uninstall left etch entries behind")
	}
	for _, want := range []string{`echo hello`, `"timeout": 30`, `say ping`} {
		if !strings.Contains(s, want) {
			t.Errorf("uninstall destroyed foreign content: %s", want)
		}
	}
}

func TestForceReinstall(t *testing.T) {
	path := tmpSettings(t, "")
	if _, err := installClaudeHooks(path, false); err != nil {
		t.Fatal(err)
	}
	n, err := installClaudeHooks(path, true)
	if err != nil {
		t.Fatalf("force install: %v", err)
	}
	if n != 5 {
		t.Errorf("force reinstall: got %d, want 5", n)
	}
	installed, _ := areClaudeHooksInstalled(path)
	if !installed {
		t.Error("not installed after force reinstall")
	}
	// No duplicates.
	data, _ := os.ReadFile(path)
	if got := strings.Count(string(data), "exec entire-agent-etch session_start"); got != 1 {
		t.Errorf("session_start entries after force: got %d, want 1", got)
	}
}

func TestAreHooksInstalledPartial(t *testing.T) {
	path := tmpSettings(t, "")
	if _, err := installClaudeHooks(path, false); err != nil {
		t.Fatal(err)
	}

	// Remove one event: partial installs must read as not installed.
	data, _ := os.ReadFile(path)
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(settings["hooks"], &hooks); err != nil {
		t.Fatal(err)
	}
	delete(hooks, "SessionEnd")
	if err := writeSettings(path, settings, hooks); err != nil {
		t.Fatal(err)
	}

	installed, err := areClaudeHooksInstalled(path)
	if err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Error("partial install must read installed=false")
	}
}

func TestAreHooksInstalledNoFile(t *testing.T) {
	path := tmpSettings(t, "")
	installed, err := areClaudeHooksInstalled(path)
	if err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Error("missing settings file must read installed=false")
	}
}

func TestUninstallNoFileIsNoop(t *testing.T) {
	path := tmpSettings(t, "")
	if err := uninstallClaudeHooks(path); err != nil {
		t.Fatalf("uninstall with no file: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("uninstall must not create the settings file")
	}
}
