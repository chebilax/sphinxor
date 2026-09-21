package spring

import (
	"github.com/chebilax/sphinxor/internal/model"
)

// methodSecurityAnnotations are the recognized method-security annotation
// names, per docs/decisions/0011-spring-second-framework.md §1.
//
// A name here is necessary but NOT sufficient: ADR 0022 §1 requires the
// file's imports to bind the name to a package that makes it the real
// Spring (or JSR-250) annotation. Matching on the name alone treated
// alibaba/nacos's own @Secured as Spring method security, 392 times. See
// acceptedAnnotationPackages in imports.go for the bindings.
var methodSecurityAnnotations = map[string]bool{
	"PreAuthorize": true,
	"Secured":      true,
	"RolesAllowed": true,
}

// pendingGuard is one recognized method-security annotation's outcome, not
// yet tied to an Endpoint — a class-level annotation applies to every
// endpoint in the controller, discovered only once methods are walked
// (mirrors internal/extract/nestjs/controllers.go's pendingGuard).
type pendingGuard struct {
	guardName string // "PreAuthorize" | "Secured" | "RolesAllowed"
	roles     []roleArg
	// declaresRoles is false only for @PreAuthorize("isAuthenticated()") —
	// docs/decisions/0017-declaresroles-excludes-isauthenticated.md. True
	// for every other recognized shape, including permitAll()/denyAll()/
	// unrecognized SpEL, per that ADR's stated boundary.
	declaresRoles bool
	// authCandidate is true only for @PreAuthorize("isAuthenticated()") —
	// the signal authentication.go's final pass consumes to decide
	// whether this endpoint gets an AuthenticationRequirement.
	authCandidate bool
	// rolesUnresolved is true when this annotation declares a role
	// requirement whose list could not be read — docs/decisions/0020-unanalyzable-is-unknown-not-absent.md
	// Amendment 3 §9. It is never true together with a non-empty roles
	// slice: either the list was read, or it was not.
	rolesUnresolved bool
	file            string
	line            int
}

// pendingUnrecognized is one annotation that carried a recognized name
// without a binding that makes it Spring's — ADR 0022 §2. It is kept
// apart from pendingGuard all the way through so that no code path can
// turn it into a GuardApplication by accident.
type pendingUnrecognized struct {
	name    string
	boundTo string
	file    string
	line    int
}

type roleArg struct {
	raw    string
	declID *model.ID
}

// pendingGuardsFromAnnotations builds one pendingGuard per recognized
// method-security annotation found in anns, and one pendingUnrecognized
// per annotation whose name is recognized but whose import binding is not
// (ADR 0022 §1/§2).
//
// The two are returned separately rather than as one list with a flag, so
// that a caller cannot treat an unidentified annotation as a guard by
// forgetting to check a field — the failure mode ADR 0011 §1 documented
// and ADR 0022 §2 refuses to repeat.
func pendingGuardsFromAnnotations(anns []annotationCall, src []byte, file string, roleByName map[string]model.ID, imports importTable) ([]pendingGuard, []pendingUnrecognized) {
	var out []pendingGuard
	var unknown []pendingUnrecognized
	for _, ann := range anns {
		line := int(ann.Node.StartPoint().Row) + 1

		// ADR 0025 §3: an annotation written fully qualified carries its
		// own binding, and a stronger one than an import. Answered here,
		// before the import-based paths below, because those would find
		// no import for it and misread Spring's own annotation as
		// foreign.
		if ann.Qualifier != "" {
			if !methodSecurityAnnotations[ann.Name] && !isThirdPartyQualifier(ann.Qualifier) {
				continue
			}
			boundTo, kind := resolveQualifiedAuth(ann.Name, ann.Qualifier)
			if kind != "spring" {
				unknown = append(unknown, pendingUnrecognized{name: ann.Name, boundTo: boundTo, file: file, line: line})
				continue
			}
			out = append(out, springGuard(ann, src, file, line, roleByName)...)
			continue
		}

		// ADR 0023 §1: an annotation from a third-party authorization
		// framework. Recognized by the package its import binds it to,
		// never by its name — Shiro's @RequiresPermissions shares no
		// name with anything Spring uses, so it would otherwise fall
		// straight through to "no access control found", which ADR 0022
		// §3 established is the wrong thing to report about an endpoint
		// that has some.
		if boundTo, _, isThirdParty := imports.resolveThirdPartyAuth(ann.Name); isThirdParty {
			unknown = append(unknown, pendingUnrecognized{name: ann.Name, boundTo: boundTo, file: file, line: line})
			continue
		}

		if !methodSecurityAnnotations[ann.Name] {
			continue
		}

		// ADR 0022 §1: the name is not the annotation. Without a binding
		// to an accepted package this is someone else's annotation that
		// happens to share a word, and recording it as a Spring guard
		// would assert protection this extractor never established.
		boundTo, accepted := imports.resolveAnnotation(ann.Name)
		if !accepted {
			unknown = append(unknown, pendingUnrecognized{name: ann.Name, boundTo: boundTo, file: file, line: line})
			continue
		}

		out = append(out, springGuard(ann, src, file, line, roleByName)...)
	}
	return out, unknown
}

