package spring

import (
	"sort"
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/export/cerbos"
	"github.com/chebilax/sphinxor/internal/model"
)

// chainBranchProject is a one-chain project whose authorizeHttpRequests
// lambda body is given by the caller, with one controller under /reports.
func chainBranchProject(t *testing.T, lambdaBody, extraMembers string) *model.Model {
	t.Helper()
	return extractProject(t, map[string]string{
		"SecurityConfig.java": `package app;
import java.util.List;
import org.springframework.http.HttpMethod;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.web.SecurityFilterChain;
public class SecurityConfig {
    @Value("${reports.open:false}")
    private boolean reportsOpen;
    @Bean
    SecurityFilterChain chain(HttpSecurity http) throws Exception {
        http.authorizeHttpRequests(auth -> {
` + lambdaBody + `
        });
        return http.build();
    }
` + extraMembers + `
}`,
		"ReportController.java": `package app;
import org.springframework.web.bind.annotation.*;
@RestController
public class ReportController {
    @GetMapping("/reports/{id}")
    public String get(@PathVariable String id) { return id; }
    @DeleteMapping("/reports/{id}")
    public void delete(@PathVariable String id) { }
}`,
	})
}

// urlRolesByEndpoint lists the URL-layer roles attached to each endpoint.
func urlRolesByEndpoint(m *model.Model) map[string]string {
	path := map[model.ID]string{}
	for _, e := range m.Endpoints {
		path[e.ID] = string(e.HTTPMethod) + " " + e.Path
	}
	guardEndpoint := map[model.ID]string{}
	for _, g := range m.GuardApplications {
		if g.AppliedAt == model.ScopeRequestMatcher {
			guardEndpoint[g.ID] = path[g.EndpointID]
		}
	}
	roles := map[string][]string{}
	for _, r := range m.RoleReferences {
		if ep, ok := guardEndpoint[r.GuardApplicationID]; ok {
			roles[ep] = append(roles[ep], r.RawLiteral)
		}
	}
	out := map[string]string{}
	for ep, rs := range roles {
		sort.Strings(rs)
		out[ep] = strings.Join(rs, ",")
	}
	return out
}

// TestChainRules_RuleInsideABranchIsUnknown: ADR 0040 §4/§5 decided that a
// rule configured inside a code branch is decided at runtime, so Sphinxor
// cannot say whether it applies. The check only looked for a branch AROUND
// the authorizeHttpRequests call. A branch INSIDE its lambda was walked as
// straight-line code: both arms flattened into one list, the first arm
// won first-match, and the export granted USER on DELETE /reports/{id}
// while the default configuration (reports.open=false) requires ADMIN.
//
// Each shape below puts the /reports rule somewhere other than straight-line
// code at the lambda's top level. The rule is then unknown: it still stops
// evaluation (ADR 0018), contributes nothing, and the export omits the
// endpoints rather than granting them.
func TestChainRules_RuleInsideABranchIsUnknown(t *testing.T) {
	for _, tc := range []struct {
		name, body, members string
	}{
		{name: "if/else", body: `
            if (reportsOpen) {
                auth.requestMatchers("/reports/**").hasRole("USER");
            } else {
                auth.requestMatchers("/reports/**").hasRole("ADMIN");
            }
            auth.anyRequest().authenticated();`},
		{name: "if without else", body: `
            if (reportsOpen) {
                auth.requestMatchers("/reports/**").hasRole("USER");
            }
            auth.anyRequest().hasRole("ADMIN");`},
		{name: "switch", body: `
            switch (System.getenv("MODE")) {
                case "open" -> auth.requestMatchers("/reports/**").hasRole("USER");
                default -> auth.requestMatchers("/reports/**").hasRole("ADMIN");
            }`},
		{name: "loop", body: `
            for (int i = 0; i < 1; i++) {
                auth.requestMatchers("/reports/**").hasRole("USER");
            }
            auth.anyRequest().hasRole("ADMIN");`},
		{name: "ternary", body: `
            var rule = reportsOpen
                ? auth.requestMatchers("/reports/**").hasRole("USER")
                : auth.requestMatchers("/reports/**").hasRole("ADMIN");`},
		{name: "try", body: `
            try {
                auth.requestMatchers("/reports/**").hasRole("USER");
            } catch (RuntimeException e) {
                auth.requestMatchers("/reports/**").hasRole("ADMIN");
            }`},
		{name: "nested lambda", body: `
            List.of("USER").forEach(r -> auth.requestMatchers("/reports/**").hasRole(r));
            auth.anyRequest().hasRole("ADMIN");`},
		// The helper's rules were never read at all: the walk stops at the
		// lambda. Here the helper restricts /reports to ADMIN and the lambda
		// then grants USER everything else, so skipping the helper let
		// anyRequest() answer for /reports.
		{name: "rule added in a helper method", body: `
            configureReports(auth);
            auth.anyRequest().hasRole("USER");`,
			members: `
    private void configureReports(org.springframework.security.config.annotation.web.configurers.AuthorizeHttpRequestsConfigurer<HttpSecurity>.AuthorizationManagerRequestMatcherRegistry auth) {
        auth.requestMatchers("/reports/**").hasRole("ADMIN");
    }`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := chainBranchProject(t, tc.body, tc.members)
			if got := urlRolesByEndpoint(m); len(got) != 0 {
				t.Errorf("URL-layer roles %v; a rule that is not straight-line code is unknown and must stop evaluation with nothing attached", got)
			}
			if r := cerbos.Translate(m); len(r.Rules) != 0 {
				t.Errorf("exported %+v; the /reports rule is decided at runtime, so nothing may be granted", r.Rules)
			}
		})
	}
}

// TestChainRules_BranchOnlyAffectsWhatItMatches: an unknown rule stops
// evaluation only for endpoints it could match (ADR 0018). A branched rule
// on another path leaves the straight-line /reports rule in force, which
// is what keeps this fix from blanking every chain that has a branch.
func TestChainRules_BranchOnlyAffectsWhatItMatches(t *testing.T) {
	m := chainBranchProject(t, `
            if (reportsOpen) {
                auth.requestMatchers("/public/**").permitAll();
            }
            auth.requestMatchers("/reports/**").hasRole("ADMIN");
            auth.anyRequest().authenticated();`, "")
	want := map[string]string{"GET /reports/{id}": "ADMIN", "DELETE /reports/{id}": "ADMIN"}
	got := urlRolesByEndpoint(m)
	if len(got) != len(want) {
		t.Fatalf("URL-layer roles %v, want %v", got, want)
	}
	for ep, roles := range want {
		if got[ep] != roles {
			t.Errorf("%s: URL-layer roles %q, want %q", ep, got[ep], roles)
		}
	}
}
