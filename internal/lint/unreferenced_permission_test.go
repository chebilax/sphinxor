package lint

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

func TestPermissionDeclaredButUnreferenced(t *testing.T) {
	declUser := model.ID("role-user")
	m := &model.Model{
		RoleDeclarations: []model.RoleDeclaration{
			{ID: "role-admin", Name: "RoleEnum.admin"},
			{ID: declUser, Name: "RoleEnum.user"},
		},
		RoleReferences: []model.RoleReference{
			{ID: "ref1", RoleDeclarationID: idPtr("role-admin")},
		},
	}

	findings := PermissionDeclaredButUnreferenced{}.Check(m)

	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.SubjectID != declUser {
		t.Errorf("subject = %q, want %q", f.SubjectID, declUser)
	}
	if f.Confidence != model.ConfidenceLow {
		t.Errorf("confidence = %q, want low", f.Confidence)
	}
	if f.SubjectKind != model.SubjectRoleDeclaration {
		t.Errorf("subject kind = %q, want role_declaration", f.SubjectKind)
	}
}

func TestPermissionDeclaredButUnreferenced_NilDeclarationIDIgnored(t *testing.T) {
	m := &model.Model{
		RoleDeclarations: []model.RoleDeclaration{
			{ID: "role-admin", Name: "RoleEnum.admin"},
		},
		RoleReferences: []model.RoleReference{
			{ID: "ref1", RoleDeclarationID: nil, RawLiteral: "admin"},
		},
	}

	findings := PermissionDeclaredButUnreferenced{}.Check(m)

	if len(findings) != 1 {
		t.Fatalf("an unresolved reference (nil RoleDeclarationID) must not count as referencing role-admin; got %d findings", len(findings))
	}
}

func idPtr(id model.ID) *model.ID {
	return &id
}
