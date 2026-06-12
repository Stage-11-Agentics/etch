// Package recovery commits orphaned .wip session buffers left behind by
// crashed or interrupted sessions. It rides the SAME event→session reducer
// as the normal finalize path (capture.ReduceEvents / capture.FinishSession)
// so recovered records cannot drift from finalized ones (ETCH-40 finding 9).
package recovery

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Stage-11-Agentics/etch/internal/capture"
)

const DefaultTimeoutHours = 4

// scanActivityGrace is the stat-first pre-filter: a wip whose mtime is this
// recent is presumed live and skipped without opening it. Every appended
// event refreshes the mtime, so an active session never gets past the stat
// (the scan used to fully JSON-parse every wip — including live ones — on
// every session_start; at 60–80 concurrent agents that's O(sessions×events)
// per start, ETCH-36).
const scanActivityGrace = 5 * time.Minute

type RefWriter interface {
	WriteSessionRef(repoDir string, session *capture.Session) error
}

type NoOpRefWriter struct{}

func (NoOpRefWriter) WriteSessionRef(string, *capture.Session) error { return nil }

type OrphanedWIP struct {
	Path      string
	SessionID string
	LastEvent time.Time
	Reason    string // "dead_pid" or "timeout"
}

// ScanOrphaned returns the .wip files in sessionsDir that belong to sessions
// which are no longer running. Liveness policy (ETCH-40 finding 1):
//
//   - A wip touched within scanActivityGrace is presumed live — skipped on
//     the stat alone, never opened.
//   - A recorded agent PID that is verifiably alive (same PID, same process
//     start time) ALWAYS vetoes recovery, even past the idle timeout: an
//     alive agent can still end its session normally, and recovering it
//     would destroy its live wip and double-record the session. The wip of
//     a hung-but-alive agent therefore stays uncommitted until the process
//     exits — the deliberate lesser evil, logged for visibility.
//   - A recorded PID that is dead (or whose start time mismatches — PID
//     reuse) marks the wip orphaned as "dead_pid" without waiting for the
//     full timeout.
//   - No recorded PID (0): the idle timeout governs, judged on file mtime —
//     every appended event refreshes it.
func ScanOrphaned(sessionsDir string, timeout time.Duration) ([]OrphanedWIP, error) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading sessions dir: %w", err)
	}

	now := time.Now()
	var orphaned []OrphanedWIP

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".wip.jsonl") {
			continue
		}

		fi, err := entry.Info()
		if err != nil {
			continue
		}
		idle := now.Sub(fi.ModTime())
		if idle < scanActivityGrace {
			continue // recently active — presumed live, not even opened
		}

		path := filepath.Join(sessionsDir, entry.Name())
		sessionID := strings.TrimSuffix(entry.Name(), ".wip.jsonl")

		pid, pidStart, ok := readWipHeader(path)
		if !ok {
			log.Printf("recovery: skipping unreadable wip file %s", entry.Name())
			continue
		}

		if pid > 0 {
			if sessionAlive(pid, pidStart) {
				if idle > timeout {
					log.Printf("recovery: %s idle %s (past timeout) but agent pid %d is alive — not recovering", entry.Name(), idle.Round(time.Minute), pid)
				}
				continue
			}
			orphaned = append(orphaned, OrphanedWIP{
				Path:      path,
				SessionID: sessionID,
				LastEvent: fi.ModTime(),
				Reason:    "dead_pid",
			})
			continue
		}

		if idle > timeout {
			orphaned = append(orphaned, OrphanedWIP{
				Path:      path,
				SessionID: sessionID,
				LastEvent: fi.ModTime(),
				Reason:    "timeout",
			})
		}
	}

	return orphaned, nil
}

// readWipHeader reads only the wip's first valid event line and extracts the
// recorded agent PID + start time when that line is the session_start. The
// scan never full-parses a wip — the first line is written first and carries
// everything liveness needs.
func readWipHeader(path string) (pid int, pidStart string, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev capture.HookEvent
		if json.Unmarshal([]byte(line), &ev) != nil || ev.Hook == "" {
			continue
		}
		if ev.Hook == "session_start" && ev.Data != nil {
			var d struct {
				PID          int    `json:"pid"`
				PIDStartTime string `json:"pid_start_time"`
			}
			if json.Unmarshal(ev.Data, &d) == nil {
				return d.PID, d.PIDStartTime, true
			}
		}
		// First valid event is not a usable session_start: no PID recorded.
		return 0, "", true
	}
	return 0, "", false
}

// sessionAlive reports whether the recorded agent process is verifiably the
// same process and still running. A start-time mismatch means the PID was
// recycled by another process — that cannot veto recovery. When the start
// time cannot be read for a live PID, we err on the side of the live
// session (veto): destroying a live wip is the worse failure.
func sessionAlive(pid int, recordedStart string) bool {
	if !processAlive(pid) {
		return false
	}
	if recordedStart == "" {
		return true
	}
	start, ok := capture.ProcessStartTime(pid)
	if !ok {
		return true
	}
	return start == recordedStart
}

