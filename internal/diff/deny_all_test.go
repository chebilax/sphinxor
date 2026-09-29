package diff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chebilax/sphinxor/internal/extract/spring"
	"github.com/chebilax/sphinxor/internal/lint"
)

// TestDenyAll_RemovedIsBecamePublic is ADR 0012 Amendment 1 for the diff:
// an endpoint behind a SecurityFilterChain denyAll() is protected, so a
// change that drops the rule, leaving the endpoint with nothing, loses
// protection and must trip ADR 0036's became-public gate.
func TestDenyAll_RemovedIsBecamePublic(t *testing.T) {
	controller := `package app;
import org.springframework.web.bind.annotation.*;
@RestController
public class C {
    @DeleteMapping("/admin/wipe")
    public void wipe() { }
}`
	chain := func(rules string) string {
		return `package app;
import org.springframework.context.annotation.Bean;
import org.springframework.security.web.SecurityFilterChain;
public class SecurityConfig {
    @Bean
    SecurityFilterChain chain(HttpSecurity http) throws Exception {
        http.authorizeHttpRequests(a -> a` + rules + `);
        return http.build();
    }
}`
	}
	project := func(rules string) Snapshot {
		dir := t.TempDir()
		for name, body := range map[string]string{"C.java": controller, "SecurityConfig.java": chain(rules)} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		m, outcome, err := spring.Extract(dir)
		if err != nil {
			t.Fatal(err)
		}
		return Snapshot{Model: m, Findings: lint.Run(m, lint.DefaultRules(), outcome.AllowlistedEndpoints), AllowlistedEndpoints: outcome.AllowlistedEndpoints}
	}

	base := project(`.requestMatchers("/admin/**").denyAll().anyRequest().permitAll()`)
	head := project(`.anyRequest().permitAll()`)
	r := Compare(base, head)
	if len(r.BecamePublic) != 1 || !r.HasRegressions() {
		t.Fatalf("denyAll() → nothing must be became-public and gate; BecamePublic=%+v regressions=%+v", r.BecamePublic, r.Regressions)
	}
}

// TestPermitAll_ReplacingAGuardIsBecamePublic pins why @PermitAll is
// announced rather than recorded on the endpoint: a guard replaced by
// @PermitAll must still trip the became-public gate.
func TestPermitAll_ReplacingAGuardIsBecamePublic(t *testing.T) {
	project := func(imp, ann string) Snapshot {
		dir := t.TempDir()
		src := "package app;\n" + imp + "\nimport org.springframework.web.bind.annotation.*;\n@RestController\npublic class C {\n    " +
			ann + "\n    @DeleteMapping(\"/admin/wipe\")\n    public void wipe() { }\n}\n"
		if err := os.WriteFile(filepath.Join(dir, "C.java"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		m, outcome, err := spring.Extract(dir)
		if err != nil {
			t.Fatal(err)
		}
		return Snapshot{Model: m, Findings: lint.Run(m, lint.DefaultRules(), outcome.AllowlistedEndpoints), AllowlistedEndpoints: outcome.AllowlistedEndpoints}
	}
	base := project("import org.springframework.security.access.prepost.PreAuthorize;", `@PreAuthorize("hasRole('ADMIN')")`)
	head := project("import jakarta.annotation.security.PermitAll;", "@PermitAll")
	r := Compare(base, head)
	if len(r.BecamePublic) != 1 || !r.HasRegressions() {
		t.Fatalf("a guard replaced by @PermitAll must gate; BecamePublic=%+v regressions=%+v", r.BecamePublic, r.Regressions)
	}
}

// TestPermissionWidenedToPermitAllGates pins ADR 0037 §3's recorded case,
// which until now was pinned only in that ADR's table of constructed runs:
// @PreAuthorize("@ss.hasPermi('x')") becoming @PreAuthorize("permitAll()")
// keeps a guard on both sides, so the became-public gate does not see it,
// and it fails the build only through the new empty-role finding. The
// forthcoming permit-all ADR must keep this failing.
func TestPermissionWidenedToPermitAllGates(t *testing.T) {
	project := func(expr string) Snapshot {
		dir := t.TempDir()
		src := "package app;\nimport org.springframework.security.access.prepost.PreAuthorize;\n" +
			"import org.springframework.web.bind.annotation.*;\n@RestController\npublic class C {\n" +
			"    @PreAuthorize(\"" + expr + "\")\n    @DeleteMapping(\"/system/user\")\n    public void remove() { }\n}\n"
		if err := os.WriteFile(filepath.Join(dir, "C.java"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		m, outcome, err := spring.Extract(dir)
		if err != nil {
			t.Fatal(err)
		}
		return Snapshot{Model: m, Findings: lint.Run(m, lint.DefaultRules(), outcome.AllowlistedEndpoints), AllowlistedEndpoints: outcome.AllowlistedEndpoints}
	}
	r := Compare(project("@ss.hasPermi('system:user:remove')"), project("permitAll()"))
	if !r.HasRegressions() {
		t.Fatalf("a permission widened to permitAll() must fail the build (ADR 0037 §3); regressions=%+v", r.Regressions)
	}
	// Since ADR 0043 it fails as what it is, a loss of protection.
	if len(r.BecamePublic) != 1 {
		t.Errorf("BecamePublic = %+v, want the endpoint (ADR 0043)", r.BecamePublic)
	}
}

// TestPermitAll_ReplacingProtectionIsBecamePublic is ADR 0043 §2's gate
// row: a permit-all is not a guard, so any protection replaced by one is
// lost protection. Before ADR 0043, @Secured → permitAll() also failed the
// build, but only because the head gained an empty-role; checked by running
// this test against the previous code. It now fails as became-public.
func TestPermitAll_ReplacingProtectionIsBecamePublic(t *testing.T) {
	project := func(imp, ann string) Snapshot {
		dir := t.TempDir()
		src := "package app;\n" + imp + "\nimport org.springframework.web.bind.annotation.*;\n@RestController\npublic class C {\n    " +
			ann + "\n    @DeleteMapping(\"/admin/wipe\")\n    public void wipe() { }\n}\n"
		if err := os.WriteFile(filepath.Join(dir, "C.java"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		m, outcome, err := spring.Extract(dir)
		if err != nil {
			t.Fatal(err)
		}
		return Snapshot{Model: m, Findings: lint.Run(m, lint.DefaultRules(), outcome.AllowlistedEndpoints), AllowlistedEndpoints: outcome.AllowlistedEndpoints}
	}
	for _, tc := range []struct{ name, baseImp, baseAnn, headImp, headAnn string }{
		{"@Secured → permitAll()", "import org.springframework.security.access.annotation.Secured;", `@Secured("ROLE_ADMIN")`,
			"import org.springframework.security.access.prepost.PreAuthorize;", `@PreAuthorize("permitAll()")`},
		{"@RolesAllowed → @PermitAll", "import jakarta.annotation.security.RolesAllowed;", `@RolesAllowed("ADMIN")`,
			"import jakarta.annotation.security.PermitAll;", "@PermitAll"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Compare(project(tc.baseImp, tc.baseAnn), project(tc.headImp, tc.headAnn))
			if len(r.BecamePublic) != 1 || !r.HasRegressions() {
				t.Fatalf("want became-public and a gate; BecamePublic=%+v regressions=%+v", r.BecamePublic, r.Regressions)
			}
			if len(r.AddedPublicDeclarations) != 1 {
				t.Errorf("AddedPublicDeclarations = %+v, want the new declaration listed", r.AddedPublicDeclarations)
			}
		})
	}
}
