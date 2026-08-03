package diff

import (
	"sort"
	"strings"

	"github.com/chebilax/sphinxor/internal/model"
)

// guardAppKey is GuardApplication's derived stable cross-run identity —
// it has no natural one, since its ID is a per-run sequential value
// (ADR 0007 §2). FromComposite is deliberately excluded: whether a guard
// was resolved via composite expansion or found literally is an
// extraction-mechanism detail, not a fact about the endpoint's
// authorization surface.
type guardAppKey struct {
	endpointID model.ID
	guardName  string
	appliedAt  model.GuardScope
}

func keyOfGuardApplication(g model.GuardApplication) guardAppKey {
	return guardAppKey{endpointID: g.EndpointID, guardName: g.GuardName, appliedAt: g.AppliedAt}
}

func indexGuardApplications(m *model.Model) map[guardAppKey]model.GuardApplication {
	out := make(map[guardAppKey]model.GuardApplication, len(m.GuardApplications))
	for _, g := range m.GuardApplications {
		out[keyOfGuardApplication(g)] = g
	}
	return out
}

func guardAppByID(m *model.Model) map[model.ID]model.GuardApplication {
	out := make(map[model.ID]model.GuardApplication, len(m.GuardApplications))
	for _, g := range m.GuardApplications {
		out[g.ID] = g
	}
	return out
}

func diffGuardApplications(base, head map[guardAppKey]model.GuardApplication) (added, removed []model.GuardApplication) {
	for k, g := range head {
		if _, ok := base[k]; !ok {
			added = append(added, g)
		}
	}
	for k, g := range base {
		if _, ok := head[k]; !ok {
			removed = append(removed, g)
		}
	}
	sortGuardApplications(added)
	sortGuardApplications(removed)
	return added, removed
}

func sortGuardApplications(guards []model.GuardApplication) {
	sort.Slice(guards, func(i, j int) bool {
		if guards[i].EndpointID != guards[j].EndpointID {
			return guards[i].EndpointID < guards[j].EndpointID
		}
		return guards[i].GuardName < guards[j].GuardName
	})
}

// roleRefKey is RoleReference's derived stable cross-run identity: which
// (stable-keyed) guard application it belongs to, plus the role it
// names. literal is normalizeRawLiteral(RawLiteral), not the raw field —
// see that function's doc comment.
type roleRefKey struct {
	guardApp guardAppKey
	literal  string
}

// normalizeRawLiteral collapses whitespace (including newlines) to a
// single space and trims the result, for keying purposes only — the
// model's own RoleReference.RawLiteral field is never modified, so
// display always shows the verbatim source text.
//
// Exists because RawLiteral's fallback case (an argument that's neither
// a clean member-expression nor a string literal) is the argument's raw
// source text verbatim (see resolveRoleArg in internal/extract/nestjs) —
// a pure reformat of that expression (e.g. a multi-line call collapsed
// onto one line) would otherwise change the key without changing
// meaning, diffing an unchanged role reference as removed-then-added
// noise in the structural diff on every such reformat.
func normalizeRawLiteral(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func keyOfRoleReference(r model.RoleReference, guardsByID map[model.ID]model.GuardApplication) (roleRefKey, bool) {
	g, ok := guardsByID[r.GuardApplicationID]
	if !ok {
		return roleRefKey{}, false
	}
	return roleRefKey{guardApp: keyOfGuardApplication(g), literal: normalizeRawLiteral(r.RawLiteral)}, true
}

func indexRoleReferences(m *model.Model, guardsByID map[model.ID]model.GuardApplication) map[roleRefKey]model.RoleReference {
	out := make(map[roleRefKey]model.RoleReference, len(m.RoleReferences))
	for _, ref := range m.RoleReferences {
		k, ok := keyOfRoleReference(ref, guardsByID)
		if !ok {
			continue // orphaned reference — shouldn't happen given extraction always creates the owning guard application first, but degrade gracefully rather than panic
		}
		out[k] = ref
	}
	return out
}

func diffRoleReferences(base, head map[roleRefKey]model.RoleReference) (added, removed []model.RoleReference) {
	for k, ref := range head {
		if _, ok := base[k]; !ok {
			added = append(added, ref)
		}
	}
	for k, ref := range base {
		if _, ok := head[k]; !ok {
			removed = append(removed, ref)
		}
	}
	sortRoleReferences(added)
	sortRoleReferences(removed)
	return added, removed
}

func sortRoleReferences(refs []model.RoleReference) {
	sort.Slice(refs, func(i, j int) bool { return refs[i].RawLiteral < refs[j].RawLiteral })
}

// becamePublic finds endpoints present in both base and head that had a
// guard application in base and none in head — vision.md's "endpoints
// that became public".
func becamePublic(baseModel, headModel *model.Model, baseGuardsByKey, headGuardsByKey map[guardAppKey]model.GuardApplication) []model.Endpoint {
	baseGuardedEndpoints := make(map[model.ID]bool, len(baseGuardsByKey))
	for k := range baseGuardsByKey {
		baseGuardedEndpoints[k.endpointID] = true
	}
	headGuardedEndpoints := make(map[model.ID]bool, len(headGuardsByKey))
	for k := range headGuardsByKey {
		headGuardedEndpoints[k.endpointID] = true
	}

	baseByID := make(map[model.ID]model.Endpoint, len(baseModel.Endpoints))
	for _, e := range baseModel.Endpoints {
		baseByID[e.ID] = e
	}
	headByID := make(map[model.ID]model.Endpoint, len(headModel.Endpoints))
	for _, e := range headModel.Endpoints {
		headByID[e.ID] = e
	}

	var out []model.Endpoint
	for id := range baseByID {
		e, stillExists := headByID[id]
		if !stillExists {
			continue // removed entirely — RemovedEndpoints' concern, not this one
		}
		if baseGuardedEndpoints[id] && !headGuardedEndpoints[id] {
			out = append(out, e)
		}
	}
	sortEndpoints(out)
	return out
}
