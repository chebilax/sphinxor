package spring

import (
	"testing"
)

// TestQualifiedName_ControllerAndMappings is the core regression for
// docs/decisions/0025-qualified-annotation-names.md §1/§2, modelled on
// microcks, which writes @org.springframework.web.bind.annotation.RestController
// in 13 files because its own class in io.github.microcks.web is named
// RestController and Spring's cannot be imported there.
//
// The mapping annotations are qualified too, although the corpus has zero
// such uses: §2 covers them because fixing the controller and leaving its
// routes invisible would produce a recognized controller with no
// endpoints, which no warning covers and which looks resolved.
func TestQualifiedName_ControllerAndMappings(t *testing.T) {
	m, _ := extractOne(t, `
@org.springframework.web.bind.annotation.RestController
@org.springframework.web.bind.annotation.RequestMapping("/api")
public class RestController {
    @org.springframework.web.bind.annotation.GetMapping("/a")
    public String a() { return ""; }

    @org.springframework.web.bind.annotation.PostMapping("/b")
    public void b() { }
}
`)
	if len(m.Controllers) != 1 {
		t.Fatalf("a fully-qualified @RestController must make this a controller, got %d", len(m.Controllers))
	}
	if m.Controllers[0].BasePath != "/api" {
		t.Errorf("BasePath = %q, want /api from the qualified @RequestMapping", m.Controllers[0].BasePath)
	}
	got := map[string]bool{}
	for _, e := range m.Endpoints {
		got[string(e.HTTPMethod)+" "+e.Path] = true
	}
	for _, want := range []string{"GET /api/a", "POST /api/b"} {
		if !got[want] {
			t.Errorf("missing %s; got %v", want, got)
		}
	}
}

// TestQualifiedName_ForeignPackageIsNotSpring pins ADR 0025 §4. Accepting
// a dotted name by its last segment would read this as Spring's — the
// nacos collision (ADR 0022) reintroduced by the change meant to fix its
// mirror image.
func TestQualifiedName_ForeignPackageIsNotSpring(t *testing.T) {
	m, _ := extractOne(t, `
@com.example.RestController
public class C {
    @com.example.GetMapping("/a")
    public String a() { return ""; }
}
`)
	if len(m.Controllers) != 0 || len(m.Endpoints) != 0 {
		t.Errorf("@com.example.RestController is not Spring's; got %d controllers / %d endpoints",
			len(m.Controllers), len(m.Endpoints))
	}
}

// TestQualifiedName_SpringSecurityIsRecognized is ADR 0025 §3, and the
// reason that section exists.
//
// Without it, §1's normalization sends a fully-qualified @PreAuthorize
// through ADR 0022's import check, which finds no import binding the
// simple name — there is none, the path is written out in full — and
// correctly concludes "not Spring's". The result would be an unrecognized
// authorization annotation recorded for Spring's own annotation: ADR
// 0022's defect recreated on the annotation it protects.
//
// The corpus has ZERO fully-qualified method-security annotations, so
// nothing else in the suite would catch a typo in these package strings.
func TestQualifiedName_SpringSecurityIsRecognized(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ann       string
		wantRoles []string
	}{
		{
			"PreAuthorize",
			`@org.springframework.security.access.prepost.PreAuthorize("hasRole('ADMIN')")`,
			[]string{"ADMIN"},
		},
		{
			"Secured",
			`@org.springframework.security.access.annotation.Secured({"ROLE_ADMIN"})`,
			[]string{"ROLE_ADMIN"},
		},
		{
			"RolesAllowed javax",
			`@javax.annotation.security.RolesAllowed({"ADMIN"})`,
			[]string{"ADMIN"},
		},
		{
			"RolesAllowed jakarta",
			`@jakarta.annotation.security.RolesAllowed({"ADMIN"})`,
			[]string{"ADMIN"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := extractOne(t, `
@org.springframework.web.bind.annotation.RestController
public class C {
    `+tc.ann+`
    @org.springframework.web.bind.annotation.PostMapping("/a")
    public void a() { }
}
`)
			if len(m.UnrecognizedAuthAnnotations) != 0 {
				t.Fatalf("a fully-qualified Spring annotation must NOT be recorded as unrecognized (§3), got %+v",
					m.UnrecognizedAuthAnnotations)
			}
			if len(m.GuardApplications) != 1 {
				t.Fatalf("got %d GuardApplications, want 1 — is the accepted package string right?", len(m.GuardApplications))
			}
			var roles []string
			for _, r := range m.RoleReferences {
				roles = append(roles, r.RawLiteral)
			}
			if len(roles) != len(tc.wantRoles) || (len(roles) > 0 && roles[0] != tc.wantRoles[0]) {
				t.Errorf("roles = %v, want %v", roles, tc.wantRoles)
			}
		})
	}
}

