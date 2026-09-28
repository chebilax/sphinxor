package spring

import (
	"strings"
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

// TestRoleHierarchy_ReadButNotExpanded is ADR 0038 Stage 1, and the part
// of ADR 0031 §1 it keeps. The rules are now read, and still expand no
// grant: that is Stage 2, deferred until a real project has rows for it
// to change (ADR 0038 §12).
func TestRoleHierarchy_ReadButNotExpanded(t *testing.T) {
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
	want := []model.RoleHierarchyEdge{{Higher: "ROLE_ADMIN", Lower: "ROLE_USER"}}
	if h := m.RoleHierarchy; !h.Read || !equalEdges(h.Edges, want) {
		t.Errorf("hierarchy = %+v, want read with edges %v (ADR 0038 §1)", h, want)
	}
	var roles []string
	for _, r := range m.RoleReferences {
		roles = append(roles, r.RawLiteral)
	}
	if len(roles) != 1 || roles[0] != "USER" {
		t.Errorf("roles = %v, want exactly [USER] — ROLE_ADMIN must NOT be added (ADR 0038 Stage 1)", roles)
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
// Its literal puts three pairs on one line, which Spring Security reads as
// a hierarchy up to 5.0 and, from 5.2, as one malformed chain in which
// ROLE_ADMIN reaches only ROLE_ANONYMOUS — and nothing in these files fixes
// the version: the legacy enabler and a WebSecurityConfigurerAdapter exist
// on both sides of the change. So ADR 0038 §2 states both readings and
// chooses neither, and the roles stay exactly as declared.
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
	h := m.RoleHierarchy
	if h.Read {
		t.Errorf("the version-dependent literal must not be read as either version (ADR 0038 §2), got %v", h.Edges)
	}
	wantUpTo50 := []model.RoleHierarchyEdge{
		{Higher: "ROLE_ADMIN", Lower: "ROLE_SUPERVISOR"},
		{Higher: "ROLE_SUPERVISOR", Lower: "ROLE_EMPLOYEE"},
		{Higher: "ROLE_EMPLOYEE", Lower: "ROLE_ANONYMOUS"},
	}
	wantFrom52 := []model.RoleHierarchyEdge{
		{Higher: "ROLE_ADMIN", Lower: "ROLE_SUPERVISOR ROLE_SUPERVISOR"},
		{Higher: "ROLE_SUPERVISOR ROLE_SUPERVISOR", Lower: "ROLE_EMPLOYEE ROLE_EMPLOYEE"},
		{Higher: "ROLE_EMPLOYEE ROLE_EMPLOYEE", Lower: "ROLE_ANONYMOUS"},
	}
	if !equalEdges(h.UpTo50, wantUpTo50) || !equalEdges(h.From52, wantFrom52) {
		t.Errorf("readings = ≤5.0 %v / ≥5.2 %v, want %v / %v", h.UpTo50, h.From52, wantUpTo50, wantFrom52)
	}
	if h.Condition != `@Profile("granular-security")` {
		t.Errorf("Condition = %q, want the class's @Profile verbatim (ADR 0038 §12)", h.Condition)
	}
	// Legacy @EnableGlobalMethodSecurity applies a hierarchy to
	// @PreAuthorize on every version, so nothing is assumed.
	if len(h.AssumedReach) != 0 {
		t.Errorf("AssumedReach = %v, want none under the legacy enabler", h.AssumedReach)
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

// TestRoleHierarchy_VideochatFixture is the enum-concatenation form on real
// code (testdata/videochat/NOTICE.md): UserRole.ROLE_ADMIN.name() + " > " +
// UserRole.ROLE_USER.name() + "\n", with the enum in another package.
//
// It also carries the one reach question the sample raised (ADR 0038
// Finding 2). The hierarchy is set on the project's own
// MethodSecurityExpressionHandler, which @EnableMethodSecurity uses on
// every version, so @PreAuthorize is not assumed. @Secured is enabled and
// receives the bean only from 6.3, and nothing here proves 6.3: that one
// is assumed, and the warning says so.
func TestRoleHierarchy_VideochatFixture(t *testing.T) {
	m, _, err := Extract("testdata/videochat")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	h := m.RoleHierarchy
	want := []model.RoleHierarchyEdge{{Higher: "ROLE_ADMIN", Lower: "ROLE_USER"}}
	if !h.Read || !equalEdges(h.Edges, want) {
		t.Fatalf("hierarchy = %+v, want read with %v", h, want)
	}
	if len(h.AssumedReach) != 1 || h.AssumedReach[0] != "@Secured" {
		t.Errorf("AssumedReach = %v, want [@Secured] — @PreAuthorize is wired explicitly", h.AssumedReach)
	}
}

// The cases below are synthetic by necessity, per docs/testing.md: they
// pin the grammar and evaluation rules of ADR 0038 §1–§2 and the §12
// assumption, each of which the sample exercises at most once.

func hierarchyProject(config string, extra map[string]string) map[string]string {
	files := map[string]string{"SecurityConfig.java": config}
	for k, v := range extra {
		files[k] = v
	}
	return files
}

func TestRoleHierarchy_ReadForms(t *testing.T) {
	adminUser := []model.RoleHierarchyEdge{{Higher: "ROLE_ADMIN", Lower: "ROLE_USER"}}
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []model.RoleHierarchyEdge
	}{
		{
			"literal via setHierarchy",
			hierarchyProject(`package app;
import org.springframework.security.access.hierarchicalroles.RoleHierarchyImpl;
public class SecurityConfig {
    @Bean RoleHierarchy roleHierarchy() {
        RoleHierarchyImpl h = new RoleHierarchyImpl();
        h.setHierarchy("ROLE_ADMIN > ROLE_USER");
        return h;
    }
}`, nil),
			adminUser,
		},
		{
			"literal through a local variable",
			hierarchyProject(`package app;
public class SecurityConfig {
    @Bean RoleHierarchy roleHierarchy() {
        RoleHierarchyImpl h = new RoleHierarchyImpl();
        String hierarchy = "ROLE_ADMIN > ROLE_USER";
        h.setHierarchy(hierarchy);
        return h;
    }
}`, nil),
			adminUser,
		},
		{
			"static final constants across classes, concatenated, two lines",
			hierarchyProject(`package app;
public class SecurityConfig {
    @Bean RoleHierarchy roleHierarchy() {
        RoleHierarchyImpl h = new RoleHierarchyImpl();
        h.setHierarchy(Roles.ADMIN + " > " + Roles.USER + "\n" + Roles.USER + " > ROLE_GUEST");
        return h;
    }
}`, map[string]string{"Roles.java": `package app;
public class Roles {
    public static final String ADMIN = "ROLE_ADMIN";
    public static final String USER = "ROLE_" + "USER";
}`}),
			[]model.RoleHierarchyEdge{{Higher: "ROLE_ADMIN", Lower: "ROLE_USER"}, {Higher: "ROLE_USER", Lower: "ROLE_GUEST"}},
		},
		{
			// A ≥5.2 floor (fromHierarchy is 6.3) fixes the grammar, so a
			// chain on one line is read as Spring reads it today.
			"a chain is read when the source proves the version",
			hierarchyProject(`package app;
public class SecurityConfig {
    @Bean RoleHierarchy roleHierarchy() {
        return RoleHierarchyImpl.fromHierarchy("ROLE_SUPER > ROLE_ADMIN > ROLE_USER");
    }
}`, nil),
			[]model.RoleHierarchyEdge{{Higher: "ROLE_SUPER", Lower: "ROLE_ADMIN"}, {Higher: "ROLE_ADMIN", Lower: "ROLE_USER"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := extractProject(t, tc.files).RoleHierarchy
			if !h.Read || !equalEdges(h.Edges, tc.want) {
				t.Errorf("hierarchy = %+v, want read with %v", h, tc.want)
			}
		})
	}
}

func TestRoleHierarchy_NotRead(t *testing.T) {
	for _, tc := range []struct {
		name, src, reason string
	}{
		{"a runtime value", `package app;
public class SecurityConfig {
    @Bean RoleHierarchy roleHierarchy() {
        return RoleHierarchyImpl.fromHierarchy(config.getRoleHierarchyString());
    }
}`, "config.getRoleHierarchyString()"},
		{"the 6.3 builder", `package app;
public class SecurityConfig {
    @Bean RoleHierarchy roleHierarchy() {
        return RoleHierarchyImpl.withDefaultRolePrefix().role("ADMIN").implies("USER").build();
    }
}`, "builder"},
		{"no string at all", `package app;
public class SecurityConfig {
    @Bean RoleHierarchy roleHierarchy(DataService d) { return new CachedRoleHierarchy(d); }
}`, "not handed to RoleHierarchyImpl as a string"},
		{"a cycle", `package app;
public class SecurityConfig {
    @Bean RoleHierarchy roleHierarchy() {
        return RoleHierarchyImpl.fromHierarchy("ROLE_A > ROLE_B\nROLE_B > ROLE_A");
    }
}`, "cycle"},
		{"a text block", `package app;
public class SecurityConfig {
    @Bean RoleHierarchy roleHierarchy() {
        return RoleHierarchyImpl.fromHierarchy("""
            ROLE_A > ROLE_B
            """);
    }
}`, "text block"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := extractProject(t, map[string]string{"SecurityConfig.java": tc.src}).RoleHierarchy
			if !h.Found || h.Read || !strings.Contains(h.NotRead, tc.reason) {
				t.Errorf("hierarchy = %+v, want found, not read, reason containing %q", h, tc.reason)
			}
		})
	}
}

// TestRoleHierarchy_ChainWithoutFloorIsAmbiguous is ADR 0038 §2 on the
// shape DuDoong writes in Kotlin: "A > B > C" is one rule up to 5.0 (the
// regex never finds a left operand for "> C") and two from 5.2.
func TestRoleHierarchy_ChainWithoutFloorIsAmbiguous(t *testing.T) {
	h := extractProject(t, map[string]string{"SecurityConfig.java": `package app;
public class SecurityConfig {
    @Bean RoleHierarchy roleHierarchy() {
        RoleHierarchyImpl h = new RoleHierarchyImpl();
        h.setHierarchy("ROLE_SUPER > ROLE_ADMIN > ROLE_USER");
        return h;
    }
}`}).RoleHierarchy
	if h.Read {
		t.Fatalf("read as %v, want both readings stated instead", h.Edges)
	}
	if want := []model.RoleHierarchyEdge{{Higher: "ROLE_SUPER", Lower: "ROLE_ADMIN"}}; !equalEdges(h.UpTo50, want) {
		t.Errorf("≤5.0 reading = %v, want %v", h.UpTo50, want)
	}
	if len(h.From52) != 2 {
		t.Errorf("≥5.2 reading = %v, want the two-edge chain", h.From52)
	}
}

// TestRoleHierarchy_AssumedReach is ADR 0038 §12's list, one case per
// branch of its rule.
func TestRoleHierarchy_AssumedReach(t *testing.T) {
	bean := `package app;
public class SecurityConfig {
    @Bean RoleHierarchy roleHierarchy() {
        RoleHierarchyImpl h = new RoleHierarchyImpl();
        h.setHierarchy("ROLE_ADMIN > ROLE_USER");
        return h;
    }
}`
	for _, tc := range []struct {
		name    string
		enabler string
		want    []string
	}{
		{"modern enabler, no floor: every enabled family", "@EnableMethodSecurity(securedEnabled = true, jsr250Enabled = true)",
			[]string{"@PreAuthorize", "@Secured", "@RolesAllowed"}},
		{"legacy enabler: nothing assumed", "@EnableGlobalMethodSecurity(prePostEnabled = true)", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := extractProject(t, map[string]string{
				"SecurityConfig.java": bean,
				"Config.java":         "package app;\n" + tc.enabler + "\npublic class Config { }\n",
			}).RoleHierarchy
			if strings.Join(h.AssumedReach, ",") != strings.Join(tc.want, ",") {
				t.Errorf("AssumedReach = %v, want %v", h.AssumedReach, tc.want)
			}
		})
	}

	// A 6.3 API anywhere in the tree fixes the version: nothing assumed.
	h := extractProject(t, map[string]string{
		"SecurityConfig.java": `package app;
public class SecurityConfig {
    @Bean RoleHierarchy roleHierarchy() { return RoleHierarchyImpl.fromHierarchy("ROLE_ADMIN > ROLE_USER"); }
}`,
		"Config.java": "package app;\n@EnableMethodSecurity(securedEnabled = true)\npublic class Config { }\n",
	}).RoleHierarchy
	if len(h.AssumedReach) != 0 {
		t.Errorf("AssumedReach = %v with fromHierarchy in the tree, want none", h.AssumedReach)
	}
}
