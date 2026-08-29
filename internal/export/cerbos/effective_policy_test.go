package cerbos

import (
	"sort"
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// This file covers the method×URL effective-policy reduction (ADR 0012
// §2). Each test is named for the real shape it mirrors from the two
// repos examined before this ADR was written (categolj/blog-api,
// Kitty-Hivens/Pharmacy) — the design was verified against real data
// before any Spring extraction code existed, and these tests lock that
// verification in as regressions, independent of when the Spring
// extractor itself lands.

// TestTranslate_SupplierControllerShape mirrors Kitty-Hivens/Pharmacy's
// SupplierController.GET exactly: method-level requires ADMIN or
// PHARMACIST, but the URL-level SecurityFilterChain rule requires ADMIN
// only. Spring AND-combines both, so the real effective policy is
// ADMIN-only -- this is the finding that drove ADR 0012 in the first
// place, and the subset relationship here means both the original
// subset-check draft and the corrected intersection design agree on the
// answer. Locked in as the baseline case neither revision may regress.
func TestTranslate_SupplierControllerShape(t *testing.T) {
	m := &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "SupplierController"}},
		Endpoints:   []model.Endpoint{{ID: "e1", HTTPMethod: model.MethodGet, Path: "/api/suppliers", ControllerID: "c1"}},
		GuardApplications: []model.GuardApplication{
			{ID: "g-method", EndpointID: "e1", GuardName: "PreAuthorize", AppliedAt: model.ScopeMethod, DeclaresRoles: true},
			{ID: "g-url", EndpointID: "e1", GuardName: "requestMatcher", AppliedAt: model.ScopeRequestMatcher, DeclaresRoles: true},
		},
		RoleReferences: []model.RoleReference{
			{ID: "r1", GuardApplicationID: "g-method", RawLiteral: "ADMIN"},
			{ID: "r2", GuardApplicationID: "g-method", RawLiteral: "PHARMACIST"},
			{ID: "r3", GuardApplicationID: "g-url", RawLiteral: "ADMIN"},
		},
	}

	result := Translate(m)

	if len(result.Rules) != 1 {
		t.Fatalf("got %d rules, want 1: %+v", len(result.Rules), result.Rules)
	}
	if got := result.Rules[0].Roles; len(got) != 1 || got[0] != "ADMIN" {
		t.Errorf("effective roles = %v, want [ADMIN] (the URL layer's stricter set)", got)
	}
	if len(result.Omissions) != 0 {
		t.Errorf("expected no omissions, got %+v", result.Omissions)
	}
}

// TestTranslate_CustomerControllerShape mirrors Pharmacy's
// CustomerController.GET (list) exactly, verified by reading the real
// file: every endpoint on this controller carries
// @PreAuthorize("hasAnyRole('ADMIN', 'PHARMACIST')"), and /api/customers/**
// matches none of SecurityConfig's explicit .requestMatchers(...) rules,
// so it falls through to the trailing .anyRequest().authenticated().
// That's the method layer holding the concrete roles and the URL layer
// being the universal, unconstraining one -- the opposite direction from
// TestTranslate_UniversalGrantIsIdentityForIntersection below (which is
// not tied to any real endpoint), and a real, non-hypothetical exercise
// of intersectGrants' identity-element handling in this direction.
//
// An earlier version of this test claimed the reverse -- that
// CustomerController has no method-level annotation at all, and the only
// requirement is the URL layer's catch-all. That was never true of the
// real file (every endpoint here has @PreAuthorize) and was caught only
// by rereading the real source, not by the test passing -- the same
// class of stale, unverified real-world claim as the ReasonNoRole detail
// text going stale after AuthenticationRequirement shipped. Fixed here
// rather than left to mislead the next reader.
func TestTranslate_CustomerControllerShape(t *testing.T) {
	m := &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "CustomerController"}},
		Endpoints:   []model.Endpoint{{ID: "e1", HTTPMethod: model.MethodGet, Path: "/api/customers", ControllerID: "c1"}},
		GuardApplications: []model.GuardApplication{
			{ID: "g-method", EndpointID: "e1", GuardName: "PreAuthorize", AppliedAt: model.ScopeMethod, DeclaresRoles: true},
		},
		RoleReferences: []model.RoleReference{
			{ID: "r1", GuardApplicationID: "g-method", RawLiteral: "ADMIN"},
			{ID: "r2", GuardApplicationID: "g-method", RawLiteral: "PHARMACIST"},
		},
		AuthenticationRequirements: []model.AuthenticationRequirement{
			{ID: "a1", EndpointID: "e1", AppliedAt: model.ScopeRequestMatcher},
		},
	}

	result := Translate(m)

	if len(result.Rules) != 1 {
		t.Fatalf("got %d rules, want 1: %+v", len(result.Rules), result.Rules)
	}
	got := result.Rules[0].Roles
	sort.Strings(got)
	if len(got) != 2 || got[0] != "ADMIN" || got[1] != "PHARMACIST" {
		t.Errorf("effective roles = %v, want [ADMIN PHARMACIST] (the method layer's concrete roles, unconstrained by the URL layer's authenticated()-only catch-all)", got)
	}
}

