package index

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Stage-11-Agentics/etch/internal/schema"
)

const refPrefix = "refs/etch/sessions/"

// nowRFC3339 is overridable in tests for deterministic timestamps.
var nowRFC3339 = func() string { return time.Now().UTC().Format(time.RFC3339) }

// Build walks every refs/etch/sessions/* ref, parses each session.json, and
// writes a fresh index file. Blobs are read through a single git cat-file
// --batch process rather than one git show per ref, so a 1000-session build
// stays well under a second. Any ref whose session.json is missing or
// unparseable is skipped (not fatal).
func Build(repo string) (BuildResult, error) {
	ids, err := listSessionIDs(repo)
	if err != nil {
		return BuildResult{}, err
	}

	sessions, err := batchReadSessions(repo, ids)
	if err != nil {
		return BuildResult{}, err
	}

	var (
		entries []Entry
		res     BuildResult
	)
	for _, id := range ids {
		s, ok := sessions[id]
		if !ok {
			continue
		}
		res.Parsed++
		entries = append(entries, EntryFromSession(s))
	}

	if err := Write(repo, nowRFC3339(), entries); err != nil {
		return BuildResult{}, err
	}
	res.Total = len(entries)
	return res, nil
}

// listSessionIDs returns the session_id (ULID) of every etch session ref,
// derived from the ref name without reading any blob.
func listSessionIDs(repo string) ([]string, error) {
	out, err := runGit(repo, "for-each-ref", refPrefix, "--format=%(refname)")
	if err != nil {
		return nil, fmt.Errorf("listing session refs: %w", err)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}
	var ids []string
	for _, ref := range strings.Split(trimmed, "\n") {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		ids = append(ids, strings.TrimPrefix(ref, refPrefix))
	}
	return ids, nil
}

// batchReadSessions reads session.json for each id through one git cat-file
// --batch process and returns a map keyed by the ref-derived id. Missing or
// unparseable blobs are simply absent from the map. The result is keyed by the
// ref id (not the record's session_id) so callers can map back to refs even if
// a record omits its own id.
func batchReadSessions(repo string, ids []string) (map[string]schema.Session, error) {
	result := make(map[string]schema.Session, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	args := []string{}
	if repo != "" {
		args = append(args, "-C", repo)
	}
	args = append(args, "cat-file", "--batch")
	cmd := exec.Command("git", args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("cat-file stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("cat-file stdout: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting cat-file: %w", err)
	}

	// Feed specs concurrently with reading responses to avoid pipe-buffer deadlock.
	go func() {
		w := bufio.NewWriter(stdin)
		for _, id := range ids {
			fmt.Fprintf(w, "%s%s:session.json\n", refPrefix, id)
		}
		w.Flush()
		stdin.Close()
	}()

	r := bufio.NewReader(stdout)
	for _, id := range ids {
		header, err := r.ReadString('\n')
		if err != nil {
			break // stream ended early; remaining ids are simply absent
		}
		fields := strings.Fields(strings.TrimRight(header, "\n"))
		// Missing / ambiguous: "<spec> missing" or "<spec> ambiguous".
		if len(fields) < 3 {
			continue
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}
		buf := make([]byte, size)
		if _, err := io.ReadFull(r, buf); err != nil {
			break
		}
		// Consume the trailing LF that follows the object contents.
		if _, err := r.ReadByte(); err != nil {
			break
		}
		var s schema.Session
		if err := json.Unmarshal(buf, &s); err != nil {
			continue
		}
		if s.SessionID == "" {
			s.SessionID = id
		}
		result[id] = s
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("git cat-file --batch: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return result, nil
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
