// Package info implements the `info` subcommand: Entire's external-agent
// discovery contract. Entire scans $PATH for entire-agent-<name> binaries,
// runs `<binary> info`, and unmarshals stdout into its InfoResponse struct
// (entire cmd/entire/cli/agent/external/types.go, protocol version 1).
// A response without protocol_version == 1 is silently skipped at discovery.
package info

import (
	"encoding/json"
	"fmt"

	"forgejo.stage11.ai/s11/etch/internal/version"
)

// ProtocolVersion is Entire's external-agent protocol version this binary
// implements. Entire v0.6.3 enforces an exact match.
const ProtocolVersion = 1

// Capabilities mirrors Entire's agent.DeclaredCaps. Only declare what is
// implemented against Entire's exact subcommand protocol — declaring a
// capability makes Entire call the corresponding subcommands and expect
// protocol-shaped responses.
type Capabilities struct {
	Hooks                  bool `json:"hooks"`
	TranscriptAnalyzer     bool `json:"transcript_analyzer"`
	TranscriptPreparer     bool `json:"transcript_preparer"`
	TokenCalculator        bool `json:"token_calculator"`
	CompactTranscript      bool `json:"compact_transcript"`
	TextGenerator          bool `json:"text_generator"`
	HookResponseWriter     bool `json:"hook_response_writer"`
	SubagentAwareExtractor bool `json:"subagent_aware_extractor"`
}

// Response mirrors Entire's external.InfoResponse. The extra "version" field
// is ignored by Entire and kept for humans.
type Response struct {
	ProtocolVersion int          `json:"protocol_version"`
	Name            string       `json:"name"`
	Type            string       `json:"type"`
	Description     string       `json:"description"`
	IsPreview       bool         `json:"is_preview"`
	ProtectedDirs   []string     `json:"protected_dirs"`
	ProtectedFiles  []string     `json:"protected_files"`
	HookNames       []string     `json:"hook_names"`
	Capabilities    Capabilities `json:"capabilities"`
	Version         string       `json:"version"`
}

// HookNames are the hook subcommands this binary accepts on stdin.
var HookNames = []string{
	"session_start",
	"user_prompt_submit",
	"pre_tool_use",
	"post_tool_use",
	"stop",
	"session_end",
}

func Run() error {
	resp := Response{
		ProtocolVersion: ProtocolVersion,
		Name:            "etch",
		Type:            "etch",
		Description:     "Etch — flat session metadata capture into refs/etch/sessions/*",
		IsPreview:       false,
		ProtectedDirs:   []string{},
		ProtectedFiles:  []string{},
		HookNames:       HookNames,
		Capabilities: Capabilities{
			Hooks: true,
			// All other capabilities are deliberately false: their Entire
			// protocol subcommand shapes are not implemented by this binary.
		},
		Version: version.Version,
	}
	out, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}
