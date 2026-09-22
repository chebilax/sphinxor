package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

func testModel() *model.Model {
	return &model.Model{
		Controllers: []model.Controller{
			{ID: "c1", Name: "UsersController"},
		},
		Endpoints: []model.Endpoint{
			{ID: "e1", HTTPMethod: model.MethodPost, Path: "/users", HandlerName: "create", ControllerID: "c1"},
		},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "e1", GuardName: "AuthGuard"},
			{ID: "g2", EndpointID: "e1", GuardName: "Roles", DeclaresRoles: true},
		},
		RoleReferences: []model.RoleReference{
			{ID: "r1", GuardApplicationID: "g2", RawLiteral: "RoleEnum.admin"},
		},
	}
}

func TestBuildMatrix(t *testing.T) {
	m := testModel()
	findings := []model.Finding{
		{ID: "f1", RuleID: "empty-role", SubjectID: "e1", SubjectKind: model.SubjectEndpoint, Confidence: model.ConfidenceHigh},
	}

	matrix := BuildMatrix(m, findings)

	if len(matrix.Rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(matrix.Rows))
	}
	row := matrix.Rows[0]
	if row.Controller != "UsersController" {
		t.Errorf("controller = %q, want UsersController", row.Controller)
	}
	if len(row.Guards) != 1 || row.Guards[0] != "AuthGuard" {
		t.Errorf("guards = %v, want [AuthGuard] (Roles guard app should not appear as a guard)", row.Guards)
	}
	if len(row.Roles) != 1 || row.Roles[0] != "RoleEnum.admin" {
		t.Errorf("roles = %v, want [RoleEnum.admin]", row.Roles)
	}
	if len(row.Findings) != 1 {
		t.Errorf("row findings = %v, want 1 finding attached", row.Findings)
	}
}

func TestWrite_Markdown(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, testModel(), nil, FormatMarkdown); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "# RBAC Matrix") {
		t.Errorf("markdown output missing header: %s", out)
	}
	if !strings.Contains(out, "/users") {
		t.Errorf("markdown output missing endpoint path: %s", out)
	}
}

func TestWrite_JSON(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, testModel(), nil, FormatJSON); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var decoded Matrix
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON matching Matrix: %v\n%s", err, buf.String())
	}
	if len(decoded.Rows) != 1 {
		t.Errorf("decoded %d rows, want 1", len(decoded.Rows))
	}
}

func TestWrite_UnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, testModel(), nil, Format("yaml")); err == nil {
		t.Fatal("expected an error for an unknown format, got nil")
	}
}

// TestRenderPermissions_QuestionMarkDiscipline pins
// docs/decisions/0035-permissions-in-the-model.md §5: the "?"/"-"
// distinction ADR 0020 Amendment 3 §11 established applies to the new
// column, driven by the same flag.
//
// "-" says no permission is required. "?" says a requirement exists and
// was not recovered. An unread expression leaves the requirement unknown
// WITHOUT saying which kind of requirement it names, which is why one
// flag drives both cells.
func TestRenderPermissions_QuestionMarkDiscipline(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  Row
		want string
	}{
		{"nothing read, nothing found", Row{}, "-"},
		{"read as a permission", Row{Permissions: []string{"@ss.hasPermi('system:user:edit')"}}, "@ss.hasPermi('system:user:edit')"},
		{"two permissions", Row{Permissions: []string{"@el.check('user:list')", "@el.check('dept:list')"}}, "@el.check('user:list'), @el.check('dept:list')"},
		{"unread: the requirement exists and was not recovered", Row{RolesUnresolved: true}, "?"},
		{"partially read", Row{RolesUnresolved: true, Permissions: []string{"@ss.hasPermi('a')"}}, "@ss.hasPermi('a'), ?"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderPermissions(tc.row); got != tc.want {
				t.Errorf("renderPermissions() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestMarkdown_PermissionRowIsNotAllDashes is the rendered form of ADR
// 0035 §5's own argument for adding the column in the same change as the
// model term, taken from the real RuoYi-Vue row the PR shows:
//
//	PUT /system/user  @PreAuthorize("@ss.hasPermi('system:user:edit')")
//
// Guards is "-" because a role-declaring guard is surfaced under Roles,
// not Guards. Roles is "-" because the expression was read end to end and
// names no role. Without the Permissions cell that row reads "- | -" — a
// stronger and entirely false claim than the "?" it replaced. This test
// fails if the column is ever dropped while the model term stays.
func TestMarkdown_PermissionRowIsNotAllDashes(t *testing.T) {
	var b strings.Builder
	matrix := Matrix{Rows: []Row{{
		Controller:  "SysUserController",
		Method:      model.MethodPut,
		Path:        "/system/user",
		Handler:     "edit",
		Permissions: []string{"@ss.hasPermi('system:user:edit')"},
	}}}
	if err := writeMarkdown(&b, matrix); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "| Guards | Roles | Permissions | Findings |") {
		t.Errorf("the Permissions column is missing from the header:\n%s", out)
	}
	if !strings.Contains(out, "@ss.hasPermi('system:user:edit')") {
		t.Errorf("the permission is not rendered with the bean call that named it (ADR 0035 §3):\n%s", out)
	}
	if strings.Contains(out, "| - | - | - |") {
		t.Errorf("the row renders as all dashes, claiming nothing is required on an endpoint whose requirement was read:\n%s", out)
	}
}
