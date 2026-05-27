package hooks

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"time"

	"forgejo.stage11.ai/s11/etch/internal/capture"
	"forgejo.stage11.ai/s11/etch/internal/version"
	"github.com/oklog/ulid/v2"
)

func RunSessionStart() error {
	ev, err := readStdin()
	if err != nil {
		return err
	}

	repoRoot := findRepoRoot()
	if err := capture.EnsureDirs(repoRoot); err != nil {
		return err
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

	data := capture.SessionStartData{
		SessionID:     sessionID,
		Agent:         agent,
		Orchestration: capture.CaptureOrchestration(),
		Machine:       capture.CaptureMachine(),
		Operator:      capture.CaptureOperator(repoRoot),
		GitState:      capture.CaptureGitState(repoRoot),
		C11:           capture.CaptureC11(),
		TranscriptRef: capture.CaptureTranscriptRef(ev.SessionRef),
	}

	if parentID := os.Getenv("CAIRN_PARENT_SESSION_ID"); parentID != "" {
		data.ParentSessionID = &parentID
	}

	if err := capture.AppendEvent(repoRoot, sessionID, "session_start", data); err != nil {
		return err
	}

	// Write mapping from Entire session ID to our ULID
	if err := capture.WriteMapping(repoRoot, ev.SessionID, sessionID); err != nil {
		return err
	}

	printOK()
	return nil
}
