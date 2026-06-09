package redact

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Stage-11-Agentics/etch/internal/schema"
)

func strPtr(s string) *string { return &s }

func sampleSession() *schema.Session {
	return &schema.Session{
		SchemaVersion: schema.SchemaVersion,
		SessionID:     "01TESTSTRIP000000000000000",
		Status:        "complete",
		ExitReason:    "normal",
		Agent: schema.Agent{
			Runtime: "claude-code",
			Model:   strPtr("claude-opus-4-8"),
		},
		Prompt: &schema.Prompt{
			Text:   "secret prompt text",
			Source: "interactive",
		},
		Orchestration: &schema.Orchestration{
			Type:     "lattice",
			TicketID: strPtr("ETCH-41"),
			Extra: map[string]any{
				"customer": "acme-corp",
				"nested":   map[string]any{"inner": "hidden"},
				"count":    3,
			},
		},
		Machine: &schema.Machine{
			HostnameHash: "sha256:abc",
			OS:           "darwin",
			OSVersion:    "Darwin 25.5.0",
			Arch:         "arm64",
		},
		GitStart: &schema.GitState{
			Branch:  "feat/secret-project",
			HeadSHA: "abc123",
		},
		FilesTouched: []schema.FileEntry{
			{Path: "/secret/path/a.go", Action: "edit"},
			{Path: "/secret/path/b.go", Action: "create"},
		},
		ToolUse: &schema.ToolUse{
			TotalCalls: 5,
			ByTool:     map[string]int{"Bash": 3, "Read": 2},
		},
	}
}

func TestStripStringLeaf(t *testing.T) {
	s := sampleSession()
	applied := StripLocalOnly(s, []string{"prompt.text"})

	if s.Prompt.Text != "[LOCAL_ONLY:prompt.text]" {
		t.Errorf("prompt.text = %q, want marker", s.Prompt.Text)
	}
	if s.Prompt.Source != "interactive" {
		t.Errorf("prompt.source should be untouched, got %q", s.Prompt.Source)
	}
	if !reflect.DeepEqual(applied, []string{"prompt.text"}) {
		t.Errorf("applied = %v", applied)
	}
}

// agent_session_id is deliberately strippable (decision recorded in the
// ETCH-41 review cycle): it is a nullable join key back to runtime
// transcripts, not an identity field — hiding transcript correlation is a
// legitimate use of local_only_fields. Etch's minted session_id stays
// canonical and protected.
func TestStripAgentSessionIDStrippable(t *testing.T) {
	s := sampleSession()
	s.AgentSessionID = strPtr("f3a9c2e1-7b4d-4e8a-9c0d-runtime-uuid")
	applied := StripLocalOnly(s, []string{"agent_session_id"})

	if s.AgentSessionID == nil || *s.AgentSessionID != "[LOCAL_ONLY:agent_session_id]" {
		t.Errorf("agent_session_id = %v, want marker", s.AgentSessionID)
	}
	if s.SessionID != "01TESTSTRIP000000000000000" {
		t.Error("canonical session_id must be untouched")
	}
	if len(applied) != 1 {
		t.Errorf("applied = %v", applied)
	}
}

func TestStripStringPointerLeaf(t *testing.T) {
	s := sampleSession()
	applied := StripLocalOnly(s, []string{"agent.model"})

	if s.Agent.Model == nil || *s.Agent.Model != "[LOCAL_ONLY:agent.model]" {
		t.Errorf("agent.model = %v, want marker (not nil)", s.Agent.Model)
	}
	if len(applied) != 1 {
		t.Errorf("applied = %v", applied)
	}
}

func TestStripWholeObject(t *testing.T) {
	s := sampleSession()
	applied := StripLocalOnly(s, []string{"machine", "prompt"})

	if s.Machine != nil {
		t.Errorf("machine should be nil, got %+v", s.Machine)
	}
	if s.Prompt != nil {
		t.Errorf("prompt should be nil, got %+v", s.Prompt)
	}
	if !reflect.DeepEqual(applied, []string{"machine", "prompt"}) {
		t.Errorf("applied = %v", applied)
	}
}

func TestStripWholeArray(t *testing.T) {
	s := sampleSession()
	applied := StripLocalOnly(s, []string{"files_touched"})

	if s.FilesTouched != nil {
		t.Errorf("files_touched should be nil, got %v", s.FilesTouched)
	}
	if len(applied) != 1 {
		t.Errorf("applied = %v", applied)
	}
}

func TestStripArrayFanOut(t *testing.T) {
	s := sampleSession()
	applied := StripLocalOnly(s, []string{"files_touched.path"})

	for i, f := range s.FilesTouched {
		if f.Path != "[LOCAL_ONLY:files_touched.path]" {
			t.Errorf("files_touched[%d].path = %q, want marker", i, f.Path)
		}
		if f.Action == "" {
			t.Errorf("files_touched[%d].action should be untouched", i)
		}
	}
	if len(applied) != 1 {
		t.Errorf("applied = %v", applied)
	}
}

func TestStripMapEntry(t *testing.T) {
	s := sampleSession()
	applied := StripLocalOnly(s, []string{"orchestration.extra.customer"})

	if got := s.Orchestration.Extra["customer"]; got != "[LOCAL_ONLY:orchestration.extra.customer]" {
		t.Errorf("extra.customer = %v, want marker", got)
	}
	if s.Orchestration.Extra["count"] != 3 {
		t.Errorf("extra.count should be untouched")
	}
	if len(applied) != 1 {
		t.Errorf("applied = %v", applied)
	}
}

