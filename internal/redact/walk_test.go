package redact

import (
	"strings"
	"testing"

	"forgejo.stage11.ai/s11/etch/internal/config"
	"forgejo.stage11.ai/s11/etch/internal/schema"
)

const (
	walkSecret = "sk-proj-AbCdEf123456_789-abcdefGHIJKL"
	walkJWT    = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature_part"
)

func strptr(s string) *string { return &s }

// ETCH-40 finding 5: secrets in FilesTouched paths must be redacted.
func TestDeepRedactFilesTouchedPath(t *testing.T) {
	s := &schema.Session{
		FilesTouched: []schema.FileEntry{
			{Path: "/Users/x/keys/" + walkSecret + ".pem", Action: "modified"},
			{Path: "internal/redact/walk.go", Action: "modified"},
		},
	}
	DeepRedact(s, config.Defaults())
	if strings.Contains(s.FilesTouched[0].Path, walkSecret) {
		t.Errorf("secret in file path not redacted: %s", s.FilesTouched[0].Path)
	}
	if !strings.Contains(s.FilesTouched[0].Path, "[REDACTED:openai-api-key]") {
		t.Errorf("expected marker in file path: %s", s.FilesTouched[0].Path)
	}
	if s.FilesTouched[1].Path != "internal/redact/walk.go" {
		t.Errorf("clean path clobbered: %s", s.FilesTouched[1].Path)
	}
}

// Secrets as ToolUse.ByTool map KEYS must be rewritten, counts preserved.
func TestDeepRedactToolUseKeys(t *testing.T) {
	s := &schema.Session{
		ToolUse: &schema.ToolUse{
			TotalCalls: 7,
			ByTool: map[string]int{
				"Bash " + walkJWT: 3,
				"Read":            4,
			},
		},
	}
	DeepRedact(s, config.Defaults())
	if s.ToolUse.TotalCalls != 7 {
		t.Errorf("non-string field changed: %d", s.ToolUse.TotalCalls)
	}
	if s.ToolUse.ByTool["Read"] != 4 {
		t.Errorf("clean key clobbered: %v", s.ToolUse.ByTool)
	}
	for k, v := range s.ToolUse.ByTool {
		if strings.Contains(k, "eyJ") {
			t.Errorf("secret survived in map key: %s", k)
		}
		if strings.HasPrefix(k, "Bash ") {
			if !strings.Contains(k, "[REDACTED:jwt]") {
				t.Errorf("expected jwt marker in key: %s", k)
			}
			if v != 3 {
				t.Errorf("count lost on key rewrite: %s=%d", k, v)
			}
		}
	}
	if len(s.ToolUse.ByTool) != 2 {
		t.Errorf("expected 2 keys, got %v", s.ToolUse.ByTool)
	}
}

// Secrets nested inside Orchestration.Extra (map[string]any — interface-boxed,
// non-addressable) must be redacted, including nested maps and slices.
func TestDeepRedactOrchestrationExtra(t *testing.T) {
	s := &schema.Session{
		Orchestration: &schema.Orchestration{
			Type: "delegator",
			Extra: map[string]any{
				"note":  "key is " + walkSecret,
				"count": 42,
				"nested": map[string]any{
					"deep": "token " + walkJWT + " here",
				},
				"list": []any{"clean", "jwt: " + walkJWT},
			},
		},
	}
	DeepRedact(s, config.Defaults())
	extra := s.Orchestration.Extra
	if note := extra["note"].(string); strings.Contains(note, walkSecret) {
		t.Errorf("secret in Extra value not redacted: %s", note)
	}
	if extra["count"].(int) != 42 {
		t.Errorf("non-string Extra value changed: %v", extra["count"])
	}
	nested := extra["nested"].(map[string]any)
	if deep := nested["deep"].(string); strings.Contains(deep, "eyJ") {
		t.Errorf("nested secret not redacted: %s", deep)
	}
	list := extra["list"].([]any)
	if list[0].(string) != "clean" {
		t.Errorf("clean list element clobbered: %v", list[0])
	}
	if strings.Contains(list[1].(string), "eyJ") {
		t.Errorf("secret in list not redacted: %v", list[1])
	}
}

// Prompt text still redacts through the deep walk (the original behavior).
func TestDeepRedactPromptText(t *testing.T) {
	s := &schema.Session{
		Prompt: &schema.Prompt{Text: "fix it, key " + walkSecret, Source: "user_prompt_submit"},
	}
	DeepRedact(s, config.Defaults())
	if strings.Contains(s.Prompt.Text, walkSecret) {
		t.Errorf("prompt secret not redacted: %s", s.Prompt.Text)
	}
	if s.Prompt.Source != "user_prompt_submit" {
		t.Errorf("prompt source clobbered: %s", s.Prompt.Source)
	}
}

// Structural fields survive intact: SHAs, ULIDs, hostname hashes.
func TestDeepRedactStructuralFieldsSurvive(t *testing.T) {
	sha := "01a2ca4f3e8b9d2c5a7f1e6b4d8c3a9f2e5b7d1c"
	hash := "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	s := &schema.Session{
		SchemaVersion: schema.SchemaVersion,
		SessionID:     "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Status:        "complete",
		ExitReason:    "normal",
		GitStart:      &schema.GitState{Branch: "fix/redaction-batch", HeadSHA: sha},
		Machine:       &schema.Machine{HostnameHash: hash, OS: "darwin"},
	}
	DeepRedact(s, config.Defaults())
	if s.GitStart.HeadSHA != sha {
		t.Errorf("git SHA clobbered: %s", s.GitStart.HeadSHA)
	}
	if s.Machine.HostnameHash != hash {
		t.Errorf("hostname hash clobbered: %s", s.Machine.HostnameHash)
	}
	if s.SessionID != "01ARZ3NDEKTSV4RRFFQ69G5FAV" {
		t.Errorf("ULID clobbered: %s", s.SessionID)
	}
	if s.GitStart.Branch != "fix/redaction-batch" {
		t.Errorf("branch clobbered: %s", s.GitStart.Branch)
	}
}

// Nil pointers everywhere must not panic; pointer-to-string fields redact.
func TestDeepRedactNilSafety(t *testing.T) {
	DeepRedact(nil, config.Defaults())
	DeepRedact((*schema.Session)(nil), config.Defaults())
	DeepRedact(42, config.Defaults()) // non-pointer: no-op

	s := &schema.Session{
		ParentSessionID: strptr("parent " + walkSecret),
		Agent:           schema.Agent{Runtime: "claude-code", Model: nil},
	}
	DeepRedact(s, config.Defaults())
	if strings.Contains(*s.ParentSessionID, walkSecret) {
		t.Errorf("pointer-to-string field not redacted: %s", *s.ParentSessionID)
	}
}

// Custom settings patterns apply through the deep walk too.
func TestDeepRedactCustomPatterns(t *testing.T) {
	s := &schema.Session{
		FilesTouched: []schema.FileEntry{{Path: "docs/MYTOKEN-ABCD1234.txt", Action: "added"}},
	}
	DeepRedact(s, config.Settings{RedactionPatterns: []string{`MYTOKEN-[A-Z0-9]{8}`}})
	if strings.Contains(s.FilesTouched[0].Path, "MYTOKEN-ABCD1234") {
		t.Errorf("custom pattern not applied in walk: %s", s.FilesTouched[0].Path)
	}
	if !strings.Contains(s.FilesTouched[0].Path, "[REDACTED:custom-0]") {
		t.Errorf("expected custom marker: %s", s.FilesTouched[0].Path)
	}
}
