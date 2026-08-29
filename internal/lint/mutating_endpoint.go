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
// Confidence: Low. This rule sees literal @UseGuards()/@Roles() decorator
// call sites, plus one level of composite-decorator resolution (ADR 0006:
// a decorator built with applyDecorators(), resolved when it matches a
// bounded, stated shape). A global guard (APP_GUARD provider,
// app.useGlobalGuards()), an AOP-style interceptor, a multi-level
// composite chain, or a composite outside ADR 0006's resolved shape
// (conditional construction, destructured parameters, transformed
// arguments — all confirmed on real code, not hypothetical) can still
// protect an endpoint invisibly to this syntactic analysis. An endpoint
// flagged here may genuinely be unguarded, or may be protected by
// something outside this rule's field of view; either way, it's worth a
// human look, not an automatic failure. See docs/limitations.md for the
// full, current list of what this rule cannot see.
//
// A GuardApplication whose annotation family is *confirmed* not enabled
// project-wide (docs/decisions/0015-inert-method-security-guard.md, e.g. a
// Spring @Secured method with no securedEnabled = true anywhere) does not
// count as guarding its endpoint here — it's real source, but inert at
// runtime, and treating it as protection would be the exact
// false-confidence failure this rule exists to avoid. Absence of evidence
// (m.MethodSecurity.Found == false) is never treated as confirmed-inert —
// only Spring's own documented defaults, positively located, downgrade a
// guard this way.
type MutatingEndpointWithoutAccessControl struct{}

func (MutatingEndpointWithoutAccessControl) ID() string {
	return "mutating-endpoint-without-access-control"
}

func (r MutatingEndpointWithoutAccessControl) Check(m *model.Model) []model.Finding {
	guarded := make(map[model.ID]bool, len(m.GuardApplications))
	for _, g := range m.GuardApplications {
		if !isConfirmedInert(g, m.MethodSecurity) {
			guarded[g.EndpointID] = true
		}
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

// isConfirmedInert reports whether g's annotation family is positively
// known, project-wide, not to be enabled — docs/decisions/0015-inert-method-security-guard.md.
// status.Found == false means "no evidence either way," never "confirmed
// disabled": a base class, a parent module, or unparsed Kotlin config
// (a stated blind spot, ADR 0011) could enable it outside what was
// scanned, so absence of evidence never downgrades a guard here.
func isConfirmedInert(g model.GuardApplication, status model.MethodSecurityStatus) bool {
	if !status.Found {
		return false
	}
	switch g.GuardName {
	case "PreAuthorize", "PostAuthorize":
		return !status.PrePostEnabled
	case "Secured":
		return !status.SecuredEnabled
	case "RolesAllowed":
		return !status.Jsr250Enabled
	default:
		return false // NestJS guards, or anything not method-security-gated
	}
}

func isMutating(m model.HTTPMethod) bool {
	switch m {
	case model.MethodPost, model.MethodPut, model.MethodPatch, model.MethodDelete:
		return true
	default:
		return false
	}
}
