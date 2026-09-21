package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/lint"
)

const reactiveConfig = `package app;
import org.springframework.context.annotation.Configuration;
import org.springframework.security.config.annotation.method.configuration.EnableReactiveMethodSecurity;

@Configuration
@EnableReactiveMethodSecurity
public class Config { }
`

// TestReactiveMethodSecurity_PrePostIsEnabled is the core regression for
// ADR 0015 Amendment 1, and the reason the amendment exists.
//
// Before it, a reactive project enabling method security this way was
// told its annotations "are inert at runtime and the endpoints they
// appear to protect are NOT protected" — the one item on ADR 0029's
// silent list where the tool did not merely stay quiet but said
// something false about a correctly configured application.
func TestReactiveMethodSecurity_PrePostIsEnabled(t *testing.T) {
	m := extractProject(t, map[string]string{
		"Config.java": reactiveConfig,
		"C.java": `package app;
import org.springframework.security.access.prepost.PreAuthorize;
import org.springframework.web.bind.annotation.*;

@RestController
public class C {
    @PreAuthorize("hasRole('ADMIN')")
    @PostMapping("/a")
    public void a() { }
}
`,
	})
	if !m.MethodSecurity.Found {
		t.Fatal("@EnableReactiveMethodSecurity must be located")
	}
	if !m.MethodSecurity.PrePostEnabled {
		t.Error("it enables @PreAuthorize unconditionally — that is the annotation's whole purpose")
	}
}

// TestReactiveMethodSecurity_SecuredAndJsr250AreOff pins the half of the
// amendment that was wrong in its first draft.
//
// That draft had JSR-250 following useAuthorizationManager, on the
// strength of the servlet annotation's documentation. Reading Spring's
// two reactive configuration classes falsified it: neither
// ReactiveAuthorizationManagerMethodSecurityConfiguration nor
// ReactiveMethodSecurityConfiguration registers a
// SecuredAuthorizationManager or a Jsr250AuthorizationManager, on either
// side of the flag. Both families are confirmed off.
func TestReactiveMethodSecurity_SecuredAndJsr250AreOff(t *testing.T) {
	for _, useAM := range []string{"", "(useAuthorizationManager = true)", "(useAuthorizationManager = false)"} {
		t.Run("useAuthorizationManager"+useAM, func(t *testing.T) {
			m := extractProject(t, map[string]string{
				"Config.java": `package app;
import org.springframework.security.config.annotation.method.configuration.EnableReactiveMethodSecurity;

@Configuration
@EnableReactiveMethodSecurity` + useAM + `
public class Config { }
`,
			})
			if !m.MethodSecurity.Found || !m.MethodSecurity.PrePostEnabled {
				t.Fatalf("pre/post is unconditional on both paths, got %+v", m.MethodSecurity)
			}
			if m.MethodSecurity.SecuredEnabled {
				t.Error("no reactive configuration registers a SecuredAuthorizationManager")
			}
			if m.MethodSecurity.Jsr250Enabled {
				t.Error("no reactive configuration registers a Jsr250AuthorizationManager")
			}
		})
	}
}

// TestReactiveMethodSecurity_SecuredIsConfirmedInert is the consequence
// of the previous test at the level a user sees, and the reason the
// @Secured answer had to be established from Spring's source rather than
// from the documentation's silence.
//
// Found now being true means ADR 0015's liveness check applies, so a
// @Secured under a reactive-only configuration is downgraded to
// unguarded and the endpoint is flagged. That is a real behaviour change
// riding on the fact, in the direction ADR 0015 calls safe (protection
// understated) — but it is still only defensible because the negative is
// confirmed.
func TestReactiveMethodSecurity_SecuredIsConfirmedInert(t *testing.T) {
	m := extractProject(t, map[string]string{
		"Config.java": reactiveConfig,
		"C.java": `package app;
import org.springframework.security.access.annotation.Secured;
import org.springframework.web.bind.annotation.*;

@RestController
public class C {
    @Secured({"ROLE_ADMIN"})
    @PostMapping("/a")
    public void a() { }
}
`,
	})
	if len(m.GuardApplications) != 1 {
		t.Fatalf("the annotation is still recorded — it is real source (ADR 0011 §1); got %+v", m.GuardApplications)
	}
	findings := (lint.MutatingEndpointWithoutAccessControl{}).Check(m)
	if len(findings) != 1 {
		t.Errorf("a @Secured under reactive-only method security is inert, so the endpoint is unguarded; got %d findings", len(findings))
	}
}

// TestReactiveMethodSecurity_HybridKeepsSecured covers the OR across
// enablers. An application carrying both configurations really does have
// @Secured switched on, by the servlet one, and the reactive enabler
// must not cancel it.
//
// No corpus project does this; halo, the corpus's only reactive project,
// carries the reactive enabler alone.
func TestReactiveMethodSecurity_HybridKeepsSecured(t *testing.T) {
	m := extractProject(t, map[string]string{
		"Reactive.java": reactiveConfig,
		"Servlet.java": `package app;
import org.springframework.security.config.annotation.method.configuration.EnableMethodSecurity;

@Configuration
@EnableMethodSecurity(securedEnabled = true)
public class Servlet { }
`,
	})
	if !m.MethodSecurity.SecuredEnabled {
		t.Errorf("the servlet enabler switches @Secured on; the reactive one must not cancel it, got %+v", m.MethodSecurity)
	}
}

// TestReactiveMethodSecurity_AbsenceStillUnknown is ADR 0015's boundary,
// unchanged by this amendment: a reactive project with no enabler at all
// is still "no evidence either way", not confirmed inert.
func TestReactiveMethodSecurity_AbsenceStillUnknown(t *testing.T) {
	m := extractProject(t, map[string]string{
		"C.java": `package app;
import org.springframework.security.access.prepost.PreAuthorize;
import org.springframework.web.bind.annotation.*;

@RestController
public class C {
    @PreAuthorize("hasRole('ADMIN')")
    @PostMapping("/a")
    public void a() { }
}
`,
	})
	if m.MethodSecurity.Found {
		t.Error("nothing enables method security here; Found must stay false")
	}
	if n := len((lint.MutatingEndpointWithoutAccessControl{}).Check(m)); n != 0 {
		t.Errorf("absence of an enabler never downgrades a guard (ADR 0015), got %d findings", n)
	}
}
