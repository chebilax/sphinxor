// Package lint implements Sphinxor's v0.1 rule engine: the three minimal
// lint rules from docs/vision.md, applied to an extracted model.
package lint

import (
	"fmt"

	"github.com/chebilax/sphinxor/internal/model"
)

// Rule is a single lint rule.
type Rule interface {
	// ID identifies the rule, e.g. for allowlist reporting and output.
	ID() string
	// Check inspects m and returns every Finding it produces. Findings
	// need not set ID or Allowlisted — Run fills those in.
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

// Run applies every rule in rules to m, assigns each finding a stable ID,
// and marks a finding Allowlisted when its subject is an endpoint present
// in allowlistedEndpoints (docs/decisions/0003-allowlist-format.md).
func Run(m *model.Model, rules []Rule, allowlistedEndpoints map[model.ID]bool) []model.Finding {
	var findings []model.Finding
	for _, rule := range rules {
		n := 0
		for _, f := range rule.Check(m) {
			n++
			f.ID = model.ID(fmt.Sprintf("%s-%d", rule.ID(), n))
			if f.SubjectKind == model.SubjectEndpoint && allowlistedEndpoints[f.SubjectID] {
				f.Allowlisted = true
			}
			findings = append(findings, f)
		}
	}
	return findings
}