// TestQualifiedName_ForeignSecurityStillUnrecognized is the other half of
// §3: nacos's @Secured spelled out in full is still not Spring's, and
// must land where ADR 0022 put it rather than being swept in by the
// normalization.
func TestQualifiedName_ForeignSecurityStillUnrecognized(t *testing.T) {
	m, _ := extractOne(t, `
@org.springframework.web.bind.annotation.RestController
public class C {
    @com.alibaba.nacos.auth.annotation.Secured(resource = "x", action = ActionTypes.WRITE)
    @org.springframework.web.bind.annotation.PostMapping("/a")
    public void a() { }
}
`)
	if len(m.GuardApplications) != 0 {
		t.Errorf("a fully-qualified foreign @Secured must not become a guard, got %+v", m.GuardApplications)
	}
	if len(m.UnrecognizedAuthAnnotations) != 1 {
		t.Fatalf("it must land as unrecognized (ADR 0022 §2), got %+v", m.UnrecognizedAuthAnnotations)
	}
	if got := m.UnrecognizedAuthAnnotations[0].BoundTo; got != "com.alibaba.nacos.auth.annotation.Secured" {
		t.Errorf("BoundTo = %q, want the nacos FQN — the warning names the package", got)
	}
}

// TestQualifiedName_ShiroQualifiedIsUnrecognized covers the third arm of
// resolveQualifiedAuth: a known third-party authorization package written
// out in full keeps ADR 0023's treatment.
func TestQualifiedName_ShiroQualifiedIsUnrecognized(t *testing.T) {
	m, _ := extractOne(t, `
@org.springframework.web.bind.annotation.RestController
public class C {
    @org.apache.shiro.authz.annotation.RequiresPermissions("system:user:edit")
    @org.springframework.web.bind.annotation.PostMapping("/a")
    public void a() { }
}
`)
	if len(m.UnrecognizedAuthAnnotations) != 1 {
		t.Fatalf("a qualified Shiro annotation should be recorded as unrecognized, got %+v", m.UnrecognizedAuthAnnotations)
	}
	if got := m.UnrecognizedAuthAnnotations[0].BoundTo; got != "org.apache.shiro.authz.annotation.RequiresPermissions" {
		t.Errorf("BoundTo = %q, want the Shiro FQN", got)
	}
}

// TestSplitAnnotationName covers the normalization itself, including the
// @lombok.Data shape that motivated §4's whole-path rule: a one-segment
// package is a complete qualification, not a truncation, and nothing but
// the whole path can tell the two apart.
func TestSplitAnnotationName(t *testing.T) {
	for _, tc := range []struct{ in, name, qualifier string }{
		{"RestController", "RestController", ""},
		{"org.springframework.web.bind.annotation.RestController", "RestController", "org.springframework.web.bind.annotation"},
		{"lombok.Data", "Data", "lombok"},
		{"com.example.RestController", "RestController", "com.example"},
	} {
		n, q := splitAnnotationName(tc.in)
		if n != tc.name || q != tc.qualifier {
			t.Errorf("splitAnnotationName(%q) = (%q, %q), want (%q, %q)", tc.in, n, q, tc.name, tc.qualifier)
		}
	}
}

// TestQualifiedName_MetaAnnotationStillResolves guards the interaction
// with ADR 0024: a controller meta-annotation declaration whose composed
// @RestController is written qualified must still register.
func TestQualifiedName_MetaAnnotationStillResolves(t *testing.T) {
	metas := map[string]controllerMeta{}
	root, src := parseJava(t, `
@Target(ElementType.TYPE)
@org.springframework.web.bind.annotation.RestController
@org.springframework.web.bind.annotation.RequestMapping("/base")
public @interface QualifiedApi {
}
`)
	scanControllerMetaAnnotations(root, src, metas)
	m, ok := metas["QualifiedApi"]
	if !ok {
		t.Fatalf("a meta-annotation composing a qualified @RestController must register, got %v", metas)
	}
	if !m.hasDeclaredPath || m.declaredPath != "/base" {
		t.Errorf("declared path = %q (has=%v), want /base", m.declaredPath, m.hasDeclaredPath)
	}
}

// TestQualifiedName_MetaAnnotationQualifierMustMatchItsPackage closes the
// hole the ADR 0025 call-site audit found: `metas[name]` matched on the
// simple name alone, so a qualified use from an unrelated package would
// have resolved to a project's meta-annotation. That is §4's hazard
// applied to project-declared annotations rather than Spring's.
//
// No corpus project writes a project meta-annotation qualified, so this
// branch has no real-code coverage.
func TestQualifiedName_MetaAnnotationQualifierMustMatchItsPackage(t *testing.T) {
	decl := `package app.ann;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@Target(ElementType.TYPE)
@RestController
@RequestMapping("/base")
public @interface RestApi {
}
`
	for _, tc := range []struct {
		name string
		use  string
		want int
	}{
		{"unqualified use resolves", "@RestApi", 1},
		{"use qualified with its own package resolves", "@app.ann.RestApi", 1},
		{"use qualified with another package does not", "@other.pkg.RestApi", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := extractProject(t, map[string]string{
				"ann/RestApi.java": decl,
				"C.java": `package app;
import app.ann.RestApi;
import org.springframework.web.bind.annotation.*;

` + tc.use + `
public class C {
    @GetMapping("/a")
    public String a() { return ""; }
}
`,
			})
			if len(m.Endpoints) != tc.want {
				t.Errorf("got %d endpoints, want %d", len(m.Endpoints), tc.want)
			}
		})
	}
}
