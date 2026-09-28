package diff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/extract/nestjs"
	"github.com/chebilax/sphinxor/internal/extract/spring"
	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
)

// analyze mirrors internal/cli's analyzeDirectory — duplicated rather
// than imported to avoid a diff -> cli dependency (cli already depends
// on diff, so the reverse would cycle).
func analyze(t *testing.T, dir string) Snapshot {
	t.Helper()
	m, outcome, err := nestjs.Extract(dir)
	if err != nil {
		t.Fatalf("Extract(%s): %v", dir, err)
	}
	findings := lint.Run(m, lint.DefaultRules(), outcome.AllowlistedEndpoints)
	findings = append(findings, outcome.StaleMarkers...)
	// AllowlistedEndpoints is carried, not dropped: ADR 0036 §4's gate
	// reads it, and a helper that omitted it would test the gate with an
	// empty allowlist while the real CLI passes a full one.
	return Snapshot{Model: m, Findings: findings, AllowlistedEndpoints: outcome.AllowlistedEndpoints}
}

// copyTree copies the real vendored fixture tree into dst, so each test
// can apply its own targeted source edit without disturbing the shared
// testdata/ fixtures other tests depend on.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copying %s to %s: %v", src, dst, err)
	}
}

func mustReplace(t *testing.T, path, old, new string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, old) {
		t.Fatalf("expected %q to contain %q — the vendored fixture may have changed", path, old)
	}
	content = strings.Replace(content, old, new, 1)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRealWorldDiff_GuardRemoved makes a real edit to a copy of the
// vendored nestjs-boilerplate fixture — stripping UsersController's
// class-level @Roles()/@UseGuards() entirely — rather than constructing
// a synthetic model pair, per the validation this feature was asked to
// meet: diff has to be checked against genuine before/after source, the
// same standard docs/testing.md sets for extraction itself.
//
// THIS TEST'S EXPECTATION WAS INVERTED BY
// docs/decisions/0036-became-public-gates-ci.md, deliberately and not by
// flipping a boolean until the suite passed. What it asserted before:
//
//	"a Low-confidence-only change (guard removed) must not gate CI"
//
// That was a correct reading of ADR 0007 §3, which placed "endpoints that
// became public" under the informational structural diff. It was also the
// reason README.md's claim that diff "fails the build on an endpoint that
// newly lost its protection" was false for the whole of 0.7.x: five
// endpoints lose their class-level guard here and the run exited 0.
//
// ADR 0036 §1 adds case (c) and this is its real-source positive case —
// a genuine vendored fixture with a class-level @Roles()/@UseGuards()
// stripped, not a constructed model pair. The gate is NOT the Low rule
// being promoted (§6 keeps it Low and ungated): it is the TRANSITION,
// which requires protection Sphinxor positively saw in the base.
//
// The structural assertions below are unchanged and still matter: the gate
// must fire on exactly the endpoints BecamePublic reports, never on a
// superset.
func TestRealWorldDiff_GuardRemoved(t *testing.T) {
	baseDir := t.TempDir()
	headDir := t.TempDir()
	copyTree(t, "../extract/nestjs/testdata/nestjs-boilerplate/src", baseDir)
	copyTree(t, "../extract/nestjs/testdata/nestjs-boilerplate/src", headDir)

	mustReplace(t,
		filepath.Join(headDir, "users/users.controller.ts"),
		"@ApiBearerAuth()\n@Roles(RoleEnum.admin)\n@UseGuards(AuthGuard('jwt'), RolesGuard)\n@ApiTags('Users')",
		"@ApiBearerAuth()\n@ApiTags('Users')",
	)

	result := Compare(analyze(t, baseDir), analyze(t, headDir))

	if len(result.BecamePublic) != 5 {
		t.Fatalf("got %d endpoints in BecamePublic, want 5 (every UsersController endpoint, class-level guard removed): %+v", len(result.BecamePublic), result.BecamePublic)
	}
	for _, e := range result.BecamePublic {
		if !strings.HasPrefix(e.Path, "/users") {
			t.Errorf("unexpected endpoint in BecamePublic: %s %s", e.HTTPMethod, e.Path)
		}
	}

	if !result.HasRegressions() {
		t.Fatal("five endpoints lost their only guard and the run did not gate — ADR 0036 §1 case (c)")
	}
	if len(result.Regressions) != 5 {
		t.Errorf("got %d regression(s), want 5 — the gate must fire on exactly the endpoints BecamePublic reports, not a superset: %+v", len(result.Regressions), result.Regressions)
	}
	for _, reg := range result.Regressions {
		if reg.Reason != ReasonBecamePublic {
			t.Errorf("regression %q has reason %q, want %q", reg.Finding.SubjectID, reg.Reason, ReasonBecamePublic)
		}
		if reg.Finding.RuleID != becamePublicRuleID {
			t.Errorf("regression %q has rule %q, want %q", reg.Finding.SubjectID, reg.Finding.RuleID, becamePublicRuleID)
		}
		if reg.Finding.Confidence != model.ConfidenceHigh {
			t.Errorf("regression %q is %q, but only High gates", reg.Finding.SubjectID, reg.Finding.Confidence)
		}
	}

	// ADR 0036 §6: the Low rule is NOT what gates. Every one of these
	// endpoints also carries mutating-endpoint-without-access-control or
	// nothing at all, and neither may contribute a regression.
	for _, reg := range result.Regressions {
		if reg.Finding.RuleID == "mutating-endpoint-without-access-control" {
			t.Errorf("the Low rule gated; ADR 0036 §6 keeps it ungated")
		}
	}
}

