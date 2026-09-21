package cerbos

import (
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// TestAnyVerb_OmittedNotExported is ADR 0028 §2 consumer 2.
//
// ADR 0009 §2 derives a rule's action from the HTTP method, so an
// any-verb endpoint's action would be "any" — which no request carries.
// Such a rule reads as a grant and governs nothing, which is worse than
// omitting, because a reviewer sees coverage that does not exist.
// Writing all eight instead would grant verbs the handler may never have
// been meant to serve, which ADR 0009 §3 forbids.
func TestAnyVerb_OmittedNotExported(t *testing.T) {
	m := &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "ThingController"}},
		Endpoints: []model.Endpoint{{
			ID: "ANY /things", HTTPMethod: model.MethodAny, Path: "/things",
			HandlerName: "any", ControllerID: "c1",
		}},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "ANY /things", GuardName: "PreAuthorize", DeclaresRoles: true},
		},
		RoleReferences: []model.RoleReference{
			{ID: "r1", GuardApplicationID: "g1", RawLiteral: "ADMIN"},
		},
	}
	out := Translate(m)
	if len(out.Rules) != 0 {
		t.Errorf("an any-verb endpoint must not become a rule, got %+v", out.Rules)
	}
	if len(out.Omissions) != 1 {
		t.Fatalf("it must be omitted with a reason, got %+v", out.Omissions)
	}
	if out.Omissions[0].Reason != ReasonAnyVerb {
		t.Errorf("Reason = %q, want %q", out.Omissions[0].Reason, ReasonAnyVerb)
	}
	// The endpoint has a role, so it would otherwise have exported —
	// which is what makes this a real omission rather than a no-op.
	if !strings.Contains(out.Omissions[0].Detail, "every HTTP verb") {
		t.Errorf("the detail should say why, got %q", out.Omissions[0].Detail)
	}
}
