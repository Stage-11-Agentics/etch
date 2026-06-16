package capture

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// CaptureOrchestration reads orchestration provenance from the environment.
//
// Three layers, highest precedence first:
//   - Typed vars: ETCH_TICKET_ID, ETCH_AGENT_ROLE, ETCH_RUN_ID, etc. — what an
//     orchestrator explicitly declares.
//   - Flexible namespace: any ETCH_META_<key> var is harvested into Extra[<key>]
//     so new provenance dimensions need no etch code change — an orchestrator
//     just sets an env var. Merged over the ETCH_ORCHESTRATION_EXTRA JSON blob.
//
// The third layer — best-effort auto-detection of fields nobody declared, from
// signals etch already captures (git branch, c11 tab title) — is applied
// separately by EnrichOrchestration, which needs those signals as input.
func CaptureOrchestration() Orchestration {
	o := Orchestration{
		Type:  envOrDefault("ETCH_ORCHESTRATOR_TYPE", "manual"),
		Extra: make(map[string]any),
	}

	o.DispatchMethod = envPtr("ETCH_DISPATCH_METHOD")
	o.TicketID = envPtr("ETCH_TICKET_ID")
	o.RunID = envPtr("ETCH_RUN_ID")
	o.Role = envPtr("ETCH_AGENT_ROLE")
	o.WorkflowVersion = envPtr("ETCH_WORKFLOW_VERSION")

	if extra := os.Getenv("ETCH_ORCHESTRATION_EXTRA"); extra != "" {
		var parsed map[string]any
		if json.Unmarshal([]byte(extra), &parsed) == nil {
			o.Extra = parsed
		}
	}

	// Flexible namespace: ETCH_META_<key>=<value> → Extra[<key>]. The whole
	// point is future-proofing: an orchestrator can attach arbitrary provenance
	// (wave, delegator_index, eval_gate, ...) with no change to this code.
	for _, kv := range os.Environ() {
		name, val, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(name, metaEnvPrefix) || val == "" {
			continue
		}
		key := strings.ToLower(strings.TrimPrefix(name, metaEnvPrefix))
		if key != "" {
			o.Extra[key] = val
		}
	}

	return o
}

const metaEnvPrefix = "ETCH_META_"

// knownRoles are the orchestration roles auto-detection recognizes in a c11 tab
// title or pane lineage. Conservative on purpose — only well-known roles, so a
// stray title word is unlikely to be misread as a role.
var knownRoles = []string{
	"orchestrator", "delegator", "reviewer", "implementer",
	"validator", "planner", "architect", "fixer",
}

