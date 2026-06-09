# Validation Plan — Etch Backlog Completion

Prepared: 2026-06-03
Source spec: [SPEC.md](../../SPEC.md)
Current backlog source: `lattice list --status backlog --json`
Scope: ETCH-16 through ETCH-39 only.

## Baseline Commands

Run before and after the orchestrated backlog work:

```bash
go test ./...
make build
make smoke
make test-density
go test -run 'Test.*(Redact|Recovery|Refspec|Root|Token|Secret|Crash)' ./internal/... ./cmd/...
```

Pass condition: all commands exit 0. `make smoke` must continue to validate real `entire` availability plus manual hook capture.

## Cluster Validation Gates

| Gate | Tickets | Verification | Pass condition |
|---|---|---|---|
| Secret redaction | ETCH-25, ETCH-26, ETCH-27, ETCH-28, ETCH-29, ETCH-39 | Add/update tests in `internal/redact/redact_test.go` and at least one end-to-end prompt-capture test in `internal/hooks` or `scripts/smoke.sh` equivalent. Exercise `sk-proj-`, `sk-svcacct-`, bare AWS secret access key, JWT, full private-key block, documentation placeholder `sk-ant-EXAMPLE`, and password/passwd/pwd/token/client_secret keys. | Real secrets are redacted with stable marker names; placeholder/documentation examples remain unredacted where intended; no private key body remains in `session.json`. |
| Refspec and remote UX | ETCH-16, ETCH-18, ETCH-22, ETCH-24, ETCH-38 | Add/update tests around `internal/commands/setup_refspec.go` and any CLI docs. Validate no-remote repo, non-`origin` remote, normal branch push behavior, fetch refspec consistency, and fresh clone setup docs. | `setup-refspec` does not silently create unusable phantom remotes, does not break normal branch pushes, supports or clearly reports remote selection, and README matches implementation. |
| Capture/recovery correctness | ETCH-30, ETCH-33, ETCH-34, ETCH-35, ETCH-36 | Add/update tests in `internal/hooks`, `internal/recovery`, and `internal/capture`. Reproduce crash with no session_end, nested subdir invocation, non-git repo invocation, recovered tool counts, and session_start latency shape. | Crash recovery works on next invocation when the original process is gone; recovered tool calls match complete-session counting; `.etch` and settings resolve to git root; non-git failures are visible; recovery scan is bounded or throttled. |
| Token/session identity | ETCH-23, ETCH-32 | Add schema/capture tests for upstream runtime session ID preservation and token extraction from supported raw event data. | Etch ULID remains the canonical ref/session ID, upstream agent session ID is preserved in an explicit field or documented as intentionally unavailable, and token fields populate when raw usage data is present. |
| CLI/docs discoverability | ETCH-17, ETCH-19, ETCH-20, ETCH-21, ETCH-37 | Validate `entire-agent-etch`, `help`, `--help`, and relevant subcommand help. Review README for auto-capture status, hook payload examples, query/index/archive usage, and hostname hashing claim. | New users can discover commands from the binary, docs accurately describe the tested Entire path, hook JSON examples use correct field names, shipped features are no longer described as missing, and hostname privacy claims match implementation. |
| Privacy contract | ETCH-31 | Product/design review before code. Decide whether `local_only_fields` is implemented for real or removed/softened from docs/settings. Add tests only after decision. | No false privacy guarantee remains. Either configured fields are actually excluded from pushable refs by design, or docs clearly state the feature is not implemented. |

## Cluster Commands

### Ref Transport + CLI Onboarding

Tickets: ETCH-16, ETCH-18, ETCH-21, ETCH-22, ETCH-24, ETCH-38

```bash
go test -v ./cmd/entire-agent-etch ./internal/hooks -run 'Test(NoSubcommand|UnknownSubcommand|E2ESetupRefspec)'
make smoke
```

Manual checks:

- `entire-agent-etch`, `help`, `--help`, and `-h` list all real subcommands.
- `setup-refspec` does not break normal branch pushes.
- Fresh repo with no usable remote fails or warns clearly, not "configured".
- Non-`origin` remote path is handled or clearly rejected.
- Clone/fetch instructions are documented and work: session refs fetch on a second clone after setup.

### Entire Hook Contract + Docs

Tickets: ETCH-17, ETCH-19, ETCH-20

```bash
go test -v ./cmd/entire-agent-etch ./internal/hooks ./internal/query ./internal/index ./internal/archive
make smoke
```

Manual checks:

