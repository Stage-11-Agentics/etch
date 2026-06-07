package schema

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
	Agent           Agent          `json:"agent"`
	Prompt          *Prompt        `json:"prompt"`
	Orchestration   *Orchestration `json:"orchestration"`
	Timing          Timing         `json:"timing"`
	Machine         *Machine       `json:"machine"`
	Operator        *Operator      `json:"operator"`
	GitStart        *GitState      `json:"git_start"`
	GitEnd          *GitState      `json:"git_end"`
	Outcome         *Outcome       `json:"outcome"`
	FilesTouched    []FileEntry    `json:"files_touched"`
	// Tokens is reserved in v1 — always null. The upstream hook payload
	// carries no token data; v2 enrichment is future work (ETCH-40 f.10).
	Tokens          *Tokens        `json:"tokens"`
	ToolUse         *ToolUse       `json:"tool_use"`
	TranscriptRef   *TranscriptRef `json:"transcript_ref"`
	C11             *C11Context    `json:"c11"`
}

type Agent struct {
	Runtime string  `json:"runtime"`
	Model   *string `json:"model"`
	Version *string `json:"version"`
}

type Prompt struct {
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
	StartedAt  *string `json:"started_at"`
	EndedAt    *string `json:"ended_at"`
	DurationMS *int64  `json:"duration_ms"`
}

type Machine struct {
	HostnameHash string  `json:"hostname_hash"`
	HostnameRaw  *string `json:"hostname_raw"`
	OS           string  `json:"os"`
	OSVersion    string  `json:"os_version"`
	Arch         string  `json:"arch"`
}

type Operator struct {
	GitUser string `json:"git_user"`
	OSUser  string `json:"os_user"`
}

type GitState struct {
	Branch         string   `json:"branch"`
	HeadSHA        string   `json:"head_sha"`
	WorktreePath   string   `json:"worktree_path,omitempty"`
	IsWorktree     bool     `json:"is_worktree,omitempty"`
	RepoRoot       string   `json:"repo_root,omitempty"`
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

type Tokens struct {
	Input            int64    `json:"input"`
	Output           int64    `json:"output"`
	CacheRead        int64    `json:"cache_read"`
	CacheWrite       int64    `json:"cache_write"`
	APICalls         int      `json:"api_calls"`
	EstimatedCostUSD float64  `json:"estimated_cost_usd"`
}

type ToolUse struct {
	TotalCalls int            `json:"total_calls"`
	ByTool     map[string]int `json:"by_tool"`
}

type TranscriptRef struct {
	EntireCheckpointID *string `json:"entire_checkpoint_id"`
	LocalPath          *string `json:"local_path"`
	Available          bool    `json:"available"`
}

type C11Context struct {
	WorkspaceID  string   `json:"workspace_id"`
	SurfaceID    string   `json:"surface_id"`
	TabTitle     string   `json:"tab_title"`
	PaneLineage  []string `json:"pane_lineage"`
}
