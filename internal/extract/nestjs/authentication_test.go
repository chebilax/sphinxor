package nestjs

import (
	"fmt"
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

func newAuthReqCounter() func() model.ID {
	n := 0
	return func() model.ID {
		n++
		return model.ID(fmt.Sprintf("authreq-%d", n))
	}
}

func TestComputeAuthenticationRequirements_RecognizedGuardNoRole(t *testing.T) {
	m := &model.Model{
		Endpoints: []model.Endpoint{{ID: "e1"}},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "e1", GuardName: "AuthGuard"},
		},
	}

	got := computeAuthenticationRequirements(m, newAuthReqCounter())

	if len(got) != 1 || got[0].EndpointID != "e1" {
		t.Fatalf("got %+v, want one AuthenticationRequirement for e1", got)
	}
}

func TestComputeAuthenticationRequirements_UnrecognizedGuardExcluded(t *testing.T) {
	m := &model.Model{
		Endpoints: []model.Endpoint{{ID: "e1"}},
		GuardApplications: []model.GuardApplication{
			// Not in recognizedAuthenticationGuards -- must not qualify,
			// no matter how auth-guard-like the name looks. This is the
			// exact failure ADR 0010 was corrected to prevent: an
			// unrelated guard's mere presence must never become a ["*"]
			// grant downstream.
			{ID: "g1", EndpointID: "e1", GuardName: "ThrottlerGuard"},
		},
	}

	got := computeAuthenticationRequirements(m, newAuthReqCounter())

	if len(got) != 0 {
		t.Fatalf("an unrecognized guard must never produce an AuthenticationRequirement, got %+v", got)
	}
}

func TestComputeAuthenticationRequirements_ResolvedRoleExcluded(t *testing.T) {
	m := &model.Model{
		Endpoints: []model.Endpoint{{ID: "e1"}},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "e1", GuardName: "AuthGuard"},
			{ID: "g2", EndpointID: "e1", GuardName: "Roles"},
		},
		RoleReferences: []model.RoleReference{
			{ID: "r1", GuardApplicationID: "g2", RawLiteral: "admin"},
		},
	}

	got := computeAuthenticationRequirements(m, newAuthReqCounter())

	if len(got) != 0 {
		t.Fatalf("an endpoint with a resolved role already has a real grant, must not also get AuthenticationRequirement, got %+v", got)
	}
}

func TestComputeAuthenticationRequirements_LiteralEmptyRolesExcluded(t *testing.T) {
	m := &model.Model{
		Endpoints: []model.Endpoint{{ID: "e1"}},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "e1", GuardName: "AuthGuard"},
			// Literal @Roles() with zero arguments -- FromComposite false,
			// zero RoleReferences. This is exactly the shape EmptyRole
			// (internal/lint/empty_role.go) flags as a probable mistake;
			// AuthenticationRequirement must not export a grant for it.
			{ID: "g2", EndpointID: "e1", GuardName: "Roles", FromComposite: false},
		},
	}

	got := computeAuthenticationRequirements(m, newAuthReqCounter())

	if len(got) != 0 {
		t.Fatalf("a literal empty @Roles() must exclude AuthenticationRequirement (EmptyRole's territory), got %+v", got)
	}
}

func TestComputeAuthenticationRequirements_CompositeEmptyRolesStillQualifies(t *testing.T) {
	m := &model.Model{
		Endpoints: []model.Endpoint{{ID: "e1"}},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "e1", GuardName: "AuthGuard", FromComposite: true},
			// Composite-resolved empty role list (e.g. @Auth([])) -- this
			// is the composite's documented "authenticated, no specific
			// role" default, not the forgotten-argument smell EmptyRole
			// targets. Must still qualify.
			{ID: "g2", EndpointID: "e1", GuardName: "Roles", FromComposite: true},
		},
	}

	got := computeAuthenticationRequirements(m, newAuthReqCounter())

	if len(got) != 1 {
		t.Fatalf("a composite-resolved empty role list must still qualify, got %+v", got)
	}
}

func TestComputeAuthenticationRequirements_NoGuardAtAll(t *testing.T) {
	m := &model.Model{
		Endpoints: []model.Endpoint{{ID: "e1"}},
	}

	got := computeAuthenticationRequirements(m, newAuthReqCounter())

	if len(got) != 0 {
		t.Fatalf("an endpoint with no guard at all must not get an AuthenticationRequirement, got %+v", got)
	}
}
