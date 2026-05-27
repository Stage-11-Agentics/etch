package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewTestRepo(t *testing.T) {
	dir := NewTestRepo(t)
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("expected .git directory in %s", dir)
	}
}

func TestMustParseJSON(t *testing.T) {
	m := MustParseJSON(t, `{"key": "value", "num": 42}`)
	if m["key"] != "value" {
		t.Errorf("expected key=value, got %v", m["key"])
	}
	if m["num"] != float64(42) {
		t.Errorf("expected num=42, got %v", m["num"])
	}
}
