# ETCH-3: Git ref writer

## Objective

Implement `WriteSessionRef()` — the core function that takes finalized session JSON + agent-trace JSON and creates an orphan git commit via plumbing commands, pointing `refs/cairn/sessions/<ULID>` at it.

## Approach

### Package: `internal/refs`

Two files: `writer.go` (implementation) and `writer_test.go` (tests).

### Data types

```go
type RefMeta struct {
    Runtime      string
    Model        string
    Status       string
    Branch       string
    CommitCount  int
    DurationSecs int
    EndTime      time.Time
}
```

### Function signature

```go
func WriteSessionRef(repoPath, sessionID string, sessionJSON, traceJSON []byte, meta RefMeta) error
```

### Implementation steps (inside WriteSessionRef)

1. **hash-object** — write `sessionJSON` as a blob: `git hash-object -w --stdin` with `sessionJSON` piped to stdin. Same for `traceJSON`.
2. **mktree** — build a tree with exactly two entries: `100644 blob <sha>\tsession.json` and `100644 blob <sha>\tagent-trace.json`. Pipe to `git mktree`.
3. **commit-tree** — create an orphan commit (no `-p` flag). Set env vars for author/committer identity (`cairn <cairn@localhost>`) and timestamps (`GIT_AUTHOR_DATE`, `GIT_COMMITTER_DATE` from `meta.EndTime`). Commit message format per OUTPUT_SPEC.md §4.
4. **update-ref** — point `refs/cairn/sessions/<sessionID>` at the commit.

### Git command execution

A helper `func runGit(repoPath string, stdin []byte, args ...string) (string, error)` that:
- Creates `exec.Command("git", args...)`
- Sets `cmd.Dir = repoPath`
- Optionally pipes stdin
- Returns trimmed stdout or error with stderr context

### Commit message format

```
cairn session <ULID>
agent: <runtime> / <model>
status: <status>
branch: <branch>
commits: <N>
duration: <N>s
```

### Tests (writer_test.go)

1. **TestWriteSessionRef_Basic** — write a session, verify ref exists, commit has no parent, tree has exactly two blobs with correct names, commit message format matches.
2. **TestWriteSessionRef_BlobContent** — `git show <ref>:session.json` and `git show <ref>:agent-trace.json` return the exact input bytes.
3. **TestWriteSessionRef_AuthorCommitter** — verify author/committer are `cairn <cairn@localhost>`.
4. **TestWriteSessionRef_Timestamp** — verify commit timestamp matches `meta.EndTime`.
5. **TestWriteSessionRef_CommitMessage** — verify each line of the commit message.
6. **TestWriteSessionRef_Concurrent** — 20 goroutines each write a distinct session ref concurrently; all 20 refs exist and are valid afterward.

All tests use `testutil.NewTestRepo(t)`.

### Not in scope

- Subcommand wiring (ETCH-5 wires hooks → ref writer)
- Session data capture (ETCH-2 handles accumulation)
- Crash recovery (ETCH-4)

## Risk

Low. Pure git plumbing, no shared state, well-defined inputs/outputs. The only subtlety is getting env vars right for `commit-tree`.

## Estimated effort

Small — one file of ~100 lines of implementation, one file of ~150 lines of tests.
