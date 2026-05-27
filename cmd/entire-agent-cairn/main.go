package main

import (
	"fmt"
	"os"

	"forgejo.stage11.ai/s11/etch/internal/info"
	"forgejo.stage11.ai/s11/etch/internal/parsehook"
	"forgejo.stage11.ai/s11/etch/internal/stubs"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <subcommand> [args...]\n", os.Args[0])
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "info":
		err = info.Run()
	case "parse-hook":
		err = parsehook.Run(os.Args[2:])

	// Stub subcommands — real implementations in later tickets
	case "session_start", "session_end", "user_prompt_submit", "stop",
		"pre_tool_use", "post_tool_use",
		"extract-modified-files", "calculate-tokens",
		"extract-all-modified-files", "calculate-total-tokens":
		err = stubs.Run()

	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", os.Args[1])
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
