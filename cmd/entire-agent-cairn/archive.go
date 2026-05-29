package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"forgejo.stage11.ai/s11/etch/internal/archive"
	"forgejo.stage11.ai/s11/etch/internal/config"
)

// runArchive handles `cairn archive [flags]`.
func runArchive(args []string) error {
	fs := flag.NewFlagSet("archive", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "print what would be archived without modifying anything")
	thresholdDays := fs.Int("threshold-days", -1, "override archive_threshold_days from config")
	sinceStr := fs.String("since", "", "archive only refs at/after this RFC3339 time")
	untilStr := fs.String("until", "", "archive only refs at/before this RFC3339 time")
	quarter := fs.String("quarter", "", "archive only this quarter (YYYY-Qn)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}

	threshold := *thresholdDays
	if threshold < 0 {
		settings, _ := config.Load(repoRoot)
		threshold = settings.ArchiveThresholdDays
	}

	opts := archive.Options{
		RepoRoot:      repoRoot,
		ThresholdDays: threshold,
		Now:           time.Now().UTC(),
		DryRun:        *dryRun,
		Quarter:       *quarter,
	}
	if *sinceStr != "" {
		t, err := time.Parse(time.RFC3339, *sinceStr)
		if err != nil {
			return fmt.Errorf("parsing --since: %w", err)
		}
		opts.Since = &t
	}
	if *untilStr != "" {
		t, err := time.Parse(time.RFC3339, *untilStr)
		if err != nil {
			return fmt.Errorf("parsing --until: %w", err)
		}
		opts.Until = &t
	}

	if *dryRun {
		plan, err := archive.BuildPlan(opts)
		if err != nil {
			return err
		}
		printPlan(plan, true)
		return nil
	}

	plan, err := archive.Archive(opts)
	if err != nil {
		return err
	}
	printPlan(plan, false)
	return nil
}

func printPlan(plan archive.Plan, dryRun bool) {
	if plan.Empty() {
		fmt.Println("nothing to archive")
		return
	}
	verb := "archived"
	if dryRun {
		verb = "would archive"
	}
	fmt.Printf("%s %d session(s) across %d quarter(s):\n", verb, plan.SessionCount(), len(plan.Quarters))
	for _, q := range plan.Quarters {
		fmt.Printf("  refs/cairn/archive/%s  (%d sessions)\n", q.Label, len(q.Sessions))
		for _, s := range q.Sessions {
			fmt.Printf("    %s\n", s.ULID)
		}
	}
}

// runRestoreArchive handles `cairn restore-archive <ULID>`.
func runRestoreArchive(args []string) error {
	fs := flag.NewFlagSet("restore-archive", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: restore-archive <ULID>")
	}
	ulid := fs.Arg(0)

	repoRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}

	if err := archive.Restore(repoRoot, ulid, time.Now().UTC()); err != nil {
		return err
	}
	fmt.Printf("restored refs/cairn/sessions/%s\n", ulid)
	return nil
}
