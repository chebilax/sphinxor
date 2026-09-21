package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
)

// TestAnyVerb_IsMutating is ADR 0028 §2 consumer 1. A handler answering
// every verb accepts POST, PUT, PATCH and DELETE, so a rule about state
// change must say so — this is what produces the decision's 126 findings.
func TestAnyVerb_IsMutating(t *testing.T) {
	m, _ := extractOne(t, `
@RestController
@RequestMapping("/api")
public class C {
    @RequestMapping("/anything")
    public void anything() { }
}
`)
	got := (lint.MutatingEndpointWithoutAccessControl{}).Check(m)
	if len(got) != 1 {
		t.Fatalf("an unguarded any-verb handler must be flagged once, got %d: %+v", len(got), got)
	}
	// Once, not four times. Four findings naming one handler is the
	// outcome ADR 0028 rejected Option A to avoid.
	if len(m.Endpoints) != 1 {
		t.Errorf("one handler is one endpoint, got %d", len(m.Endpoints))
	}
}

// TestAnyVerb_ClassLevelVerblessIsOnlyABasePath guards the 735
// class-level verb-less @RequestMapping annotations in the corpus, which
// are path prefixes and must not become endpoints. Getting this wrong
// would turn every annotated controller into a phantom route.
func TestAnyVerb_ClassLevelVerblessIsOnlyABasePath(t *testing.T) {
	m, _ := extractOne(t, `
@RestController
@RequestMapping("/api")
public class C {
    @GetMapping("/a")
    public String a() { return ""; }
}
`)
	if len(m.Endpoints) != 1 {
		t.Fatalf("got %d endpoints, want 1 — the class-level mapping is a prefix, not a route", len(m.Endpoints))
	}
	if m.Endpoints[0].HTTPMethod != model.MethodGet || m.Endpoints[0].Path != "/api/a" {
		t.Errorf("got %s %s, want GET /api/a", m.Endpoints[0].HTTPMethod, m.Endpoints[0].Path)
	}
}

// TestAnyVerb_OverlapWithVerbSpecific is ADR 0028 §4, modelled on the
// corpus's only instance: spring-cloud-dataflow's TaskSchedulerController,
// where DELETE reaches one handler and every other verb reaches the
// verb-less one.
//
// Two endpoints, each carrying its own handler's guards. Neither is
// merged into the other, which is ADR 0020 Amendment 2 §8's property
// achieved without its machinery, since ANY and DELETE already have
// distinct identities.
func TestAnyVerb_OverlapWithVerbSpecific(t *testing.T) {
	m, handler := extractOne(t, `
import org.springframework.security.access.prepost.PreAuthorize;

@RestController
@RequestMapping("/tasks/schedules")
public class TaskSchedulerController {
    @RequestMapping("/instances/{name}")
    public String filteredList() { return ""; }

    @PreAuthorize("hasRole('ADMIN')")
    @DeleteMapping("/instances/{name}")
    public void deleteSchedules() { }
}
`)
	if len(m.Endpoints) != 2 {
		t.Fatalf("a verb-less and a @DeleteMapping on one path are two endpoints, got %d: %+v", len(m.Endpoints), m.Endpoints)
	}
	byVerb := map[model.HTTPMethod]model.Endpoint{}
	for _, e := range m.Endpoints {
		byVerb[e.HTTPMethod] = e
	}
	if _, ok := byVerb[model.MethodAny]; !ok {
		t.Error("the verb-less handler should be ANY")
	}
	if _, ok := byVerb[model.MethodDelete]; !ok {
		t.Error("the @DeleteMapping handler should be DELETE")
	}
	// The DELETE handler's guard must not leak onto the ANY row, and
	// vice versa: that is the whole safety property.
	for _, g := range m.GuardApplications {
		if handler[g.EndpointID] != "deleteSchedules" {
			t.Errorf("guard attached to %q, want only deleteSchedules", handler[g.EndpointID])
		}
	}
	// So the ANY row, which has no guard of its own, is still flagged.
	var flagged []string
	for _, f := range (lint.MutatingEndpointWithoutAccessControl{}).Check(m) {
		flagged = append(flagged, handler[f.SubjectID])
	}
	if len(flagged) != 1 || flagged[0] != "filteredList" {
		t.Errorf("flagged %v, want exactly [filteredList]", flagged)
	}
}

// TestAnyVerb_VerbScopedURLRuleIsUnresolved is ADR 0028 §2 consumer 3,
// and the subtlest of the six.
//
// A rule scoped to POST covers one of the eight verbs an ANY endpoint
// answers. Granting its roles would over-report across the other seven;
// skipping it would let a later, more permissive rule apply. It matches
// and is neutralised, which is ADR 0018's existing outcome.
func TestAnyVerb_VerbScopedURLRuleIsUnresolved(t *testing.T) {
	m := extractProject(t, map[string]string{
		"C.java": `package app;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api")
public class C {
    @RequestMapping("/thing")
    public String thing() { return ""; }
}
`,
		"SecurityConfig.java": `package app;
import org.springframework.context.annotation.Bean;
import org.springframework.security.web.SecurityFilterChain;

@Configuration
public class SecurityConfig {
    @Bean
    public SecurityFilterChain filterChain(HttpSecurity http) throws Exception {
        http.authorizeHttpRequests(requests -> requests
            .requestMatchers(HttpMethod.POST, "/api/thing").hasRole("ADMIN")
            .anyRequest().permitAll());
        return http.build();
    }
}
`,
	})
	// The POST-scoped rule must not hand ADMIN to an endpoint that also
	// answers seven other verbs.
	for _, r := range m.RoleReferences {
		t.Errorf("a verb-scoped rule must not grant roles to an ANY endpoint, got %+v", r)
	}
	// Nor may evaluation fall through to the later permitAll().
	if len(m.AuthenticationRequirements) != 0 {
		t.Errorf("evaluation must stop at the neutralised rule, got %+v", m.AuthenticationRequirements)
	}
}
