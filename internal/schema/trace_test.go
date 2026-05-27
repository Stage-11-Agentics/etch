package schema

import (
	"encoding/json"
	"testing"
)

func strPtr(s string) *string { return &s }

func TestSessionToAgentTrace_CompleteSession(t *testing.T) {
	endedAt := "2026-05-26T14:47:22.109Z"
	session := &Session{
		SessionID: "01JWB8K3XQPNR7TV0ZYM4GD2AH",
		Agent: Agent{
			Runtime: "claude-code",
			Model:   strPtr("claude-opus-4-7"),
		},
		Timing: Timing{
			StartedAt: strPtr("2026-05-26T14:32:08.441Z"),
			EndedAt:   &endedAt,
		},
		FilesTouched: []FileEntry{
			{Path: "src/components/LoginButton.tsx", Action: "added"},
			{Path: "src/components/LoginButton.test.tsx", Action: "added"},
			{Path: "src/pages/auth/login.tsx", Action: "modified"},
		},
	}

	trace := SessionToAgentTrace(session)

	if trace.Version != "1.0" {
		t.Errorf("version = %q, want %q", trace.Version, "1.0")
	}
	if len(trace.Traces) != 1 {
		t.Fatalf("len(traces) = %d, want 1", len(trace.Traces))
	}

	entry := trace.Traces[0]
	if entry.AgentID != "claude-code" {
		t.Errorf("agent_id = %q, want %q", entry.AgentID, "claude-code")
	}
	if entry.Model != "claude-opus-4-7" {
		t.Errorf("model = %q, want %q", entry.Model, "claude-opus-4-7")
	}
	if entry.SessionID != "01JWB8K3XQPNR7TV0ZYM4GD2AH" {
		t.Errorf("session_id = %q, want %q", entry.SessionID, "01JWB8K3XQPNR7TV0ZYM4GD2AH")
	}
	if entry.Timestamp != endedAt {
		t.Errorf("timestamp = %q, want %q (EndedAt)", entry.Timestamp, endedAt)
	}

	wantFiles := []string{
		"src/components/LoginButton.tsx",
		"src/components/LoginButton.test.tsx",
		"src/pages/auth/login.tsx",
	}
	if len(entry.Files) != len(wantFiles) {
		t.Fatalf("len(files) = %d, want %d", len(entry.Files), len(wantFiles))
	}
	for i, f := range entry.Files {
		if f != wantFiles[i] {
			t.Errorf("files[%d] = %q, want %q", i, f, wantFiles[i])
		}
	}
}

func TestSessionToAgentTrace_VersionAlways1_0(t *testing.T) {
	for _, runtime := range []string{"claude-code", "codex", "gemini-cli"} {
		session := &Session{
			SessionID: "01TEST",
			Agent:     Agent{Runtime: runtime, Model: strPtr("some-model")},
			Timing:    Timing{StartedAt: strPtr("2026-01-01T00:00:00Z")},
		}
		trace := SessionToAgentTrace(session)
		if trace.Version != "1.0" {
			t.Errorf("runtime=%s: version = %q, want %q", runtime, trace.Version, "1.0")
		}
	}
}

func TestSessionToAgentTrace_EmptyFiles(t *testing.T) {
	session := &Session{
		SessionID:    "01EMPTY",
		Agent:        Agent{Runtime: "claude-code", Model: strPtr("claude-opus-4-7")},
		Timing:       Timing{StartedAt: strPtr("2026-01-01T00:00:00Z")},
		FilesTouched: []FileEntry{},
	}

	trace := SessionToAgentTrace(session)
	entry := trace.Traces[0]

	if entry.Files == nil {
		t.Fatal("files should be empty slice, not nil")
	}
	if len(entry.Files) != 0 {
		t.Errorf("len(files) = %d, want 0", len(entry.Files))
	}
}

func TestSessionToAgentTrace_NilFilesTouched(t *testing.T) {
	session := &Session{
		SessionID:    "01NILFILES",
		Agent:        Agent{Runtime: "codex", Model: strPtr("o3")},
		Timing:       Timing{StartedAt: strPtr("2026-01-01T00:00:00Z")},
		FilesTouched: nil,
	}

	trace := SessionToAgentTrace(session)
	entry := trace.Traces[0]

	if entry.Files == nil {
		t.Fatal("files should be empty slice, not nil")
	}
	if len(entry.Files) != 0 {
		t.Errorf("len(files) = %d, want 0", len(entry.Files))
	}
}

