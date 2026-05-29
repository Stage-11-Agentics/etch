# Entire.io: Deep Evaluation as Etch Substrate

**Date:** 2026-05-26
**Recommendation:** **Use + active upstream contribution**

Install Entire CLI as the capture substrate. Use the external agent plugin protocol to observe session lifecycle. Contribute a metadata-branch CAS fix upstream (the pattern exists in their shadow-branch code — it's a copy-paste PR). Build Etch's workflow modeling, outcome binding, and multi-agent attribution as a separate layer on top, writing to a Etch-owned git ref. Fall back to hard fork only if upstream becomes uncooperative — MIT makes that costless.

---

## Executive summary

Entire.io's open-source CLI is a well-engineered session capture tool with a clean MIT license, zero cloud dependencies on the capture path, and a first-class external agent plugin protocol that Etch can use without forking. The shadow-branch write path handles per-worktree concurrency correctly (OS flock + CAS). However, the metadata-branch commit path (`entire/checkpoints/v1`) uses a bare `go-git SetReference` with no CAS and no lock — two concurrent `post-commit` hooks from different worktrees can race, and the loser's checkpoint metadata is silently lost. This is a fixable bug, not a fundamental architecture problem; the fix pattern already exists in their own codebase. The product has had zero traction with multi-agent or multi-machine users (none found anywhere on the web), and the developer community response after a loud launch has been silence — no organic Reddit threads, no independent X usage tweets after 3.5 months, HN consensus is "trivial to implement." Etch's value-add — workflow-as-versioned-outcome-bound-artifact and verified multi-agent attribution at 60–80 concurrent sessions — is entirely above where Entire is investing and completely unclaimed in the discourse.

---

## The four gates

### 1. Locally queryable without their cloud — **YES**

Zero imports of `cli/api` or `cli/auth` from the capture or storage paths (`checkpoint/`, `strategy/`). The commands `explain`, `rewind`, `sessions`, `transcript`, `trace`, and `status` all work fully offline. The entire.io web app is a read layer over the user's git repo via the GitHub App — there is no server-side data store the CLI feeds. Telemetry is PostHog EU, hashed machine ID, flag names only (not values), opt-out via `ENTIRE_TELEMETRY_OPTOUT=1` or `--telemetry=false`.

**Source:** `committed.go`, `strategy/`, `telemetry/detached.go` — verified by `grep -rln "api\.BaseURL\|api\.Client\|auth\.Store" cmd/entire/cli/checkpoint/ cmd/entire/cli/strategy/` returning no results.

### 2. Survives Etch density — **PARTIAL (fixable)**

**What works:**
- Per-shadow-branch writes: OS flock (`syscall.Flock(LOCK_EX)`) + `git update-ref` CAS retry loop (16 retries, jitter) — `temporary.go:108-176`, `shadow_ref.go:69-151`. Solid.
- Per-worktree namespacing: shadow branches named `entire/<commit[:7]>-<sha256(worktreeID)[:6]>` — different worktrees can't fight. `temporary.go:704-715`.
- Per-session-state: flock at `.git/entire-session-locks/<id>.lock` — `session_state.go:442-484`.

**What breaks:**
- `WriteCommitted` uses `go-git Storer.SetReference` with no CAS, no flock — `committed.go:127, 596, 1420, 1539, 1703`. Two concurrent `post-commit` hooks race; loser's commit becomes a dangling object with unreachable metadata. Silent data loss.
- `KNOWN_LIMITATIONS.md:46-54`: concurrent ACTIVE sessions in one directory produce spurious empty checkpoints. Documented workaround: "use separate worktrees."
- `KNOWN_LIMITATIONS.md:17-44`: git auto-GC can corrupt worktree indexes because go-git creates loose objects GC doesn't fully track. Mitigation: `git config gc.auto 0`.
- Issue #784: only first commit per turn gets `Entire-Checkpoint` trailer; subsequent commits silently get nothing (shadow branch deleted after first condensation).
- Issue #1072: concurrent-session timeout at densities below Etch's target (Windows-specific but same shape).
- No test cases at 20-concurrent density found anywhere.

**Fix path:** Copy the flock + CAS pattern from `shadow_ref.go` to `committed.go`. This is a single PR targeting ~5 call sites. Alternatively, Etch writes its own parallel ref (`etch/metadata/v1`) using the correct CAS discipline and cross-references Entire's checkpoint IDs.

**Net:** The one-worktree-per-session pattern (which Lattice already drives toward) works today. The metadata-branch race is real but architecturally fixable.

### 3. License enables fork/mutate — **YES**

Clean MIT. `.allowed-licenses` restricts transitive deps to MIT/BSD/Apache/MPL/CC0/0BSD — all fork-friendly. No CLA found. No DRM, license check, or phone-home gating. Etch can fork, vendor, rebrand, or slice.

**Source:** `LICENSE`, `.allowed-licenses`, `go.mod`.

### 4. Extensible from outside — **PARTIAL (sufficient for Etch)**

**External agent plugin protocol** (`docs/architecture/external-agent-protocol.md`): the CLI scans `$PATH` for executables named `entire-agent-<name>`, each implementing ~15 JSON-over-stdin subcommands (`info`, `detect`, `get-session-id`, `read-session`, `write-session`, `read-transcript`, etc.). A `entire-agent-etch` binary on $PATH gets every session lifecycle event without forking — session IDs, transcript refs, prompts, tool-use IDs, modified-file lists.

**Git hook chaining** (`strategy/hooks.go:259-318`): Entire renames existing hooks as `.bak` and chains from the new hook. Etch can install its own hooks alongside Entire's using the same pattern.

**Schema stability:** `entire/checkpoints/v1` JSON is documented and machine-readable (per-checkpoint `metadata.json` + per-session `full.jsonl`, `prompt.txt`, `content_hash.txt`). They are actively removing v2 surface area (PRs #1249, #1269), standardizing on v1.

**What you can't do without forking:** inject into `WriteCommitted` to add Etch-specific metadata to `entire/checkpoints/v1`. No published Go SDK. No webhook on commit/condensation.

**Net:** Etch doesn't need to be inside Entire's write path. Etch's workflow/outcome data is fundamentally different from Entire's per-checkpoint snapshot — it belongs in its own ref. The plugin protocol + hook chaining + schema readability give Etch everything it needs.

---

## Architecture as actually implemented

### Session capture
Agent hooks (Claude Code, Codex, Gemini CLI, OpenCode, Cursor, Copilot CLI, Factory, Pi) + git hooks (`prepare-commit-msg`, `commit-msg`, `post-commit`, `post-rewrite`, `pre-push`). Each hook shells out to `entire hooks <agent|git> <hook-name>`, reading the event JSON from stdin. Not a wrapper process, not a filesystem poller.

### Session ID
Agent's native session ID wrapped with date: `2025-12-01-8f76b0e8-b8f1-4a87-...`, persisted to `.entire/current_session`. Handles midnight-crossing sessions.

### Shadow-branch lifecycle
- **Created** on first `WriteTemporary` (turn end, subagent checkpoint, or todo-write)
- **Named** `entire/<commit[:7]>-<sha256(worktreeID)[:6]>` — one per worktree per base commit
- **Local only** — never pushed. Explicit: "do not push them manually, because unredacted source content would be visible"
- **Carries** full worktree tree + `.entire/metadata/<session-id>/` overlay (transcript, prompt, tasks)
- **Flushed** on user `git commit` → `post-commit` runs `CondenseSession` → `WriteCommitted` to `entire/checkpoints/v1`
- **Deleted** after condensation (this is why #784 loses second-and-later commits per turn)

### Metadata-branch (`entire/checkpoints/v1`)
Single branch with per-checkpoint subdirectories (`<id[:2]>/<id[2:]>/<n>/`). Commits carry trailers: `Entire-Session`, `Entire-Strategy`, `Checkpoint`. Pushed to remote on `pre-push` hook.

### Phone-home
None on the capture path. Cloud commands (`recap`, `trail`, `activity`, `search`, `dispatch --cloud`) are separate and require explicit `entire login`. A user can run indefinitely with no Entire account, no network, no telemetry.

### Repo health
Go codebase, 4,438 commits, 53 contributors, 4,422 stars, 32 stable releases (v0.3.0 → v0.6.2), daily nightlies. Top three contributors (Haubold/Pfleiderer/Ong) account for ~53%. Active: 1,638 commits since April 1. 69 open issues (32 bugs). The v2→v1 storage consolidation is in flight (last 2 weeks of PRs). Dohmke is contributor #8 by volume (42 commits) — this is a team-driven project, not a solo show.

---

## What real users say

### Signal volume
The review surface is thin:
- **Zero** G2, Capterra, Trustpilot, ProductHunt reviews
- **Zero** Reddit threads across 13 searched subreddits (r/programming, r/ChatGPTCoding, r/ClaudeAI, r/cursor, r/LocalLLaMA, r/MachineLearning, r/AI_Agents, r/ExperiencedDevs, r/SoftwareEngineering, r/devtools, r/github, r/devops, r/vibecoding)
- **Zero** independent engineer usage tweets after 3.5 months
- **Two** substantive hands-on blog posts (TechStackUps, 10x.pub)
- **One** large HN thread (611 points, 577 comments) that faded immediately
- Notable silence from simonw, swyx, levelsio, rauchg, steipete, and even disclosed angels Theo Browne and Gergely Orosz

### What the two real reviewers said

**Lewis Dwyer (TechStackUps, Apr 2026)** — built a 4-iteration Claude Code project:
- *"Checkpoints is very easy to get started with... the immediate impression that I was working with well-crafted software."*
- *"The checkpoint data tells a different story"* than the git commit — surfaced three failed server startup attempts invisible in the diff
- **Critical:** *"The full API key, in plaintext, is on a Git branch"* and *"Passwords entered in prompts are also captured verbatim."*
- Successfully tested sequential hand-off across clones via `entire resume main` (single-agent, not concurrent)

**"Alex" (10x.pub, spring 2026)** — one week of daily use:
- *"The 'zero friction' claim is mostly accurate. Once installed, you forget it is there."*
- *"Checkpoint data was about 3x the size of my actual codebase"* after one week
- *"You can browse checkpoints chronologically, but there is no semantic search."*
- *"Checkpoints only captures the Claude Code sessions. If I switch to Cursor mid-task, there is a gap."*
- Verdict: **5/10 current utility, 7/10 potential**; *"nice-to-have, not a must-have"*

### HN consensus
- *"$300M valuation for a CLI tool that adds some metadata to Git commits."* (`_el1s7`)
- *"What kind of barrier/moat/network effects would prevent someone with a Claude Code subscription from replicating whatever 'innovation' is so uniquely valuable here?"* (`toraway`)
- *"The VC's didn't give Dohmke $60m to build a new frontend... They're going to capture your conversations and code with AI and use that to train better models."* (`pistoriusp`)
- Multiple DIY clones already shipped by HN commenters within days of launch
- Even defenders focus on the *future enterprise play*, not the current product: *"Day one is to ship the basic capability... Day two is all the enterprise stuff"* (`ttul`)

### Multi-agent / multi-machine users
**None found.** No reviewer, no commenter, no tweeter describes using Entire with multiple concurrent agents or across machines. The only person articulating the pain shape (`whh` on HN: *"I have a lot of concurrent agents working on things at the same time, so I'm not always sure why a piece of code is the way it is months later"*) does not report using Entire for it. Subagent capture is documented as **not currently supported**.

### Standards positioning
Entire has **not** adopted Cursor's Agent Trace RFC — called out on HN (issue #47217015) as a deliberate non-adoption. This makes Entire a "standards orphan" relative to the Cursor + Vercel + Cognition + Cloudflare + Anthropic coalition. Etch should emit Agent Trace by default, which would make Etch-produced metadata more interoperable than Entire's.

---

## Gaps Etch would need to fill on top

1. **Workflow modeling.** Entire models `Session → Checkpoints[]`. Etch needs `Workflow-version → Outcome → Sessions[] → Checkpoints[]`. This is Etch's primary differentiator — Entire has no equivalent and isn't building toward one.

2. **Outcome binding.** Correlate sessions to PR state, CI status, rework count, time-to-merge. Entire captures inputs richly and outcomes not at all.

3. **Multi-agent attribution at density.** Entire's `Entire-Attribution` trailer is per-commit, per-session. Etch needs per-workflow attribution across N concurrent agents touching overlapping files, with explicit "attributed / failed to attribute" status.

4. **Metadata-branch CAS fix.** Either upstream PR (copy `shadow_ref.go` CAS pattern to `committed.go`, ~5 call sites) or Etch-owned parallel ref with correct CAS. Not optional at Etch's density.

5. **Loud failure mode.** Entire's default is silent skip (#784, #686). Etch needs explicit telemetry when attribution succeeds or fails.

6. **Agent Trace emission.** Entire is a standards orphan. Etch should emit Agent Trace and Contextual Commits as side-channel outputs — free interop with the Cursor/Cognition/Anthropic ecosystem.

7. **Next-agent query interface.** The "fresh agent walks into the repo and queries history" use case — `SKILL.md` + context CLI. Entire's `entire explain` is the seed; Etch extends it into a structured query API.

8. **Auto-GC defense.** `git config gc.auto 0` on Etch-managed repos, or a Etch-managed safe-GC pass.

---

## Risks

### Technical
- **Metadata-branch race** is a real bug, not a design flaw. Fixable via PR. If upstream declines the fix, Etch's own ref sidesteps it entirely.
- **Storage bloat** (3x codebase per week per the 10x.pub reviewer) will compound at Etch's density. Needs either aggressive redaction or a compaction strategy.
- **Secret leakage** in shadow branches is a known issue with no current fix beyond "don't push shadow branches." Etch's workflow metadata must not inherit this — Etch should consume from `entire/checkpoints/v1` (which runs through the redaction pipeline) not from shadow branches.
- **go-git alpha dependency** (`go-git/v6 v6.0.0-alpha.4`) — API churn risk for any Etch code that vendors Entire's Go packages.

### Strategic
- **Entire has $60M and 11+ GitHub alumni.** They will iterate fast. Etch can't out-recruit or out-fund. Etch's advantage is operating at a density Entire hasn't even encountered — the 20-agent-per-repo reality — and building the layer above (workflow + outcome + query) that Entire isn't targeting.
- **Native checkpointing inside agent runtimes** (Claude Code `/rewind`, Cursor parallel agents) erodes Entire's single-agent capture moat. Entire's edge has to be cross-agent and at concurrency — exactly where it's weakest. Etch benefits from this erosion because it reduces the value of building capture from scratch.
- **Standards orphan risk.** Entire not adopting Agent Trace could lock it out of the emerging interop ecosystem. Etch should be on the other side of that line.
- **Traction vacuum.** 3.5 months post-launch with no organic community adoption signal is concerning — but for Etch's "use as substrate" posture this is actually neutral. Etch doesn't need Entire to win the market; it needs Entire's code to be sound (it is) and MIT (it is).

### Vendor
- **MIT license eliminates vendor lock.** Etch can fork at any time with zero legal friction.
- **Upstream dependency risk is bounded.** Etch's own layers (workflow, outcome, attribution) don't depend on Entire's roadmap. The only upstream dependency is the capture path, which is stable (v1 consolidation in progress, shadow-branch design is mature).
- **If Entire pivots hard** (e.g., relicenses, goes AGPL, adds phone-home gating), Etch forks the last MIT commit and continues. The codebase as of today (`16e67f3`) is a self-contained Go binary with no cloud dependencies on the capture path.

---

## Recommended next step

1. **Install Entire CLI** on Hyperion and Atlas. Run `entire enable` on one Etch/Forge++ test repo. Capture a week of real multi-agent sessions at normal density (20+ per repo). Verify: do checkpoints land? Do any silently drop? Does storage growth match expectations?

2. **Write a `entire-agent-etch` plugin** — minimal binary on $PATH implementing the external agent protocol. Observe session lifecycle events; log to a Etch-owned JSONL file. This is the integration proof-of-concept; it validates that the plugin protocol gives Etch what it needs without forking.

3. **Open a PR for the metadata-branch CAS fix** — copy the flock + `git update-ref` CAS pattern from `shadow_ref.go` to `committed.go`. Five call sites. Small, well-scoped, directly improves Entire for everyone. If accepted, Etch benefits. If declined, Etch has the diff and can carry it in a fork.

4. **Design the Etch-owned ref** (`etch/workflows/v1` or similar) for workflow modeling and outcome binding. This is where Etch's differentiator lives. Use Entire's checkpoint IDs as cross-references but store Etch's data in Etch's ref — clean separation, no upstream dependency.

5. **Emit Agent Trace from the Etch layer.** Every session Etch captures also produces `agent-trace.json` — instant interop with the Cursor/Cognition/Anthropic ecosystem that Entire is sitting outside of.

---

## Sources

### Sub-Agent A: Architecture & Code Deep-Dive
Full report: `/tmp/etch-entire-eval/A-architecture-report.md`

**Repo files** (all under `/tmp/entire-cli-eval/`, HEAD `16e67f3f`):
- `LICENSE` (MIT)
- `.allowed-licenses`
- `go.mod`, `go.sum`
- `docs/architecture/sessions-and-checkpoints.md`
- `docs/architecture/checkpoint-scenarios.md`
- `docs/architecture/external-agent-protocol.md`
- `docs/architecture/claude-hooks-integration.md`
- `docs/security-and-privacy.md`
- `docs/KNOWN_LIMITATIONS.md`
- `cmd/entire/cli/checkpoint/temporary.go:57-181, 704-715`
- `cmd/entire/cli/checkpoint/shadow_ref.go:69-151`
- `cmd/entire/cli/checkpoint/committed.go:57-132, 596, 1420, 1539, 1703`
- `cmd/entire/cli/internal/flock/flock_unix.go`
- `cmd/entire/cli/strategy/session_state.go:370-540`
- `cmd/entire/cli/strategy/manual_commit_condensation.go:103-266`
- `cmd/entire/cli/strategy/manual_commit_hooks.go:874-1046`
- `cmd/entire/cli/strategy/hooks.go:174-318`
- `cmd/entire/cli/agent/external/discovery.go:18-60`
- `cmd/entire/cli/telemetry/detached.go`

**Live docs:** `docs.entire.io/llms.txt`, `docs.entire.io/overview.md`, `docs.entire.io/cli/checkpoints.md`, `docs.entire.io/security.md`

**GitHub API:** `entireio/cli` repo metadata, issue search (race, concurrent, concurrency, worktree, parallel, multi-agent, corruption), issues #1072, #784, #686, recent PRs, labels

### Sub-Agent B: General Web Reviews
Full report: `/tmp/etch-entire-eval/B-general-reviews-report.md`

- [TechStackUps — Entire.io Hands-On: What It Actually Captures (Apr 8, 2026)](https://techstackups.com/guides/entire-io-hands-on-what-it-actually-captures/)
- [10x.pub — I Tried Entire's Checkpoints CLI for a Week](https://tianpan.co/forum/t/i-tried-entires-checkpoints-cli-for-a-week-here-is-what-the-ai-reasoning-traces-actually-look-like/859)
- [HN launch thread (611pts, 577 comments, Feb 10 2026)](https://news.ycombinator.com/item?id=46961345)
- [GitHub issue #237 — hook ABI breakage](https://github.com/entireio/cli/issues/237)
- [TechCrunch — $60M seed at $300M](https://techcrunch.com/2026/02/10/former-github-ceo-raises-record-60m-dev-tool-seed-round-at-300m-valuation/)
- [Entire — Hello Entire World](https://entire.io/blog/hello-entire-world)
- [Entire — How We Improved Agentic Search (no customer data)](https://entire.io/blog/improving-agentic-search-in-coding-agents)

### Sub-Agent C: Reddit Sentiment
Full report: `/tmp/etch-entire-eval/C-reddit-report.md`

- 30+ `site:reddit.com` queries across 13 subreddits — all NULL
- [HN launch thread](https://news.ycombinator.com/item?id=46961345) (proxy)
- [HN: "If AI writes code, should the session be part of the commit?"](https://news.ycombinator.com/item?id=47217015)
- [Show HN: Git for AI Agents (May 8, 2026 — "inspired by entire.io")](https://news.ycombinator.com/item?id=48063548)
- [aitooldiscovery.com — Best AI for Coding: Reddit's Top Picks 2026](https://www.aitooldiscovery.com/guides/best-ai-for-coding-reddit) — Entire absent

### Sub-Agent D: Twitter/X Discourse
Full report: `/tmp/etch-entire-eval/D-twitter-report.md`

- [@ashtom launch tweet](https://x.com/ashtom/status/2021255786966708280)
- [@OfficialLoganK reaction](https://x.com/OfficialLoganK/status/2021270096623124526)
- [@brianrumao congratulations](https://x.com/brianrumao/status/2021272816495231196)
- [@ashtom Copilot CLI alliance](https://x.com/ashtom/status/2032864804625592708)
- [@thenewstack launch coverage](https://x.com/thenewstack/status/2021283087640879584)
- [Julien Danjou — "git is unkillable"](https://julien.danjou.info/blog/github-wont-work-for-ai-agents/)
- [HN: Agent Trace adoption / Entire non-adoption](https://news.ycombinator.com/item?id=47217015)

### Prior research
- [Etch RESEARCH.md](/Users/atin/Projects/Stage11/code/Forge++/RESEARCH.md)
- [Agent Trace RFC](https://github.com/cursor/agent-trace)
- [Contextual Commits](https://github.com/berserkdisruptors/contextual-commits)
