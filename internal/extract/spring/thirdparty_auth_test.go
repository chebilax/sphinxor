package spring

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chebilax/sphinxor/internal/lint"
)

// TestThirdPartyAuth_ShiroIsFoundNotAbsent is the core regression for
// docs/decisions/0023-third-party-authorization-annotations.md §1/§2.
//
// Shiro's @RequiresPermissions shares no name with anything Spring uses,
// so before ADR 0023 it never entered the recognized set at all and the
// endpoint fell straight through to "no access control found" — the state
// ADR 0022 §3 established is the wrong thing to report about an endpoint
// that has some. Measured across 20 repositories, that was 907 mutating
// routes.
//
// The genuinely bare sibling is the control: this must be a distinction,
// not blanket suppression.
func TestThirdPartyAuth_ShiroIsFoundNotAbsent(t *testing.T) {
	m, handler := extractOne(t, `
import org.apache.shiro.authz.annotation.RequiresPermissions;

@RestController
@RequestMapping("/api")
public class C {
    @RequiresPermissions("system:user:edit")
    @PostMapping("/shiro-protected")
    public void shiroProtected() {}

    @PostMapping("/genuinely-bare")
    public void genuinelyBare() {}
}
`)
	if len(m.UnrecognizedAuthAnnotations) != 1 {
		t.Fatalf("Shiro annotation should be recorded as unrecognized, got %+v", m.UnrecognizedAuthAnnotations)
	}
	got := m.UnrecognizedAuthAnnotations[0]
	if got.BoundTo != "org.apache.shiro.authz.annotation.RequiresPermissions" {
		t.Errorf("BoundTo = %q, want the Shiro FQN", got.BoundTo)
	}
	// Nothing about what Shiro *requires* is read: "system:user:edit" is
	// not extracted, for the same reason nacos's resource/action pairs
	// are not. ADR 0023 withdraws one claim; it does not widen scope.
	if len(m.GuardApplications) != 0 || len(m.RoleReferences) != 0 {
		t.Errorf("a Shiro annotation must not become a guard or a role, got %+v / %+v",
			m.GuardApplications, m.RoleReferences)
	}

	findings := lint.MutatingEndpointWithoutAccessControl{}.Check(m)
	var flagged []string
	for _, f := range findings {
		flagged = append(flagged, handler[f.SubjectID])
	}
	if len(flagged) != 1 || flagged[0] != "genuinelyBare" {
		t.Errorf("mutating-endpoint-without-access-control fired on %v, want exactly [genuinelyBare]", flagged)
	}
}

// TestThirdPartyAuth_SameNameWithoutTheShiroBinding applies ADR 0022 §1's
// discipline to a name Spring does not use: the binding decides, never
// the spelling. A project-local @RequiresPermissions is not assumed to be
// Shiro's.
//
// This is the first place that discipline is tested outside Spring's own
// names, and it is the reason §1 recognizes by package rather than by a
// list of annotation names.
func TestThirdPartyAuth_SameNameWithoutTheShiroBinding(t *testing.T) {
	m, _ := extractOne(t, `
import com.example.homegrown.RequiresPermissions;

@RestController
public class C {
    @RequiresPermissions("whatever")
    @PostMapping("/a")
    public void a() {}
}
`)
	if len(m.UnrecognizedAuthAnnotations) != 0 {
		t.Errorf("a project-local @RequiresPermissions must not be assumed to be Shiro's, got %+v",
			m.UnrecognizedAuthAnnotations)
	}
	if got := (lint.MutatingEndpointWithoutAccessControl{}).Check(m); len(got) != 1 {
		t.Errorf("it must not suppress the finding either, got %d findings", len(got))
	}
}

// TestThirdPartyAuth_IgnoreAuthDoesNotSuppress pins the rejected
// alternative's failure mode as a regression.
//
// JeecgBoot declares @IgnoreAuth in a package called
// org.jeecg.config.shiro — which is NOT org.apache.shiro.authz.annotation
// — and the annotation *skips* authentication. A name heuristic matching
// "Auth" would have suppressed mutating-endpoint-without-access-control
// on endpoints explicitly declared public, inverting the finding exactly
// where it is correct. That single case is why §1 matches on the package.
func TestThirdPartyAuth_IgnoreAuthDoesNotSuppress(t *testing.T) {
	m, _ := extractOne(t, `
import org.jeecg.config.shiro.IgnoreAuth;

@RestController
public class C {
    @IgnoreAuth
    @PostMapping("/deliberately-public")
    public void deliberatelyPublic() {}
}
`)
	if len(m.UnrecognizedAuthAnnotations) != 0 {
		t.Errorf("@IgnoreAuth is not an authorization annotation, got %+v", m.UnrecognizedAuthAnnotations)
	}
	if got := (lint.MutatingEndpointWithoutAccessControl{}).Check(m); len(got) != 1 {
		t.Errorf("an endpoint marked public must still be flagged, got %d findings", len(got))
	}
}

