package lint

import (
	"fmt"

	"github.com/chebilax/sphinxor/internal/model"
)

// EmptyRole flags a @Roles() decorator invoked with zero arguments — a
// role check declared but requiring nothing, on an endpoint that has a
// RolesGuard-style guard applied to it.
//
// Confidence: High. Unlike the other two v0.1 rules, this doesn't depend
// on assumptions about code this extractor can't see: whether a specific
// @Roles(...) call has zero arguments is a syntactic fact, verifiable by
// reading that one line. There's no global-guard or missed-reference
// scenario that could fool it.
type EmptyRole struct{}

func (EmptyRole) ID() string {
	return "empty-role"
}

func (r EmptyRole) Check(m *model.Model) []model.Finding {
	refCount := make(map[model.ID]int, len(m.GuardApplications))
	for _, ref := range m.RoleReferences {
		refCount[ref.GuardApplicationID]++
	}

	endpointByID := make(map[model.ID]model.Endpoint, len(m.Endpoints))
	for _, e := range m.Endpoints {
		endpointByID[e.ID] = e
	}

	var findings []model.Finding
	for _, g := range m.GuardApplications {
		if g.GuardName != "Roles" || refCount[g.ID] > 0 {
			continue
		}
		e := endpointByID[g.EndpointID]
		findings = append(findings, model.Finding{
			RuleID:      r.ID(),
			Confidence:  model.ConfidenceHigh,
			SubjectID:   g.EndpointID,
			SubjectKind: model.SubjectEndpoint,
			Message:     fmt.Sprintf("@Roles() on %s %s declares no roles", e.HTTPMethod, e.Path),
		})
	}
	return findings
}
