package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallOpenCodePlugin(t *testing.T) {
	root := t.TempDir()

	if err := installOpenCodePluginAt(root); err != nil {
		t.Fatalf("install: %v", err)
	}

	path := filepath.Join(root, ".opencode", "plugins", "etch.ts")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("plugin not written to %s: %v", path, err)
	}
	content := string(data)
	// The embedded plugin must be the real one: a named export that dispatches
	// to the binary, with the Buffer-based stdin redirect (a string redirect is
	// treated as a filename by Bun and silently captures nothing).
	for _, want := range []string{"export const EtchPlugin", "entire-agent-etch", "Buffer.from"} {
		if !strings.Contains(content, want) {
			t.Errorf("embedded plugin missing %q", want)
		}
	}

	// Idempotent: a second install overwrites cleanly.
	if err := installOpenCodePluginAt(root); err != nil {
		t.Fatalf("second install: %v", err)
	}

	// Uninstall removes the file and is a no-op when already absent.
	if err := uninstallOpenCodePluginAt(root); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("plugin still present after uninstall: %v", err)
	}
	if err := uninstallOpenCodePluginAt(root); err != nil {
		t.Errorf("uninstall when absent should be a no-op, got %v", err)
	}
}
