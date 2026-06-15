package importer

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Stage-11-Agentics/etch/internal/capture"
	"github.com/Stage-11-Agentics/etch/internal/testutil"
)

// writeClaudeTranscript writes a Claude Code JSONL transcript into a project
// subdir of root and returns the parser rooted there.
func writeClaudeTranscript(t *testing.T, root, name string, lines []string) {
	t.Helper()
	dir := filepath.Join(root, "proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeParserParse(t *testing.T) {
	root := t.TempDir()
	cwd := "/work/repo"
	writeClaudeTranscript(t, root, "s.jsonl", []string{
		`{"type":"user","sessionId":"UP-1","cwd":"` + cwd + `","gitBranch":"main","timestamp":"2026-06-10T10:00:00.000Z","version":"2.1.170","message":{"role":"user","content":"Do the thing"}}`,
		`{"type":"assistant","sessionId":"UP-1","cwd":"` + cwd + `","timestamp":"2026-06-10T10:00:05.000Z","message":{"role":"assistant","model":"claude-opus-4-8","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/work/repo/a.txt"}},{"type":"tool_use","name":"Read","input":{"file_path":"/work/repo/b.txt"}}]}}`,
		`{"type":"user","sessionId":"UP-1","cwd":"` + cwd + `","timestamp":"2026-06-10T10:01:00.000Z","message":{"role":"user","content":[{"type":"tool_result","content":"ok"}]}}`,
	})

	p := &ClaudeParser{Root: root}
	parsed, err := p.Parse(filepath.Join(root, "proj", "s.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed == nil {
		t.Fatal("expected a parsed session")
	}
	s := parsed.Session

	if parsed.Cwd != cwd {
		t.Errorf("cwd: got %q want %q", parsed.Cwd, cwd)
	}
	if s.AgentSessionID == nil || *s.AgentSessionID != "UP-1" {
		t.Errorf("agent_session_id: got %v want UP-1", s.AgentSessionID)
	}
	if s.Agent.Runtime != "claude-code" {
		t.Errorf("runtime: got %q", s.Agent.Runtime)
	}
	if s.Agent.Model == nil || *s.Agent.Model != "claude-opus-4-8" {
		t.Errorf("model: got %v", s.Agent.Model)
	}
	if s.Prompt == nil || s.Prompt.Text != "Do the thing" {
		t.Errorf("prompt: got %v", s.Prompt)
	}
	if s.Capture.Method != capture.CaptureMethodImport {
		t.Errorf("capture.method: got %q want import", s.Capture.Method)
	}
	if s.Capture.Fidelity != capture.FidelityFull {
		t.Errorf("capture.fidelity: got %q want full (tools present)", s.Capture.Fidelity)
	}
	if s.ToolUse.TotalCalls != 2 {
		t.Errorf("tool calls: got %d want 2", s.ToolUse.TotalCalls)
	}
	if len(s.FilesTouched) != 2 {
		t.Errorf("files_touched: got %d want 2", len(s.FilesTouched))
	}
	if s.Timing.StartedAt != "2026-06-10T10:00:00.000Z" {
		t.Errorf("started_at: got %q", s.Timing.StartedAt)
	}
	if s.Timing.DurationMs == nil || *s.Timing.DurationMs != 60000 {
		t.Errorf("duration: got %v want 60000", s.Timing.DurationMs)
	}
	// ULID timestamp component should track the session start, not now.
	if s.SessionID == "" {
		t.Error("session id (ULID) not minted")
	}
}

func TestClaudeParserFidelitySessionOnly(t *testing.T) {
	root := t.TempDir()
	writeClaudeTranscript(t, root, "s.jsonl", []string{
		`{"type":"user","sessionId":"UP-2","cwd":"/x","timestamp":"2026-06-10T10:00:00.000Z","message":{"role":"user","content":"hi"}}`,
		`{"type":"assistant","sessionId":"UP-2","cwd":"/x","timestamp":"2026-06-10T10:00:05.000Z","message":{"role":"assistant","model":"m","content":[{"type":"text","text":"hello"}]}}`,
	})
	p := &ClaudeParser{Root: root}
	parsed, err := p.Parse(filepath.Join(root, "proj", "s.jsonl"))
	if err != nil || parsed == nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Session.Capture.Fidelity != capture.FidelitySessionOnly {
		t.Errorf("fidelity: got %q want session_only (no tools)", parsed.Session.Capture.Fidelity)
	}
}

func TestClaudeParserSkipsUnusable(t *testing.T) {
	root := t.TempDir()
	// No sessionId anywhere → not a usable session.
	writeClaudeTranscript(t, root, "s.jsonl", []string{
		`{"type":"summary","summary":"x"}`,
	})
	p := &ClaudeParser{Root: root}
	parsed, err := p.Parse(filepath.Join(root, "proj", "s.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed != nil {
		t.Errorf("expected nil for transcript with no session id, got %+v", parsed)
	}
}

func TestCodexParserParse(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "2026", "06", "14")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"type":"session_meta","timestamp":"2026-06-14T23:15:19.000Z","payload":{"id":"CX-1","cwd":"/work/repo","cli_version":"0.139.0"}}`,
		`{"type":"turn_context","timestamp":"2026-06-14T23:15:20.000Z","payload":{"model":"gpt-5.5"}}`,
		`{"type":"response_item","timestamp":"2026-06-14T23:15:21.000Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"build it"}]}}`,
		`{"type":"response_item","timestamp":"2026-06-14T23:15:25.000Z","payload":{"type":"function_call","name":"shell","arguments":"{\"file_path\":\"/work/repo/x.go\"}"}}`,
	}
	if err := os.WriteFile(filepath.Join(dir, "rollout-x.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &CodexParser{Root: root}
	files, err := p.Discover()
	if err != nil || len(files) != 1 {
		t.Fatalf("discover: %v files=%v", err, files)
	}
	parsed, err := p.Parse(files[0])
	if err != nil || parsed == nil {
		t.Fatalf("parse: %v", err)
	}
	s := parsed.Session
	if s.AgentSessionID == nil || *s.AgentSessionID != "CX-1" {
		t.Errorf("agent_session_id: got %v", s.AgentSessionID)
	}
	if s.Agent.Runtime != "codex" {
		t.Errorf("runtime: got %q", s.Agent.Runtime)
	}
	if s.Agent.Model == nil || *s.Agent.Model != "gpt-5.5" {
		t.Errorf("model: got %v", s.Agent.Model)
	}
	if s.Prompt == nil || s.Prompt.Text != "build it" {
		t.Errorf("prompt: got %v", s.Prompt)
	}
	if s.ToolUse.TotalCalls != 1 || s.ToolUse.ByTool["shell"] != 1 {
		t.Errorf("tool_use: got %+v", s.ToolUse)
	}
	if len(s.FilesTouched) != 1 || s.FilesTouched[0].Path != "/work/repo/x.go" {
		t.Errorf("files: got %+v", s.FilesTouched)
	}
	if parsed.Cwd != "/work/repo" {
		t.Errorf("cwd: got %q", parsed.Cwd)
	}
}

// readRecord reads a committed session.json for a given ULID from the repo.
func readRecord(t *testing.T, repo, ulid string) capture.Session {
	t.Helper()
	cmd := exec.Command("git", "show", "refs/etch/sessions/"+ulid+":session.json")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git show: %v", err)
	}
	var s capture.Session
	if err := json.Unmarshal(out, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func listSessionULIDs(t *testing.T, repo string) []string {
	t.Helper()
	cmd := exec.Command("git", "for-each-ref", "refs/etch/sessions/", "--format=%(refname:short)")
	cmd.Dir = repo
	out, _ := cmd.Output()
	var ids []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l == "" {
			continue
		}
		ids = append(ids, l[strings.LastIndex(l, "/")+1:])
	}
	return ids
}

func TestRunCommitsAndDedups(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	root := t.TempDir()
	writeClaudeTranscript(t, root, "s.jsonl", []string{
		`{"type":"user","sessionId":"UP-1","cwd":"` + repo + `","timestamp":"2026-06-10T10:00:00.000Z","message":{"role":"user","content":"hi"}}`,
		`{"type":"assistant","sessionId":"UP-1","cwd":"` + repo + `","timestamp":"2026-06-10T10:00:05.000Z","message":{"role":"assistant","model":"m","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"` + repo + `/a.txt"}}]}}`,
	})
	parsers := []Parser{&ClaudeParser{Root: root}}

	res, err := runImport(Options{RepoRoot: repo}, parsers, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 1 {
		t.Fatalf("first run: imported %d want 1", res.Imported)
	}
	ids := listSessionULIDs(t, repo)
	if len(ids) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(ids))
	}
	rec := readRecord(t, repo, ids[0])
	if rec.Capture.Method != capture.CaptureMethodImport {
		t.Errorf("committed capture.method: got %q", rec.Capture.Method)
	}
	if rec.Machine.OS == "" {
		t.Error("machine identity not stamped at commit")
	}

	// Second run: same transcript, must dedup (hooks/prior-import win).
	res2, err := runImport(Options{RepoRoot: repo}, parsers, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Imported != 0 || res2.Skipped != 1 {
		t.Errorf("second run: imported=%d skipped=%d want 0/1", res2.Imported, res2.Skipped)
	}
	if got := len(listSessionULIDs(t, repo)); got != 1 {
		t.Errorf("after dedup re-run: %d refs want 1", got)
	}
}

func TestRunOutOfRepoSkipped(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	root := t.TempDir()
	writeClaudeTranscript(t, root, "s.jsonl", []string{
		`{"type":"user","sessionId":"UP-9","cwd":"/somewhere/else","timestamp":"2026-06-10T10:00:00.000Z","message":{"role":"user","content":"hi"}}`,
	})
	res, err := runImport(Options{RepoRoot: repo}, []Parser{&ClaudeParser{Root: root}}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 0 || res.OutOfRepo != 1 {
		t.Errorf("imported=%d outOfRepo=%d want 0/1", res.Imported, res.OutOfRepo)
	}
}

func TestRunSinceFilter(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	root := t.TempDir()
	writeClaudeTranscript(t, root, "old.jsonl", []string{
		`{"type":"user","sessionId":"OLD","cwd":"` + repo + `","timestamp":"2026-06-01T10:00:00.000Z","message":{"role":"user","content":"hi"}}`,
	})
	root2 := t.TempDir()
	writeClaudeTranscript(t, root2, "new.jsonl", []string{
		`{"type":"user","sessionId":"NEW","cwd":"` + repo + `","timestamp":"2026-06-12T10:00:00.000Z","message":{"role":"user","content":"hi"}}`,
	})
	since, _ := time.Parse(time.RFC3339, "2026-06-10T00:00:00Z")
	res, err := runImport(Options{RepoRoot: repo, Since: since},
		[]Parser{&ClaudeParser{Root: root}, &ClaudeParser{Root: root2}}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 1 {
		t.Errorf("imported=%d want 1 (only the post-since session)", res.Imported)
	}
}

func TestRunDryRunWritesNothing(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	root := t.TempDir()
	writeClaudeTranscript(t, root, "s.jsonl", []string{
		`{"type":"user","sessionId":"UP-D","cwd":"` + repo + `","timestamp":"2026-06-10T10:00:00.000Z","message":{"role":"user","content":"hi"}}`,
	})
	res, err := runImport(Options{RepoRoot: repo, DryRun: true}, []Parser{&ClaudeParser{Root: root}}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 1 {
		t.Errorf("dry-run imported (reported)=%d want 1", res.Imported)
	}
	if got := len(listSessionULIDs(t, repo)); got != 0 {
		t.Errorf("dry-run wrote %d refs, want 0", got)
	}
}

func TestExtractAgentSessionID(t *testing.T) {
	id, err := extractAgentSessionID([]byte(`{"agent_session_id":"abc"}`))
	if err != nil || id != "abc" {
		t.Errorf("got %q %v", id, err)
	}
	id, err = extractAgentSessionID([]byte(`{"agent_session_id":null}`))
	if err != nil || id != "" {
		t.Errorf("null: got %q %v", id, err)
	}
}
