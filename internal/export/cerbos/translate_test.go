package cerbos

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

func ptr(id model.ID) *model.ID { return &id }

func TestTranslate_ConfirmedEndpointBecomesRule(t *testing.T) {
	m := &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "UsersController"}},
		Endpoints:   []model.Endpoint{{ID: "e1", HTTPMethod: model.MethodGet, Path: "/users", ControllerID: "c1"}},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "e1", GuardName: "Roles"},
		},
		RoleDeclarations: []model.RoleDeclaration{{ID: "r1", Name: "RoleEnum.admin"}},
		RoleReferences: []model.RoleReference{
			{ID: "ref1", GuardApplicationID: "g1", RoleDeclarationID: ptr("r1"), RawLiteral: "RoleEnum.admin"},
		},
	}

	result := Translate(m)

	if len(result.Rules) != 1 {
		t.Fatalf("got %d rules, want 1: %+v", len(result.Rules), result.Rules)
	}
	rule := result.Rules[0]
	if rule.Resource != "users" || rule.Action != "get" {
		t.Errorf("rule = %+v, want resource=users action=get", rule)
	}
	if len(rule.Roles) != 1 || rule.Roles[0] != "RoleEnum.admin" {
		t.Errorf("roles = %v, want [RoleEnum.admin]", rule.Roles)
	}
	if len(result.Omissions) != 0 {
		t.Errorf("expected no omissions, got %+v", result.Omissions)
	}
	if len(result.UnverifiedRoles) != 0 {
		t.Errorf("expected no unverified roles (RoleDeclarationID was set), got %+v", result.UnverifiedRoles)
	}
}

func TestTranslate_UnguardedEndpointOmitted(t *testing.T) {
	m := &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "UsersController"}},
		Endpoints:   []model.Endpoint{{ID: "e1", HTTPMethod: model.MethodDelete, Path: "/users/:id", ControllerID: "c1"}},
	}

	result := Translate(m)

	if len(result.Rules) != 0 {
		t.Fatalf("expected no rules, got %+v", result.Rules)
	}
	if len(result.Omissions) != 1 || result.Omissions[0].Reason != ReasonNoGuard {
		t.Fatalf("omissions = %+v, want one ReasonNoGuard", result.Omissions)
	}
}

func TestTranslate_GuardedButNoRoleOmitted(t *testing.T) {
	m := &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "AuthController"}},
		Endpoints:   []model.Endpoint{{ID: "e1", HTTPMethod: model.MethodGet, Path: "/auth/me", ControllerID: "c1"}},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "e1", GuardName: "AuthGuard"},
		},
		// No RoleReferences at all -- guarded, but no specific role.
	}

	result := Translate(m)

	if len(result.Rules) != 0 {
		t.Fatalf("expected no rules, got %+v", result.Rules)
	}
	if len(result.Omissions) != 1 || result.Omissions[0].Reason != ReasonNoRole {
		t.Fatalf("omissions = %+v, want one ReasonNoRole", result.Omissions)
	}
}

func TestTranslate_SameActionSameRoleMerges(t *testing.T) {
	m := &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "UsersController"}},
		Endpoints: []model.Endpoint{
			{ID: "e1", HTTPMethod: model.MethodGet, Path: "/users", ControllerID: "c1"},
			{ID: "e2", HTTPMethod: model.MethodGet, Path: "/users/:id", ControllerID: "c1"},
		},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "e1", GuardName: "Roles"},
			{ID: "g2", EndpointID: "e2", GuardName: "Roles"},
		},
		RoleReferences: []model.RoleReference{
			{ID: "ref1", GuardApplicationID: "g1", RawLiteral: "admin"},
			{ID: "ref2", GuardApplicationID: "g2", RawLiteral: "admin"},
		},
	}

	result := Translate(m)

	if len(result.Rules) != 1 {
		t.Fatalf("got %d rules, want 1 (identical role sets should merge): %+v", len(result.Rules), result.Rules)
	}
	if len(result.Rules[0].Endpoints) != 2 {
		t.Errorf("merged rule should list both endpoints, got %+v", result.Rules[0].Endpoints)
	}
	if len(result.Omissions) != 0 {
		t.Errorf("expected no omissions, got %+v", result.Omissions)
	}
}

