package hooks

import (
	"unicode/utf8"

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

	prompt := ev.PromptText()
	if prompt == "" {
		warnMissing("user_prompt_submit", `prompt ("user_prompt" or "prompt")`, ev.payloadKeys)
	}
	truncated := false
	if len(prompt) > maxPromptBytes {
		// Back off to the previous rune boundary — a mid-rune slice degrades
		// the trailing bytes to U+FFFD when the record is JSON-encoded.
		cut := maxPromptBytes
		for cut > 0 && !utf8.RuneStart(prompt[cut]) {
			cut--
		}
		prompt = prompt[:cut]
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
