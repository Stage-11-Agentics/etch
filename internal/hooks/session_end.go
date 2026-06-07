package hooks

import (
	"encoding/json"
	"fmt"
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

	rc, err := resolveContext()
	if err != nil {
		return err
	}

	sessionID := capture.LookupMapping(rc.StateRoot, ev.SessionID)
	if sessionID == "" {
		// By design: a stop arriving after session_end already finalized takes this
		// path. Log so a genuinely dropped session is visible on stderr.
		log.Printf("etch: %s: no session mapping for %q under %s (already finalized, or session_start never ran)", hookName, ev.SessionID, rc.StateRoot)
		printOK()
		return nil
	}

	// Read start SHA from the existing wip to compute commits_produced
	var startSHA string
	events, _ := capture.ReadEvents(rc.StateRoot, sessionID)
	for _, e := range events {
		if e.Hook == "session_start" {
			var d capture.SessionStartData
			if json.Unmarshal(e.Data, &d) == nil && d.GitState != nil {
				startSHA = d.GitState.HeadSHA
			}
			break
		}
	}

	gitEnd := capture.CaptureGitEnd(rc.WorkDir, startSHA)

	data := capture.SessionEndData{
		GitState:   gitEnd,
		ExitReason: defaultExitReason,
	}

	if err := capture.AppendEvent(rc.StateRoot, sessionID, hookName, data); err != nil {
		return err
	}

	// Finalize the session
	session, err := capture.Finalize(rc.StateRoot, rc.WorkDir, sessionID)
	if err != nil {
		return err
	}

	// Write git ref, apply redaction, generate trace, clean up.
	// A failure here must be visible — never print ok while dropping data. The wip and
	// mapping are deliberately left on disk so the next session_start recovery scan can
	// retry the commit.
	if err := commitSession(rc.StateRoot, session, ev.SessionID); err != nil {
		msg := fmt.Sprintf("failed to commit session %s (wip retained for recovery): %v", sessionID, err)
		log.Printf("etch: %s", msg)
		printNotOK(msg)
		return fmt.Errorf("%s", msg)
	}

	printOK()
	return nil
}
