package spring

import "github.com/chebilax/sphinxor/internal/model"

// authCandidate is one @PreAuthorize("isAuthenticated()") occurrence,
// recorded at parse time (guards.go). Spring's recognized-authentication
// signal depends on annotation *content* (docs/decisions/0011-spring-second-framework.md §2),
// unlike NestJS's guard-*name*-based signal
// (internal/extract/nestjs/authentication.go), so it can't be re-derived
// from the assembled model afterward the way NestJS's final pass does —
// it has to be captured here and carried through to
// computeAuthenticationRequirements below.
type authCandidate struct {
	EndpointID model.ID
	File       string
	Line       int
	AppliedAt  model.GuardScope
}

// computeAuthenticationRequirements mirrors
// internal/extract/nestjs/authentication.go's function of the same name
// and purpose (ADR 0010): an AuthenticationRequirement is created only for
// an endpoint with at least one recognized-authentication candidate AND
// zero resolved RoleReferences anywhere on that endpoint —
// isAuthenticated() only wins when nothing stricter was also found on the
// same endpoint, the same rule NestJS applies, evaluated against Spring's
// own candidate list instead of a recognized-guard-name lookup.
//
// One candidate per endpoint at most: a class-level and a method-level
// isAuthenticated() on the same endpoint would otherwise produce two
// AuthenticationRequirements for one fact.
func computeAuthenticationRequirements(m *model.Model, candidates []authCandidate, next func() model.ID) []model.AuthenticationRequirement {
	roleRefCountByGuard := make(map[model.ID]int, len(m.RoleReferences))
	for _, r := range m.RoleReferences {
		roleRefCountByGuard[r.GuardApplicationID]++
	}
	roleRefCountByEndpoint := make(map[model.ID]int, len(m.GuardApplications))
	for _, g := range m.GuardApplications {
		roleRefCountByEndpoint[g.EndpointID] += roleRefCountByGuard[g.ID]
	}

	seen := make(map[model.ID]bool, len(candidates))
	var out []model.AuthenticationRequirement
	for _, c := range candidates {
		if seen[c.EndpointID] || roleRefCountByEndpoint[c.EndpointID] > 0 {
			continue
		}
		seen[c.EndpointID] = true
		out = append(out, model.AuthenticationRequirement{
			ID:         next(),
			EndpointID: c.EndpointID,
			File:       c.File,
			Line:       c.Line,
			AppliedAt:  c.AppliedAt,
		})
	}
	return out
}
