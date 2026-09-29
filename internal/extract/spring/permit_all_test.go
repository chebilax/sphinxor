package spring

import (
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/export/cerbos"
	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
	"github.com/chebilax/sphinxor/internal/report"
)

// Tests for docs/decisions/0043-permit-all-is-a-public-declaration.md.
// Synthetic by necessity for the precedence cases, which yudao never
// reaches (none of its 106 @PermitAll shares a class or method with a
// guard); the plain case is also pinned on the vendored ruoyi-vue-pro
// fixture below.

func permitController(classAnns, classImports, methodAnns string) map[string]string {
	return map[string]string{
		"C.java": "package app;\n" + classImports + "\nimport org.springframework.web.bind.annotation.*;\n" +
			classAnns + "\n@RestController\npublic class C {\n    " + methodAnns +
			"\n    @DeleteMapping(\"/x\")\n    public void x() { }\n}\n",
		"Config.java": "package app;\n@EnableMethodSecurity(jsr250Enabled = true)\npublic class Config { }\n",
	}
}

const jsr250Imports = "import jakarta.annotation.security.PermitAll;\nimport jakarta.annotation.security.RolesAllowed;\nimport org.springframework.security.access.prepost.PreAuthorize;"

// TestPermitAll_EveryConsumer walks ADR 0043 §2 for a JSR-250 @PermitAll
// on a mutating handler.
func TestPermitAll_EveryConsumer(t *testing.T) {
	m := extractProject(t, permitController("", jsr250Imports, "@PermitAll"))
	if len(m.GuardApplications) != 0 {
		t.Errorf("a permit-all is not a guard: %+v", m.GuardApplications)
	}
	if p := m.PublicDeclarations; len(p) != 1 || p[0].Form != "@PermitAll" {
		t.Fatalf("PublicDeclarations = %+v", p)
	}
	if n := findingsFor(m, "empty-role"); n != 0 {
		t.Errorf("empty-role fired %d time(s)", n)
	}
	var msg string
	for _, f := range lintAll(m) {
		if f.RuleID == "mutating-endpoint-without-access-control" {
			msg = f.Message
			if f.Confidence != model.ConfidenceLow {
				t.Errorf("confidence = %s, want Low", f.Confidence)
			}
		}
	}
	if !strings.Contains(msg, "declared public by @PermitAll") || !strings.Contains(msg, "sphinxor-allow") {
		t.Errorf("mutating message = %q, want it to name @PermitAll and sphinxor-allow", msg)
	}
	if o := onlyOmission(t, m); o.Reason != cerbos.ReasonDeclaredPublic {
		t.Errorf("omission = %s, want declared-public", o.Reason)
	}
	var b strings.Builder
	if err := report.Write(&b, m, nil, report.FormatMarkdown); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "public (@PermitAll)") {
		t.Errorf("matrix does not label the endpoint public:\n%s", b.String())
	}
}

// TestPermitAll_Precedence is ADR 0043 §3: method over class within one
// annotation family only, because Spring runs one interceptor per family.
func TestPermitAll_Precedence(t *testing.T) {
	for _, tc := range []struct {
		name, class, method string
		wantGuards          int
		wantPublic          int
		wantMutating        int
	}{
		{"same family: method @PermitAll overrides class @RolesAllowed", `@RolesAllowed("ADMIN")`, "@PermitAll", 0, 1, 1},
		{"same family: method @RolesAllowed overrides class @PermitAll", "@PermitAll", `@RolesAllowed("ADMIN")`, 1, 0, 0},
		{"same family: method permitAll() overrides class @PreAuthorize", `@PreAuthorize("hasRole('ADMIN')")`, `@PreAuthorize("permitAll()")`, 0, 1, 1},
		{"across families: class @PreAuthorize still applies under method @PermitAll", `@PreAuthorize("hasRole('ADMIN')")`, "@PermitAll", 1, 1, 0},
		{"class @PermitAll applies to an unannotated method", "@PermitAll", "", 0, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := extractProject(t, permitController(tc.class, jsr250Imports, tc.method))
			if len(m.GuardApplications) != tc.wantGuards || len(m.PublicDeclarations) != tc.wantPublic {
				t.Errorf("guards %d, public %d; want %d, %d", len(m.GuardApplications), len(m.PublicDeclarations), tc.wantGuards, tc.wantPublic)
			}
			if n := findingsFor(m, "mutating-endpoint-without-access-control"); n != tc.wantMutating {
				t.Errorf("mutating finding %d, want %d", n, tc.wantMutating)
			}
		})
	}
}

// TestPermitAll_CollisionWithUnannotatedTwinWarns: a public side and an
// unannotated one differ, as on 4 of yudao's colliding routes.
func TestPermitAll_CollisionWithUnannotatedTwinWarns(t *testing.T) {
	m := extractProject(t, map[string]string{
		"a/A.java": "package a;\nimport jakarta.annotation.security.PermitAll;\nimport org.springframework.web.bind.annotation.*;\n@RestController\npublic class A {\n    @PermitAll\n    @PostMapping(\"/login\")\n    public void login() { }\n}\n",
		"b/B.java": "package b;\nimport org.springframework.web.bind.annotation.*;\n@RestController\npublic class B {\n    @PostMapping(\"/login\")\n    public void login() { }\n}\n",
	})
	if len(m.RouteCollisions) != 1 || !m.RouteCollisions[0].GuardsDiffer {
		t.Errorf("RouteCollisions = %+v, want one that differs", m.RouteCollisions)
	}
	if len(m.PublicDeclarations) != 1 {
		t.Fatalf("PublicDeclarations = %+v", m.PublicDeclarations)
	}
	owner := m.PublicDeclarations[0].EndpointID
	found := false
	for _, e := range m.Endpoints {
		if e.ID == owner && e.RouteCollision {
			found = true
		}
	}
	if !found {
		t.Error("the declaration was not re-pointed to its endpoint's collision identity")
	}
}

func lintAll(m *model.Model) []model.Finding { return lint.Run(m, lint.DefaultRules(), nil) }
