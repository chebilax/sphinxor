package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
)

// extractOne parses one Java snippet and returns the built model, keyed
// helpers included — the shape every test below needs.
func extractOne(t *testing.T, src string) (*model.Model, map[model.ID]string) {
	t.Helper()
	root, source := parseJava(t, src)
	b := newBuilder()
	extractControllers(root, source, "Thing.java", b, nil, nil)
	handler := make(map[model.ID]string, len(b.model.Endpoints))
	for _, e := range b.model.Endpoints {
		handler[e.ID] = e.HandlerName
	}
	return &b.model, handler
}

// TestAnnotationIdentity_SameNameDifferentPackage is the central
// regression for docs/decisions/0022-annotation-identity-and-unrecognized-authorization.md §1.
//
// Both handlers below are annotated `@Secured`. They are different
// annotations, and the only thing that says so is the import. Before ADR
// 0022 extraction matched on the simple name and recorded both as Spring
// method-security guards; on alibaba/nacos that produced 392 asserted
// guards from an annotation Spring has never heard of.
//
// The two live in one file deliberately: the failure being guarded
// against is that the binding stops being consulted, and a test with only
// the foreign case would pass just as well if extraction rejected every
// @Secured, including Spring's.
func TestAnnotationIdentity_SameNameDifferentPackage(t *testing.T) {
	m, handler := extractOne(t, `
import org.springframework.security.access.annotation.Secured;

@RestController
@RequestMapping("/api")
public class SpringController {
    @Secured({"ROLE_ADMIN"})
    @PostMapping("/spring")
    public void springSecured() {}
}
`)
	if len(m.GuardApplications) != 1 {
		t.Fatalf("Spring-imported @Secured: got %d GuardApplications, want 1", len(m.GuardApplications))
	}
	if len(m.UnrecognizedAuthAnnotations) != 0 {
		t.Errorf("Spring-imported @Secured must not be recorded as unrecognized, got %+v", m.UnrecognizedAuthAnnotations)
	}
	if len(m.RoleReferences) != 1 || m.RoleReferences[0].RawLiteral != "ROLE_ADMIN" {
		t.Errorf("Spring-imported @Secured should yield ROLE_ADMIN, got %+v", m.RoleReferences)
	}
	_ = handler

	// The same simple name, bound to nacos's own annotation.
	m2, handler2 := extractOne(t, `
import com.alibaba.nacos.auth.annotation.Secured;

@RestController
@RequestMapping("/api")
public class NacosController {
    @Secured(resource = "nacos/thing", action = ActionTypes.WRITE)
    @PostMapping("/nacos")
    public void nacosSecured() {}
}
`)
	if len(m2.GuardApplications) != 0 {
		t.Errorf("foreign @Secured must not become a GuardApplication, got %+v", m2.GuardApplications)
	}
	if len(m2.UnrecognizedAuthAnnotations) != 1 {
		t.Fatalf("foreign @Secured should be recorded as unrecognized, got %+v", m2.UnrecognizedAuthAnnotations)
	}
	got := m2.UnrecognizedAuthAnnotations[0]
	if got.Name != "Secured" {
		t.Errorf("Name = %q, want Secured", got.Name)
	}
	// The warning names BoundTo, not Name — "@Secured" alone reads as
	// Spring's, which is the confusion ADR 0022 §3a removes.
	if got.BoundTo != "com.alibaba.nacos.auth.annotation.Secured" {
		t.Errorf("BoundTo = %q, want the nacos FQN", got.BoundTo)
	}
	if handler2[got.EndpointID] != "nacosSecured" {
		t.Errorf("unrecognized annotation attached to %q, want nacosSecured", handler2[got.EndpointID])
	}
}

// TestAnnotationIdentity_SpringSecuredIsRecognized is a positive test the
// survey corpus cannot supply: no project in the 20 surveyed repositories
// imports Spring's own @Secured, so the only @Secured in measured code is
// nacos's.
//
// That makes `org.springframework.security.access.annotation.Secured` the
// one accepted package string with no real-world coverage — and a typo in
// it would silently reject every genuine Spring @Secured, which is
// exactly this ADR's defect inverted. The string is verified against
// Spring Security's API docs; this test is what keeps it verified.
func TestAnnotationIdentity_SpringSecuredIsRecognized(t *testing.T) {
	m, _ := extractOne(t, `
import org.springframework.security.access.annotation.Secured;

@RestController
public class C {
    @Secured({"ROLE_USER", "ROLE_ADMIN"})
    @GetMapping("/a")
    public void a() {}
}
`)
	if len(m.GuardApplications) != 1 {
		t.Fatalf("got %d GuardApplications, want 1 — is the accepted package string right?", len(m.GuardApplications))
	}
	var roles []string
	for _, r := range m.RoleReferences {
		roles = append(roles, r.RawLiteral)
	}
	if len(roles) != 2 || roles[0] != "ROLE_USER" || roles[1] != "ROLE_ADMIN" {
		t.Errorf("roles = %v, want [ROLE_USER ROLE_ADMIN]", roles)
	}
}

