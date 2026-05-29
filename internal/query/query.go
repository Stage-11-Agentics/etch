package query

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"forgejo.stage11.ai/s11/etch/internal/schema"
)

// Run parses args and executes the query subcommand, writing results to stdout.
func Run(args []string) error {
	return RunTo(args, os.Stdout, os.Stderr)
}

// RunTo is Run with explicit output writers, for testability.
func RunTo(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		repo       = fs.String("repo", "", "path to the git repo (default: current directory)")
		ticket     = fs.String("ticket", "", "filter by orchestration.ticket_id")
		runtime    = fs.String("runtime", "", "filter by agent.runtime")
		status     = fs.String("status", "", "filter by status (complete/incomplete)")
		exitReason = fs.String("exit-reason", "", "filter by exit_reason")
		runID      = fs.String("run-id", "", "filter by orchestration.run_id")
		since      = fs.String("since", "", "filter sessions started at or after this RFC3339 time")
		until      = fs.String("until", "", "filter sessions started at or before this RFC3339 time")
		hasFiles   = fs.String("has-files", "", "filter by files_touched path glob")
		branch     = fs.String("branch", "", "filter by git_start.branch or git_end.branch")
		asJSON     = fs.Bool("json", false, "output a JSON array of full session records")
		count      = fs.Bool("count", false, "output only the count of matching sessions")
		sortKey    = fs.String("sort", "started_at", "sort key: started_at|duration|session_id")
		reverse    = fs.Bool("reverse", false, "reverse the sort order")
	)

	if err := fs.Parse(args); err != nil {
		return err
	}

	switch *sortKey {
	case "started_at", "duration", "session_id":
	default:
		return fmt.Errorf("invalid --sort %q: must be started_at, duration, or session_id", *sortKey)
	}

	filters := Filters{
		Ticket:     *ticket,
		Runtime:    *runtime,
		Status:     *status,
		ExitReason: *exitReason,
		RunID:      *runID,
		Since:      *since,
		Until:      *until,
		HasFiles:   *hasFiles,
		Branch:     *branch,
	}

	sessions, err := loadSessions(*repo, stderr)
	if err != nil {
		return err
	}

	var matched []schema.Session
	for _, s := range sessions {
		if filters.Match(&s) {
			matched = append(matched, s)
		}
	}

	sortSessions(matched, *sortKey, *reverse)

	switch {
	case *count:
		fmt.Fprintf(stdout, "%d\n", len(matched))
	case *asJSON:
		return writeJSON(stdout, matched)
	default:
		writeTable(stdout, matched)
	}
	return nil
}

// loadSessions enumerates refs/cairn/sessions/* and parses each session.json.
// Refs whose session.json is missing or unparseable are skipped with a warning.
func loadSessions(repo string, stderr io.Writer) ([]schema.Session, error) {
	refNames, err := listSessionRefs(repo)
	if err != nil {
		return nil, err
	}

	var sessions []schema.Session
	for _, ref := range refNames {
		data, err := gitShow(repo, ref+":session.json")
		if err != nil {
			fmt.Fprintf(stderr, "warning: skipping %s: %v\n", ref, err)
			continue
		}
		var s schema.Session
		if err := json.Unmarshal(data, &s); err != nil {
			fmt.Fprintf(stderr, "warning: skipping %s: invalid session.json: %v\n", ref, err)
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func listSessionRefs(repo string) ([]string, error) {
	out, err := runGit(repo, "for-each-ref", "refs/cairn/sessions/", "--format=%(refname:short)")
	if err != nil {
		return nil, fmt.Errorf("listing session refs: %w", err)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}
	return strings.Split(trimmed, "\n"), nil
}

func gitShow(repo, spec string) ([]byte, error) {
	return runGit(repo, "show", spec)
}

func runGit(repo string, args ...string) ([]byte, error) {
	fullArgs := args
	if repo != "" {
		fullArgs = append([]string{"-C", repo}, args...)
	}
	cmd := exec.Command("git", fullArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func sortSessions(sessions []schema.Session, key string, reverse bool) {
	// Default ordering is descending (newest/largest first); --reverse flips it.
	less := func(i, j int) bool {
		switch key {
		case "duration":
			return durationOf(sessions[i]) > durationOf(sessions[j])
		case "session_id":
			return sessions[i].SessionID > sessions[j].SessionID
		default: // started_at
			ti, oki := startedAtTime(sessions[i])
			tj, okj := startedAtTime(sessions[j])
			// Sessions without a start time sort last in the default (desc) order.
			if oki != okj {
				return oki
			}
			if !oki && !okj {
				return sessions[i].SessionID > sessions[j].SessionID
			}
			return ti.After(tj)
		}
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		if reverse {
			return less(j, i)
		}
		return less(i, j)
	})
}

func durationOf(s schema.Session) int64 {
	if s.Timing.DurationMS != nil {
		return *s.Timing.DurationMS
	}
	return -1
}

func startedAtTime(s schema.Session) (time.Time, bool) {
	if s.Timing.StartedAt == nil || *s.Timing.StartedAt == "" {
		return time.Time{}, false
	}
	t, err := parseTime(*s.Timing.StartedAt)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

func writeJSON(w io.Writer, sessions []schema.Session) error {
	if sessions == nil {
		sessions = []schema.Session{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(sessions)
}

func writeTable(w io.Writer, sessions []schema.Session) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "SESSION\tRUNTIME/MODEL\tTICKET\tDURATION\tSTATUS")
	for _, s := range sessions {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			shortID(s.SessionID),
			runtimeModel(s),
			ticketOf(s),
			durationHuman(s),
			s.Status,
		)
	}
	tw.Flush()
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func runtimeModel(s schema.Session) string {
	rt := s.Agent.Runtime
	if rt == "" {
		rt = "-"
	}
	if s.Agent.Model != nil && *s.Agent.Model != "" {
		return rt + "/" + *s.Agent.Model
	}
	return rt
}

func ticketOf(s schema.Session) string {
	if s.Orchestration != nil && s.Orchestration.TicketID != nil && *s.Orchestration.TicketID != "" {
		return *s.Orchestration.TicketID
	}
	return "-"
}

func durationHuman(s schema.Session) string {
	if s.Timing.DurationMS == nil {
		return "-"
	}
	d := time.Duration(*s.Timing.DurationMS) * time.Millisecond
	return d.Round(time.Second).String()
}
