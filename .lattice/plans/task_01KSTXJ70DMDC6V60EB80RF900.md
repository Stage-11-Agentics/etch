# ETCH-21: No subcommand discovery: bare invocation and --help/help give no command list

'entire-agent-etch' with no args prints only: 'usage: entire-agent-etch <subcommand> [args...]' — no list of subcommands. 'entire-agent-etch --help' and 'entire-agent-etch help' both return 'unknown subcommand: ...' (exit 1). A naive user has no way to discover info/setup-refspec/query/index/archive from the tool itself; they must read the README and grep source. EXPECTED: bare/-h/--help/help prints a subcommand list with one-line descriptions. Low-effort, high-impact for first-run UX.
