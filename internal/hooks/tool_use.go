package hooks

import (
	"encoding/json"
	"strings"

	"github.com/Stage-11-Agentics/etch/internal/capture"
)

func RunPreToolUse() error {
	return runToolUse("pre_tool_use")
}

func RunPostToolUse() error {
	return runToolUse("post_tool_use")
}

func runToolUse(hookName string) error {
	ev, err := readStdin()
	if err != nil {
		return err
	}

	rc, err := resolveContext()
	if err != nil {
		return err
	}

	sessionID := capture.LookupMapping(rc.StateRoot, ev.SessionID)
	if sessionID == "" {
		printOK()
		return nil
	}

	if ev.ToolName == "" {
		warnMissing(hookName, "tool_name", ev.payloadKeys)
	}

	data := capture.ToolUseData{
		ToolName:  ev.ToolName,
		ToolUseID: ev.ToolUseID,
	}

	// Extract file path from tool input for Read/Write/Edit tools
	if ev.ToolInput != nil {
		data.FilePath = extractFilePath(ev.ToolName, ev.ToolInput)
	}

	if err := capture.AppendEvent(rc.StateRoot, sessionID, hookName, data); err != nil {
		return err
	}

	printOK()
	return nil
}

// extractFilePath pulls the edited/read file path out of a tool's input for
// the file-touching tools. The tool name is matched case-insensitively so this
// works across runtimes whose tool names differ only in casing — Claude Code's
// Read/Write/Edit and OpenCode's read/write/edit are the same tools.
func extractFilePath(toolName string, input json.RawMessage) string {
	switch strings.ToLower(toolName) {
	case "read", "write", "edit", "multiedit", "notebookedit":
		var ti struct {
			FilePath string `json:"file_path"`
			Path     string `json:"path"`
		}
		if json.Unmarshal(input, &ti) == nil {
			if ti.FilePath != "" {
				return ti.FilePath
			}
			return ti.Path
		}
	}
	return ""
}
