package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Settings struct {
	RawMachineIdentity   bool     `json:"raw_machine_identity"`
	LocalOnlyFields      []string `json:"local_only_fields"`
	ArchiveThresholdDays int      `json:"archive_threshold_days"`
	RedactionPatterns     []string `json:"redaction_patterns"`
	RecoveryTimeoutHours int      `json:"recovery_timeout_hours"`
}

func Defaults() Settings {
	return Settings{
		RawMachineIdentity:   false,
		ArchiveThresholdDays: 90,
		RecoveryTimeoutHours: 4,
	}
}

func Load(repoRoot string) (Settings, error) {
	p := filepath.Join(repoRoot, ".cairn", "settings.json")
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
