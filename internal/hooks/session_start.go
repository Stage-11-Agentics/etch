package hooks

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/Stage-11-Agentics/etch/internal/capture"
	"github.com/Stage-11-Agentics/etch/internal/config"
	"github.com/Stage-11-Agentics/etch/internal/recovery"
	"github.com/Stage-11-Agentics/etch/internal/version"
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

	// Duplicate/resumed session_start (ETCH-40 finding 4): a second
	// session_start for the same upstream session id must not mint a fresh
	// ULID and clobber the mapping — that splits one logical session into a
	// live record plus an orphaned 'crash' record. Reuse the existing
	// session, and shield its wip from this invocation's recovery pass (a
	// resume-after-crash arrives with a dead recorded PID — without the
	// shield, recovery would commit the very wip this session is resuming).
	var resumeULID string
	if existing := capture.LookupMapping(rc.StateRoot, ev.SessionID); existing != "" && capture.WipExists(rc.StateRoot, existing) {
		resumeULID = existing
	}

	// Recover any orphaned .wip files from crashed sessions
	timeout := recovery.ReadTimeoutFromSettings(rc.StateRoot)
	var exclude map[string]bool
	if resumeULID != "" {
		exclude = map[string]bool{resumeULID: true}
	}
	if n, err := recovery.RecoverAll(rc.StateRoot, timeout, &etchRefWriter{}, exclude); err != nil {
		log.Printf("etch: recovery scan failed: %v", err)
	} else if n > 0 {
		log.Printf("etch: recovered %d orphaned session(s)", n)
	}

	if resumeULID != "" {
		log.Printf("etch: duplicate session_start for %q — reusing session %s (no new ULID minted)", ev.SessionID, resumeULID)
		printOK()
		return nil
	}

	sessionID := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()

	// Determine agent info
	agent := capture.AgentInfo{
		Runtime: capture.InferRuntime(),
	}

	// Model: either dialect's explicit field wins. Native Claude Code
	// payloads carry no model at all — it is backfilled from the transcript
	// at finalize time (see runEnd), so only warn when there is no
	// transcript to derive it from later.
	if model := ev.ModelName(); model != "" {
		agent.Model = &model
	} else if ev.TranscriptRefPath() == "" {
		warnMissing("session_start", "model (raw_data.model or model) or transcript_path", ev.payloadKeys)
	}
	if ev.RawData != nil {
		var rd struct {
			AgentName string `json:"agent_name"`
		}
		if json.Unmarshal(ev.RawData, &rd) == nil && rd.AgentName != "" {
			agent.Runtime = rd.AgentName
		}
	}

	v := version.Version
	agent.Version = &v

	settings, _ := config.Load(rc.StateRoot)

	// Salt lives next to settings.json in the state root (per-repo).
	salt, err := config.EnsureHostnameSalt(rc.StateRoot)
	if err != nil {
		log.Printf("etch: hostname salt unavailable, falling back to unsalted hash: %v", err)
	}

	data := capture.SessionStartData{
		SessionID:     sessionID,
		Agent:         agent,
		Orchestration: capture.CaptureOrchestration(),
		Machine:       capture.CaptureMachine(settings, salt),
		Operator:      capture.CaptureOperator(rc.WorkDir),
		GitState:      capture.CaptureGitState(rc.WorkDir),
		C11:           capture.CaptureC11(),
		TranscriptRef: capture.CaptureTranscriptRef(ev.TranscriptRefPath()),
	}

	// Auto-detect orchestration fields nobody declared, from signals already
	// captured (git branch, c11 tab title/lineage). This closes the gap where
	// an orchestrator never exported ETCH_TICKET_ID/ETCH_AGENT_ROLE — capture
	// no longer goes blank just because the env vars weren't wired. Explicit
	// values win; inferred ones are tagged in orchestration.extra._sources.
	{
		branch := ""
		if data.GitState != nil {
			branch = data.GitState.Branch
		}
		tabTitle := ""
		var lineage []string
		if data.C11 != nil {
			tabTitle = data.C11.TabTitle
			lineage = data.C11.PaneLineage
		}
		capture.EnrichOrchestration(&data.Orchestration, branch, tabTitle, lineage)
	}

	// Preserve the upstream runtime's own session id (ETCH-23). Etch's
	// minted ULID stays canonical for refs; this is the join key back to
	// the agent runtime's transcripts and logs.
	if ev.SessionID != "" {
		upstreamID := ev.SessionID
		data.AgentSessionID = &upstreamID
	}

	if parentID := os.Getenv("ETCH_PARENT_SESSION_ID"); parentID != "" {
		data.ParentSessionID = &parentID
	}

	// Record the agent-runtime process identity so the recovery scan can tell
	// a live idle session from a crashed one (ETCH-40 finding 1). Best-effort:
	// 0 when no unambiguous agent ancestor exists, and the idle timeout
	// governs recovery as before.
	data.PID, data.PIDStartTime = capture.CaptureAgentPID()

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
