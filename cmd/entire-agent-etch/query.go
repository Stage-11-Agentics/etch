package main

import "forgejo.stage11.ai/s11/etch/internal/query"

// RunQuery is the thin cmd-layer entrypoint for the `query` subcommand.
func RunQuery(args []string) error {
	return query.Run(args)
}
