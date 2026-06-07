# ETCH-21: No subcommand discovery: bare invocation and --help/help give no command list

'entire-agent-etch' with no args prints only: 'usage: entire-agent-etch <subcommand> [args...]' — no list of subcommands. 'entire-agent-etch --help' and 'entire-agent-etch help' both return 'unknown subcommand: ...' (exit 1). A naive user has no way to discover info/setup-refspec/query/index/archive from the tool itself; they must read the README and grep source. EXPECTED: bare/-h/--help/help prints a subcommand list with one-line descriptions. Low-effort, high-impact for first-run UX.

## Plan

Batched with ETCH-19 (one PR, branch `docs/cli-discoverability`). Full shared plan:
`.lattice/plans/task_01KSTXHTANQNSZQS44241G8EFF.md`.

ETCH-21 portion: add a structured command table in `cmd/entire-agent-etch/usage.go`
(sections: operational / install-capability / hook entry points / stubs);
`printUsage(w)` renders it deterministically. Dispatch: bare → listing to stderr,
exit 1; `help`/`--help`/`-h` → listing to stdout, exit 0; unknown subcommand gains
a "run 'entire-agent-etch help'" hint. Tests assert exit codes and that every
dispatched subcommand name appears in the help output.
