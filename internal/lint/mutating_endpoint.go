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
// An endpoint carrying an unrecognized authorization annotation
// (model.UnrecognizedAuthAnnotation, docs/decisions/0022-annotation-identity-and-unrecognized-authorization.md
// §3) is skipped rather than flagged — with one exception, @PostAuthorize,
// described below. This rule's message says
// the endpoint "has no detected guard or role decorator"; when an
// authorization annotation is sitting one line above the handler, that
// premise is false on its face, and on alibaba/nacos it would have been
// false 242 times against genuinely protected code. What such an endpoint
// carries is recorded in the model and named in a project-level warning
// instead — the same treatment as a global guard, where protection is
// known to exist and its requirement is not.
//
// The accepted cost: an annotation that is decorative — declared, never
// enforced — now goes unflagged. That trade is deliberate and lopsided in
// the right direction, a false negative on a rare case against a false
// positive on a common one (ADR 0022 §3).
//
// **@PostAuthorize is the exception, and it is verb-dependent**
// (docs/decisions/0030-post-authorize-and-method-security-filters.md §1/§2).
// Spring evaluates it AFTER the handler runs: AuthorizationManagerAfter-
// MethodInterceptor calls mi.proceed() and only then authorizes the value
// it returned. On a read that prevents disclosure, so the annotation
// genuinely protects the endpoint. On a POST/PUT/PATCH/DELETE the state
// change has already happened when AccessDeniedException is thrown, so
// nothing stopped the mutation. An enclosing transaction can roll it back,
// but only when @EnableTransactionManagement is ordered ahead of
// @EnableMethodSecurity — advisor ordering this extractor cannot see, and
// something Spring's own documentation presents as a deliberate
// arrangement rather than the default.
//
// So a @PostAuthorize does NOT suppress this finding on a mutating
// endpoint. It is still recorded, still marks the Guards column ?, and
// still omits the endpoint from the Cerbos export. Suppressing the finding
// too would assert that nothing needs looking at, on an endpoint whose
// mutation Spring does not stop.
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

// springPostAuthorize is the ONLY annotation this rule's carve-out
// applies to, matched on its binding rather than its simple name.
//
// Keying on the name would mean a project-local or third-party
// @PostAuthorize — one this extractor knows nothing about, and which may
// well run before the method — lost ADR 0022 §3's suppression and gained
// a message describing Spring's evaluation order. That is the nacos fault
// (ADR 0022 §1) reintroduced in the change meant to be careful about
// exactly this, and a test caught it.
const springPostAuthorize = "org.springframework.security.access.prepost.PostAuthorize"

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

	// ADR 0022 §3: an endpoint with an access-control annotation this
	// extractor could not identify is not in the state this rule
	// describes. ADR 0030 §2 carves out @PostAuthorize, which authorizes
	// too late to stop a mutation, so it is tracked separately rather
	// than joining the suppression set.
	unidentified := make(map[model.ID]bool, len(m.UnrecognizedAuthAnnotations))
	postAuthorize := make(map[model.ID]bool)
	for _, a := range m.UnrecognizedAuthAnnotations {
		if a.BoundTo == springPostAuthorize {
			postAuthorize[a.EndpointID] = true
			continue
		}
		unidentified[a.EndpointID] = true
	}

	var findings []model.Finding
	for _, e := range m.Endpoints {
		if !isMutating(e.HTTPMethod) || guarded[e.ID] || unidentified[e.ID] {
			continue
		}
		findings = append(findings, model.Finding{
			RuleID:      r.ID(),
			Confidence:  model.ConfidenceLow,
			SubjectID:   e.ID,
			SubjectKind: model.SubjectEndpoint,
			Message:     mutatingMessage(e, postAuthorize[e.ID], m.MethodSecurity),
		})
	}
	return findings
}

// mutatingMessage explains why the finding fired. The default premise —
// "no detected guard" — is visibly false to a reader looking at a handler
// with @PostAuthorize one line above it, and ADR 0022 §3 established that
// a message whose premise the reader can see is wrong discredits the rule.
//
// ADR 0030 §3: when the annotation is confirmed inert, the endpoint gets
// the ordinary message instead. Nothing evaluates the annotation at all,
// so describing WHEN it evaluates would be the false statement ADR 0015
// Amendment 1 was written to remove.
func mutatingMessage(e model.Endpoint, hasPostAuthorize bool, status model.MethodSecurityStatus) string {
	if hasPostAuthorize && !(status.Found && !status.PrePostEnabled) {
		return fmt.Sprintf(
			"%s %s has a @PostAuthorize but no guard that runs before the method. "+
				"@PostAuthorize is evaluated after the handler executes, so the state change has "+
				"already happened when access is denied — unless an enclosing transaction rolls it "+
				"back, which is not visible here.",
			e.HTTPMethod, e.Path)
	}
	return fmt.Sprintf("%s %s has no detected guard or role decorator", e.HTTPMethod, e.Path)
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
	case model.MethodPost, model.MethodPut, model.MethodPatch, model.MethodDelete,
		// A handler mapping every verb accepts POST, PUT, PATCH and
		// DELETE, so a rule about state change must say so
		// (docs/decisions/0028-verbless-request-mapping.md §2).
		model.MethodAny:
		return true
	default:
		return false
	}
}
