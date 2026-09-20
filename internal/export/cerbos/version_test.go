package cerbos

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/extract/nestjs"
	"github.com/chebilax/sphinxor/internal/model"
)

// TestExport_UnresolvedVersionIsOmitted is ADR 0020 Amendment 2 §7's
// exporter rule, against the vendored cal.com pair: both controllers
// declare a version by constant reference, so neither endpoint can say
// which route a policy would govern.
func TestExport_UnresolvedVersionIsOmitted(t *testing.T) {
	m, _, err := nestjs.Extract("../../extract/nestjs/testdata/cal.com")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	res := Translate(m)

	if len(res.Rules) != 0 {
		t.Errorf("got %d exported rules, want 0 — every endpoint here declares an unreadable version: %+v", len(res.Rules), res.Rules)
	}

	omitted := 0
	for _, o := range res.Omissions {
		if o.Reason == ReasonVersionUnresolved {
			omitted++
		}
	}
	if omitted == 0 {
		t.Fatalf("no endpoint omitted for an unresolved version; omissions = %+v", res.Omissions)
	}
	for _, e := range m.Endpoints {
		if !e.VersionUnresolved {
			t.Errorf("%s %s: expected every cal.com endpoint to carry an unreadable version", e.HTTPMethod, e.Path)
		}
	}
}

// TestExport_ReadableVersionStillExports pins the narrowing recorded in
// §7's "Correction" note. Omitting every *version-bearing* endpoint — the
// clause as originally accepted — emptied this fixture's export entirely,
// because brocoders/nestjs-boilerplate declares version '1' on every
// controller. A readable version names nothing in a Cerbos policy
// (resource comes from the controller class, action from the HTTP
// method), so it must not cost the project its export.
func TestExport_ReadableVersionStillExports(t *testing.T) {
	m, _, err := nestjs.Extract("../../extract/nestjs/testdata/nestjs-boilerplate/src")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	versioned := 0
	for _, e := range m.Endpoints {
		if e.Version != "" && !e.VersionUnresolved {
			versioned++
		}
	}
	if versioned == 0 {
		t.Fatal("fixture no longer declares a readable version — this test has lost its subject")
	}

	res := Translate(m)
	if len(res.Rules) == 0 {
		t.Fatalf("readable-version endpoints exported nothing; omissions = %+v", res.Omissions)
	}
	for _, o := range res.Omissions {
		if o.Reason == ReasonVersionUnresolved {
			t.Errorf("readable version omitted as unresolved: %s %s", o.Endpoint.HTTPMethod, o.Endpoint.Path)
		}
	}

	// The 'users' resource is the one with a uniform class-level
	// @Roles(RoleEnum.admin) guard, so it is the concrete thing the
	// broader clause destroyed.
	users := 0
	for _, r := range res.Rules {
		if r.Resource == "users" {
			users++
		}
	}
	if users != 4 {
		t.Errorf("got %d 'users' rules, want 4 (post, get, patch, delete): %+v", users, res.Rules)
	}
}

// TestExport_SameControllerVersionCollisionAlreadyHandled documents why
// §7's exporter clause did not need to be broader: two versioned handlers
// sharing a controller and an HTTP method land in one (resource, action)
// group, where ReasonActionCollision already omits them if their grants
// disagree — the pre-existing ADR 0009 machinery, not new behavior.
func TestExport_SameControllerVersionCollisionAlreadyHandled(t *testing.T) {
	m := &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "ProductController"}},
		Endpoints: []model.Endpoint{
			{ID: "a", HTTPMethod: model.MethodGet, Path: "/products", Version: "1.0", ControllerID: "c1", HandlerName: "v1"},
			{ID: "b", HTTPMethod: model.MethodGet, Path: "/products", Version: "2.0", ControllerID: "c1", HandlerName: "v2"},
		},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "a", GuardName: "Roles", AppliedAt: model.ScopeMethod, DeclaresRoles: true},
		},
		RoleReferences: []model.RoleReference{
			{ID: "r1", GuardApplicationID: "g1", RawLiteral: "ADMIN"},
		},
	}

	res := Translate(m)
	if len(res.Rules) != 0 {
		t.Errorf("got %d rules, want 0 — the two versions disagree on grants: %+v", len(res.Rules), res.Rules)
	}
	for _, o := range res.Omissions {
		if o.Reason != ReasonActionCollision {
			t.Errorf("omission reason = %q, want %q", o.Reason, ReasonActionCollision)
		}
	}
	if len(res.Omissions) != 2 {
		t.Errorf("got %d omissions, want 2 (both sides of the collision)", len(res.Omissions))
	}
}
