package nestjs

import "github.com/chebilax/sphinxor/internal/model"

// recognizedAuthenticationGuards are the NestJS guard class names
// extraction treats as positively establishing authentication, per
// docs/decisions/0010-authenticated-any-role.md. A guard name not in this
// set never produces an AuthenticationRequirement, no matter how
// auth-guard-like its name looks — recognizing "any guard" as
// authentication would silently upgrade an unrelated guard (a rate
// limiter, a feature flag, ...) into a granted "authenticated, any role"
// requirement it never earned.
//
// Deliberately small and NestJS-specific — confirmed against real
// per-endpoint GuardName data from both vendored repos (ADR 0010), not
// picked speculatively. Grows only from the same kind of evidence, the
// same discipline extractRoleDeclarations already applies to which enums
// count as role declarations.
//
// This list is intentionally framework-scoped and lives in this package,
// not in internal/model: a future second framework (e.g. Spring) has its
// own authentication conventions, not "AuthGuard", and will define its
// own recognized set in its own extractor package rather than extending
// this one.
var recognizedAuthenticationGuards = map[string]bool{
	"AuthGuard": true,
}

// computeAuthenticationRequirements derives every AuthenticationRequirement
// in m: one per Endpoint that has at least one recognized authentication
// guard (recognizedAuthenticationGuards) and zero resolved RoleReferences
// across all of its guards, excluding the one case EmptyRole already
// flags as a probable mistake — a literal @Roles() call with zero
// arguments (GuardName "Roles", not FromComposite, zero RoleReferences).
// That exclusion mirrors EmptyRole's own check (internal/lint/empty_role.go)
// exactly, for the same reason: exporting a grant for code already flagged
// elsewhere as likely wrong would be a second guess-in-the-permissive-
// direction failure, one layer upstream (ADR 0010).
//
// Must run after every GuardApplication and RoleReference for the project
// is known — an endpoint's guards and role references can come from both
// class-level and method-level decorators, resolved across multiple
// passes, so this is a final pass over the assembled model rather than
// something computed incrementally per file.
func computeAuthenticationRequirements(m *model.Model, next func() model.ID) []model.AuthenticationRequirement {
	guardsByEndpoint := make(map[model.ID][]model.GuardApplication, len(m.Endpoints))
	for _, g := range m.GuardApplications {
		guardsByEndpoint[g.EndpointID] = append(guardsByEndpoint[g.EndpointID], g)
	}

	roleRefCountByGuard := make(map[model.ID]int, len(m.GuardApplications))
	for _, r := range m.RoleReferences {
		roleRefCountByGuard[r.GuardApplicationID]++
	}

	var out []model.AuthenticationRequirement
	for _, e := range m.Endpoints {
		var recognized *model.GuardApplication
		totalRoleRefs := 0
		literalEmptyRoles := false

		for _, g := range guardsByEndpoint[e.ID] {
			g := g
			totalRoleRefs += roleRefCountByGuard[g.ID]
			if recognizedAuthenticationGuards[g.GuardName] && recognized == nil {
				recognized = &g
			}
			if g.GuardName == "Roles" && !g.FromComposite && roleRefCountByGuard[g.ID] == 0 {
				literalEmptyRoles = true
			}
		}

		if recognized == nil || totalRoleRefs > 0 || literalEmptyRoles {
			continue
		}

		out = append(out, model.AuthenticationRequirement{
			ID:         next(),
			EndpointID: e.ID,
			File:       recognized.File,
			Line:       recognized.Line,
		})
	}
	return out
}