// TestTranslate_PartialOverlapIntersectsToSharedRole is the case the
// original subset-only draft of ADR 0012 would have discarded as
// "conflicting, can't determine": method requires ADMIN or PHARMACIST,
// URL requires PHARMACIST or MANAGER. Neither set contains the other,
// but they share PHARMACIST, and a PHARMACIST-only principal genuinely
// passes both of Spring's independent interceptors. The corrected,
// intersection-based design must recover this as an exact, zero-guessing
// answer rather than omit it.
//
// Deliberately synthetic, not a real-repo shape, and that's fine here per
// docs/testing.md's real-fixture-vs-pure-logic principle: Pharmacy's
// entire role vocabulary is exactly {ADMIN, PHARMACIST} (model/Role.java),
// so no MANAGER role -- or any genuine non-subset overlap -- exists in
// either vendored repo to exercise. intersectGrants is a pure function of
// two role-name sets with no framework-specific logic, so a hand-built
// pair of sets exercises it identically to one an extractor produced;
// chasing a third real repo just to hit this branch would test the same
// code twice for no added confidence, not close a real gap.
func TestTranslate_PartialOverlapIntersectsToSharedRole(t *testing.T) {
	m := &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "WidgetController"}},
		Endpoints:   []model.Endpoint{{ID: "e1", HTTPMethod: model.MethodPost, Path: "/api/widgets", ControllerID: "c1"}},
		GuardApplications: []model.GuardApplication{
			{ID: "g-method", EndpointID: "e1", GuardName: "PreAuthorize", AppliedAt: model.ScopeMethod, DeclaresRoles: true},
			{ID: "g-url", EndpointID: "e1", GuardName: "requestMatcher", AppliedAt: model.ScopeRequestMatcher, DeclaresRoles: true},
		},
		RoleReferences: []model.RoleReference{
			{ID: "r1", GuardApplicationID: "g-method", RawLiteral: "ADMIN"},
			{ID: "r2", GuardApplicationID: "g-method", RawLiteral: "PHARMACIST"},
			{ID: "r3", GuardApplicationID: "g-url", RawLiteral: "PHARMACIST"},
			{ID: "r4", GuardApplicationID: "g-url", RawLiteral: "MANAGER"},
		},
	}

	result := Translate(m)

	if len(result.Rules) != 1 {
		t.Fatalf("got %d rules, want 1 (a real, exact answer exists — PHARMACIST satisfies both layers), got %+v rules and %+v omissions",
			len(result.Rules), result.Rules, result.Omissions)
	}
	if got := result.Rules[0].Roles; len(got) != 1 || got[0] != "PHARMACIST" {
		t.Errorf("effective roles = %v, want [PHARMACIST] (the only role satisfying both {ADMIN,PHARMACIST} and {PHARMACIST,MANAGER})", got)
	}
}

// TestTranslate_DisjointLayersProduceNoCommonRole: method requires ADMIN,
// URL requires MANAGER -- no role satisfies both. No rule may be
// exported (no role is safely grantable), but this must be a distinct,
// specifically-worded omission (ReasonNoCommonRole), not the generic
// "guarded, no role resolved" reason, and the message must not claim the
// endpoint is unreachable -- soundness-without-completeness means a
// principal separately holding both ADMIN and MANAGER could still pass
// the real app.
//
// Deliberately synthetic, same justification as
// TestTranslate_PartialOverlapIntersectsToSharedRole above: with Pharmacy's
// two-role vocabulary and every real rule requiring ADMIN alone or
// ADMIN-or-PHARMACIST, ADMIN is always in both non-universal layers'
// sets -- an empty intersection is structurally unreachable in either
// vendored repo, not merely unobserved, and this branch is pure
// role-name-set logic a real repo can't exercise any more thoroughly
// than a hand-built pair of sets already does.
func TestTranslate_DisjointLayersProduceNoCommonRole(t *testing.T) {
	m := &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "ReportController"}},
		Endpoints:   []model.Endpoint{{ID: "e1", HTTPMethod: model.MethodGet, Path: "/api/reports", ControllerID: "c1"}},
		GuardApplications: []model.GuardApplication{
			{ID: "g-method", EndpointID: "e1", GuardName: "PreAuthorize", AppliedAt: model.ScopeMethod, DeclaresRoles: true},
			{ID: "g-url", EndpointID: "e1", GuardName: "requestMatcher", AppliedAt: model.ScopeRequestMatcher, DeclaresRoles: true},
		},
		RoleReferences: []model.RoleReference{
			{ID: "r1", GuardApplicationID: "g-method", RawLiteral: "ADMIN"},
			{ID: "r2", GuardApplicationID: "g-url", RawLiteral: "MANAGER"},
		},
	}

	result := Translate(m)

	if len(result.Rules) != 0 {
		t.Fatalf("expected no rule (no role satisfies both layers), got %+v", result.Rules)
	}
	if len(result.Omissions) != 1 || result.Omissions[0].Reason != ReasonNoCommonRole {
		t.Fatalf("omissions = %+v, want one ReasonNoCommonRole", result.Omissions)
	}
	detail := result.Omissions[0].Detail
	if strings.Contains(strings.ToLower(detail), "unreachable") {
		t.Errorf("detail must not claim the endpoint is unreachable (a multi-role principal could still pass), got: %s", detail)
	}
}

