# ETCH-14 Plan — Fix SPEC #7 and #11 gaps

## Gap 1 — SPEC #7: Raw hostname opt-in not wired

`CaptureMachine()` always emits `sha256:<hash>` and never populates `hostname_raw`
even when `.cairn/settings.json` has `raw_machine_identity: true`.

**Fix:**
- Change signature to `CaptureMachine(settings *config.Settings) MachineInfo`.
- `HostnameHash` stays populated unconditionally (join key downstream).
- When `settings != nil && settings.RawMachineIdentity`, set `HostnameRaw = &hostname`.
- nil settings → defaults → hashed-only (current behavior).
- Capture hostname once via `os.Hostname()` and reuse for both hash + raw.

**Caller:** `internal/hooks/session_start.go` — load `config.Load(repoRoot)` once,
pass `&settings` to `CaptureMachine`. Load is non-fatal: on error use `config.Defaults()`.

## Gap 2 — SPEC #11: pane_lineage never populated

`CaptureC11()` populates workspace/surface/tab_title but never `PaneLineage`.

**Investigation result:** c11 exposes **no orchestration-ancestry API**. The
`c11 tree` structure is spatial (window → workspace → pane → surface). The most
meaningful "lineage" c11 surfaces is the **ordered stack of surfaces sharing the
current pane** — e.g. pane:59 holds
`['…/Etch', 'ETCH-9 Delegator', 'ETCH-12 Delegator', 'ETCH-14 Delegator']`,
which is exactly the sibling-agent stack. We derive `pane_lineage` from that:
titles ordered by `index_in_pane`, up to and including the current surface.

**Fix:**
- Add a `c11Cmd` indirection var (`func(args ...string) ([]byte, error)`) so tests
  can fake CLI output. Default runs the real `c11` binary.
- `c11PaneLineage(surfaceID) []string`: run `c11 tree --all --json`, find the
  surface whose `ref == surfaceID`, return its pane's surface titles ordered by
  `index_in_pane` up to and including that surface.
- In `CaptureC11`: try `c11PaneLineage`; if empty/err, fall back to `[tab_title]`
  (only when tab_title is non-empty). Document semantics in a comment.
- File a lessons-learned note: c11 has no true ancestry API; lineage = pane stack.

## Tests (all in internal/capture)

Gap 1 (`machine_test.go`):
- `TestCaptureMachine_DefaultHashesOnly` — RawMachineIdentity:false → hash set, raw nil.
- `TestCaptureMachine_RawOptIn` — RawMachineIdentity:true → hash set, *raw == os.Hostname().
- `TestCaptureMachine_NilSettings` — nil → hashed-only.

Gap 1 integration (`internal/hooks`):
- `TestSessionStart_WithRawHostnameConfig` — write settings.json raw=true, run
  session_start, finalize/read ref → machine.hostname_raw non-null == hostname.

Gap 2 (`environ_test.go`):
- `TestCaptureC11_NoLineage` — env unset → nil C11 block.
- `TestCaptureC11_PaneLineagePresent` — set env + stub c11Cmd tree JSON → multi-element lineage.
- `TestCaptureC11_SingleElementFallback` — stub c11Cmd to fail/empty → lineage == [tab_title].

## Verify
`go test ./...` green; build clean; inspect a live session.json for both fields.
