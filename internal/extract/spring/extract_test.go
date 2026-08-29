package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// TestExtract_Pharmacy runs full extraction (structure, guards, roles,
// method-security status) against the vendored, in-scope Pharmacy fixture
// (testdata/Pharmacy/NOTICE.md) and checks the resulting Controller/Endpoint
// list against ground truth verified by hand against the real source, per
// docs/testing.md. Guard/role assertions are in TestExtract_Pharmacy_Guards
// below, kept separate for readability.
func TestExtract_Pharmacy(t *testing.T) {
	m, err := Extract("testdata/Pharmacy/backend/src/main/java")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	wantControllers := map[string]string{ // name -> base path
		"SupplierController": "/api/suppliers",
		"CustomerController": "/api/customers",
		"AuthController":     "/auth",
	}
	if len(m.Controllers) != len(wantControllers) {
		t.Fatalf("got %d controllers, want %d: %+v", len(m.Controllers), len(wantControllers), controllerNames(m.Controllers))
	}
	controllerByID := make(map[model.ID]model.Controller, len(m.Controllers))
	for _, c := range m.Controllers {
		controllerByID[c.ID] = c
		wantBase, ok := wantControllers[c.Name]
		if !ok {
			t.Errorf("unexpected controller %q", c.Name)
			continue
		}
		if c.BasePath != wantBase {
			t.Errorf("%s.BasePath = %q, want %q", c.Name, c.BasePath, wantBase)
		}
	}

	wantEndpoints := []struct {
		method     model.HTTPMethod
		path       string
		handler    string
		controller string
	}{
		{model.MethodPost, "/api/suppliers", "create", "SupplierController"},
		{model.MethodGet, "/api/suppliers", "getAll", "SupplierController"},
		{model.MethodPost, "/api/customers", "create", "CustomerController"},
		{model.MethodPut, "/api/customers/{id}", "update", "CustomerController"},
		{model.MethodGet, "/api/customers/{id}", "get", "CustomerController"},
		{model.MethodGet, "/api/customers", "getAll", "CustomerController"},
		{model.MethodDelete, "/api/customers/{id}", "delete", "CustomerController"},
		{model.MethodPost, "/auth/login", "login", "AuthController"},
	}
	if len(m.Endpoints) != len(wantEndpoints) {
		t.Fatalf("got %d endpoints, want %d: %+v", len(m.Endpoints), len(wantEndpoints), endpointSummaries(m.Endpoints))
	}
	for _, want := range wantEndpoints {
		e, ok := findEndpoint(m.Endpoints, want.method, want.path)
		if !ok {
			t.Errorf("missing endpoint %s %s", want.method, want.path)
			continue
		}
		if e.HandlerName != want.handler {
			t.Errorf("%s %s: handler = %q, want %q", want.method, want.path, e.HandlerName, want.handler)
		}
		if c := controllerByID[e.ControllerID]; c.Name != want.controller {
			t.Errorf("%s %s: controller = %q, want %q", want.method, want.path, c.Name, want.controller)
		}
	}
}

