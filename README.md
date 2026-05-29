# Etch

Etch captures flat metadata for **every AI agent session in a repository** and
stores it as immutable git refs (`refs/cairn/sessions/<ULID>`). It runs invisibly
on [Entire CLI](https://github.com/entireio/cli)'s hook substrate — every session
becomes a permanent, queryable record with zero contention, designed for 60–80+
concurrent agents across multiple machines.

The capture binary is named `entire-agent-cairn` (Entire discovers external agents
by the `entire-agent-<name>` naming convention; this registers Etch as the `cairn`
agent). Environment variables use the `CAIRN_*` namespace.

## Status

- **Production-ready core** — session capture, immutable per-session refs, crash
  recovery via `.wip.jsonl` buffers, [Agent Trace](./OUTPUT_SPEC.md) emission, and
  20-concurrent-session density validation are all in place and tested.
- **In progress** — `cairn query` (ETCH-9), `cairn index` (ETCH-10), and
  `cairn archive` (ETCH-11) are ticket-tracked. Phase 2/3 work is ongoing.

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
go install forgejo.stage11.ai/s11/etch/cmd/entire-agent-cairn@latest
```

> `go install` depends on the Forgejo host being reachable by your Go toolchain.
> If it isn't (private host, no module proxy), use the clone + `make install` path
> above, which only needs git and the Go compiler.

**Verify the install:**

```bash
entire-agent-cairn info
# → {"name":"cairn","version":"0.01.001","hooks":true,"transcript_analyzer":true,...}
```

The binary must be on your `$PATH` — that is how Entire discovers it as an agent.

## Configure

Etch's binary follows Entire's `entire-agent-<name>` naming convention, which
registers it as the **`cairn` agent**. Confirm the install step put it on your
`$PATH`, then enable Entire in the repository you want to capture:

```bash
cd your-repo
entire enable                     # interactive; or: entire enable --agent claude-code --no-github
command -v entire-agent-cairn     # confirm the cairn binary is discoverable on $PATH
```

> **Entire version note (tested against v0.6.3):** the built-in `entire agent add`
> / `entire enable --agent` roster is a fixed list (claude-code, codex,
> copilot-cli, cursor, factoryai-droid, gemini, opencode, pi, vogon) — `entire
> agent add cairn` returns "Unknown agent", so external-agent auto-dispatch from a
> live session depends on your Entire version's external-agent support. The
> capture engine itself is version-independent: Etch consumes the exact hook
> contract Entire dispatches (`session_start`, `user_prompt_submit`,
> `pre_tool_use`, `post_tool_use`, `session_end`, `stop` on stdin). Run
> `make smoke` to verify the full capture path end-to-end against your `entire`.

Configure git so per-session refs sync with your remote:

```bash
entire-agent-cairn setup-refspec  # adds the refs/cairn/sessions/* fetch + push refspec
```

Equivalent manual config:

```bash
git config --add remote.origin.fetch '+refs/cairn/sessions/*:refs/cairn/sessions/*'
git config --add remote.origin.push  'refs/cairn/sessions/*:refs/cairn/sessions/*'
```

### Optional: `.cairn/settings.json`

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
  (consumed by `cairn archive`, ETCH-11).
- **`redaction_patterns`** — extra regexes applied on top of the built-in
  best-effort secret scanning of prompts.
- **`recovery_timeout_hours`** — how long an orphaned `.wip.jsonl` buffer must be
  idle before crash recovery finalizes it as an `incomplete` session.

## Usage

As an operator you do **nothing** — once `cairn` is registered, Etch captures every
session invisibly through Entire's hooks. Sessions are written as immutable refs the
moment they end.

Inspect captured sessions with plain git:

```bash
# List every captured session
git for-each-ref refs/cairn/sessions/

# Read one session record
git show refs/cairn/sessions/<ULID>:session.json | jq

# Read its Agent Trace (Cursor/Cognition-compatible) record
git show refs/cairn/sessions/<ULID>:agent-trace.json | jq
```

Richer querying is coming: **`cairn query`** (ETCH-9) for structured search across
sessions and **`cairn archive`** (ETCH-11) for aging old sessions out of the active
ref namespace.

## Architecture

Etch is pure git plumbing. Each session is buffered to a `.wip.jsonl` file as hook
events arrive, then finalized on `session_end` into an **orphan commit** holding a
`session.json` blob (and an `agent-trace.json` blob), pointed at by a per-session
ref `refs/cairn/sessions/<ULID>`. Per-session refs mean zero write contention and
immutability after creation — structure emerges at query time from shared
identifiers, not from any hierarchy. If a session crashes, its buffer is recovered
and finalized on the next invocation. See [BUILDPLAN.md](./BUILDPLAN.md) for the
full design and ticket breakdown.

## Session record schema

Records use the `cairn.session.v1` schema. The complete field reference and
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
make build          # compile ./bin/entire-agent-cairn
make test           # unit tests (go test ./...)
make test-density   # 20-concurrent-session stress test
make smoke          # end-to-end smoke test against the real Entire CLI
make help           # list all targets
```

Project layout:

```
cmd/entire-agent-cairn/   # binary entrypoint + subcommand dispatch
internal/                 # capture, hooks, refs, recovery, redact, schema, config, ...
test/density/             # concurrency / density stress tests (build tag: density)
scripts/                  # smoke.sh and friends
```

Etch has zero runtime dependencies and every test runs on the filesystem against a
temp git repo — see the testing philosophy in [CLAUDE.md](./CLAUDE.md).

## License

TBD.