// TestAnnotationIdentity_RolesAllowedBothNamespaces is the same kind of
// positive test for the other uncovered name. @RolesAllowed does not
// appear anywhere in 20 surveyed repositories, in either namespace.
//
// Both javax and jakarta are accepted because both are the real JSR-250
// annotation and Spring reads both — a binding fact, not a preference
// (ADR 0022 §1).
func TestAnnotationIdentity_RolesAllowedBothNamespaces(t *testing.T) {
	for _, ns := range []string{"javax.annotation.security", "jakarta.annotation.security"} {
		t.Run(ns, func(t *testing.T) {
			m, _ := extractOne(t, `
import `+ns+`.RolesAllowed;

@RestController
public class C {
    @RolesAllowed({"ADMIN"})
    @GetMapping("/a")
    public void a() {}
}
`)
			if len(m.GuardApplications) != 1 {
				t.Fatalf("%s: got %d GuardApplications, want 1 — is the accepted package string right?", ns, len(m.GuardApplications))
			}
			if len(m.RoleReferences) != 1 || m.RoleReferences[0].RawLiteral != "ADMIN" {
				t.Errorf("%s: roles = %+v, want [ADMIN]", ns, m.RoleReferences)
			}
		})
	}
}

// TestAnnotationIdentity_NoImportIsNotSpring covers the branch the corpus
// cannot exercise at all: measured across 20 repositories and every
// vendored fixture, there is no file that uses a recognized name without
// a binding import.
//
// ADR 0022 §1 treats it as unrecognized on principle rather than on
// evidence of need — Spring's annotations live under org.springframework.*,
// so an unbound @Secured cannot be Spring's by same-package resolution;
// it is a project-local annotation, which is the foreign case.
func TestAnnotationIdentity_NoImportIsNotSpring(t *testing.T) {
	m, _ := extractOne(t, `
@RestController
public class C {
    @Secured({"ROLE_ADMIN"})
    @PostMapping("/a")
    public void a() {}
}
`)
	if len(m.GuardApplications) != 0 {
		t.Errorf("an unbound @Secured must not be treated as Spring's, got %+v", m.GuardApplications)
	}
	if len(m.UnrecognizedAuthAnnotations) != 1 {
		t.Fatalf("an unbound @Secured should be recorded as unrecognized, got %+v", m.UnrecognizedAuthAnnotations)
	}
	if b := m.UnrecognizedAuthAnnotations[0].BoundTo; b != "" {
		t.Errorf("BoundTo = %q, want empty — nothing in the file binds the name", b)
	}
}

// TestAnnotationIdentity_WildcardImportBinds covers the one import form
// that binds a name without naming it.
func TestAnnotationIdentity_WildcardImportBinds(t *testing.T) {
	m, _ := extractOne(t, `
import org.springframework.security.access.annotation.*;

@RestController
public class C {
    @Secured({"ROLE_ADMIN"})
    @PostMapping("/a")
    public void a() {}
}
`)
	if len(m.GuardApplications) != 1 {
		t.Fatalf("a wildcard import of an accepted package should bind the name, got %d guards", len(m.GuardApplications))
	}
	if len(m.UnrecognizedAuthAnnotations) != 0 {
		t.Errorf("wildcard-bound Spring @Secured must not be unrecognized, got %+v", m.UnrecognizedAuthAnnotations)
	}
}

// TestAnnotationIdentity_StaticImportDoesNotBind guards a detail of the
// import parser: a static import binds a member, never an annotation
// type, so it must not be mistaken for one.
func TestAnnotationIdentity_StaticImportDoesNotBind(t *testing.T) {
	it := parseImportsFromSnippet(t, `
import static com.example.Constants.Secured;
`)
	if fqn, ok := it.exact["Secured"]; ok {
		t.Errorf("a static import must not bind an annotation name, got %q", fqn)
	}
}

func parseImportsFromSnippet(t *testing.T, src string) importTable {
	t.Helper()
	root, source := parseJava(t, src)
	return parseImports(root, source)
}

// TestUnrecognizedAuth_SuppressesMutatingFinding is ADR 0022 §3 end to
// end: the rule's message says the endpoint "has no detected guard or
// role decorator", and an authorization annotation is sitting one line
// above the handler. On nacos that premise would be false 242 times.
//
// The second handler is the control — genuinely bare, and it must still
// be flagged, or this would be suppression rather than a distinction.
func TestUnrecognizedAuth_SuppressesMutatingFinding(t *testing.T) {
	m, handler := extractOne(t, `
import com.alibaba.nacos.auth.annotation.Secured;

@RestController
@RequestMapping("/api")
public class C {
    @Secured(resource = "x", action = ActionTypes.WRITE)
    @PostMapping("/protected-somehow")
    public void protectedSomehow() {}

    @PostMapping("/genuinely-bare")
    public void genuinelyBare() {}
}
`)
	findings := lint.MutatingEndpointWithoutAccessControl{}.Check(m)
	var flagged []string
	for _, f := range findings {
		flagged = append(flagged, handler[f.SubjectID])
	}
	if len(flagged) != 1 || flagged[0] != "genuinelyBare" {
		t.Errorf("mutating-endpoint-without-access-control fired on %v, want exactly [genuinelyBare]", flagged)
	}
}
