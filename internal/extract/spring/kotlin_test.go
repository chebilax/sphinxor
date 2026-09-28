package spring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/export/cerbos"
)

// Tests for docs/decisions/0042-kotlin-sources-announced.md. Synthetic by
// necessity: the 20-repository corpus has no Kotlin, and the sample's
// Kotlin projects are copyleft (ADR 0005's note), so none is vendored.

func kotlinTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestKotlin_DeclarationNotMention is ADR 0027's rule, which ADR 0042
// adopts: a SecurityFilterChain named in a comment, an import or a string
// must not announce Kotlin security configuration. Only a declaration does.
func TestKotlin_DeclarationNotMention(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      int
	}{
		{"line comment", "package app\n// fun filterChain(http: HttpSecurity): SecurityFilterChain\nclass A\n", 0},
		{"block comment, nested", "package app\n/* outer /* inner */ fun filterChain(http: HttpSecurity): SecurityFilterChain */\nclass A\n", 0},
		{"import", "package app\nimport org.springframework.security.web.SecurityFilterChain\nclass A\n", 0},
		{"string", "package app\nval s = \"fun chain(http: HttpSecurity): SecurityFilterChain\"\nclass A\n", 0},
		{"raw string", "package app\nval s = \"\"\"\nfun chain(http: HttpSecurity): SecurityFilterChain\n\"\"\"\nclass A\n", 0},
		{"a declared chain bean", "package app\nclass C {\n  @Bean\n  fun filterChain(http: HttpSecurity): SecurityFilterChain {\n    return http.build()\n  }\n}\n", 1},
		{"a declared adapter", "package app\nclass C : WebSecurityConfigurerAdapter() {\n}\n", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, err := ScanKotlin(kotlinTree(t, map[string]string{"src/main/kotlin/app/A.kt": tc.src}))
			if err != nil {
				t.Fatal(err)
			}
			if st.URLSecurityFiles != tc.want {
				t.Errorf("URLSecurityFiles = %d, want %d", st.URLSecurityFiles, tc.want)
			}
		})
	}
}

// TestKotlin_ControllersAndEnablers applies the same rule to controllers
// and method-security enablers, and keeps the Java walk's exclusions.
func TestKotlin_ControllersAndEnablers(t *testing.T) {
	st, err := ScanKotlin(kotlinTree(t, map[string]string{
		"src/main/kotlin/app/Api.kt":      "package app\n@RestController\nclass Api\n",
		"src/main/kotlin/app/Mention.kt":  "package app\n// @RestController would go here\nval x = \"@EnableMethodSecurity\"\nclass M\n",
		"src/main/kotlin/app/Config.kt":   "package app\n@EnableMethodSecurity\nclass Config\n",
		"src/test/kotlin/app/ApiTest.kt":  "package app\n@RestController\nclass T\n",
		"src/main/kotlin/app/FooTests.kt": "package app\n@RestController\nclass F\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if st.Files != 3 || st.ControllerFiles != 1 || st.MethodEnablerFiles != 1 || st.URLSecurityFiles != 0 {
		t.Errorf("status = %+v, want 3 files, 1 controller file, 1 enabler file, 0 URL security", st)
	}
}

// TestKotlin_SecurityConfigMakesTheURLLayerUnknown is ADR 0042 §3 end to
// end: the constructed mixed project whose Kotlin chain requires
// SUPERADMIN on /java/**. Before, the chain was unseen and the export
// granted the Java DELETE to ADMIN — a grant the application denies.
func TestKotlin_SecurityConfigMakesTheURLLayerUnknown(t *testing.T) {
	dir := kotlinTree(t, map[string]string{
		"src/main/java/app/JavaController.java": `package app;
import org.springframework.security.access.prepost.PreAuthorize;
import org.springframework.web.bind.annotation.*;
@RestController
@RequestMapping("/java")
public class JavaController {
    @PreAuthorize("hasRole('ADMIN')")
    @DeleteMapping("/items/{id}")
    public void delete(@PathVariable String id) { }
}`,
		"src/main/kotlin/app/SecurityConfig.kt": `package app
import org.springframework.security.web.SecurityFilterChain
@Configuration
@EnableMethodSecurity
class SecurityConfig {
    @Bean
    fun filterChain(http: HttpSecurity): SecurityFilterChain {
        http.authorizeHttpRequests {
            it.requestMatchers("/java/**").hasRole("SUPERADMIN")
              .anyRequest().authenticated()
        }
        return http.build()
    }
}`,
	})
	m, _, err := Extract(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !m.URLLayer.Unknown() || !strings.Contains(m.URLLayer.Reason, "Kotlin") {
		t.Errorf("URLLayer = %+v, want unknown, naming Kotlin", m.URLLayer)
	}
	if r := cerbos.Translate(m); len(r.Rules) != 0 {
		t.Errorf("exported %+v; an unknown URL layer must export nothing", r.Rules)
	}
}
