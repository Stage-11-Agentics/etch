package schema

type AgentTrace struct {
	Version string       `json:"version"`
	Traces  []TraceEntry `json:"traces"`
}

type TraceEntry struct {
	AgentID   string   `json:"agent_id"`
	Model     string   `json:"model"`
	SessionID string   `json:"session_id"`
	Files     []string `json:"files"`
	Timestamp string   `json:"timestamp"`
}

func SessionToAgentTrace(session *Session) *AgentTrace {
	files := make([]string, len(session.FilesTouched))
	for i, f := range session.FilesTouched {
		files[i] = f.Path
	}

	var ts string
	if session.Timing.EndedAt != nil {
		ts = *session.Timing.EndedAt
	} else if session.Timing.StartedAt != nil {
		ts = *session.Timing.StartedAt
	}

	model := ""
	if session.Agent.Model != nil {
		model = *session.Agent.Model
	}

	return &AgentTrace{
		Version: "1.0",
		Traces: []TraceEntry{
			{
				AgentID:   session.Agent.Runtime,
				Model:     model,
				SessionID: session.SessionID,
				Files:     files,
				Timestamp: ts,
			},
		},
	}
}
