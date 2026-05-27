package parsehook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

type hookInput struct {
	SessionID  string          `json:"session_id"`
	SessionRef string          `json:"session_ref"`
	Timestamp  string          `json:"timestamp"`
	UserPrompt string          `json:"user_prompt"`
	ToolName   string          `json:"tool_name"`
	ToolUseID  string          `json:"tool_use_id"`
	ToolInput  json.RawMessage `json:"tool_input"`
	RawData    json.RawMessage `json:"raw_data"`
}

type rawData struct {
	Model string `json:"model"`
}

type Result struct {
	HookType  string `json:"hook_type"`
	SessionID string `json:"session_id"`
	Timestamp string `json:"timestamp"`

	// Hook-specific fields (omitted when empty)
	Model      string          `json:"model,omitempty"`
	Prompt     string          `json:"prompt,omitempty"`
	SessionRef string          `json:"session_ref,omitempty"`
	ToolName   string          `json:"tool_name,omitempty"`
	ToolUseID  string          `json:"tool_use_id,omitempty"`
	ToolInput  json.RawMessage `json:"tool_input,omitempty"`
}

func Run(args []string) error {
	hookName := extractHookFlag(args)
	if hookName == "" {
		return fmt.Errorf("parse-hook: --hook flag is required")
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("parse-hook: reading stdin: %w", err)
	}

	var input hookInput
	if err := json.Unmarshal(data, &input); err != nil {
		return fmt.Errorf("parse-hook: invalid JSON: %w", err)
	}

	ts := input.Timestamp
	if ts == "" {
		ts = time.Now().UTC().Format(time.RFC3339)
	}

	result := Result{
		HookType:  normalizeHookName(hookName),
		SessionID: input.SessionID,
		Timestamp: ts,
	}

	switch normalizeHookName(hookName) {
	case "session_start":
		if input.RawData != nil {
			var rd rawData
			if json.Unmarshal(input.RawData, &rd) == nil {
				result.Model = rd.Model
			}
		}
		result.SessionRef = input.SessionRef
	case "session_end", "stop":
		result.SessionRef = input.SessionRef
	case "user_prompt_submit":
		result.Prompt = input.UserPrompt
	case "pre_tool_use", "post_tool_use":
		result.ToolName = input.ToolName
		result.ToolUseID = input.ToolUseID
		result.ToolInput = input.ToolInput
	}

	out, err := json.Marshal(result)
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func extractHookFlag(args []string) string {
	for i, a := range args {
		if a == "--hook" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// normalizeHookName converts hyphenated hook names to underscore form.
func normalizeHookName(name string) string {
	switch name {
	case "session-start":
		return "session_start"
	case "session-end":
		return "session_end"
	case "user-prompt-submit":
		return "user_prompt_submit"
	case "pre-tool-use":
		return "pre_tool_use"
	case "post-tool-use":
		return "post_tool_use"
	default:
		return name
	}
}
