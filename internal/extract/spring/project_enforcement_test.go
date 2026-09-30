package spring

import (
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/export/cerbos"
)

const enforcementChain = `package app;
import org.springframework.context.annotation.Bean;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.web.SecurityFilterChain;
public class SecurityConfig {
    @Bean
    SecurityFilterChain chain(HttpSecurity http) throws Exception {
        http.authorizeHttpRequests(auth -> auth.requestMatchers("/api/**").authenticated().anyRequest().permitAll());
        return http.build();
    }
}`

const enforcementController = `package app;
import org.springframework.web.bind.annotation.*;
@RestController
@RequestMapping("/api/agents")
public class AgentController {
    @RequireRole("member")
    @DeleteMapping("/{id}")
    public void delete(@PathVariable String id) { }
    @GetMapping("/{id}")
    public String get(@PathVariable String id) { return id; }
}`

const enforcementAnnotation = `package app;
import java.lang.annotation.*;
@Target(ElementType.METHOD) @Retention(RetentionPolicy.RUNTIME)
public @interface RequireRole { String value(); }`

// TestProjectEnforcement_ExportOmits: ADR 0023 Amendment 1, the mateclaw
// shape. /api/** requires authentication, nothing on the method is read,
// and the export granted "*" on DELETE — while the project's own
// interceptor answers 403 unless the caller holds the workspace role.
func TestProjectEnforcement_ExportOmits(t *testing.T) {
	for _, tc := range []struct{ name, reader string }{
		{"interceptor", `package app;
import org.springframework.web.method.HandlerMethod;
import org.springframework.web.servlet.HandlerInterceptor;
public class RoleInterceptor implements HandlerInterceptor {
    public boolean preHandle(Object req, Object res, Object handler) {
        RequireRole r = ((HandlerMethod) handler).getMethodAnnotation(RequireRole.class);
        return r == null || check(r.value());
    }
    private boolean check(String role) { return false; }
}`},
		{"aspect with a bound parameter", `package app;
import org.aspectj.lang.ProceedingJoinPoint;
import org.aspectj.lang.annotation.*;
@Aspect
public class RoleAspect {
    @Around("@annotation(requireRole)")
    public Object check(ProceedingJoinPoint pjp, RequireRole requireRole) throws Throwable {
        throw new SecurityException(requireRole.value());
    }
}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := extractProject(t, map[string]string{
				"SecurityConfig.java":  enforcementChain,
				"AgentController.java": enforcementController,
				"RequireRole.java":     enforcementAnnotation,
				"Reader.java":          tc.reader,
			})
			r := cerbos.Translate(m)
			for _, rule := range r.Rules {
				if rule.Action == "delete" {
					t.Errorf("exported %+v; @RequireRole is enforced by the project and not read", rule)
				}
			}
			var got *cerbos.Omission
			for i, o := range r.Omissions {
				if o.Endpoint.HandlerName == "delete" {
					got = &r.Omissions[i]
				}
			}
			if got == nil || got.Reason != cerbos.ReasonProjectEnforcementUnread || !strings.Contains(got.Detail, "@RequireRole") {
				t.Errorf("DELETE omission = %+v; want %q naming @RequireRole", got, cerbos.ReasonProjectEnforcementUnread)
			}
			// The unannotated GET is untouched: authenticated() still exports.
			exportedGet := false
			for _, rule := range r.Rules {
				if rule.Action == "get" {
					exportedGet = true
				}
			}
			if !exportedGet {
				t.Error("GET was not exported; only annotated endpoints are omitted")
			}
		})
	}
}

// TestProjectEnforcement_NoReaderNoOmission: an annotation declared in the
// project with no aspect or interceptor reading it changes nothing —
// dataease's @DePermit is the case ADR 0023 recorded.
func TestProjectEnforcement_NoReaderNoOmission(t *testing.T) {
	m := extractProject(t, map[string]string{
		"SecurityConfig.java":  enforcementChain,
		"AgentController.java": enforcementController,
		"RequireRole.java":     enforcementAnnotation,
	})
	if len(m.ProjectEnforcements) != 0 {
		t.Errorf("project enforcements = %+v; nothing reads @RequireRole", m.ProjectEnforcements)
	}
}

// TestProjectEnforcement_CanStopTheRequest: a reader counts only if it can
// stop the request (ADR 0023 Amendment 1). Structure alone removed 59 of
// RuoYi-Vue's 100 declared rules because its @Log is read by LogAspect.
func TestProjectEnforcement_CanStopTheRequest(t *testing.T) {
	for _, tc := range []struct {
		name, reader string
		enforcing    bool
	}{
		{name: "a logger: @Before records a time, after-advice swallows every exception", enforcing: false, reader: `package app;
import org.aspectj.lang.JoinPoint;
import org.aspectj.lang.annotation.*;
@Aspect
public class LogAspect {
    private static final ThreadLocal<Long> TIME = new ThreadLocal<>();
    @Before("@annotation(requireRole)")
    public void before(JoinPoint jp, RequireRole requireRole) { TIME.set(System.currentTimeMillis()); }
    @AfterReturning(pointcut = "@annotation(requireRole)", returning = "r")
    public void after(JoinPoint jp, RequireRole requireRole, Object r) { handle(jp, requireRole); }
    protected void handle(JoinPoint jp, RequireRole requireRole) {
        try {
            Audit.record(jp.getSignature().getName(), requireRole.value());
        } catch (Exception e) {
            e.printStackTrace();
        } finally {
            TIME.remove();
        }
    }
}`},
		{name: "@Before denying through a project helper", enforcing: true, reader: `package app;
import org.aspectj.lang.JoinPoint;
import org.aspectj.lang.annotation.*;
@Aspect
public class RoleAspect {
    private Checker checker;
    @Before("@annotation(requireRole)")
    public void before(JoinPoint jp, RequireRole requireRole) { checker.require(requireRole.value()); }
}`},
		{name: "after-advice that throws", enforcing: true, reader: `package app;
import org.aspectj.lang.annotation.*;
@Aspect
public class RoleAspect {
    @AfterReturning(pointcut = "@annotation(requireRole)", returning = "r")
    public void after(RequireRole requireRole, Object r) { if (r == null) throw new SecurityException(requireRole.value()); }
}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := extractProject(t, map[string]string{
				"SecurityConfig.java":  enforcementChain,
				"AgentController.java": enforcementController,
				"RequireRole.java":     enforcementAnnotation,
				"Reader.java":          tc.reader,
				"Checker.java": `package app;
public class Checker { public void require(String role) { throw new SecurityException(role); } }`,
				"Audit.java": `package app;
public class Audit { public static void record(String m, String v) { throw new IllegalStateException(m); } }`,
			})
			if got := len(m.ProjectEnforcements) > 0; got != tc.enforcing {
				t.Errorf("recorded as enforcement: %v, want %v (%+v)", got, tc.enforcing, m.ProjectEnforcements)
			}
		})
	}
}