// TestExtract_Pharmacy_Guards checks method-layer guard/role extraction
// against Pharmacy's real @PreAuthorize usage — every occurrence in the
// vendored fixture, hand-verified against the source, per docs/testing.md:
// real extraction logic (annotation recognition, SpEL parsing,
// role-declaration resolution) gets real-fixture coverage, not
// synthetic-only. Scoped to ScopeMethod only; the URL layer
// (SecurityFilterChain) is TestExtract_Pharmacy_SecurityFilterChain below —
// kept separate because SupplierController's two endpoints now carry both
// a method-layer and a URL-layer GuardApplication, and conflating them
// here would obscure which layer each assertion is actually about.
func TestExtract_Pharmacy_Guards(t *testing.T) {
	m, err := Extract("testdata/Pharmacy/backend/src/main/java")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Role.java declares exactly ADMIN and PHARMACIST, both referenced by
	// real hasRole/hasAnyRole calls below — both must resolve.
	if len(m.RoleDeclarations) != 2 {
		t.Fatalf("got %d role declarations, want 2: %+v", len(m.RoleDeclarations), m.RoleDeclarations)
	}
	roleDeclByName := make(map[string]model.ID, len(m.RoleDeclarations))
	for _, d := range m.RoleDeclarations {
		if d.Kind != model.RoleDeclarationEnum {
			t.Errorf("role declaration %q: kind = %q, want enum (Role.java is a plain Java enum)", d.Name, d.Kind)
		}
		roleDeclByName[d.Name] = d.ID
	}
	if _, ok := roleDeclByName["ADMIN"]; !ok {
		t.Error("missing role declaration ADMIN")
	}
	if _, ok := roleDeclByName["PHARMACIST"]; !ok {
		t.Error("missing role declaration PHARMACIST")
	}

	// Every @PreAuthorize in the vendored source, method + resolved roles,
	// hand-counted against SupplierController.java/CustomerController.java.
	// AuthController.login carries no @PreAuthorize at all — no entry here,
	// confirmed separately below (zero guards on that endpoint).
	wantGuards := []struct {
		method model.HTTPMethod
		path   string
		roles  []string
	}{
		{model.MethodPost, "/api/suppliers", []string{"ADMIN"}},
		{model.MethodGet, "/api/suppliers", []string{"ADMIN", "PHARMACIST"}},
		{model.MethodPost, "/api/customers", []string{"ADMIN", "PHARMACIST"}},
		{model.MethodPut, "/api/customers/{id}", []string{"ADMIN", "PHARMACIST"}},
		{model.MethodGet, "/api/customers/{id}", []string{"ADMIN", "PHARMACIST"}},
		{model.MethodGet, "/api/customers", []string{"ADMIN", "PHARMACIST"}},
		{model.MethodDelete, "/api/customers/{id}", []string{"ADMIN"}},
	}

	roleRefsByGuardApp := make(map[model.ID][]model.RoleReference, len(m.RoleReferences))
	for _, r := range m.RoleReferences {
		roleRefsByGuardApp[r.GuardApplicationID] = append(roleRefsByGuardApp[r.GuardApplicationID], r)
	}
	// Method-layer only (AppliedAt == ScopeMethod) — SupplierController's
	// two endpoints also carry a URL-layer GuardApplication now
	// (TestExtract_Pharmacy_SecurityFilterChain), which would otherwise
	// double-count against wantGuards here.
	methodGuardsByEndpoint := make(map[model.ID][]model.GuardApplication, len(m.GuardApplications))
	var methodLayerCount int
	for _, g := range m.GuardApplications {
		if g.AppliedAt != model.ScopeMethod {
			continue
		}
		methodLayerCount++
		methodGuardsByEndpoint[g.EndpointID] = append(methodGuardsByEndpoint[g.EndpointID], g)
	}

	if methodLayerCount != len(wantGuards) {
		t.Fatalf("got %d method-layer GuardApplications, want %d", methodLayerCount, len(wantGuards))
	}

	for _, want := range wantGuards {
		e, ok := findEndpoint(m.Endpoints, want.method, want.path)
		if !ok {
			t.Errorf("missing endpoint %s %s", want.method, want.path)
			continue
		}
		apps := methodGuardsByEndpoint[e.ID]
		if len(apps) != 1 {
			t.Errorf("%s %s: got %d method-layer GuardApplications, want 1: %+v", want.method, want.path, len(apps), apps)
			continue
		}
		g := apps[0]
		if g.GuardName != "PreAuthorize" || !g.DeclaresRoles {
			t.Errorf("%s %s: GuardApplication = %+v, want PreAuthorize/DeclaresRoles=true", want.method, want.path, g)
		}
		refs := roleRefsByGuardApp[g.ID]
		if len(refs) != len(want.roles) {
			t.Errorf("%s %s: got %d role refs, want %d: %+v", want.method, want.path, len(refs), len(want.roles), refs)
			continue
		}
		for i, r := range refs {
			if r.RawLiteral != want.roles[i] {
				t.Errorf("%s %s: role[%d] = %q, want %q", want.method, want.path, i, r.RawLiteral, want.roles[i])
			}
			if r.RoleDeclarationID == nil || *r.RoleDeclarationID != roleDeclByName[r.RawLiteral] {
				t.Errorf("%s %s: role %q did not resolve to its Role.java declaration", want.method, want.path, r.RawLiteral)
			}
		}
	}

	// AuthController.login: no @PreAuthorize, no method-layer guard at
	// all — real, by-design unguarded. Its URL layer (SecurityConfig.java's
	// permitAll() on /auth/**) is checked in
	// TestExtract_Pharmacy_SecurityFilterChain.
	login, ok := findEndpoint(m.Endpoints, model.MethodPost, "/auth/login")
	if !ok {
		t.Fatal("missing endpoint POST /auth/login")
	}
	if apps := methodGuardsByEndpoint[login.ID]; len(apps) != 0 {
		t.Errorf("POST /auth/login: got %d method-layer GuardApplications, want 0: %+v", len(apps), apps)
	}

	// No real isAuthenticated() usage anywhere in Pharmacy's vendored
	// @PreAuthorize calls — zero method-layer AuthenticationRequirements.
	// (CustomerController's real URL-layer AuthenticationRequirements —
	// from SecurityConfig.java's anyRequest().authenticated() catch-all —
	// are checked in TestExtract_Pharmacy_SecurityFilterChain, not here.)
	for _, a := range m.AuthenticationRequirements {
		if a.AppliedAt == model.ScopeMethod {
			t.Errorf("expected no method-layer AuthenticationRequirements, got %+v", a)
		}
	}

	// SecurityConfig.java carries a bare @EnableMethodSecurity: prePost
	// defaults true, secured/jsr250 default false — Spring's own
	// documented defaults (docs/decisions/0015-inert-method-security-guard.md),
	// not this project's assumption.
	want := model.MethodSecurityStatus{Found: true, PrePostEnabled: true, SecuredEnabled: false, Jsr250Enabled: false}
	if m.MethodSecurity != want {
		t.Errorf("MethodSecurity = %+v, want %+v", m.MethodSecurity, want)
	}
}

