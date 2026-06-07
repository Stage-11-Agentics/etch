package capture

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	etchDir    = ".etch"
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

	session := &Session{
		SchemaVersion: SchemaVersion,
		SessionID:     sessionID,
		Status:        "complete",
		ExitReason:    "normal",
		ToolUse: ToolUseSummary{
			ByTool: make(map[string]int),
		},
		Outcome: Outcome{
			Commits: []string{},
		},
		FilesTouched: []FileEntry{},
	}

	toolFilePaths := make(map[string]bool)

	for _, ev := range events {
		switch ev.Hook {
		case "session_start":
			var d SessionStartData
			if json.Unmarshal(ev.Data, &d) == nil {
				session.SessionID = d.SessionID
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
					session.ToolUse.TotalCalls++
					session.ToolUse.ByTool[d.ToolName]++
				}
				if d.FilePath != "" {
					toolFilePaths[d.FilePath] = true
				}
			}

		case "session_end", "stop":
			var d SessionEndData
			if json.Unmarshal(ev.Data, &d) == nil {
				session.GitEnd = d.GitState
				if d.ExitReason != "" {
					session.ExitReason = d.ExitReason
				}
				session.Timing.EndedAt = stringPtr(ev.Timestamp)
			}
			if ev.Hook == "stop" && session.ExitReason == "normal" {
				session.ExitReason = "unknown"
			}
		}
	}

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

	// files_touched: defer accurate actions to git diff at session end
	if session.GitStart != nil && session.GitEnd != nil && session.GitStart.HeadSHA != "" {
		files, err := gitDiffFiles(workDir, session.GitStart.HeadSHA)
		if err == nil {
			session.FilesTouched = files
		}
	}
	// Fallback: if git diff didn't produce results, use tool-tracked paths
	if len(session.FilesTouched) == 0 && len(toolFilePaths) > 0 {
		for p := range toolFilePaths {
			session.FilesTouched = append(session.FilesTouched, FileEntry{Path: p, Action: "modified"})
		}
	}

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
