package diff

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// The unit tests for docs/decisions/0036-became-public-gates-ci.md.
//
// Constructed from model.Model literals rather than from source, and
// deliberately so: by the time becamePublic and becamePublicRegressions
// run, their input is already a framework-independent set of collections,
// which is docs/testing.md's own carve-out for downstream logic. The
// real-source bar is met by TestRealWorldDiff_GuardRemoved, which strips a
// class-level guard from the vendored nestjs-boilerplate fixture, and by
// the corpus regression sweep in ADR 0036's own bar.

func endpoint(id string) model.Endpoint {
	return model.Endpoint{ID: model.ID(id), HTTPMethod: model.MethodDelete, Path: "/things/{id}"}
}

func snapshot(e model.Endpoint, m *model.Model) Snapshot {
	m.Endpoints = []model.Endpoint{e}
	return Snapshot{Model: m}
}

// TestBecamePublic_ProtectionTerms is ADR 0036 §2: protection is ANY
// evidence of authorization, not a GuardApplication.
//
// The UnrecognizedAuthAnnotation row is the one that changes behaviour.
// Measured across the corpus, GuardApplication alone protects 802 of 5,530
// endpoints and all four terms protect 2,704 — the 1,902 difference is
// entirely unrecognized annotations, and seven repositories (metersphere
// 869, nacos 393, JeecgBoot 244, litemall 115, streampark 102, shenyu 100,
// inlong 79) have ZERO protection under the narrow definition. A Shiro
// annotation disappearing there was not even reported as a transition.
func TestBecamePublic_ProtectionTerms(t *testing.T) {
	e := endpoint("DELETE /things/{id}")
	for _, tc := range []struct {
		name      string
		protected func() *model.Model
	}{
		{"guard application", func() *model.Model {
			return &model.Model{GuardApplications: []model.GuardApplication{{ID: "g1", EndpointID: e.ID}}}
		}},
		{"url-layer grant", func() *model.Model {
			return &model.Model{GuardApplications: []model.GuardApplication{
				{ID: "g1", EndpointID: e.ID, AppliedAt: model.ScopeRequestMatcher},
			}}
		}},
		{"unrecognized annotation (Shiro, nacos, @PostAuthorize)", func() *model.Model {
			return &model.Model{UnrecognizedAuthAnnotations: []model.UnrecognizedAuthAnnotation{
				{ID: "u1", EndpointID: e.ID, Name: "RequiresPermissions", BoundTo: "org.apache.shiro.authz.annotation"},
			}}
		}},
		{"permission reference", func() *model.Model {
			return &model.Model{
				GuardApplications:    []model.GuardApplication{{ID: "g1", EndpointID: e.ID}},
				PermissionReferences: []model.PermissionReference{{ID: "p1", GuardApplicationID: "g1", RawLiteral: "thing:delete", Via: "@ss.hasPermi"}},
			}
		}},
		{"authentication requirement", func() *model.Model {
			return &model.Model{AuthenticationRequirements: []model.AuthenticationRequirement{{ID: "a1", EndpointID: e.ID}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := snapshot(e, tc.protected())
			head := snapshot(e, &model.Model{})

			if got := becamePublic(base.Model, head.Model); len(got) != 1 {
				t.Fatalf("losing this protection was not seen as a transition: got %+v", got)
			}
			if got := becamePublicRegressions(base, head); len(got) != 1 {
				t.Fatalf("got %d regression(s), want 1", len(got))
			}

			// And the mirror: gaining it is never a regression.
			if got := becamePublicRegressions(snapshot(e, &model.Model{}), snapshot(e, tc.protected())); len(got) != 0 {
				t.Errorf("gaining protection produced %d regression(s), want 0", len(got))
			}
		})
	}
}

// TestBecamePublic_ReadableToUnreadIsNotALoss is ADR 0036 §3.
//
// RolesUnresolved is a fact about EXTRACTION, never about the application
// (ADR 0020 Amendment 3). A gate that fired here would fail a PR for a
// refactor that changed no protection, and would fire more the less
// Sphinxor understands — the incentive exactly inverted.
func TestBecamePublic_ReadableToUnreadIsNotALoss(t *testing.T) {
	e := endpoint("DELETE /things/{id}")

	readable := func() *model.Model {
		return &model.Model{
			GuardApplications: []model.GuardApplication{{ID: "g1", EndpointID: e.ID, GuardName: "PreAuthorize", DeclaresRoles: true}},
			RoleReferences:    []model.RoleReference{{ID: "r1", GuardApplicationID: "g1", RawLiteral: "ADMIN"}},
		}
	}
	permission := func() *model.Model {
		return &model.Model{
			GuardApplications:    []model.GuardApplication{{ID: "g1", EndpointID: e.ID, GuardName: "PreAuthorize", DeclaresRoles: true, DeclaresPermissions: true}},
			PermissionReferences: []model.PermissionReference{{ID: "p1", GuardApplicationID: "g1", RawLiteral: "thing:delete", Via: "@ss.hasPermi"}},
		}
	}
	unreadable := func() *model.Model {
		return &model.Model{
			GuardApplications: []model.GuardApplication{{ID: "g1", EndpointID: e.ID, GuardName: "PreAuthorize", DeclaresRoles: true, RolesUnresolved: true}},
		}
	}

	for _, tc := range []struct {
		name     string
		from, to func() *model.Model
	}{
		{"role expression to a permission bean call", readable, permission},
		{"role expression to an unreadable bean call", readable, unreadable},
		{"unreadable back to readable", unreadable, readable},
		{"permission back to a role expression", permission, readable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, head := snapshot(e, tc.from()), snapshot(e, tc.to())
			if got := becamePublic(base.Model, head.Model); len(got) != 0 {
				t.Errorf("a change of requirement was reported as becoming public: %+v", got)
			}
			if got := becamePublicRegressions(base, head); len(got) != 0 {
				t.Errorf("a change of requirement gated CI: %+v", got)
			}
		})
	}
}

// TestBecamePublic_ContentChangeDoesNotGate is ADR 0036 §2's stated
// non-goal, pinned so it is a decision rather than an accident: protection
// is presence, never content, so a privilege WIDENING is invisible here.
// Catching it needs an ordering over roles, and ADR 0031 announces a
// RoleHierarchy precisely because Sphinxor cannot read one.
func TestBecamePublic_ContentChangeDoesNotGate(t *testing.T) {
	e := endpoint("DELETE /things/{id}")
	with := func(role string) *model.Model {
		return &model.Model{
			GuardApplications: []model.GuardApplication{{ID: "g1", EndpointID: e.ID, DeclaresRoles: true}},
			RoleReferences:    []model.RoleReference{{ID: "r1", GuardApplicationID: "g1", RawLiteral: role}},
		}
	}
	if got := becamePublicRegressions(snapshot(e, with("ADMIN")), snapshot(e, with("USER"))); len(got) != 0 {
		t.Errorf("ADMIN -> USER gated; ADR 0036 §2 states this is not caught: %+v", got)
	}
}

// TestBecamePublic_AllowlistedDoesNotGate is ADR 0036 §4 — and the
// report must still SHOW the transition. Excused is not invisible (§7).
func TestBecamePublic_AllowlistedDoesNotGate(t *testing.T) {
	e := endpoint("DELETE /things/{id}")
	base := snapshot(e, &model.Model{GuardApplications: []model.GuardApplication{{ID: "g1", EndpointID: e.ID}}})
	head := snapshot(e, &model.Model{})
	head.AllowlistedEndpoints = map[model.ID]bool{e.ID: true}

	if got := becamePublicRegressions(base, head); len(got) != 0 {
		t.Errorf("a deliberately-public, marked endpoint gated: %+v", got)
	}
	if got := becamePublic(base.Model, head.Model); len(got) != 1 {
		t.Errorf("the transition disappeared from the report because it was excused; got %+v", got)
	}

	// The base's allowlist state is irrelevant: the marker is a statement
	// about the code as it now stands.
	base.AllowlistedEndpoints = map[model.ID]bool{e.ID: true}
	head.AllowlistedEndpoints = nil
	if got := becamePublicRegressions(base, head); len(got) != 1 {
		t.Errorf("a marker removed from the head did not gate; only head state counts (§4): %+v", got)
	}
}

// TestBecamePublic_IdentityChangeDoesNotGate is ADR 0036 §5. A route that
// gains a version is a different Endpoint.ID, so it is removed-plus-added
// and "present in both" fails. The cost — a rename plus an unguard in one
// PR does not gate — is accepted there, and pinned here so it cannot be
// "fixed" into the fuzzy matching ADR 0002 and ADR 0007 §2 both rejected.
func TestBecamePublic_IdentityChangeDoesNotGate(t *testing.T) {
	before := endpoint("DELETE /things/{id}")
	after := model.Endpoint{ID: "DELETE /v2/things/{id}", HTTPMethod: model.MethodDelete, Path: "/v2/things/{id}"}

	base := snapshot(before, &model.Model{GuardApplications: []model.GuardApplication{{ID: "g1", EndpointID: before.ID}}})
	head := snapshot(after, &model.Model{})

	if got := becamePublicRegressions(base, head); len(got) != 0 {
		t.Errorf("an identity change gated as became-public: %+v", got)
	}
	r := Compare(base, head)
	if len(r.RemovedEndpoints) != 1 || len(r.AddedEndpoints) != 1 {
		t.Errorf("an identity change must still be VISIBLE as removed+added; got %d removed, %d added", len(r.RemovedEndpoints), len(r.AddedEndpoints))
	}
}

// TestBecamePublic_NewUnprotectedEndpointDoesNotGate is ADR 0036 §6, a
// decision rather than an omission: mutating-endpoint-without-access-control
// fires 1,313 times across 16 of the 20 corpus repositories, almost all
// false positives in the safe direction, so gating new endpoints would
// fail essentially every PR adding a route to any of them.
func TestBecamePublic_NewUnprotectedEndpointDoesNotGate(t *testing.T) {
	e := endpoint("DELETE /things/{id}")
	base := Snapshot{Model: &model.Model{}}
	head := snapshot(e, &model.Model{})

	if got := becamePublicRegressions(base, head); len(got) != 0 {
		t.Errorf("a brand-new unprotected endpoint gated: %+v", got)
	}
}

// TestBecamePublic_RemovedEndpointDoesNotGate: deleting an endpoint is not
// making it public. RemovedEndpoints' concern, and unchanged by ADR 0036.
func TestBecamePublic_RemovedEndpointDoesNotGate(t *testing.T) {
	e := endpoint("DELETE /things/{id}")
	base := snapshot(e, &model.Model{GuardApplications: []model.GuardApplication{{ID: "g1", EndpointID: e.ID}}})
	head := Snapshot{Model: &model.Model{}}

	if got := becamePublicRegressions(base, head); len(got) != 0 {
		t.Errorf("a deleted endpoint gated as became-public: %+v", got)
	}
}
