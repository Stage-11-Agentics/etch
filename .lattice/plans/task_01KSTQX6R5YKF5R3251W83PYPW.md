# ETCH-15 — Full Cairn → Etch Rename — Plan

## Goal
Eliminate every current-project use of the name "Cairn" across code, docs, settings, refs,
schema, env vars, the binary, and the lattice-orchestrator skill. Clean cutover — no backward
compat. Historical `lessons-learned.md` entries keep their period-accurate naming.

## Survey findings
- 653 case-insensitive `cairn` hits across 49 tracked files (excluding `.git`, `.lattice`,
  `forge-solution-review-pack-*`, and the prebuilt `entire-agent-cairn-poc` binary).
- Casing is a clean three-way set in code: `cairn` (lowercase), `Cairn` (Pascal, only
  `clearCairn`/`allCairnVars`), `CAIRN` (env-var prefix). No mixed/odd casings.
- `CMUX_*` env vars in `environ.go` are **legacy c11**, NOT cairn — must stay untouched.
- Go module path is already `forgejo.stage11.ai/s11/etch` — no module rename.
- Schema constants: `cairn.session.v1` (schema/session.go), `cairn.index.v1` (index/index.go).
- Refs: `refs/cairn/sessions/`, `refs/cairn/archive/`.
- Settings: `.cairn/settings.json`, `.cairn/sessions/*.wip.jsonl`, `.cairn/index/sessions.idx`.
- Commit author identity: `cairn <cairn@localhost>` in refs/writer.go.
- Binary: `cmd/entire-agent-cairn/` dir + `entire-agent-cairn-poc` prebuilt + Makefile targets.
- Docs: CAIRN.md, CAIRN_PLAN.md (rename files), plus SPEC/BUILDPLAN/OUTPUT_SPEC/README/CLAUDE/
  PHASE0_RESULTS/RESEARCH/SOLUTION/PROBLEM/ENTIRE_EVAL/NOTES + scripts/README.md.
- Skill: c11 repo `skills/lattice-orchestrator/SKILL.md` + `references/orchestrator.md` export
  CAIRN_* — flip to ETCH_*. Installed copy is a symlink; edit source only.

## Strategy — case-sensitive three-way replace
Apply across code files (cmd/, internal/, test/, Makefile, scripts/, .gitignore):
- `cairn` → `etch`
- `Cairn` → `Etch`
- `CAIRN` → `ETCH`

This correctly maps: `refs/cairn/` → `refs/etch/`, `cairn.session.v1` → `etch.session.v1`,
`CAIRN_TICKET_ID` → `ETCH_TICKET_ID`, `cairnDir`→`etchDir`, `clearCairn`→`clearEtch`,
`allCairnVars`→`allEtchVars`, `"name":"cairn"`→`"name":"etch"`, author identity, README prose
(`cairn query` → `etch query`).

## Commit sequence (logical)
1. **Binary + cmd**: rename `cmd/entire-agent-cairn/` → `cmd/entire-agent-etch/` (git mv),
   rename `entire-agent-cairn-poc` → `entire-agent-etch-poc`, update Makefile targets, scripts.
2. **Env vars**: environ.go + all tests (CAIRN_* → ETCH_*).
3. **Schema**: session.go + index.go version constants + asserting tests.
4. **Refs + settings + author identity**: writer.go, archive, query, index walkers, config,
   buffer, setup-refspec + tests.
5. **Docs**: rename CAIRN.md→ETCH.md, CAIRN_PLAN.md→ETCH_PLAN.md (git mv), three-way replace in
   all active docs; CLAUDE.md reference-artifacts section updated to new filenames; preserve
   historical lessons-learned entries (pre-2026-05-29).
6. **Skill** (separate commit in c11 repo): CAIRN_* → ETCH_* in SKILL.md + orchestrator.md.

## New tests (insurance against partial rename)
- `info` returns `"name":"etch"`.
- Schema version asserted `etch.session.v1`.
- Captured session writes a ref under `refs/etch/sessions/`.
- `ETCH_TICKET_ID` captured into `orchestration.ticket_id`; `CAIRN_TICKET_ID` ignored.
(Most are covered by updating existing assertions; add explicit legacy-ignored test.)

## Historical preservation
`lessons-learned.md`: entries dated before 2026-05-29 keep "Cairn" in prose/headers. Do NOT
three-way-replace this file wholesale — hand-edit only if a new entry is added (uses "Etch").
`forge-solution-review-pack-*/` contents are immutable external artifacts — untouched.

## Verification (mandatory, loopy)
1. `go build ./...`  2. `go test ./...`  3. `make smoke`
4. grep `cairn` across cmd/internal/test/Makefile/scripts + active docs → zero hits.
5. Confirm old files/dirs gone (CAIRN.md, CAIRN_PLAN.md, cmd/entire-agent-cairn, poc binary).
Iterate until all green.

## Wide sweep (environment-wide, beyond the repo)
A rename confined to the Etch repo leaves stale `cairn` references in the surrounding
environment. Sweep these and fix what is in-scope; surface (don't silently change) anything
that is data, external, or another ticket's responsibility:

- **Stage11 monorepo** — `grep -rni 'cairn'` across `code/`, `deployments/`, `company/`,
  `tasks/`, etc. (exclude the Etch repo, `.git`, and `forge-solution-review-pack-*`). Catch
  other projects that hardcode `entire-agent-cairn`, `CAIRN_*`, `refs/cairn`, or `.cairn`.
- **`~/.claude`** — settings.json / settings.local.json, skills, hooks, and references for
  `cairn`, `CAIRN_*`, or `entire-agent-cairn` (e.g. a hook installing the plugin by old name).
- **lattice-orchestrator skill (c11 repo)** — §10 premise check: d44c948 (ETCH-12's CAIRN_*
  export block) is NOT an ancestor of c11 HEAD — the live skill exports nothing. There is no
  CAIRN_* to flip. FLAG this gap (orchestration capture currently exports nothing); re-landing
  the export block as ETCH_* is ETCH-12's scope, not a rename. Do not silently re-introduce it.
- **Installed artifacts** — `~/.local/bin/entire-agent-cairn`, `/usr/local/bin/entire-agent-cairn`,
  and any other PATH entry. Old-named installed binary should be re-installed/removed.
- **Global & repo git config** — refspecs referencing `refs/cairn/*` (e.g. fetch/push lines
  added by an old `setup-refspec`). Update or note for re-run of the renamed command.
- **Home/state dirs** — `~/.cairn/` (active markers), and any `.cairn/` dirs in dogfooded repos.
- **Live git data** — existing `refs/cairn/sessions/*` refs created by prior dogfooding are
  DATA, not code. Clean cutover means new sessions go to `refs/etch/`; old `refs/cairn` refs
  are historical. SURFACE the count; do not auto-delete or auto-migrate without sign-off.

Report the sweep findings in the completion comment: what was fixed, what was flagged, counts.
