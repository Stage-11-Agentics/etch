package importer

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Stage-11-Agentics/etch/internal/capture"
)

// ClaudeParser imports Claude Code transcripts. Claude Code writes one JSONL
// file per session under ~/.claude/projects/<encoded-cwd>/<session-id>.jsonl.
// Each line is an entry; the meaningful types are "user" (prompts and tool
// results), "assistant" (model + tool_use blocks). Entries carry sessionId,
// cwd, gitBranch, and an RFC3339 timestamp.
type ClaudeParser struct {
	// Root overrides the transcript directory (tests). Empty = ~/.claude/projects.
	Root string
}

func (p *ClaudeParser) Runtime() string { return "claude-code" }

func (p *ClaudeParser) root() string {
	if p.Root != "" {
		return p.Root
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects")
}

func (p *ClaudeParser) Discover() ([]string, error) {
	root := p.root()
	if root == "" {
		return nil, nil
	}
	return filepath.Glob(filepath.Join(root, "*", "*.jsonl"))
}

// claudeEntry is the subset of a transcript line the importer reads.
type claudeEntry struct {
	Type        string          `json:"type"`
	SessionID   string          `json:"sessionId"`
	Cwd         string          `json:"cwd"`
	GitBranch   string          `json:"gitBranch"`
	Timestamp   string          `json:"timestamp"`
	Version     string          `json:"version"`
	IsMeta      bool            `json:"isMeta"`
	IsSidechain bool            `json:"isSidechain"`
	Message     json.RawMessage `json:"message"`
}

type claudeMessage struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
}

type claudeContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func (p *ClaudeParser) Parse(path string) (*Parsed, error) {
	f, err := os.Open(path) //nolint:gosec // path comes from our own glob of ~/.claude
	if err != nil {
		return nil, err
	}
	defer f.Close()

	s := &capture.Session{
		SchemaVersion: capture.SchemaVersion,
		Status:        "complete",
		ExitReason:    "normal",
		Capture:       capture.CaptureInfo{Method: capture.CaptureMethodImport, Fidelity: capture.FidelitySessionOnly, Source: strPtr("claude-code-transcript")},
		Agent:         capture.AgentInfo{Runtime: p.Runtime()},
		Orchestration: capture.Orchestration{Type: "manual", Extra: map[string]any{}},
		ToolUse:       capture.ToolUseSummary{ByTool: map[string]int{}},
		Outcome:       capture.Outcome{Commits: []string{}},
		FilesTouched:  []capture.FileEntry{},
	}

	var (
		upstreamID string
		cwd        string
		branch     string
		version    string
		firstTS    string
		lastTS     string
		promptText string
		gotPrompt  bool
		model      string
		toolCount  int
		fileSet    = map[string]bool{}
	)

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for sc.Scan() {
		var e claudeEntry
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		if e.SessionID != "" {
			upstreamID = e.SessionID
		}
		if e.Cwd != "" {
			cwd = e.Cwd
		}
		if e.GitBranch != "" {
			branch = e.GitBranch
		}
		if e.Version != "" {
			version = e.Version
		}
		if e.Timestamp != "" {
			if firstTS == "" {
				firstTS = e.Timestamp
			}
			lastTS = e.Timestamp
		}

		if len(e.Message) == 0 {
			continue
		}
		var m claudeMessage
		if json.Unmarshal(e.Message, &m) != nil {
			continue
		}

		switch e.Type {
		case "user":
			// First real user prompt: not a meta/tool-result line, not a
			// subagent (sidechain) message.
			if !gotPrompt && !e.IsMeta && !e.IsSidechain {
				if txt := claudeUserText(m.Content); txt != "" {
					promptText = txt
					gotPrompt = true
				}
			}
		case "assistant":
			if model == "" && m.Model != "" {
				model = m.Model
			}
			for _, b := range claudeBlocks(m.Content) {
				if b.Type != "tool_use" {
					continue
				}
				toolCount++
				s.ToolUse.ByTool[b.Name]++
				if fp := jsonStringField(b.Input, "file_path"); fp != "" {
					fileSet[fp] = true
				}
			}
		}
	}

	if upstreamID == "" {
		return nil, nil // not a usable session transcript
	}

	id := upstreamID
	s.AgentSessionID = &id
	s.SessionID = mintULID(parseTS(firstTS))
	if model != "" {
		s.Agent.Model = &model
	}
	if version != "" {
		v := version
		s.Agent.Version = &v
	}
	if gotPrompt {
		s.Prompt = &capture.PromptInfo{Text: promptText, Source: "import"}
	}
	s.Timing.StartedAt = firstTS
	if lastTS != "" {
		end := lastTS
		s.Timing.EndedAt = &end
		if d := durationMs(firstTS, lastTS); d != nil {
			s.Timing.DurationMs = d
		}
	}
	if branch != "" || cwd != "" {
		s.GitStart = &capture.GitState{Branch: branch, RepoRoot: cwd}
	}
	s.ToolUse.TotalCalls = toolCount
	if len(fileSet) > 0 {
		paths := make([]string, 0, len(fileSet))
		for fp := range fileSet {
			paths = append(paths, fp)
		}
		sort.Strings(paths)
		for _, fp := range paths {
			s.FilesTouched = append(s.FilesTouched, capture.FileEntry{Path: fp, Action: "modified"})
		}
	}
	// Tool-level events recovered → full fidelity; otherwise only session shape.
	if toolCount > 0 {
		s.Capture.Fidelity = capture.FidelityFull
	}

	return &Parsed{Session: s, Cwd: cwd}, nil
}

// claudeUserText extracts prompt text from a user message's content, which is
// either a plain string or an array of content blocks.
func claudeUserText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	var str string
	if json.Unmarshal(content, &str) == nil {
		return strings.TrimSpace(str)
	}
	var parts []string
	for _, b := range claudeBlocks(content) {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func claudeBlocks(content json.RawMessage) []claudeContentBlock {
	if len(content) == 0 {
		return nil
	}
	var blocks []claudeContentBlock
	if json.Unmarshal(content, &blocks) == nil {
		return blocks
	}
	return nil
}

func strPtr(s string) *string { return &s }

func parseTS(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func durationMs(start, end string) *int64 {
	s, e := parseTS(start), parseTS(end)
	if s.IsZero() || e.IsZero() {
		return nil
	}
	d := e.Sub(s).Milliseconds()
	if d < 0 {
		return nil
	}
	return &d
}
