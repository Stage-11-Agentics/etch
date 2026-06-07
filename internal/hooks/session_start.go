package hooks

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"forgejo.stage11.ai/s11/etch/internal/capture"
	"forgejo.stage11.ai/s11/etch/internal/config"
	"forgejo.stage11.ai/s11/etch/internal/recovery"
	"forgejo.stage11.ai/s11/etch/internal/version"
	"github.com/oklog/ulid/v2"
)

func RunSessionStart() error {
	ev, err := readStdin()
	if err != nil {
		return err
	}

	rc, err := resolveContext()
	if err != nil {
		return err
	}

	if err := capture.EnsureDirs(rc.StateRoot); err != nil {
		return err
	}

	// Recover any orphaned .wip files from crashed sessions
	sessionsDir := filepath.Join(rc.StateRoot, ".etch", "sessions")
	timeout := recovery.ReadTimeoutFromSettings(rc.StateRoot)
	if n, err := recovery.RecoverAll(sessionsDir, rc.StateRoot, timeout, &etchRefWriter{}); err != nil {
		log.Printf("etch: recovery scan failed: %v", err)
	} else if n > 0 {
		log.Printf("etch: recovered %d orphaned session(s)", n)
	}

	sessionID := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()

	// Determine agent info
	agent := capture.AgentInfo{
		Runtime: capture.InferRuntime(),
	}

	// Extract model from raw_data if available
	if ev.RawData != nil {
		var rd struct {
			Model     string `json:"model"`
			AgentName string `json:"agent_name"`
		}
		if json.Unmarshal(ev.RawData, &rd) == nil {
			if rd.Model != "" {
				agent.Model = &rd.Model
			}
			if rd.AgentName != "" {
				agent.Runtime = rd.AgentName
			}
		}
	}

	v := version.Version
	agent.Version = &v

	settings, _ := config.Load(rc.StateRoot)

	data := capture.SessionStartData{
		SessionID:     sessionID,
		Agent:         agent,
		Orchestration: capture.CaptureOrchestration(),
		Machine:       capture.CaptureMachine(settings),
		Operator:      capture.CaptureOperator(rc.WorkDir),
		GitState:      capture.CaptureGitState(rc.WorkDir),
		C11:           capture.CaptureC11(),
		TranscriptRef: capture.CaptureTranscriptRef(ev.SessionRef),
	}

	if parentID := os.Getenv("ETCH_PARENT_SESSION_ID"); parentID != "" {
		data.ParentSessionID = &parentID
	}

	if err := capture.AppendEvent(rc.StateRoot, sessionID, "session_start", data); err != nil {
		return err
	}

	// Write mapping from Entire session ID to our ULID
	if err := capture.WriteMapping(rc.StateRoot, ev.SessionID, sessionID); err != nil {
		return err
	}

	printOK()
	return nil
}
