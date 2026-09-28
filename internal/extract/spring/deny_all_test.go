package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/export/cerbos"
	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
)

// Tests for ADR 0012 Amendment 1: denyAll() admits no one, at either
// layer, and each consumer reads it that way. Synthetic by necessity: no
// analyzed chain and no @PreAuthorize in the corpus uses denyAll().

const denyAllController = `package app;
import org.springframework.security.access.prepost.PreAuthorize;
import org.springframework.web.bind.annotation.*;
@RestController
@RequestMapping("/admin")
public class C {
    @PreAuthorize("hasRole('ADMIN')")
    @DeleteMapping("/wipe")
    public void wipe() { }
}`

func denyAllChain(rules string) string {
	return `package app;
import org.springframework.context.annotation.Bean;
import org.springframework.security.config.annotation.method.configuration.EnableMethodSecurity;
import org.springframework.security.web.SecurityFilterChain;
@EnableMethodSecurity
public class SecurityConfig {
    @Bean
    SecurityFilterChain chain(HttpSecurity http) throws Exception {
        http.authorizeHttpRequests(a -> a
` + rules + `);
        return http.build();
    }
}`
}

func findingsFor(m *model.Model, rule string) int {
	n := 0
	for _, f := range lint.Run(m, lint.DefaultRules(), nil) {
		if f.RuleID == rule {
			n++
		}
	}
	return n
}

func onlyOmission(t *testing.T, m *model.Model) cerbos.Omission {
	t.Helper()
	r := cerbos.Translate(m)
	if len(r.Rules) != 0 || len(r.Omissions) != 1 {
		t.Fatalf("want exactly one omission and no rule, got rules %+v omissions %+v", r.Rules, r.Omissions)
	}
	return r.Omissions[0]
}

// TestDenyAll_URLLayer: a SecurityFilterChain denyAll() is a guard that
// admits no one. Before, it was "no requirement", and the export granted
// the method layer's ADMIN — a grant the application refuses everyone.
func TestDenyAll_URLLayer(t *testing.T) {
	m := extractProject(t, map[string]string{
		"SecurityConfig.java": denyAllChain(`            .requestMatchers("/admin/**").denyAll()
            .anyRequest().authenticated()`),
		"C.java": denyAllController,
	})
	found := false
	for _, g := range m.GuardApplications {
		if g.DeniesAll && g.AppliedAt == model.ScopeRequestMatcher && !g.DeclaresRoles {
			found = true
		}
	}
	if !found {
		t.Fatalf("no URL-layer DeniesAll guard recorded: %+v", m.GuardApplications)
	}
	if n := findingsFor(m, "mutating-endpoint-without-access-control"); n != 0 {
		t.Errorf("a DELETE behind denyAll() is maximally protected, but the mutating finding fired %d time(s)", n)
	}
	if o := onlyOmission(t, m); o.Reason != cerbos.ReasonDeniedToAll {
		t.Errorf("omission = %s, want %s", o.Reason, cerbos.ReasonDeniedToAll)
	}
}

// TestDenyAll_MethodLayer settles the question ADR 0017 left open, for
// denyAll() only: it no longer raises empty-role — a deliberate maximal
// restriction is not a role check left empty — and it protects the
// endpoint. permitAll() is unchanged (next test).
func TestDenyAll_MethodLayer(t *testing.T) {
	m := extractProject(t, map[string]string{
		"C.java": `package app;
import org.springframework.security.access.prepost.PreAuthorize;
import org.springframework.web.bind.annotation.*;
@RestController
public class C {
    @PreAuthorize("denyAll()")
    @DeleteMapping("/admin/wipe")
    public void wipe() { }
}`,
		"Config.java": "package app;\n@EnableMethodSecurity\npublic class Config { }\n",
	})
	if n := findingsFor(m, "empty-role"); n != 0 {
		t.Errorf("@PreAuthorize(\"denyAll()\") raised empty-role %d time(s)", n)
	}
	if n := findingsFor(m, "mutating-endpoint-without-access-control"); n != 0 {
		t.Errorf("@PreAuthorize(\"denyAll()\") raised the mutating finding %d time(s)", n)
	}
	if o := onlyOmission(t, m); o.Reason != cerbos.ReasonDeniedToAll {
		t.Errorf("omission = %s, want %s", o.Reason, cerbos.ReasonDeniedToAll)
	}
}

