package config

import (
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
