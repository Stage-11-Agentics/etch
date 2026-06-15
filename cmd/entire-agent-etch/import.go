package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Stage-11-Agentics/etch/internal/importer"
)

// runImport implements the `import` subcommand: post-hoc ingestion of agent
// runtime transcripts into refs/etch/sessions/* (see docs/INGESTION.md).
func runImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	var (
		repo    = fs.String("repo", "", "path to the git repo (default: current directory)")
		runtime = fs.String("runtime", "", "import only this runtime (claude-code|codex); default: all")
		since   = fs.String("since", "", "import only sessions started at or after this RFC3339 time")
		dryRun  = fs.Bool("dry-run", false, "report what would be imported without writing refs")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	dir := *repo
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		dir = cwd
	}
	repoRoot, err := importer.ResolveRepoRoot(dir)
	if err != nil {
		return err
	}

	opts := importer.Options{
		RepoRoot: repoRoot,
		Runtime:  *runtime,
		DryRun:   *dryRun,
	}
	if *since != "" {
		t, err := time.Parse(time.RFC3339, *since)
		if err != nil {
			return fmt.Errorf("invalid --since %q: %w", *since, err)
		}
		opts.Since = t
	}

	res, err := importer.Run(opts, os.Stderr)
	if err != nil {
		return err
	}

	verb := "imported"
	if *dryRun {
		verb = "would import"
	}
	fmt.Printf("%s %d session(s); skipped %d (already recorded); %d out-of-repo; %d failed\n",
		verb, res.Imported, res.Skipped, res.OutOfRepo, res.Failed)
	return nil
}
