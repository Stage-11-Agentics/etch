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
- **In progress** — `etch query` (ETCH-9), `etch index` (ETCH-10), and
  `etch archive` (ETCH-11) are ticket-tracked. Phase 2/3 work is ongoing.

## Requirements

- **Go 1.22+** (built and tested on 1.26)
- **Git 2.30+**
- **[Entire CLI](https://github.com/entireio/cli)** — the hook substrate Etch plugs into

## Install

**Quick (from a clone):**

```bash
git clone forgejo.stage11.ai:s11/etch && cd etch
make install                      # installs to /usr/local/bin (may need sudo)
# or pick your own prefix:
PREFIX=$HOME/.local make install  # installs to ~/.local/bin
```

**From source with `go install`:**

```bash
go install forgejo.stage11.ai/s11/etch/cmd/entire-agent-etch@latest
```

> `go install` depends on the Forgejo host being reachable by your Go toolchain.
> If it isn't (private host, no module proxy), use the clone + `make install` path
> above, which only needs git and the Go compiler.

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
choose with `--remote <name>`. To sync etch refs with more than one remote
(e.g. Forgejo *and* GitHub), rerun it once per remote:

```bash
entire-agent-etch setup-refspec --remote forgejo
entire-agent-etch setup-refspec --remote github
```

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
- **`local_only_fields`** — field names to strip before a ref is pushed to a remote
  (kept in the local ref only).
- **`archive_threshold_days`** — age after which sessions are eligible for archival
  (consumed by `etch archive`, ETCH-11).
- **`redaction_patterns`** — extra regexes applied on top of the built-in
  best-effort secret scanning of prompts.
- **`recovery_timeout_hours`** — how long an orphaned `.wip.jsonl` buffer must be
  idle before crash recovery finalizes it as an `incomplete` session.

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

Richer querying is coming: **`etch query`** (ETCH-9) for structured search across
sessions and **`etch archive`** (ETCH-11) for aging old sessions out of the active
ref namespace.

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

- **Hostname is hashed by default** — opt into the raw hostname via
  `raw_machine_identity: true`.
- **Best-effort secret scanning** runs over captured prompts (regex-based, not
  exhaustive); extend it with `redaction_patterns`.
- **`local_only_fields`** lets you keep selected fields out of any ref that gets
  pushed to a remote.

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
