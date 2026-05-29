package query

import (
	"path"

	"forgejo.stage11.ai/s11/etch/internal/schema"
)

// Filters holds the AND-combined query criteria. An empty string (or nil-ish
// zero value) means the corresponding filter is unset and matches everything.
type Filters struct {
	Ticket     string
	Runtime    string
	Status     string
	ExitReason string
	RunID      string
	Since      string // RFC3339
	Until      string // RFC3339
	HasFiles   string // glob pattern
	Branch     string
}

// Match reports whether s satisfies every set filter. Unset filters are skipped.
// Filters AND together: a session must satisfy all of them to match.
func (f Filters) Match(s *schema.Session) bool {
	if f.Ticket != "" {
		if s.Orchestration == nil || s.Orchestration.TicketID == nil || *s.Orchestration.TicketID != f.Ticket {
			return false
		}
	}
	if f.Runtime != "" {
		if s.Agent.Runtime != f.Runtime {
			return false
		}
	}
	if f.Status != "" {
		if s.Status != f.Status {
			return false
		}
	}
	if f.ExitReason != "" {
		if s.ExitReason != f.ExitReason {
			return false
		}
	}
	if f.RunID != "" {
		if s.Orchestration == nil || s.Orchestration.RunID == nil || *s.Orchestration.RunID != f.RunID {
			return false
		}
	}
	if f.Since != "" {
		if !startedAtOnOrAfter(s, f.Since) {
			return false
		}
	}
	if f.Until != "" {
		if !startedAtOnOrBefore(s, f.Until) {
			return false
		}
	}
	if f.HasFiles != "" {
		if !matchesAnyFile(s, f.HasFiles) {
			return false
		}
	}
	if f.Branch != "" {
		if !matchesBranch(s, f.Branch) {
			return false
		}
	}
	return true
}

func startedAt(s *schema.Session) (string, bool) {
	if s.Timing.StartedAt == nil || *s.Timing.StartedAt == "" {
		return "", false
	}
	return *s.Timing.StartedAt, true
}

// startedAtOnOrAfter reports whether the session started at or after the given
// RFC3339 bound. Sessions without a parseable start time do not match a range
// filter.
func startedAtOnOrAfter(s *schema.Session, bound string) bool {
	at, ok := startedAt(s)
	if !ok {
		return false
	}
	t, err := parseTime(at)
	if err != nil {
		return false
	}
	b, err := parseTime(bound)
	if err != nil {
		return false
	}
	return !t.Before(b)
}

func startedAtOnOrBefore(s *schema.Session, bound string) bool {
	at, ok := startedAt(s)
	if !ok {
		return false
	}
	t, err := parseTime(at)
	if err != nil {
		return false
	}
	b, err := parseTime(bound)
	if err != nil {
		return false
	}
	return !t.After(b)
}

func matchesAnyFile(s *schema.Session, pattern string) bool {
	for _, fe := range s.FilesTouched {
		if ok, err := path.Match(pattern, fe.Path); err == nil && ok {
			return true
		}
		// Also match against the base name so a pattern like "*.go" matches
		// nested paths like "internal/query/query.go".
		if ok, err := path.Match(pattern, path.Base(fe.Path)); err == nil && ok {
			return true
		}
	}
	return false
}

func matchesBranch(s *schema.Session, branch string) bool {
	if s.GitStart != nil && s.GitStart.Branch == branch {
		return true
	}
	if s.GitEnd != nil && s.GitEnd.Branch == branch {
		return true
	}
	return false
}
