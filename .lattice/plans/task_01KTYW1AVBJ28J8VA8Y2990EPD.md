# ETCH-50: agent.version writes Etch's own version, OUTPUT_SPEC says agent-CLI version

Found during ETCH-49 README review: internal/hooks/session_start.go sets agent.version from version.Version (Etch's 0.01.001), but OUTPUT_SPEC.md:23 documents agent.version as the agent CLI's version (nullable, e.g. 1.0.33). Code and spec disagree; decide which is canonical and align the other. Real refs show {runtime:claude-code, version:0.01.001}.
