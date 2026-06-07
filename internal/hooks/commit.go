package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"forgejo.stage11.ai/s11/etch/internal/capture"
	"forgejo.stage11.ai/s11/etch/internal/config"
	"forgejo.stage11.ai/s11/etch/internal/redact"
	"forgejo.stage11.ai/s11/etch/internal/refs"
	"forgejo.stage11.ai/s11/etch/internal/schema"
)

// commitRecord applies redaction to a reduced capture.Session, generates
// agent-trace.json, applies the strip-before-push projection when configured
// (ETCH-41), and writes the git ref(s). It is the single commit boundary:
// the normal finalize path (commitSession) and crash recovery
// (etchRefWriter) both ride it, so neither can commit a less-processed
// record than the other.
func commitRecord(repoRoot string, session *capture.Session) error {
	settings, _ := config.Load(repoRoot)

	// One redaction pass over every string-bearing field of the finalized
	// record — prompt text, file paths, tool names, orchestration extras —
	// not just Prompt.Text (ETCH-40 finding 5).
	redact.DeepRedact(session, settings)

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

	if len(settings.LocalOnlyFields) > 0 {
		// Strip-before-push projection (ETCH-41). When a configured path
		// actually strips something, the full-fidelity record goes to the
		// never-pushed local namespace FIRST; the canonical, pushable
		// sessions ref is written LAST (below) from the stripped record so
		// .wip removal coincides with the canonical ref existing. A crash
		// between the writes self-heals: recovery re-commits both refs and
		// the canonical sessions ref converges; a partial local/ commit is
		// overwritten or left for GC. When no path matches this record, the
		// projection is a no-op and no local ref is written.
		strippedJSON, strippedTrace, strippedMeta, applied, err := stripForPush(&schemaSession, settings.LocalOnlyFields)
		if err != nil {
			return err
		}
		if len(applied) > 0 {
			localRef := refs.LocalRefPrefix + session.SessionID
			if err := refs.WriteSessionRefAt(repoRoot, localRef, session.SessionID, sessionJSON, traceJSON, meta); err != nil {
				return fmt.Errorf("writing local ref: %w", err)
			}
			sessionJSON, traceJSON, meta = strippedJSON, strippedTrace, strippedMeta
		}
	}

	if err := refs.WriteSessionRef(repoRoot, session.SessionID, sessionJSON, traceJSON, meta); err != nil {
		if errors.Is(err, refs.ErrRefExists) {
			// A record for this ULID is already committed (an earlier commit
			// whose cleanup failed, or a concurrent recovery won the write)
			// and ours may not replace it. Visible, then treated as success:
			// the session IS recorded, and local state must be cleaned up so
			// retries stop.
			log.Printf("etch: session %s already committed; keeping the existing record: %v", session.SessionID, err)
			return nil
		}
		return fmt.Errorf("writing ref: %w", err)
	}

	return nil
}

// commitSession commits a finalized session and cleans up its temp state
// (wip buffer, session.json scratch file, upstream-session-id mapping).
func commitSession(repoRoot string, session *capture.Session, entireSessionID string) error {
	if err := commitRecord(repoRoot, session); err != nil {
		return err
	}

	capture.RemoveWip(repoRoot, session.SessionID)
	capture.RemoveSessionJSON(repoRoot, session.SessionID)
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

	// Recovered crash records have no ended_at; fall back to started_at so
	// the ref's commit date reflects the session, not the recovery pass.
	endTime := time.Now().UTC()
	if session.Timing.EndedAt != nil {
		if t, err := time.Parse(time.RFC3339Nano, *session.Timing.EndedAt); err == nil {
			endTime = t
		}
	} else if session.Timing.StartedAt != "" {
		if t, err := time.Parse(time.RFC3339Nano, session.Timing.StartedAt); err == nil {
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

// stripForPush produces the pushable projection of a session (ETCH-41):
// configured local_only_fields are stripped in place on the schema record,
// the strip manifest is set, and session.json, agent-trace.json, and the ref
// commit metadata are all regenerated from the stripped record — the trace
// derives files/model from the session, and the commit message embeds
// branch/model/status, so building any of them from the full record would
// leak stripped values. Both commit paths (normal and crash recovery) strip
// the same *schema.Session shape so the pushed JSON is path-independent.
// applied lists the paths that stripped something; when empty the caller
// keeps the original record and skips the local ref entirely.
func stripForPush(session *schema.Session, fields []string) (sessionJSON, traceJSON []byte, meta refs.RefMeta, applied []string, err error) {
	applied = redact.StripLocalOnly(session, fields)
	session.LocalOnlyStripped = applied
	if len(applied) == 0 {
		return nil, nil, refs.RefMeta{}, nil, nil
	}

	sessionJSON, err = json.MarshalIndent(session, "", "  ")
	if err != nil {
		return nil, nil, refs.RefMeta{}, nil, fmt.Errorf("marshaling stripped session: %w", err)
	}

	trace := schema.SessionToAgentTrace(session)
	traceJSON, err = json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return nil, nil, refs.RefMeta{}, nil, fmt.Errorf("marshaling stripped trace: %w", err)
	}

	return sessionJSON, traceJSON, buildRefMetaFromSchema(session), applied, nil
}

// etchRefWriter implements recovery.RefWriter for crash recovery. It rides
// the same commitRecord boundary as the normal path — same redaction, same
// trace generation, same strip-before-push projection, same ref writes.
type etchRefWriter struct{}

func (w *etchRefWriter) WriteSessionRef(repoDir string, session *capture.Session) error {
	if err := commitRecord(repoDir, session); err != nil {
		return err
	}
	log.Printf("etch: recovered session %s", session.SessionID)
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
