package spring

import (
	"sort"
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// Tests for docs/decisions/0039-route-path-constants.md. The real-code
// cases are the vendored conductor and nacos fixtures, further down. The
// rest are synthetic, per docs/testing.md: each pins one rule of the ADR
// whose corpus occurrences are either zero or tangled with other shapes.

// routes returns "METHOD path" for every resolved endpoint, sorted, and
// the unresolved endpoints' reasons.
func routes(m *model.Model) (resolved []string, unresolved []string) {
	for _, e := range m.Endpoints {
		if e.PathUnresolved {
			unresolved = append(unresolved, e.PathUnresolvedReason)
			continue
		}
		resolved = append(resolved, string(e.HTTPMethod)+" "+e.Path)
	}
	sort.Strings(resolved)
	return resolved, unresolved
}

func controller(imports, classAnn, methods string) string {
	return "package app;\n" + imports + "\nimport org.springframework.web.bind.annotation.*;\n\n@RestController\n" +
		classAnn + "\npublic class C {\n" + methods + "\n}\n"
}

// TestRoutePath_NoPathGivenIsResolved is §1: a mapping that names no path
// has the base path. Before ADR 0039 each of these was reported as an
// unreadable path — 66 routes across the corpus.
func TestRoutePath_NoPathGivenIsResolved(t *testing.T) {
	m := extractProject(t, map[string]string{"C.java": controller("",
		`@RequestMapping(value = "/items", produces = "application/json")`, `
    @GetMapping()
    public void list() { }
    @RequestMapping(method = RequestMethod.OPTIONS)
    public void options() { }
    @PostMapping(consumes = "application/json")
    public void create() { }
`)})
	got, unresolved := routes(m)
	want := []string{"GET /items", "OPTIONS /items", "POST /items"}
	if strings.Join(got, ",") != strings.Join(want, ",") || len(unresolved) != 0 {
		t.Errorf("routes = %v, unresolved = %v; want %v and none unresolved", got, unresolved, want)
	}
}

// TestRoutePath_ClassLevelNoPath is §1 at the class level:
// @RequestMapping(produces = ...) names no prefix.
func TestRoutePath_ClassLevelNoPath(t *testing.T) {
	m := extractProject(t, map[string]string{"C.java": controller("",
		`@RequestMapping(produces = "application/json")`, `
    @GetMapping("/ping")
    public void ping() { }
`)})
	if got, u := routes(m); len(got) != 1 || got[0] != "GET /ping" || len(u) != 0 {
		t.Errorf("routes = %v, unresolved = %v; want [GET /ping]", got, u)
	}
}

// TestRoutePath_ResolvedForms is §2: each resolution step, resolved.
func TestRoutePath_ResolvedForms(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"static import of an interface constant built by concatenation", map[string]string{
			"config/Paths.java": "package app.config;\npublic interface Paths {\n    String API = \"/api/\";\n    String ADMIN = API + \"admin\";\n}\n",
			"C.java":            controller("import static app.config.Paths.ADMIN;", "@RequestMapping(ADMIN)", "    @GetMapping(\"/x\") public void x() { }"),
		}, "GET /api/admin/x"},
		{"a nested class, reached through a single-type import", map[string]string{
			"k/Constants.java": "package app.k;\npublic class Constants {\n    public static class A2A {\n        public static final String ADMIN_PATH = \"/v3/admin/a2a\";\n    }\n}\n",
			"C.java":           controller("import app.k.Constants;", "@RequestMapping(Constants.A2A.ADMIN_PATH)", "    @GetMapping public void x() { }"),
		}, "GET /v3/admin/a2a"},
		{"a constant inherited from a supertype in the tree", map[string]string{
			"Base.java": "package app;\npublic interface Base { String ROOT = \"/root\"; }\n",
			"C.java":    "package app;\nimport org.springframework.web.bind.annotation.*;\n@RestController\n@RequestMapping(ROOT)\npublic class C implements Base {\n    @GetMapping(\"/x\") public void x() { }\n}\n",
		}, "GET /root/x"},
		{"a char literal in a String concatenation", map[string]string{
			"C.java": controller("", "@RequestMapping(C.API + '/' + C.V)", "    static final String API = \"/api\";\n    static final String V = \"v2\";\n    @GetMapping public void x() { }"),
		}, "GET /api/v2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, u := routes(extractProject(t, tc.files))
			if len(got) != 1 || got[0] != tc.want || len(u) != 0 {
				t.Errorf("routes = %v, unresolved = %v; want [%s]", got, u, tc.want)
			}
		})
	}
}

