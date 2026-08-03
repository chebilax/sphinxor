package diff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/extract/nestjs"
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
	return Snapshot{Model: m, Findings: findings}
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
// This is deliberately NOT expected to gate: mutating-endpoint-without-access-control
// is Low confidence (ADR 0004), and Low never gates (ADR 0007 §3) — a
// removed guard is real, structural information (it shows up in
// BecamePublic), but not, on its own, the kind of finding this tool
// states High confidence about. Asserting it does NOT gate is as
// important a check as asserting the structural change is seen at all.
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

	if result.HasRegressions() {
		t.Errorf("a Low-confidence-only change (guard removed) must not gate CI, got regressions: %+v", result.Regressions)
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
