package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/export/cerbos"
	"github.com/chebilax/sphinxor/internal/model"
)

// TestExtract_Pharmacy_SecurityFilterChain checks URL-layer extraction
// against Pharmacy's real authorizeHttpRequests chain
// (testdata/Pharmacy/NOTICE.md) — every rule, in its real source order,
// hand-verified against SecurityConfig.java, per docs/testing.md.
func TestExtract_Pharmacy_SecurityFilterChain(t *testing.T) {
	m, err := Extract("testdata/Pharmacy/backend/src/main/java")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	urlGuardsByEndpoint := make(map[model.ID][]model.GuardApplication, len(m.GuardApplications))
	for _, g := range m.GuardApplications {
		if g.AppliedAt == model.ScopeRequestMatcher {
			urlGuardsByEndpoint[g.EndpointID] = append(urlGuardsByEndpoint[g.EndpointID], g)
		}
	}
	roleRefsByGuardApp := make(map[model.ID][]model.RoleReference, len(m.RoleReferences))
	for _, r := range m.RoleReferences {
		roleRefsByGuardApp[r.GuardApplicationID] = append(roleRefsByGuardApp[r.GuardApplicationID], r)
	}
	urlAuthByEndpoint := make(map[model.ID]bool, len(m.AuthenticationRequirements))
	for _, a := range m.AuthenticationRequirements {
		if a.AppliedAt == model.ScopeRequestMatcher {
			urlAuthByEndpoint[a.EndpointID] = true
		}
	}

	// SupplierController: /api/suppliers/** -> hasRole("ADMIN") — the real
	// SecurityConfig.java rule. Both endpoints match it (the pattern has
	// no HTTP-method scoping).
	for _, path := range []string{"/api/suppliers"} {
		for _, method := range []model.HTTPMethod{model.MethodPost, model.MethodGet} {
			e, ok := findEndpoint(m.Endpoints, method, path)
			if !ok {
				t.Fatalf("missing endpoint %s %s", method, path)
			}
			apps := urlGuardsByEndpoint[e.ID]
			if len(apps) != 1 {
				t.Fatalf("%s %s: got %d URL-layer GuardApplications, want 1: %+v", method, path, len(apps), apps)
			}
			g := apps[0]
			if g.GuardName != "requestMatcher" || !g.DeclaresRoles {
				t.Errorf("%s %s: %+v, want GuardName=requestMatcher DeclaresRoles=true", method, path, g)
			}
			refs := roleRefsByGuardApp[g.ID]
			if len(refs) != 1 || refs[0].RawLiteral != "ADMIN" {
				t.Errorf("%s %s: URL-layer roles = %+v, want [ADMIN]", method, path, refs)
			}
		}
	}

	// CustomerController: /api/customers/** matches no explicit rule, so
	// all 5 endpoints fall through to the trailing
	// .anyRequest().authenticated() — a real URL-layer
	// AuthenticationRequirement, not a role.
	customerEndpoints := []struct {
		method model.HTTPMethod
		path   string
	}{
		{model.MethodPost, "/api/customers"},
		{model.MethodPut, "/api/customers/{id}"},
		{model.MethodGet, "/api/customers/{id}"},
		{model.MethodGet, "/api/customers"},
		{model.MethodDelete, "/api/customers/{id}"},
	}
	for _, ce := range customerEndpoints {
		e, ok := findEndpoint(m.Endpoints, ce.method, ce.path)
		if !ok {
			t.Fatalf("missing endpoint %s %s", ce.method, ce.path)
		}
		if !urlAuthByEndpoint[e.ID] {
			t.Errorf("%s %s: expected a URL-layer AuthenticationRequirement (falls through to anyRequest().authenticated()), got none", ce.method, ce.path)
		}
		if apps := urlGuardsByEndpoint[e.ID]; len(apps) != 0 {
			t.Errorf("%s %s: unexpected URL-layer GuardApplications: %+v", ce.method, ce.path, apps)
		}
	}

	// AuthController.login matches /auth/** -> permitAll(): contributes
	// nothing at all (ADR 0012 §1), not authentication, not a role.
	login, ok := findEndpoint(m.Endpoints, model.MethodPost, "/auth/login")
	if !ok {
		t.Fatal("missing endpoint POST /auth/login")
	}
	if apps := urlGuardsByEndpoint[login.ID]; len(apps) != 0 {
		t.Errorf("POST /auth/login: unexpected URL-layer GuardApplications: %+v", apps)
	}
	if urlAuthByEndpoint[login.ID] {
		t.Error("POST /auth/login: permitAll() must not become an AuthenticationRequirement")
	}
}

