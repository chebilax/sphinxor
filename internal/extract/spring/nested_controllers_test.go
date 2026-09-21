package spring

import (
	"testing"
)

// TestNestedController_Resolved is ADR 0034. Before it, a nested
// @RestController yielded no endpoints AND no controller, so it was
// invisible rather than incomplete — ADR 0032's detection needs a
// recognized controller to report on, so nothing covered this.
func TestNestedController_Resolved(t *testing.T) {
	m := extractProject(t, map[string]string{
		"Outer.java": `package app;
import org.springframework.web.bind.annotation.*;

public class Outer {
    @RestController
    @RequestMapping("/inner")
    public static class Inner {
        @GetMapping("/b")
        public String b() { return ""; }

        @PostMapping("/c")
        public void c() { }
    }
}
`,
	})
	if len(m.Controllers) != 1 || m.Controllers[0].Name != "Inner" {
		t.Fatalf("the nested class is a controller, got %+v", m.Controllers)
	}
	got := map[string]bool{}
	for _, e := range m.Endpoints {
		got[string(e.HTTPMethod)+" "+e.Path] = true
	}
	for _, want := range []string{"GET /inner/b", "POST /inner/c"} {
		if !got[want] {
			t.Errorf("missing %s; got %v", want, got)
		}
	}
}

// TestNestedController_OuterAndInnerBothCount covers the case the old
// walk half-handled: an outer controller was read and its nested one
// silently dropped.
func TestNestedController_OuterAndInnerBothCount(t *testing.T) {
	m := extractProject(t, map[string]string{
		"Outer.java": `package app;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/outer")
public class Outer {
    @GetMapping("/a")
    public String a() { return ""; }

    @RestController
    @RequestMapping("/inner")
    public static class Inner {
        @GetMapping("/b")
        public String b() { return ""; }
    }
}
`,
	})
	if len(m.Controllers) != 2 {
		t.Fatalf("both classes are controllers, got %d: %+v", len(m.Controllers), m.Controllers)
	}
	if len(m.Endpoints) != 2 {
		t.Fatalf("want 2 endpoints, got %d", len(m.Endpoints))
	}
}

// TestNestedController_GuardsAttach checks the part that matters for an
// authorization tool: a nested controller's guards must reach its
// endpoints exactly as a top-level one's do. The routes appearing
// without their protection would be worse than both being absent.
func TestNestedController_GuardsAttach(t *testing.T) {
	m := extractProject(t, map[string]string{
		"Config.java": `package app;
import org.springframework.security.config.annotation.method.configuration.EnableMethodSecurity;

@Configuration
@EnableMethodSecurity
public class Config { }
`,
		"Outer.java": `package app;
import org.springframework.security.access.prepost.PreAuthorize;
import org.springframework.web.bind.annotation.*;

public class Outer {
    @RestController
    @PreAuthorize("hasRole('ADMIN')")
    public static class Inner {
        @PostMapping("/danger")
        public void danger() { }
    }
}
`,
	})
	if len(m.GuardApplications) != 1 {
		t.Fatalf("the class-level guard must apply, got %+v", m.GuardApplications)
	}
	var roles []string
	for _, r := range m.RoleReferences {
		roles = append(roles, r.RawLiteral)
	}
	if len(roles) != 1 || roles[0] != "ADMIN" {
		t.Errorf("roles = %v, want [ADMIN]", roles)
	}
}

// TestNestedController_PlainNestedClassIsNotAController is the negative
// control: descending must not turn every inner class into a controller.
func TestNestedController_PlainNestedClassIsNotAController(t *testing.T) {
	m := extractProject(t, map[string]string{
		"Outer.java": `package app;
import org.springframework.web.bind.annotation.*;

@RestController
public class Outer {
    @GetMapping("/a")
    public String a() { return ""; }

    public static class Dto {
        private String name;
        public String getName() { return name; }
    }
}
`,
	})
	if len(m.Controllers) != 1 {
		t.Errorf("a plain nested class is not a controller, got %+v", m.Controllers)
	}
}
