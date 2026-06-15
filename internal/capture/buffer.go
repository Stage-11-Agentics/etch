package capture

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	etchDir     = ".etch"
	sessionsDir = "sessions"
	mapDir      = ".map"
	wipSuffix   = ".wip.jsonl"
	sessionFile = ".session.json"
)

func sessionsPath(repoRoot string) string {
	return filepath.Join(repoRoot, etchDir, sessionsDir)
}

func mapPath(repoRoot string) string {
	return filepath.Join(sessionsPath(repoRoot), mapDir)
}

func wipPath(repoRoot string, sessionID string) string {
	return filepath.Join(sessionsPath(repoRoot), sessionID+wipSuffix)
}

func sessionJSONPath(repoRoot string, sessionID string) string {
	return filepath.Join(sessionsPath(repoRoot), sessionID+sessionFile)
}

// EnsureDirs creates .etch/sessions/ and .etch/sessions/.map/ if they don't exist.
func EnsureDirs(repoRoot string) error {
	if err := os.MkdirAll(sessionsPath(repoRoot), 0o755); err != nil {
		return fmt.Errorf("creating sessions dir: %w", err)
	}
	if err := os.MkdirAll(mapPath(repoRoot), 0o755); err != nil {
		return fmt.Errorf("creating map dir: %w", err)
	}
	return nil
}

// WriteMapping stores the ULID for an Entire session ID.
func WriteMapping(repoRoot, entireSessionID, ulid string) error {
	if entireSessionID == "" {
		return nil
	}
	safe := sanitizeFilename(entireSessionID)
	return os.WriteFile(filepath.Join(mapPath(repoRoot), safe), []byte(ulid), 0o644)
}

// LookupMapping returns the ULID for an Entire session ID, or "" if not found.
func LookupMapping(repoRoot, entireSessionID string) string {
	if entireSessionID == "" {
		return ""
	}
	safe := sanitizeFilename(entireSessionID)
	data, err := os.ReadFile(filepath.Join(mapPath(repoRoot), safe))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// CleanupMapping removes the mapping file for an Entire session ID.
func CleanupMapping(repoRoot, entireSessionID string) {
	if entireSessionID == "" {
		return
	}
	safe := sanitizeFilename(entireSessionID)
	os.Remove(filepath.Join(mapPath(repoRoot), safe))
}

// CleanupMappingByULID removes any mapping file whose content is the given
// ULID. Recovery only knows the ULID (mappings are keyed by the upstream
// session id), so it reverse-scans the map dir. A stale mapping left behind
// after recovery would let the "recovered" session's later hooks silently
// recreate its wip from nothing (ETCH-40 finding 1's tail).
func CleanupMappingByULID(repoRoot, ulid string) {
	entries, err := os.ReadDir(mapPath(repoRoot))
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		p := filepath.Join(mapPath(repoRoot), e.Name())
		data, err := os.ReadFile(p)
		if err == nil && strings.TrimSpace(string(data)) == ulid {
			os.Remove(p)
		}
	}
}

// AppendEvent writes a HookEvent as a JSON line to the .wip.jsonl file.
func AppendEvent(repoRoot, sessionID string, hook string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling event data: %w", err)
	}

	event := HookEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Hook:      hook,
		Data:      raw,
	}

	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshaling event: %w", err)
	}

	f, err := os.OpenFile(wipPath(repoRoot, sessionID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening wip file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("writing event: %w", err)
	}
	return nil
}

// ReadEvents reads all HookEvents from a .wip.jsonl file.
func ReadEvents(repoRoot, sessionID string) ([]HookEvent, error) {
	f, err := os.Open(wipPath(repoRoot, sessionID))
	if err != nil {
		return nil, fmt.Errorf("opening wip file: %w", err)
	}
	defer f.Close()

	var events []HookEvent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)
	for scanner.Scan() {
		var ev HookEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		return events, fmt.Errorf("scanning wip file: %w", err)
	}
	return events, nil
}

