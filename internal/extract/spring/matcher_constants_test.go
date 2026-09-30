package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// matcherConstProject is a one-chain project with the given class members
// and authorizeHttpRequests lambda body, and a controller under /api.
func matcherConstProject(t *testing.T, members, body string) *model.Model {
	t.Helper()
	return extractProject(t, map[string]string{
		"SecurityConfig.java": `package app;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.web.SecurityFilterChain;
public class SecurityConfig {
    @Value("${openapi.expose:false}")
    private boolean exposeOpenApiUi;
` + members + `
    @Bean
    SecurityFilterChain chain(HttpSecurity http) throws Exception {
        http.authorizeHttpRequests(auth -> {
` + body + `
        });
        return http.build();
    }
}`,
		"AgentController.java": `package app;
import org.springframework.web.bind.annotation.*;
@RestController
public class AgentController {
    @DeleteMapping("/api/v1/agents/{id}")
    public void delete(@PathVariable String id) { }
}`,
	})
}

// TestMatcherConstants_ArrayConstantIsRead: ADR 0039 Amendment 1, the
// mateclaw shape. A static final String[] of literal springdoc paths, used
// in both arms of a branch, left the matcher unreadable, so its ADMIN arm
// could narrow every endpoint and all of them were unknown. Read, the
// patterns cannot match /api/**, and the endpoint reaches authenticated().
func TestMatcherConstants_ArrayConstantIsRead(t *testing.T) {
	m := matcherConstProject(t, `
    private static final String DOCS = "/v3/api-docs";
    private static final String[] OPENAPI_PATHS = { "/swagger-ui.html", "/swagger-ui/**", DOCS, DOCS + "/**" };`, `
            if (exposeOpenApiUi) {
                auth.requestMatchers(OPENAPI_PATHS).permitAll();
            } else {
                auth.requestMatchers(OPENAPI_PATHS).hasRole("ADMIN");
            }
            auth.requestMatchers("/api/**").authenticated()
                .anyRequest().permitAll();`)
	e := m.Endpoints[0]
	if e.URLRuleUnresolved {
		t.Fatalf("endpoint marked URLRuleUnresolved (line %d); OPENAPI_PATHS is a readable constant array", e.URLRuleLine)
	}
	if len(m.AuthenticationRequirements) != 1 {
		t.Errorf("authentication requirements = %+v; want /api/** authenticated() applied", m.AuthenticationRequirements)
	}
}

// TestMatcherConstants_UnreadableElementStaysUnknown: an array with one
// element that cannot be evaluated leaves the whole matcher unknown
// (ADR 0020 §1), and so does a local array, whose elements can change.
func TestMatcherConstants_UnreadableElementStaysUnknown(t *testing.T) {
	for _, tc := range []struct{ name, members, body string }{
		{"element from a method call", `
    private static final String[] PATHS = { "/swagger-ui/**", System.getenv("DOCS") };`, `
            auth.requestMatchers(PATHS).hasRole("ADMIN");
            auth.anyRequest().authenticated();`},
		{"local array", ``, `
            String[] paths = { "/swagger-ui/**" };
            auth.requestMatchers(paths).hasRole("ADMIN");
            auth.anyRequest().authenticated();`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := matcherConstProject(t, tc.members, tc.body)
			if !m.Endpoints[0].URLRuleUnresolved {
				t.Errorf("endpoint not marked URLRuleUnresolved; the ADMIN matcher cannot be read and may cover it")
			}
		})
	}
}
