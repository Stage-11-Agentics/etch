package capture

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
)

// CaptureOrchestration reads CAIRN_* env vars into an Orchestration struct.
func CaptureOrchestration() Orchestration {
	o := Orchestration{
		Type:  envOrDefault("CAIRN_ORCHESTRATOR_TYPE", "manual"),
		Extra: make(map[string]any),
	}

	o.DispatchMethod = envPtr("CAIRN_DISPATCH_METHOD")
	o.TicketID = envPtr("CAIRN_TICKET_ID")
	o.RunID = envPtr("CAIRN_RUN_ID")
	o.Role = envPtr("CAIRN_AGENT_ROLE")
	o.WorkflowVersion = envPtr("CAIRN_WORKFLOW_VERSION")

	if extra := os.Getenv("CAIRN_ORCHESTRATION_EXTRA"); extra != "" {
		var parsed map[string]any
		if json.Unmarshal([]byte(extra), &parsed) == nil {
			o.Extra = parsed
		}
	}

	return o
}

// CaptureOperator reads git user and OS user.
func CaptureOperator(dir string) OperatorInfo {
	name := gitOutput(dir, "git", "config", "user.name")
	email := gitOutput(dir, "git", "config", "user.email")

	gitUser := name
	if email != "" {
		gitUser = name + " <" + email + ">"
	}

	return OperatorInfo{
		GitUser: gitUser,
		OSUser:  os.Getenv("USER"),
	}
}

// CaptureC11 reads c11 env vars. Returns nil if not in a c11 session.
func CaptureC11() *C11Info {
	wsID := os.Getenv("C11_WORKSPACE_ID")
	surfID := os.Getenv("C11_SURFACE_ID")

	if wsID == "" && surfID == "" {
		// Check legacy env vars
		wsID = os.Getenv("CMUX_WORKSPACE_ID")
		surfID = os.Getenv("CMUX_SURFACE_ID")
	}

	if wsID == "" && surfID == "" {
		return nil
	}

	info := &C11Info{
		WorkspaceID: wsID,
		SurfaceID:   surfID,
	}

	// Try to get tab title from c11 CLI
	if title := c11TabTitle(surfID); title != "" {
		info.TabTitle = title
	}

	return info
}

// CaptureTranscriptRef builds a TranscriptRef from the session_ref value.
func CaptureTranscriptRef(sessionRef string) *TranscriptRef {
	if sessionRef == "" {
		return nil
	}

	ref := &TranscriptRef{
		LocalPath: &sessionRef,
	}

	// Check if the file actually exists
	_, err := os.Stat(sessionRef)
	ref.Available = err == nil

	return ref
}

// InferRuntime tries to determine the agent runtime from available signals.
func InferRuntime() string {
	if os.Getenv("CLAUDECODE") != "" {
		return "claude-code"
	}
	if os.Getenv("CODEX_CLI") != "" {
		return "codex"
	}
	if os.Getenv("GEMINI_CLI") != "" {
		return "gemini-cli"
	}
	return "unknown"
}

// InferPromptSource guesses how the prompt was delivered.
func InferPromptSource() string {
	if os.Getenv("C11_SURFACE_ID") != "" || os.Getenv("CMUX_SURFACE_ID") != "" {
		return "c11_send"
	}

	// Check if stdin is a pipe (non-interactive)
	fi, err := os.Stdin.Stat()
	if err == nil && fi.Mode()&os.ModeCharDevice == 0 {
		return "pipe"
	}

	return "interactive"
}

func envPtr(key string) *string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	return &v
}

func envOrDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func c11TabTitle(surfaceID string) string {
	if surfaceID == "" {
		return ""
	}
	cmd := exec.Command("c11", "get-titlebar-state", "--surface", surfaceID, "--json")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	var state struct {
		Title string `json:"title"`
	}
	if json.Unmarshal(out, &state) == nil {
		return state.Title
	}
	return strings.TrimSpace(string(out))
}
