package capture

import (
	"os"
	"testing"
)

// fabricated process tables for selectAgentPID (plan-review R2: the matcher
// is pure logic over an injectable reader, tested against ancestry shapes).
func tableLookup(table map[int]procInfo) func(int) (procInfo, bool) {
	return func(pid int) (procInfo, bool) {
		p, ok := table[pid]
		return p, ok
	}
}

func TestSelectAgentPID_DirectAgentParent(t *testing.T) {
	table := map[int]procInfo{
		100: {PPID: 50, Start: "Sat Jun  6 10:00:00 2026", Comm: "/usr/local/bin/claude"},
	}
	pid, start := selectAgentPID(100, tableLookup(table))
	if pid != 100 {
		t.Fatalf("want pid 100, got %d", pid)
	}
	if start != "Sat Jun  6 10:00:00 2026" {
		t.Errorf("want claude's start time, got %q", start)
	}
}

func TestSelectAgentPID_TransientRunnerChain(t *testing.T) {
	// etch's parent is a per-invocation entire runner, whose parent is a
	// shell, whose parent is the long-lived claude. The matcher must walk
	// PAST entire and sh (no allowlist match) and pin claude.
	table := map[int]procInfo{
		300: {PPID: 200, Start: "s3", Comm: "entire"},
		200: {PPID: 100, Start: "s2", Comm: "/bin/sh"},
		100: {PPID: 1, Start: "s1", Comm: "claude"},
	}
	pid, start := selectAgentPID(300, tableLookup(table))
	if pid != 100 {
		t.Fatalf("want claude's pid 100, got %d", pid)
	}
	if start != "s1" {
		t.Errorf("want s1, got %q", start)
	}
}

func TestSelectAgentPID_NoAgentAncestor(t *testing.T) {
	// A bare shell chain (manual run): nothing matches → 0, timeout governs.
	table := map[int]procInfo{
		300: {PPID: 200, Start: "s3", Comm: "zsh"},
		200: {PPID: 100, Start: "s2", Comm: "Terminal"},
		100: {PPID: 1, Start: "s1", Comm: "launchd"},
	}
	pid, start := selectAgentPID(300, tableLookup(table))
	if pid != 0 || start != "" {
		t.Fatalf("want (0, \"\"), got (%d, %q)", pid, start)
	}
}

func TestSelectAgentPID_GenericNamesNeverMatch(t *testing.T) {
	// node and entire are ambiguous (transient runner OR over-durable
	// supervisor) — they must NOT be selected, even when nothing else matches.
	table := map[int]procInfo{
		300: {PPID: 200, Start: "s3", Comm: "node"},
		200: {PPID: 100, Start: "s2", Comm: "entire"},
		100: {PPID: 1, Start: "s1", Comm: "init"},
	}
	pid, _ := selectAgentPID(300, tableLookup(table))
	if pid != 0 {
		t.Fatalf("generic names must not match; got pid %d", pid)
	}
}

func TestSelectAgentPID_UnreadableTable(t *testing.T) {
	pid, start := selectAgentPID(300, tableLookup(map[int]procInfo{}))
	if pid != 0 || start != "" {
		t.Fatalf("want (0, \"\"), got (%d, %q)", pid, start)
	}
}

func TestSelectAgentPID_CycleBounded(t *testing.T) {
	// A corrupt table with a ppid cycle must terminate.
	table := map[int]procInfo{
		300: {PPID: 200, Start: "s3", Comm: "a"},
		200: {PPID: 300, Start: "s2", Comm: "b"},
	}
	pid, _ := selectAgentPID(300, tableLookup(table))
	if pid != 0 {
		t.Fatalf("want 0 on cycle, got %d", pid)
	}
}

func TestParseProcLine(t *testing.T) {
	p, ok := parseProcLine("  123 Sat Jun  6 10:00:00 2026 /usr/local/bin/claude code\n")
	if !ok {
		t.Fatal("expected parse to succeed")
	}
	if p.PPID != 123 {
		t.Errorf("ppid: want 123, got %d", p.PPID)
	}
	if p.Start != "Sat Jun 6 10:00:00 2026" {
		t.Errorf("start: got %q", p.Start)
	}
	if p.Comm != "/usr/local/bin/claude code" {
		t.Errorf("comm: got %q", p.Comm)
	}
}

func TestParseProcLine_Garbage(t *testing.T) {
	if _, ok := parseProcLine("nope"); ok {
		t.Error("expected parse failure")
	}
	if _, ok := parseProcLine(""); ok {
		t.Error("expected parse failure on empty")
	}
}

// TestProcessStartTime_RealProcess: the live ps path round-trips for our own
// process, and the value is stable across two reads (the property the
// liveness equality check depends on).
func TestProcessStartTime_RealProcess(t *testing.T) {
	s1, ok := ProcessStartTime(os.Getpid())
	if !ok || s1 == "" {
		t.Fatalf("expected readable start time for own pid, got ok=%v %q", ok, s1)
	}
	s2, ok := ProcessStartTime(os.Getpid())
	if !ok || s1 != s2 {
		t.Fatalf("start time must be stable: %q vs %q", s1, s2)
	}
}
