package hooks

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"forgejo.stage11.ai/s11/etch/internal/capture"
	"forgejo.stage11.ai/s11/etch/internal/config"
	"forgejo.stage11.ai/s11/etch/internal/redact"
	"forgejo.stage11.ai/s11/etch/internal/refs"
	"forgejo.stage11.ai/s11/etch/internal/schema"
)

// commitSession takes a finalized capture.Session, applies redaction,
// generates agent-trace.json, writes the git ref, and cleans up temp files.
func commitSession(repoRoot string, session *capture.Session, entireSessionID string) error {
	settings, _ := config.Load(repoRoot)

	if session.Prompt != nil {
		session.Prompt.Text = redact.Redact(session.Prompt.Text, settings)
	}

	sessionJSON, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}

	var schemaSession schema.Session
	if err := json.Unmarshal(sessionJSON, &schemaSession); err != nil {
		return fmt.Errorf("converting to schema session: %w", err)
	}

	trace := schema.SessionToAgentTrace(&schemaSession)
	traceJSON, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling trace: %w", err)
	}

	meta := buildRefMeta(session)

	if err := refs.WriteSessionRef(repoRoot, session.SessionID, sessionJSON, traceJSON, meta); err != nil {
		return fmt.Errorf("writing ref: %w", err)
	}

	capture.RemoveWip(repoRoot, session.SessionID)

	sessionJSONPath := filepath.Join(repoRoot, ".cairn", "sessions", session.SessionID+".session.json")
	os.Remove(sessionJSONPath)

	capture.CleanupMapping(repoRoot, entireSessionID)

	return nil
}

func buildRefMeta(session *capture.Session) refs.RefMeta {
	model := ""
	if session.Agent.Model != nil {
		model = *session.Agent.Model
	}

	branch := ""
	if session.GitStart != nil {
		branch = session.GitStart.Branch
	}

	commitCount := 0
	if session.GitEnd != nil {
		commitCount = len(session.GitEnd.CommitsProduced)
	}

	var durationSecs int
	if session.Timing.DurationMs != nil {
		durationSecs = int(*session.Timing.DurationMs / 1000)
	}

	endTime := time.Now().UTC()
	if session.Timing.EndedAt != nil {
		if t, err := time.Parse(time.RFC3339Nano, *session.Timing.EndedAt); err == nil {
			endTime = t
		}
	}

	return refs.RefMeta{
		Runtime:      session.Agent.Runtime,
		Model:        model,
		Status:       session.Status,
		Branch:       branch,
		CommitCount:  commitCount,
		DurationSecs: durationSecs,
		EndTime:      endTime,
	}
}

// cairnRefWriter implements recovery.RefWriter for crash recovery.
type cairnRefWriter struct{}

func (w *cairnRefWriter) WriteSessionRef(repoDir string, session *schema.Session) error {
	settings, _ := config.Load(repoDir)

	if session.Prompt != nil {
		session.Prompt.Text = redact.Redact(session.Prompt.Text, settings)
	}

	sessionJSON, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}

	trace := schema.SessionToAgentTrace(session)
	traceJSON, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling trace: %w", err)
	}

	meta := buildRefMetaFromSchema(session)

	if err := refs.WriteSessionRef(repoDir, session.SessionID, sessionJSON, traceJSON, meta); err != nil {
		return fmt.Errorf("writing ref: %w", err)
	}

	log.Printf("cairn: recovered session %s", session.SessionID)
	return nil
}

func buildRefMetaFromSchema(session *schema.Session) refs.RefMeta {
	model := ""
	if session.Agent.Model != nil {
		model = *session.Agent.Model
	}

	branch := ""
	if session.GitStart != nil {
		branch = session.GitStart.Branch
	}

	commitCount := 0
	if session.GitEnd != nil {
		commitCount = len(session.GitEnd.CommitsProduced)
	}

	var durationSecs int
	if session.Timing.DurationMS != nil {
		durationSecs = int(*session.Timing.DurationMS / 1000)
	}

	endTime := time.Now().UTC()
	if session.Timing.EndedAt != nil {
		if t, err := time.Parse(time.RFC3339Nano, *session.Timing.EndedAt); err == nil {
			endTime = t
		}
	} else if session.Timing.StartedAt != nil {
		if t, err := time.Parse(time.RFC3339Nano, *session.Timing.StartedAt); err == nil {
			endTime = t
		}
	}

	return refs.RefMeta{
		Runtime:      session.Agent.Runtime,
		Model:        model,
		Status:       session.Status,
		Branch:       branch,
		CommitCount:  commitCount,
		DurationSecs: durationSecs,
		EndTime:      endTime,
	}
}