// ticketRe matches a ticket-shaped token: a letter-led alphanumeric project key
// (C11, FT, ETCH, LAT) then a dash and digits — "C11-143", "ft-481", "ETCH-52".
var ticketRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*-\d+`)

// DetectTicketID best-effort extracts a ticket id from a git branch or c11 tab
// title, returning the id (uppercased) and which signal it came from, or
// ("","") if none. Branch wins over tab title. Version-like tokens ("bump-0" in
// "bump-0.01.002") are rejected so a release branch isn't read as a ticket.
func DetectTicketID(branch, tabTitle string) (id, source string) {
	if t := extractTicket(branch); t != "" {
		return t, "branch"
	}
	if t := extractTicket(tabTitle); t != "" {
		return t, "c11_tab_title"
	}
	return "", ""
}

func extractTicket(s string) string {
	for _, loc := range ticketRe.FindAllStringIndex(s, -1) {
		// Reject a match whose digit run is immediately followed by '.' or
		// preceded by '.', i.e. part of a dotted version like 0.01.002.
		if loc[1] < len(s) && s[loc[1]] == '.' {
			continue
		}
		if loc[0] > 0 && s[loc[0]-1] == '.' {
			continue
		}
		return strings.ToUpper(s[loc[0]:loc[1]])
	}
	return ""
}

// DetectRole best-effort reads THIS session's known orchestration role out of
// its c11 tab title or pane lineage (e.g. "C11-143 :: Delegator" → "delegator"),
// or "". The current pane's role wins over ancestors': the tab title is checked
// first, then the lineage from the most-recent (last) element backward — so a
// delegator under an orchestrator reads as "delegator", not "orchestrator".
func DetectRole(tabTitle string, lineage []string) string {
	if r := roleIn(tabTitle); r != "" {
		return r
	}
	for i := len(lineage) - 1; i >= 0; i-- {
		if r := roleIn(lineage[i]); r != "" {
			return r
		}
	}
	return ""
}

func roleIn(s string) string {
	s = strings.ToLower(s)
	for _, r := range knownRoles {
		if strings.Contains(s, r) {
			return r
		}
	}
	return ""
}

// EnrichOrchestration fills orchestration fields that no explicit ETCH_* var
// provided, using signals etch already captures — the git branch and c11 tab
// title/lineage. This is what makes capture work with zero orchestrator
// cooperation: a delegator that exported nothing still gets its ticket/role
// recovered. Explicit values are NEVER overridden, and every inferred field is
// recorded in Extra["_sources"] so a consumer can tell declared from inferred.
func EnrichOrchestration(o *Orchestration, branch, tabTitle string, lineage []string) {
	if o.Extra == nil {
		o.Extra = map[string]any{}
	}
	sources := map[string]string{}

	if o.TicketID == nil {
		if id, src := DetectTicketID(branch, tabTitle); id != "" {
			o.TicketID = &id
			sources["ticket_id"] = src
		}
	}
	if o.Role == nil {
		if r := DetectRole(tabTitle, lineage); r != "" {
			o.Role = &r
			sources["role"] = "c11_tab_title"
		}
	}

	if len(sources) > 0 {
		o.Extra["_sources"] = sources
	}
}

// CaptureOperator reads git user and OS user.
func CaptureOperator(dir string) OperatorInfo {
	name := gitOutput(dir, "git", "config", "user.name")
	email := gitOutput(dir, "git", "config", "user.email")

	gitUser := name
	if email != "" {
		gitUser = name + " <" + email + ">"
	}

	return OperatorInfo{
		GitUser: gitUser,
		OSUser:  os.Getenv("USER"),
	}
}

// CaptureC11 reads c11 env vars. Returns nil if not in a c11 session.
//
// pane_lineage is built from ETCH_PANE_LINEAGE (a JSON array of ancestor tab
// titles set by the spawning orchestrator) with the current tab title appended.
// Solo sessions get a single-element lineage of their own title.
func CaptureC11() *C11Info {
	wsID := os.Getenv("C11_WORKSPACE_ID")
	surfID := os.Getenv("C11_SURFACE_ID")

	if wsID == "" && surfID == "" {
		// Check legacy env vars
		wsID = os.Getenv("CMUX_WORKSPACE_ID")
		surfID = os.Getenv("CMUX_SURFACE_ID")
	}

	if wsID == "" && surfID == "" {
		return nil
	}

	info := &C11Info{
		WorkspaceID: wsID,
		SurfaceID:   surfID,
	}

	if title := c11TabTitle(surfID); title != "" {
		info.TabTitle = title
	}

	info.PaneLineage = buildPaneLineage(info.TabTitle)

	return info
}

// buildPaneLineage returns the chain of tab titles from the root orchestrator
// to the current pane. The parent chain is read from ETCH_PANE_LINEAGE
// (JSON array); the current tab title is appended.
func buildPaneLineage(currentTitle string) []string {
	var lineage []string

	if raw := os.Getenv("ETCH_PANE_LINEAGE"); raw != "" {
		var parsed []string
		if json.Unmarshal([]byte(raw), &parsed) == nil {
			lineage = parsed
		}
	}

	if currentTitle != "" {
		lineage = append(lineage, currentTitle)
	}

	return lineage
}

// CaptureTranscriptRef builds a TranscriptRef from the session_ref value.
func CaptureTranscriptRef(sessionRef string) *TranscriptRef {
	if sessionRef == "" {
		return nil
	}

	ref := &TranscriptRef{
		LocalPath: &sessionRef,
	}

	// Check if the file actually exists
	_, err := os.Stat(sessionRef)
	ref.Available = err == nil

	return ref
}

// InferRuntime tries to determine the agent runtime from available signals.
func InferRuntime() string {
	if os.Getenv("CLAUDECODE") != "" {
		return "claude-code"
	}
	if os.Getenv("CODEX_CLI") != "" {
		return "codex"
	}
	if os.Getenv("GEMINI_CLI") != "" {
		return "gemini-cli"
	}
	return "unknown"
}

// InferPromptSource guesses how the prompt was delivered.
func InferPromptSource() string {
	if os.Getenv("C11_SURFACE_ID") != "" || os.Getenv("CMUX_SURFACE_ID") != "" {
		return "c11_send"
	}

	// Check if stdin is a pipe (non-interactive)
	fi, err := os.Stdin.Stat()
	if err == nil && fi.Mode()&os.ModeCharDevice == 0 {
		return "pipe"
	}

	return "interactive"
}

func envPtr(key string) *string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	return &v
}

func envOrDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func c11TabTitle(surfaceID string) string {
	if surfaceID == "" {
		return ""
	}
	cmd := exec.Command("c11", "get-titlebar-state", "--surface", surfaceID, "--json")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	var state struct {
		Title string `json:"title"`
	}
	if json.Unmarshal(out, &state) == nil {
		return state.Title
	}
	return strings.TrimSpace(string(out))
}
