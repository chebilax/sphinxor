package lint

import (
	"fmt"

	"github.com/chebilax/sphinxor/internal/model"
)

// EmptyRole flags a role-declaring construct (GuardApplication.DeclaresRoles)
// invoked with zero roles — e.g. NestJS's literal @Roles() called with no
// arguments — a role check declared but requiring nothing, on an endpoint
// that has a role-list-bearing guard applied to it.
//
// Confidence: High. Unlike the other two v0.1 rules, this doesn't depend
// on assumptions about code this extractor can't see: whether a specific
// role-declaring construct resolved zero roles is a syntactic fact,
// verifiable by reading that one location. There's no global-guard or
// missed-reference scenario that could fool it.
//
// Composite-resolved applications (GuardApplication.FromComposite,
// docs/decisions/0006) are deliberately excluded: a composite decorator
// commonly defaults its roles parameter to an empty list
// (e.g. Auth(roles: RoleType[] = [])), meaning an empty resolved role set
// is that composite's documented "authenticated, no specific role
// required" behavior, not the forgotten-argument smell this rule targets.
// Telling those two cases apart would require modeling the composite's
// own default-parameter semantics — exactly the dataflow ADR 0006 scoped
// out — so this rule simply doesn't have an opinion about them.
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
		if !g.DeclaresRoles || g.FromComposite || refCount[g.ID] > 0 {
			continue
		}
		e := endpointByID[g.EndpointID]
		findings = append(findings, model.Finding{
			RuleID:      r.ID(),
			Confidence:  model.ConfidenceHigh,
			SubjectID:   g.EndpointID,
			SubjectKind: model.SubjectEndpoint,
			Message:     fmt.Sprintf("@%s() on %s %s declares no roles", g.GuardName, e.HTTPMethod, e.Path),
		})
	}
	return findings
}
