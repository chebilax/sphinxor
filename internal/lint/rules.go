// Package lint implements Sphinxor's v0.1 rule engine: the three minimal
// lint rules from docs/vision.md, applied to an extracted model.
package lint

import "github.com/chebilax/sphinxor/internal/model"

// Rule is a single lint rule.
type Rule interface {
	// ID identifies the rule, e.g. for allowlist reporting and output.
	ID() string
	// Check inspects m and returns every Finding it produces.
	Check(m *model.Model) []model.Finding
}

// DefaultRules returns the v0.1 rule set described in docs/vision.md.
func DefaultRules() []Rule {
	return []Rule{
		MutatingEndpointWithoutAccessControl{},
		PermissionDeclaredButUnreferenced{},
		EmptyRole{},
	}
}

// Run applies every rule in rules to m and returns the combined findings.
//
// Not yet implemented — allowlist matching (marking findings as
// Allowlisted) also happens here, once the rules themselves exist.
func Run(m *model.Model, rules []Rule) []model.Finding {
	panic("lint: Run not implemented yet")
}
