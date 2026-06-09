package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Stage-11-Agentics/etch/internal/index"
)

// RunIndex dispatches the `index` subcommand: build | update | show | drop.
func RunIndex(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: index <build|update|show|drop> [--repo PATH]")
	}

	action := args[0]
	fs := flag.NewFlagSet("index "+action, flag.ContinueOnError)
	repo := fs.String("repo", "", "path to the git repo (default: current directory)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	switch action {
	case "build":
		res, err := index.Build(*repo)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "index built: %d sessions (%s)\n", res.Total, index.IndexPath(*repo))
		return nil
	case "update":
		res, err := index.Update(*repo)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "index updated: %d new, %d unchanged, %d total\n", res.Parsed, res.Skipped, res.Total)
		return nil
	case "show":
		st, err := index.Show(*repo)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "path:     %s\n", st.Path)
		fmt.Fprintf(os.Stdout, "sessions: %d\n", st.Count)
		fmt.Fprintf(os.Stdout, "size:     %d bytes\n", st.SizeBytes)
		fmt.Fprintf(os.Stdout, "built_at: %s\n", st.BuiltAt)
		fmt.Fprintf(os.Stdout, "oldest:   %s\n", emptyDash(st.Oldest))
		fmt.Fprintf(os.Stdout, "newest:   %s\n", emptyDash(st.Newest))
		return nil
	case "drop":
		if err := index.Drop(*repo); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "index dropped: %s\n", index.IndexPath(*repo))
		return nil
	default:
		return fmt.Errorf("unknown index action %q: must be build, update, show, or drop", action)
	}
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
