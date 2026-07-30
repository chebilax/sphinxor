package lint

import "github.com/chebilax/sphinxor/internal/model"

// PermissionDeclaredButUnreferenced flags a RoleDeclaration with no
// RoleReference pointing at it anywhere in the model — a role that's
// declared (e.g. in an enum) but never required by any guard.
type PermissionDeclaredButUnreferenced struct{}

func (PermissionDeclaredButUnreferenced) ID() string {
	return "permission-declared-but-unreferenced"
}

// Not yet implemented.
func (PermissionDeclaredButUnreferenced) Check(m *model.Model) []model.Finding {
	panic("lint: PermissionDeclaredButUnreferenced.Check not implemented yet")
}
