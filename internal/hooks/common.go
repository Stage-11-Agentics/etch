package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"forgejo.stage11.ai/s11/etch/internal/capture"
)

// StdinEvent is the JSON structure Entire sends on stdin for hook invocations.
type StdinEvent struct {
	SessionID  string          `json:"session_id"`
	SessionRef string          `json:"session_ref"`
	Timestamp  string          `json:"timestamp"`
	UserPrompt string          `json:"user_prompt"`
	ToolName   string          `json:"tool_name"`
	ToolUseID  string          `json:"tool_use_id"`
	ToolInput  json.RawMessage `json:"tool_input"`
	RawData    json.RawMessage `json:"raw_data"`
}

func readStdin() (*StdinEvent, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
	var ev StdinEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return nil, fmt.Errorf("parsing stdin JSON: %w", err)
	}
	return &ev, nil
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
