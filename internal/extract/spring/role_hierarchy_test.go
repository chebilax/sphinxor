package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
)

// TestRoleHierarchy_ThreeShapes covers ADR 0031 §3's enumerated list.
// The corpus has zero role hierarchies, so nothing real exercises any of
// these — which is why each shape is written out rather than left to
// whichever one the first project happens to use.
func TestRoleHierarchy_ThreeShapes(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{
			"@Bean method returning RoleHierarchy",
			`package app;
import org.springframework.context.annotation.Bean;
import org.springframework.security.access.hierarchicalroles.RoleHierarchy;

public class SecurityConfig {
    @Bean
    RoleHierarchy roleHierarchy() { return null; }
}
`,
		},
		{
			"RoleHierarchyImpl.fromHierarchy",
			`package app;
import org.springframework.security.access.hierarchicalroles.RoleHierarchyImpl;

public class SecurityConfig {
    void configure() {
        var h = RoleHierarchyImpl.fromHierarchy("ROLE_ADMIN > ROLE_USER");
    }
}
`,
		},
		{
			"setRoleHierarchy on an expression handler",
			`package app;

public class SecurityConfig {
    void configure() {
        handler.setRoleHierarchy(myHierarchy);
    }
}
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := extractProject(t, map[string]string{"SecurityConfig.java": tc.src})
			if !m.RoleHierarchy.Found {
				t.Fatalf("this shape must be detected (ADR 0031 §3)")
			}
			if got := m.RoleHierarchy.DeclaredIn; len(got) != 1 || got[0] != "SecurityConfig" {
				t.Errorf("DeclaredIn = %v, want [SecurityConfig] for the warning text", got)
			}
		})
	}
}

// TestRoleHierarchy_ContentIsNotParsed is ADR 0031 §1. Detecting the
// hierarchy must not expand a single grant — that would change reported
// access rather than annotate it, which the ADR keeps as a separate
// decision.
func TestRoleHierarchy_ContentIsNotParsed(t *testing.T) {
	m := extractProject(t, map[string]string{
		"SecurityConfig.java": `package app;
import org.springframework.context.annotation.Bean;
import org.springframework.security.access.hierarchicalroles.RoleHierarchy;
import org.springframework.security.access.hierarchicalroles.RoleHierarchyImpl;

public class SecurityConfig {
    @Bean
    RoleHierarchy roleHierarchy() {
        return RoleHierarchyImpl.fromHierarchy("ROLE_ADMIN > ROLE_USER");
    }
}
`,
		"Config.java": `package app;
import org.springframework.security.config.annotation.method.configuration.EnableMethodSecurity;

@Configuration
@EnableMethodSecurity
public class Config { }
`,
		"C.java": `package app;
import org.springframework.security.access.prepost.PreAuthorize;
import org.springframework.web.bind.annotation.*;

@RestController
public class C {
    @PreAuthorize("hasRole('USER')")
    @PostMapping("/a")
    public void a() { }
}
`,
	})
	if !m.RoleHierarchy.Found {
		t.Fatal("the hierarchy must be detected")
	}
	var roles []string
	for _, r := range m.RoleReferences {
		roles = append(roles, r.RawLiteral)
	}
	if len(roles) != 1 || roles[0] != "USER" {
		t.Errorf("roles = %v, want exactly [USER] — ROLE_ADMIN must NOT be added (ADR 0031 §1)", roles)
	}
	// §2: a warning, never a finding. The endpoint is guarded, and the
	// hierarchy does nothing to change that.
	if n := len((lint.MutatingEndpointWithoutAccessControl{}).Check(m)); n != 0 {
		t.Errorf("a role hierarchy must not produce a finding, got %d", n)
	}
}

// TestRoleHierarchy_AbsenceIsNotConfirmedAbsent pins §3's boundary. A
// project with no located hierarchy reports Found false, which the
// warning treats as "none located" rather than "confirmed none" — the
// same distinction ADR 0015 draws for @EnableMethodSecurity.
func TestRoleHierarchy_AbsenceIsNotConfirmedAbsent(t *testing.T) {
	m := extractProject(t, map[string]string{
		"C.java": `package app;
import org.springframework.web.bind.annotation.*;

@RestController
public class C {
    @GetMapping("/a")
    public String a() { return ""; }
}
`,
	})
	if m.RoleHierarchy.Found {
		t.Errorf("nothing here declares a hierarchy, got %+v", m.RoleHierarchy)
	}
}

// TestRoleHierarchy_UnrelatedNamesDoNotMatch is the negative control.
// fromHierarchy and withDefaultRolePrefix are generic enough that the
// receiver has to matter, or an unrelated builder would invent a
// hierarchy and a warning with it.
func TestRoleHierarchy_UnrelatedNamesDoNotMatch(t *testing.T) {
	m := extractProject(t, map[string]string{
		"C.java": `package app;

public class OrgChart {
    void build() {
        var tree = DepartmentTree.fromHierarchy("sales > west");
        namer.withDefaultRolePrefix("x");
    }
}
`,
	})
	if m.RoleHierarchy.Found {
		t.Errorf("an unrelated fromHierarchy call is not Spring's, got %+v", m.RoleHierarchy)
	}
}

// TestRoleHierarchy_TrackrFixture is the first real code this decision has
// had (testdata/trackr-backend/NOTICE.md): a hierarchy written in source,
// wired into method security, next to endpoints whose roles it would widen.
//
// It pins today's behaviour, which is ADR 0031's — detected, announced,
// not read — so that ADR 0038, if accepted, changes this test deliberately
// rather than by accident. Under ADR 0038 as proposed the expectation for
// this fixture does NOT become "expanded": its literal puts three pairs on
// one line, which Spring Security reads as a hierarchy up to 5.0 and, from
// 5.1, as one malformed chain in which ROLE_ADMIN reaches only
// ROLE_ANONYMOUS — and nothing in these files fixes the version.
func TestRoleHierarchy_TrackrFixture(t *testing.T) {
	m, _, err := Extract("testdata/trackr-backend")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !m.RoleHierarchy.Found {
		t.Fatal("trackr's hierarchy must be detected (ADR 0031 §3: @Bean returning RoleHierarchy, and setRoleHierarchy)")
	}
	if got := m.RoleHierarchy.DeclaredIn; len(got) != 1 || got[0] != "MethodSecurityConfiguration" {
		t.Errorf("DeclaredIn = %v, want [MethodSecurityConfiguration]", got)
	}

	guardEndpoint := make(map[model.ID]model.ID, len(m.GuardApplications))
	for _, g := range m.GuardApplications {
		guardEndpoint[g.ID] = g.EndpointID
	}
	rolesByPath := map[string][]string{}
	for _, e := range m.Endpoints {
		for _, r := range m.RoleReferences {
			if guardEndpoint[r.GuardApplicationID] == e.ID {
				rolesByPath[string(e.HTTPMethod)+" "+e.Path] = append(rolesByPath[string(e.HTTPMethod)+" "+e.Path], r.RawLiteral)
			}
		}
	}
	want := map[string]string{
		"PUT /vacationRequests/{id}/approve":           "ROLE_SUPERVISOR",
		"PUT /vacationRequests/{id}/reject":            "ROLE_SUPERVISOR",
		"GET /vacationRequests/daysPerEmployeeBetween": "ROLE_ADMIN",
	}
	if len(rolesByPath) != len(want) {
		t.Errorf("role-carrying endpoints = %v, want exactly %v", rolesByPath, want)
	}
	for path, role := range want {
		if got := rolesByPath[path]; len(got) != 1 || got[0] != role {
			t.Errorf("%s roles = %v, want exactly [%s] — the hierarchy must not be expanded here", path, got, role)
		}
	}
}
