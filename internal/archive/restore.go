package archive

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Stage-11-Agentics/etch/internal/refs"
	"github.com/Stage-11-Agentics/etch/internal/schema"
)

// Restore recreates refs/etch/sessions/<ulid> from whichever archive ref holds it.
// Used for forensic recovery after compaction.
func Restore(repoRoot, ulid string, now time.Time) error {
	archiveRef, ok := findArchiveContaining(repoRoot, ulid)
	if !ok {
		return fmt.Errorf("session %s not found in any archive ref", ulid)
	}

	sessionJSON, err := readBlob(repoRoot, archiveRef, ulid+"/session.json")
	if err != nil {
		return fmt.Errorf("reading session.json for %s: %w", ulid, err)
	}
	traceJSON, err := readBlob(repoRoot, archiveRef, ulid+"/agent-trace.json")
	if err != nil {
		return fmt.Errorf("reading agent-trace.json for %s: %w", ulid, err)
	}

	meta := metaFromSession(sessionJSON, now)
	if err := refs.WriteSessionRef(repoRoot, ulid, sessionJSON, traceJSON, meta); err != nil {
		return fmt.Errorf("recreating session ref for %s: %w", ulid, err)
	}
	return nil
}

// findArchiveContaining returns the first archive ref whose tree contains
// <ulid>/session.json.
func findArchiveContaining(repoRoot, ulid string) (string, bool) {
	out, err := runGit(repoRoot, nil, "for-each-ref", "--format=%(refname)", archivePrefix)
	if err != nil {
		return "", false
	}
	for _, ref := range strings.Split(strings.TrimSpace(out), "\n") {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		// `git cat-file -e <ref>:<path>` exits 0 iff the path exists.
		if _, err := runGit(repoRoot, nil, "cat-file", "-e", ref+":"+ulid+"/session.json"); err == nil {
			return ref, true
		}
	}
	return "", false
}

func readBlob(repoRoot, ref, path string) ([]byte, error) {
	out, err := runGit(repoRoot, nil, "cat-file", "blob", ref+":"+path)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

// metaFromSession derives RefMeta from the archived session.json so the recreated
// commit message is meaningful. Missing fields fall back to zero values.
func metaFromSession(sessionJSON []byte, now time.Time) refs.RefMeta {
	meta := refs.RefMeta{EndTime: now.UTC()}
	var s schema.Session
	if err := json.Unmarshal(sessionJSON, &s); err != nil {
		return meta
	}
	meta.Runtime = s.Agent.Runtime
	if s.Agent.Model != nil {
		meta.Model = *s.Agent.Model
	}
	meta.Status = s.Status
	if s.GitEnd != nil {
		meta.Branch = s.GitEnd.Branch
	} else if s.GitStart != nil {
		meta.Branch = s.GitStart.Branch
	}
	if s.Outcome != nil {
		meta.CommitCount = len(s.Outcome.Commits)
	}
	if s.Timing.DurationMS != nil {
		meta.DurationSecs = int(*s.Timing.DurationMS / 1000)
	}
	return meta
}
