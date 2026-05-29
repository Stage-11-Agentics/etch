package info

import (
	"encoding/json"
	"fmt"

	"forgejo.stage11.ai/s11/etch/internal/version"
)

type Response struct {
	Name                    string       `json:"name"`
	Version                 string       `json:"version"`
	Hooks                   bool         `json:"hooks"`
	TranscriptAnalyzer      bool         `json:"transcript_analyzer"`
	CompactTranscript       bool         `json:"compact_transcript"`
	TokenCalculator         bool         `json:"token_calculator"`
	TextGenerator           bool         `json:"text_generator"`
	HookResponseWriter      bool         `json:"hook_response_writer"`
	SubagentAwareExtractor  bool         `json:"subagent_aware_extractor"`
}

func Run() error {
	resp := Response{
		Name:                   "etch",
		Version:                version.Version,
		Hooks:                  true,
		TranscriptAnalyzer:     true,
		CompactTranscript:      false,
		TokenCalculator:        true,
		TextGenerator:          false,
		HookResponseWriter:     false,
		SubagentAwareExtractor: true,
	}
	out, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}