func TestStripNestedMapEntry(t *testing.T) {
	s := sampleSession()
	applied := StripLocalOnly(s, []string{"orchestration.extra.nested.inner"})

	nested, ok := s.Orchestration.Extra["nested"].(map[string]any)
	if !ok {
		t.Fatalf("extra.nested has wrong type: %T", s.Orchestration.Extra["nested"])
	}
	if nested["inner"] != "[LOCAL_ONLY:orchestration.extra.nested.inner]" {
		t.Errorf("extra.nested.inner = %v, want marker", nested["inner"])
	}
	if len(applied) != 1 {
		t.Errorf("applied = %v", applied)
	}
}

func TestStripNonStringMapValue(t *testing.T) {
	s := sampleSession()
	applied := StripLocalOnly(s, []string{"tool_use.by_tool.Bash"})

	if got := s.ToolUse.ByTool["Bash"]; got != 0 {
		t.Errorf("by_tool.Bash = %d, want 0", got)
	}
	if s.ToolUse.ByTool["Read"] != 2 {
		t.Errorf("by_tool.Read should be untouched")
	}
	if len(applied) != 1 {
		t.Errorf("applied = %v", applied)
	}
}

func TestStripMissingPathNoOp(t *testing.T) {
	s := sampleSession()
	applied := StripLocalOnly(s, []string{"promt.text", "outcome.pr_number", "orchestration.extra.absent"})

	// outcome is nil, "promt" is a typo, "absent" key doesn't exist —
	// all no-ops, none applied.
	if applied != nil {
		t.Errorf("applied = %v, want none", applied)
	}
	if s.Prompt.Text != "secret prompt text" {
		t.Errorf("prompt.text should be untouched, got %q", s.Prompt.Text)
	}
}

func TestStripAlreadyZeroNoOp(t *testing.T) {
	s := sampleSession()
	s.Prompt.Text = ""
	applied := StripLocalOnly(s, []string{"prompt.text"})

	if s.Prompt.Text != "" {
		t.Errorf("empty string should stay empty, got %q", s.Prompt.Text)
	}
	if applied != nil {
		t.Errorf("applied = %v, want none (nothing to hide)", applied)
	}
}

func TestStripProtectedPathsSkipped(t *testing.T) {
	s := sampleSession()
	applied := StripLocalOnly(s, []string{
		"schema_version", "session_id", "status", "agent.runtime",
		"agent", // contains required agent.runtime — protected as a prefix
		"local_only_stripped",
	})

	if applied != nil {
		t.Errorf("applied = %v, want none — all protected", applied)
	}
	if s.SchemaVersion != schema.SchemaVersion || s.SessionID == "" || s.Status != "complete" || s.Agent.Runtime != "claude-code" {
		t.Error("protected fields must be untouched")
	}
}

func TestStripDedupesAndOrders(t *testing.T) {
	s := sampleSession()
	applied := StripLocalOnly(s, []string{"git_start.branch", "prompt.text", "git_start.branch"})

	want := []string{"git_start.branch", "prompt.text"}
	if !reflect.DeepEqual(applied, want) {
		t.Errorf("applied = %v, want %v (config order, deduped)", applied, want)
	}
}

func TestStripIdempotent(t *testing.T) {
	s := sampleSession()
	fields := []string{"prompt.text", "machine", "files_touched.path"}
	StripLocalOnly(s, fields)
	first, _ := json.Marshal(s)

	// Second pass: markers are non-zero strings so they'd re-strip to the
	// same marker; nil objects stay nil. Record must be byte-identical.
	StripLocalOnly(s, fields)
	second, _ := json.Marshal(s)
	if string(first) != string(second) {
		t.Errorf("strip not idempotent:\nfirst:  %s\nsecond: %s", first, second)
	}
}

func TestStripNilAndNonPointerNoOp(t *testing.T) {
	if got := StripLocalOnly(nil, []string{"prompt.text"}); got != nil {
		t.Errorf("nil input: applied = %v", got)
	}
	s := sampleSession()
	if got := StripLocalOnly(*s, []string{"prompt.text"}); got != nil {
		t.Errorf("non-pointer input: applied = %v", got)
	}
	if s.Prompt.Text != "secret prompt text" {
		t.Error("non-pointer input must not mutate")
	}
}

// The stripped record must remain a valid etch.session.v1 record: it
// round-trips through schema.Session marshal/unmarshal with identity intact.
func TestStrippedRecordRoundTrips(t *testing.T) {
	s := sampleSession()
	applied := StripLocalOnly(s, []string{"prompt.text", "machine", "files_touched", "git_start.branch", "orchestration.extra.customer"})
	s.LocalOnlyStripped = applied

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal stripped: %v", err)
	}
	var back schema.Session
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("stripped record does not unmarshal: %v", err)
	}
	if back.SchemaVersion != schema.SchemaVersion {
		t.Errorf("schema_version = %q", back.SchemaVersion)
	}
	if back.SessionID != s.SessionID {
		t.Errorf("session_id = %q", back.SessionID)
	}
	if back.Status != "complete" || back.Agent.Runtime != "claude-code" {
		t.Error("required fields lost in round-trip")
	}
	if !reflect.DeepEqual(back.LocalOnlyStripped, applied) {
		t.Errorf("local_only_stripped = %v, want %v", back.LocalOnlyStripped, applied)
	}
	if strings.Contains(string(data), "secret prompt text") || strings.Contains(string(data), "feat/secret-project") {
		t.Error("stripped values still present in marshaled record")
	}
	round, err := json.Marshal(&back)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if string(round) != string(data) {
		t.Errorf("round-trip not stable:\n%s\n%s", data, round)
	}
}
