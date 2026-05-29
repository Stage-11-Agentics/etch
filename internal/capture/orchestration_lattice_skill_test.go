package capture

import "testing"

// These tests pin the ETCH_* env-var contract that the lattice-orchestrator
// skill exports in its delegator boot prompts (c11 repo:
// skills/lattice-orchestrator/references/orchestrator.md).
// If CaptureOrchestration drifts from that contract, orchestrated sessions
// will silently mis-capture and these tests fail. See OUTPUT_SPEC.md §3 and
// docs/lattice-skill-integration.md.

// allEtchVars is every ETCH_* var the skill may export. Tests clear them all
// up front so a var leaking in from the runner's environment can't taint a case.
var allEtchVars = []string{
	"ETCH_ORCHESTRATOR_TYPE",
	"ETCH_DISPATCH_METHOD",
	"ETCH_TICKET_ID",
	"ETCH_RUN_ID",
	"ETCH_AGENT_ROLE",
	"ETCH_WORKFLOW_VERSION",
	"ETCH_PARENT_SESSION_ID",
	"ETCH_ORCHESTRATION_EXTRA",
}

func clearEtch(t *testing.T) {
	t.Helper()
	for _, k := range allEtchVars {
		t.Setenv(k, "")
	}
}

// TestCaptureOrchestration_LatticeSkillExports sets all 8 ETCH_* vars to the
// representative values from the skill's fast-track export block and asserts
// every Orchestration field is populated. ETCH_PARENT_SESSION_ID is set too
// (as the skill exports it) but is consumed at the hook layer onto
// Session.ParentSessionID, not onto the Orchestration struct — so it is not
// asserted here; see TestSessionStart hook tests for that path.
func TestCaptureOrchestration_LatticeSkillExports(t *testing.T) {
	clearEtch(t)
	t.Setenv("ETCH_ORCHESTRATOR_TYPE", "lattice-orchestrator")
	t.Setenv("ETCH_DISPATCH_METHOD", "c11_delegator")
	t.Setenv("ETCH_TICKET_ID", "ETCH-12")
	t.Setenv("ETCH_RUN_ID", "01JWB8FGXQPNR7TV0ZYM4GD1AA")
	t.Setenv("ETCH_AGENT_ROLE", "delegator")
	t.Setenv("ETCH_WORKFLOW_VERSION", "d44c948")
	t.Setenv("ETCH_PARENT_SESSION_ID", "01JWB7MMXQPNR7TV0ZYM4GD0ZZ")
	t.Setenv("ETCH_ORCHESTRATION_EXTRA", `{"mode":"fast-track"}`)

	o := CaptureOrchestration()

	if o.Type != "lattice-orchestrator" {
		t.Errorf("Type: got %q, want lattice-orchestrator", o.Type)
	}
	assertPtr(t, "DispatchMethod", o.DispatchMethod, "c11_delegator")
	assertPtr(t, "TicketID", o.TicketID, "ETCH-12")
	assertPtr(t, "RunID", o.RunID, "01JWB8FGXQPNR7TV0ZYM4GD1AA")
	assertPtr(t, "Role", o.Role, "delegator")
	assertPtr(t, "WorkflowVersion", o.WorkflowVersion, "d44c948")
	if o.Extra["mode"] != "fast-track" {
		t.Errorf("Extra[mode]: got %v, want fast-track", o.Extra["mode"])
	}
}

// TestCaptureOrchestration_ExtraJSON verifies the open property bag parses a
// mixed-type JSON object: string + JSON numbers (which decode to float64).
func TestCaptureOrchestration_ExtraJSON(t *testing.T) {
	clearEtch(t)
	t.Setenv("ETCH_ORCHESTRATION_EXTRA", `{"phase":"impl","wave":2,"retry_count":3}`)

	o := CaptureOrchestration()

	if got := o.Extra["phase"]; got != "impl" {
		t.Errorf("Extra[phase]: got %v (%T), want impl", got, got)
	}
	if got := o.Extra["wave"]; got != float64(2) {
		t.Errorf("Extra[wave]: got %v (%T), want float64(2)", got, got)
	}
	if got := o.Extra["retry_count"]; got != float64(3) {
		t.Errorf("Extra[retry_count]: got %v (%T), want float64(3)", got, got)
	}
	if len(o.Extra) != 3 {
		t.Errorf("Extra: got %d keys, want 3", len(o.Extra))
	}
}

// TestCaptureOrchestration_AllAbsent: with no ETCH_* vars set, type defaults to
// "manual" (solo human session) and every optional field is nil/empty.
func TestCaptureOrchestration_AllAbsent(t *testing.T) {
	clearEtch(t)

	o := CaptureOrchestration()

	if o.Type != "manual" {
		t.Errorf("Type: got %q, want manual", o.Type)
	}
	if o.DispatchMethod != nil {
		t.Errorf("DispatchMethod: got %v, want nil", *o.DispatchMethod)
	}
	if o.TicketID != nil {
		t.Errorf("TicketID: got %v, want nil", *o.TicketID)
	}
	if o.RunID != nil {
		t.Errorf("RunID: got %v, want nil", *o.RunID)
	}
	if o.Role != nil {
		t.Errorf("Role: got %v, want nil", *o.Role)
	}
	if o.WorkflowVersion != nil {
		t.Errorf("WorkflowVersion: got %v, want nil", *o.WorkflowVersion)
	}
	if len(o.Extra) != 0 {
		t.Errorf("Extra: got %d keys, want 0", len(o.Extra))
	}
}

