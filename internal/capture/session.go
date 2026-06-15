package capture

import "encoding/json"

const SchemaVersion = "etch.session.v1"

type Session struct {
	SchemaVersion   string         `json:"schema_version"`
	SessionID       string         `json:"session_id"`
	// AgentSessionID is the upstream runtime's own session id (from the hook
	// payload); null when the runtime supplied none. Etch's minted ULID
	// (SessionID) stays canonical for refs.
	AgentSessionID  *string        `json:"agent_session_id"`
	ParentSessionID *string        `json:"parent_session_id"`
	Status          string         `json:"status"`
	ExitReason      string         `json:"exit_reason"`
	// Capture records how this record was produced (live hooks vs post-hoc
	// import) and at what fidelity. Stamped on every record so the two-path
	// ingestion model (see docs/INGESTION.md) is queryable, never implicit.
	Capture         CaptureInfo    `json:"capture"`
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
	// Tokens is reserved in v1 — always null. The upstream hook payload
	// carries no token data; v2 enrichment is future work (ETCH-40 f.10).
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

// CaptureInfo is the ingestion provenance of a record.
//
//   - Method:   "hooks" (live hook dispatch or crash recovery) | "import"
//     (post-hoc transcript ingestion).
//   - Fidelity: "full" (tool-level events captured) | "session_only" (only
//     session boundaries were available — e.g. a transcript with no tool calls
//     or a coarse notify-driven record).
//   - Source:   free-form origin tag for imports (e.g. "claude-code-transcript");
//     nil for the hook path.
type CaptureInfo struct {
	Method   string  `json:"method"`
	Fidelity string  `json:"fidelity"`
	Source   *string `json:"source,omitempty"`
}

// Capture method/fidelity constants — the only legal values of CaptureInfo.
const (
	CaptureMethodHooks  = "hooks"
	CaptureMethodImport = "import"

	FidelityFull        = "full"
	FidelitySessionOnly = "session_only"
)

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
	Branch       string `json:"branch"`
	HeadSHA      string `json:"head_sha"`
	WorktreePath string `json:"worktree_path"`
	IsWorktree   bool   `json:"is_worktree"`
	RepoRoot     string `json:"repo_root"`
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
//
// PID and PIDStartTime identify the agent-runtime process for the recovery
// liveness check (ETCH-40 finding 1). They are wip-only recovery metadata:
// the reducer never copies them into the Session, so they never reach the
// committed etch.session.v1 record.
type SessionStartData struct {
	SessionID       string         `json:"session_id"`
	AgentSessionID  *string        `json:"agent_session_id,omitempty"`
	ParentSessionID *string        `json:"parent_session_id,omitempty"`
	Agent           AgentInfo      `json:"agent"`
	Orchestration   Orchestration  `json:"orchestration"`
	Machine         MachineInfo    `json:"machine"`
	Operator        OperatorInfo   `json:"operator"`
	GitState        *GitState      `json:"git_state"`
	C11             *C11Info       `json:"c11"`
	TranscriptRef   *TranscriptRef `json:"transcript_ref"`
	PID             int            `json:"pid,omitempty"`
	PIDStartTime    string         `json:"pid_start_time,omitempty"`
}

// PromptData is the data payload for a user_prompt_submit event.
type PromptData struct {
	Prompt    string `json:"prompt"`
	Source    string `json:"source"`
	Truncated bool   `json:"truncated"`
}

// ToolUseData is the data payload for pre_tool_use / post_tool_use events.
// ToolUseID lets the reducer count a re-delivered pre_tool_use exactly once.
type ToolUseData struct {
	ToolName  string `json:"tool_name"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	FilePath  string `json:"file_path,omitempty"`
}

// SessionEndData is the data payload for session_end / stop events.
type SessionEndData struct {
	GitState   *GitState `json:"git_state"`
	ExitReason string    `json:"exit_reason"`
}
