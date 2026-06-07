# Self-review — ETCH-19 (agent:clidocs-w2-reviewer)

**Verdict: PASS**

Walked `git diff origin/main...HEAD` (commit 4cf8dac, README-only).

## Findings

1. **[FIXED] "see config below" was wrong direction** — the `.etch/settings.json` section sits above Usage; corrected to "see the `.etch/settings.json` section above" before the commit was cut.
2. **[OK] No stale claims remain** — grepped for "coming / in progress / not yet / ETCH-n / ticket-tracked"; only remaining hit is `--ticket ETCH-9` as an example filter value, which is intentional.
3. **[OK] `--repo` discipline** — README shows `--repo` on query/index only; archive/restore-archive examples are flag-accurate (they operate on cwd). Matches auto plan-review MINOR #2 guardrail.
4. **[OK] Additive accuracy** — recently merged redaction, refspec, and auto-capture/install sections untouched (diff confirms only Status bullet, config note, and the Usage querying paragraph changed).
5. **[OK] Per-ticket commits** — ETCH-21 code commit lands before this README commit, so the README's `help` reference is never ahead of the binary. Matches auto plan-review MINOR #1.

Validation: every README example executed in a temp repo with a real captured session — all query filter variants, index build/update/show/drop, archive --dry-run/real/--threshold-days/--quarter, plus a full archive → restore-archive round-trip.