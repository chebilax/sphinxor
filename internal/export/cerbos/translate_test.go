package cerbos

import (
	"strings"
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

// TestTranslate_DifferingRolesSplitByRoute: two endpoints on one
// controller and verb with different roles. Merging would grant the wrong
// role to one of them; before ADR 0044 both were omitted as a collision.
// Now each becomes its own rule, conditioned on its own route, so neither
// rule can govern the other endpoint.
func TestTranslate_DifferingRolesSplitByRoute(t *testing.T) {
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

	if len(result.Omissions) != 0 || len(result.Rules) != 2 {
		t.Fatalf("want two route-conditioned rules and no omission, got rules %+v omissions %+v", result.Rules, result.Omissions)
	}
	want := map[string]string{"/posts": "RoleType.USER", "/posts/:id": "RoleType.ADMIN"}
	for _, r := range result.Rules {
		if r.Route == "" || r.Condition == nil || r.Condition.Expr != `"`+r.Route+`" == R.attr.route` {
			t.Errorf("rule %+v is not conditioned on its own route", r)
		}
		if len(r.Roles) != 1 || r.Roles[0] != want[r.Route] {
			t.Errorf("route %s roles %v, want [%s]", r.Route, r.Roles, want[r.Route])
		}
	}
	if len(result.RoutedActions) != 1 || result.RouteContract == "" {
		t.Errorf("RoutedActions = %v, contract set = %v", result.RoutedActions, result.RouteContract != "")
	}
}

// TestTranslate_ConfirmedSiblingWithUnconfirmed is the bug the collision
// design closed: a rule from the confirmed endpoint alone, keyed only by
// resource and action, would also govern the unguarded sibling in Cerbos.
// ADR 0044 keeps that closed a different way: the rule is conditioned on
// the confirmed endpoint's route, so it cannot match the sibling, and the
// sibling gets its true reason, no-guard, instead of "collision".
func TestTranslate_ConfirmedSiblingWithUnconfirmed(t *testing.T) {
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

	if len(result.Rules) != 1 || result.Rules[0].Route != "/posts" || result.Rules[0].Condition == nil {
		t.Fatalf("want one rule conditioned on /posts, got %+v", result.Rules)
	}
	if len(result.Omissions) != 1 || result.Omissions[0].Endpoint.Path != "/posts/:id" || result.Omissions[0].Reason != ReasonNoGuard {
		t.Errorf("want /posts/:id omitted as no-guard, got %+v", result.Omissions)
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

// TestTranslate_RoleHierarchyCaveat is ADR 0038 Stage 1's export line. The
// hierarchy is not applied, so the exported rules are narrower than the
// application: the report says so. The rules themselves are unchanged, and
// a project with no hierarchy gets no caveat, which keeps its JSON exactly
// as it was before the field existed.
func TestTranslate_RoleHierarchyCaveat(t *testing.T) {
	m := &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "UsersController"}},
		Endpoints:   []model.Endpoint{{ID: "e1", HTTPMethod: model.MethodGet, Path: "/users", ControllerID: "c1"}},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "e1", GuardName: "PreAuthorize"},
		},
		RoleReferences: []model.RoleReference{{ID: "ref1", GuardApplicationID: "g1", RawLiteral: "ROLE_USER"}},
	}
	if c := Translate(m).Caveats; c != nil {
		t.Fatalf("no hierarchy, want no caveat, got %v", c)
	}

	m.RoleHierarchy = model.RoleHierarchyStatus{Found: true, DeclaredIn: []string{"SecurityConfig"}, Read: true,
		Edges: []model.RoleHierarchyEdge{{Higher: "ROLE_ADMIN", Lower: "ROLE_USER"}}}
	result := Translate(m)
	if len(result.Caveats) != 1 || !strings.Contains(result.Caveats[0], "does not apply") ||
		!strings.Contains(result.Caveats[0], "SecurityConfig") {
		t.Errorf("caveats = %v, want one naming SecurityConfig and saying the hierarchy is not applied", result.Caveats)
	}
	if len(result.Rules) != 1 || len(result.Rules[0].Roles) != 1 || result.Rules[0].Roles[0] != "ROLE_USER" {
		t.Errorf("rules = %+v, want ROLE_USER alone — Stage 1 exports no implied role", result.Rules)
	}
}
