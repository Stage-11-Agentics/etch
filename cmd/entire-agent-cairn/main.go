package main

import (
	"fmt"
	"os"

	"forgejo.stage11.ai/s11/etch/internal/hooks"
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

	// Hook handlers
	case "session_start":
		err = hooks.RunSessionStart()
	case "session_end":
		err = hooks.RunSessionEnd()
	case "user_prompt_submit":
		err = hooks.RunUserPromptSubmit()
	case "stop":
		err = hooks.RunStop()
	case "pre_tool_use":
		err = hooks.RunPreToolUse()
	case "post_tool_use":
		err = hooks.RunPostToolUse()

	// Stub subcommands — real implementations in later tickets
	case "extract-modified-files", "calculate-tokens",
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
