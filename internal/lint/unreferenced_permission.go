package lint

import (
	"fmt"

	"github.com/chebilax/sphinxor/internal/model"
)

// PermissionDeclaredButUnreferenced flags a RoleDeclaration with no
// RoleReference pointing at it anywhere in the model — a role that's
// declared (e.g. as an enum member) but never required by any @Roles()
// decorator this extractor found.
//
// Confidence: Low. "Never referenced" here means "never referenced via a
// @Roles() decorator we recognized." A role checked directly in service
// logic (e.g. `if (user.role === Role.Admin)`), or referenced via a raw
// string literal that happens to match its value rather than the enum
// symbol (see internal/extract/nestjs's role-resolution limitation), is
// invisible to this rule. A flagged role may genuinely be dead, or may
// just be used somewhere this extractor doesn't look.
type PermissionDeclaredButUnreferenced struct{}

func (PermissionDeclaredButUnreferenced) ID() string {
	return "permission-declared-but-unreferenced"
}

func (r PermissionDeclaredButUnreferenced) Check(m *model.Model) []model.Finding {
	referenced := make(map[model.ID]bool, len(m.RoleReferences))
	for _, ref := range m.RoleReferences {
		if ref.RoleDeclarationID != nil {
			referenced[*ref.RoleDeclarationID] = true
		}
	}

	var findings []model.Finding
	for _, d := range m.RoleDeclarations {
		if referenced[d.ID] {
			continue
		}
		findings = append(findings, model.Finding{
			RuleID:      r.ID(),
			Confidence:  model.ConfidenceLow,
			SubjectID:   d.ID,
			SubjectKind: model.SubjectRoleDeclaration,
			Message:     fmt.Sprintf("role %q is declared but never referenced by a @Roles() decorator", d.Name),
		})
	}
	return findings
}