// isThirdPartyQualifier reports whether a fully-qualified annotation's
// package is a known third-party authorization package (ADR 0023 §1).
func isThirdPartyQualifier(qualifier string) bool {
	_, known := thirdPartyAuthPackages[qualifier]
	return known
}

// springGuard builds the pendingGuard(s) for an annotation already
// established to be Spring's own method security — whether that was
// established by an import (ADR 0022 §1) or by a fully-qualified use
// (ADR 0025 §3). Both paths must produce identical results, which is why
// this is one function rather than two.
func springGuard(ann annotationCall, src []byte, file string, line int, roleByName map[string]model.ID) []pendingGuard {
	if ann.Name == "PreAuthorize" {
		lit, ok := stringLiteralValue(soleStringLiteralArg(ann.Args), src)
		if !ok {
			// Not the recognized single-string shape at all (e.g. a
			// SpEL expression built from a constant reference rather
			// than a literal) — still a real guard, but its role
			// list was not read, so it is unknown rather than empty
			// (ADR 0020 Amendment 3 §9).
			return []pendingGuard{{guardName: ann.Name, declaresRoles: true, rolesUnresolved: true, file: file, line: line}}
		}
		result := parseSpEL(lit)
		switch result.Kind {
		case spelAuthenticated:
			return []pendingGuard{{guardName: ann.Name, declaresRoles: false, authCandidate: true, file: file, line: line}}
		case spelRoles:
			return []pendingGuard{{guardName: ann.Name, roles: resolveRoleArgs(result.Roles, roleByName), declaresRoles: true, file: file, line: line}}
		case spelNoRole:
			// permitAll()/denyAll(): read, and resolving to no role
			// list. ADR 0017 decided these keep DeclaresRoles: true and
			// keep surfacing through empty-role; ADR 0020 Amendment 3
			// §10 preserves that deliberately rather than reversing it
			// as a side effect, so rolesUnresolved stays false.
			return []pendingGuard{{guardName: ann.Name, declaresRoles: true, file: file, line: line}}
		default: // spelUnrecognized: a bean call, a boolean combination, ...
			return []pendingGuard{{guardName: ann.Name, declaresRoles: true, rolesUnresolved: true, file: file, line: line}}
		}
	}

	// Secured / RolesAllowed: plain string-array arguments, no SpEL.
	// resolved distinguishes @Secured({}) — genuinely empty — from an
	// argument shape that was never read, such as the named attributes on
	// alibaba's same-named @Secured (ADR 0020 Amendment 3 §9).
	literals, resolved := stringArrayValues(ann.Args, src)
	return []pendingGuard{{
		guardName:       ann.Name,
		roles:           resolveRoleArgs(literals, roleByName),
		declaresRoles:   true,
		rolesUnresolved: !resolved,
		file:            file,
		line:            line,
	}}
}

// applyUnrecognized materializes unidentified access-control annotations
// against endpointID (ADR 0022 §2). Nothing here touches
// GuardApplications: these are evidence that something protects the
// endpoint, not a claim about what.
func (b *builder) applyUnrecognized(endpointID model.ID, anns []pendingUnrecognized, scope model.GuardScope) {
	for _, a := range anns {
		b.model.UnrecognizedAuthAnnotations = append(b.model.UnrecognizedAuthAnnotations, model.UnrecognizedAuthAnnotation{
			ID:         b.nextIDFor("unrecognizedauth"),
			EndpointID: endpointID,
			Name:       a.name,
			BoundTo:    a.boundTo,
			AppliedAt:  scope,
			File:       a.file,
			Line:       a.line,
		})
	}
}

func resolveRoleArgs(literals []string, roleByName map[string]model.ID) []roleArg {
	out := make([]roleArg, len(literals))
	for i, lit := range literals {
		out[i] = roleArg{raw: lit}
		if id, ok := roleByName[lit]; ok {
			idCopy := id
			out[i].declID = &idCopy
		}
	}
	return out
}

// applyGuards materializes guards (already scoped to either the endpoint's
// controller or its own handler method) as GuardApplications and
// RoleReferences on endpointID, and records an authCandidate for
// authentication.go's final pass whenever a guard's SpEL was
// isAuthenticated().
func (b *builder) applyGuards(endpointID model.ID, guards []pendingGuard, scope model.GuardScope) {
	for _, g := range guards {
		appID := b.nextIDFor("guardapp")
		b.guardOwner = append(b.guardOwner, b.curEndpoint)
		b.model.GuardApplications = append(b.model.GuardApplications, model.GuardApplication{
			ID:              appID,
			EndpointID:      endpointID,
			GuardName:       g.guardName,
			AppliedAt:       scope,
			File:            g.file,
			Line:            g.line,
			DeclaresRoles:   g.declaresRoles,
			RolesUnresolved: g.rolesUnresolved,
		})
		for _, r := range g.roles {
			b.model.RoleReferences = append(b.model.RoleReferences, model.RoleReference{
				ID:                 b.nextIDFor("roleref"),
				GuardApplicationID: appID,
				RoleDeclarationID:  r.declID,
				RawLiteral:         r.raw,
				File:               g.file,
				Line:               g.line,
			})
		}
		if g.authCandidate {
			b.authCandidates = append(b.authCandidates, authCandidate{
				EndpointID: endpointID,
				File:       g.file,
				Line:       g.line,
				AppliedAt:  scope,
			})
		}
	}
}
