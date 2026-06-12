package testutil

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Stage-11-Agentics/etch/internal/refs"
	"github.com/Stage-11-Agentics/etch/internal/schema"
)

// NewTestRepo creates a temporary git repo and registers cleanup with t.Cleanup.
func NewTestRepo(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.local")
	run(t, dir, "git", "config", "user.name", "Test")
	return dir
}

// BinaryResult holds stdout, stderr, and exit code from a binary invocation.
type BinaryResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// RunBinary builds and runs entire-agent-etch in the given directory with the
// specified subcommand and optional stdin JSON. The binary is built once per test
// and cached in the test's temp directory.
func RunBinary(t testing.TB, dir string, args []string, stdinJSON string) BinaryResult {
	t.Helper()
	binPath := buildBinary(t)

	cmd := exec.Command(binPath, args...)
	cmd.Dir = dir
	if stdinJSON != "" {
		cmd.Stdin = strings.NewReader(stdinJSON)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("running binary: %v", err)
		}
	}

	return BinaryResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
}

// MustParseJSON parses s as JSON into a map. Fails the test if parsing fails.
func MustParseJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &m); err != nil {
		t.Fatalf("invalid JSON %q: %v", s, err)
	}
	return m
}

// RunBinaryWithEnv is like RunBinary but adds extra environment variables.
func RunBinaryWithEnv(t testing.TB, dir string, args []string, stdinJSON string, env map[string]string) BinaryResult {
	t.Helper()
	binPath := buildBinary(t)

	cmd := exec.Command(binPath, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if stdinJSON != "" {
		cmd.Stdin = strings.NewReader(stdinJSON)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("running binary: %v", err)
		}
	}

	return BinaryResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
}

// WriteSession seeds a etch session ref in repo from a schema.Session. It
// fills SchemaVersion if empty, marshals the session to session.json, derives a
// minimal RefMeta + agent-trace, and writes the ref via refs.WriteSessionRef.
// Shared infrastructure for query/* and other tests that need realistic refs.
func WriteSession(t testing.TB, repo string, s schema.Session) {
	t.Helper()
	if s.SchemaVersion == "" {
		s.SchemaVersion = schema.SchemaVersion
	}
	sessionJSON, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}

	trace := []byte(`{"version":"1.0","traces":[{"agent_id":"` + s.Agent.Runtime + `","session_id":"` + s.SessionID + `"}]}`)

	meta := refs.RefMeta{
		Runtime: s.Agent.Runtime,
		Status:  s.Status,
		EndTime: time.Unix(1700000000, 0),
	}
	if s.Agent.Model != nil {
		meta.Model = *s.Agent.Model
	}
	if s.GitStart != nil {
		meta.Branch = s.GitStart.Branch
	}
	if s.Timing.DurationMS != nil {
		meta.DurationSecs = int(*s.Timing.DurationMS / 1000)
	}
	if s.Timing.EndedAt != nil {
		if et, perr := time.Parse(time.RFC3339, *s.Timing.EndedAt); perr == nil {
			meta.EndTime = et
		}
	}

	if err := refs.WriteSessionRef(repo, s.SessionID, sessionJSON, trace, meta); err != nil {
		t.Fatalf("WriteSessionRef(%s): %v", s.SessionID, err)
	}
}

// StrPtr returns a pointer to s, a convenience for building schema.Session
// literals in tests.
func StrPtr(s string) *string { return &s }

// Int64Ptr returns a pointer to n.
func Int64Ptr(n int64) *int64 { return &n }

// BinaryPath builds (or reuses) the entire-agent-etch test binary and
// returns its path. For tests that need the binary reachable on PATH --
// e.g. a git hook or an `sh -c` dispatch command invoking it by name --
// prepend filepath.Dir of this to the subprocess PATH.
func BinaryPath(t testing.TB) string {
	t.Helper()
	return buildBinary(t)
}

// RunCmd runs an arbitrary command in the given directory. Exported for use by other test packages.
func RunCmd(t testing.TB, dir string, name string, args ...string) {
	t.Helper()
	run(t, dir, name, args...)
}

var cachedBinaryPath string

func buildBinary(t testing.TB) string {
	t.Helper()
	if cachedBinaryPath != "" {
		if _, err := os.Stat(cachedBinaryPath); err == nil {
			return cachedBinaryPath
		}
	}

	dir := t.TempDir()
	binPath := filepath.Join(dir, "entire-agent-etch")
	moduleRoot := findModuleRoot(t)

	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/entire-agent-etch")
	cmd.Dir = moduleRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	cachedBinaryPath = binPath
	return binPath
}

func findModuleRoot(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod")
		}
		dir = parent
	}
}

func run(t testing.TB, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}
