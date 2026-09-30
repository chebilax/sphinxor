package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/export/cerbos"
)

// TestSeveralChainBeans_URLLayerUnknown: ADR 0012 §1 and ADR 0040 read the
// URL layer only when there is exactly one SecurityFilterChain bean. The
// implementation counted authorizeHttpRequests LAMBDAS instead: a project
// whose other chains configure their rules another way (here a method
// reference) had its single lambda read as the whole URL layer. The
// @Order(1) chain requires ADMIN on /admin/**, and the export granted USER
// there from the method layer and the second chain alone — Stirling-PDF's
// four chains produced 98 rules this way.
func TestSeveralChainBeans_URLLayerUnknown(t *testing.T) {
	m := extractProject(t, map[string]string{
		"SecurityConfig.java": `package app;
import org.springframework.context.annotation.Bean;
import org.springframework.core.annotation.Order;
import org.springframework.security.config.annotation.method.configuration.EnableMethodSecurity;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.config.annotation.web.configurers.AuthorizeHttpRequestsConfigurer;
import org.springframework.security.web.SecurityFilterChain;
@EnableMethodSecurity
public class SecurityConfig {
    @Bean
    @Order(1)
    SecurityFilterChain admin(HttpSecurity http) throws Exception {
        http.securityMatcher("/admin/**").authorizeHttpRequests(this::adminRules);
        return http.build();
    }
    private void adminRules(AuthorizeHttpRequestsConfigurer<HttpSecurity>.AuthorizationManagerRequestMatcherRegistry auth) {
        auth.anyRequest().hasRole("ADMIN");
    }
    @Bean
    @Order(2)
    SecurityFilterChain app(HttpSecurity http) throws Exception {
        http.authorizeHttpRequests(auth -> auth.anyRequest().authenticated());
        return http.build();
    }
}`,
		"AdminController.java": `package app;
import org.springframework.security.access.prepost.PreAuthorize;
import org.springframework.web.bind.annotation.*;
@RestController
public class AdminController {
    @PreAuthorize("hasRole('USER')")
    @DeleteMapping("/admin/users/{id}")
    public void delete(@PathVariable String id) { }
}`,
	})
	if !m.URLLayer.Unknown() {
		t.Errorf("URL layer = %+v; two SecurityFilterChain beans must leave it unknown, whatever form each chain's rules take", m.URLLayer)
	}
	r := cerbos.Translate(m)
	if len(r.Rules) != 0 {
		t.Errorf("exported %+v; the /admin/** chain is not read, so nothing may be granted", r.Rules)
	}
	if len(r.Omissions) != 1 || r.Omissions[0].Reason != cerbos.ReasonURLLayerUnknown {
		t.Errorf("omissions = %+v; want the endpoint omitted as %q", r.Omissions, cerbos.ReasonURLLayerUnknown)
	}
}
