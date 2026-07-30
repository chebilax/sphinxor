package lint

import (
	"fmt"

	"github.com/chebilax/sphinxor/internal/model"
)

// MutatingEndpointWithoutAccessControl flags a mutating endpoint
// (POST/PUT/PATCH/DELETE) with no GuardApplication found protecting it —
// no @UseGuards() and no @Roles() at either the method or controller
// level.
//
// Confidence: Low. This rule only sees guards declared via decorators on
// the endpoint or its controller. A global guard (APP_GUARD provider,
// app.useGlobalGuards()) or AOP-style interceptor can protect an endpoint
// invisibly to this syntactic analysis — Sphinxor does not parse module
// provider wiring in v0.1 (see the PR's known blind spots). An endpoint
// flagged here may genuinely be unguarded, or may be protected by
// something outside this rule's field of view; either way, it's worth a
// human look, not an automatic failure.
type MutatingEndpointWithoutAccessControl struct{}

func (MutatingEndpointWithoutAccessControl) ID() string {
	return "mutating-endpoint-without-access-control"
}

func (r MutatingEndpointWithoutAccessControl) Check(m *model.Model) []model.Finding {
	guarded := make(map[model.ID]bool, len(m.GuardApplications))
	for _, g := range m.GuardApplications {
		guarded[g.EndpointID] = true
	}

	var findings []model.Finding
	for _, e := range m.Endpoints {
		if !isMutating(e.HTTPMethod) || guarded[e.ID] {
			continue
		}
		findings = append(findings, model.Finding{
			RuleID:      r.ID(),
			Confidence:  model.ConfidenceLow,
			SubjectID:   e.ID,
			SubjectKind: model.SubjectEndpoint,
			Message:     fmt.Sprintf("%s %s has no detected guard or role decorator", e.HTTPMethod, e.Path),
		})
	}
	return findings
}

func isMutating(m model.HTTPMethod) bool {
	switch m {
	case model.MethodPost, model.MethodPut, model.MethodPatch, model.MethodDelete:
		return true
	default:
		return false
	}
}
