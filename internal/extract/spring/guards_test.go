package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// TestExtractControllers_SecuredArrayAndSingleValueShorthand covers
// @Secured/@RolesAllowed's two real Java annotation-array shapes —
// standard, well-documented Java syntax (not SpEL), so synthetic coverage
// is appropriate per docs/testing.md: real code can't surprise a
// well-defined array-literal parse the way it can surprise a bounded SpEL
// matcher. Neither vendored fixture uses these annotations at all
// (Pharmacy: @PreAuthorize only; blog-api: none, method security globally
// disabled).
func TestExtractControllers_SecuredArrayAndSingleValueShorthand(t *testing.T) {
	src := `
@RestController
public class ThingController {
    @Secured({"ROLE_ADMIN", "ROLE_MANAGER"})
    @PostMapping("/a")
    public void a() {}

    @Secured("ROLE_ADMIN")
    @PostMapping("/b")
    public void b() {}

    @RolesAllowed({"ADMIN", "MANAGER"})
    @PostMapping("/c")
    public void c() {}
}
`
	root, source := parseJava(t, src)
	b := newBuilder()
	extractControllers(root, source, "ThingController.java", b, nil)

	cases := []struct {
		path      string
		guardName string
		wantRoles []string
	}{
		{"/a", "Secured", []string{"ROLE_ADMIN", "ROLE_MANAGER"}},
		{"/b", "Secured", []string{"ROLE_ADMIN"}},
		{"/c", "RolesAllowed", []string{"ADMIN", "MANAGER"}},
	}
	for _, tc := range cases {
		e, ok := findEndpoint(b.model.Endpoints, model.MethodPost, tc.path)
		if !ok {
			t.Fatalf("missing endpoint POST %s", tc.path)
		}
		var app *model.GuardApplication
		for i := range b.model.GuardApplications {
			if b.model.GuardApplications[i].EndpointID == e.ID {
				app = &b.model.GuardApplications[i]
			}
		}
		if app == nil {
			t.Fatalf("POST %s: no GuardApplication found", tc.path)
		}
		if app.GuardName != tc.guardName || !app.DeclaresRoles {
			t.Errorf("POST %s: GuardApplication = %+v, want GuardName=%q DeclaresRoles=true", tc.path, app, tc.guardName)
		}
		var gotRoles []string
		for _, r := range b.model.RoleReferences {
			if r.GuardApplicationID == app.ID {
				gotRoles = append(gotRoles, r.RawLiteral)
			}
		}
		if len(gotRoles) != len(tc.wantRoles) {
			t.Fatalf("POST %s: roles = %v, want %v", tc.path, gotRoles, tc.wantRoles)
		}
		for i, want := range tc.wantRoles {
			if gotRoles[i] != want {
				t.Errorf("POST %s: role[%d] = %q, want %q", tc.path, i, gotRoles[i], want)
			}
		}
	}
}

// TestExtractControllers_IsAuthenticatedDeclaresNoRoles is the ADR 0017
// regression: @PreAuthorize("isAuthenticated()") must set DeclaresRoles:
// false (unlike every other recognized/unrecognized PreAuthorize shape),
// so it doesn't trip empty_role.go's "declares roles, zero given" check on
// an annotation that's intentionally role-less.
func TestExtractControllers_IsAuthenticatedDeclaresNoRoles(t *testing.T) {
	src := `
@RestController
public class ThingController {
    @PreAuthorize("isAuthenticated()")
    @PostMapping("/a")
    public void a() {}
}
`
	root, source := parseJava(t, src)
	b := newBuilder()
	extractControllers(root, source, "ThingController.java", b, nil)

	if len(b.model.GuardApplications) != 1 {
		t.Fatalf("got %d GuardApplications, want 1: %+v", len(b.model.GuardApplications), b.model.GuardApplications)
	}
	app := b.model.GuardApplications[0]
	if app.DeclaresRoles {
		t.Error("isAuthenticated(): DeclaresRoles should be false (docs/decisions/0017-declaresroles-excludes-isauthenticated.md)")
	}
	if len(b.model.RoleReferences) != 0 {
		t.Errorf("isAuthenticated() should produce zero RoleReferences, got %+v", b.model.RoleReferences)
	}
	if len(b.authCandidates) != 1 {
		t.Fatalf("got %d authCandidates, want 1: %+v", len(b.authCandidates), b.authCandidates)
	}

	// End to end through the real final pass, same as Extract() would run it.
	authReqs := computeAuthenticationRequirements(&b.model, b.authCandidates, b.nextID("authreq"))
	if len(authReqs) != 1 {
		t.Fatalf("got %d AuthenticationRequirements, want 1: %+v", len(authReqs), authReqs)
	}
	if authReqs[0].EndpointID != app.EndpointID {
		t.Errorf("AuthenticationRequirement.EndpointID = %v, want %v", authReqs[0].EndpointID, app.EndpointID)
	}
}

