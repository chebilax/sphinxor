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
			{ID: "g2", EndpointID: "e1", GuardName: "Roles"},
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
