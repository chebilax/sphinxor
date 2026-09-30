package spring

import (
	"sort"
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// Tests for docs/decisions/0040-multiple-security-filter-chains.md §3: a
// SecurityFilterChain rule whose role argument is not a string literal was
// DROPPED — chainRuleFromTerminal returned no rule — so evaluation fell
// through to whatever came after it. Synthetic by necessity: no analyzed
// single-chain corpus repository has such a rule (ADR 0040 §3).

const chainAdminDelete = `package app;
import org.springframework.web.bind.annotation.*;
@RestController
public class C {
    @DeleteMapping("/admin/wipe") public void wipe() { }
}`

func chainConfig(imports, rules string) string {
	return `package app;
import org.springframework.context.annotation.Bean;
import org.springframework.security.web.SecurityFilterChain;
` + imports + `
public class SecurityConfig {
    static final String ROLE_ADMIN = "ADMIN";
    @Bean
    SecurityFilterChain chain(HttpSecurity http) throws Exception {
        http.authorizeHttpRequests(a -> a
` + rules + `);
        return http.build();
    }
}`
}

// urlRoles returns the roles the URL layer grants on the one endpoint, and
// whether it recorded an authentication requirement for it.
func urlRoles(t *testing.T, m *model.Model) (roles []string, authenticated bool) {
	t.Helper()
	if len(m.Endpoints) != 1 {
		t.Fatalf("want 1 endpoint, got %d", len(m.Endpoints))
	}
	guard := map[model.ID]bool{}
	for _, g := range m.GuardApplications {
		if g.AppliedAt == model.ScopeRequestMatcher {
			guard[g.ID] = true
		}
	}
	for _, r := range m.RoleReferences {
		if guard[r.GuardApplicationID] {
			roles = append(roles, r.RawLiteral)
		}
	}
	sort.Strings(roles)
	return roles, len(m.AuthenticationRequirements) > 0
}

// TestChainRule_ConstantRoleIsResolved is the reproduction: an ADMIN-only
// endpoint came out with no guard, because the rule naming ADMIN through
// a constant vanished and anyRequest().permitAll() applied instead.
func TestChainRule_ConstantRoleIsResolved(t *testing.T) {
	m := extractProject(t, map[string]string{
		"SecurityConfig.java": chainConfig("", `            .requestMatchers("/admin/**").hasAnyRole(ROLE_ADMIN)
            .anyRequest().permitAll()`),
		"C.java": chainAdminDelete,
	})
	roles, _ := urlRoles(t, m)
	if len(roles) != 1 || roles[0] != "ADMIN" {
		t.Errorf("URL-layer roles = %v, want [ADMIN] — the constant resolves through ADR 0039's index", roles)
	}
}

// TestChainRule_UnresolvableRoleStopsEvaluation is the fallback. A role
// constant from outside the tree cannot be read, so the rule is kept as
// unrecognized and stops evaluation (ADR 0018). Dropping it let the next
// rule answer — and when that rule is authenticated(), an ADMIN-only
// endpoint was reported as open to any authenticated user, which the
// Cerbos export turns into a grant the application denies.
func TestChainRule_UnresolvableRoleStopsEvaluation(t *testing.T) {
	m := extractProject(t, map[string]string{
		"SecurityConfig.java": chainConfig("import static com.vendor.Roles.OPERATOR;", `            .requestMatchers("/admin/**").hasAnyRole(OPERATOR)
            .anyRequest().authenticated()`),
		"C.java": chainAdminDelete,
	})
	roles, authenticated := urlRoles(t, m)
	if len(roles) != 0 || authenticated {
		t.Errorf("roles = %v, authenticated = %v; want neither — the unreadable rule must stop evaluation, not be skipped", roles, authenticated)
	}
}

// TestChainRule_PartlyReadableRoleListIsUnrecognized: a role list with one
// unreadable member was read as the members that were literals, a partial
// list presented as the whole. All members must resolve, or none is taken.
func TestChainRule_PartlyReadableRoleListIsUnrecognized(t *testing.T) {
	m := extractProject(t, map[string]string{
		"SecurityConfig.java": chainConfig("import static com.vendor.Roles.OPERATOR;", `            .requestMatchers("/admin/**").hasAnyRole("USER", OPERATOR)
            .anyRequest().authenticated()`),
		"C.java": chainAdminDelete,
	})
	roles, authenticated := urlRoles(t, m)
	if len(roles) != 0 || authenticated {
		t.Errorf("roles = %v, authenticated = %v; want neither — [USER] alone would be a partial list", roles, authenticated)
	}
}

// TestMultipleChains_ReasonNamesBlockers is ADR 0040 §5: with several
// chains the URL layer stays unknown, and the reason says what blocks
// reading them — here one chain per blocker, as the corpus shows them.
func TestMultipleChains_ReasonNamesBlockers(t *testing.T) {
	m := extractProject(t, map[string]string{
		"SecurityConfig.java": `package app;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Profile;
import org.springframework.security.web.SecurityFilterChain;
public class SecurityConfig {
    @Bean @Profile("ldap")
    SecurityFilterChain ldap(HttpSecurity http) throws Exception {
        http.authorizeHttpRequests(a -> a.anyRequest().authenticated());
        return http.build();
    }
    @Bean
    SecurityFilterChain api(HttpSecurity http) throws Exception {
        http.securityMatcher(endpointsMatcher());
        if (enabled) {
            http.authorizeHttpRequests(a -> a.anyRequest().authenticated());
        }
        return http.build();
    }
}`,
		"C.java": chainAdminDelete,
	})
	r := m.URLLayer.Reason
	for _, want := range []string{
		"2 SecurityFilterChain beans were found",
		"1 of them conditional on @Profile/@Conditional",
		"1 configuring their rules inside a code branch or a helper method",
		"1 with a securityMatcher that is not string literals",
	} {
		if !strings.Contains(r, want) {
			t.Errorf("URLLayer.Reason = %q, want it to contain %q", r, want)
		}
	}
	if !m.URLLayer.Unknown() {
		t.Error("several chains must still leave the URL layer unknown (ADR 0040, option D)")
	}
}
