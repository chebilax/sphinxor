package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

func unrecovered(m *model.Model) (none, partial []string) {
	for _, c := range m.UnrecoveredRoutes {
		if c.NoRoutesAtAll {
			none = append(none, c.Name)
		} else {
			partial = append(partial, c.Name)
		}
	}
	return
}

// TestUnrecovered_ConditionA is ADR 0032 §1's first condition: a
// recognized controller from which nothing came out. Modelled on
// apollo's OpenAPI controllers.
func TestUnrecovered_ConditionA(t *testing.T) {
	m := extractProject(t, map[string]string{
		"C.java": `package app;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class AppController implements AppManagementApi {
    @Override
    public String listApps() { return ""; }
}
`,
	})
	none, partial := unrecovered(m)
	if len(none) != 1 || none[0] != "AppController" {
		t.Errorf("want AppController reported with no routes, got none=%v partial=%v", none, partial)
	}
}

// TestUnrecovered_ConditionB_ExternalInterface is the partially-inherited
// case, and the reason condition A alone was not enough. apollo's
// PortalManagementController has 47 @Override methods, 9 with an inline
// mapping and 38 without: it yields endpoints, so condition A never sees
// it, and the matrix shows it looking complete.
func TestUnrecovered_ConditionB_ExternalInterface(t *testing.T) {
	m := extractProject(t, map[string]string{
		"C.java": `package app;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class PortalManagementController implements PortalManagementApi {
    @Override
    @GetMapping("/visible")
    public String visible() { return ""; }

    @Override
    public String hidden() { return ""; }
}
`,
	})
	if len(m.Endpoints) != 1 {
		t.Fatalf("the mapped handler is still an endpoint, got %d", len(m.Endpoints))
	}
	none, partial := unrecovered(m)
	if len(partial) != 1 || partial[0] != "PortalManagementController" {
		t.Errorf("want it reported as producing fewer routes than it declares, got none=%v partial=%v", none, partial)
	}
}

// TestUnrecovered_RouteBearingInterfaceIsEnough pins the arm that
// shenyu's PagedController forced.
//
// It declares @PostMapping on DEFAULT methods, and the eight
// shenyu-admin controllers implementing it override only pageService(),
// which is not a route. Counting unmapped @Override methods flagged
// those eight for a reason unrelated to why their routes are missing, so
// a route-bearing in-repo interface now suffices on its own.
func TestUnrecovered_RouteBearingInterfaceIsEnough(t *testing.T) {
	m := extractProject(t, map[string]string{
		"PagedController.java": `package app;
import org.springframework.web.bind.annotation.PostMapping;

public interface PagedController<V, T> {
    @PostMapping("list/search")
    default String search() { return ""; }
}
`,
		"PluginController.java": `package app;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class PluginController implements PagedController<String, String> {
    @GetMapping("/plugins")
    public String list() { return ""; }

    @Override
    public String pageService() { return ""; }
}
`,
	})
	_, partial := unrecovered(m)
	if len(partial) != 1 || partial[0] != "PluginController" {
		t.Errorf("a route-bearing interface hides routes whether or not they are overridden; got %v", partial)
	}
}

// TestUnrecovered_NonRoutingInterfaceIsIgnored is the negative control,
// and the measured reason condition B resolves the interface instead of
// matching on @Override alone. Stated without it, the condition fired on
// InitializingBean, Function, Predicate, Callback, SubscriptionOutput
// and ClientRegisterConfig across five repositories.
func TestUnrecovered_NonRoutingInterfaceIsIgnored(t *testing.T) {
	for _, tc := range []struct{ name, decl, impl string }{
		{
			"in-repo interface bearing no mappings",
			`package app;
public interface SubscriptionOutput {
    void onInit();
}
`,
			"SubscriptionOutput",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := extractProject(t, map[string]string{
				"Iface.java": tc.decl,
				"C.java": `package app;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class StreamController implements ` + tc.impl + ` {
    @GetMapping("/x")
    public String x() { return ""; }

    @Override
    public void onInit() { }
}
`,
			})
			if _, partial := unrecovered(m); len(partial) != 0 {
				t.Errorf("an interface declaring no mappings is not a routing interface; got %v", partial)
			}
		})
	}
}

// TestUnrecovered_FrameworkInterfaceIsIgnored covers the other exclusion
// arm: a controller implementing a JDK or Spring type is implementing a
// lifecycle or functional interface, not declaring routes.
func TestUnrecovered_FrameworkInterfaceIsIgnored(t *testing.T) {
	m := extractProject(t, map[string]string{
		"C.java": `package app;
import java.util.function.Function;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class TaskDefinitionController implements Function<String, String> {
    @GetMapping("/tasks")
    public String list() { return ""; }

    @Override
    public String apply(String in) { return in; }
}
`,
	})
	if _, partial := unrecovered(m); len(partial) != 0 {
		t.Errorf("java.util.function.Function is not a routing interface; got %v", partial)
	}
}

// TestUnrecovered_AmbiguousInterfaceIsNotFlagged pins the JeecgBoot
// case. It declares org.jeecg.common.airag.api.IAiragBaseApi twice — in
// its local-api module with no mappings, and in its cloud-api module as
// a Feign client with seven. Which is on the classpath is a build
// profile, so announcing on the strength of the routing one would be a
// guess.
//
// This is also why the index is keyed by fully-qualified name: a
// simple-name index conflated the two and produced the corpus's only
// false positive for this condition.
func TestUnrecovered_AmbiguousInterfaceIsNotFlagged(t *testing.T) {
	m := extractProject(t, map[string]string{
		"local/IAiragBaseApi.java": `package org.jeecg.common.airag.api;
public interface IAiragBaseApi {
    String writeDoc(String id);
}
`,
		"cloud/IAiragBaseApi.java": `package org.jeecg.common.airag.api;
import org.springframework.web.bind.annotation.PostMapping;

public interface IAiragBaseApi {
    @PostMapping("/airag/write")
    String writeDoc(String id);
}
`,
		"C.java": `package app;
import org.jeecg.common.airag.api.IAiragBaseApi;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class AiragBaseApiController implements IAiragBaseApi {
    @GetMapping("/ok")
    public String ok() { return ""; }

    @Override
    public String writeDoc(String id) { return ""; }
}
`,
	})
	if _, partial := unrecovered(m); len(partial) != 0 {
		t.Errorf("two declarations disagreeing about routes is ambiguous, not a warning; got %v", partial)
	}
}

// TestUnrecovered_OrdinaryControllerIsQuiet is the noise floor. Sixteen
// of the twenty corpus repositories produce nothing here, and a
// condition that fired on a plain controller would be worthless.
func TestUnrecovered_OrdinaryControllerIsQuiet(t *testing.T) {
	m := extractProject(t, map[string]string{
		"C.java": `package app;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/things")
public class ThingController {
    @GetMapping("/a")
    public String a() { return ""; }

    @PostMapping("/b")
    public void b() { }
}
`,
	})
	if len(m.UnrecoveredRoutes) != 0 {
		t.Errorf("an ordinary controller must be silent, got %+v", m.UnrecoveredRoutes)
	}
}
