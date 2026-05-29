package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func readSessionFromRef(sessionID string) (map[string]any, error) {
	refPath := "refs/etch/sessions/" + sessionID + ":session.json"

	cmd := exec.Command("git", "show", refPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("reading session from ref %s: %w: %s", sessionID, err, strings.TrimSpace(stderr.String()))
	}

	var session map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &session); err != nil {
		return nil, fmt.Errorf("parsing session JSON: %w", err)
	}

	return session, nil
}
