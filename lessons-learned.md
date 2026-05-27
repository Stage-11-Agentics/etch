# Lessons learned — Etch

Append-only log. Every failure, point of confusion, or thrash gets an entry. The point is to make the next agent or session pay less for the same problem.

**Format per entry:**
- `## YYYY-MM-DD — <short title>` (one-line header)
- **What happened**: factual one-paragraph
- **Why it bit**: the root cause, not just the symptom
- **Fix applied** (if any): what was done in this run
- **For next time**: what should change in scripts, skills, or process

---

## 2026-05-26 — Phase 0 PoC is Python, production must be Go

**What happened**: The Phase 0 proof-of-concept `entire-agent-cairn` binary was built as a Python script for speed of validation. Entire's plugin ecosystem is Go-native and the production binary must be Go to avoid a Python runtime dependency.

**Why it bit**: Not a failure — intentional design choice. But worth recording so no future agent tries to extend the Python PoC instead of building the Go replacement.

**Fix applied**: Phase 0 PoC stays as `./entire-agent-cairn` (Python) for reference. Ticket ETCH-1 builds the Go replacement.

**For next time**: When building PoCs for validation gates, name them distinctly (e.g., `entire-agent-cairn-poc`) to avoid confusion with the production artifact.
