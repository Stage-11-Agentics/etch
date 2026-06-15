package hooks

import (
	"encoding/json"
	"testing"
)

func TestExtractFilePathAcrossRuntimes(t *testing.T) {
	cases := []struct {
		name  string
		tool  string
		input string
		want  string
	}{
		{"claude write file_path", "Write", `{"file_path":"/a/b.go","content":"x"}`, "/a/b.go"},
		{"claude read path fallback", "Read", `{"path":"/a/c.go"}`, "/a/c.go"},
		// OpenCode tool names are lowercase; the plugin normalizes filePath →
		// file_path, so etch must match the lowercase tool name to read it.
		{"opencode write lowercase", "write", `{"file_path":"/a/d.go","content":"x"}`, "/a/d.go"},
		{"opencode edit lowercase", "edit", `{"file_path":"/a/e.go"}`, "/a/e.go"},
		{"multiedit", "MultiEdit", `{"file_path":"/a/f.go"}`, "/a/f.go"},
		{"non-file tool ignored", "bash", `{"command":"ls /etc"}`, ""},
		{"unknown tool", "Grep", `{"path":"/should/not/count"}`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractFilePath(c.tool, json.RawMessage(c.input))
			if got != c.want {
				t.Errorf("extractFilePath(%q): got %q want %q", c.tool, got, c.want)
			}
		})
	}
}
