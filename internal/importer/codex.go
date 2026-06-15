package importer

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/Stage-11-Agentics/etch/internal/capture"
)

// CodexParser imports OpenAI Codex CLI rollout transcripts. Codex writes one
// JSONL file per session under ~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl.
// Line types: "session_meta" (id, cwd, timestamp, cli_version), "turn_context"
// (model), "response_item" (messages and function_call tool calls), "event_msg"
// (UI events, ignored). Codex's only live hook is a coarse notify callback, so
// import is its primary full-fidelity path (see docs/INGESTION.md).
type CodexParser struct {
	// Root overrides the sessions directory (tests). Empty = ~/.codex/sessions.
	Root string
}

func (p *CodexParser) Runtime() string { return "codex" }

func (p *CodexParser) root() string {
	if p.Root != "" {
		return p.Root
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex", "sessions")
}

func (p *CodexParser) Discover() ([]string, error) {
	root := p.root()
	if root == "" {
		return nil, nil
	}
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable subtrees, keep walking
		}
		if !d.IsDir() && filepath.Ext(path) == ".jsonl" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

type codexLine struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

func (p *CodexParser) Parse(path string) (*Parsed, error) {
	f, err := os.Open(path) //nolint:gosec // path comes from our own walk of ~/.codex
	if err != nil {
		return nil, err
	}
	defer f.Close()

	s := &capture.Session{
		SchemaVersion: capture.SchemaVersion,
		Status:        "complete",
		ExitReason:    "normal",
		Capture:       capture.CaptureInfo{Method: capture.CaptureMethodImport, Fidelity: capture.FidelitySessionOnly, Source: strPtr("codex-rollout")},
		Agent:         capture.AgentInfo{Runtime: p.Runtime()},
		Orchestration: capture.Orchestration{Type: "manual", Extra: map[string]any{}},
		ToolUse:       capture.ToolUseSummary{ByTool: map[string]int{}},
		Outcome:       capture.Outcome{Commits: []string{}},
		FilesTouched:  []capture.FileEntry{},
	}

	var (
		upstreamID string
		cwd        string
		version    string
		model      string
		firstTS    string
		lastTS     string
		promptText string
		gotPrompt  bool
		toolCount  int
		fileSet    = map[string]bool{}
	)

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for sc.Scan() {
		var line codexLine
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		if line.Timestamp != "" {
			if firstTS == "" {
				firstTS = line.Timestamp
			}
			lastTS = line.Timestamp
		}

		switch line.Type {
		case "session_meta":
			upstreamID = firstNonEmpty(jsonStringField(line.Payload, "id"), upstreamID)
			cwd = firstNonEmpty(jsonStringField(line.Payload, "cwd"), cwd)
			version = firstNonEmpty(jsonStringField(line.Payload, "cli_version"), version)
		case "turn_context":
			if model == "" {
				model = jsonStringField(line.Payload, "model")
			}
		case "response_item":
			itemType := jsonStringField(line.Payload, "type")
			switch itemType {
			case "message":
				if !gotPrompt && jsonStringField(line.Payload, "role") == "user" {
					if txt := codexMessageText(line.Payload); txt != "" {
						promptText = txt
						gotPrompt = true
					}
				}
			case "function_call", "local_shell_call", "custom_tool_call":
				toolCount++
				name := firstNonEmpty(jsonStringField(line.Payload, "name"), itemType)
				s.ToolUse.ByTool[name]++
				if args := jsonStringField(line.Payload, "arguments"); args != "" {
					if fp := jsonStringField(json.RawMessage(args), "file_path"); fp != "" {
						fileSet[fp] = true
					}
				}
			}
		}
	}

	if upstreamID == "" {
		return nil, nil
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
	if cwd != "" {
		s.GitStart = &capture.GitState{RepoRoot: cwd}
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
	if toolCount > 0 {
		s.Capture.Fidelity = capture.FidelityFull
	}

	return &Parsed{Session: s, Cwd: cwd}, nil
}

// codexMessageText extracts text from a Codex message payload, whose content is
// an array of {type, text} parts (input_text / output_text / text).
func codexMessageText(payload json.RawMessage) string {
	var p struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(payload, &p) != nil {
		return ""
	}
	for _, c := range p.Content {
		if c.Text != "" {
			return c.Text
		}
	}
	return ""
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
