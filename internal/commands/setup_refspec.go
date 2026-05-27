package commands

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

const cairnRefspec = "refs/cairn/sessions/*:refs/cairn/sessions/*"

func RunSetupRefspec() error {
	if err := addRefspecIfMissing("push"); err != nil {
		return err
	}
	if err := addRefspecIfMissing("fetch"); err != nil {
		return err
	}
	fmt.Println("cairn refspec configured for push and fetch")
	return nil
}

func addRefspecIfMissing(direction string) error {
	key := "remote.origin." + direction

	cmd := exec.Command("git", "config", "--get-all", key)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Run() // exit code 1 means no entries, which is fine

	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.TrimSpace(line) == cairnRefspec {
			return nil
		}
	}

	add := exec.Command("git", "config", "--add", key, cairnRefspec)
	var stderr bytes.Buffer
	add.Stderr = &stderr
	if err := add.Run(); err != nil {
		return fmt.Errorf("git config --add %s: %w: %s", key, err, strings.TrimSpace(stderr.String()))
	}

	return nil
}
