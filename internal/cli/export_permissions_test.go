package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/cerbostest"
	"github.com/chebilax/sphinxor/internal/export/cerbos"
	"github.com/chebilax/sphinxor/internal/extract/spring"
)

// Tests for docs/decisions/0041-permission-export.md at the command
// line, where §1's condition and §3's refusal live.

func runExport(t *testing.T, args ...string) (outDir string, err error) {
	t.Helper()
	outDir = filepath.Join(t.TempDir(), "policies")
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"export", "cerbos", "--out", outDir}, args...))
	return outDir, cmd.Execute()
}

func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(dir, p)
			files[rel] = string(b)
		}
		return nil
	})
	return files
}

// TestExportCerbos_NoDeclarationsNeverRefusesAndIsUnchanged is ADR 0041
// §1's condition, stated by the owner when accepting it: with none of the
// new flags the export must succeed and produce exactly what it did
// before, or existing CI exports break. The fixture carries permission
// references, so it is the case the refusal could wrongly reach.
func TestExportCerbos_NoDeclarationsNeverRefusesAndIsUnchanged(t *testing.T) {
	const fixture = "../extract/spring/testdata/ruoyi-vue-pro"
	outDir, err := runExport(t, fixture)
	if err != nil {
		t.Fatalf("an export with no ADR 0041 flags was refused: %v", err)
	}

	// Unchanged: the command's output equals the pre-ADR-0041 path,
	// Translate, written the same way.
	m, _, err := spring.Extract(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cerbos.Translate(m), cerbos.TranslateWith(m, cerbos.Declarations{})) {
		t.Fatal("TranslateWith with no declarations differs from Translate")
	}
	want := filepath.Join(t.TempDir(), "want")
	if _, err := cerbos.WritePolicies(want, cerbos.Translate(m)); err != nil {
		t.Fatal(err)
	}
	if got, exp := readTree(t, outDir), readTree(t, want); !reflect.DeepEqual(got, exp) {
		t.Errorf("policies differ from the pre-ADR-0041 output:\ngot  %v\nwant %v", got, exp)
	}
	d, err := buildDeclarations(nil, nil, nil, nil, false)
	if err != nil || d.Active() {
		t.Errorf("no flags must give the zero declarations and no error, got %+v, %v", d, err)
	}
}

// TestExportCerbos_RefusesWithoutEscape is §3: a declared callee with the
// superuser escape neither declared nor waived is refused, before
// anything is written.
func TestExportCerbos_RefusesWithoutEscape(t *testing.T) {
	outDir, err := runExport(t, "../extract/spring/testdata/ruoyi-vue-pro", "--permission-callee", "@ss.hasPermission")
	if !errors.Is(err, cerbos.ErrEscapeUndeclared) {
		t.Fatalf("err = %v, want ErrEscapeUndeclared", err)
	}
	if !strings.Contains(err.Error(), "Amendment 1") {
		t.Errorf("the refusal must cite why: %v", err)
	}
	if _, statErr := os.Stat(outDir); !os.IsNotExist(statErr) {
		t.Errorf("a refused export must write nothing, but %s exists", outDir)
	}
	for _, args := range [][]string{
		{"--superuser-role", "admin"}, // an escape with nothing declared
		{"--permission-callee", "@ss.hasPermission", "--no-superuser-escape", "--superuser-role", "admin"}, // both
	} {
		if _, err := runExport(t, append([]string{"../extract/spring/testdata/ruoyi-vue-pro"}, args...)...); err == nil {
			t.Errorf("%v: want a refusal", args)
		}
	}
}

// TestExportCerbos_DeclaredPermissionsInTheRealEngine exports a
// RuoYi-shaped project with declarations and runs the generated policies
// through the real cerbos CLI with a test suite: a holder of the
// permission is allowed, a principal without it is denied, and the
// declared superuser (RuoYi's *:*:*) is allowed. That last case is ADR
// 0035 Amendment 1's escape, which a literal-only policy would deny.
func TestExportCerbos_DeclaredPermissionsInTheRealEngine(t *testing.T) {
	project := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(project, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("src/main/java/app/SecurityConfig.java", `package app;
import org.springframework.security.config.annotation.method.configuration.EnableMethodSecurity;
@EnableMethodSecurity
public class SecurityConfig { }
`)
	write("src/main/java/app/SysUserController.java", `package app;
import org.springframework.security.access.prepost.PreAuthorize;
import org.springframework.web.bind.annotation.*;
@RestController
@RequestMapping("/system/user")
public class SysUserController {
    @PreAuthorize("@ss.hasPermi('system:user:edit')")
    @PutMapping
    public void edit() { }
    @PreAuthorize("@ss.hasPermi('system:user:list')")
    @GetMapping("/list")
    public void list() { }
    @PreAuthorize("@ss.lacksPermi('system:user:list')")
    @DeleteMapping("/{id}")
    public void remove() { }
}
`)
	outDir, err := runExport(t, project,
		"--permission-callee", "@ss.hasPermi", "--superuser-permission", "*:*:*")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	policy := readTree(t, outDir)["sys_user.yaml"]
	if !strings.Contains(policy, `"system:user:edit\" in P.attr.permissions`) {
		t.Fatalf("no permission condition in the policy:\n%s", policy)
	}
	// lacksPermi was not declared, and is a negation: it must not export.
	if !strings.Contains(policy, "callee-not-declared") {
		t.Errorf("the undeclared @ss.lacksPermi endpoint must be omitted as callee-not-declared:\n%s", policy)
	}

	cerbosPath := cerbostest.Binary(t)
	tests := filepath.Join(outDir, "tests")
	if err := os.MkdirAll(tests, 0o755); err != nil {
		t.Fatal(err)
	}
	suite := `name: DeclaredPermissions
principals:
  editor:    {id: e, roles: ["common"], attr: {permissions: ["system:user:edit"]}}
  lister:    {id: l, roles: ["common"], attr: {permissions: ["system:user:list"]}}
  superuser: {id: s, roles: ["common"], attr: {permissions: ["*:*:*"]}}
  nobody:    {id: n, roles: ["common"], attr: {permissions: []}}
resources:
  u: {kind: sys_user, id: "1"}
tests:
  - name: declared semantics
    input: {principals: [editor, lister, superuser, nobody], resources: [u], actions: [put, get, delete]}
    expected:
      - {principal: editor,    resource: u, actions: {put: EFFECT_ALLOW, get: EFFECT_DENY,  delete: EFFECT_DENY}}
      - {principal: lister,    resource: u, actions: {put: EFFECT_DENY,  get: EFFECT_ALLOW, delete: EFFECT_DENY}}
      - {principal: superuser, resource: u, actions: {put: EFFECT_ALLOW, get: EFFECT_ALLOW, delete: EFFECT_DENY}}
      - {principal: nobody,    resource: u, actions: {put: EFFECT_DENY,  get: EFFECT_DENY,  delete: EFFECT_DENY}}
`
	if err := os.WriteFile(filepath.Join(tests, "sys_user_test.yaml"), []byte(suite), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(cerbosPath, "compile", outDir).CombinedOutput()
	if err != nil {
		t.Fatalf("cerbos compile with tests failed:\n%s\npolicy:\n%s", out, policy)
	}
}
