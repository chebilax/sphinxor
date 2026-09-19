package spring

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// writeJavaProject lays out a minimal Spring source tree in a temp dir.
// These cases are constructed, not vendored, and labelled as such: each
// reproduces a configuration shape found during the ADR 0020 blind-spot
// audit, reduced to the smallest source that still triggers it.
func writeJavaProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, "src", "main", "java", "app", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const twoEndpointControllers = `package app;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/admin")
class AdminController {
    @DeleteMapping("/wipe")
    public void wipe() { }
}

@RestController
@RequestMapping("/public")
class PublicController {
    @DeleteMapping("/wipe")
    public void wipe() { }
}
`

// rolesFor returns every URL-layer role literal attached to the endpoint.
func rolesFor(t *testing.T, m *model.Model, method model.HTTPMethod, path string) []string {
	t.Helper()
	e, ok := findEndpoint(m.Endpoints, method, path)
	if !ok {
		t.Fatalf("missing endpoint %s %s", method, path)
	}
	guards := map[model.ID]bool{}
	for _, g := range m.GuardApplications {
		if g.EndpointID == e.ID {
			guards[g.ID] = true
		}
	}
	var out []string
	for _, r := range m.RoleReferences {
		if guards[r.GuardApplicationID] {
			out = append(out, r.RawLiteral)
		}
	}
	return out
}

// TestUnreadableMatcher_DoesNotBecomeMatchAll is ADR 0020 §1's regression
// test, and the most important test in this package.
//
// Before the fix, requestMatchersArgs returned nil patterns whenever no
// string literal could be extracted, and nil patterns was *also* the
// sentinel for .anyRequest() — so a matcher Sphinxor could not read
// silently became a rule matching every path. This exact configuration
// reported DELETE /public/wipe, which really falls through to
// .anyRequest().permitAll() and is callable by anyone, as ADMIN-protected
// with zero findings: a false assurance on a destructive endpoint, with
// the mutating-endpoint safety net suppressed at the same time.
//
// Confirmed to fail against that behavior before being kept.
func TestUnreadableMatcher_DoesNotBecomeMatchAll(t *testing.T) {
	dir := writeJavaProject(t, map[string]string{
		"Controllers.java": twoEndpointControllers,
		"SecurityConfig.java": `package app;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.web.SecurityFilterChain;
import org.springframework.security.web.util.matcher.RegexRequestMatcher;

@Configuration
public class SecurityConfig {
    @Bean
    public SecurityFilterChain chain(HttpSecurity http) throws Exception {
        return http.authorizeHttpRequests(authorize -> authorize
                .requestMatchers(RegexRequestMatcher.regexMatcher("^/admin/.*$")).hasRole("ADMIN")
                .anyRequest().permitAll())
            .build();
    }
}
`,
	})

	m, _, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// The endpoint the unreadable rule does NOT cover must never inherit
	// its roles. This is the assertion that bites: it returned [ADMIN].
	if got := rolesFor(t, m, model.MethodDelete, "/public/wipe"); len(got) != 0 {
		t.Errorf("DELETE /public/wipe roles = %v, want none — it really falls through to permitAll(); "+
			"an unreadable matcher must not grant its roles to every endpoint", got)
	}

	// The rule is opaque, so the endpoint it may cover is unresolved too —
	// unknown, not a confident answer in either direction.
	if got := rolesFor(t, m, model.MethodDelete, "/admin/wipe"); len(got) != 0 {
		t.Errorf("DELETE /admin/wipe roles = %v, want none: the matcher is unreadable, so its scope is unknown", got)
	}
}

// TestAntMatcherWrapper_IsRead is the other half of ADR 0020 §1. Making
// unreadable matchers unknown is only safe to ship alongside reading the
// wrapper forms that carry a perfectly ordinary pattern one call deeper:
// requestMatchers(antMatcher("/admin/**")) is idiomatic Spring Security 6,
// and treating it as unknown would trade a silent wrong answer for a
// large, needless loss of honest coverage.
func TestAntMatcherWrapper_IsRead(t *testing.T) {
	dir := writeJavaProject(t, map[string]string{
		"Controllers.java": twoEndpointControllers,
		"SecurityConfig.java": `package app;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.web.SecurityFilterChain;
import static org.springframework.security.web.util.matcher.AntPathRequestMatcher.antMatcher;

@Configuration
public class SecurityConfig {
    @Bean
    public SecurityFilterChain chain(HttpSecurity http) throws Exception {
        return http.authorizeHttpRequests(authorize -> authorize
                .requestMatchers(antMatcher("/admin/**")).hasRole("ADMIN")
                .anyRequest().permitAll())
            .build();
    }
}
`,
	})

	m, _, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	got := rolesFor(t, m, model.MethodDelete, "/admin/wipe")
	if len(got) != 1 || got[0] != "ADMIN" {
		t.Errorf("DELETE /admin/wipe roles = %v, want [ADMIN] — the pattern is readable inside antMatcher(...)", got)
	}
	if got := rolesFor(t, m, model.MethodDelete, "/public/wipe"); len(got) != 0 {
		t.Errorf("DELETE /public/wipe roles = %v, want none (permitAll)", got)
	}
}

// TestAnyRequest_StillMatchesEverything guards the sentinel replacement
// itself: .anyRequest() genuinely does match every path, and separating
// it from "no pattern could be read" must not have broken it.
func TestAnyRequest_StillMatchesEverything(t *testing.T) {
	dir := writeJavaProject(t, map[string]string{
		"Controllers.java": twoEndpointControllers,
		"SecurityConfig.java": `package app;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.web.SecurityFilterChain;

@Configuration
public class SecurityConfig {
    @Bean
    public SecurityFilterChain chain(HttpSecurity http) throws Exception {
        return http.authorizeHttpRequests(authorize -> authorize
                .anyRequest().hasRole("ADMIN"))
            .build();
    }
}
`,
	})

	m, _, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	for _, path := range []string{"/admin/wipe", "/public/wipe"} {
		got := rolesFor(t, m, model.MethodDelete, path)
		if len(got) != 1 || got[0] != "ADMIN" {
			t.Errorf("DELETE %s roles = %v, want [ADMIN] via anyRequest()", path, got)
		}
	}
}
