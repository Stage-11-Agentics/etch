# ETCH-36: session_start latency ~187ms median / 367ms p99 (target 50ms); O(N) recovery scan per start

AUDIT ITEM 5 (latency benchmark, 100 runs each, /tmp/etch-lat/bench.py on m4 Max).
RESULTS (median / p99 / max ms): session_start 186.9 / 366.8 / 398.3; user_prompt_submit 6.9 / 14.2; pre_tool_use 5.7 / 7.1; post_tool_use 5.7 / 6.8; stop 6.1 / 8.9; session_end 6.6 / 9.8.
session_start is ~3.7x the 50ms target; p99 (367ms) breaches the 200ms user-lag threshold. Other 5 hooks are fine (~6ms).
ROOT CAUSE: session_start does many serial git subprocess calls (branch, head sha, worktree, repo root, 2x git config for operator, uname x1-2) PLUS a full recovery scan (RecoverAll -> ScanOrphaned reads the sessions dir and parses every .wip file) on EVERY start. The recovery scan is O(N) in accumulated/concurrent .wip files -> O(N^2) across a 60-80 agent burst, exactly the design's target scale. FIX: parallelize/ctx-cache git calls; throttle or bound the recovery scan (e.g. probabilistic or lock-guarded sweep) so it doesn't run fully on every start.
