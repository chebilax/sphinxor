package spring

import (
	"github.com/chebilax/sphinxor/internal/model"
)

// methodSecurityAnnotations are the recognized method-security annotation
// names, per docs/decisions/0011-spring-second-framework.md §1.
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
	file          string
	line          int
}

type roleArg struct {
	raw    string
	declID *model.ID
}

// pendingGuardsFromAnnotations builds one pendingGuard per recognized
// method-security annotation found in anns.
func pendingGuardsFromAnnotations(anns []annotationCall, src []byte, file string, roleByName map[string]model.ID) []pendingGuard {
	var out []pendingGuard
	for _, ann := range anns {
		if !methodSecurityAnnotations[ann.Name] {
			continue
		}
		line := int(ann.Node.StartPoint().Row) + 1

		if ann.Name == "PreAuthorize" {
			lit, ok := stringLiteralValue(soleStringLiteralArg(ann.Args), src)
			if !ok {
				// Not the recognized single-string shape at all (e.g. a
				// SpEL expression built from a constant reference rather
				// than a literal) — still a real guard, just with
				// nothing further to recognize; declaresRoles stays true,
				// the same "guarded, no role" default as unrecognized
				// SpEL content.
				out = append(out, pendingGuard{guardName: ann.Name, declaresRoles: true, file: file, line: line})
				continue
			}
			result := parseSpEL(lit)
			switch result.Kind {
			case spelAuthenticated:
				out = append(out, pendingGuard{guardName: ann.Name, declaresRoles: false, authCandidate: true, file: file, line: line})
			case spelRoles:
				out = append(out, pendingGuard{guardName: ann.Name, roles: resolveRoleArgs(result.Roles, roleByName), declaresRoles: true, file: file, line: line})
			default: // spelUnrecognized, including permitAll()/denyAll()
				out = append(out, pendingGuard{guardName: ann.Name, declaresRoles: true, file: file, line: line})
			}
			continue
		}

		// Secured / RolesAllowed: plain string-array arguments, no SpEL.
		literals := stringArrayValues(ann.Args, src)
		out = append(out, pendingGuard{guardName: ann.Name, roles: resolveRoleArgs(literals, roleByName), declaresRoles: true, file: file, line: line})
	}
	return out
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
		b.model.GuardApplications = append(b.model.GuardApplications, model.GuardApplication{
			ID:            appID,
			EndpointID:    endpointID,
			GuardName:     g.guardName,
			AppliedAt:     scope,
			File:          g.file,
			Line:          g.line,
			DeclaresRoles: g.declaresRoles,
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
