package recovery

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"forgejo.stage11.ai/s11/etch/internal/schema"
)

const DefaultTimeoutHours = 4

type RefWriter interface {
	WriteSessionRef(repoDir string, session *schema.Session) error
}

type NoOpRefWriter struct{}

func (NoOpRefWriter) WriteSessionRef(string, *schema.Session) error { return nil }

type OrphanedWIP struct {
	Path      string
	SessionID string
	LastEvent time.Time
	Reason    string // "dead_pid" or "timeout"
}

type wipEvent struct {
	HookType  string `json:"hook_type"`
	SessionID string `json:"session_id"`
	Timestamp string `json:"timestamp"`
	PID       int    `json:"pid,omitempty"`

	// session_start fields
	Model      string `json:"model,omitempty"`
	Runtime    string `json:"runtime,omitempty"`
	Version    string `json:"version,omitempty"`
	SessionRef string `json:"session_ref,omitempty"`

	// prompt fields
	Prompt        string `json:"prompt,omitempty"`
	PromptSource  string `json:"prompt_source,omitempty"`

	// orchestration (captured at session_start)
	OrchestrationType   string `json:"orchestration_type,omitempty"`
	DispatchMethod      string `json:"dispatch_method,omitempty"`
	TicketID            string `json:"ticket_id,omitempty"`
	RunID               string `json:"run_id,omitempty"`
	AgentRole           string `json:"agent_role,omitempty"`
	ParentSessionID     string `json:"parent_session_id,omitempty"`
	WorkflowVersion     string `json:"workflow_version,omitempty"`

	// git state
	Branch       string `json:"branch,omitempty"`
	HeadSHA      string `json:"head_sha,omitempty"`
	WorktreePath string `json:"worktree_path,omitempty"`
	IsWorktree   bool   `json:"is_worktree,omitempty"`
	RepoRoot     string `json:"repo_root,omitempty"`

	// machine
	HostnameHash string `json:"hostname_hash,omitempty"`
	HostnameRaw  string `json:"hostname_raw,omitempty"`
	OS           string `json:"os,omitempty"`
	OSVersion    string `json:"os_version,omitempty"`
	Arch         string `json:"arch,omitempty"`

	// operator
	GitUser string `json:"git_user,omitempty"`
	OSUser  string `json:"os_user,omitempty"`

	// tool use
	ToolName  string `json:"tool_name,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`

	// tokens (cumulative snapshots)
	TokensInput      int64   `json:"tokens_input,omitempty"`
	TokensOutput     int64   `json:"tokens_output,omitempty"`
	TokensCacheRead  int64   `json:"tokens_cache_read,omitempty"`
	TokensCacheWrite int64   `json:"tokens_cache_write,omitempty"`
	APICalls         int     `json:"api_calls,omitempty"`
	EstimatedCost    float64 `json:"estimated_cost_usd,omitempty"`
}

func ScanOrphaned(sessionsDir string, timeout time.Duration) ([]OrphanedWIP, error) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading sessions dir: %w", err)
	}

	now := time.Now()
	var orphaned []OrphanedWIP

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".wip.jsonl") {
			continue
		}

		path := filepath.Join(sessionsDir, entry.Name())
		events, parseErr := parseWIPFile(path)
		if parseErr != nil || len(events) == 0 {
			log.Printf("recovery: skipping unreadable wip file %s: %v", entry.Name(), parseErr)
			continue
		}

		sessionID := extractSessionID(events, entry.Name())
		lastEvent := lastEventTime(events)
		pid := extractPID(events)

		if pid > 0 && !processAlive(pid) {
			orphaned = append(orphaned, OrphanedWIP{
				Path:      path,
				SessionID: sessionID,
				LastEvent: lastEvent,
				Reason:    "dead_pid",
			})
			continue
		}

		if !lastEvent.IsZero() && now.Sub(lastEvent) > timeout {
			orphaned = append(orphaned, OrphanedWIP{
				Path:      path,
				SessionID: sessionID,
				LastEvent: lastEvent,
				Reason:    "timeout",
			})
		}
	}

	return orphaned, nil
}

