package capture

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Agent-runtime liveness identity (ETCH-40 finding 1).
//
// The Entire hook payload carries no PID, and the hook binary's direct parent
// is often a transient runner (a per-invocation `entire` process or a shell),
// so neither is a usable liveness anchor. Instead, session_start walks the
// ancestor chain looking for a process whose name is unambiguously the agent
// runtime itself — the process whose lifetime equals the session's.
//
// The allowlist is deliberately strict (plan-review R2): generic names like
// `node` or `entire` can be either transient hook-runners or over-durable
// supervisors, and both failure modes are worse than not recording a PID at
// all. A transient match would flag a LIVE session dead (recovery destroys
// its wip — the very bug this fixes); an over-durable match would block a
// crashed session's recovery forever. When no specific agent name matches,
// we record 0: unknown, and the idle timeout governs recovery as before.
//
// The recorded start time makes the liveness check immune to PID reuse: a
// recycled PID is alive but has a different start time, so it cannot veto
// recovery on behalf of a dead agent.
var agentRuntimeNames = map[string]bool{
	"claude":      true,
	"claude-code": true,
	"codex":       true,
	"gemini":      true,
}

// procInfo is one process-table row, as read by readProc.
type procInfo struct {
	PPID  int
	Start string // ps lstart, e.g. "Sat Jun  7 12:34:56 2026"
	Comm  string
}

// CaptureAgentPID returns the PID and start time of the nearest ancestor
// that is a known agent runtime, or (0, "") when none can be identified.
func CaptureAgentPID() (int, string) {
	return selectAgentPID(os.Getppid(), readProc)
}

// selectAgentPID walks the ancestor chain from fromPID via lookup and picks
// the first process whose command name is a known agent runtime. Pure logic
// over an injectable process-table reader, so it is testable against
// fabricated ancestry tables.
func selectAgentPID(fromPID int, lookup func(int) (procInfo, bool)) (int, string) {
	pid := fromPID
	for depth := 0; depth < 16 && pid > 1; depth++ {
		p, ok := lookup(pid)
		if !ok {
			return 0, ""
		}
		if agentRuntimeNames[normalizeComm(p.Comm)] {
			return pid, p.Start
		}
		pid = p.PPID
	}
	return 0, ""
}

func normalizeComm(comm string) string {
	return strings.ToLower(filepath.Base(strings.TrimSpace(comm)))
}

// readProc reads one process-table row. ps is universal across macOS/Linux;
// comm comes last because it may contain spaces.
func readProc(pid int) (procInfo, bool) {
	out, err := exec.Command("ps", "-o", "ppid=,lstart=,comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return procInfo{}, false
	}
	return parseProcLine(string(out))
}

// parseProcLine parses "PPID LSTART(5 tokens) COMM...".
func parseProcLine(line string) (procInfo, bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 7 {
		return procInfo{}, false
	}
	ppid, err := strconv.Atoi(fields[0])
	if err != nil {
		return procInfo{}, false
	}
	return procInfo{
		PPID:  ppid,
		Start: strings.Join(fields[1:6], " "),
		Comm:  strings.Join(fields[6:], " "),
	}, true
}

// ProcessStartTime returns the ps start time for a live PID, in the same
// representation CaptureAgentPID records, for equality comparison.
func ProcessStartTime(pid int) (string, bool) {
	p, ok := readProc(pid)
	if !ok {
		return "", false
	}
	return p.Start, true
}
