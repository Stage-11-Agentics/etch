package capture

import "testing"

// These tests pin the CAIRN_* env-var contract that the lattice-orchestrator
// skill exports in its delegator boot prompts (c11 repo:
// skills/lattice-orchestrator/references/orchestrator.md, commit d44c948).
// If CaptureOrchestration drifts from that contract, orchestrated sessions
// will silently mis-capture and these tests fail. See OUTPUT_SPEC.md §3 and
// docs/lattice-skill-integration.md.

// allCairnVars is every CAIRN_* var the skill may export. Tests clear them all
// up front so a var leaking in from the runner's environment can't taint a case.
var allCairnVars = []string{
	"CAIRN_ORCHESTRATOR_TYPE",
	"CAIRN_DISPATCH_METHOD",
	"CAIRN_TICKET_ID",
	"CAIRN_RUN_ID",
	"CAIRN_AGENT_ROLE",
	"CAIRN_WORKFLOW_VERSION",
	"CAIRN_PARENT_SESSION_ID",
	"CAIRN_ORCHESTRATION_EXTRA",
}

func clearCairn(t *testing.T) {
	t.Helper()
	for _, k := range allCairnVars {
		t.Setenv(k, "")
	}
}

// TestCaptureOrchestration_LatticeSkillExports sets all 8 CAIRN_* vars to the
// representative values from the skill's fast-track export block and asserts
// every Orchestration field is populated. CAIRN_PARENT_SESSION_ID is set too
// (as the skill exports it) but is consumed at the hook layer onto
// Session.ParentSessionID, not onto the Orchestration struct — so it is not
// asserted here; see TestSessionStart hook tests for that path.
func TestCaptureOrchestration_LatticeSkillExports(t *testing.T) {
	clearCairn(t)
	t.Setenv("CAIRN_ORCHESTRATOR_TYPE", "lattice-orchestrator")
	t.Setenv("CAIRN_DISPATCH_METHOD", "c11_delegator")
	t.Setenv("CAIRN_TICKET_ID", "ETCH-12")
	t.Setenv("CAIRN_RUN_ID", "01JWB8FGXQPNR7TV0ZYM4GD1AA")
	t.Setenv("CAIRN_AGENT_ROLE", "delegator")
	t.Setenv("CAIRN_WORKFLOW_VERSION", "d44c948")
	t.Setenv("CAIRN_PARENT_SESSION_ID", "01JWB7MMXQPNR7TV0ZYM4GD0ZZ")
	t.Setenv("CAIRN_ORCHESTRATION_EXTRA", `{"mode":"fast-track"}`)

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
	clearCairn(t)
	t.Setenv("CAIRN_ORCHESTRATION_EXTRA", `{"phase":"impl","wave":2,"retry_count":3}`)

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

// TestCaptureOrchestration_AllAbsent: with no CAIRN_* vars set, type defaults to
// "manual" (solo human session) and every optional field is nil/empty.
func TestCaptureOrchestration_AllAbsent(t *testing.T) {
	clearCairn(t)

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

// TestCaptureOrchestration_OnlyOrchestratorType: only CAIRN_ORCHESTRATOR_TYPE is
// set (e.g. a minimal custom wrapper). Type is captured; all other fields nil.
func TestCaptureOrchestration_OnlyOrchestratorType(t *testing.T) {
	clearCairn(t)
	t.Setenv("CAIRN_ORCHESTRATOR_TYPE", "lattice-orchestrator")

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
