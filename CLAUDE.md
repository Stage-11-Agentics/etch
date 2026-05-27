# Etch

Flat metadata capture for every AI agent session in a repository, stored as immutable git refs. Built on Entire CLI's hook substrate. Designed for 60–80+ concurrent agents across multiple machines.

- [SPEC.md](./SPEC.md) — requirements and acceptance criteria
- [BUILDPLAN.md](./BUILDPLAN.md) — technical decisions, architecture, ticket breakdown
- [OUTPUT_SPEC.md](./OUTPUT_SPEC.md) — full session record schema and scenario variants
- [PHASE0_RESULTS.md](./PHASE0_RESULTS.md) — Phase 0 validation gate results

**Project home:** `/Users/atin/Projects/Stage11/code/Etch`
**Remote:** `forgejo.stage11.ai:s11/etch`

## Naming

The project is **Etch**. The binary is `entire-agent-cairn` (Entire's plugin discovery requires `entire-agent-<name>`). Environment variables use `CAIRN_*` namespace. Migration to `ETCH_*` / `entire-agent-etch` is deferred — don't block on it.

## Autonomy default — Fully Autonomous

Lattice orchestrator runs default to Fully Autonomous for this project.

## PR merge policy — auto-merge through to done

## Tech stack

- **Go 1.22+** — single static binary, no runtime dependencies
- **Git plumbing** — `hash-object`, `mktree`, `commit-tree`, `update-ref` via shell exec
- **ULID** — `oklog/ulid` for session IDs
- **No frameworks** — plain subcommand dispatch, `encoding/json`

## Build / test / run

```bash
cd /Users/atin/Projects/Stage11/code/Etch
go build -o entire-agent-cairn ./cmd/entire-agent-cairn   # build
go test ./...                                               # test
# Install: copy binary to a directory on $PATH
cp entire-agent-cairn ~/.local/bin/
```

## Key design decisions

- Per-session refs (`refs/cairn/sessions/<ULID>`) — zero-contention writes, immutable after creation
- Entire plugin protocol for hook substrate — no need to rebuild 8+ agent runtime integrations
- Agent Trace emission alongside internal format — free interop with Cursor/Cognition ecosystem
- Flat records, no hierarchy — structure emerges from shared identifiers at query time
- Crash recovery via `.wip.jsonl` buffer files — partial records committed on next invocation

## Testing philosophy

Etch is pure git plumbing — every test runs on the filesystem with zero external dependencies. This makes comprehensive testing not just possible but mandatory.

**Unit tests per ticket:** Every ticket ships with tests. No exceptions. A Go binary that touches git refs is trivially testable:
1. Create a temp git repo (`git init` in a tmpdir)
2. Pipe simulated hook events (stdin JSON) to the binary
3. Verify the output: refs exist, session.json is valid, blobs are correct, .wip files behave as expected
4. Clean up

**Test helpers:** Build a shared `testutil` package early (in ETCH-1) that provides:
- `NewTestRepo()` — creates a temp git repo, returns path + cleanup func
- `SimulateHookEvent(subcommand, json)` — runs the binary with stdin
- `ReadSessionRef(repo, ulid)` — reads and parses session.json from a ref
- `MustValidateSchema(t, session)` — validates against cairn.session.v1

**Dogfooding:** The sessions building Etch ARE test data. Once the binary works, enable it on this repo — every agent session becomes a live integration test.

**Loopy validation:** Delegators must validate their own work before declaring done. Run the tests, inspect the output, iterate if tests fail. Don't stop at "I wrote the code" — stop at "the tests pass and I verified the behavior."

## Conventions

- Session records are `cairn.session.v1` schema (see OUTPUT_SPEC.md)
- Commit author for session refs: `cairn <cairn@localhost>`
- Orphan commits only — no DAG entanglement between session refs
- Secret scanning is best-effort regex, not exhaustive

## Lessons learned

See [lessons-learned.md](./lessons-learned.md). Every agent working in this project must append an entry when they hit a failure, point of confusion, or thrash that cost time. Format and trigger conditions are in that file.

## Reference artifacts (read-only)

These are research/design artifacts from the pre-implementation phase. Don't merge into SPEC/BUILDPLAN — they're reference material:

- [CAIRN.md](./CAIRN.md) — comprehensive design document (pre-SPEC consolidation)
- [CAIRN_PLAN.md](./CAIRN_PLAN.md) — architecture diagrams (Mermaid)
- [RESEARCH.md](./RESEARCH.md) — prior art research
- [ENTIRE_EVAL.md](./ENTIRE_EVAL.md) — deprecated (Forge++ era)
- `forge-solution-review-pack-*/` — multi-model design reviews
