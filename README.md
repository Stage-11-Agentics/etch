# Etch

[![CI](https://github.com/Stage-11-Agentics/etch/actions/workflows/ci.yml/badge.svg)](https://github.com/Stage-11-Agentics/etch/actions/workflows/ci.yml)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)
[![Go 1.26+](https://img.shields.io/badge/go-1.26%2B-00ADD8.svg)](./go.mod)

**The flight recorder for AI agent fleets.** Etch captures every AI agent
session in a repository — prompt, model, files touched, ticket, timing,
machine, git state — and stores each one as an immutable git ref. No server,
no database, no account. The write path holds at 60–80+ concurrent agents
across worktrees and machines, because every session is its own ref and
nothing ever contends.

Run enough agents and the question stops being *is the code good?* and
becomes *what actually happened?* Thirty sessions ran in your repo overnight,
across four worktrees and two machines. Which one touched the auth
middleware? Which ticket was it carrying? Did it finish, or die mid-run? If
the answer lives in terminal scrollback, it is already gone. Etch gives the
repository a memory — and the primary reader of that memory is the **next
agent** that starts work there, not a human at a dashboard.

## What a captured session looks like

Every session ends as a table you can filter and a record you can trust.
Both outputs below are real, produced by this binary:

```console
$ entire-agent-etch query --repo .
SESSION   RUNTIME/MODEL                TICKET   DURATION  STATUS
01KTYTCR  claude-code/claude-opus-4-8  ETCH-49  1s        complete
```

```console
$ git show refs/etch/sessions/01KTYTCRTX2D64T59S0G49C2V5:session.json | jq
```

```jsonc
{
  "schema_version": "etch.session.v1",
  "session_id": "01KTYTCRTX2D64T59S0G49C2V5",
  "status": "complete",
  "agent": {
    "runtime": "claude-code",
    "model": "claude-opus-4-8",
    "version": "0.01.001"
  },
  "prompt": {
    "text": "Fix the flaky retry test in internal/backoff and make the suite green.",
    "truncated": false
  },
  "orchestration": {
    "type": "lattice-orchestrator",
    "ticket_id": "ETCH-49",
    "run_id": "run-2026-06-12-a",
    "role": "delegator"
  },
  "timing": {
    "started_at": "2026-06-12T21:04:19.007483Z",
    "duration_ms": 1126
  },
  "machine": {
    "hostname_hash": "sha256:65b7f5ae0749a30a2bbeef385ee5a487…",
    "hostname_raw": null,
    "os": "darwin",
    "arch": "arm64"
  },
  "git_start": { "branch": "main", "head_sha": "66478b8a…", "is_worktree": false },
  "files_touched": [
    { "path": "internal/backoff/backoff_test.go", "action": "modified" }
  ],
  "tool_use": { "total_calls": 1, "by_tool": { "Edit": 1 } }
  // … abridged — full field reference in OUTPUT_SPEC.md
}
```

Each record is an orphan commit at `refs/etch/sessions/<ULID>`, alongside an
[Agent Trace](https://github.com/cursor/agent-trace) (`agent-trace.json`)
sidecar for interop with the Cursor/Cognition ecosystem. Refs travel over
ordinary `git push` / `git fetch` between your own machines — that is the
entire transport layer.

## Why Etch

- **Built for fleets, not a laptop.** Per-session refs mean zero write
  contention at any concurrency — stress-validated at 20 concurrent sessions
  per repo, designed for 60–80+ across a fleet. Single-agent capture tools
  assume one terminal and one human; Etch assumes a swarm.
- **Worktree-proof.** Most of a fleet's work happens in worktrees on feature
  branches. All worktrees share one ref store, one crash-recovery buffer, one
  config — and operator mode stamps every future worktree automatically.
- **Sovereign.** No cloud, no account, no telemetry, no control plane. Git
  refs you already know how to push, fetch, and grep, on hosts you control.
- **Invisible.** Capture rides hooks fired by the agent runtime. No wrapper,
  no special launcher, nothing changes about how you run your agents.
- **Crash-honest.** Sessions that die without a clean end are recovered from
  their `.wip.jsonl` buffer on the next invocation and committed as
  `status: incomplete, exit_reason: crash` — the record never silently loses
  a session.
- **Permanent.** Records are immutable the moment they land. This is a
  queryable substrate that accumulates value, not a feed you watch and
  forget. ([PHILOSOPHY.md](./PHILOSOPHY.md))

## Quickstart

Five minutes from install to your first captured session. Requirements:
**Git 2.30+** and a runtime that fires hooks — the installer wires **Claude
Code** today; other runtimes hand-feed the same stdin contract
([docs/HOOK_CONTRACT.md](./docs/HOOK_CONTRACT.md)). Building from source
needs **Go 1.26+**.

**1. Install** (the `entire-agent-<name>` binary name is how
[Entire CLI](https://github.com/entireio/cli) discovers Etch as a plugin;
Entire itself is optional — hooks dispatch straight to the binary):

```bash
go install github.com/Stage-11-Agentics/etch/cmd/entire-agent-etch@latest
```

or from a clone:

```bash
git clone https://github.com/Stage-11-Agentics/etch && cd etch
PREFIX=$HOME/.local make install   # default PREFIX=/usr/local (may need sudo)
```

Verify — the binary must be on `$PATH`:

```bash
entire-agent-etch info
# → {"protocol_version":1,"name":"etch","type":"etch",...,"capabilities":{"hooks":true,...}}
```

**2. Enable capture** in a repository (operator mode — covers every branch
and every worktree of your clone, commits nothing):

```console
$ cd your-repo
$ entire-agent-etch enable
etch: enabled
  etch.enabled = true (.git/config)
  managed ignore block in .git/info/exclude
  1 worktree(s) newly stamped, 0 already stamped (.claude/settings.local.json)
  post-checkout self-propagation in .git/hooks/post-checkout
```

**3. Run an agent session.** Work as you normally would — a Claude Code
session in the repo fires the installed hooks, and the record is committed
the moment the session ends.

**4. See the record:**

```bash
entire-agent-etch query --repo .          # table of captured sessions
git for-each-ref refs/etch/sessions/      # the refs themselves
git show refs/etch/sessions/<ULID>:session.json | jq
```

If anything looks quiet, `entire-agent-etch doctor` tells you why — see
[Health check](#health-check-doctor).

## Enablement modes

Two ways a repository opts in, serving two different owners. Full design and
edge cases: [docs/ENABLEMENT.md](./docs/ENABLEMENT.md).

| | **Operator mode** | **Team mode** |
|---|---|---|
| Command | `entire-agent-etch enable` | `entire-agent-etch install-hooks` |
| Owner | you, per clone | the repo, for every contributor |
| Lives in | git config + untracked stamps | `.claude/settings.json`, committed |
| Coverage | all branches, all worktrees, including future ones | rides branch content — branches predating the enablement commit have no hooks |
| Footprint | nothing committed, nothing in any diff | distributed through clone/pull |

Operator mode sets `etch.enabled=true` in the repo's git config, stamps every
existing worktree, and installs a `post-checkout` hook so future worktrees
stamp themselves at `git worktree add`. `entire-agent-etch disable` stops all
capture from either mode. The modes coexist without double capture — stamps
yield to committed entries.

Team-mode hook commands are guarded (`command -v … || exit 0`): contributors
and CI without the binary are completely untouched. With Entire CLI
installed, `entire enable --agent etch --no-github` drives the same
install through Entire's plugin protocol (use that form, not
`entire agent add etch`, which skips external-agent discovery on v0.6.3).

## Syncing records between machines

> **Records contain prompt text. Only sync them to private remotes.**
> On public repos, capture stays local-only — no refspec, nothing pushed.
> This is the posture this very repository runs.

Custom ref namespaces don't sync by default. On a **private** remote, one
command makes session refs ride your ordinary `git push` / `git fetch`:

```bash
entire-agent-etch setup-refspec                   # picks origin
entire-agent-etch setup-refspec --remote backup   # once per extra remote
```

`setup-refspec` adds the `refs/etch/sessions/*` fetch and push refspecs plus
a `HEAD` push entry, so a bare `git push` keeps pushing your current branch
alongside the session refs (git drops its implicit push behavior the moment
any push refspec exists). If you already configure your own push refspecs, it
adds only the etch entry. Rerunning is safe and upgrades older configs.

A fresh clone has zero etch refs until you run `setup-refspec` and
`git fetch` once; from then on every ordinary fetch keeps them in sync. An
agent finishing work on one machine is visible to the next agent that picks
up the repo anywhere else.

## Querying

`query` prints captured sessions (newest first), with filters that compose:

```bash
entire-agent-etch query --repo .                 # every captured session
entire-agent-etch query --runtime claude-code    # by agent runtime
entire-agent-etch query --ticket ETCH-9          # by orchestration ticket
entire-agent-etch query --status incomplete      # complete | incomplete
entire-agent-etch query --since 2026-06-01T00:00:00Z --until 2026-06-07T00:00:00Z
entire-agent-etch query --branch main --has-files 'src/**'
entire-agent-etch query --json                   # full records as a JSON array
entire-agent-etch query --count                  # just the count
```

Also: `--exit-reason`, `--run-id`, `--sort`, `--reverse`, `--no-index`.

For repos with many sessions, a materialized index accelerates `query`
(used automatically when present, falls back to walking refs):

```bash
entire-agent-etch index build    # build from scratch
entire-agent-etch index update   # incrementally add new sessions
entire-agent-etch index show     # path, session count, size, built_at
entire-agent-etch index drop     # remove it (query falls back to refs)
```

Age old sessions out of the active namespace into per-quarter archive refs
(default threshold 90 days, configurable):

```bash
entire-agent-etch archive --dry-run
entire-agent-etch archive
entire-agent-etch restore-archive <ULID>          # bring one back
```

`entire-agent-etch help` lists every subcommand.

## Health check (`doctor`)

One command answers "is Etch actually capturing in this repo?":

```console
$ entire-agent-etch doctor
etch doctor — /path/to/repo
  ✓ binary       on PATH at ~/.local/bin/entire-agent-etch (v0.01.001)
  ✓ enablement   operator mode (etch.enabled=true; all worktrees, all branches)
  ✓ hooks        committed 0/5, stamp 5/5
  • refspec      no remotes — local-only capture
  ✓ sessions     newest moments ago (1 total)
  ✓ wip-buffers  none
  ✓ stamps       1/1 worktree(s) stamped
  ✓ propagation  post-checkout block installed (.git/hooks)
  ✓ dedupe       all stamps carry the committed-entries-win guard
ok — capture healthy
```

Hard failures (binary missing, hook coverage missing while capture isn't
explicitly disabled) exit non-zero; everything else is a warning. Doctor
never writes. `--json` for a structured report, `--warn-age N` to tune the
stale-capture threshold.

## Privacy posture

Session records include prompt text. Etch treats that as a fact to design
around, not a footnote:

- **Local-only by default.** Capture writes to your local ref store. Nothing
  leaves the machine unless you explicitly configure a refspec — and you only
  do that for **private** remotes. `setup-refspec` never writes a
  `refs/etch/*` wildcard that could catch the local namespace.
- **Machine identity is hashed.** Hostnames are stored as
  `SHA-256(salt + hostname)` with a random per-repo salt, so hashes don't
  correlate across repos and low-entropy hostnames aren't directly
  rainbow-tableable. Raw hostname is opt-in (`raw_machine_identity: true`).
  On private repos, commit `.etch/settings.json` so clones share the salt;
  on public repos, leave it untracked.
- **`local_only_fields` keeps selected fields off the wire entirely.** Dot
  paths into `session.json` (e.g. `"prompt.text"`) are stripped **at commit
  time**: the pushable `refs/etch/sessions/<ULID>` ref holds the stripped
  record, while the full-fidelity twin lives at `refs/etch/local/<ULID>`,
  which no etch-written refspec ever names. Safety does not depend on
  refspec config — the pushable namespace never contained the field.
  Stripped values carry a `[LOCAL_ONLY:<path>]` marker and are listed in the
  record's `local_only_stripped` manifest. Note that local tooling (`query`,
  `index`) reads the stripped record too; full fidelity is at
  `git show refs/etch/local/<ULID>:session.json`.
- **Best-effort secret scanning** runs over captured prompts before commit
  (regex-based, **not exhaustive** — it is a seatbelt, not a publishing
  gate). Extend it with `redaction_patterns`.

Tune all of this in `.etch/settings.json` at the repo root (all fields
optional): `raw_machine_identity`, `local_only_fields`,
`archive_threshold_days`, `redaction_patterns`, `recovery_timeout_hours` —
field semantics in [OUTPUT_SPEC.md](./OUTPUT_SPEC.md) and
[docs/ENABLEMENT.md](./docs/ENABLEMENT.md).

## How it works

Pure git plumbing. Hook events stream into a per-session `.wip.jsonl` buffer;
`session_end` finalizes the buffer into an **orphan commit** holding
`session.json` and `agent-trace.json`, pointed at by
`refs/etch/sessions/<ULID>`. Orphan commits share no DAG with your branches —
your history never sees them. Per-session refs are why concurrency is free:
no shared branch, no merge churn, no last-writer-wins. Records are flat by
design; structure (runs, tickets, lineage) emerges at query time from shared
identifiers. Orchestration context arrives through `ETCH_*` environment
variables (`ETCH_TICKET_ID`, `ETCH_RUN_ID`, `ETCH_AGENT_ROLE`, …) set by
whatever drives your agents.

Per-event hooks complete in ≤ 50 ms; the once-per-session start hook stays
flat as the orphaned-buffer population grows. The budgets are acceptance
criteria, not aspirations — see [SPEC.md](./SPEC.md).

Depth lives in the docs: [SPEC.md](./SPEC.md) (guarantees),
[OUTPUT_SPEC.md](./OUTPUT_SPEC.md) (full `etch.session.v1` schema),
[docs/HOOK_CONTRACT.md](./docs/HOOK_CONTRACT.md) (the process boundary),
[docs/ENABLEMENT.md](./docs/ENABLEMENT.md) (modes),
[BUILDPLAN.md](./BUILDPLAN.md) (architecture decisions).

## What Etch is not

Scope honesty, so you can decide fast:

- **Not session restore.** Entire's own Checkpoints rewinds a working
  session; Etch records the fleet's history permanently. Complementary
  layers — Etch rides Entire's hook substrate and adds the record store.
- **Not a dashboard.** No web UI, no live feed. The data lives in git;
  consumers (usually the next agent, via `query`) bring the intelligence.
  If you want real-time human monitoring, run a dashboard tool alongside.
- **Not an analysis engine.** Etch captures and stores. Correlations
  ("which prompts produce clean CI?") are downstream consumers of the data.
- **Not line-level attribution.** Tools like Git AI and Agent Blame annotate
  *who wrote each line*; Etch records *what each session did*. Different
  layer. (Etch emits [Agent Trace](https://github.com/cursor/agent-trace)
  records, so attribution toolchains can read its output.)
- **Not a publishing gate.** Secret scanning is best-effort regex. The real
  guarantees are local-only defaults and commit-time `local_only_fields`
  projection.

## Development

```bash
make build          # compile ./bin/entire-agent-etch
make test           # unit tests (go test ./...)
make test-density   # 20-concurrent-session stress test
make smoke          # end-to-end smoke test against the real Entire CLI
make help           # list all targets
```

```
cmd/entire-agent-etch/   # binary entrypoint + subcommand dispatch
internal/                # capture, hooks, refs, recovery, redact, schema, config, ...
test/density/            # concurrency stress tests (build tag: density)
scripts/                 # smoke.sh and friends
```

Zero runtime dependencies; every test runs on the filesystem against a temp
git repo. Etch captures its own development sessions — the repo is its own
integration test. Conventions and testing philosophy: [CLAUDE.md](./CLAUDE.md).

## License

[Apache License 2.0](./LICENSE) © Stage 11 Agentics.