// TestThirdPartyAuth_WildcardImportIsNotHonored pins ADR 0023 §1's
// on-demand exclusion, which exists because of a bug this test found.
//
// A wildcard says a package is in scope, not which names come from it.
// Since this rule asks "is this annotation in an authorization package?"
// of every annotation in the file, honoring a wildcard bound every
// otherwise-unimported name to it — the first implementation recorded
// @RestController and @PostMapping as Shiro authorization annotations,
// which is the over-capture ADR 0023 rejects the name heuristic for,
// arrived at from the other side.
//
// The cost of excluding it is one mutating route in 20 repositories
// (litemall's AdminIndexController is the only on-demand Shiro import
// among 275), and it falls in the safe direction: that route keeps the
// finding it has today.
func TestThirdPartyAuth_WildcardImportIsNotHonored(t *testing.T) {
	m, _ := extractOne(t, `
import org.apache.shiro.authz.annotation.*;

@RestController
public class C {
    @RequiresPermissions("index:permission:read")
    @PostMapping("/a")
    public void a() {}
}
`)
	if len(m.UnrecognizedAuthAnnotations) != 0 {
		t.Errorf("a wildcard must bind nothing here — it cannot say which names are Shiro's; got %+v",
			m.UnrecognizedAuthAnnotations)
	}
	// And specifically: no ordinary Spring annotation may be swept up.
	for _, u := range m.UnrecognizedAuthAnnotations {
		if u.Name == "RestController" || u.Name == "PostMapping" {
			t.Errorf("@%s was bound to an authorization package by a wildcard — the original bug", u.Name)
		}
	}
}

// TestThirdPartyAuth_EnablerReportedBothWays is ADR 0023 §3. §2 suppresses
// a finding on every endpoint carrying a Shiro annotation; this is what
// tells a reader whether that suppression rests on anything.
//
// Not finding the wiring never changes which findings are suppressed —
// absence is not evidence, exactly as ADR 0015 holds, and Shiro's
// spring-boot starter enables annotations by auto-configuration with no
// Java bean to find.
func TestThirdPartyAuth_EnablerReportedBothWays(t *testing.T) {
	controller := `package app;
import org.apache.shiro.authz.annotation.RequiresPermissions;
import org.springframework.web.bind.annotation.*;

@RestController
public class C {
    @RequiresPermissions("a:b")
    @PostMapping("/a")
    public void a() {}
}
`
	shiroConfig := `package app;
import org.apache.shiro.spring.security.interceptor.AuthorizationAttributeSourceAdvisor;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class ShiroConfig {
    @Bean
    public AuthorizationAttributeSourceAdvisor authorizationAttributeSourceAdvisor() {
        return new AuthorizationAttributeSourceAdvisor();
    }
}
`
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"wiring present", map[string]string{"C.java": controller, "ShiroConfig.java": shiroConfig}, true},
		{"wiring absent", map[string]string{"C.java": controller}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for n, src := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, n), []byte(src), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			m, _, err := Extract(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(m.ThirdPartyAuth) != 1 {
				t.Fatalf("got %d ThirdPartyAuth entries, want 1: %+v", len(m.ThirdPartyAuth), m.ThirdPartyAuth)
			}
			st := m.ThirdPartyAuth[0]
			if st.Framework != "Apache Shiro" || st.Enabler != "AuthorizationAttributeSourceAdvisor" {
				t.Errorf("unexpected status %+v", st)
			}
			if st.EnablerFound != tc.want {
				t.Errorf("EnablerFound = %v, want %v", st.EnablerFound, tc.want)
			}
			// Either way, the suppression is unchanged: absence is not
			// evidence (ADR 0015, applied by ADR 0023 §3).
			if got := (lint.MutatingEndpointWithoutAccessControl{}).Check(m); len(got) != 0 {
				t.Errorf("suppression must not depend on the wiring being found, got %d findings", len(got))
			}
		})
	}
}

// TestThirdPartyAuth_NoShiroNoStatus keeps the caveat off healthy code: a
// project with no Shiro annotations has nothing to say about Shiro's
// wiring, and a warning that fires on projects with nothing wrong is the
// kind that trains people to stop reading warnings.
func TestThirdPartyAuth_NoShiroNoStatus(t *testing.T) {
	dir := t.TempDir()
	src := `package app;
import org.springframework.security.access.prepost.PreAuthorize;
import org.springframework.web.bind.annotation.*;

@RestController
public class C {
    @PreAuthorize("hasRole('ADMIN')")
    @PostMapping("/a")
    public void a() {}
}
`
	if err := os.WriteFile(filepath.Join(dir, "C.java"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _, err := Extract(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.ThirdPartyAuth) != 0 {
		t.Errorf("a project with no Shiro annotations must report no Shiro status, got %+v", m.ThirdPartyAuth)
	}
}