// TestRealWorldDiff_AllowlistMarkerRemoved makes a real edit across two
// independent copies of the vendored fixture: base adds a method-level
// @Roles() (empty — a real, if contrived, code state: a role check
// present but requiring nothing) together with a sphinxor-allow marker
// suppressing the resulting empty-role finding; head has the same empty
// @Roles() but the marker has been deleted, as if a reviewer removed it
// without addressing the underlying empty check. This is the exact
// scenario ADR 0007 §3 case (b) exists for.
func TestRealWorldDiff_AllowlistMarkerRemoved(t *testing.T) {
	const target = `  @Delete(':id')
  @ApiParam({
    name: 'id',
    type: String,
    required: true,
  })
  @HttpCode(HttpStatus.NO_CONTENT)
  remove(@Param('id') id: User['id']): Promise<void> {`

	baseDir := t.TempDir()
	headDir := t.TempDir()
	copyTree(t, "../extract/nestjs/testdata/nestjs-boilerplate/src", baseDir)
	copyTree(t, "../extract/nestjs/testdata/nestjs-boilerplate/src", headDir)

	controllerRel := "users/users.controller.ts"
	mustReplace(t, filepath.Join(baseDir, controllerRel), target,
		"  // sphinxor-allow: temporary empty role check while permissions are being redesigned\n  @Roles()\n"+target)
	mustReplace(t, filepath.Join(headDir, controllerRel), target,
		"  @Roles()\n"+target)

	base := analyze(t, baseDir)
	head := analyze(t, headDir)

	// Ground truth check on each side individually, before diffing —
	// same discipline as the extraction validation this mirrors: verify
	// what's actually being compared, don't just trust the diff output.
	if !hasFinding(base.Findings, "empty-role", true) {
		t.Fatalf("base: expected an allowlisted empty-role finding on remove(), got %+v", base.Findings)
	}
	if !hasFinding(head.Findings, "empty-role", false) {
		t.Fatalf("head: expected a non-allowlisted empty-role finding on remove() (marker removed), got %+v", head.Findings)
	}

	result := Compare(base, head)

	if !result.HasRegressions() {
		t.Fatal("removing the allowlist marker from an unchanged High-confidence finding must gate CI, got no regressions")
	}
	var found bool
	for _, reg := range result.Regressions {
		if reg.Finding.RuleID == "empty-role" && reg.Reason == ReasonAllowlistRemoved {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a ReasonAllowlistRemoved regression for empty-role, got %+v", result.Regressions)
	}
}

func hasFinding(findings []model.Finding, ruleID string, allowlisted bool) bool {
	for _, f := range findings {
		if f.RuleID == ruleID && f.Allowlisted == allowlisted {
			return true
		}
	}
	return false
}

// analyzeSpring mirrors analyze for the Spring extractor, which is where
// the corpus's permissions live — ADR 0035 read them on Spring only.
func analyzeSpring(t *testing.T, dir string) Snapshot {
	t.Helper()
	m, outcome, err := spring.Extract(dir)
	if err != nil {
		t.Fatalf("spring.Extract(%s): %v", dir, err)
	}
	findings := lint.Run(m, lint.DefaultRules(), outcome.AllowlistedEndpoints)
	findings = append(findings, outcome.StaleMarkers...)
	return Snapshot{Model: m, Findings: findings, AllowlistedEndpoints: outcome.AllowlistedEndpoints}
}

// TestRealWorldDiff_PermissionChanged makes a real edit to a copy of the
// vendored ruoyi-vue-pro fixture — the one fixture carrying literal
// permissions — rather than constructing a model pair, to the same
// standard TestRealWorldDiff_GuardRemoved is held to.
//
// It is the regression test for ADR 0035 §7's stated gap. Before
// ADR 0037, this edit produced a diff whose every section read "No
// change" — confirmed on RuoYi-Vue itself at 13db1fc, not only on this
// fixture, and it matters most on exactly those projects: RuoYi-Vue and
// eladmin report ZERO roles, so "the diff compares roles" meant the diff
// compared nothing about what their endpoints require.
//
// It also pins the other half of ADR 0037 §3: the change is REPORTED and
// does NOT gate. Sphinxor holds no ordering over roles (ADR 0031) and
// strictly less over permissions — under ADR 0035 §3 it does not even
// decide that @ss.hasPermission denotes a permission check.
func TestRealWorldDiff_PermissionChanged(t *testing.T) {
	baseDir := t.TempDir()
	headDir := t.TempDir()
	copyTree(t, "../extract/spring/testdata/ruoyi-vue-pro", baseDir)
	copyTree(t, "../extract/spring/testdata/ruoyi-vue-pro", headDir)

	mustReplace(t,
		filepath.Join(headDir, "yudao-module-member/src/main/java/cn/iocoder/yudao/module/member/controller/admin/user/MemberUserController.java"),
		"@PreAuthorize(\"@ss.hasPermission('member:user:update-level')\")",
		"@PreAuthorize(\"@ss.hasPermission('member:user:audit-level')\")",
	)

	result := Compare(analyzeSpring(t, baseDir), analyzeSpring(t, headDir))

	if len(result.AddedPermissionReferences) != 1 || result.AddedPermissionReferences[0].RawLiteral != "member:user:audit-level" {
		t.Errorf("added = %+v, want exactly [member:user:audit-level]", result.AddedPermissionReferences)
	}
	if len(result.RemovedPermissionReferences) != 1 || result.RemovedPermissionReferences[0].RawLiteral != "member:user:update-level" {
		t.Errorf("removed = %+v, want exactly [member:user:update-level]", result.RemovedPermissionReferences)
	}

	// The endpoint is protected on both sides, so ADR 0036 §2's
	// presence test does not fire, and nothing else may either.
	if len(result.BecamePublic) != 0 {
		t.Errorf("a permission edit is not a became-public transition: %+v", result.BecamePublic)
	}
	if result.HasRegressions() {
		t.Errorf("a permission literal changing must not gate CI: %+v", result.Regressions)
	}

	// The rest of the structural diff must be silent: nothing about the
	// endpoint, its guard or any role moved.
	if len(result.AddedEndpoints)+len(result.RemovedEndpoints) != 0 {
		t.Errorf("endpoints moved: +%+v -%+v", result.AddedEndpoints, result.RemovedEndpoints)
	}
	if len(result.AddedGuardApplications)+len(result.RemovedGuardApplications) != 0 {
		t.Errorf("guard applications moved: +%+v -%+v", result.AddedGuardApplications, result.RemovedGuardApplications)
	}
	if len(result.AddedRoleReferences)+len(result.RemovedRoleReferences) != 0 {
		t.Errorf("role references moved: +%+v -%+v", result.AddedRoleReferences, result.RemovedRoleReferences)
	}
}

// TestRealWorldDiff_SpringFixtureSelfDiffIsANoOp is ADR 0037 §5's first
// regression bar at fixture scale. A derived key that is not stable
// across two independent extraction runs of the same source shows up
// here as phantom additions and removals — five of them on this fixture,
// 212 across the corpus.
func TestRealWorldDiff_SpringFixtureSelfDiffIsANoOp(t *testing.T) {
	dir := "../extract/spring/testdata/ruoyi-vue-pro"

	base := analyzeSpring(t, dir)
	if len(base.Model.PermissionReferences) != 5 {
		t.Fatalf("fixture carries %d permission reference(s), want 5 — the vendored fixture may have changed", len(base.Model.PermissionReferences))
	}

	result := Compare(base, analyzeSpring(t, dir))

	if len(result.AddedPermissionReferences)+len(result.RemovedPermissionReferences) != 0 {
		t.Errorf("a snapshot compared with itself reported permission changes: +%+v -%+v",
			result.AddedPermissionReferences, result.RemovedPermissionReferences)
	}
	if result.HasRegressions() {
		t.Errorf("self-diff produced regressions: %+v", result.Regressions)
	}
}
