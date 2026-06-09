package main

import "github.com/Stage-11-Agentics/etch/internal/query"

// RunQuery is the thin cmd-layer entrypoint for the `query` subcommand.
func RunQuery(args []string) error {
	return query.Run(args)
}
