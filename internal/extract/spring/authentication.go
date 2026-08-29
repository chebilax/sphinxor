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
// an endpoint-and-layer with at least one recognized-authentication
// candidate AND zero resolved RoleReferences anywhere *on that same
// layer* — isAuthenticated() only wins when nothing stricter was also
// found on the same layer.
//
// Per-layer, not per-endpoint-globally, per ADR 0011 §3: once an endpoint
// can have independent method- and URL-layer facts, a global "zero role
// refs anywhere on this endpoint" check would wrongly suppress a real,
// independently-true method-layer isAuthenticated() just because the URL
// layer happened to name a concrete role elsewhere (or vice versa) — the
// two layers are reconciled later, by internal/export/cerbos's
// intersection (ADR 0012 §2), not pre-suppressed here. This was
// equivalent to a global, per-endpoint check in PR 2 (only one layer
// existed then); now that the URL layer exists (PR 3), it isn't, and this
// function is corrected accordingly rather than left silently wrong.
//
// At most one AuthenticationRequirement per endpoint-and-layer: two
// isAuthenticated() candidates on the same layer of the same endpoint
// (e.g. a redundant class- and method-level pair, both layerMethod)
// would otherwise produce two AuthenticationRequirements for one fact.
func computeAuthenticationRequirements(m *model.Model, candidates []authCandidate, next func() model.ID) []model.AuthenticationRequirement {
	roleRefCountByGuard := make(map[model.ID]int, len(m.RoleReferences))
	for _, r := range m.RoleReferences {
		roleRefCountByGuard[r.GuardApplicationID]++
	}
	type endpointLayer struct {
		endpoint model.ID
		url      bool
	}
	roleRefCountByLayer := make(map[endpointLayer]int, len(m.GuardApplications))
	for _, g := range m.GuardApplications {
		k := endpointLayer{endpoint: g.EndpointID, url: g.AppliedAt == model.ScopeRequestMatcher}
		roleRefCountByLayer[k] += roleRefCountByGuard[g.ID]
	}

	seen := make(map[endpointLayer]bool, len(candidates))
	var out []model.AuthenticationRequirement
	for _, c := range candidates {
		k := endpointLayer{endpoint: c.EndpointID, url: c.AppliedAt == model.ScopeRequestMatcher}
		if seen[k] || roleRefCountByLayer[k] > 0 {
			continue
		}
		seen[k] = true
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
