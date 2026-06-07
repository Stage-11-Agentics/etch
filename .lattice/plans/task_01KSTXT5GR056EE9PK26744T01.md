# ETCH-37 — Plan pointer

ETCH-37 is implemented as part of the **schema/privacy batch** (ETCH-23 + ETCH-37 + ETCH-40 f.10, one PR, worker agent:schema-w2).

**The authoritative plan is the batch plan: [`task_01KSTXJG3MWS6CJWNQV1VVYF6Q.md`](./task_01KSTXJG3MWS6CJWNQV1VVYF6Q.md)** — see "Item 2 — ETCH-37: per-repo hostname salt" and the "Plan-Review Cycle 1 Resolutions" section.

Decision (operator, 2026-06-06, run-state.md Operator Decisions): **per-repo salt** — random salt generated at first use, stored in committed `.etch/settings.json`; `hostname_hash = SHA-256(salt + hostname)`. README's "salted hash" claim becomes true. Decision is final; not re-litigated.

Summary of the ETCH-37 slice:
- `config.Settings` gains `hostname_salt`; new `config.EnsureHostnameSalt(repoRoot)` generates + persists (preserving unknown settings keys) on first use.
- Single hash derivation: `redact.HashHostname(salt, hostname)`; `capture.CaptureMachine` delegates (fixes the dual unsalted implementations flagged by plan-review).
- Doc surfaces: README settings + privacy sections, OUTPUT_SPEC.md:52/:149, SPEC.md acceptance #7.
- Tests: salt generation/persistence/preservation; cross-repo hash difference; within-repo stability; signature updates for existing machine/hostname tests.
