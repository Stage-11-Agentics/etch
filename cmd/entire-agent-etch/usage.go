package main

import (
	"fmt"
	"io"
)

// command is one row of the help listing. Every subcommand dispatched in
// main.go must appear here, and every row here must be dispatched —
// assertFullListing and TestListedSubcommandsAreDispatched enforce both.
type command struct {
	name string // subcommand name as dispatched
	args string // argument synopsis, "" if none
	desc string // one-line description
}

// section groups commands in the help listing.
type section struct {
	title    string
	commands []command
}

// sections is the canonical, ordered subcommand listing.
var sections = []section{
	{
		title: "Session commands",
		commands: []command{
			{"query", "[--repo PATH] [filters...]", "search captured sessions (--ticket, --runtime, --status, --since/--until, --branch, --capture-method, --json, --count, ...)"},
			{"import", "[--repo PATH] [--runtime NAME] [--since RFC3339] [--dry-run]", "post-hoc ingest agent transcripts (Claude Code, Codex) into session refs — see docs/INGESTION.md"},
			{"index", "<build|update|show|drop> [--repo PATH]", "manage the materialized session index that accelerates query"},
			{"archive", "[--dry-run] [--threshold-days N] [--quarter YYYY-Qn]", "move old session refs into per-quarter archive refs"},
			{"restore-archive", "<ULID>", "restore one archived session back to refs/etch/sessions/"},
			{"setup-refspec", "[--remote NAME]", "configure git fetch/push refspecs so session refs sync with a remote"},
			{"doctor", "[--json] [--warn-age DAYS]", "capture health check: binary, hooks, refspec, session age, wip buffers, operator-mode state"},
		},
	},
	{
		title: "Enablement (operator mode, per-clone — see docs/ENABLEMENT.md)",
		commands: []command{
			{"enable", "", "turn on capture for this clone: etch.enabled=true + .git/info/exclude entries"},
			{"disable", "", "turn off all capture in this repo (etch.enabled=false wins over everything)"},
			{"stamp-worktree", "", "stamp the current worktree's .claude/settings.local.json (run by the post-checkout hook)"},
		},
	},
	{
		title: "Install & protocol commands",
		commands: []command{
			{"info", "", "print the agent's protocol-v1 capability manifest (JSON)"},
			{"detect", "", "report agent presence to Entire's discovery (JSON)"},
			{"install-hooks", "[--force]", "write etch's hook entries into .claude/settings.json"},
			{"uninstall-hooks", "", "remove etch's hook entries from .claude/settings.json"},
			{"are-hooks-installed", "", "report whether etch's hooks are installed (JSON)"},
			{"parse-hook", "--hook <name>", "parse a native hook payload from stdin into etch's normalized form"},
			{"extract-modified-files", "<session-id>", "list files touched by a captured session (JSON)"},
			{"calculate-tokens", "<session-id>", "print token usage for a captured session (JSON)"},
		},
	},
	{
		title: "Hook entry points (invoked by the agent runtime, JSON on stdin)",
		commands: []command{
			{"session_start", "", "begin a session capture buffer"},
			{"session_end", "", "finalize the session into an immutable git ref"},
			{"user_prompt_submit", "", "record a user prompt event"},
			{"stop", "", "record an agent stop event"},
			{"pre_tool_use", "", "record a tool-use start event"},
			{"post_tool_use", "", "record a tool-use result event"},
		},
	},
	{
		title: "Stubs (accepted for protocol compatibility, no-op)",
		commands: []command{
			{"extract-all-modified-files", "", "not yet implemented"},
			{"calculate-total-tokens", "", "not yet implemented"},
		},
	},
}

// printUsage renders the full subcommand listing. Output is deterministic:
// fixed section order, fixed command order, stable column alignment.
func printUsage(w io.Writer) {
	fmt.Fprintf(w, "usage: entire-agent-etch <subcommand> [args...]\n\n")
	fmt.Fprintf(w, "Etch captures flat metadata for every AI agent session in a repository\nand stores it as immutable git refs (refs/etch/sessions/<ULID>).\n")

	// One global column width so every section aligns identically.
	width := 0
	for _, s := range sections {
		for _, c := range s.commands {
			if n := len(c.name); n > width {
				width = n
			}
		}
	}

	for _, s := range sections {
		fmt.Fprintf(w, "\n%s:\n", s.title)
		for _, c := range s.commands {
			fmt.Fprintf(w, "  %-*s  %s\n", width, c.name, c.desc)
			if c.args != "" {
				fmt.Fprintf(w, "  %-*s    usage: %s %s\n", width, "", c.name, c.args)
			}
		}
	}

	fmt.Fprintf(w, "\nRun 'entire-agent-etch help' to see this listing. See the README for setup.\n")
}