// TestTranslate_DifferingRolesCollide is the case this design has to get
// right: two endpoints on the same controller sharing an HTTP method
// (real example: awesome-nest-boilerplate's PostController has GET /posts
// requiring RoleType.USER and GET /posts/:id requiring no specific role).
// Neither may be exported, since the controller+method mapping (ADR 0009
// §2) has no path component to tell them apart, and merging would grant
// the wrong permission to whichever endpoint didn't actually have it.
func TestTranslate_DifferingRolesCollide(t *testing.T) {
	m := &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "PostController"}},
		Endpoints: []model.Endpoint{
			{ID: "e1", HTTPMethod: model.MethodGet, Path: "/posts", ControllerID: "c1"},
			{ID: "e2", HTTPMethod: model.MethodGet, Path: "/posts/:id", ControllerID: "c1"},
		},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "e1", GuardName: "Roles"},
			{ID: "g2", EndpointID: "e2", GuardName: "Roles"},
		},
		RoleReferences: []model.RoleReference{
			{ID: "ref1", GuardApplicationID: "g1", RawLiteral: "RoleType.USER"},
			{ID: "ref2", GuardApplicationID: "g2", RawLiteral: "RoleType.ADMIN"},
		},
	}

	result := Translate(m)

	if len(result.Rules) != 0 {
		t.Fatalf("expected no rules from a colliding action, got %+v", result.Rules)
	}
	if len(result.Omissions) != 2 {
		t.Fatalf("expected both colliding endpoints omitted, got %+v", result.Omissions)
	}
	for _, o := range result.Omissions {
		if o.Reason != ReasonActionCollision {
			t.Errorf("omission reason = %q, want %q", o.Reason, ReasonActionCollision)
		}
	}
}

// TestTranslate_ConfirmedSiblingWithUnconfirmedCollide is the bug this
// design was rewritten to close: one endpoint confirmed with a role, its
// action-sharing sibling with NO guard at all (or guarded with no role).
// A naive implementation that only compares "confirmed" endpoints against
// each other would emit a Rule from the confirmed one alone and silently
// let it also govern the unconfirmed sibling once matched by resource+
// action in a real Cerbos deployment.
func TestTranslate_ConfirmedSiblingWithUnconfirmedCollide(t *testing.T) {
	m := &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "PostController"}},
		Endpoints: []model.Endpoint{
			{ID: "e1", HTTPMethod: model.MethodGet, Path: "/posts", ControllerID: "c1"},
			{ID: "e2", HTTPMethod: model.MethodGet, Path: "/posts/:id", ControllerID: "c1"},
		},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "e1", GuardName: "Roles"},
		},
		RoleReferences: []model.RoleReference{
			{ID: "ref1", GuardApplicationID: "g1", RawLiteral: "RoleType.USER"},
		},
		// e2 has no GuardApplication at all.
	}

	result := Translate(m)

	if len(result.Rules) != 0 {
		t.Fatalf("a confirmed endpoint must not produce a Rule when an action-sharing sibling has no confirmed role, got %+v", result.Rules)
	}
	if len(result.Omissions) != 2 {
		t.Fatalf("expected both endpoints omitted (collision), got %+v", result.Omissions)
	}
	for _, o := range result.Omissions {
		if o.Reason != ReasonActionCollision {
			t.Errorf("omission reason = %q, want %q for endpoint %s", o.Reason, ReasonActionCollision, o.Endpoint.Path)
		}
	}
}

func TestTranslate_UnresolvedRoleReferenceStillExportsButFlagged(t *testing.T) {
	m := &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "ThingsController"}},
		Endpoints:   []model.Endpoint{{ID: "e1", HTTPMethod: model.MethodDelete, Path: "/things/:id", ControllerID: "c1"}},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "e1", GuardName: "Roles"},
		},
		RoleReferences: []model.RoleReference{
			// RoleDeclarationID nil: bare string literal, no enum/const backing it.
			{ID: "ref1", GuardApplicationID: "g1", RoleDeclarationID: nil, RawLiteral: "admin"},
		},
	}

	result := Translate(m)

	if len(result.Rules) != 1 || len(result.Rules[0].Roles) != 1 || result.Rules[0].Roles[0] != "admin" {
		t.Fatalf("expected one rule granting \"admin\" despite being unresolved, got %+v", result.Rules)
	}
	if len(result.UnverifiedRoles) != 1 || result.UnverifiedRoles[0].Role != "admin" {
		t.Fatalf("expected the unresolved role flagged, got %+v", result.UnverifiedRoles)
	}
}

func TestTranslate_AllowlistedEndpointStillOmittedIfUnguarded(t *testing.T) {
	// sphinxor-allow only suppresses Sphinxor's own finding — it must have
	// no bearing on what the exporter emits (ADR 0009 §3). Findings aren't
	// even part of Translate's input, so there's nothing for an allowlist
	// marker to influence here; this test exists to document that
	// guarantee structurally, not just assert it in prose.
	m := &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "HealthController"}},
		Endpoints:   []model.Endpoint{{ID: "e1", HTTPMethod: model.MethodGet, Path: "/health", ControllerID: "c1"}},
	}

	result := Translate(m)

	if len(result.Rules) != 0 {
		t.Fatalf("an unguarded endpoint must never become a Rule, allowlisted or not, got %+v", result.Rules)
	}
	if len(result.Omissions) != 1 || result.Omissions[0].Reason != ReasonNoGuard {
		t.Fatalf("expected ReasonNoGuard, got %+v", result.Omissions)
	}
}

func TestResourceKind(t *testing.T) {
	cases := map[string]string{
		"UsersController":       "users",
		"PostController":        "post",
		"AuthController":        "auth",
		"UserProfileController": "user_profile",
		"Controller":            "controller",
	}
	for in, want := range cases {
		if got := ResourceKind(in); got != want {
			t.Errorf("ResourceKind(%q) = %q, want %q", in, got, want)
		}
	}
}