- README no longer promises invisible auto-capture on Entire `v0.6.3` unless there is a proven registration path.
- Hook stdin examples document working fields: `raw_data.model`, `user_prompt`, `tool_name`, `tool_use_id`, `tool_input`, shared `session_id`.
- Malformed or unknown hook fields either warn visibly or docs make the accepted contract explicit.
- README reflects shipped `query`, `index`, and `archive` commands with examples.

### Redaction + Privacy

Tickets: ETCH-25, ETCH-26, ETCH-27, ETCH-28, ETCH-29, ETCH-31, ETCH-37, ETCH-39

```bash
go test -v ./internal/redact ./internal/hooks ./internal/config -run 'TestScanSecrets|TestRedact|TestGetHostname|TestE2EFullLifecycle|TestLoad'
```

Manual checks:

- Inspect actual committed `session.json`, not only in-memory structs.
- Positive tests cover real secret shapes.
- Negative tests cover placeholders and documentation examples.
- `local_only_fields` is either implemented and verified against pushed/read-back refs, or removed/softened from the privacy promise.

### Crash Recovery + Repo Root Handling

Tickets: ETCH-30, ETCH-33, ETCH-34, ETCH-35

```bash
go test -v ./internal/recovery ./internal/hooks ./internal/capture -run 'Test(E2ECrashRecovery|Recover|ScanOrphaned|FullSessionLifecycle|NoMapping|CaptureGit)'
go test -tags density -v ./test/density/ -run 'TestDensityCrashRecovery|TestDensity20Concurrent'
```

Manual checks:

- Drive actual hook calls and omit `session_end`; do not rely only on fabricated old WIP files.
- Verify recovered records have `status: incomplete` and `exit_reason: crash`.
- Verify `.etch/` and `.etch/settings.json` resolve from git root even when hooks run from subdirectories.
- Verify non-git repo behavior is visible and does not return `{"ok":true}` while dropping data.

### Tokens, Latency, Capture Completeness

Tickets: ETCH-23, ETCH-32, ETCH-36

```bash
go test -v ./internal/capture ./internal/hooks ./internal/commands ./internal/index -run 'Test(Finalize|E2ECapabilitySubcommands|CaptureOrchestration|Index)'
```

Manual checks:

- Preserve upstream agent/Entire `session_id` somewhere if the schema decision chooses that route.
- Populate token fields when token data is present in hook raw data or transcript-derived source.
- Confirm `calculate-tokens` and session finalization agree.
- Benchmark `session_start` across at least 100 temp-repo invocations. **Accepted threshold (recorded 2026-06-09, see SPEC AC #13):** p99 ≤ 200 ms with a **flat** curve as the wip population grows; the 50 ms median target is superseded — measured floor is ~170 ms median (170.9 / p90 175.8 / p99 178.3 over 0→99 wip growth), dominated by process-spawn + git-plumbing, not scan cost. The load-bearing checks are: (a) per-event hooks ≤ 50 ms, and (b) no per-start growth (proving the stat-bounded scan removed the old O(N) behavior — pre-fix was 186.9 median / 366.8 p99 *with* growth).
- Benchmark with empty, 100, and 1000 WIP files.

### Cross-Machine Read Path + Archive Integrity

Tickets: ETCH-19, ETCH-24, plus SPEC AC #5/#12

```bash
go test -v ./internal/query ./internal/index ./internal/archive
```

Manual temp-repo flow:

```bash
git for-each-ref refs/etch/sessions/
entire-agent-etch query --repo .
entire-agent-etch index build --repo .
entire-agent-etch index show --repo .
entire-agent-etch archive --repo . --dry-run
```

Pass condition: session refs round-trip through a bare remote and second clone; `session.json` and `agent-trace.json` remain readable; archive destructive behavior is validated only in temp repos and with `--dry-run` first.

## Phase 4 Result Validator Checklist

1. Confirm all 24 backlog tickets are `pr_open` or `done`, or explicitly cancelled/duplicated with rationale.
2. Pull each PR diff and map it back to the ticket table in `run-state.md`.
3. Run the baseline commands.
4. Run or inspect cluster-specific tests added by delegators.
5. Verify README and OUTPUT_SPEC references use `etch`, `ETCH_*`, and `refs/etch/*`, not stale Cairn names.
6. Inspect `git status --short --branch`; only intentional artifacts should remain dirty.
7. Produce `.lattice/orchestration/validation-report.md` with pass/fail/partial per cluster and unresolved risk.

## Known Risks to Watch

- Several Wave A tickets all touch `internal/redact/secrets.go`; coordinate branches or sequence merges carefully.
- Refspec tickets overlap; avoid implementing incompatible fixes in parallel.
- ETCH-31 is not a pure coding bug unless the product direction is explicit.
- Existing worktree contains many Lattice artifacts from prior runs; do not revert them casually.
