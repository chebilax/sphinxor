package spring

import (
	"testing"
)

const routerImport = `import org.springframework.web.reactive.function.server.RouterFunction;
import org.springframework.web.reactive.function.server.ServerResponse;`

// TestFunctionalRouting_ThreeIdioms is ADR 0033 §1. The return type is
// the unit precisely because these three shapes have nothing else in
// common: counting @Bean methods finds 14 of halo's 82 files, and
// counting RouterFunctions.route( calls misses its springdoc builder.
func TestFunctionalRouting_ThreeIdioms(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want int
	}{
		{
			"@Bean method",
			`package app;
` + routerImport + `
import org.springframework.context.annotation.Bean;
import static org.springframework.web.reactive.function.server.RouterFunctions.route;
import static org.springframework.web.reactive.function.server.RequestPredicates.GET;

public class GatewayConfig {
    @Bean
    public RouterFunction<ServerResponse> indexRouter(@Value("classpath:/doc.html") final Resource indexHtml) {
        return route(GET("/"), request -> null);
    }
}
`,
			1,
		},
		{
			"interface declaration",
			`package app;
` + routerImport + `

public interface CustomEndpoint {
    RouterFunction<ServerResponse> endpoint();
}
`,
			1,
		},
		{
			"implementation of a project interface",
			`package app;
` + routerImport + `

class PolicyEndpoint implements CustomEndpoint {
    @Override
    public RouterFunction<ServerResponse> endpoint() {
        return SpringdocRouteBuilder.route().GET("/policies", this::list).build();
    }
}
`,
			1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := extractProject(t, map[string]string{"F.java": tc.src})
			if got := m.FunctionalRouting.Builders; got != tc.want {
				t.Errorf("Builders = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestFunctionalRouting_ParenthesesInParameters pins the shape that
// broke the measurement this ADR was sized with. JeecgBoot's builder is
//
//	public RouterFunction<ServerResponse> indexRouter(@Value("classpath:/…") final Resource indexHtml)
//
// and a regex matching `\([^)]*\)` skips it, so the first scan reported
// zero for that repository. Tree-sitter does not care; this keeps it
// that way.
func TestFunctionalRouting_ParenthesesInParameters(t *testing.T) {
	m := extractProject(t, map[string]string{
		"F.java": `package app;
` + routerImport + `

public class C {
    public RouterFunction<ServerResponse> r(@Value("classpath:/x(1).html") final Resource x) { return null; }
}
`,
	})
	if m.FunctionalRouting.Builders != 1 {
		t.Errorf("Builders = %d, want 1", m.FunctionalRouting.Builders)
	}
}

// TestFunctionalRouting_CommentedOutIsNotCounted is the other half of
// the same lesson, in the opposite direction: the regex estimate counted
// shenyu's commented-out dynamicRouter and reported 5 where the truth is
// 4.
func TestFunctionalRouting_CommentedOutIsNotCounted(t *testing.T) {
	m := extractProject(t, map[string]string{
		"F.java": `package app;
` + routerImport + `

public class McpServerPluginConfiguration {
//    public RouterFunction<ServerResponse> dynamicRouter(final Manager m) {
//        return RouterFunctions.route(RequestPredicates.all(), m::dispatch);
//    }
}
`,
	})
	if m.FunctionalRouting.Builders != 0 {
		t.Errorf("Builders = %d, want 0 — a commented-out method declares nothing", m.FunctionalRouting.Builders)
	}
}

// TestFunctionalRouting_IdentityIsTheImport is ADR 0033 §5. A
// same-named type from another package is not Spring's, the same rule
// ADR 0022 applies to annotations. No corpus project does this, so
// nothing else would catch a regression.
func TestFunctionalRouting_IdentityIsTheImport(t *testing.T) {
	m := extractProject(t, map[string]string{
		"F.java": `package app;
import com.example.routing.RouterFunction;

public class C {
    public RouterFunction<String> r() { return null; }
}
`,
	})
	if m.FunctionalRouting.Builders != 0 {
		t.Errorf("Builders = %d, want 0 — com.example.routing.RouterFunction is not Spring's", m.FunctionalRouting.Builders)
	}
}

// TestFunctionalRouting_NoRoutesAreInvented is §1/§3: the routes inside
// the builder are not read, so no endpoint appears and no finding can
// fire on one.
func TestFunctionalRouting_NoRoutesAreInvented(t *testing.T) {
	m := extractProject(t, map[string]string{
		"F.java": `package app;
` + routerImport + `
import static org.springframework.web.reactive.function.server.RouterFunctions.route;
import static org.springframework.web.reactive.function.server.RequestPredicates.*;

public class Routes {
    public RouterFunction<ServerResponse> admin() {
        return route(DELETE("/api/v1/users/{name}"), this::del)
            .andRoute(POST("/api/v1/users"), this::create);
    }
}
`,
	})
	if m.FunctionalRouting.Builders != 1 {
		t.Fatalf("Builders = %d, want 1", m.FunctionalRouting.Builders)
	}
	if len(m.Endpoints) != 0 {
		t.Errorf("functional routes are announced, not read; got %d endpoints", len(m.Endpoints))
	}
	if len(m.Controllers) != 0 {
		t.Errorf("a route builder is not a controller; got %d", len(m.Controllers))
	}
}

// TestFunctionalRouting_OrdinaryProjectIsQuiet is the noise floor:
// seventeen of twenty corpus repositories produce nothing here.
func TestFunctionalRouting_OrdinaryProjectIsQuiet(t *testing.T) {
	m := extractProject(t, map[string]string{
		"C.java": `package app;
import org.springframework.web.bind.annotation.*;

@RestController
public class C {
    @GetMapping("/a")
    public String a() { return ""; }
}
`,
	})
	if m.FunctionalRouting.Builders != 0 {
		t.Errorf("an annotated controller declares no functional routes, got %d", m.FunctionalRouting.Builders)
	}
}
