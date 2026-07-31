package lint

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

func TestEmptyRole(t *testing.T) {
	m := &model.Model{
		Endpoints: []model.Endpoint{
			{ID: "ep-empty", HTTPMethod: model.MethodDelete, Path: "/things/:id"},
			{ID: "ep-full", HTTPMethod: model.MethodPost, Path: "/things"},
		},
		GuardApplications: []model.GuardApplication{
			{ID: "ga-empty", EndpointID: "ep-empty", GuardName: "Roles"},
			{ID: "ga-full", EndpointID: "ep-full", GuardName: "Roles"},
			{ID: "ga-other", EndpointID: "ep-full", GuardName: "AuthGuard"},
		},
		RoleReferences: []model.RoleReference{
			{ID: "ref1", GuardApplicationID: "ga-full", RawLiteral: "RoleEnum.admin"},
		},
	}

	findings := EmptyRole{}.Check(m)

	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.SubjectID != "ep-empty" {
		t.Errorf("subject = %q, want ep-empty", f.SubjectID)
	}
	if f.Confidence != model.ConfidenceHigh {
		t.Errorf("confidence = %q, want high", f.Confidence)
	}
}

func TestEmptyRole_NonRolesGuardsIgnored(t *testing.T) {
	m := &model.Model{
		Endpoints: []model.Endpoint{
			{ID: "ep", HTTPMethod: model.MethodPost, Path: "/things"},
		},
		GuardApplications: []model.GuardApplication{
			{ID: "ga", EndpointID: "ep", GuardName: "AuthGuard"},
		},
	}

	findings := EmptyRole{}.Check(m)

	if len(findings) != 0 {
		t.Fatalf("a non-Roles guard with no references should never trigger EmptyRole, got %+v", findings)
	}
}

// TestEmptyRole_ExcludesFromComposite is the unit-level check for ADR
// 0006's exclusion: a composite decorator's roles parameter commonly
// defaults to an empty list (e.g. Auth(roles: RoleType[] = [])), meaning
// an empty resolved role set is deliberate "authenticated, no specific
// role" behavior, not the forgotten-argument smell this rule targets.
func TestEmptyRole_ExcludesFromComposite(t *testing.T) {
	m := &model.Model{
		Endpoints: []model.Endpoint{
			{ID: "ep", HTTPMethod: model.MethodPost, Path: "/things"},
		},
		GuardApplications: []model.GuardApplication{
			{ID: "ga", EndpointID: "ep", GuardName: "Roles", FromComposite: true},
		},
	}

	findings := EmptyRole{}.Check(m)

	if len(findings) != 0 {
		t.Errorf("a composite-resolved empty Roles application must not trigger empty-role, got %+v", findings)
	}
}
