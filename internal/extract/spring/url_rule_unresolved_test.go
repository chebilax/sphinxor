package spring

import (
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/diff"
	"github.com/chebilax/sphinxor/internal/export/cerbos"
	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
	"github.com/chebilax/sphinxor/internal/report"
)

// urlRuleProject is a one-chain project with the given authorizeHttpRequests
// lambda body, and a controller under /reports: GET guarded by
// @PreAuthorize("hasRole('USER')"), DELETE with nothing on its method.
func urlRuleProject(t *testing.T, lambdaBody string, getGuarded bool) *model.Model {
	t.Helper()
	guard := ""
	if getGuarded {
		guard = `@PreAuthorize("hasRole('USER')")`
	}
	return extractProject(t, map[string]string{
		"SecurityConfig.java": `package app;
import java.util.List;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.security.config.annotation.method.configuration.EnableMethodSecurity;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.web.SecurityFilterChain;
@EnableMethodSecurity
public class SecurityConfig {
    @Value("${reports.open:false}")
    private boolean reportsOpen;
    private List<String> anonymousUrls;
    @Bean
    SecurityFilterChain chain(HttpSecurity http) throws Exception {
        http.authorizeHttpRequests(auth -> {
` + lambdaBody + `
        });
        return http.build();
    }
}`,
		"ReportController.java": `package app;
import org.springframework.security.access.prepost.PreAuthorize;
import org.springframework.web.bind.annotation.*;
@RestController
public class ReportController {
    ` + guard + `
    @GetMapping("/reports/{id}")
    public String get(@PathVariable String id) { return id; }
    @DeleteMapping("/reports/{id}")
    public void delete(@PathVariable String id) { }
}`,
	})
}

func omissionReasons(r cerbos.Result) map[string]cerbos.OmissionReason {
	out := map[string]cerbos.OmissionReason{}
	for _, o := range r.Omissions {
		out[string(o.Endpoint.HTTPMethod)+" "+o.Endpoint.Path] = o.Reason
	}
	return out
}

// TestURLRuleUnresolved_OpaqueRuleMakesPolicyUnknown: ADR 0018 Amendment 1.
// GET /reports/{id} carries hasRole('USER') on its method, and the first
// URL rule matching it is .access(...), which Sphinxor cannot read. The
// effective policy is the intersection of both layers (ADR 0012), and the
// opaque rule may narrow it, so exporting USER alone could grant what the
// application denies. It is omitted under its own reason instead.
func TestURLRuleUnresolved_OpaqueRuleMakesPolicyUnknown(t *testing.T) {
	m := urlRuleProject(t, `
            auth.requestMatchers("/reports/**").access(new WebExpressionAuthorizationManager("hasIpAddress('10.0.0.0/8')"));
            auth.anyRequest().authenticated();`, true)

	r := cerbos.Translate(m)
	if len(r.Rules) != 0 {
		t.Errorf("exported %+v; the opaque URL rule may narrow the method layer, so nothing may be granted", r.Rules)
	}
	reasons := omissionReasons(r)
	for _, ep := range []string{"GET /reports/{id}", "DELETE /reports/{id}"} {
		if reasons[ep] != cerbos.ReasonURLRuleUnresolved {
			t.Errorf("%s omitted as %q, want %q", ep, reasons[ep], cerbos.ReasonURLRuleUnresolved)
		}
	}

	// Matrix: the method roles stay as inventory, and the row is marked.
	mx := report.BuildMatrix(m, nil)
	for _, row := range mx.Rows {
		if !row.URLRuleUnresolved {
			t.Errorf("%s %s: row not marked urlRuleUnresolved", row.Method, row.Path)
		}
		if row.Method == model.MethodGet && (len(row.Roles) != 1 || row.Roles[0] != "USER") {
			t.Errorf("GET row roles = %v, want [USER] kept as inventory", row.Roles)
		}
	}

	// mutating-endpoint-without-access-control still fires (interim), and
	// says why its premise is incomplete.
	var msgs []string
	for _, f := range (lint.MutatingEndpointWithoutAccessControl{}).Check(m) {
		msgs = append(msgs, f.Message)
	}
	if len(msgs) != 1 || !strings.Contains(msgs[0], "URL rule") {
		t.Errorf("mutating findings = %q; want one for DELETE naming the unread URL rule", msgs)
	}
}

// TestURLRuleUnresolved_HarmlessOutcomesStayExported: the RuoYi-Vue shape.
// The opaque rule, a permitAll() over URLs read at runtime, can only end in
// permitAll() or reach authenticated(); neither narrows the method layer,
// so the method roles are exact and must still be exported.
func TestURLRuleUnresolved_HarmlessOutcomesStayExported(t *testing.T) {
	m := urlRuleProject(t, `
            anonymousUrls.forEach(url -> auth.requestMatchers(url).permitAll());
            auth.requestMatchers("/login").permitAll()
                .anyRequest().authenticated();`, true)

	r := cerbos.Translate(m)
	if len(r.Rules) != 1 || r.Rules[0].Action != "get" || strings.Join(r.Rules[0].Roles, ",") != "USER" {
		t.Errorf("rules = %+v; want USER on get, unchanged by an opaque permitAll()", r.Rules)
	}
	for _, row := range report.BuildMatrix(m, nil).Rows {
		if row.URLRuleUnresolved {
			t.Errorf("%s %s marked urlRuleUnresolved; every possible outcome is permitAll() or authenticated()", row.Method, row.Path)
		}
	}
}

// TestURLRuleUnresolved_NarrowingOutcomeMakesPolicyUnknown: shapes where an
// uncertain rule could lead to a role rule, so the effective policy is
// unknown although no rule is .access(...).
func TestURLRuleUnresolved_NarrowingOutcomeMakesPolicyUnknown(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"role rule inside a branch", `
            if (reportsOpen) {
                auth.requestMatchers("/reports/**").hasRole("ADMIN");
            }
            auth.anyRequest().authenticated();`},
		{"unreadable permitAll before a role rule", `
            anonymousUrls.forEach(url -> auth.requestMatchers(url).permitAll());
            auth.requestMatchers("/reports/**").hasRole("ADMIN");
            auth.anyRequest().authenticated();`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := urlRuleProject(t, tc.body, true)
			r := cerbos.Translate(m)
			if len(r.Rules) != 0 {
				t.Errorf("exported %+v; an outcome requires ADMIN, which may narrow USER to nothing", r.Rules)
			}
			if got := omissionReasons(r)["GET /reports/{id}"]; got != cerbos.ReasonURLRuleUnresolved {
				t.Errorf("GET omitted as %q, want %q", got, cerbos.ReasonURLRuleUnresolved)
			}
		})
	}
}

// TestURLRuleUnresolved_NotConfirmedPublicInDiff: ADR 0036 counts protection
// it cannot read as not confirmed public. Removing the method guard while
// an opaque URL rule governs the endpoint is not "became public".
func TestURLRuleUnresolved_NotConfirmedPublicInDiff(t *testing.T) {
	body := `
            auth.requestMatchers("/reports/**").access(new WebExpressionAuthorizationManager("hasIpAddress('10.0.0.0/8')"));
            auth.anyRequest().authenticated();`
	base := urlRuleProject(t, body, true)
	head := urlRuleProject(t, body, false)
	res := diff.Compare(diff.Snapshot{Model: base}, diff.Snapshot{Model: head})
	for _, e := range res.BecamePublic {
		t.Errorf("%s %s reported as became-public; an unread URL rule governs it", e.HTTPMethod, e.Path)
	}
}
