package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// StdinEvent is the JSON structure Entire sends on stdin for hook invocations.
type StdinEvent struct {
	SessionID  string          `json:"session_id"`
	SessionRef string          `json:"session_ref"`
	Timestamp  string          `json:"timestamp"`
	UserPrompt string          `json:"user_prompt"`
	ToolName   string          `json:"tool_name"`
	ToolUseID  string          `json:"tool_use_id"`
	ToolInput  json.RawMessage `json:"tool_input"`
	RawData    json.RawMessage `json:"raw_data"`
}

func readStdin() (*StdinEvent, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
	var ev StdinEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return nil, fmt.Errorf("parsing stdin JSON: %w", err)
	}
	return &ev, nil
}

func printOK() {
	fmt.Println(`{"ok":true}`)
}

// findRepoRoot returns the git repo root for the current directory.
func findRepoRoot() string {
	dir, _ := os.Getwd()
	return dir
}