// TestExtract_BlogAPI runs the same structural extraction against the
// vendored, out-of-scope blog-api fixture (testdata/blog-api/NOTICE.md).
// Endpoint discovery and authorization resolution are separate concerns
// (docs/decisions/0011-spring-second-framework.md) — every endpoint here
// must still be found even though most of this app's real access control
// (`.access(AuthorizationManager)`) is out of scope for extraction to ever
// resolve.
//
// CategoryRestController surfaces the real shape that drove
// docs/decisions/0014-endpoint-identity-and-content-negotiation.md:
// `categories()` and `categoriesAsProtobuf()` (also their per-tenant
// counterparts) are two distinct real Spring handlers mapped to the exact
// same HTTP method and path, differing only in `produces`. Per ADR 0014,
// `produces` does not participate in Endpoint identity, so these merge
// into one Endpoint each at discovery — asserted explicitly below, along
// with which handler's name survives (the first encountered in source
// order, per ADR 0014's Consequences).
func TestExtract_BlogAPI(t *testing.T) {
	m, err := Extract("testdata/blog-api/src/main/java")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	wantControllers := map[string]string{
		"EntryImportController":  "",
		"CategoryRestController": "",
	}
	if len(m.Controllers) != len(wantControllers) {
		t.Fatalf("got %d controllers, want %d: %+v", len(m.Controllers), len(wantControllers), controllerNames(m.Controllers))
	}
	for _, c := range m.Controllers {
		if _, ok := wantControllers[c.Name]; !ok {
			t.Errorf("unexpected controller %q", c.Name)
		}
		if c.BasePath != "" {
			t.Errorf("%s.BasePath = %q, want \"\" (neither vendored blog-api controller has a class-level @RequestMapping)", c.Name, c.BasePath)
		}
	}

	// 2 (EntryImportController) + 2 (CategoryRestController, with its two
	// produces-only variant pairs merged per ADR 0014) = 4 distinct
	// endpoints, hand-counted against the vendored source.
	if len(m.Endpoints) != 4 {
		t.Fatalf("got %d endpoints, want 4: %+v", len(m.Endpoints), endpointSummaries(m.Endpoints))
	}

	wantHandlers := map[string]bool{
		"importEntries":          true,
		"importEntriesForTenant": true,
		"categories":             true, // wins the merge over categoriesAsProtobuf: encountered first, source order
		"categoriesForTenant":    true, // wins the merge over categoriesAsProtobufForTenant, same reason
	}
	for _, e := range m.Endpoints {
		if !wantHandlers[e.HandlerName] {
			t.Errorf("unexpected handler %q", e.HandlerName)
		}
	}

	categories, ok := findEndpoint(m.Endpoints, model.MethodGet, "/categories")
	if !ok {
		t.Fatal("missing endpoint GET /categories")
	}
	if categories.HandlerName != "categories" {
		t.Errorf("GET /categories: handler = %q, want %q (the merge winner, per ADR 0014)", categories.HandlerName, "categories")
	}

	tenantCategories, ok := findEndpoint(m.Endpoints, model.MethodGet, "/tenants/{tenantId}/categories")
	if !ok {
		t.Fatal("missing endpoint GET /tenants/{tenantId}/categories")
	}
	if tenantCategories.HandlerName != "categoriesForTenant" {
		t.Errorf("GET /tenants/{tenantId}/categories: handler = %q, want %q (the merge winner, per ADR 0014)", tenantCategories.HandlerName, "categoriesForTenant")
	}

	if _, ok := findEndpoint(m.Endpoints, model.MethodPost, "/admin/import"); !ok {
		t.Error("missing endpoint POST /admin/import")
	}
	if _, ok := findEndpoint(m.Endpoints, model.MethodPost, "/tenants/{tenantId}/admin/import"); !ok {
		t.Error("missing endpoint POST /tenants/{tenantId}/admin/import")
	}

	// blog-api's SecurityConfig.java carries @EnableMethodSecurity(prePostEnabled = false)
	// explicitly — a real, positive parse of a non-default attribute
	// value, not just the bare-annotation default case Pharmacy already
	// covers. secured/jsr250 stay at their own defaults (false), since
	// neither is set here either. Zero @PreAuthorize/@Secured/@RolesAllowed
	// exist anywhere in the vendored blog-api controllers to be
	// downstream-affected by this (confirmed by grep against the real
	// source, not assumed) — the confirmed-inert consequence itself
	// (internal/lint/mutating_endpoint.go) is exercised by a synthetic
	// test instead, per docs/testing.md: this MethodSecurityStatus value
	// is what real extraction produces from real annotation syntax, and
	// isConfirmedInert is a pure function of that value once produced.
	wantStatus := model.MethodSecurityStatus{Found: true, PrePostEnabled: false, SecuredEnabled: false, Jsr250Enabled: false}
	if m.MethodSecurity != wantStatus {
		t.Errorf("MethodSecurity = %+v, want %+v", m.MethodSecurity, wantStatus)
	}
	// Zero *method-layer* GuardApplications — confirmed above. The URL
	// layer (SecurityConfig.java's real, mixed hasAuthority/access()
	// chain) now contributes its own GuardApplications; that's
	// TestExtract_BlogAPI_SecurityFilterChain's job, not this test's —
	// kept separate so a URL-layer assertion failure doesn't read as if
	// method-layer extraction (this test's actual subject) regressed.
	for _, g := range m.GuardApplications {
		if g.AppliedAt == model.ScopeMethod {
			t.Errorf("expected no method-layer GuardApplications in vendored blog-api controllers, got %+v", g)
		}
	}
}

func controllerNames(controllers []model.Controller) []string {
	out := make([]string, len(controllers))
	for i, c := range controllers {
		out[i] = c.Name
	}
	return out
}

func endpointSummaries(endpoints []model.Endpoint) []string {
	out := make([]string, len(endpoints))
	for i, e := range endpoints {
		out[i] = string(e.HTTPMethod) + " " + e.Path + " (" + e.HandlerName + ")"
	}
	return out
}

func findEndpoint(endpoints []model.Endpoint, method model.HTTPMethod, path string) (model.Endpoint, bool) {
	for _, e := range endpoints {
		if e.HTTPMethod == method && e.Path == path {
			return e, true
		}
	}
	return model.Endpoint{}, false
}