// TestExtractControllers_PermitAllStillDeclaresRoles confirms ADR 0017's
// stated boundary the other direction: permitAll() (and any unrecognized
// SpEL) keeps DeclaresRoles: true — only isAuthenticated() moves, because
// only it has a positive non-role representation to move to.
func TestExtractControllers_PermitAllStillDeclaresRoles(t *testing.T) {
	src := `
@RestController
public class ThingController {
    @PreAuthorize("permitAll()")
    @PostMapping("/a")
    public void a() {}
}
`
	root, source := parseJava(t, src)
	b := newBuilder()
	extractControllers(root, source, "ThingController.java", b, nil)

	if len(b.model.GuardApplications) != 1 {
		t.Fatalf("got %d GuardApplications, want 1", len(b.model.GuardApplications))
	}
	if !b.model.GuardApplications[0].DeclaresRoles {
		t.Error("permitAll(): DeclaresRoles should stay true (ADR 0011 §2 / ADR 0017) — still surfaces through empty-role")
	}
	if len(b.authCandidates) != 0 {
		t.Errorf("permitAll() must not become an authCandidate, got %+v", b.authCandidates)
	}
}

// TestExtractRoleDeclarations_AmbiguousNameStaysUnresolved covers ADR
// 0016's stated boundary: when more than one declaration in the project
// shares an exact name, resolution is skipped entirely rather than
// guessing which one a bare SpEL literal meant.
func TestExtractRoleDeclarations_AmbiguousNameStaysUnresolved(t *testing.T) {
	src := `
enum RoleA { ADMIN, USER }
enum RoleB { ADMIN, GUEST }
`
	root, source := parseJava(t, src)
	used := map[string]bool{"ADMIN": true, "USER": true, "GUEST": true}
	decls := extractRoleDeclarations(root, source, "Roles.java", newBuilder().nextID("role"), used)

	if len(decls) != 4 {
		t.Fatalf("got %d role declarations, want 4 (ADMIN x2, USER, GUEST): %+v", len(decls), decls)
	}
	roleByName := uniqueRoleDeclarationsByName(decls)
	if _, ok := roleByName["ADMIN"]; ok {
		t.Error("ADMIN is declared twice (RoleA and RoleB) — must stay unresolved, not silently pick one")
	}
	if _, ok := roleByName["USER"]; !ok {
		t.Error("USER is declared once — should resolve")
	}
	if _, ok := roleByName["GUEST"]; !ok {
		t.Error("GUEST is declared once — should resolve")
	}
}

// TestExtractControllers_MergedHandlerRetainsOwnGuard is the regression
// test for the ADR 0014 merge bug found while wiring guard extraction in:
// the first version of this fix skipped guard extraction entirely for a
// handler merged away by docs/decisions/0014-endpoint-identity-and-content-negotiation.md's
// same-method+path rule, using the same `continue` that correctly skips
// re-appending the Endpoint row. That's a real security false negative —
// a merged-away handler's own @PreAuthorize/@Secured/@RolesAllowed would
// silently vanish from the model, invisible to every downstream consumer
// (internal/lint, internal/export/cerbos, internal/diff) — not just a
// missing test.
//
// `first` (no guard) is encountered before `second` (guarded); per ADR
// 0014, `first` wins the Endpoint row's own HandlerName, but `second`'s
// guard must still attach to the shared Endpoint.ID. This is deliberately
// not the same claim TestExtract_BlogAPI already covers (that a merge
// produces exactly one Endpoint) — blog-api's real merged handlers all
// carry zero guards, so that test cannot catch this bug; this one
// specifically exercises the merged-away handler's own annotation.
func TestExtractControllers_MergedHandlerRetainsOwnGuard(t *testing.T) {
	src := `
@RestController
public class ThingController {
    @GetMapping(path = "/x")
    public void first() {}

    @GetMapping(path = "/x", produces = "application/xml")
    @PreAuthorize("hasRole('ADMIN')")
    public void second() {}
}
`
	root, source := parseJava(t, src)
	b := newBuilder()
	extractControllers(root, source, "ThingController.java", b, nil)

	if len(b.model.Endpoints) != 1 {
		t.Fatalf("got %d endpoints, want 1 (merged per ADR 0014): %+v", len(b.model.Endpoints), b.model.Endpoints)
	}
	e := b.model.Endpoints[0]
	if e.HandlerName != "first" {
		t.Fatalf("HandlerName = %q, want %q (first-encountered wins the Endpoint row)", e.HandlerName, "first")
	}

	var guardsOnEndpoint []model.GuardApplication
	for _, g := range b.model.GuardApplications {
		if g.EndpointID == e.ID {
			guardsOnEndpoint = append(guardsOnEndpoint, g)
		}
	}
	if len(guardsOnEndpoint) != 1 {
		t.Fatalf("got %d GuardApplications on the merged endpoint, want 1 (second's own @PreAuthorize, even though second lost the HandlerName): %+v", len(guardsOnEndpoint), guardsOnEndpoint)
	}
	if guardsOnEndpoint[0].GuardName != "PreAuthorize" {
		t.Errorf("GuardApplication = %+v, want GuardName=PreAuthorize", guardsOnEndpoint[0])
	}

	var gotRoles []string
	for _, r := range b.model.RoleReferences {
		if r.GuardApplicationID == guardsOnEndpoint[0].ID {
			gotRoles = append(gotRoles, r.RawLiteral)
		}
	}
	if len(gotRoles) != 1 || gotRoles[0] != "ADMIN" {
		t.Errorf("resolved roles = %v, want [ADMIN] — second's own guard, not silently dropped by the merge", gotRoles)
	}
}
