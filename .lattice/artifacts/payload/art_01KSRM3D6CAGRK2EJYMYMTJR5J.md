## Self-review — ETCH-12 (fast-track, PASS)

**Scope split across two repos, both done:**

### c11 repo (load-bearing) — commit `d44c94889`, branch `feat/etch-12-cairn-skill-export`, pushed
- New HARD RULE in `references/orchestrator.md` sub-agent boilerplate section: "### Export CAIRN_* env vars before launching any sub-agent" — full contract + "absent → manual" note + pointer to `OUTPUT_SPEC.md §3`.
- CAIRN_* export block added to **all three** boot templates (fast-track, inline-full, sub-agent-full) immediately after `export LATTICE_ROOT=…`. Heredoc `$` correctly escaped as `\$` for runtime evaluation of `CAIRN_WORKFLOW_VERSION`.
- Atomic-cwd launch line now also exports per-sub-agent `CAIRN_AGENT_ROLE` + `CAIRN_PARENT_SESSION_ID`; the "three load-bearing pieces" note updated to match.
- `SKILL.md` reference index notes the CAIRN_* export.
- Committed onto a dedicated branch (NOT the unrelated active `feat/create-workspace-dialog-tweaks` branch); active branch restored to its original tip with its `.lattice` working changes intact.

### Etch repo — branch `feat/etch-12-skill-env`, commit `36b51d7`
- `docs/lattice-skill-integration.md`: contract table + c11 SHA + the not-a-symlink caveat.
- `internal/capture/orchestration_lattice_skill_test.go`: four mandated tests, all pass.
- `.gitignore`: ignore the local `.cairn/` WIP buffer dir.

**Validation:**
- `go vet ./...` clean; `go test ./...` all green (every package).
- Four new tests pass: `TestCaptureOrchestration_LatticeSkillExports`, `_ExtraJSON`, `_AllAbsent`, `_OnlyOrchestratorType`.
- **Smoke test:** exported the skill's CAIRN block, piped a `session_start` event to a fresh build, confirmed the `.wip.jsonl` `.data.orchestration` block:
  `{"type":"lattice-orchestrator","dispatch_method":"c11_delegator","ticket_id":"ETCH-12-smoke","run_id":"01TESTRUN","role":"delegator","workflow_version":null,"extra":{}}`

**Note:** `CAIRN_PARENT_SESSION_ID` is captured at the hook layer onto `Session.parent_session_id`, not inside the `Orchestration` struct — documented in the test and marker doc. No `Orchestration` schema change needed.

**Verdict: PASS.** Ready for PR.