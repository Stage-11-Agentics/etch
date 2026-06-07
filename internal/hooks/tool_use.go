package hooks

import (
	"encoding/json"

	"forgejo.stage11.ai/s11/etch/internal/capture"
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
		ToolName: ev.ToolName,
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

func extractFilePath(toolName string, input json.RawMessage) string {
	switch toolName {
	case "Read", "Write", "Edit":
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
