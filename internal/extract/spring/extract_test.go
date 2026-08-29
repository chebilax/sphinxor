package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// TestExtract_Pharmacy runs structural extraction against the vendored,
// in-scope Pharmacy fixture (testdata/Pharmacy/NOTICE.md) and checks the
// resulting Controller/Endpoint list against ground truth verified by hand
// against the real source, per docs/testing.md. No guard/role/authentication
// assertions here — this package's first cut extracts structure only.
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
