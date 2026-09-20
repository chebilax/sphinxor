package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
)

// TestRolesUnresolved_DeclaredEmptyVersusUnread is the regression for
// docs/decisions/0020-unanalyzable-is-unknown-not-absent.md Amendment 3 §9.
//
// Both shapes below produce DeclaresRoles: true with zero RoleReferences,
// which is exactly what made them indistinguishable before this amendment
// and cost 675 High-confidence, CI-gating false positives across four
// production repositories. They are deliberately in one source file: the
// failure mode being guarded against is not "empty-role fires too often"
// but "the two cases get conflated again", and a test that only covered
// the unread side would pass just as happily if the fix silenced both.
//
//   - @Secured({}) was read, and is empty. empty-role must fire — this is
//     the case the rule exists for.
//   - @Secured(resource = ..., action = ...) was not read at all. It is
//     alibaba/nacos's own same-named annotation (392 of the 675), and
//     empty-role must not fire, because nothing is known about what it
//     requires.
func TestRolesUnresolved_DeclaredEmptyVersusUnread(t *testing.T) {
	src := `
@RestController
@RequestMapping("/api")
public class ThingController {
    @Secured({})
    @PostMapping("/declared-empty")
    public void declaredEmpty() {}

    @Secured(resource = "nacos/thing", action = ActionTypes.WRITE)
    @PostMapping("/unread-named-attributes")
    public void unreadNamedAttributes() {}

    @PreAuthorize("@ss.hasPermi('system:user:edit')")
    @PostMapping("/unread-bean-call")
    public void unreadBeanCall() {}

    @Secured({"ROLE_ADMIN"})
    @PostMapping("/resolved")
    public void resolved() {}
}
`
	root, source := parseJava(t, src)
	b := newBuilder()
	extractControllers(root, source, "ThingController.java", b, nil)

	byHandler := make(map[string]model.GuardApplication)
	endpointHandler := make(map[model.ID]string)
	for _, e := range b.model.Endpoints {
		endpointHandler[e.ID] = e.HandlerName
	}
	for _, g := range b.model.GuardApplications {
		byHandler[endpointHandler[g.EndpointID]] = g
	}
	if len(byHandler) != 4 {
		t.Fatalf("got guards on %d handlers, want 4: %+v", len(byHandler), byHandler)
	}

	for _, tc := range []struct {
		handler        string
		wantUnresolved bool
		why            string
	}{
		{"declaredEmpty", false, "@Secured({}) is an empty array literal: read, and genuinely empty"},
		{"unreadNamedAttributes", true, "@Secured(resource=..., action=...) carries no string array to read"},
		{"unreadBeanCall", true, "a bean-call SpEL expression is outside the recognized subset"},
		{"resolved", false, "@Secured({\"ROLE_ADMIN\"}) resolves normally"},
	} {
		g := byHandler[tc.handler]
		if !g.DeclaresRoles {
			t.Errorf("%s: DeclaresRoles should stay true (ADR 0011 §1 fusion is unchanged by Amendment 3)", tc.handler)
		}
		if g.RolesUnresolved != tc.wantUnresolved {
			t.Errorf("%s: RolesUnresolved = %v, want %v — %s", tc.handler, g.RolesUnresolved, tc.wantUnresolved, tc.why)
		}
	}

	// End to end through the real rule, not just the extraction flag:
	// the flag only matters because of what empty-role does with it.
	findings := lint.EmptyRole{}.Check(&b.model)
	var flagged []string
	for _, f := range findings {
		flagged = append(flagged, endpointHandler[f.SubjectID])
	}
	if len(flagged) != 1 || flagged[0] != "declaredEmpty" {
		t.Errorf("empty-role fired on %v, want exactly [declaredEmpty]", flagged)
	}
}

// TestRolesUnresolved_PermitAllStillFires pins Amendment 3 §10 from the
// rule's side. TestExtractControllers_PermitAllStillDeclaresRoles already
// pins the extraction flag; this pins the consequence that ADR 0017
// actually decided — that permitAll() keeps surfacing through empty-role —
// so a later change to §9's classification cannot quietly retire it.
func TestRolesUnresolved_PermitAllStillFires(t *testing.T) {
	src := `
@RestController
public class ThingController {
    @PreAuthorize("permitAll()")
    @PostMapping("/a")
    public void a() {}
}
`
	root, source := parseJava(t, src)
	b := newBuilder()
	extractControllers(root, source, "ThingController.java", b, nil)

	if len(b.model.GuardApplications) != 1 {
		t.Fatalf("got %d GuardApplications, want 1", len(b.model.GuardApplications))
	}
	if b.model.GuardApplications[0].RolesUnresolved {
		t.Error("permitAll(): RolesUnresolved must stay false — it is read, not unread (Amendment 3 §10)")
	}
	if got := (lint.EmptyRole{}).Check(&b.model); len(got) != 1 {
		t.Errorf("permitAll() must still produce empty-role (ADR 0017's boundary), got %d findings", len(got))
	}
}
