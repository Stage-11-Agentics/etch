package capture

import (
	"reflect"
	"testing"
)

func TestDetectTicketID(t *testing.T) {
	cases := []struct {
		name, branch, tab, wantID, wantSrc string
	}{
		{"branch ticket lowercase", "c11-143", "", "C11-143", "branch"},
		{"branch ticket in path", "feat/C11-144-stable-addr", "", "C11-144", "branch"},
		{"branch FT key", "ft-481", "", "FT-481", "branch"},
		{"branch wins over tab", "etch-52", "C11-143 :: Delegator", "ETCH-52", "branch"},
		{"tab fallback", "main", "C11-143 :: Delegator", "C11-143", "c11_tab_title"},
		{"version branch is not a ticket", "chore/bump-0.01.002", "", "", ""},
		{"plain branch no ticket", "docs/c11-audit-followups", "", "", ""},
		{"nothing", "main", "Some Title", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, src := DetectTicketID(c.branch, c.tab)
			if id != c.wantID || src != c.wantSrc {
				t.Errorf("DetectTicketID(%q,%q) = (%q,%q), want (%q,%q)", c.branch, c.tab, id, src, c.wantID, c.wantSrc)
			}
		})
	}
}

func TestDetectRole(t *testing.T) {
	if r := DetectRole("C11-143 :: Delegator", nil); r != "delegator" {
		t.Errorf("tab role: got %q want delegator", r)
	}
	// Lineage is [ancestor ... current]; the current (last) role wins.
	if r := DetectRole("", []string{"Orchestrator", "Reviewer"}); r != "reviewer" {
		t.Errorf("lineage role: got %q want reviewer (most-current)", r)
	}
	if r := DetectRole("Just a title", nil); r != "" {
		t.Errorf("no role: got %q want empty", r)
	}
}

func TestEnrichOrchestrationFillsWhenUnset(t *testing.T) {
	o := &Orchestration{Type: "manual", Extra: map[string]any{}}
	EnrichOrchestration(o, "c11-143", "C11-143 :: Reviewer", []string{"Orchestrator", "C11-143 :: Reviewer"})

	if o.TicketID == nil || *o.TicketID != "C11-143" {
		t.Errorf("ticket_id: got %v want C11-143", o.TicketID)
	}
	if o.Role == nil || *o.Role != "reviewer" {
		t.Errorf("role: got %v want reviewer", o.Role)
	}
	sources, ok := o.Extra["_sources"].(map[string]string)
	if !ok {
		t.Fatalf("_sources missing or wrong type: %T", o.Extra["_sources"])
	}
	want := map[string]string{"ticket_id": "branch", "role": "c11_tab_title"}
	if !reflect.DeepEqual(sources, want) {
		t.Errorf("_sources = %v, want %v", sources, want)
	}
}

func TestEnrichOrchestrationNeverOverridesExplicit(t *testing.T) {
	explicitTicket := "EXPLICIT-9"
	explicitRole := "implementer"
	o := &Orchestration{
		Type:     "lattice-orchestrator",
		TicketID: &explicitTicket,
		Role:     &explicitRole,
		Extra:    map[string]any{},
	}
	EnrichOrchestration(o, "c11-143", "C11-143 :: Delegator", nil)

	if *o.TicketID != "EXPLICIT-9" {
		t.Errorf("explicit ticket overridden: %q", *o.TicketID)
	}
	if *o.Role != "implementer" {
		t.Errorf("explicit role overridden: %q", *o.Role)
	}
	// Nothing was inferred, so no _sources should be written.
	if _, ok := o.Extra["_sources"]; ok {
		t.Errorf("_sources written despite all-explicit fields: %v", o.Extra["_sources"])
	}
}

func TestCaptureOrchestrationMetaNamespace(t *testing.T) {
	clearEtchVars(t)
	t.Setenv("ETCH_META_wave", "2")
	t.Setenv("ETCH_META_delegator_index", "3")
	t.Setenv("ETCH_META_Eval_Gate", "passed") // key lowercased

	o := CaptureOrchestration()
	if o.Extra["wave"] != "2" {
		t.Errorf("ETCH_META_wave: got %v", o.Extra["wave"])
	}
	if o.Extra["delegator_index"] != "3" {
		t.Errorf("ETCH_META_delegator_index: got %v", o.Extra["delegator_index"])
	}
	if o.Extra["eval_gate"] != "passed" {
		t.Errorf("ETCH_META_Eval_Gate (lowercased key): got %v", o.Extra["eval_gate"])
	}
}

func TestCaptureOrchestrationMetaOverlaysExtraBlob(t *testing.T) {
	clearEtchVars(t)
	t.Setenv("ETCH_ORCHESTRATION_EXTRA", `{"phase":"impl","wave":"1"}`)
	t.Setenv("ETCH_META_wave", "2")

	o := CaptureOrchestration()
	if o.Extra["phase"] != "impl" {
		t.Errorf("blob phase lost: %v", o.Extra["phase"])
	}
	if o.Extra["wave"] != "2" {
		t.Errorf("ETCH_META_ should overlay blob for same key: got %v want 2", o.Extra["wave"])
	}
}

// clearEtchVars neutralizes ETCH_* vars that could leak from the ambient
// environment, so these tests are hermetic. t.Setenv auto-restores after the
// test; the capture code treats empty values as unset.
func clearEtchVars(t *testing.T) {
	t.Helper()
	for _, v := range []string{
		"ETCH_ORCHESTRATOR_TYPE", "ETCH_DISPATCH_METHOD", "ETCH_TICKET_ID",
		"ETCH_RUN_ID", "ETCH_AGENT_ROLE", "ETCH_WORKFLOW_VERSION",
		"ETCH_ORCHESTRATION_EXTRA",
	} {
		t.Setenv(v, "")
	}
}
