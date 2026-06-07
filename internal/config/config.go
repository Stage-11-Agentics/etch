package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

type Settings struct {
	RawMachineIdentity   bool     `json:"raw_machine_identity"`
	LocalOnlyFields      []string `json:"local_only_fields"`
	ArchiveThresholdDays int      `json:"archive_threshold_days"`
	RedactionPatterns     []string `json:"redaction_patterns"`
	RecoveryTimeoutHours int      `json:"recovery_timeout_hours"`
	// HostnameSalt is a random per-repo salt mixed into hostname_hash.
	// Auto-generated at first use; commit .etch/settings.json so all
	// clones of the repo share it (cross-machine correlation within a repo).
	HostnameSalt string `json:"hostname_salt"`
}

func Defaults() Settings {
	return Settings{
		RawMachineIdentity:   false,
		ArchiveThresholdDays: 90,
		RecoveryTimeoutHours: 4,
	}
}

func Load(repoRoot string) (Settings, error) {
	p := filepath.Join(repoRoot, ".etch", "settings.json")
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Defaults(), nil
		}
		return Settings{}, fmt.Errorf("reading %s: %w", p, err)
	}

	s := Defaults()
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}, fmt.Errorf("parsing %s: %w", p, err)
	}
	return s, nil
}

// EnsureHostnameSalt returns the per-repo hostname salt, generating and
// persisting it in .etch/settings.json on first use. The file is rewritten
// from a generic map so unknown/user-added keys survive. The write is atomic
// (temp file + rename) and the salt is re-read after writing so concurrent
// first-use racers converge on the last writer for all subsequent calls.
func EnsureHostnameSalt(repoRoot string) (string, error) {
	p := filepath.Join(repoRoot, ".etch", "settings.json")

	if salt := readSalt(p); salt != "" {
		return salt, nil
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating hostname salt: %w", err)
	}
	salt := hex.EncodeToString(raw)

	// Read existing settings as a generic map to preserve unknown fields.
	m := map[string]any{}
	if data, err := os.ReadFile(p); err == nil {
		if err := json.Unmarshal(data, &m); err != nil {
			return "", fmt.Errorf("parsing %s: %w", p, err)
		}
		// Another writer may have landed a salt between readSalt and here.
		if existing, ok := m["hostname_salt"].(string); ok && existing != "" {
			return existing, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("reading %s: %w", p, err)
	}
	m["hostname_salt"] = salt

	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling settings: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", fmt.Errorf("creating .etch dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), "settings-*.json.tmp")
	if err != nil {
		return "", fmt.Errorf("creating temp settings file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(out, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", fmt.Errorf("writing temp settings file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", fmt.Errorf("closing temp settings file: %w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		os.Remove(tmpName)
		return "", fmt.Errorf("chmod temp settings file: %w", err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		os.Remove(tmpName)
		return "", fmt.Errorf("renaming settings file: %w", err)
	}

	log.Printf("etch: generated per-repo hostname salt; commit .etch/settings.json to share it across clones")

	// Converge with concurrent writers: whatever is in the file now wins.
	if persisted := readSalt(p); persisted != "" {
		return persisted, nil
	}
	return salt, nil
}

func readSalt(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var s struct {
		HostnameSalt string `json:"hostname_salt"`
	}
	if json.Unmarshal(data, &s) != nil {
		return ""
	}
	return s.HostnameSalt
}