// ReduceInfo carries reducer outputs that feed FinishSession but are not part
// of the Session record itself.
type ReduceInfo struct {
	HasEnd        bool     // an end event (session_end or stop) was seen
	ToolFilePaths []string // tool-reported file paths, files_touched fallback
}

// ReduceEvents aggregates wip events into a Session. It is THE single
// event→session reducer: the normal finalize path and crash recovery both
// ride it, so the two can never diverge (ETCH-40 finding 9). Pure
// aggregation — no filesystem writes, no git execs.
func ReduceEvents(sessionID string, events []HookEvent) (*Session, ReduceInfo) {
	session := &Session{
		SchemaVersion: SchemaVersion,
		SessionID:     sessionID,
		Status:        "complete",
		ExitReason:    "normal",
		// Every record built from wip events came through the live hook path
		// (normal finalize and crash recovery both reduce wip). Import builds
		// its Session separately and stamps method=import itself.
		Capture: CaptureInfo{Method: CaptureMethodHooks, Fidelity: FidelityFull},
		ToolUse: ToolUseSummary{
			ByTool: make(map[string]int),
		},
		Outcome: Outcome{
			Commits: []string{},
		},
		FilesTouched: []FileEntry{},
	}

	toolFilePaths := make(map[string]bool)
	seenToolUse := make(map[string]bool)
	// End-event precedence: the first session_end is authoritative; a stop
	// never overrides a seen session_end; same-type duplicates are ignored.
	// The normal flow only ever writes one end event (wip and mapping are
	// torn down right after finalize) — this matters for wips retained after
	// a failed commit (ETCH-40 finding 8), where a later stop or re-delivered
	// end event must not clobber the truthful end state.
	endState := ""

	for _, ev := range events {
		switch ev.Hook {
		case "session_start":
			var d SessionStartData
			if json.Unmarshal(ev.Data, &d) == nil {
				if d.SessionID != "" {
					session.SessionID = d.SessionID
				}
				session.AgentSessionID = d.AgentSessionID
				session.Agent = d.Agent
				session.Orchestration = d.Orchestration
				session.Machine = d.Machine
				session.Operator = d.Operator
				session.GitStart = d.GitState
				session.C11 = d.C11
				session.TranscriptRef = d.TranscriptRef
				session.Timing.StartedAt = ev.Timestamp
				session.ParentSessionID = d.ParentSessionID
				if d.Orchestration.Extra == nil {
					session.Orchestration.Extra = map[string]any{}
				}
				if session.Orchestration.Type == "" {
					session.Orchestration.Type = "manual"
				}
			}

		case "user_prompt_submit":
			var d PromptData
			if json.Unmarshal(ev.Data, &d) == nil {
				session.Prompt = &PromptInfo{
					Text:      d.Prompt,
					Source:    d.Source,
					Truncated: d.Truncated,
				}
			}

		case "pre_tool_use", "post_tool_use":
			var d ToolUseData
			if json.Unmarshal(ev.Data, &d) == nil {
				if ev.Hook == "pre_tool_use" {
					// A re-delivered event (duplicate hook invocation under
					// load) must not double-count: each tool_use_id counts once.
					if d.ToolUseID == "" || !seenToolUse[d.ToolUseID] {
						if d.ToolUseID != "" {
							seenToolUse[d.ToolUseID] = true
						}
						session.ToolUse.TotalCalls++
						session.ToolUse.ByTool[d.ToolName]++
					}
				}
				if d.FilePath != "" {
					toolFilePaths[d.FilePath] = true
				}
			}

		case "session_end", "stop":
			if endState == "session_end" || (ev.Hook == "stop" && endState != "") {
				continue
			}
			var d SessionEndData
			if json.Unmarshal(ev.Data, &d) == nil {
				session.GitEnd = d.GitState
				session.ExitReason = "normal"
				if d.ExitReason != "" {
					session.ExitReason = d.ExitReason
				}
				session.Timing.EndedAt = stringPtr(ev.Timestamp)
				if ev.Hook == "stop" && session.ExitReason == "normal" {
					session.ExitReason = "unknown"
				}
				endState = ev.Hook
			}
		}
	}

	info := ReduceInfo{HasEnd: endState != ""}
	for p := range toolFilePaths {
		info.ToolFilePaths = append(info.ToolFilePaths, p)
	}
	sort.Strings(info.ToolFilePaths)

	return session, info
}