// RecoverSession reduces an orphaned wip into a Session via the shared
// reducer. Two cases:
//
//   - The wip contains an end event (a session that ended normally but whose
//     ref commit failed — ETCH-40 finding 8): the reduced record is already
//     truthful (complete, recorded exit_reason and git_end); recovery commits
//     it as-is. The files diff is bounded by the recorded SHAs and runs in
//     the session's own worktree when it still exists.
//
//   - No end event (a true crash): status/exit_reason are overridden to
//     incomplete/crash; git_end is the wip's last known git snapshot — a copy
//     of git_start with no commits_produced (OUTPUT_SPEC §2c). No live git is
//     consulted: capturing state hours later would attribute other sessions'
//     intervening work to this record. files_touched falls back to
//     tool-reported paths.
func RecoverSession(repoRoot, sessionID string) (*capture.Session, error) {
	events, err := capture.ReadEvents(repoRoot, sessionID)
	if err != nil {
		return nil, fmt.Errorf("reading wip events: %w", err)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("wip file is empty or contains no valid events")
	}

	session, info := capture.ReduceEvents(sessionID, events)

	workDir := ""
	if info.HasEnd {
		if session.GitStart != nil && dirExists(session.GitStart.WorktreePath) {
			workDir = session.GitStart.WorktreePath
		}
	} else {
		session.Status = "incomplete"
		session.ExitReason = "crash"
		if session.GitStart != nil && (session.GitStart.Branch != "" || session.GitStart.HeadSHA != "") {
			session.GitEnd = &capture.GitState{
				Branch:  session.GitStart.Branch,
				HeadSHA: session.GitStart.HeadSHA,
			}
		}
	}

	capture.FinishSession(session, info, workDir)
	return session, nil
}

func dirExists(path string) bool {
	if path == "" {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func CleanupWIP(wipPath string) error {
	return os.Remove(wipPath)
}

func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// RecoverAll scans repoRoot's sessions dir for orphaned .wip files and
// commits them via writer. exclude lists session ULIDs that must not be
// recovered in this invocation (e.g. the wip a duplicate session_start is
// about to resume). Returns the number of sessions recovered and any error
// from the scan itself.
func RecoverAll(repoRoot string, timeout time.Duration, writer RefWriter, exclude map[string]bool) (int, error) {
	sessionsDir := filepath.Join(repoRoot, ".etch", "sessions")
	orphaned, err := ScanOrphaned(sessionsDir, timeout)
	if err != nil {
		return 0, err
	}

	recovered := 0
	for _, wip := range orphaned {
		if exclude[wip.SessionID] {
			continue
		}

		session, recErr := RecoverSession(repoRoot, wip.SessionID)
		if recErr != nil {
			log.Printf("recovery: failed to recover %s: %v", filepath.Base(wip.Path), recErr)
			continue
		}

		if writeErr := writer.WriteSessionRef(repoRoot, session); writeErr != nil {
			log.Printf("recovery: failed to write ref for %s: %v", wip.SessionID, writeErr)
			continue
		}

		if cleanErr := CleanupWIP(wip.Path); cleanErr != nil {
			log.Printf("recovery: failed to cleanup %s: %v", filepath.Base(wip.Path), cleanErr)
		}
		capture.RemoveSessionJSON(repoRoot, wip.SessionID)
		capture.CleanupMappingByULID(repoRoot, wip.SessionID)

		recovered++
	}

	return recovered, nil
}

// ReadTimeoutFromSettings reads recovery_timeout_hours from .etch/settings.json.
// Returns the default timeout if the file doesn't exist or doesn't contain the field.
func ReadTimeoutFromSettings(repoDir string) time.Duration {
	settingsPath := filepath.Join(repoDir, ".etch", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return DefaultTimeoutHours * time.Hour
	}

	var settings struct {
		RecoveryTimeoutHours json.Number `json:"recovery_timeout_hours"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return DefaultTimeoutHours * time.Hour
	}

	if settings.RecoveryTimeoutHours == "" {
		return DefaultTimeoutHours * time.Hour
	}

	hours, err := strconv.ParseFloat(string(settings.RecoveryTimeoutHours), 64)
	if err != nil || hours <= 0 {
		return DefaultTimeoutHours * time.Hour
	}

	return time.Duration(hours * float64(time.Hour))
}

// WipAgentAlive probes one wip buffer's recorded agent process: alive
// reports whether it is verifiably the same process and still running;
// known=false when the wip records no usable PID (liveness can't be
// determined). Doctor uses this to tell live sessions from true orphans —
// an old wip with a live agent is a long-running session, not a recovery
// failure.
func WipAgentAlive(path string) (alive, known bool) {
	pid, pidStart, ok := readWipHeader(path)
	if !ok || pid <= 0 {
		return false, false
	}
	return sessionAlive(pid, pidStart), true
}
