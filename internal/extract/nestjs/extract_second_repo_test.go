package nestjs

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// TestExtract_RealAwesomeNestBoilerplate runs the full extraction pipeline
// against a second real, vendored NestJS project (see
// testdata/awesome-nest-boilerplate/NOTICE.md), chosen specifically for a
// different guard style than testdata/nestjs-boilerplate/: a custom
// composite @Auth([...]) decorator (built with applyDecorators()) instead
// of separate @UseGuards()/@Roles() decorators at the call site.
//
// This fixture set previously demonstrated the composite-decorator blind
// spot (docs/decisions/0006): POST /posts is genuinely guarded via
// @Auth(), but was invisible to extraction, producing a Low-confidence
// false positive. Since ADR 0006 shipped, that false positive is gone —
// this test is now the regression check for that fix, not a
// demonstration of the gap. Expected values below were verified by hand
// against the vendored source, not assumed.
func TestExtract_RealAwesomeNestBoilerplate(t *testing.T) {
	m, outcome, err := Extract("testdata/awesome-nest-boilerplate/src")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(outcome.AllowlistedEndpoints) != 0 || len(outcome.StaleMarkers) != 0 {
		t.Errorf("vendored fixtures contain no sphinxor-allow markers, got %+v", outcome)
	}

	// 3 AuthController endpoints + 5 PostController endpoints.
	if len(m.Endpoints) != 8 {
		t.Fatalf("got %d endpoints, want 8: %v", len(m.Endpoints), endpointSummaries(m.Endpoints))
	}

	// RoleType.USER and RoleType.ADMIN, both referenced only through
	// @Auth([...]) calls — resolved now that composite expansion follows
	// @Auth() to its inner Roles(roles) call and RoleType is vendored
	// (testdata/awesome-nest-boilerplate/src/constants/role-type.ts).
	roleNames := make(map[string]bool, len(m.RoleDeclarations))
	for _, d := range m.RoleDeclarations {
		roleNames[d.Name] = true
	}
	if len(roleNames) != 2 || !roleNames["RoleType.USER"] || !roleNames["RoleType.ADMIN"] {
		t.Errorf("role declarations = %v, want exactly {RoleType.USER, RoleType.ADMIN}", roleNames)
	}
	// One reference per @Auth([...]) call site naming a role: getCurrentUser
	// (USER + ADMIN), createPost (USER), getPosts (USER) — 4 total.
	if len(m.RoleReferences) != 4 {
		t.Fatalf("got %d role references, want 4: %+v", len(m.RoleReferences), m.RoleReferences)
	}
	for _, ref := range m.RoleReferences {
		if ref.RoleDeclarationID == nil {
			t.Errorf("role reference %+v did not resolve to a declaration — composite role resolution should always resolve a qualified RoleType.X reference here", ref)
		}
	}

	endpointByPath := make(map[string]model.Endpoint, len(m.Endpoints))
	for _, e := range m.Endpoints {
		endpointByPath[string(e.HTTPMethod)+" "+e.Path] = e
	}

	// The target false positive: POST /posts now shows the guards
	// @Auth([RoleType.USER]) genuinely implies (AuthGuard, RolesGuard),
	// resolved through the composite — not "no guards detected". "Roles"
	// itself is also a real GuardApplication (the role-check application,
	// same as for a literal @Roles() call) but is excluded here the same
	// way report.go's Guards column excludes it — it's surfaced via Roles
	// references instead, checked separately below.
	createPost, ok := endpointByPath["POST /posts"]
	if !ok {
		t.Fatalf("POST /posts not found: %v", endpointByPath)
	}
	guards := nonRoleGuardNames(*m, createPost.ID)
	wantGuards := map[string]bool{"AuthGuard": true, "RolesGuard": true}
	if len(guards) != len(wantGuards) {
		t.Errorf("POST /posts guards = %v, want AuthGuard and RolesGuard (resolved via @Auth())", guards)
	}
	for _, g := range guards {
		if !wantGuards[g] {
			t.Errorf("POST /posts: unexpected guard %q", g)
		}
	}

	// GET /posts/:id carries @Auth([]) — an empty, but still genuine,
	// Roles application (composite-resolved). It must not trigger
	// empty-role (see the FromComposite exclusion in
	// internal/lint/empty_role.go), but it should still count as guarded
	// for mutating-endpoint-without-access-control's purposes (moot here
	// since GET isn't mutating, but the GuardApplication should exist).
	getSinglePost, ok := endpointByPath["GET /posts/:id"]
	if !ok {
		t.Fatalf("GET /posts/:id not found: %v", endpointByPath)
	}
	foundEmptyRolesApp := false
	for _, g := range m.GuardApplications {
		if g.EndpointID != getSinglePost.ID {
			continue
		}
		if g.GuardName == "Roles" {
			foundEmptyRolesApp = true
			if !g.FromComposite {
				t.Errorf("GET /posts/:id's Roles application should have FromComposite = true")
			}
		}
	}
	if !foundEmptyRolesApp {
		t.Errorf("GET /posts/:id: expected a Roles GuardApplication (from @Auth([])), found none")
	}

	findings := runAllRules(t, m)

	mutating := findingsByRule(findings, "mutating-endpoint-without-access-control")
	wantFlagged := map[string]bool{
		"POST /auth/login":    true, // genuinely unguarded in source
		"POST /auth/register": true, // genuinely unguarded in source
		"PUT /posts/:id":      true, // genuinely unguarded in source
		"DELETE /posts/:id":   true, // genuinely unguarded in source
		// POST /posts is deliberately absent: it's genuinely guarded via
		// @Auth([RoleType.USER]), and composite resolution (ADR 0006) now
		// sees that guard — this is the false positive that resolution
		// was built to eliminate.
	}
	if len(mutating) != len(wantFlagged) {
		t.Errorf("got %d mutating-endpoint-without-access-control findings, want %d: %+v", len(mutating), len(wantFlagged), mutating)
	}
	for _, f := range mutating {
		e := endpointByID(m.Endpoints, f.SubjectID)
		key := string(e.HTTPMethod) + " " + e.Path
		if !wantFlagged[key] {
			t.Errorf("unexpected mutating-endpoint finding for %s", key)
		}
		if f.Confidence != model.ConfidenceLow {
			t.Errorf("%s: confidence = %q, want low", key, f.Confidence)
		}
	}

	if unreferenced := findingsByRule(findings, "permission-declared-but-unreferenced"); len(unreferenced) != 0 {
		t.Errorf("expected no permission-declared-but-unreferenced findings (both RoleType members are referenced via @Auth()), got %+v", unreferenced)
	}
	if empty := findingsByRule(findings, "empty-role"); len(empty) != 0 {
		t.Errorf("expected no empty-role findings — GET /posts/:id's @Auth([]) is composite-resolved and must be excluded, got %+v", empty)
	}
}
