package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// TestUnreadableRequestMappingPath_DoesNotDropEndpoints is the Spring
// half of ADR 0020 Amendment 1 §5.
//
// Endpoint identity is derived from (method, path), and
// @RequestMapping(Routes.ADMIN) read as the empty prefix. Where NestJS
// merged the colliding endpoints, Spring dropped one outright: two real
// routes produced a single row, and the row that survived was the
// guarded one — so the wide-open DELETE disappeared from the report
// entirely, taking its mutating-endpoint finding with it.
//
// Confirmed to fail against that behavior before being kept.
func TestUnreadableRequestMappingPath_DoesNotDropEndpoints(t *testing.T) {
	dir := writeJavaProject(t, map[string]string{
		"Routes.java": `package app;
public final class Routes {
    public static final String ADMIN = "/admin";
    public static final String PUBLIC = "/public";
}
`,
		"Controllers.java": `package app;
import org.springframework.web.bind.annotation.*;
import org.springframework.security.access.prepost.PreAuthorize;

@RestController
@RequestMapping(Routes.ADMIN)
class AdminController {
    @PreAuthorize("hasRole('ADMIN')")
    @DeleteMapping("/wipe")
    public void wipe() { }
}

@RestController
@RequestMapping(Routes.PUBLIC)
class PublicController {
    @DeleteMapping("/wipe")
    public void wipe() { }
}
`,
	})

	m, _, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(m.Endpoints) != 2 {
		t.Fatalf("got %d endpoint(s), want 2 — a colliding identity silently replaced one: %+v",
			len(m.Endpoints), m.Endpoints)
	}

	byController := map[string]model.Endpoint{}
	for _, e := range m.Endpoints {
		for _, c := range m.Controllers {
			if c.ID == e.ControllerID {
				byController[c.Name] = e
			}
		}
	}
	public, ok := byController["PublicController"]
	if !ok {
		t.Fatalf("PublicController's endpoint is missing — it is the unguarded one: %v", byController)
	}
	if !public.PathUnresolved {
		t.Error("PathUnresolved = false, want true: the @RequestMapping prefix was never read")
	}

	// The unguarded DELETE must not inherit the other controller's role.
	for _, g := range m.GuardApplications {
		if g.EndpointID == public.ID {
			t.Errorf("DELETE /public/wipe has guard %+v, want none", g)
		}
	}
}

// TestRequestMappingPathShapes keeps the marking narrow: a controller
// with a readable prefix, or with no @RequestMapping at all, must not
// start reporting uncertainty it doesn't have.
func TestRequestMappingPathShapes(t *testing.T) {
	cases := []struct {
		name           string
		classAnns      string
		wantUnresolved bool
	}{
		{"string literal", "@RestController\n@RequestMapping(\"/api\")", false},
		{"value attribute", "@RestController\n@RequestMapping(value = \"/api\")", false},
		{"no request mapping", "@RestController", false},
		{"constant reference", "@RestController\n@RequestMapping(Routes.ADMIN)", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeJavaProject(t, map[string]string{
				"C.java": `package app;
import org.springframework.web.bind.annotation.*;
` + tc.classAnns + `
class C {
    @GetMapping("/x")
    public void x() { }
}
`,
			})
			m, _, err := Extract(dir)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			if len(m.Endpoints) != 1 {
				t.Fatalf("got %d endpoint(s), want 1: %+v", len(m.Endpoints), m.Endpoints)
			}
			if got := m.Endpoints[0].PathUnresolved; got != tc.wantUnresolved {
				t.Errorf("PathUnresolved = %v, want %v", got, tc.wantUnresolved)
			}
		})
	}
}
