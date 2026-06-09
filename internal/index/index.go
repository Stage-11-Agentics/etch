// Package index materializes a fast lookup index from refs/etch/sessions/*.
//
// The index is a JSON-lines file at .etch/index/sessions.idx: a versioned
// header line followed by one flat entry per session. etch query reads it as a
// pre-filter so common questions can be answered without git show-ing every
// ref. The full session.json always stays in its ref; the index carries only
// the commonly-filtered scalar fields plus enough to render the default table.
package index

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Stage-11-Agentics/etch/internal/schema"
)

// SchemaVersion marks the on-disk index format. Bump on incompatible changes.
const SchemaVersion = "etch.index.v1"

// DefaultRelPath is the index location relative to the repo root.
const DefaultRelPath = ".etch/index/sessions.idx"

// Header is the first line of the index file.
type Header struct {
	Schema       string `json:"_schema"`
	BuiltAt      string `json:"_built_at"`
	SessionCount int    `json:"_session_count"`
}

// Entry is one indexed session — a flat projection of the commonly-filtered
// fields from session.json. The full record stays in the ref.
type Entry struct {
	SessionID    string  `json:"session_id"`
	TS           string  `json:"ts,omitempty"` // started_at (RFC3339)
	Runtime      string  `json:"runtime,omitempty"`
	Model        string  `json:"model,omitempty"`
	TicketID     string  `json:"ticket_id,omitempty"`
	RunID        string  `json:"run_id,omitempty"`
	Role         string  `json:"role,omitempty"`
	Status       string  `json:"status,omitempty"`
	ExitReason   string  `json:"exit_reason,omitempty"`
	BranchStart  string  `json:"branch_start,omitempty"`
	BranchEnd    string  `json:"branch_end,omitempty"`
	DurationMS   *int64  `json:"duration_ms,omitempty"`
	FilesCount   int     `json:"files_count"`
	InputTokens  int64   `json:"input_tokens,omitempty"`
	OutputTokens int64   `json:"output_tokens,omitempty"`
	Cost         float64 `json:"cost,omitempty"`
}

// Stats summarizes an index file for `etch index show`.
type Stats struct {
	Count     int    `json:"count"`
	SizeBytes int64  `json:"size_bytes"`
	BuiltAt   string `json:"built_at"`
	Oldest    string `json:"oldest"`
	Newest    string `json:"newest"`
	Path      string `json:"path"`
}

// BuildResult reports what a build/update touched. Parsed counts session.json
// blobs actually read (git show); Skipped counts refs left untouched because
// they were already indexed; Total is the entry count in the resulting index.
type BuildResult struct {
	Parsed  int
	Skipped int
	Total   int
}

// IndexPath returns the absolute path to the index file for repo. An empty repo
// means the current working directory.
func IndexPath(repo string) string {
	if repo == "" {
		repo = "."
	}
	return filepath.Join(repo, DefaultRelPath)
}

// Exists reports whether an index file is present for repo.
func Exists(repo string) bool {
	_, err := os.Stat(IndexPath(repo))
	return err == nil
}

// EntryFromSession projects a full session record into an index entry.
func EntryFromSession(s schema.Session) Entry {
	e := Entry{
		SessionID:  s.SessionID,
		Runtime:    s.Agent.Runtime,
		Status:     s.Status,
		ExitReason: s.ExitReason,
		FilesCount: len(s.FilesTouched),
	}
	if s.Agent.Model != nil {
		e.Model = *s.Agent.Model
	}
	if s.Timing.StartedAt != nil {
		e.TS = *s.Timing.StartedAt
	}
	if s.Timing.DurationMS != nil {
		d := *s.Timing.DurationMS
		e.DurationMS = &d
	}
	if s.Orchestration != nil {
		if s.Orchestration.TicketID != nil {
			e.TicketID = *s.Orchestration.TicketID
		}
		if s.Orchestration.RunID != nil {
			e.RunID = *s.Orchestration.RunID
		}
		if s.Orchestration.Role != nil {
			e.Role = *s.Orchestration.Role
		}
	}
	// Carry both branches: --branch matches against either, so collapsing them
	// would drop matches when a session started and ended on different branches.
	if s.GitStart != nil {
		e.BranchStart = s.GitStart.Branch
	}
	if s.GitEnd != nil {
		e.BranchEnd = s.GitEnd.Branch
	}
	if s.Tokens != nil {
		e.InputTokens = s.Tokens.Input
		e.OutputTokens = s.Tokens.Output
		e.Cost = s.Tokens.EstimatedCostUSD
	}
	return e
}

