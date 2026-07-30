package lint

import "github.com/chebilax/sphinxor/internal/model"

// MutatingEndpointWithoutAccessControl flags a mutating endpoint
// (POST/PUT/PATCH/DELETE) with no GuardApplication found protecting it.
type MutatingEndpointWithoutAccessControl struct{}

func (MutatingEndpointWithoutAccessControl) ID() string {
	return "mutating-endpoint-without-access-control"
}

// Not yet implemented.
func (MutatingEndpointWithoutAccessControl) Check(m *model.Model) []model.Finding {
	panic("lint: MutatingEndpointWithoutAccessControl.Check not implemented yet")
}