func TestSessionToAgentTrace_IncompleteSession_UsesStartedAt(t *testing.T) {
	startedAt := "2026-05-26T22:10:33.008Z"
	session := &Session{
		SessionID: "01CRASHED",
		Status:    "incomplete",
		Agent:     Agent{Runtime: "codex", Model: strPtr("o3")},
		Timing: Timing{
			StartedAt: &startedAt,
			EndedAt:   nil,
		},
		FilesTouched: []FileEntry{
			{Path: "src/cache/redis_client.ts", Action: "added"},
		},
	}

	trace := SessionToAgentTrace(session)
	entry := trace.Traces[0]

	if entry.Timestamp != startedAt {
		t.Errorf("timestamp = %q, want %q (StartedAt fallback)", entry.Timestamp, startedAt)
	}
}

func TestSessionToAgentTrace_JSONRoundTrip(t *testing.T) {
	endedAt := "2026-05-26T14:47:22.109Z"
	session := &Session{
		SessionID: "01ROUNDTRIP",
		Agent:     Agent{Runtime: "claude-code", Model: strPtr("claude-sonnet-4-6")},
		Timing: Timing{
			StartedAt: strPtr("2026-05-26T14:32:08.441Z"),
			EndedAt:   &endedAt,
		},
		FilesTouched: []FileEntry{
			{Path: "main.go", Action: "modified"},
		},
	}

	trace := SessionToAgentTrace(session)

	data, err := json.Marshal(trace)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var roundTripped AgentTrace
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if roundTripped.Version != trace.Version {
		t.Errorf("version mismatch after round-trip: got %q, want %q", roundTripped.Version, trace.Version)
	}
	if len(roundTripped.Traces) != 1 {
		t.Fatalf("traces count mismatch after round-trip: got %d, want 1", len(roundTripped.Traces))
	}

	got := roundTripped.Traces[0]
	want := trace.Traces[0]
	if got.AgentID != want.AgentID {
		t.Errorf("agent_id mismatch: got %q, want %q", got.AgentID, want.AgentID)
	}
	if got.Model != want.Model {
		t.Errorf("model mismatch: got %q, want %q", got.Model, want.Model)
	}
	if got.SessionID != want.SessionID {
		t.Errorf("session_id mismatch: got %q, want %q", got.SessionID, want.SessionID)
	}
	if got.Timestamp != want.Timestamp {
		t.Errorf("timestamp mismatch: got %q, want %q", got.Timestamp, want.Timestamp)
	}
	if len(got.Files) != len(want.Files) {
		t.Fatalf("files count mismatch: got %d, want %d", len(got.Files), len(want.Files))
	}
	for i := range got.Files {
		if got.Files[i] != want.Files[i] {
			t.Errorf("files[%d] mismatch: got %q, want %q", i, got.Files[i], want.Files[i])
		}
	}
}

func TestSessionToAgentTrace_JSONFieldNames(t *testing.T) {
	session := &Session{
		SessionID: "01FIELDS",
		Agent:     Agent{Runtime: "gemini-cli", Model: strPtr("gemini-2.5-pro")},
		Timing:    Timing{StartedAt: strPtr("2026-01-01T00:00:00Z")},
	}

	trace := SessionToAgentTrace(session)
	data, err := json.Marshal(trace)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	if _, ok := raw["version"]; !ok {
		t.Error("missing JSON field: version")
	}
	traces, ok := raw["traces"].([]any)
	if !ok || len(traces) == 0 {
		t.Fatal("missing or empty JSON field: traces")
	}

	entry := traces[0].(map[string]any)
	for _, field := range []string{"agent_id", "model", "session_id", "files", "timestamp"} {
		if _, ok := entry[field]; !ok {
			t.Errorf("missing JSON field in trace entry: %s", field)
		}
	}
}

func TestSessionToAgentTrace_SingleTraceEntry(t *testing.T) {
	session := &Session{
		SessionID: "01SINGLE",
		Agent:     Agent{Runtime: "opencode", Model: strPtr("o3")},
		Timing:    Timing{StartedAt: strPtr("2026-01-01T00:00:00Z")},
	}

	trace := SessionToAgentTrace(session)
	if len(trace.Traces) != 1 {
		t.Errorf("len(traces) = %d, want exactly 1", len(trace.Traces))
	}
}
