package hooks

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/Stage-11-Agentics/etch/internal/capture"
)

// StdinEvent is the hook payload read on stdin. Two dialects are accepted:
//
//   - Entire HookInput dialect (Entire's internal hook-input serialization):
//     session_id, session_ref, user_prompt, tool_name, tool_use_id,
//     tool_input, raw_data.model
//   - Agent-runtime native dialect (Claude Code hook JSON, what the installed
//     hooks actually deliver): session_id, transcript_path, hook_event_name,
//     prompt, tool_name, tool_use_id, tool_input, reason
//
// Dialect-specific fields win; native fields fill gaps. Unknown fields are
// ignored. See docs/HOOK_CONTRACT.md for the full contract.
type StdinEvent struct {
	SessionID  string          `json:"session_id"`
	SessionRef string          `json:"session_ref"`
	Timestamp  string          `json:"timestamp"`
	UserPrompt string          `json:"user_prompt"`
	ToolName   string          `json:"tool_name"`
	ToolUseID  string          `json:"tool_use_id"`
	ToolInput  json.RawMessage `json:"tool_input"`
	RawData    json.RawMessage `json:"raw_data"`

	// Native (Claude Code) dialect fields.
	TranscriptPath string `json:"transcript_path"`
	HookEventName  string `json:"hook_event_name"`
	Prompt         string `json:"prompt"`
	Model          string `json:"model"`
	Reason         string `json:"reason"`

	// payloadKeys are the top-level keys actually received, for warnings.
	payloadKeys []string
}

// PromptText returns the prompt in either dialect.
func (ev *StdinEvent) PromptText() string {
	if ev.UserPrompt != "" {
		return ev.UserPrompt
	}
	return ev.Prompt
}

// TranscriptRefPath returns the transcript path in either dialect.
func (ev *StdinEvent) TranscriptRefPath() string {
	if ev.SessionRef != "" {
		return ev.SessionRef
	}
	return ev.TranscriptPath
}

// ModelName returns the model in either dialect: raw_data.model
// (Entire dialect) wins, top-level model is the fallback. Native Claude Code
// payloads carry neither — there the model is derived from the transcript at
// finalize time (see modelFromTranscript).
func (ev *StdinEvent) ModelName() string {
	if ev.RawData != nil {
		var rd struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(ev.RawData, &rd) == nil && rd.Model != "" {
			return rd.Model
		}
	}
	return ev.Model
}

func readStdin() (*StdinEvent, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
	return parseEvent(data)
}

func parseEvent(data []byte) (*StdinEvent, error) {
	var ev StdinEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return nil, fmt.Errorf("parsing stdin JSON: %w", err)
	}

	var keys map[string]json.RawMessage
	if json.Unmarshal(data, &keys) == nil {
		for k := range keys {
			ev.payloadKeys = append(ev.payloadKeys, k)
		}
		sort.Strings(ev.payloadKeys)
	}

	if ev.SessionID == "" {
		warnMissing("any", "session_id", ev.payloadKeys)
	}
	return &ev, nil
}

// warnMissing emits a visible stderr warning when a hook payload lacks a
// field the event is expected to carry. stdout is never touched and callers
// keep exiting 0 — capture must not break the agent's session. This replaces
// the old behavior of silently dropping unrecognized payloads (ETCH-20).
func warnMissing(hook, expected string, gotKeys []string) {
	fmt.Fprintf(os.Stderr, "etch: warning: %s carried no %s (payload keys: [%s]); see docs/HOOK_CONTRACT.md\n",
		hook, expected, strings.Join(gotKeys, " "))
}

func printOK() {
	fmt.Println(`{"ok":true}`)
}

// printNotOK emits a non-ok result on stdout so Entire never sees success for an
// invocation that dropped data.
func printNotOK(msg string) {
	out, _ := json.Marshal(map[string]any{"ok": false, "error": msg})
	fmt.Println(string(out))
}

// resolveContext resolves the repo context for the hook process CWD. Every hook calls
// this FIRST, before any filesystem write. On failure (non-git directory, unusable
// repo, git missing) it prints a clear warning to stderr and a non-ok result to stdout,
// then returns an error — capture is disabled, nothing is created, nothing is orphaned.
func resolveContext() (*capture.RepoContext, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting cwd: %w", err)
	}
	rc, err := capture.ResolveRepoContext(cwd)
	if err != nil {
		msg := fmt.Sprintf("could not resolve a git repository (cwd=%s): %v", cwd, err)
		fmt.Fprintf(os.Stderr, "etch: %s; session capture disabled, no record will be written\n", msg)
		printNotOK(msg)
		return nil, fmt.Errorf("%s", msg)
	}
	return rc, nil
}

// modelFromTranscript scans a Claude Code transcript JSONL for the first
// assistant entry carrying message.model. Native hook payloads do not include
// the model; the transcript is the only source. Bounded, best-effort: any
// failure returns "".
func modelFromTranscript(path string) string {
	if path == "" {
		return ""
	}
	f, err := os.Open(path) //nolint:gosec // path comes from the agent's own hook payload
	if err != nil {
		return ""
	}
	defer f.Close()

	const maxLine = 4 * 1024 * 1024 // transcript lines can be large
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), maxLine)
	for sc.Scan() {
		var entry struct {
			Message struct {
				Model string `json:"model"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &entry) == nil && entry.Message.Model != "" {
			return entry.Message.Model
		}
	}
	return ""
}
