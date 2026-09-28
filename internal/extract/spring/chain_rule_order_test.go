package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/export/cerbos"
	"github.com/chebilax/sphinxor/internal/model"
)

// TestChainRules_OneLineChainKeepsDeclarationOrder: ADR 0012 matches
// SecurityFilterChain rules first-match-wins over declaration order. Rules
// were sorted by source line alone, and the tree walk visits a fluent
// chain from its outermost call — the LAST rule — so on one line the
// order was reversed. Here that let anyRequest().authenticated() answer
// for /java/**, and the export granted ADMIN a DELETE whose URL rule
// requires SUPERADMIN: a grant the application denies.
func TestChainRules_OneLineChainKeepsDeclarationOrder(t *testing.T) {
	m := extractProject(t, map[string]string{
		"SecurityConfig.java": `package app;
import org.springframework.context.annotation.Bean;
import org.springframework.security.config.annotation.method.configuration.EnableMethodSecurity;
import org.springframework.security.web.SecurityFilterChain;
@EnableMethodSecurity
public class SecurityConfig {
    @Bean
    SecurityFilterChain filterChain(HttpSecurity http) throws Exception {
        http.authorizeHttpRequests(a -> a.requestMatchers("/java/**").hasRole("SUPERADMIN").anyRequest().authenticated());
        return http.build();
    }
}`,
		"JavaController.java": `package app;
import org.springframework.security.access.prepost.PreAuthorize;
import org.springframework.web.bind.annotation.*;
@RestController
@RequestMapping("/java")
public class JavaController {
    @PreAuthorize("hasRole('ADMIN')")
    @DeleteMapping("/items/{id}")
    public void delete(@PathVariable String id) { }
}`,
	})

	urlGuard := map[model.ID]bool{}
	for _, g := range m.GuardApplications {
		if g.AppliedAt == model.ScopeRequestMatcher {
			urlGuard[g.ID] = true
		}
	}
	var urlRoles []string
	for _, r := range m.RoleReferences {
		if urlGuard[r.GuardApplicationID] {
			urlRoles = append(urlRoles, r.RawLiteral)
		}
	}
	if len(urlRoles) != 1 || urlRoles[0] != "SUPERADMIN" {
		t.Errorf("URL-layer roles = %v, want [SUPERADMIN] — the first declared rule matches /java/**", urlRoles)
	}
	for _, a := range m.AuthenticationRequirements {
		if a.AppliedAt == model.ScopeRequestMatcher {
			t.Errorf("anyRequest().authenticated() answered for /java/items/{id}; it is declared after the /java/** rule")
		}
	}
	if r := cerbos.Translate(m); len(r.Rules) != 0 {
		t.Errorf("exported %+v; method ADMIN and URL SUPERADMIN share no role, so nothing may be granted", r.Rules)
	}
}