// TestPermitAll_Unchanged pins what the amendment leaves alone: a
// method-level permitAll() still raises empty-role (ADR 0017, still open),
// and a URL-layer permitAll() still contributes no requirement (ADR 0012
// §1), so the method layer's ADMIN is exported.
func TestPermitAll_Unchanged(t *testing.T) {
	m := extractProject(t, map[string]string{
		"C.java": `package app;
import org.springframework.security.access.prepost.PreAuthorize;
import org.springframework.web.bind.annotation.*;
@RestController
public class C {
    @PreAuthorize("permitAll()")
    @DeleteMapping("/open")
    public void open() { }
}`,
		"Config.java": "package app;\n@EnableMethodSecurity\npublic class Config { }\n",
	})
	if n := findingsFor(m, "empty-role"); n != 1 {
		t.Errorf("method permitAll(): empty-role fired %d time(s), want 1 (unchanged)", n)
	}
	m = extractProject(t, map[string]string{
		"SecurityConfig.java": denyAllChain(`            .requestMatchers("/admin/**").permitAll()
            .anyRequest().authenticated()`),
		"C.java": denyAllController,
	})
	if r := cerbos.Translate(m); len(r.Rules) != 1 || r.Rules[0].Roles[0] != "ADMIN" {
		t.Errorf("URL permitAll(): want the method layer's ADMIN exported (unchanged), got %+v", r.Rules)
	}
}

func jsr250Controller(imp, ann string) string {
	return "package app;\n" + imp + "\nimport org.springframework.web.bind.annotation.*;\n@RestController\npublic class C {\n    " +
		ann + "\n    @DeleteMapping(\"/admin/wipe\")\n    public void wipe() { }\n}\n"
}

// TestJSR250DenyAll: @DenyAll is what denyAll() is, bound by its import
// (ADR 0022) and gated by jsr250Enabled (ADR 0015).
func TestJSR250DenyAll(t *testing.T) {
	enabled := "package app;\n@EnableMethodSecurity(jsr250Enabled = true)\npublic class Config { }\n"
	for _, imp := range []string{"import jakarta.annotation.security.DenyAll;", "import javax.annotation.security.DenyAll;"} {
		m := extractProject(t, map[string]string{"C.java": jsr250Controller(imp, "@DenyAll"), "Config.java": enabled})
		if n := findingsFor(m, "mutating-endpoint-without-access-control"); n != 0 {
			t.Errorf("%s: a DELETE behind @DenyAll raised the mutating finding", imp)
		}
		if o := onlyOmission(t, m); o.Reason != cerbos.ReasonDeniedToAll {
			t.Errorf("%s: omission = %s, want denied-to-all", imp, o.Reason)
		}
	}

	// @EnableMethodSecurity leaves jsr250Enabled false by default: @DenyAll
	// is then confirmed inert and protects nothing (ADR 0015).
	m := extractProject(t, map[string]string{
		"C.java":      jsr250Controller("import jakarta.annotation.security.DenyAll;", "@DenyAll"),
		"Config.java": "package app;\n@EnableMethodSecurity\npublic class Config { }\n",
	})
	if n := findingsFor(m, "mutating-endpoint-without-access-control"); n != 1 {
		t.Errorf("an inert @DenyAll (jsr250Enabled false) must not protect: mutating finding fired %d time(s), want 1", n)
	}

	// A project's own @DenyAll is not JSR-250's.
	m = extractProject(t, map[string]string{"C.java": jsr250Controller("import com.example.security.DenyAll;", "@DenyAll"), "Config.java": enabled})
	for _, g := range m.GuardApplications {
		if g.DeniesAll {
			t.Errorf("a foreign @DenyAll was read as JSR-250's: %+v", g)
		}
	}
}

// TestJSR250PermitAll_AnnouncedNotRead: @PermitAll is counted and
// announced, and nothing attaches it to the endpoint — its meaning is not
// decided. The endpoint is analyzed as unannotated: a mutating one gets
// the Low finding, and no guard means the became-public gate still sees a
// guard replaced by @PermitAll (tested in internal/diff).
func TestJSR250PermitAll_AnnouncedNotRead(t *testing.T) {
	m := extractProject(t, map[string]string{"C.java": jsr250Controller("import jakarta.annotation.security.PermitAll;", "@PermitAll")})
	if m.PermitAll.Count != 1 || len(m.PermitAll.Classes) != 1 || m.PermitAll.Classes[0] != "C" {
		t.Errorf("PermitAll = %+v, want one, in C", m.PermitAll)
	}
	if len(m.GuardApplications) != 0 || len(m.UnrecognizedAuthAnnotations) != 0 {
		t.Errorf("@PermitAll must attach nothing to the endpoint: guards %+v, unrecognized %+v", m.GuardApplications, m.UnrecognizedAuthAnnotations)
	}
	if n := findingsFor(m, "mutating-endpoint-without-access-control"); n != 1 {
		t.Errorf("mutating finding fired %d time(s), want 1", n)
	}
	if n := findingsFor(m, "empty-role"); n != 0 {
		t.Errorf("empty-role fired %d time(s) on @PermitAll, want 0", n)
	}
	m = extractProject(t, map[string]string{"C.java": jsr250Controller("import com.example.security.PermitAll;", "@PermitAll")})
	if m.PermitAll.Count != 0 {
		t.Errorf("a project's own @PermitAll was counted: %+v", m.PermitAll)
	}
}
