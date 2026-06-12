package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Stage-11-Agentics/etch/internal/commands"
	"github.com/Stage-11-Agentics/etch/internal/enable"
	"github.com/Stage-11-Agentics/etch/internal/hooks"
	"github.com/Stage-11-Agentics/etch/internal/info"
	"github.com/Stage-11-Agentics/etch/internal/install"
	"github.com/Stage-11-Agentics/etch/internal/parsehook"
	"github.com/Stage-11-Agentics/etch/internal/stubs"
)

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "help", "--help", "-h":
		printUsage(os.Stdout)
	case "info":
		err = info.Run()
	case "detect":
		err = install.RunDetect()
	case "install-hooks":
		err = install.RunInstallHooks(os.Args[2:])
	case "uninstall-hooks":
		err = install.RunUninstallHooks()
	case "are-hooks-installed":
		err = install.RunAreHooksInstalled()
	case "enable":
		err = enable.RunEnable(os.Args[2:])
	case "disable":
		err = enable.RunDisable()
	case "parse-hook":
		err = parsehook.Run(os.Args[2:])
	case "query":
		err = RunQuery(os.Args[2:])
	case "index":
		err = RunIndex(os.Args[2:])

	// Hook handlers. The fast-exit guard runs before stdin is read: outside
	// a git repo, or with etch.enabled=false, every hook is a silent exit 0
	// (docs/ENABLEMENT.md).
	case "session_start":
		err = runHook(hooks.RunSessionStart)
	case "session_end":
		err = runHook(hooks.RunSessionEnd)
	case "user_prompt_submit":
		err = runHook(hooks.RunUserPromptSubmit)
	case "stop":
		err = runHook(hooks.RunStop)
	case "pre_tool_use":
		err = runHook(hooks.RunPreToolUse)
	case "post_tool_use":
		err = runHook(hooks.RunPostToolUse)

	// Capability subcommands
	case "extract-modified-files":
		err = commands.RunExtractModifiedFiles(os.Args[2:])
	case "calculate-tokens":
		err = commands.RunCalculateTokens(os.Args[2:])
	case "setup-refspec":
		err = commands.RunSetupRefspec(os.Args[2:])
	case "archive":
		err = runArchive(os.Args[2:])
	case "restore-archive":
		err = runRestoreArchive(os.Args[2:])

	// Stub subcommands — not yet implemented
	case "extract-all-modified-files", "calculate-total-tokens":
		err = stubs.Run()

	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\nrun 'entire-agent-etch help' for a list of subcommands\n", os.Args[1])
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// runHook gates a hook entrypoint behind the operator-mode fast-exit guard.
func runHook(handler func() error) error {
	if enable.HooksDisabled() {
		// Drain stdin so the dispatcher's payload write never sees EPIPE —
		// the enabled path reads it all anyway, so the contract stays
		// uniform: the hook process always consumes its payload.
		_, _ = io.Copy(io.Discard, os.Stdin)
		return nil
	}
	return handler()
}