func RecoverSession(wipPath string) (*schema.Session, error) {
	events, err := parseWIPFile(wipPath)
	if err != nil {
		return nil, fmt.Errorf("parsing wip file: %w", err)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("wip file is empty or contains no valid events")
	}

	session := &schema.Session{
		SchemaVersion: schema.SchemaVersion,
		Status:        "incomplete",
		ExitReason:    "crash",
		Timing: schema.Timing{
			EndedAt:    nil,
			DurationMS: nil,
		},
	}

	toolCounts := make(map[string]int)
	totalToolCalls := 0

	for _, ev := range events {
		if session.SessionID == "" && ev.SessionID != "" {
			session.SessionID = ev.SessionID
		}

		switch ev.HookType {
		case "session_start":
			applySessionStart(session, ev)
		case "user_prompt_submit":
			if session.Prompt == nil && ev.Prompt != "" {
				source := ev.PromptSource
				if source == "" {
					source = "unknown"
				}
				session.Prompt = &schema.Prompt{
					Text:   ev.Prompt,
					Source: source,
				}
			}
		case "pre_tool_use", "post_tool_use":
			if ev.ToolName != "" {
				toolCounts[ev.ToolName]++
				totalToolCalls++
			}
		}

		applyTokenSnapshot(session, ev)
	}

	if totalToolCalls > 0 {
		session.ToolUse = &schema.ToolUse{
			TotalCalls: totalToolCalls,
			ByTool:     toolCounts,
		}
	}

	if session.SessionID == "" {
		base := filepath.Base(wipPath)
		session.SessionID = strings.TrimSuffix(base, ".wip.jsonl")
	}

	return session, nil
}

func CleanupWIP(wipPath string) error {
	return os.Remove(wipPath)
}

func applySessionStart(session *schema.Session, ev wipEvent) {
	if ev.Runtime != "" || ev.Model != "" {
		session.Agent = schema.Agent{Runtime: ev.Runtime}
		if ev.Model != "" {
			session.Agent.Model = strPtr(ev.Model)
		}
		if ev.Version != "" {
			session.Agent.Version = strPtr(ev.Version)
		}
	}

	if ev.Timestamp != "" {
		session.Timing.StartedAt = strPtr(ev.Timestamp)
	}

	if ev.ParentSessionID != "" {
		session.ParentSessionID = strPtr(ev.ParentSessionID)
	}

	orchType := ev.OrchestrationType
	if orchType == "" {
		orchType = "manual"
	}
	session.Orchestration = &schema.Orchestration{
		Type:  orchType,
		Extra: make(map[string]any),
	}
	if ev.DispatchMethod != "" {
		session.Orchestration.DispatchMethod = strPtr(ev.DispatchMethod)
	}
	if ev.TicketID != "" {
		session.Orchestration.TicketID = strPtr(ev.TicketID)
	}
	if ev.RunID != "" {
		session.Orchestration.RunID = strPtr(ev.RunID)
	}
	if ev.AgentRole != "" {
		session.Orchestration.Role = strPtr(ev.AgentRole)
	}
	if ev.WorkflowVersion != "" {
		session.Orchestration.WorkflowVersion = strPtr(ev.WorkflowVersion)
	}

	if ev.Branch != "" || ev.HeadSHA != "" {
		session.GitStart = &schema.GitState{
			Branch:       ev.Branch,
			HeadSHA:      ev.HeadSHA,
			WorktreePath: ev.WorktreePath,
			IsWorktree:   ev.IsWorktree,
			RepoRoot:     ev.RepoRoot,
		}
		session.GitEnd = &schema.GitState{
			Branch:  ev.Branch,
			HeadSHA: ev.HeadSHA,
		}
	}

	if ev.HostnameHash != "" || ev.OS != "" {
		session.Machine = &schema.Machine{
			HostnameHash: ev.HostnameHash,
			OS:           ev.OS,
			OSVersion:    ev.OSVersion,
			Arch:         ev.Arch,
		}
		if ev.HostnameRaw != "" {
			session.Machine.HostnameRaw = strPtr(ev.HostnameRaw)
		}
	}

	if ev.GitUser != "" || ev.OSUser != "" {
		session.Operator = &schema.Operator{
			GitUser: ev.GitUser,
			OSUser:  ev.OSUser,
		}
	}
}

