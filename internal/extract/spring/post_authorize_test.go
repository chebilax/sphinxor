package spring

import (
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/lint"
)

// controllerWith builds a one-handler controller carrying ann on a
// handler mapped with verb.
func controllerWith(imp, ann, verb string) map[string]string {
	return map[string]string{
		"Config.java": `package app;
import org.springframework.security.config.annotation.method.configuration.EnableMethodSecurity;

@Configuration
@EnableMethodSecurity
public class Config { }
`,
		"C.java": `package app;
` + imp + `
import org.springframework.web.bind.annotation.*;

@RestController
public class C {
    ` + ann + `
    @` + verb + `Mapping("/a")
    public String a() { return ""; }
}
`,
	}
}

const postAuthorizeImport = `import org.springframework.security.access.prepost.PostAuthorize;`

// TestPostAuthorize_IsNotAGuard is ADR 0030 §2's first half. It must not
// become a GuardApplication: its expressions reference returnObject,
// which the model has no term for, so reading the subset that happens to
// look like @PreAuthorize would present the rest as absent.
func TestPostAuthorize_IsNotAGuard(t *testing.T) {
	m := extractProject(t, controllerWith(postAuthorizeImport,
		`@PostAuthorize("returnObject.owner == authentication.name")`, "Get"))

	if len(m.GuardApplications) != 0 {
		t.Errorf("@PostAuthorize must not be read as a guard, got %+v", m.GuardApplications)
	}
	if len(m.UnrecognizedAuthAnnotations) != 1 {
		t.Fatalf("it must be recorded as authorization-present (ADR 0030 §2), got %+v", m.UnrecognizedAuthAnnotations)
	}
	a := m.UnrecognizedAuthAnnotations[0]
	if a.Name != "PostAuthorize" {
		t.Errorf("Name = %q, want PostAuthorize", a.Name)
	}
	if want := "org.springframework.security.access.prepost.PostAuthorize"; a.BoundTo != want {
		t.Errorf("BoundTo = %q, want %q — it IS Spring's, and the record should say so", a.BoundTo, want)
	}
}

// TestPostAuthorize_DoesNotSuppressOnMutating is the core of ADR 0030
// §1, and the correction that redirected this decision.
//
// Spring evaluates @PostAuthorize after the handler runs
// (AuthorizationManagerAfterMethodInterceptor calls mi.proceed() first),
// so on a POST the state change has already happened when access is
// denied. Suppressing the finding would claim protection that does not
// exist. Spring's own documentation says @PostAuthorize "is not
// recommended for classes that perform database writes".
func TestPostAuthorize_DoesNotSuppressOnMutating(t *testing.T) {
	for _, verb := range []string{"Post", "Put", "Patch", "Delete"} {
		t.Run(verb, func(t *testing.T) {
			m := extractProject(t, controllerWith(postAuthorizeImport,
				`@PostAuthorize("hasRole('ADMIN')")`, verb))

			findings := (lint.MutatingEndpointWithoutAccessControl{}).Check(m)
			if len(findings) != 1 {
				t.Fatalf("a mutating endpoint whose only annotation is @PostAuthorize is not protected "+
					"against the mutation; want 1 finding, got %d", len(findings))
			}
			if !strings.Contains(findings[0].Message, "evaluated after the handler executes") {
				t.Errorf("the message must say why it still fires (ADR 0030 §3), got %q", findings[0].Message)
			}
		})
	}
}

// TestPostAuthorize_ReadIsStillMarked covers the other half of §2. The
// rule never examines reads, so nothing is suppressed there either way —
// what a GET gets is the record, the ? marker and the export omission.
func TestPostAuthorize_ReadIsStillMarked(t *testing.T) {
	m := extractProject(t, controllerWith(postAuthorizeImport,
		`@PostAuthorize("hasRole('ADMIN')")`, "Get"))

	if len(m.UnrecognizedAuthAnnotations) != 1 {
		t.Fatalf("a read's @PostAuthorize is still recorded, got %+v", m.UnrecognizedAuthAnnotations)
	}
	if n := len((lint.MutatingEndpointWithoutAccessControl{}).Check(m)); n != 0 {
		t.Errorf("the rule does not examine reads at all, got %d findings", n)
	}
}

// TestPostAuthorize_InertGetsTheOrdinaryMessage pins ADR 0030 §3's
// sub-case. Under @EnableGlobalMethodSecurity, prePostEnabled defaults
// false, so nothing evaluates the annotation — and saying when it
// evaluates would be the false statement ADR 0015 Amendment 1 removed.
func TestPostAuthorize_InertGetsTheOrdinaryMessage(t *testing.T) {
	m := extractProject(t, map[string]string{
		"Config.java": `package app;
import org.springframework.security.config.annotation.method.configuration.EnableGlobalMethodSecurity;

@Configuration
@EnableGlobalMethodSecurity
public class Config { }
`,
		"C.java": `package app;
` + postAuthorizeImport + `
import org.springframework.web.bind.annotation.*;

@RestController
public class C {
    @PostAuthorize("hasRole('ADMIN')")
    @PostMapping("/a")
    public void a() { }
}
`,
	})
	if m.MethodSecurity.PrePostEnabled {
		t.Fatal("@EnableGlobalMethodSecurity defaults prePostEnabled false — precondition for this test")
	}
	findings := (lint.MutatingEndpointWithoutAccessControl{}).Check(m)
	if len(findings) != 1 {
		t.Fatalf("want 1 finding, got %d", len(findings))
	}
	if strings.Contains(findings[0].Message, "evaluated after the handler executes") {
		t.Errorf("an inert annotation never evaluates, so the message must not describe when it does; got %q",
			findings[0].Message)
	}
}