// FinishSession computes the derived fields — duration, outcome commits, and
// files_touched — on a reduced Session. workDir is the checkout the git diff
// runs in; pass "" to skip the diff entirely (crash recovery without a
// trustworthy checkout — see internal/recovery). The diff is bounded by the
// recorded start/end SHAs, never live HEAD, so commits made after the
// recorded end (e.g. by other sessions before a recovery pass) are never
// attributed to this record.
func FinishSession(session *Session, info ReduceInfo, workDir string) {
	// Compute duration
	if session.Timing.EndedAt != nil {
		start, err1 := time.Parse(time.RFC3339Nano, session.Timing.StartedAt)
		end, err2 := time.Parse(time.RFC3339Nano, *session.Timing.EndedAt)
		if err1 == nil && err2 == nil {
			dur := end.Sub(start).Milliseconds()
			session.Timing.DurationMs = &dur
		}
	}

	// Populate outcome commits from git_end
	if session.GitEnd != nil && len(session.GitEnd.CommitsProduced) > 0 {
		session.Outcome.Commits = session.GitEnd.CommitsProduced
	}

	// files_touched: defer accurate actions to git diff between the recorded SHAs
	if workDir != "" && session.GitStart != nil && session.GitEnd != nil &&
		session.GitStart.HeadSHA != "" && session.GitEnd.HeadSHA != "" &&
		session.GitStart.HeadSHA != session.GitEnd.HeadSHA {
		files, err := gitDiffFiles(workDir, session.GitStart.HeadSHA, session.GitEnd.HeadSHA)
		if err == nil {
			session.FilesTouched = files
		}
	}
	// Fallback: if git diff didn't produce results, use tool-tracked paths
	if len(session.FilesTouched) == 0 && len(info.ToolFilePaths) > 0 {
		for _, p := range info.ToolFilePaths {
			session.FilesTouched = append(session.FilesTouched, FileEntry{Path: p, Action: "modified"})
		}
	}
}

// Finalize reads the .wip file, aggregates events into a Session, and writes session.json.
// State (wip, session.json) lives under repoRoot; git diffs run in workDir — the
// session's own checkout, which differs from repoRoot for linked worktrees.
func Finalize(repoRoot, workDir, sessionID string) (*Session, error) {
	events, err := ReadEvents(repoRoot, sessionID)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("no events in wip file for session %s", sessionID)
	}

	session, info := ReduceEvents(sessionID, events)
	FinishSession(session, info, workDir)

	// Write session.json
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling session: %w", err)
	}
	if err := os.WriteFile(sessionJSONPath(repoRoot, sessionID), data, 0o644); err != nil {
		return nil, fmt.Errorf("writing session.json: %w", err)
	}

	return session, nil
}

// WipExists checks if a .wip.jsonl file exists for the given session.
func WipExists(repoRoot, sessionID string) bool {
	_, err := os.Stat(wipPath(repoRoot, sessionID))
	return err == nil
}

// RemoveWip deletes the .wip.jsonl file.
func RemoveWip(repoRoot, sessionID string) {
	os.Remove(wipPath(repoRoot, sessionID))
}

// RemoveSessionJSON deletes the finalized session.json scratch file.
func RemoveSessionJSON(repoRoot, sessionID string) {
	os.Remove(sessionJSONPath(repoRoot, sessionID))
}

// SessionJSONExists checks if a session.json file exists for the given session.
func SessionJSONExists(repoRoot, sessionID string) bool {
	_, err := os.Stat(sessionJSONPath(repoRoot, sessionID))
	return err == nil
}

// ReadSessionJSON reads and parses the finalized session.json.
func ReadSessionJSON(repoRoot, sessionID string) (*Session, error) {
	data, err := os.ReadFile(sessionJSONPath(repoRoot, sessionID))
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func stringPtr(s string) *string {
	return &s
}

func sanitizeFilename(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "/", "_"), "\\", "_")
}
