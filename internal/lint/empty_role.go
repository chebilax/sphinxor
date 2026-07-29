package lint

import "github.com/chebilax/sphynxor/internal/model"

// EmptyRole flags a role with no real content — e.g. a guard application
// whose role reference resolves to no permissions, or a role declaration
// with no members. The exact condition this checks is pinned down during
// implementation, once real NestJS fixtures show what "empty" actually
// looks like in practice — vision.md names the rule but not its precise
// trigger, and inventing that precision ahead of real examples risks
// encoding a wrong assumption.
type EmptyRole struct{}

func (EmptyRole) ID() string {
	return "empty-role"
}

// Not yet implemented.
func (EmptyRole) Check(m *model.Model) []model.Finding {
	panic("lint: EmptyRole.Check not implemented yet")
}
