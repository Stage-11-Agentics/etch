package install

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// openCodePlugin is the canonical OpenCode capture plugin, embedded so
// install-opencode can drop it into a repo. The same file is the source humans
// read (internal/install/opencode/etch.ts) — single source, no drift.
//
//go:embed opencode/etch.ts
var openCodePlugin string

// openCodePluginRelPath is where OpenCode auto-loads project plugins from.
// The directory is "plugins" (plural) — note this differs from the
// `opencode plugin` CLI subcommand name.
const openCodePluginRelPath = ".opencode/plugins/etch.ts"

// RunInstallOpenCode writes the OpenCode capture plugin into the repo. OpenCode
// auto-loads `.opencode/plugin/*.ts`, so this is committable repo state — the
// OpenCode analog of install-hooks writing `.claude/settings.json`. Capture
// activates only for collaborators who also have the binary on PATH (the plugin
// no-ops without it), so committing the file never forces capture on anyone.
func RunInstallOpenCode(args []string) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	if err := installOpenCodePluginAt(root); err != nil {
		return err
	}
	out, _ := json.Marshal(map[string]any{"plugin_installed": true, "path": openCodePluginRelPath})
	fmt.Println(string(out))
	return nil
}

// installOpenCodePluginAt writes the embedded plugin into root's
// .opencode/plugins/. Idempotent: overwrites any existing copy (an upgrade).
func installOpenCodePluginAt(root string) error {
	path := filepath.Join(root, openCodePluginRelPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating plugin dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(openCodePlugin), 0o644); err != nil {
		return fmt.Errorf("writing plugin: %w", err)
	}
	return nil
}

// RunUninstallOpenCode removes the OpenCode capture plugin from the repo.
func RunUninstallOpenCode() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	if err := uninstallOpenCodePluginAt(root); err != nil {
		return err
	}
	fmt.Println("{}")
	return nil
}

func uninstallOpenCodePluginAt(root string) error {
	path := filepath.Join(root, openCodePluginRelPath)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing plugin: %w", err)
	}
	return nil
}