// TestTranslate_Pharmacy_SupplierControllerEffectivePolicy runs the real
// Pharmacy-extracted model through the already-merged
// internal/export/cerbos.Translate, end to end: real extraction feeding
// real, already-shipped intersection logic. This is the exact case that
// drove ADR 0012 in the first place (SupplierController.GET: method layer
// allows ADMIN or PHARMACIST, URL layer restricts to ADMIN, real effective
// policy is ADMIN-only) — and the case ADR 0011/0012's "done" bar named
// explicitly: not just "it compiles," the real effective policy verified
// against the real fixture.
func TestTranslate_Pharmacy_SupplierControllerEffectivePolicy(t *testing.T) {
	m, err := Extract("testdata/Pharmacy/backend/src/main/java")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	result := cerbos.Translate(m)

	var rule *cerbos.Rule
	for i := range result.Rules {
		if result.Rules[i].Resource == "supplier" && result.Rules[i].Action == "get" {
			rule = &result.Rules[i]
		}
	}
	if rule == nil {
		t.Fatalf("no Rule for supplier/get; rules=%+v omissions=%+v", result.Rules, result.Omissions)
	}
	if len(rule.Roles) != 1 || rule.Roles[0] != "ADMIN" {
		t.Errorf("SupplierController.GET effective policy = %v, want [ADMIN] (method {ADMIN,PHARMACIST} ∩ URL {ADMIN})", rule.Roles)
	}

	// CustomerController's identity-element case: method-layer concrete
	// roles ∩ URL-layer universal (*) = the method layer's roles,
	// unchanged. GET /api/customers (the "getAll" endpoint) requires
	// ADMIN or PHARMACIST at the method layer and nothing more specific
	// at the URL layer.
	var customerRule *cerbos.Rule
	for i := range result.Rules {
		if result.Rules[i].Resource == "customer" && result.Rules[i].Action == "get" {
			customerRule = &result.Rules[i]
		}
	}
	if customerRule == nil {
		t.Fatalf("no Rule for customer/get; rules=%+v omissions=%+v", result.Rules, result.Omissions)
	}
	gotRoles := append([]string(nil), customerRule.Roles...)
	if len(gotRoles) != 2 || gotRoles[0] != "ADMIN" || gotRoles[1] != "PHARMACIST" {
		t.Errorf("CustomerController.GET effective policy = %v, want [ADMIN PHARMACIST] (method concrete unconstrained by URL-layer authenticated()-only)", gotRoles)
	}
}

// TestExtract_BlogAPI_SecurityFilterChain checks URL-layer extraction
// against blog-api's real, mixed authorizeHttpRequests chain
// (testdata/blog-api/NOTICE.md): recognized .hasAuthority(...) rules
// sitting beside unrecognized .access(AuthorizationManager) rules in the
// same chain, hand-verified against SecurityConfig.java.
func TestExtract_BlogAPI_SecurityFilterChain(t *testing.T) {
	m, err := Extract("testdata/blog-api/src/main/java")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// POST /admin/import matches .requestMatchers("/admin/import").hasAuthority("entry:import")
	// — recognized, but "entry:import" is a real, non-role-vocabulary
	// authority string with no matching RoleDeclaration anywhere in
	// blog-api, so it's a real grant that correctly stays unresolved.
	imp, ok := findEndpoint(m.Endpoints, model.MethodPost, "/admin/import")
	if !ok {
		t.Fatal("missing endpoint POST /admin/import")
	}
	var impGuard *model.GuardApplication
	for i := range m.GuardApplications {
		if m.GuardApplications[i].EndpointID == imp.ID {
			impGuard = &m.GuardApplications[i]
		}
	}
	if impGuard == nil {
		t.Fatal("POST /admin/import: expected a URL-layer GuardApplication")
	}
	var impRoles []model.RoleReference
	for _, r := range m.RoleReferences {
		if r.GuardApplicationID == impGuard.ID {
			impRoles = append(impRoles, r)
		}
	}
	if len(impRoles) != 1 || impRoles[0].RawLiteral != "entry:import" {
		t.Fatalf("POST /admin/import: roles = %+v, want [entry:import]", impRoles)
	}
	if impRoles[0].RoleDeclarationID != nil {
		t.Error("POST /admin/import: entry:import has no matching RoleDeclaration in blog-api and must stay unresolved")
	}

	// POST /tenants/{tenantId}/admin/import — the real endpoint this
	// controller also declares. Every rule that could match it
	// (.requestMatchers(POST, "/tenants/{tenantId}/**").access(editForTenant),
	// or the more specific .../admin/import one further down) is
	// unrecognized (.access(...)); per ADR 0018, the first such match
	// stops evaluation. Zero guards, zero AuthenticationRequirements —
	// unresolved, not public, not silently treated as guarded either.
	tenantImp, ok := findEndpoint(m.Endpoints, model.MethodPost, "/tenants/{tenantId}/admin/import")
	if !ok {
		t.Fatal("missing endpoint POST /tenants/{tenantId}/admin/import")
	}
	for _, g := range m.GuardApplications {
		if g.EndpointID == tenantImp.ID {
			t.Errorf("POST /tenants/{tenantId}/admin/import: expected no GuardApplications (unrecognized rule matches first), got %+v", g)
		}
	}
	for _, a := range m.AuthenticationRequirements {
		if a.EndpointID == tenantImp.ID {
			t.Errorf("POST /tenants/{tenantId}/admin/import: expected no AuthenticationRequirements, got %+v", a)
		}
	}

	// CategoryRestController's endpoints match no explicit rule at all and
	// fall to the trailing .anyRequest().permitAll() — contributes
	// nothing, same as Pharmacy's AuthController.login.
	categories, ok := findEndpoint(m.Endpoints, model.MethodGet, "/categories")
	if !ok {
		t.Fatal("missing endpoint GET /categories")
	}
	for _, g := range m.GuardApplications {
		if g.EndpointID == categories.ID {
			t.Errorf("GET /categories: expected no GuardApplications (permitAll catch-all), got %+v", g)
		}
	}
}

