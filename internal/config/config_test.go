package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	d := Defaults()
	if d.RawMachineIdentity {
		t.Error("RawMachineIdentity should default to false")
	}
	if d.ArchiveThresholdDays != 90 {
		t.Errorf("ArchiveThresholdDays = %d, want 90", d.ArchiveThresholdDays)
	}
	if d.RecoveryTimeoutHours != 4 {
		t.Errorf("RecoveryTimeoutHours = %d, want 4", d.RecoveryTimeoutHours)
	}
	if d.LocalOnlyFields != nil {
		t.Error("LocalOnlyFields should be nil by default")
	}
	if d.RedactionPatterns != nil {
		t.Error("RedactionPatterns should be nil by default")
	}
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.RawMachineIdentity != false {
		t.Error("RawMachineIdentity should be false")
	}
	if s.ArchiveThresholdDays != 90 {
		t.Errorf("ArchiveThresholdDays = %d, want 90", s.ArchiveThresholdDays)
	}
	if s.RecoveryTimeoutHours != 4 {
		t.Errorf("RecoveryTimeoutHours = %d, want 4", s.RecoveryTimeoutHours)
	}
}

func TestLoadValidJSON(t *testing.T) {
	dir := t.TempDir()
	etchDir := filepath.Join(dir, ".etch")
	os.MkdirAll(etchDir, 0o755)
	os.WriteFile(filepath.Join(etchDir, "settings.json"), []byte(`{
		"raw_machine_identity": true,
		"archive_threshold_days": 30,
		"redaction_patterns": ["my-secret-\\d+"],
		"recovery_timeout_hours": 8
	}`), 0o644)

	s, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !s.RawMachineIdentity {
		t.Error("RawMachineIdentity should be true")
	}
	if s.ArchiveThresholdDays != 30 {
		t.Errorf("ArchiveThresholdDays = %d, want 30", s.ArchiveThresholdDays)
	}
	if s.RecoveryTimeoutHours != 8 {
		t.Errorf("RecoveryTimeoutHours = %d, want 8", s.RecoveryTimeoutHours)
	}
	if len(s.RedactionPatterns) != 1 || s.RedactionPatterns[0] != `my-secret-\d+` {
		t.Errorf("RedactionPatterns = %v, want [my-secret-\\d+]", s.RedactionPatterns)
	}
}

func TestLoadPartialJSON(t *testing.T) {
	dir := t.TempDir()
	etchDir := filepath.Join(dir, ".etch")
	os.MkdirAll(etchDir, 0o755)
	os.WriteFile(filepath.Join(etchDir, "settings.json"), []byte(`{"raw_machine_identity": true}`), 0o644)

	s, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !s.RawMachineIdentity {
		t.Error("RawMachineIdentity should be true")
	}
	if s.ArchiveThresholdDays != 90 {
		t.Errorf("defaults should be preserved: ArchiveThresholdDays = %d, want 90", s.ArchiveThresholdDays)
	}
	if s.RecoveryTimeoutHours != 4 {
		t.Errorf("defaults should be preserved: RecoveryTimeoutHours = %d, want 4", s.RecoveryTimeoutHours)
	}
}

func TestLoadMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	etchDir := filepath.Join(dir, ".etch")
	os.MkdirAll(etchDir, 0o755)
	os.WriteFile(filepath.Join(etchDir, "settings.json"), []byte(`{not json`), 0o644)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestEnsureHostnameSaltFreshRepo(t *testing.T) {
	dir := t.TempDir()

	salt, err := EnsureHostnameSalt(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(salt) != 64 {
		t.Errorf("salt should be 64 hex chars (32 bytes), got len %d", len(salt))
	}

	// Persisted to .etch/settings.json
	s, err := Load(dir)
	if err != nil {
		t.Fatalf("Load after salt generation: %v", err)
	}
	if s.HostnameSalt != salt {
		t.Errorf("persisted salt %q != returned salt %q", s.HostnameSalt, salt)
	}
}

func TestEnsureHostnameSaltIdempotent(t *testing.T) {
	dir := t.TempDir()

	salt1, err := EnsureHostnameSalt(dir)
	if err != nil {
		t.Fatal(err)
	}
	salt2, err := EnsureHostnameSalt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if salt1 != salt2 {
		t.Errorf("salt should be stable across calls: %q != %q", salt1, salt2)
	}
}

func TestEnsureHostnameSaltPreservesUnknownFields(t *testing.T) {
	dir := t.TempDir()
	etchDir := filepath.Join(dir, ".etch")
	os.MkdirAll(etchDir, 0o755)
	os.WriteFile(filepath.Join(etchDir, "settings.json"), []byte(`{
		"raw_machine_identity": true,
		"recovery_timeout_hours": 8,
		"custom_user_key": "keep-me"
	}`), 0o644)

	salt, err := EnsureHostnameSalt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if salt == "" {
		t.Fatal("expected a generated salt")
	}

	data, err := os.ReadFile(filepath.Join(etchDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("rewritten settings.json is not valid JSON: %v", err)
	}
	if m["raw_machine_identity"] != true {
		t.Error("raw_machine_identity lost on salt injection")
	}
	if m["recovery_timeout_hours"] != float64(8) {
		t.Error("recovery_timeout_hours lost on salt injection")
	}
	if m["custom_user_key"] != "keep-me" {
		t.Error("unknown user key lost on salt injection")
	}
	if m["hostname_salt"] != salt {
		t.Error("hostname_salt not persisted")
	}
}

func TestEnsureHostnameSaltExistingSaltUntouched(t *testing.T) {
	dir := t.TempDir()
	etchDir := filepath.Join(dir, ".etch")
	os.MkdirAll(etchDir, 0o755)
	original := `{"hostname_salt": "preexisting-salt-value"}`
	p := filepath.Join(etchDir, "settings.json")
	os.WriteFile(p, []byte(original), 0o644)
	before, _ := os.Stat(p)

	salt, err := EnsureHostnameSalt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if salt != "preexisting-salt-value" {
		t.Errorf("existing salt should be returned verbatim, got %q", salt)
	}

	after, _ := os.Stat(p)
	if before.ModTime() != after.ModTime() {
		t.Error("settings.json should not be rewritten when a salt already exists")
	}
	data, _ := os.ReadFile(p)
	if string(data) != original {
		t.Error("settings.json content changed despite existing salt")
	}
}

func TestEnsureHostnameSaltDiffersAcrossRepos(t *testing.T) {
	salt1, err := EnsureHostnameSalt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	salt2, err := EnsureHostnameSalt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if salt1 == salt2 {
		t.Error("two repos should generate different salts")
	}
}
