# ETCH-33: Recovered crash sessions double-count tool calls (pre+post) vs Finalize (pre only)

AUDIT ITEM 1. REPRO: crash a session after 1 pre_tool_use + 1 post_tool_use, recover via timeout; recovered session.json shows tool_use.total_calls=2 (by_tool Read:2) for ONE logical tool call.
ROOT CAUSE inconsistency: capture/buffer.go Finalize counts only 'pre_tool_use' (line ~195: if ev.Hook==pre_tool_use). recovery/recovery.go RecoverSession counts BOTH pre_tool_use AND post_tool_use (lines ~183-187). So a recovered (incomplete) record reports ~2x the tool calls of an equivalent complete record, corrupting any cross-session tool-use analytics. FIX: recovery should count pre_tool_use only, matching Finalize. Verified empirically in /tmp/etch-crash.
