package spring

import (
	"sort"
	"testing"

	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
)

// TestExtract_YudaoFixturePermissions pins
// docs/decisions/0035-permissions-in-the-model.md against the real
// vendored fixture (testdata/ruoyi-vue-pro/NOTICE.md), which is the same
// admin-framework lineage as the RuoYi-Vue the decision was measured on.
//
// It is also half of the sigil regression pair. The fixture writes
// @ss.hasPermission('member:user:update') — a project BEAN whose method
// happens to carry the name of Spring Security's own built-in
// hasPermission(...) expression, which ADR 0035 §2 deliberately does not
// read. spel_test.go pins the built-in as unrecognized; this pins the bean
// as read. Neither case means anything without the other.
//
// The predicted numbers are in ADR 0035 §9 and were established before
// any of this was implemented: 5 sites, 5 literal occurrences, 4 distinct
// (member:user:query appears twice), on 5 of the fixture's 11 endpoints.
func TestExtract_YudaoFixturePermissions(t *testing.T) {
	m, _, err := Extract(yudaoFixture)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if got := len(m.PermissionReferences); got != 5 {
		t.Fatalf("got %d permission reference(s), want 5", got)
	}

	distinct := map[string]bool{}
	for _, ref := range m.PermissionReferences {
		if ref.Via != "@ss.hasPermission" {
			t.Errorf("permission %q recorded via %q, want @ss.hasPermission — the bean call must travel with the literal (ADR 0035 §3)", ref.RawLiteral, ref.Via)
		}
		distinct[ref.RawLiteral] = true
	}
	var names []string
	for n := range distinct {
		names = append(names, n)
	}
	sort.Strings(names)
	want := []string{"member:user:query", "member:user:update", "member:user:update-level", "member:user:update-point"}
	if len(names) != len(want) {
		t.Fatalf("distinct permissions = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("distinct permissions = %v, want %v", names, want)
		}
	}

	// The flags, which are what every downstream consumer reads.
	permGuards := 0
	for _, g := range m.GuardApplications {
		if !g.DeclaresPermissions {
			continue
		}
		permGuards++
		if !g.DeclaresRoles {
			t.Errorf("guard at %s:%d: DeclaresRoles must stay true — ADR 0011 §1's fusion is unchanged by ADR 0035 §4", g.File, g.Line)
		}
		if g.RolesUnresolved {
			t.Errorf("guard at %s:%d: RolesUnresolved must be false — the expression was read end to end", g.File, g.Line)
		}
	}
	if permGuards != 5 {
		t.Errorf("got %d guard(s) with DeclaresPermissions, want 5", permGuards)
	}

	// THE REGRESSION BAR (ADR 0035 §4, §9). A guard with DeclaresRoles:
	// true, RolesUnresolved: false and zero RoleReferences is character
	// for character empty-role's trigger. Without DeclaresPermissions in
	// that rule this fires 5 times here and 205 times across the corpus,
	// at High confidence, which gates CI — the 675-finding defect ADR
	// 0020 Amendment 3 exists to have fixed, recreated by the change that
	// improved the same code.
	for _, f := range (lint.EmptyRole{}).Check(m) {
		t.Errorf("empty-role fired on a permission-bearing endpoint: %s", f.Message)
	}

	// And the rest of the fixture's reported state is unchanged, per
	// ADR 0035 §9's prediction table.
	if got := len(m.Endpoints); got != 11 {
		t.Errorf("got %d endpoints, want 11 — this decision must not move endpoint discovery", got)
	}
	findings := lint.Run(m, lint.DefaultRules(), nil)
	if got := len(findings); got != 5 {
		t.Errorf("got %d finding(s), want 5 (unchanged by this decision): %+v", got, findings)
	}
}

// TestExtract_PermissionLiteralIsNotARoleDeclaration is ADR 0035 §7's
// negative consumer, and it is the one that fails silently rather than
// loudly.
//
// collectUsedRoleLiterals feeds extractRoleDeclarations, which promotes
// any enum constant or String constant whose value appears in that set
// into a RoleDeclaration. If a permission literal leaked into it, a
// project with `static final String EDIT = "system:user:edit"` would gain
// a role named "system:user:edit" and permission-declared-but-unreferenced
// would then fire on it. No corpus project has such a constant today, so
// nothing would have caught this; the test is what keeps that zero from
// resting on luck.
func TestExtract_PermissionLiteralIsNotARoleDeclaration(t *testing.T) {
	src := `
import org.springframework.security.access.prepost.PreAuthorize;

class Perms {
    public static final String EDIT = "system:user:edit";
}

@RestController
@RequestMapping("/api")
public class ThingController {
    @PreAuthorize("@ss.hasPermi('system:user:edit')")
    @PostMapping("/edit")
    public void edit() {}
}
`
	root, source := parseJava(t, src)
	used := collectUsedRoleLiterals(root, source)
	if used["system:user:edit"] {
		t.Fatal("a permission literal entered the role-declaration usage filter; a String constant holding it would become a RoleDeclaration (ADR 0035 §7)")
	}

	b := newBuilder()
	decls := extractRoleDeclarations(root, source, "ThingController.java", b.nextID("role"), used)
	if len(decls) != 0 {
		t.Errorf("got %d role declaration(s), want 0: %+v", len(decls), decls)
	}
}

// TestExtract_PermissionSuppressesAuthenticationRequirement is ADR 0035
// §7's other silent consumer.
//
// An AuthenticationRequirement means "authenticated, any role" (ADR 0010)
// and exports as Cerbos's `*`. It is created only when nothing stricter
// was found on the same layer, and a named permission requirement is
// stricter — so an endpoint carrying both isAuthenticated() and a bean
// call must not be recorded as "any authenticated principal".
//
// SYNTHETIC, deliberately, and docs/testing.md's real-fixture bar is met
// elsewhere for this code path. No repository in the 20-project corpus
// puts isAuthenticated() and a permission bean call on one endpoint — the
// three that carry bean calls use no isAuthenticated() at all — which is
// precisely why this consumer would have gone wrong unnoticed, and why
// reaching for a 21st repository to find the shape would be chasing the
// rule's letter past its purpose.
func TestExtract_PermissionSuppressesAuthenticationRequirement(t *testing.T) {
	src := `
import org.springframework.security.access.prepost.PreAuthorize;
@RestController
@RequestMapping("/api")
@PreAuthorize("isAuthenticated()")
public class ThingController {
    @PreAuthorize("@ss.hasPermi('system:user:edit')")
    @PostMapping("/edit")
    public void edit() {}

    @PostMapping("/plain")
    public void plain() {}
}
`
	root, source := parseJava(t, src)
	b := newBuilder()
	extractControllers(root, source, "ThingController.java", b, nil, nil)
	b.model.AuthenticationRequirements = computeAuthenticationRequirements(&b.model, b.authCandidates, b.nextID("authreq"))

	handler := map[model.ID]string{}
	for _, e := range b.model.Endpoints {
		handler[e.ID] = e.HandlerName
	}
	got := map[string]bool{}
	for _, r := range b.model.AuthenticationRequirements {
		got[handler[r.EndpointID]] = true
	}
	if got["edit"] {
		t.Error("the permission-bearing endpoint was recorded as \"authenticated, any role\"; the permission is stricter and must suppress it (ADR 0035 §7)")
	}
	if !got["plain"] {
		t.Error("the endpoint with only isAuthenticated() lost its AuthenticationRequirement; ADR 0010's behaviour must be unchanged")
	}
}
