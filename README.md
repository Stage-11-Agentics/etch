# Etch

Etch captures flat metadata for **every AI agent session in a repository** and
stores it as immutable git refs (`refs/etch/sessions/<ULID>`). It runs invisibly
on [Entire CLI](https://github.com/entireio/cli)'s hook substrate — every session
becomes a permanent, queryable record with zero contention, designed for 60–80+
concurrent agents across multiple machines.

The capture binary is named `entire-agent-etch` (Entire discovers external agents
by the `entire-agent-<name>` naming convention; this registers Etch as the `etch`
agent). Environment variables use the `ETCH_*` namespace.

## Status

- **Production-ready core** — session capture, immutable per-session refs, crash
  recovery via `.wip.jsonl` buffers, [Agent Trace](./OUTPUT_SPEC.md) emission, and
  20-concurrent-session density validation are all in place and tested.
- **Query & lifecycle tooling shipped** — `query` (structured search with
  filters), `index` (materialized index that accelerates query), `archive` /
  `restore-archive` (age old sessions out of the active ref namespace and bring
  them back) are all functional. See [Usage](#usage) below.

## Requirements

- **Go 1.22+** (built and tested on 1.26)
- **Git 2.30+**
- **[Entire CLI](https://github.com/entireio/cli)** — the hook substrate Etch plugs into

## Install

**Quick (from a clone):**

```bash
git clone https://github.com/Stage-11-Agentics/etch && cd etch
make install                      # installs to /usr/local/bin (may need sudo)
# or pick your own prefix:
PREFIX=$HOME/.local make install  # installs to ~/.local/bin
```

**From source with `go install`:**

```bash
go install github.com/Stage-11-Agentics/etch/cmd/entire-agent-etch@latest
```

**Verify the install:**

```bash
entire-agent-etch info
# → {"protocol_version":1,"name":"etch","type":"etch",...,"capabilities":{"hooks":true,...}}
```

The binary must be on your `$PATH` — that is how Entire discovers it as an agent.

## Configure

Etch's binary follows Entire's `entire-agent-<name>` naming convention and
speaks Entire's external-agent protocol (v1), which registers it as the
**`etch` agent**. One command wires capture into a repository:

```bash
cd your-repo
entire enable --agent etch --no-github
```

This discovers `entire-agent-etch` on your `$PATH`, drives its
`install-hooks` subcommand (which writes etch's dispatch entries into
`.claude/settings.json`), and persists `external_agents: true` in
`.entire/settings.json`. It is additive — if you also track sessions with
Entire's own claude-code agent (`entire enable --agent claude-code`), both
sets of hooks coexist; Claude Code runs every hook entry per event.

**Without Entire** (or on any Entire version): the install step is etch's own
subcommand and works standalone —

```bash
cd your-repo
entire-agent-etch install-hooks       # → {"hooks_installed":5}
entire-agent-etch are-hooks-installed # → {"installed":true}
```

At runtime the installed hooks dispatch **directly to the etch binary** with
the agent runtime's native hook JSON — Entire is not in the dispatch path, so
capture keeps working even where `entire` is not installed.

> **Entire version note (verified against v0.6.3, source `17720a12`):**
> `entire agent add etch` fails with "Unknown agent" — that code path never
> runs external-agent discovery on v0.6.3. Use `entire enable --agent etch`,
> which does. Today the installer wires **Claude Code** hooks
> (`.claude/settings.json`, committable repo state); other runtimes hand-feed
> the same stdin contract (see [docs/HOOK_CONTRACT.md](./docs/HOOK_CONTRACT.md)).
> Run `make smoke` to verify the full install + capture path end-to-end
> against your `entire`.

Configure git so per-session refs sync with your remote:

```bash
entire-agent-etch setup-refspec  # adds the refs/etch/sessions/* fetch + push refspecs
```

`setup-refspec` requires a remote with a URL. It picks `origin` when present,
falls back to the only remote if exactly one exists, and otherwise asks you to
choose with `--remote <name>`. To sync etch refs with more than one remote,
rerun it once per remote:

```bash
entire-agent-etch setup-refspec --remote primary
entire-agent-etch setup-refspec --remote backup
```

> **Only point refspecs at private remotes.** Session records can contain
> prompt text, so a public remote must never receive a `refs/etch/sessions/*`
> refspec — capture stays local-only there.

**What this changes about `git push`:** git replaces its implicit default-push
behavior the moment any `remote.<name>.push` refspec exists, so `setup-refspec`
writes two entries — the etch refspec *and* `HEAD` — to keep a bare `git push`
pushing your current branch alongside the session refs. The result behaves like
`push.default=current`: a bare `git push` pushes the current branch to a
same-name branch on the remote (creating it if it has no upstream yet) plus any
etch session refs. Explicit pushes (`git push origin main`) are unaffected. If
you already have your own push refspecs configured, `setup-refspec` adds only
the etch refspec and leaves your push behavior untouched. Rerunning
`setup-refspec` is safe and upgrades configs written by older versions (adds
the missing `HEAD` entry and the `+` on the fetch refspec).

Equivalent manual config:

```bash
git config --add remote.origin.fetch '+refs/etch/sessions/*:refs/etch/sessions/*'
git config --add remote.origin.push  'refs/etch/sessions/*:refs/etch/sessions/*'
git config --add remote.origin.push  'HEAD'  # keeps bare 'git push' pushing your branch; omit if you already configure remote.origin.push yourself
```

### Second machine / fresh clone

A fresh clone has **zero** etch refs — git does not fetch custom ref namespaces
by default. After cloning, run `setup-refspec` in the clone and fetch:

```bash
git clone <remote-url> repo && cd repo
entire-agent-etch setup-refspec
git fetch origin
git for-each-ref refs/etch/sessions/   # session refs are now local
```

From then on every ordinary `git fetch` keeps session refs in sync.

### Optional: `.etch/settings.json`

Drop this in the repo root to tune capture behavior. All fields are optional;
defaults shown:

```json
{
  "raw_machine_identity": false,
  "local_only_fields": [],
  "archive_threshold_days": 90,
  "redaction_patterns": [],
  "recovery_timeout_hours": 4
}
```

- **`raw_machine_identity`** — when `false` (default), the hostname is stored as a
  salted hash. Set `true` to opt into recording the raw hostname.
- **`hostname_salt`** — random per-repo salt mixed into the hostname hash
  (`SHA-256(salt + hostname)`). Auto-generated on first session; you don't set
  it yourself. **Commit `.etch/settings.json`** so all clones of the repo share
  the salt — cross-machine correlation within the repo depends on it. Until the
  file is committed and pulled, each clone salts independently. Per-repo salting
  means hostname hashes don't correlate across repos.
- **`local_only_fields`** — dot-paths into `session.json` (e.g. `"prompt.text"`,
  `"files_touched"`, `"orchestration.extra.customer"`) that must never leave this
  machine. See [Privacy & security](#privacy--security) for the exact semantics.
- **`archive_threshold_days`** — age after which sessions are eligible for archival
  (consumed by the `archive` subcommand; override per-run with `--threshold-days`).
- **`redaction_patterns`** — extra regexes applied on top of the built-in
  best-effort secret scanning of prompts.
- **`recovery_timeout_hours`** — how long an orphaned `.wip.jsonl` buffer must be
  idle (file mtime) before crash recovery finalizes it as an `incomplete`
  session. The timeout governs only when no agent process was identified at
  session start: a session whose recorded agent process is verifiably alive is
  never recovered (it can still end normally), and one whose process is
  verifiably dead is recovered without waiting out the timeout.

## Usage

After the Configure step above, you do **nothing** — every Claude Code session
in the repository is captured invisibly through the installed hooks and
written as an immutable ref the moment it ends (`SessionEnd`). Sessions that
die without a SessionEnd are finalized by crash recovery on the next session
start. The hook-event contract (both supported payload dialects, per-event
examples, warning behavior) is documented in
[docs/HOOK_CONTRACT.md](./docs/HOOK_CONTRACT.md).

Inspect captured sessions with plain git:

```bash
# List every captured session
git for-each-ref refs/etch/sessions/

# Read one session record
git show refs/etch/sessions/<ULID>:session.json | jq

# Read its Agent Trace (Cursor/Cognition-compatible) record
git show refs/etch/sessions/<ULID>:agent-trace.json | jq
```

### Query sessions

`query` prints a table of captured sessions (newest first by default), with
filters that compose:

```bash
entire-agent-etch query --repo .                 # every captured session
entire-agent-etch query --runtime claude-code    # filter by agent runtime
entire-agent-etch query --ticket ETCH-9          # filter by orchestration ticket
entire-agent-etch query --status incomplete      # complete | incomplete
entire-agent-etch query --since 2026-06-01T00:00:00Z --until 2026-06-07T00:00:00Z
entire-agent-etch query --branch main --has-files 'src/**'
entire-agent-etch query --json                   # full records as a JSON array
entire-agent-etch query --count                  # just the matching count
```

Also: `--exit-reason`, `--run-id`, `--sort started_at|duration|session_id`,
`--reverse`, and `--no-index` (force the ref-walk path, ignoring any index).

### Index

For repos with many sessions, a materialized index accelerates `query`
(which uses it automatically when present and falls back to walking refs):

```bash
entire-agent-etch index build    # build the index from scratch
entire-agent-etch index update   # incrementally add new sessions
entire-agent-etch index show     # path, session count, size, built_at
entire-agent-etch index drop     # remove the index (query falls back to refs)
```

### Archive old sessions

`archive` moves sessions older than `archive_threshold_days` (default 90, see
the `.etch/settings.json` section above) out of `refs/etch/sessions/` into
per-quarter archive refs:

```bash
entire-agent-etch archive --dry-run        # print what would be archived
entire-agent-etch archive                  # actually archive
entire-agent-etch archive --threshold-days 30 --quarter 2026-Q1
entire-agent-etch restore-archive <ULID>   # bring one session back
```

Run `entire-agent-etch help` (or bare `entire-agent-etch`, `-h`, `--help`) for
the full subcommand listing with one-line descriptions.

## Architecture

Etch is pure git plumbing. Each session is buffered to a `.wip.jsonl` file as hook
events arrive, then finalized on `session_end` into an **orphan commit** holding a
`session.json` blob (and an `agent-trace.json` blob), pointed at by a per-session
ref `refs/etch/sessions/<ULID>`. Per-session refs mean zero write contention and
immutability after creation — structure emerges at query time from shared
identifiers, not from any hierarchy. If a session crashes, its buffer is recovered
and finalized on the next invocation. See [BUILDPLAN.md](./BUILDPLAN.md) for the
full design and ticket breakdown.

## Session record schema

Records use the `etch.session.v1` schema. The complete field reference and
scenario variants live in [OUTPUT_SPEC.md](./OUTPUT_SPEC.md).

## Privacy & security

- **Hostname is stored as a salted hash by default** — `SHA-256(salt + hostname)`
  with a random per-repo salt (`hostname_salt` in `.etch/settings.json`), so
  hashes don't correlate across repos and low-entropy hostnames aren't directly
  rainbow-tableable. Opt into the raw hostname via `raw_machine_identity: true`.
- **Best-effort secret scanning** runs over captured prompts (regex-based, not
  exhaustive); extend it with `redaction_patterns`.
- **`local_only_fields`** keeps selected fields off the wire entirely. A session
  in which a configured path strips something is committed as **two refs**: the
  canonical `refs/etch/sessions/<ULID>` holds the **stripped** record (this is
  what every refspec, bare `git push`, and fetch sees), and
  `refs/etch/local/<ULID>` holds the full-fidelity record and is never named by
  any etch-configured refspec. Sessions where no configured path matches commit
  a single, untouched sessions ref as usual.
  The projection happens at commit time, so safety does not depend on refspec
  config: even a stale or hand-written `refs/etch/sessions/*` push refspec
  cannot leak a stripped field — the pushable namespace never contained it.

  Semantics worth knowing:
  - Fields are dot-paths matching `session.json` keys; arrays fan out
    (`files_touched.path` strips every entry's path; `files_touched` strips the
    whole array). Paths that match nothing are no-ops — typos are not detected,
    so copy paths from [OUTPUT_SPEC.md](./OUTPUT_SPEC.md).
  - Stripped strings carry a `[LOCAL_ONLY:<path>]` marker; everything stripped
    is listed in the record's `local_only_stripped` manifest. The trace blob
    and the ref's commit message are also built from the stripped record.
  - Identity fields (`schema_version`, `session_id`, `status`, `agent.runtime`)
    are never stripped, even if configured.
  - **Local tooling reads the stripped record too**: `etch query`, the index,
    and `git show refs/etch/sessions/…` on the authoring machine all see the
    pushable projection. Full fidelity lives only at
    `git show refs/etch/local/<ULID>:session.json`.
  - Applies to sessions recorded after the setting is set — existing refs are
    immutable and unchanged.
  - Don't hand-configure a wildcard `refs/etch/*` push refspec: it would push
    the `local/` namespace. `setup-refspec` never writes one.

## Development

```bash
make build          # compile ./bin/entire-agent-etch
make test           # unit tests (go test ./...)
make test-density   # 20-concurrent-session stress test
make smoke          # end-to-end smoke test against the real Entire CLI
make help           # list all targets
```

Project layout:

```
cmd/entire-agent-etch/   # binary entrypoint + subcommand dispatch
internal/                 # capture, hooks, refs, recovery, redact, schema, config, ...
test/density/             # concurrency / density stress tests (build tag: density)
scripts/                  # smoke.sh and friends
```

Etch has zero runtime dependencies and every test runs on the filesystem against a
temp git repo — see the testing philosophy in [CLAUDE.md](./CLAUDE.md).

## License

TBD.
