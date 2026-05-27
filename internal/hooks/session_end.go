package hooks

import (
	"encoding/json"
	"log"

	"forgejo.stage11.ai/s11/etch/internal/capture"
)

func RunSessionEnd() error {
	return runEnd("session_end", "normal")
}

func RunStop() error {
	return runEnd("stop", "unknown")
}

func runEnd(hookName, defaultExitReason string) error {
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

	// Read start SHA from the existing wip to compute commits_produced
	var startSHA string
	events, _ := capture.ReadEvents(repoRoot, sessionID)
	for _, e := range events {
		if e.Hook == "session_start" {
			var d capture.SessionStartData
			if json.Unmarshal(e.Data, &d) == nil && d.GitState != nil {
				startSHA = d.GitState.HeadSHA
			}
			break
		}
	}

	gitEnd := capture.CaptureGitEnd(repoRoot, startSHA)

	data := capture.SessionEndData{
		GitState:   gitEnd,
		ExitReason: defaultExitReason,
	}

	if err := capture.AppendEvent(repoRoot, sessionID, hookName, data); err != nil {
		return err
	}

	// Finalize the session
	session, err := capture.Finalize(repoRoot, sessionID)
	if err != nil {
		return err
	}

	// Write git ref, apply redaction, generate trace, clean up
	if err := commitSession(repoRoot, session, ev.SessionID); err != nil {
		log.Printf("cairn: failed to commit session %s: %v", sessionID, err)
	}

	printOK()
	return nil
}