// TestAppliesSecurityFilterChain_UnrecognizedMatchStopsEvaluation is ADR
// 0018's regression test: the real vendored blog-api rule chain
// (testdata/blog-api/src/main/java/am/ik/blog/config/SecurityConfig.java)
// contains, in source order, .requestMatchers(GET, "/tenants/{tenantId}/entries.zip").access(exportForTenant)
// (unrecognized) followed later by the trailing .anyRequest().permitAll().
// The endpoint that would trigger this in real usage
// (GET /tenants/{tenantId}/entries.zip, owned by EntryRestController) is
// not itself vendored — only SecurityConfig.java and two other
// controllers are (testdata/blog-api/NOTICE.md) — so the endpoint here is
// constructed, run against the real, unmodified extracted rule chain. The
// chain is real; only the endpoint striking it is not, stated explicitly
// per the same split already used for internal/export/cerbos's
// partial-overlap/empty-intersection tests (docs/testing.md).
//
// Must resolve to unresolved (no guard, no AuthenticationRequirement) —
// never "public," which is what ADR 0012 §1's original "skip unrecognized,
// keep scanning" wording would have produced by reaching the trailing
// permitAll(). Confirmed (like the ADR 0014 merge-bug regression test) to
// actually fail under that old behavior before being kept — see the
// commit history for this file.
func TestAppliesSecurityFilterChain_UnrecognizedMatchStopsEvaluation(t *testing.T) {
	files, err := parseProject("testdata/blog-api/src/main/java")
	if err != nil {
		t.Fatalf("parseProject: %v", err)
	}
	rules, _, ok := findSecurityFilterChainRules(files)
	if !ok {
		t.Fatal("expected exactly one SecurityFilterChain bean in blog-api")
	}

	endpoint := model.Endpoint{
		ID:         model.NewEndpointID(model.MethodGet, "/tenants/{tenantId}/entries.zip"),
		HTTPMethod: model.MethodGet,
		Path:       "/tenants/{tenantId}/entries.zip",
	}

	rule, matched := firstMatch(rules, endpoint)
	if !matched {
		t.Fatal("expected the real .access(exportForTenant) rule to match this endpoint")
	}
	if rule.kind != chainUnrecognized {
		t.Fatalf("first matching rule kind = %v, want chainUnrecognized (.access(exportForTenant)) — got a different rule matching first, check rule ordering", rule.kind)
	}

	b := newBuilder()
	b.model.Endpoints = []model.Endpoint{endpoint}
	b.applySecurityFilterChain(rules, "SecurityConfig.java", nil)

	if len(b.model.GuardApplications) != 0 {
		t.Errorf("expected no GuardApplications (unresolved, not public), got %+v", b.model.GuardApplications)
	}
	if len(b.authCandidates) != 0 {
		t.Errorf("expected no authCandidates (unresolved, not authenticated-any-role), got %+v", b.authCandidates)
	}
}