// TestCaptureOrchestration_OnlyOrchestratorType: only ETCH_ORCHESTRATOR_TYPE is
// set (e.g. a minimal custom wrapper). Type is captured; all other fields nil.
func TestCaptureOrchestration_OnlyOrchestratorType(t *testing.T) {
	clearEtch(t)
	t.Setenv("ETCH_ORCHESTRATOR_TYPE", "lattice-orchestrator")

	o := CaptureOrchestration()

	if o.Type != "lattice-orchestrator" {
		t.Errorf("Type: got %q, want lattice-orchestrator", o.Type)
	}
	if o.DispatchMethod != nil {
		t.Errorf("DispatchMethod: got %v, want nil", *o.DispatchMethod)
	}
	if o.TicketID != nil {
		t.Errorf("TicketID: got %v, want nil", *o.TicketID)
	}
	if o.RunID != nil {
		t.Errorf("RunID: got %v, want nil", *o.RunID)
	}
	if o.Role != nil {
		t.Errorf("Role: got %v, want nil", *o.Role)
	}
	if o.WorkflowVersion != nil {
		t.Errorf("WorkflowVersion: got %v, want nil", *o.WorkflowVersion)
	}
	if len(o.Extra) != 0 {
		t.Errorf("Extra: got %d keys, want 0", len(o.Extra))
	}
}

// TestCaptureOrchestration_LegacyCairnIgnored is the ETCH-15 cutover guard: the
// old CAIRN_* contract is dropped with no backward compat. A session running with
// only legacy CAIRN_* vars set must capture NOTHING from them — Type falls back to
// "manual" and every optional field is nil. This fails loudly if anyone reintroduces
// a CAIRN_* reader.
func TestCaptureOrchestration_LegacyCairnIgnored(t *testing.T) {
	clearEtch(t)
	// Set the full legacy contract; none of it should be honored.
	t.Setenv("CAIRN_ORCHESTRATOR_TYPE", "lattice-orchestrator")
	t.Setenv("CAIRN_DISPATCH_METHOD", "c11_delegator")
	t.Setenv("CAIRN_TICKET_ID", "ETCH-15")
	t.Setenv("CAIRN_RUN_ID", "01JWB8FGXQPNR7TV0ZYM4GD1AA")
	t.Setenv("CAIRN_AGENT_ROLE", "delegator")
	t.Setenv("CAIRN_WORKFLOW_VERSION", "legacy")
	t.Setenv("CAIRN_ORCHESTRATION_EXTRA", `{"mode":"fast-track"}`)

	o := CaptureOrchestration()

	if o.Type != "manual" {
		t.Errorf("Type: got %q, want manual (legacy CAIRN_ORCHESTRATOR_TYPE must be ignored)", o.Type)
	}
	if o.DispatchMethod != nil {
		t.Errorf("DispatchMethod: got %v, want nil (legacy CAIRN_DISPATCH_METHOD must be ignored)", *o.DispatchMethod)
	}
	if o.TicketID != nil {
		t.Errorf("TicketID: got %v, want nil (legacy CAIRN_TICKET_ID must be ignored)", *o.TicketID)
	}
	if o.RunID != nil {
		t.Errorf("RunID: got %v, want nil (legacy CAIRN_RUN_ID must be ignored)", *o.RunID)
	}
	if o.Role != nil {
		t.Errorf("Role: got %v, want nil (legacy CAIRN_AGENT_ROLE must be ignored)", *o.Role)
	}
	if o.WorkflowVersion != nil {
		t.Errorf("WorkflowVersion: got %v, want nil (legacy CAIRN_WORKFLOW_VERSION must be ignored)", *o.WorkflowVersion)
	}
	if len(o.Extra) != 0 {
		t.Errorf("Extra: got %d keys, want 0 (legacy CAIRN_ORCHESTRATION_EXTRA must be ignored)", len(o.Extra))
	}
}

// TestCaptureOrchestration_EtchHonoredCairnIgnored sets BOTH the new ETCH_ and a
// conflicting legacy CAIRN_ value for ticket id, and asserts the ETCH_ value wins
// and the CAIRN_ value leaks nowhere.
func TestCaptureOrchestration_EtchHonoredCairnIgnored(t *testing.T) {
	clearEtch(t)
	t.Setenv("ETCH_TICKET_ID", "ETCH-15")
	t.Setenv("CAIRN_TICKET_ID", "CAIRN-OLD")

	o := CaptureOrchestration()

	assertPtr(t, "TicketID", o.TicketID, "ETCH-15")
}

func assertPtr(t *testing.T, name string, got *string, want string) {
	t.Helper()
	if got == nil {
		t.Errorf("%s: got nil, want %q", name, want)
		return
	}
	if *got != want {
		t.Errorf("%s: got %q, want %q", name, *got, want)
	}
}