// EntryToPartialSession reconstructs the subset of a session that the query
// filters, sort, and default table render need. Fields not carried by the index
// (e.g. files_touched paths) are left zero, so callers that need them must load
// the full session.json from the ref.
func EntryToPartialSession(e Entry) schema.Session {
	s := schema.Session{
		SchemaVersion: schema.SchemaVersion,
		SessionID:     e.SessionID,
		Status:        e.Status,
		ExitReason:    e.ExitReason,
		Agent:         schema.Agent{Runtime: e.Runtime},
	}
	if e.Model != "" {
		m := e.Model
		s.Agent.Model = &m
	}
	if e.TS != "" {
		ts := e.TS
		s.Timing.StartedAt = &ts
	}
	if e.DurationMS != nil {
		d := *e.DurationMS
		s.Timing.DurationMS = &d
	}
	if e.TicketID != "" || e.RunID != "" || e.Role != "" {
		o := &schema.Orchestration{}
		if e.TicketID != "" {
			o.TicketID = &e.TicketID
		}
		if e.RunID != "" {
			o.RunID = &e.RunID
		}
		if e.Role != "" {
			o.Role = &e.Role
		}
		s.Orchestration = o
	}
	if e.BranchStart != "" {
		s.GitStart = &schema.GitState{Branch: e.BranchStart}
	}
	if e.BranchEnd != "" {
		s.GitEnd = &schema.GitState{Branch: e.BranchEnd}
	}
	return s
}

// Write persists entries to the index file for repo, creating .etch/index/ as
// needed. It writes a fresh header line followed by one JSON line per entry.
func Write(repo, builtAt string, entries []Entry) error {
	path := IndexPath(repo)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating index dir: %w", err)
	}

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("creating index file: %w", err)
	}

	w := bufio.NewWriter(f)
	header := Header{Schema: SchemaVersion, BuiltAt: builtAt, SessionCount: len(entries)}
	if err := writeLine(w, header); err != nil {
		f.Close()
		return err
	}
	for _, e := range entries {
		if err := writeLine(w, e); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return fmt.Errorf("flushing index: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing index: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("committing index: %w", err)
	}
	return nil
}

func writeLine(w *bufio.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshaling index line: %w", err)
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	return w.WriteByte('\n')
}

// Load reads the index file for repo, returning the header and entries. It
// returns an error if no index exists or the header line is missing/invalid.
func Load(repo string) (Header, []Entry, error) {
	path := IndexPath(repo)
	f, err := os.Open(path)
	if err != nil {
		return Header{}, nil, fmt.Errorf("opening index: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	// Session entries can be large; raise the line cap well above the default 64KB.
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var header Header
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return Header{}, nil, fmt.Errorf("reading index header: %w", err)
		}
		return Header{}, nil, fmt.Errorf("index %s is empty", path)
	}
	if err := json.Unmarshal(sc.Bytes(), &header); err != nil {
		return Header{}, nil, fmt.Errorf("parsing index header: %w", err)
	}

	var entries []Entry
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return Header{}, nil, fmt.Errorf("parsing index entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		return Header{}, nil, fmt.Errorf("reading index: %w", err)
	}
	return header, entries, nil
}

// Show returns summary stats for the index file of repo.
func Show(repo string) (Stats, error) {
	path := IndexPath(repo)
	fi, err := os.Stat(path)
	if err != nil {
		return Stats{}, fmt.Errorf("stat index: %w", err)
	}
	header, entries, err := Load(repo)
	if err != nil {
		return Stats{}, err
	}
	st := Stats{
		Count:     len(entries),
		SizeBytes: fi.Size(),
		BuiltAt:   header.BuiltAt,
		Path:      path,
	}
	for _, e := range entries {
		if e.TS == "" {
			continue
		}
		if st.Oldest == "" || e.TS < st.Oldest {
			st.Oldest = e.TS
		}
		if st.Newest == "" || e.TS > st.Newest {
			st.Newest = e.TS
		}
	}
	return st, nil
}

// Drop deletes the index file for repo. Dropping a non-existent index is a
// no-op (not an error).
func Drop(repo string) error {
	path := IndexPath(repo)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("removing index: %w", err)
	}
	return nil
}
