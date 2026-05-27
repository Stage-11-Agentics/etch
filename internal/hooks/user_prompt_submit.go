package hooks

import (
	"forgejo.stage11.ai/s11/etch/internal/capture"
)

const maxPromptBytes = 32 * 1024

func RunUserPromptSubmit() error {
	ev, err := readStdin()
	if err != nil {
		return err
	}

	repoRoot := findRepoRoot()
	sessionID := capture.LookupMapping(repoRoot, ev.SessionID)
	if sessionID == "" {
		printOK()
		return nil
	}

	prompt := ev.UserPrompt
	truncated := false
	if len(prompt) > maxPromptBytes {
		prompt = prompt[:maxPromptBytes]
		truncated = true
	}

	data := capture.PromptData{
		Prompt:    prompt,
		Source:    capture.InferPromptSource(),
		Truncated: truncated,
	}

	if err := capture.AppendEvent(repoRoot, sessionID, "user_prompt_submit", data); err != nil {
		return err
	}

	printOK()
	return nil
}
