package spring

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// extractProject writes files to a temp dir and runs the real Extract, so
// the cross-file pass that finds meta-annotation declarations is exercised
// — a per-file helper could not test this, since shenyu declares @RestApi
// once and uses it 35 files away.
func extractProject(t *testing.T, files map[string]string) *model.Model {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m, _, err := Extract(dir)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

const restApiDecl = `package app.ann;
import org.springframework.core.annotation.AliasFor;
import org.springframework.validation.annotation.Validated;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@Target(ElementType.TYPE)
@Validated
@RestController
@RequestMapping
public @interface RestApi {
    @AliasFor(attribute = "path", annotation = RequestMapping.class)
    String[] value() default {};
}
`

// TestControllerMeta_RestApiResolves is the core regression for
// docs/decisions/0024-controller-meta-annotations.md §1–§3, modelled on
// apache/shenyu's real @RestApi.
//
// The declaration and the controller are in different files on purpose:
// the resolution pass is project-wide precisely because shenyu's are.
func TestControllerMeta_RestApiResolves(t *testing.T) {
	m := extractProject(t, map[string]string{
		"ann/RestApi.java": restApiDecl,
		"PluginHandleController.java": `package app;
import app.ann.RestApi;
import org.springframework.web.bind.annotation.*;

@RestApi("/plugin-handle")
public class PluginHandleController {
    @GetMapping("/all")
    public String all() { return ""; }

    @DeleteMapping("/{id}")
    public void delete(String id) { }
}
`,
	})
	if len(m.Controllers) != 1 {
		t.Fatalf("got %d controllers, want 1 — the meta-annotation should make this a controller", len(m.Controllers))
	}
	if got := m.Controllers[0].BasePath; got != "/plugin-handle" {
		t.Errorf("BasePath = %q, want /plugin-handle — the @AliasFor'd use-site argument", got)
	}
	paths := map[string]bool{}
	for _, e := range m.Endpoints {
		paths[string(e.HTTPMethod)+" "+e.Path] = true
		if e.PathUnresolved {
			t.Errorf("%s %s should have a fully resolved path", e.HTTPMethod, e.Path)
		}
	}
	for _, want := range []string{"GET /plugin-handle/all", "DELETE /plugin-handle/{id}"} {
		if !paths[want] {
			t.Errorf("missing %s; got %v", want, paths)
		}
	}
}

// TestControllerMeta_TwoLevelsNotFollowed pins ADR 0024 §1's depth bound.
//
// The corpus cannot exercise this on a recognized mapping — shenyu's only
// multi-level chains bottom out at @RequestMapping(method = …), which
// ADR 0011 §1 does not read at any depth — so the bound would otherwise
// be untested and could drift silently.
func TestControllerMeta_TwoLevelsNotFollowed(t *testing.T) {
	m := extractProject(t, map[string]string{
		"ann/RestApi.java": restApiDecl,
		"ann/AdminApi.java": `package app.ann;
@Target(ElementType.TYPE)
@RestApi
public @interface AdminApi {
    String[] value() default {};
}
`,
		"C.java": `package app;
import app.ann.AdminApi;
import org.springframework.web.bind.annotation.*;

@AdminApi("/admin")
public class C {
    @GetMapping("/a")
    public String a() { return ""; }
}
`,
	})
	if len(m.Controllers) != 0 {
		t.Errorf("a meta-annotation composing another meta-annotation must not resolve (one level only), got %+v", m.Controllers)
	}
	if len(m.Endpoints) != 0 {
		t.Errorf("no endpoints should be extracted, got %d", len(m.Endpoints))
	}
}

// TestControllerMeta_UnreadableUseSitePathIsUnresolved covers ADR 0024
// §4. All 35 of shenyu's uses are string literals, so nothing in the
// corpus reaches this branch — which is why the behaviour is specified
// and pinned rather than left to be discovered on the first project that
// differs.
func TestControllerMeta_UnreadableUseSitePathIsUnresolved(t *testing.T) {
	m := extractProject(t, map[string]string{
		"ann/RestApi.java": restApiDecl,
		"Routes.java": `package app;
public final class Routes { public static final String ADMIN = "/admin"; }
`,
		"C.java": `package app;
import app.ann.RestApi;
import org.springframework.web.bind.annotation.*;

@RestApi(Routes.ADMIN)
public class C {
    @DeleteMapping("/wipe")
    public void wipe() { }
}
`,
	})
	if len(m.Endpoints) != 1 {
		t.Fatalf("the endpoint must still be extracted, got %d", len(m.Endpoints))
	}
	if !m.Endpoints[0].PathUnresolved {
		t.Errorf("a constant-reference base path must be unresolved, not silently empty (ADR 0024 §4); got %+v", m.Endpoints[0])
	}
}

// TestControllerMeta_DeclarationSitePathWins covers the other half of §2:
// a literal path on the annotation declaration applies to every use.
func TestControllerMeta_DeclarationSitePathWins(t *testing.T) {
	m := extractProject(t, map[string]string{
		"ann/ApiV2.java": `package app.ann;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@Target(ElementType.TYPE)
@RestController
@RequestMapping("/api/v2")
public @interface ApiV2 {
}
`,
		"C.java": `package app;
import app.ann.ApiV2;
import org.springframework.web.bind.annotation.*;

@ApiV2
public class C {
    @GetMapping("/things")
    public String things() { return ""; }
}
`,
	})
	if len(m.Endpoints) != 1 || m.Endpoints[0].Path != "/api/v2/things" {
		t.Errorf("declaration-site @RequestMapping path should apply, got %+v", m.Endpoints)
	}
}

// TestControllerMeta_OrdinaryTypeAnnotationIsNotAController is the
// negative control, and the reason §1 matches on what an annotation
// *composes* rather than on its name. The corpus holds 115
// project-declared @Target(TYPE) annotations of which exactly one is a
// controller; a name-shaped rule would invent controllers out of the
// rest.
func TestControllerMeta_OrdinaryTypeAnnotationIsNotAController(t *testing.T) {
	m := extractProject(t, map[string]string{
		"ann/TbCoreComponent.java": `package app.ann;
import org.springframework.boot.autoconfigure.condition.ConditionalOnExpression;

@Target(ElementType.TYPE)
@ConditionalOnExpression("x")
public @interface TbCoreComponent {
}
`,
		"C.java": `package app;
import app.ann.TbCoreComponent;
import org.springframework.web.bind.annotation.*;

@TbCoreComponent
public class C {
    @PostMapping("/a")
    public void a() { }
}
`,
	})
	if len(m.Controllers) != 0 || len(m.Endpoints) != 0 {
		t.Errorf("an annotation composing no controller must not make its class one, got %d controllers / %d endpoints",
			len(m.Controllers), len(m.Endpoints))
	}
}

// TestControllerMeta_WithinAnnotationAliasNotFollowed pins ADR 0024 §3's
// exclusion. shenyu's @ShenyuGetMapping pairs value and path with
// `@AliasFor(attribute = "path")` — no `annotation =` element — which
// aliases within the same annotation and says nothing about
// @RequestMapping's path.
func TestControllerMeta_WithinAnnotationAliasNotFollowed(t *testing.T) {
	m := extractProject(t, map[string]string{
		"ann/Odd.java": `package app.ann;
import org.springframework.core.annotation.AliasFor;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@Target(ElementType.TYPE)
@RestController
@RequestMapping
public @interface Odd {
    @AliasFor(attribute = "path")
    String[] value() default "";
    @AliasFor(attribute = "value")
    String[] path() default "";
}
`,
		"C.java": `package app;
import app.ann.Odd;
import org.springframework.web.bind.annotation.*;

@Odd("/ignored")
public class C {
    @GetMapping("/a")
    public String a() { return ""; }
}
`,
	})
	// Still a controller — it composes @RestController — but the
	// within-annotation alias does not establish a base path.
	if len(m.Controllers) != 1 {
		t.Fatalf("got %d controllers, want 1", len(m.Controllers))
	}
	if got := m.Controllers[0].BasePath; got != "" {
		t.Errorf("BasePath = %q, want empty: a within-annotation @AliasFor says nothing about @RequestMapping", got)
	}
}
