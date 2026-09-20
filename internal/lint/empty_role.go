package lint

import (
	"fmt"

	"github.com/chebilax/sphinxor/internal/model"
)

// EmptyRole flags a role-declaring construct (GuardApplication.DeclaresRoles)
// whose role list the source states as empty — e.g. NestJS's literal
// @Roles() called with no arguments, or Spring's @Secured({}) — a role
// check declared but requiring nothing, on an endpoint that has a
// role-list-bearing guard applied to it.
//
// Confidence: High, on a narrower trigger than this rule originally had.
// The original rationale was:
//
//	"whether a specific role-declaring construct resolved zero roles is a
//	syntactic fact, verifiable by reading that one location. There's no
//	global-guard or missed-reference scenario that could fool it."
//
// That was disproved, and the record matters more than the patch, because
// the same reasoning would justify the same mistake again. "Resolved zero
// roles" is a fact about this extractor's parser, not about the source.
// Reading that one location *disproves* the finding whenever the parser
// was the reason nothing resolved: @PreAuthorize("@ss.hasPermi('system:user:edit')")
// names its requirement in plain text, and this rule used to report it as
// declaring none. A survey of 14 production Spring repositories measured
// 675 such findings across four of them — at High confidence, which gates
// CI (docs/decisions/0020-unanalyzable-is-unknown-not-absent.md
// Amendment 3).
//
// The grade survives because the trigger no longer includes that case.
// GuardApplication.RolesUnresolved separates "the source declares an empty
// list" from "the list was not read", and this rule fires only on the
// former — for which the original claim does hold: reading that one
// location confirms it, and no global guard or unread indirection changes
// what an empty argument list says.
//
// permitAll()/denyAll() deliberately still fire here, per ADR 0017's
// boundary and Amendment 3 §10: they are read, not unread, and whether
// they should be flagged is a separate open question.
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
		// RolesUnresolved: the list was not read, so its emptiness is a
		// fact about extraction, not about the endpoint (Amendment 3 §9).
		if !g.DeclaresRoles || g.FromComposite || g.RolesUnresolved || refCount[g.ID] > 0 {
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
