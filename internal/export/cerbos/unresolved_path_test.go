package cerbos

import (
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// TestTranslate_UnresolvedPathIsOmitted is ADR 0020 Amendment 1 §5's
// exporter half.
//
// The endpoint stays in the model and stays linted — that is the point
// of keeping it — but it cannot be named in a policy. A Cerbos rule's
// action comes from the route, and deriving one from the fragment that
// happened to resolve would produce a rule governing a route that does
// not exist while the real one stays ungoverned.
func TestTranslate_UnresolvedPathIsOmitted(t *testing.T) {
	m := &model.Model{
		Controllers: []model.Controller{
			{ID: "c1", Name: "AdminController"},
			{ID: "c2", Name: "ReportController"},
		},
		Endpoints: []model.Endpoint{
			{
				ID: "DELETE ?unresolved-path AdminController.wipe", HTTPMethod: model.MethodDelete,
				Path: "/wipe", HandlerName: "wipe", PathUnresolved: true, ControllerID: "c1",
			},
			{ID: "GET /api/reports", HTTPMethod: model.MethodGet, Path: "/api/reports", ControllerID: "c2"},
		},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "DELETE ?unresolved-path AdminController.wipe", GuardName: "Roles", AppliedAt: model.ScopeMethod, DeclaresRoles: true},
			{ID: "g2", EndpointID: "GET /api/reports", GuardName: "Roles", AppliedAt: model.ScopeMethod, DeclaresRoles: true},
		},
		RoleReferences: []model.RoleReference{
			{ID: "r1", GuardApplicationID: "g1", RawLiteral: "ADMIN"},
			{ID: "r2", GuardApplicationID: "g2", RawLiteral: "ADMIN"},
		},
	}

	result := Translate(m)

	// The readable endpoint is unaffected: this must not make a whole
	// project unexportable the way an unknown URL layer does, because
	// the uncertainty here is per-endpoint, not project-wide.
	if len(result.Rules) != 1 {
		t.Fatalf("got %d rule(s), want 1 — only the unresolved endpoint is omitted: %+v", len(result.Rules), result.Rules)
	}
	if result.Rules[0].Resource != ResourceKind("ReportController") {
		t.Errorf("exported resource = %q, want the readable one", result.Rules[0].Resource)
	}

	if len(result.Omissions) != 1 {
		t.Fatalf("got %d omission(s), want 1: %+v", len(result.Omissions), result.Omissions)
	}
	if got := result.Omissions[0].Reason; got != ReasonPathUnresolved {
		t.Errorf("omission reason = %q, want %q", got, ReasonPathUnresolved)
	}
	// The detail has to say the path is a fragment, or a reader will
	// take "/wipe" at face value.
	if d := result.Omissions[0].Detail; !strings.Contains(d, "could not be read") {
		t.Errorf("omission detail should explain why, got %q", d)
	}
}
