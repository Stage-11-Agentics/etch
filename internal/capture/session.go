package capture

import "encoding/json"

const SchemaVersion = "etch.session.v1"

type Session struct {
	SchemaVersion   string         `json:"schema_version"`
	SessionID       string         `json:"session_id"`
	ParentSessionID *string        `json:"parent_session_id"`
	Status          string         `json:"status"`
	ExitReason      string         `json:"exit_reason"`
	Agent           AgentInfo      `json:"agent"`
	Prompt          *PromptInfo    `json:"prompt"`
	Orchestration   Orchestration  `json:"orchestration"`
	Timing          Timing         `json:"timing"`
	Machine         MachineInfo    `json:"machine"`
	Operator        OperatorInfo   `json:"operator"`
	GitStart        *GitState      `json:"git_start"`
	GitEnd          *GitState      `json:"git_end"`
	Outcome         Outcome        `json:"outcome"`
	FilesTouched    []FileEntry    `json:"files_touched"`
	Tokens          *TokenInfo     `json:"tokens"`
	ToolUse         ToolUseSummary `json:"tool_use"`
	TranscriptRef   *TranscriptRef `json:"transcript_ref"`
	C11             *C11Info       `json:"c11"`
}

type AgentInfo struct {
	Runtime string  `json:"runtime"`
	Model   *string `json:"model"`
	Version *string `json:"version"`
}

type PromptInfo struct {
	Text      string `json:"text"`
	Source    string `json:"source"`
	Truncated bool   `json:"truncated"`
}

type Orchestration struct {
	Type            string         `json:"type"`
	DispatchMethod  *string        `json:"dispatch_method"`
	TicketID        *string        `json:"ticket_id"`
	RunID           *string        `json:"run_id"`
	Role            *string        `json:"role"`
	WorkflowVersion *string        `json:"workflow_version"`
	Extra           map[string]any `json:"extra"`
}

type Timing struct {
	StartedAt  string  `json:"started_at"`
	EndedAt    *string `json:"ended_at"`
	DurationMs *int64  `json:"duration_ms"`
}

type MachineInfo struct {
	HostnameHash string  `json:"hostname_hash"`
	HostnameRaw  *string `json:"hostname_raw"`
	OS           string  `json:"os"`
	OSVersion    string  `json:"os_version"`
	Arch         string  `json:"arch"`
}

type OperatorInfo struct {
	GitUser string `json:"git_user"`
	OSUser  string `json:"os_user"`
}

type GitState struct {
	Branch       string   `json:"branch"`
	HeadSHA      string   `json:"head_sha"`
	WorktreePath string   `json:"worktree_path"`
	IsWorktree   bool     `json:"is_worktree"`
	RepoRoot     string   `json:"repo_root"`
	// Only on git_end
	CommitsProduced []string `json:"commits_produced,omitempty"`
}

type Outcome struct {
	Commits  []string `json:"commits"`
	PRNumber *int     `json:"pr_number"`
	PRState  *string  `json:"pr_state"`
	CIStatus *string  `json:"ci_status"`
}

type FileEntry struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

type TokenInfo struct {
	Input            *int64   `json:"input"`
	Output           *int64   `json:"output"`
	CacheRead        *int64   `json:"cache_read"`
	CacheWrite       *int64   `json:"cache_write"`
	APICalls         *int64   `json:"api_calls"`
	EstimatedCostUSD *float64 `json:"estimated_cost_usd"`
}

type ToolUseSummary struct {
	TotalCalls int            `json:"total_calls"`
	ByTool     map[string]int `json:"by_tool"`
}

type TranscriptRef struct {
	EntireCheckpointID *string `json:"entire_checkpoint_id"`
	LocalPath          *string `json:"local_path"`
	Available          bool    `json:"available"`
}

type C11Info struct {
	WorkspaceID string   `json:"workspace_id"`
	SurfaceID   string   `json:"surface_id"`
	TabTitle    string   `json:"tab_title"`
	PaneLineage []string `json:"pane_lineage"`
}

// HookEvent is one line in the .wip.jsonl buffer.
type HookEvent struct {
	Timestamp string          `json:"ts"`
	Hook      string          `json:"hook"`
	Data      json.RawMessage `json:"data"`
}

// SessionStartData is the data payload for a session_start event.
type SessionStartData struct {
	SessionID       string         `json:"session_id"`
	ParentSessionID *string        `json:"parent_session_id,omitempty"`
	Agent           AgentInfo      `json:"agent"`
	Orchestration   Orchestration  `json:"orchestration"`
	Machine         MachineInfo    `json:"machine"`
	Operator        OperatorInfo   `json:"operator"`
	GitState        *GitState      `json:"git_state"`
	C11             *C11Info       `json:"c11"`
	TranscriptRef   *TranscriptRef `json:"transcript_ref"`
}

// PromptData is the data payload for a user_prompt_submit event.
type PromptData struct {
	Prompt    string `json:"prompt"`
	Source    string `json:"source"`
	Truncated bool   `json:"truncated"`
}

// ToolUseData is the data payload for pre_tool_use / post_tool_use events.
type ToolUseData struct {
	ToolName string `json:"tool_name"`
	FilePath string `json:"file_path,omitempty"`
}

// SessionEndData is the data payload for session_end / stop events.
type SessionEndData struct {
	GitState   *GitState `json:"git_state"`
	ExitReason string    `json:"exit_reason"`
}