func applyTokenSnapshot(session *schema.Session, ev wipEvent) {
	if ev.TokensInput == 0 && ev.TokensOutput == 0 {
		return
	}
	session.Tokens = &schema.Tokens{
		Input:            ev.TokensInput,
		Output:           ev.TokensOutput,
		CacheRead:        ev.TokensCacheRead,
		CacheWrite:       ev.TokensCacheWrite,
		APICalls:         ev.APICalls,
		EstimatedCostUSD: ev.EstimatedCost,
	}
}

// hookEvent is the actual format written by the capture package.
type hookEvent struct {
	Timestamp string          `json:"ts"`
	Hook      string          `json:"hook"`
	Data      json.RawMessage `json:"data"`
}

func parseWIPFile(path string) ([]wipEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var events []wipEvent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Try nested HookEvent format first (actual .wip.jsonl format)
		var he hookEvent
		if json.Unmarshal([]byte(line), &he) == nil && he.Hook != "" {
			ev := flattenHookEvent(he)
			events = append(events, ev)
			continue
		}

		// Fall back to flat wipEvent format
		var ev wipEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			log.Printf("recovery: skipping corrupt line in %s: %v", filepath.Base(path), err)
			continue
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		return events, fmt.Errorf("scanning wip file: %w", err)
	}
	return events, nil
}

// flattenHookEvent converts a nested HookEvent into a flat wipEvent for recovery processing.
func flattenHookEvent(he hookEvent) wipEvent {
	ev := wipEvent{
		HookType:  he.Hook,
		Timestamp: he.Timestamp,
	}

	if he.Data == nil {
		return ev
	}

	// Unmarshal the data payload into a generic map, then extract known fields
	var data map[string]json.RawMessage
	if json.Unmarshal(he.Data, &data) != nil {
		return ev
	}

	decodeStr := func(key string) string {
		raw, ok := data[key]
		if !ok {
			return ""
		}
		var s string
		json.Unmarshal(raw, &s)
		return s
	}

	switch he.Hook {
	case "session_start":
		ev.SessionID = decodeStr("session_id")
		ev.SessionRef = decodeStr("session_ref")

		if raw, ok := data["agent"]; ok {
			var agent struct {
				Runtime string  `json:"runtime"`
				Model   *string `json:"model"`
				Version *string `json:"version"`
			}
			if json.Unmarshal(raw, &agent) == nil {
				ev.Runtime = agent.Runtime
				if agent.Model != nil {
					ev.Model = *agent.Model
				}
				if agent.Version != nil {
					ev.Version = *agent.Version
				}
			}
		}

		if raw, ok := data["orchestration"]; ok {
			var orch struct {
				Type            string  `json:"type"`
				DispatchMethod  *string `json:"dispatch_method"`
				TicketID        *string `json:"ticket_id"`
				RunID           *string `json:"run_id"`
				Role            *string `json:"role"`
				WorkflowVersion *string `json:"workflow_version"`
			}
			if json.Unmarshal(raw, &orch) == nil {
				ev.OrchestrationType = orch.Type
				if orch.DispatchMethod != nil {
					ev.DispatchMethod = *orch.DispatchMethod
				}
				if orch.TicketID != nil {
					ev.TicketID = *orch.TicketID
				}
				if orch.RunID != nil {
					ev.RunID = *orch.RunID
				}
				if orch.Role != nil {
					ev.AgentRole = *orch.Role
				}
				if orch.WorkflowVersion != nil {
					ev.WorkflowVersion = *orch.WorkflowVersion
				}
			}
		}

		ev.ParentSessionID = decodeStr("parent_session_id")

		if raw, ok := data["machine"]; ok {
			var machine struct {
				HostnameHash string  `json:"hostname_hash"`
				HostnameRaw  *string `json:"hostname_raw"`
				OS           string  `json:"os"`
				OSVersion    string  `json:"os_version"`
				Arch         string  `json:"arch"`
			}
			if json.Unmarshal(raw, &machine) == nil {
				ev.HostnameHash = machine.HostnameHash
				ev.OS = machine.OS
				ev.OSVersion = machine.OSVersion
				ev.Arch = machine.Arch
				if machine.HostnameRaw != nil {
					ev.HostnameRaw = *machine.HostnameRaw
				}
			}
		}

		if raw, ok := data["operator"]; ok {
			var op struct {
				GitUser string `json:"git_user"`
				OSUser  string `json:"os_user"`
			}
			if json.Unmarshal(raw, &op) == nil {
				ev.GitUser = op.GitUser
				ev.OSUser = op.OSUser
			}
		}

		if raw, ok := data["git_state"]; ok {
			var gs struct {
				Branch       string `json:"branch"`
				HeadSHA      string `json:"head_sha"`
				WorktreePath string `json:"worktree_path"`
				IsWorktree   bool   `json:"is_worktree"`
				RepoRoot     string `json:"repo_root"`
			}
			if json.Unmarshal(raw, &gs) == nil {
				ev.Branch = gs.Branch
				ev.HeadSHA = gs.HeadSHA
				ev.WorktreePath = gs.WorktreePath
				ev.IsWorktree = gs.IsWorktree
				ev.RepoRoot = gs.RepoRoot
			}
		}

		if raw, ok := data["pid"]; ok {
			json.Unmarshal(raw, &ev.PID)
		}

	case "user_prompt_submit":
		ev.Prompt = decodeStr("prompt")
		ev.PromptSource = decodeStr("source")

	case "pre_tool_use", "post_tool_use":
		ev.ToolName = decodeStr("tool_name")
		ev.ToolUseID = decodeStr("tool_use_id")
	}

	return ev
}

