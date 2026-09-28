package cerbos

import (
	"strings"

	"github.com/chebilax/sphinxor/internal/model"
)

// permissionGrants turns each declared bean call into a grant on its
// endpoint's layer, and a condition where one is needed —
// docs/decisions/0041-permission-export.md §4. An endpoint it cannot
// export faithfully gets an entry in omit instead, and no grant.
//
// Only a declaration makes anything exportable. An endpoint the corpus
// gave no evidence for — several permission-bearing annotations, or
// permissions beside role annotations — is omitted rather than combined
// by a rule nobody decided: exporting its roles alone would drop the
// permission requirement, which is an over-grant.
func permissionGrants(m *model.Model, d Declarations, guardAppByID map[model.ID]model.GuardApplication,
	layerGrants func(model.ID, layer) *[]roleGrant, condition map[model.ID]*Condition, omit map[model.ID]Omission) {

	refsByGuard := map[model.ID][]model.PermissionReference{}
	var guardOrder []model.ID
	for _, p := range m.PermissionReferences {
		if _, seen := refsByGuard[p.GuardApplicationID]; !seen {
			guardOrder = append(guardOrder, p.GuardApplicationID)
		}
		refsByGuard[p.GuardApplicationID] = append(refsByGuard[p.GuardApplicationID], p)
	}
	guardsByEndpoint := map[model.ID][]model.ID{}
	var endpointOrder []model.ID
	for _, g := range guardOrder {
		app, ok := guardAppByID[g]
		if !ok {
			continue
		}
		if _, seen := guardsByEndpoint[app.EndpointID]; !seen {
			endpointOrder = append(endpointOrder, app.EndpointID)
		}
		guardsByEndpoint[app.EndpointID] = append(guardsByEndpoint[app.EndpointID], g)
	}
	hasRoleRefs := map[model.ID]bool{}
	for _, r := range m.RoleReferences {
		if app, ok := guardAppByID[r.GuardApplicationID]; ok {
			hasRoleRefs[app.EndpointID] = true
		}
	}

	omitWith := func(ep model.ID, reason OmissionReason, detail string) {
		omit[ep] = Omission{Reason: reason, Detail: detail}
	}

	for _, ep := range endpointOrder {
		guards := guardsByEndpoint[ep]
		switch {
		case len(guards) > 1:
			omitWith(ep, ReasonPermissionNotExportable, "requires permissions through more than one annotation, "+
				"and how Spring combines them here is not modelled, so nothing was exported rather than guess")
			continue
		case hasRoleRefs[ep]:
			omitWith(ep, ReasonPermissionNotExportable, "requires a permission and a role through separate "+
				"annotations; exporting the role alone would drop the permission, so nothing was exported")
			continue
		}
		refs := refsByGuard[guards[0]]
		via := refs[0].Via
		var literals []string
		seen := map[string]bool{}
		for _, r := range refs {
			if r.Via != via {
				via = ""
				break
			}
			if !seen[r.RawLiteral] {
				seen[r.RawLiteral] = true
				literals = append(literals, r.RawLiteral)
			}
		}
		if via == "" {
			omitWith(ep, ReasonPermissionNotExportable, "one annotation names permissions through more than one call")
			continue
		}

		combine, isPermission := d.PermissionCallees[via]
		roleCombine, isRole := d.RoleCallees[via]
		if isRole {
			combine = roleCombine
		}
		if !isPermission && !isRole {
			omitWith(ep, ReasonCalleeNotDeclared, "requires "+callText(via, literals)+", and "+via+
				" was not declared with --permission-callee or --role-callee, so what it means was not assumed")
			continue
		}
		if len(literals) > 1 && combine == CombineUnstated {
			omitWith(ep, ReasonCombinationNotDeclared, "requires "+callText(via, literals)+"; whether a principal "+
				"needs any or all of them was not declared (a :any-of or :all-of suffix on the callee)")
			continue
		}

		app := guardAppByID[guards[0]]
		grants := layerGrants(ep, layerOf(app.AppliedAt))
		escapes := escapeTerms(d)
		if isRole && combine != CombineAllOf && len(d.SuperuserPermissions) == 0 {
			// The plain case: the literals, and any superuser role, are
			// alternatives in a roles: list.
			for _, r := range append(append([]string{}, literals...), d.SuperuserRoles...) {
				*grants = appendUniqueGrant(*grants, roleGrant{name: r})
			}
			continue
		}
		term := permissionTerm
		if isRole {
			term = roleTerm
		}
		core := make([]Condition, len(literals))
		for i, l := range literals {
			core[i] = term(l)
		}
		*grants = appendUniqueGrant(*grants, roleGrant{name: anyAuthenticatedRole, verified: true})
		condition[ep] = combineCondition(core, combine, escapes)
	}
}

// escapeTerms are the declared superuser escape, as match terms.
func escapeTerms(d Declarations) []Condition {
	var out []Condition
	for _, p := range d.SuperuserPermissions {
		out = append(out, permissionTerm(p))
	}
	for _, r := range d.SuperuserRoles {
		out = append(out, roleTerm(r))
	}
	return out
}

// combineCondition builds the requirement, then lets any escape satisfy it
// on its own — the any.of shape verified against the real engine.
func combineCondition(core []Condition, combine Combine, escapes []Condition) *Condition {
	var req Condition
	switch {
	case len(core) == 1:
		req = core[0]
	case combine == CombineAllOf:
		req = Condition{All: core}
	default:
		req = Condition{Any: core}
	}
	if len(escapes) == 0 {
		return &req
	}
	var terms []Condition
	seen := map[string]bool{}
	add := func(cs ...Condition) {
		for _, c := range cs {
			if k := c.key(); !seen[k] {
				seen[k] = true
				terms = append(terms, c)
			}
		}
	}
	add(escapes...)
	if len(req.Any) > 0 {
		add(req.Any...)
	} else {
		add(req)
	}
	if len(terms) == 1 {
		return &terms[0]
	}
	return &Condition{Any: terms}
}

func callText(via string, literals []string) string {
	quoted := make([]string, len(literals))
	for i, l := range literals {
		quoted[i] = "'" + l + "'"
	}
	return via + "(" + strings.Join(quoted, ", ") + ")"
}
