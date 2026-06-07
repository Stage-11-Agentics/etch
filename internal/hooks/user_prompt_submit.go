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

	rc, err := resolveContext()
	if err != nil {
		return err
	}

	sessionID := capture.LookupMapping(rc.StateRoot, ev.SessionID)
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

	if err := capture.AppendEvent(rc.StateRoot, sessionID, "user_prompt_submit", data); err != nil {
		return err
	}

	printOK()
	return nil
}