func extractSessionID(events []wipEvent, filename string) string {
	for _, ev := range events {
		if ev.SessionID != "" {
			return ev.SessionID
		}
	}
	return strings.TrimSuffix(filename, ".wip.jsonl")
}

func lastEventTime(events []wipEvent) time.Time {
	var latest time.Time
	for _, ev := range events {
		if ev.Timestamp == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, ev.Timestamp)
		if err != nil {
			t, err = time.Parse(time.RFC3339Nano, ev.Timestamp)
			if err != nil {
				continue
			}
		}
		if t.After(latest) {
			latest = t
		}
	}
	return latest
}

func extractPID(events []wipEvent) int {
	for _, ev := range events {
		if ev.HookType == "session_start" && ev.PID > 0 {
			return ev.PID
		}
	}
	return 0
}

func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func strPtr(s string) *string {
	return &s
}

// RecoverAll scans for orphaned .wip files and recovers them using the provided RefWriter.
// Returns the number of sessions recovered and any error from the scan itself.
func RecoverAll(sessionsDir string, repoDir string, timeout time.Duration, writer RefWriter) (int, error) {
	orphaned, err := ScanOrphaned(sessionsDir, timeout)
	if err != nil {
		return 0, err
	}

	recovered := 0
	for _, wip := range orphaned {
		session, recErr := RecoverSession(wip.Path)
		if recErr != nil {
			log.Printf("recovery: failed to recover %s: %v", filepath.Base(wip.Path), recErr)
			continue
		}

		if writeErr := writer.WriteSessionRef(repoDir, session); writeErr != nil {
			log.Printf("recovery: failed to write ref for %s: %v", wip.SessionID, writeErr)
			continue
		}

		if cleanErr := CleanupWIP(wip.Path); cleanErr != nil {
			log.Printf("recovery: failed to cleanup %s: %v", filepath.Base(wip.Path), cleanErr)
		}

		recovered++
	}

	return recovered, nil
}

// ReadTimeoutFromSettings reads recovery_timeout_hours from .cairn/settings.json.
// Returns the default timeout if the file doesn't exist or doesn't contain the field.
func ReadTimeoutFromSettings(repoDir string) time.Duration {
	settingsPath := filepath.Join(repoDir, ".cairn", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return DefaultTimeoutHours * time.Hour
	}

	var settings struct {
		RecoveryTimeoutHours json.Number `json:"recovery_timeout_hours"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return DefaultTimeoutHours * time.Hour
	}

	if settings.RecoveryTimeoutHours == "" {
		return DefaultTimeoutHours * time.Hour
	}

	hours, err := strconv.ParseFloat(string(settings.RecoveryTimeoutHours), 64)
	if err != nil || hours <= 0 {
		return DefaultTimeoutHours * time.Hour
	}

	return time.Duration(hours * float64(time.Hour))
}