// TestPostAuthorize_ForeignPackageIsUnchanged guards the ADR 0022
// boundary: adding @PostAuthorize to the recognized names must not make
// a same-named annotation from elsewhere into Spring's.
func TestPostAuthorize_ForeignPackageIsUnchanged(t *testing.T) {
	m := extractProject(t, controllerWith(`import com.example.PostAuthorize;`,
		`@PostAuthorize("whatever")`, "Post"))

	if len(m.UnrecognizedAuthAnnotations) != 1 {
		t.Fatalf("a foreign @PostAuthorize is still unrecognized, got %+v", m.UnrecognizedAuthAnnotations)
	}
	if got := m.UnrecognizedAuthAnnotations[0].BoundTo; got != "com.example.PostAuthorize" {
		t.Errorf("BoundTo = %q, want the foreign FQN", got)
	}
	// ADR 0022 §3's suppression is unchanged for it — only Spring's own
	// @PostAuthorize is carved out, because only that one is known to
	// run late.
	if n := len((lint.MutatingEndpointWithoutAccessControl{}).Check(m)); n != 0 {
		t.Errorf("a foreign authorization annotation still suppresses the finding (ADR 0022 §3), got %d", n)
	}
}

// TestMethodSecurityFilters_CountedNotAttached is ADR 0030 §4. Neither
// annotation denies a call, so neither may mark an endpoint or suppress
// a finding.
func TestMethodSecurityFilters_CountedNotAttached(t *testing.T) {
	m := extractProject(t, map[string]string{
		"Config.java": `package app;
import org.springframework.security.config.annotation.method.configuration.EnableMethodSecurity;

@Configuration
@EnableMethodSecurity
public class Config { }
`,
		"C.java": `package app;
import org.springframework.security.access.prepost.PreFilter;
import org.springframework.security.access.prepost.PostFilter;
import org.springframework.web.bind.annotation.*;

@RestController
public class OrderController {
    @PreFilter("filterObject.owner == authentication.name")
    @PostMapping("/bulk")
    public void bulk(java.util.List<String> items) { }

    @PostFilter("hasRole('ADMIN')")
    @GetMapping("/all")
    public java.util.List<String> all() { return null; }
}
`,
	})
	if got := m.MethodSecurityFilters; got.PreFilter != 1 || got.PostFilter != 1 {
		t.Errorf("counts = %+v, want PreFilter 1 / PostFilter 1", got)
	}
	if got := m.MethodSecurityFilters.Classes; len(got) != 1 || got[0] != "OrderController" {
		t.Errorf("Classes = %v, want [OrderController] for the warning text", got)
	}
	if len(m.UnrecognizedAuthAnnotations) != 0 {
		t.Errorf("filters must NOT be attached to an endpoint, got %+v", m.UnrecognizedAuthAnnotations)
	}
	if len(m.GuardApplications) != 0 {
		t.Errorf("filters are not guards, got %+v", m.GuardApplications)
	}
	// The POST carries only a @PreFilter, which guards nothing.
	findings := (lint.MutatingEndpointWithoutAccessControl{}).Check(m)
	if len(findings) != 1 {
		t.Fatalf("a @PreFilter must not suppress the finding on a mutating endpoint, got %d", len(findings))
	}
	if strings.Contains(findings[0].Message, "PostAuthorize") {
		t.Errorf("wrong message for a filter-only endpoint: %q", findings[0].Message)
	}
}

// TestMethodSecurityFilters_IdentityIsByImport is why the filter scan
// resolves the import rather than matching the name. shenyu really does
// declare definitionPostFilter and apiPostFilter methods, and the corpus
// search for this ADR turned up exactly that kind of false positive.
func TestMethodSecurityFilters_IdentityIsByImport(t *testing.T) {
	m := extractProject(t, map[string]string{
		"C.java": `package app;
import com.example.PostFilter;
import org.springframework.web.bind.annotation.*;

@RestController
public class C {
    @PostFilter("x")
    @GetMapping("/a")
    public String a() { return ""; }
}
`,
	})
	if got := m.MethodSecurityFilters.Total(); got != 0 {
		t.Errorf("a @PostFilter from another package is not Spring's, got count %d", got)
	}
}

// TestPostAuthorize_QualifiedIsRecognized covers the ADR 0025 path,
// which resolves before imports are consulted and would otherwise read
// Spring's own annotation as foreign.
func TestPostAuthorize_QualifiedIsRecognized(t *testing.T) {
	m := extractProject(t, map[string]string{
		"C.java": `package app;
import org.springframework.web.bind.annotation.*;

@RestController
public class C {
    @org.springframework.security.access.prepost.PostAuthorize("hasRole('ADMIN')")
    @PostMapping("/a")
    public void a() { }
}
`,
	})
	if len(m.UnrecognizedAuthAnnotations) != 1 {
		t.Fatalf("a qualified @PostAuthorize must be recorded, got %+v", m.UnrecognizedAuthAnnotations)
	}
	if len(m.GuardApplications) != 0 {
		t.Errorf("and still not as a guard, got %+v", m.GuardApplications)
	}
}
