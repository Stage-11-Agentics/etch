package index

// Update incrementally refreshes the index for repo. If no index exists yet it
// is equivalent to Build. Otherwise it reads the existing entries, then walks
// only the session refs not already indexed (matched by session_id, derived
// from the ref name without reading any blob) and appends them. Existing
// entries are never re-read — BuildResult.Parsed counts only the new blobs
// loaded, and Skipped counts the refs left untouched.
func Update(repo string) (BuildResult, error) {
	if !Exists(repo) {
		return Build(repo)
	}

	_, existing, err := Load(repo)
	if err != nil {
		return BuildResult{}, err
	}

	known := make(map[string]bool, len(existing))
	for _, e := range existing {
		known[e.SessionID] = true
	}

	ids, err := listSessionIDs(repo)
	if err != nil {
		return BuildResult{}, err
	}

	// Only the not-yet-indexed refs need their blobs read; the rest are skipped
	// without any git show — this is what keeps update incremental.
	var newIDs []string
	var res BuildResult
	for _, id := range ids {
		if known[id] {
			res.Skipped++
			continue
		}
		newIDs = append(newIDs, id)
	}

	sessions, err := batchReadSessions(repo, newIDs)
	if err != nil {
		return BuildResult{}, err
	}

	entries := existing
	for _, id := range newIDs {
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
