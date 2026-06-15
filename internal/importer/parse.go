package importer

import "encoding/json"

// extractAgentSessionID reads the agent_session_id field from a committed
// session.json blob. Used by dedup to build the set of already-recorded
// upstream session ids.
func extractAgentSessionID(data []byte) (string, error) {
	var s struct {
		AgentSessionID *string `json:"agent_session_id"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return "", err
	}
	if s.AgentSessionID == nil {
		return "", nil
	}
	return *s.AgentSessionID, nil
}

// jsonStringField extracts a top-level string field from an arbitrary JSON
// object, returning "" when absent or not a string.
func jsonStringField(raw json.RawMessage, field string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	v, ok := m[field]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) != nil {
		return ""
	}
	return s
}
