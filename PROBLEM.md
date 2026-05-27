# The Problem: Agent Work Is Invisible

## The gap

When an AI coding agent finishes a session, three things survive: the code diff, the commit message, and maybe a co-author trailer. Everything else — the prompt that launched the work, the reasoning the agent walked through, the dead ends it explored, the orchestration pattern that coordinated it, the tools it called, the tokens it burned, the outcome it produced — evaporates.

This is fine when one developer runs one agent on one laptop.

It is not fine when the operating reality looks like this:

- 20+ terminal agents running concurrently inside a single repo — Claude Code, Codex, Gemini CLI, OpenCode — across worktrees and shared branches
- 20 more across three other repos on the same machine
- More agents on a second machine touching the same repos via git push/pull
- 60–80+ concurrent agent sessions, distributed across machines, multiple agent types per repo

At that density, the code diff tells you *what* changed. It tells you nothing about *why*, *how*, *who orchestrated it*, *what was tried and abandoned*, or *what the agent knew when it made the decision*. The next agent that picks up the repo is blind to everything its predecessors did except the final artifact.

## Why this matters

The cost isn't theoretical. It shows up as:

**Redundant exploration.** Agent B re-discovers the same dead end Agent A explored three hours ago, because Agent A's reasoning didn't survive. Multiply by 20 concurrent agents and the waste compounds hourly.

**Unattributable regressions.** A bug ships. Twenty agents touched the repo in the last hour. The commit log shows who wrote the line but not which orchestration pattern produced it, which prompt initiated it, or which agent runtime made the tool-use decision that led there.

**Opaque orchestration.** The human operator chose a multi-agent dispatch pattern (Lattice orchestrator, plan→implement→review, manual fan-out). That choice — which is itself an evolving artifact that gets tuned over weeks — is nowhere in the record. The next operator inherits no context about what orchestration patterns worked and which didn't.

**Lost prompts.** The prompt is the specification. When the prompt that produced a result doesn't survive alongside the result, the result becomes an orphan — correct but unexplained, and impossible to reproduce or iterate on.

**Invisible cost.** Token usage, API call count, cache hit rates, session duration — the operational cost of agent work is scattered across billing dashboards and never tied back to the work itself.

## What exists today

Several tools capture pieces of this:

- **Entire CLI** (Thomas Dohmke, $60M seed) captures session transcripts and binds them to git commits via a dedicated checkpoint branch. Well-engineered single-agent capture. No multi-agent concurrency story under load, no workflow metadata, no outcome data. The capture path is sound; the scope is narrow.
- **Agent Trace** (Cursor RFC, signed by Anthropic/OpenAI/Google/etc.) standardizes attribution metadata linking code ranges to conversations. Storage-agnostic. Deliberately punts on prompt capture, workflow modeling, and quality assessment.
- **Contextual Commits** embeds intent/decision/constraint/learned action lines in commit messages. Complementary to transcript capture but limited to what fits in a commit body.
- **promptcellar, claude-mem, observability dashboards** — various single-agent, single-machine capture tools. None handle concurrent agents. None bind to git.

No existing tool captures the full picture — prompt, transcript, orchestration, tool use, outcome, cost — as flat metadata at the density of 20+ concurrent agents per repo across machines.

## The core requirement

Capture everything. Store it flat. Make it queryable. Worry about analysis later.

Specifically, per agent session, record:
- The prompt that initiated the work
- The full chat/transcript
- Which orchestration pattern was used
- Which agent runtime (Claude Code, Codex, Gemini, etc.) and model
- Tool use events
- Files touched
- Token usage and cost
- Session timing (start, end, duration)
- Machine identity and operator
- The git state at session start and end (branch, HEAD, worktree path)
- The outcome (commit SHAs produced, PR state, CI status — as observed metadata, not a computed correlation)

This metadata should be:
- **Flat, not hierarchical.** No `Workflow → Outcome → Session → Checkpoint` tree. Just records with fields. The relationships are implicit in shared identifiers (same repo, same time window, same operator, same ticket ID if one exists). Structure emerges from querying, not from the schema.
- **Timeless.** Each record stands alone. No foreign-key dependencies that break if a parent record is missing.
- **Git-native.** Travels with push/pull. Visible wherever the repo is cloned. No external database required for basic access.
- **Concurrent-safe at 60+ sessions.** The capture mechanism cannot use a single mutable shared ref that N writers race on. Per-session isolation at the write path; merge at query time.
- **Machine-agnostic.** A session captured on Machine A is visible to an agent that next runs on Machine B, via normal git fetch.

The analysis layer — "which prompts correlate with clean first-push CI?", "which orchestration patterns produce less rework?", "why did this specific regression ship?" — is a consumer of this data, not part of the capture system. Capture first. Analyze later.
