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
//   - @Secured(Roles.ADMIN) was not read at all — a constant reference,
//     which Spring accepts and this extractor cannot resolve. empty-role
//     must not fire, because nothing is known about what it requires.
//
// docs/decisions/0035-permissions-in-the-model.md §9 changed one case here
// and added another, deliberately rather than by flipping an expectation
// until the suite passed. @ss.hasPermi('system:user:edit') is now READ as
// a permission, so its RolesUnresolved is false — and empty-role is kept
// off it by DeclaresPermissions instead, which is the single condition
// standing between this change and 205 High-confidence false positives.
//
// Flipping that case alone would have left @Secured(Roles.ADMIN) holding
// the unread side up on its own, and the unread side would no longer have
// contained a BEAN CALL — the shape that supplied the hazard. So
// unreadArgumentlessBeanCall was added: eladmin's @el.check(), which means
// "requires admin" rather than "requires nothing" because its varargs
// implementation short-circuits on an empty array. Both halves of the
// distinction stay pinned by live assertions.
//
// The nacos shape that supplied 392 of the 675 — @Secured(resource = ...,
// action = ...) from com.alibaba.nacos.auth.annotation — is deliberately
// NOT tested here. Under ADR 0022 it is not a Spring annotation at all
// and never reaches this code path; it is covered in imports_test.go.
func TestRolesUnresolved_DeclaredEmptyVersusUnread(t *testing.T) {
	src := `
import org.springframework.security.access.prepost.PreAuthorize;
import org.springframework.security.access.annotation.Secured;
@RestController
@RequestMapping("/api")
public class ThingController {
    @Secured({})
    @PostMapping("/declared-empty")
    public void declaredEmpty() {}

    @Secured(Roles.ADMIN)
    @PostMapping("/unread-constant-reference")
    public void unreadConstantReference() {}

    @PreAuthorize("@ss.hasPermi('system:user:edit')")
    @PostMapping("/read-bean-call")
    public void readBeanCall() {}

    @PreAuthorize("@el.check()")
    @PostMapping("/unread-argumentless-bean-call")
    public void unreadArgumentlessBeanCall() {}

    @Secured({"ROLE_ADMIN"})
    @PostMapping("/resolved")
    public void resolved() {}
}
`
	root, source := parseJava(t, src)
	b := newBuilder()
	extractControllers(root, source, "ThingController.java", b, nil, nil)

	byHandler := make(map[string]model.GuardApplication)
	endpointHandler := make(map[model.ID]string)
	for _, e := range b.model.Endpoints {
		endpointHandler[e.ID] = e.HandlerName
	}
	for _, g := range b.model.GuardApplications {
		byHandler[endpointHandler[g.EndpointID]] = g
	}
	if len(byHandler) != 5 {
		t.Fatalf("got guards on %d handlers, want 5: %+v", len(byHandler), byHandler)
	}

	for _, tc := range []struct {
		handler         string
		wantUnresolved  bool
		wantPermissions bool
		why             string
	}{
		{"declaredEmpty", false, false, "@Secured({}) is an empty array literal: read, and genuinely empty"},
		{"unreadConstantReference", true, false, "@Secured(Roles.ADMIN) is a constant reference, not a readable string array"},
		{"readBeanCall", false, true, "@ss.hasPermi('system:user:edit') is a bean call with an all-literal argument list: read as a permission (ADR 0035 §2)"},
		{"unreadArgumentlessBeanCall", true, false, "@el.check() names no literal, and an unreadable argument list is never an empty requirement (ADR 0035 §2)"},
		{"resolved", false, false, "@Secured({\"ROLE_ADMIN\"}) resolves normally"},
	} {
		g := byHandler[tc.handler]
		if !g.DeclaresRoles {
			t.Errorf("%s: DeclaresRoles should stay true (ADR 0011 §1 fusion is unchanged by Amendment 3, and by ADR 0035 §4)", tc.handler)
		}
		if g.RolesUnresolved != tc.wantUnresolved {
			t.Errorf("%s: RolesUnresolved = %v, want %v — %s", tc.handler, g.RolesUnresolved, tc.wantUnresolved, tc.why)
		}
		if g.DeclaresPermissions != tc.wantPermissions {
			t.Errorf("%s: DeclaresPermissions = %v, want %v — %s", tc.handler, g.DeclaresPermissions, tc.wantPermissions, tc.why)
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
import org.springframework.security.access.prepost.PreAuthorize;
@RestController
public class ThingController {
    @PreAuthorize("permitAll()")
    @PostMapping("/a")
    public void a() {}
}
`
	root, source := parseJava(t, src)
	b := newBuilder()
	extractControllers(root, source, "ThingController.java", b, nil, nil)

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
