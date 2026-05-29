package capture

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
)

// CaptureOrchestration reads ETCH_* env vars into an Orchestration struct.
func CaptureOrchestration() Orchestration {
	o := Orchestration{
		Type:  envOrDefault("ETCH_ORCHESTRATOR_TYPE", "manual"),
		Extra: make(map[string]any),
	}

	o.DispatchMethod = envPtr("ETCH_DISPATCH_METHOD")
	o.TicketID = envPtr("ETCH_TICKET_ID")
	o.RunID = envPtr("ETCH_RUN_ID")
	o.Role = envPtr("ETCH_AGENT_ROLE")
	o.WorkflowVersion = envPtr("ETCH_WORKFLOW_VERSION")

	if extra := os.Getenv("ETCH_ORCHESTRATION_EXTRA"); extra != "" {
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
//
// pane_lineage is built from ETCH_PANE_LINEAGE (a JSON array of ancestor tab
// titles set by the spawning orchestrator) with the current tab title appended.
// Solo sessions get a single-element lineage of their own title.
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

	if title := c11TabTitle(surfID); title != "" {
		info.TabTitle = title
	}

	info.PaneLineage = buildPaneLineage(info.TabTitle)

	return info
}

// buildPaneLineage returns the chain of tab titles from the root orchestrator
// to the current pane. The parent chain is read from ETCH_PANE_LINEAGE
// (JSON array); the current tab title is appended.
func buildPaneLineage(currentTitle string) []string {
	var lineage []string

	if raw := os.Getenv("ETCH_PANE_LINEAGE"); raw != "" {
		var parsed []string
		if json.Unmarshal([]byte(raw), &parsed) == nil {
			lineage = parsed
		}
	}

	if currentTitle != "" {
		lineage = append(lineage, currentTitle)
	}

	return lineage
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