// TestRoutePath_AmbiguityIsUnknown is §2's rule above all others, one case
// per way a name could be settled by guessing — the treatment ADR 0022
// gave annotation identity. In every case a plausible candidate exists in
// the tree, and taking it would be a pick.
func TestRoutePath_AmbiguityIsUnknown(t *testing.T) {
	inTree := "package app.k;\npublic class Routes { public static final String ADMIN = \"/admin\"; }\n"
	for _, tc := range []struct {
		name   string
		files  map[string]string
		reason string
	}{
		{
			// A single-type import names a class outside the tree. The
			// same-named class in C's own package — the next step Java
			// would try — must not be taken instead.
			"an import of a class outside the tree does not fall through", map[string]string{
				"Routes.java": "package app;\npublic class Routes { public static final String ADMIN = \"/admin\"; }\n",
				"C.java":      controller("import com.vendor.Routes;", "@RequestMapping(Routes.ADMIN)", "    @GetMapping public void x() { }"),
			}, unresolvedOutside,
		},
		{
			"two on-demand imports supplying the same class", map[string]string{
				"k/Routes.java": inTree,
				"j/Routes.java": "package app.j;\npublic class Routes { public static final String ADMIN = \"/other\"; }\n",
				// Spring imported by name: an on-demand import of its
				// package would make the name unknown for a different
				// reason (the case below), and hide this one.
				"C.java": "package app;\nimport app.k.*;\nimport app.j.*;\n" +
					"import org.springframework.web.bind.annotation.RestController;\n" +
					"import org.springframework.web.bind.annotation.RequestMapping;\n" +
					"import org.springframework.web.bind.annotation.GetMapping;\n" +
					"@RestController\n@RequestMapping(Routes.ADMIN)\npublic class C {\n    @GetMapping public void x() { }\n}\n",
			}, unresolvedAmbiguous,
		},
		{
			// A package this tree does not contain could declare Routes.
			"an on-demand import of a package outside the tree", map[string]string{
				"k/Routes.java": inTree,
				"C.java":        controller("import app.k.*;\nimport com.vendor.*;", "@RequestMapping(Routes.ADMIN)", "    @GetMapping public void x() { }"),
			}, unresolvedOutside,
		},
		{
			"two static on-demand imports both declaring the name", map[string]string{
				"k/Routes.java": inTree,
				"j/More.java":   "package app.j;\npublic class More { public static final String ADMIN = \"/other\"; }\n",
				"C.java":        controller("import static app.k.Routes.*;\nimport static app.j.More.*;", "@RequestMapping(ADMIN)", "    @GetMapping public void x() { }"),
			}, unresolvedAmbiguous,
		},
		{
			"a static on-demand import of a class outside the tree", map[string]string{
				"k/Routes.java": inTree,
				"C.java":        controller("import static app.k.Routes.*;\nimport static com.vendor.Paths.*;", "@RequestMapping(ADMIN)", "    @GetMapping public void x() { }"),
			}, unresolvedOutside,
		},
		{
			// A field inherited from a dependency's superclass would shadow
			// the static import in Java, and it cannot be seen.
			"a supertype outside the tree shadows a static import", map[string]string{
				"k/Routes.java": inTree,
				"C.java": "package app;\nimport static app.k.Routes.ADMIN;\nimport com.vendor.BaseController;\nimport org.springframework.web.bind.annotation.*;\n" +
					"@RestController\n@RequestMapping(ADMIN)\npublic class C extends BaseController {\n    @GetMapping public void x() { }\n}\n",
			}, unresolvedOutside,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, u := routes(extractProject(t, tc.files))
			if len(got) != 0 || len(u) != 1 || u[0] != tc.reason {
				t.Errorf("routes = %v, unresolved = %v; want one unresolved endpoint, reason %q", got, u, tc.reason)
			}
		})
	}
}