// TestTranslate_UniversalGrantIsIdentityForIntersection: method requires
// only "authenticated, any role" (no specific role), URL requires
// MANAGER specifically. The concrete side wins entirely -- "*" imposes
// no additional constraint beyond what the other layer already requires.
func TestTranslate_UniversalGrantIsIdentityForIntersection(t *testing.T) {
	m := &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "DashboardController"}},
		Endpoints:   []model.Endpoint{{ID: "e1", HTTPMethod: model.MethodGet, Path: "/api/dashboard", ControllerID: "c1"}},
		GuardApplications: []model.GuardApplication{
			{ID: "g-url", EndpointID: "e1", GuardName: "requestMatcher", AppliedAt: model.ScopeRequestMatcher, DeclaresRoles: true},
		},
		RoleReferences: []model.RoleReference{
			{ID: "r1", GuardApplicationID: "g-url", RawLiteral: "MANAGER"},
		},
		AuthenticationRequirements: []model.AuthenticationRequirement{
			{ID: "a1", EndpointID: "e1", AppliedAt: model.ScopeMethod},
		},
	}

	result := Translate(m)

	if len(result.Rules) != 1 {
		t.Fatalf("got %d rules, want 1: %+v", len(result.Rules), result.Rules)
	}
	if got := result.Rules[0].Roles; len(got) != 1 || got[0] != "MANAGER" {
		t.Errorf("effective roles = %v, want [MANAGER] (concrete URL requirement wins over method's authenticated-any-role)", got)
	}
}

// TestTranslate_VerifiedFlagSurvivesIntersection: a role that resolved to
// a known declaration on one layer but not the other should still be
// treated as verified overall in the merged answer -- one confirmed
// declaration is enough, the same rule appendUniqueGrant already applies
// within a single layer (intersectGrants: verified: g.verified ||
// other.verified).
func TestTranslate_VerifiedFlagSurvivesIntersection(t *testing.T) {
	declID := model.ID("d1")
	m := &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "InvoiceController"}},
		Endpoints:   []model.Endpoint{{ID: "e1", HTTPMethod: model.MethodGet, Path: "/api/invoices", ControllerID: "c1"}},
		GuardApplications: []model.GuardApplication{
			{ID: "g-method", EndpointID: "e1", GuardName: "PreAuthorize", AppliedAt: model.ScopeMethod, DeclaresRoles: true},
			{ID: "g-url", EndpointID: "e1", GuardName: "requestMatcher", AppliedAt: model.ScopeRequestMatcher, DeclaresRoles: true},
		},
		RoleDeclarations: []model.RoleDeclaration{
			{ID: declID, Name: "ADMIN", Kind: model.RoleDeclarationEnum},
		},
		RoleReferences: []model.RoleReference{
			{ID: "r1", GuardApplicationID: "g-method", RawLiteral: "ADMIN", RoleDeclarationID: &declID},
			{ID: "r2", GuardApplicationID: "g-url", RawLiteral: "ADMIN"},
		},
	}

	result := Translate(m)

	if len(result.Rules) != 1 {
		t.Fatalf("got %d rules, want 1: %+v", len(result.Rules), result.Rules)
	}
	if got := result.Rules[0].Roles; len(got) != 1 || got[0] != "ADMIN" {
		t.Fatalf("effective roles = %v, want [ADMIN]", got)
	}
	for _, u := range result.UnverifiedRoles {
		if u.Endpoint.ID == "e1" {
			t.Errorf("ADMIN should be treated as verified overall (confirmed on the method layer), but got flagged unverified: %+v", u)
		}
	}
}