// TestRoutePath_CharPlusCharIsNotConcatenation pins the guard on char
// literals: in Java 'a' + 'b' is an int, so no String is produced.
func TestRoutePath_CharPlusCharIsNotConcatenation(t *testing.T) {
	got, u := routes(extractProject(t, map[string]string{"C.java": controller("",
		"@RequestMapping('a' + 'b' + \"/x\")", "    @GetMapping public void x() { }")}))
	if len(got) != 0 || len(u) != 1 {
		t.Errorf("routes = %v, unresolved = %v; want the path unresolved", got, u)
	}
}

// TestRoutePath_PlaceholderIsUnresolved is §3, literal and via a constant.
// Before ADR 0039 the literal form was reported verbatim as a route.
func TestRoutePath_PlaceholderIsUnresolved(t *testing.T) {
	m := extractProject(t, map[string]string{"C.java": controller("",
		`@RequestMapping("${app.base:/api}")`, `
    static final String SUB = "${app.sub}/x";
    @GetMapping("/a") public void a() { }
    @GetMapping(SUB) public void b() { }
`)})
	got, u := routes(m)
	if len(got) != 0 || len(u) != 2 {
		t.Fatalf("routes = %v, unresolved = %v; want both unresolved", got, u)
	}
	for _, r := range u {
		if r != unresolvedPlaceholder {
			t.Errorf("reason = %q, want %q", r, unresolvedPlaceholder)
		}
	}
}

// TestRoutePath_ArraysExpand is §4: Spring combines every class path with
// every method path.
func TestRoutePath_ArraysExpand(t *testing.T) {
	m := extractProject(t, map[string]string{"C.java": controller("",
		`@RequestMapping({"/a", "/b"})`, `
    @GetMapping({"/x", "/y"}) public void x() { }
    @PostMapping(value = {"/one"}) public void one() { }
`)})
	got, u := routes(m)
	want := []string{"GET /a/x", "GET /a/y", "GET /b/x", "GET /b/y", "POST /a/one", "POST /b/one"}
	if strings.Join(got, ",") != strings.Join(want, ",") || len(u) != 0 {
		t.Errorf("routes = %v, unresolved = %v; want %v", got, u, want)
	}
}

// TestRoutePath_ConductorFixture is the static-import and placeholder case
// on real code (testdata/conductor/NOTICE.md).
func TestRoutePath_ConductorFixture(t *testing.T) {
	m, _, err := Extract("testdata/conductor")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	var admin, placeholders int
	for _, e := range m.Endpoints {
		switch {
		case e.PathUnresolved && e.PathUnresolvedReason == unresolvedPlaceholder:
			placeholders++
		case e.PathUnresolved:
			t.Errorf("%s %s unresolved for %q, want resolved or a placeholder", e.HTTPMethod, e.Path, e.PathUnresolvedReason)
		case strings.HasPrefix(e.Path, "/api/admin/"):
			admin++
		case strings.Contains(e.Path, "${"):
			t.Errorf("%s %s: a placeholder must never be reported as a path (ADR 0039 §3)", e.HTTPMethod, e.Path)
		}
	}
	if admin != 5 {
		t.Errorf("AdminResource routes under /api/admin = %d, want 5 (ADMIN via static import = API_PREFIX + \"admin\")", admin)
	}
	if placeholders == 0 {
		t.Error("A2AServerResource's placeholder paths must be unresolved, with that reason")
	}
}

// TestRoutePath_NacosFixture is the duplicate-simple-name case on real
// code (testdata/nacos/NOTICE.md): two classes named Constants, both
// genuine nacos files, and the file's own import deciding.
func TestRoutePath_NacosFixture(t *testing.T) {
	m, _, err := Extract("testdata/nacos")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(m.Endpoints) == 0 {
		t.Fatal("no endpoints extracted")
	}
	for _, e := range m.Endpoints {
		if e.PathUnresolved || !strings.HasPrefix(e.Path, "/v3/admin/ai/a2a") {
			t.Errorf("%s %s (unresolved=%v, %q), want resolved under /v3/admin/ai/a2a", e.HTTPMethod, e.Path, e.PathUnresolved, e.PathUnresolvedReason)
		}
	}
}
