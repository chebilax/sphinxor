// Package collide resolves routes declared by more than one controller in
// a single analyzed tree, per
// docs/decisions/0020-unanalyzable-is-unknown-not-absent.md Amendment 2 §8.
//
// It is shared by both extractors rather than written twice because the
// defect, the fix, and the criterion are identical across them. Only the
// symptom differed: NestJS merged the colliding endpoints, so each was
// reported carrying the other's guards, while Spring dropped one of them
// outright.
package collide

import (
	"sort"
	"strings"

	"github.com/chebilax/sphinxor/internal/allowlist"
	"github.com/chebilax/sphinxor/internal/model"
)

// Input is one extractor's assembled state, plus the ownership bookkeeping
// needed to re-point everything that references an endpoint by ID.
//
// The ownership slices exist because a collision can only be recognized
// once the whole project has been walked, by which point guards and
// allowlist anchors already reference the provisional, shared ID. Each is
// parallel to its collection and holds the index into Model.Endpoints that
// the entry was created for.
type Input struct {
	Model *model.Model
	// GuardOwner is parallel to Model.GuardApplications.
	GuardOwner []int
	// Anchors and AnchorOwner are parallel to each other.
	Anchors     []allowlist.Anchor
	AnchorOwner []int
}

// Resolve finds every route declared by two or more *different*
// controllers, gives each colliding endpoint its own identity so nothing
// merges or disappears, re-points that endpoint's guards and allowlist
// anchors, and records what it found on the model.
//
// Two handlers declaring one route *within a single controller* are not a
// collision: that is content negotiation, and
// docs/decisions/0014-endpoint-identity-and-content-negotiation.md merges
// them deliberately. Only a second *controller* means a second possible
// application, prefix, or registration.
//
// Separating the endpoints is unconditional. Whether the user is told is
// decided later, from RouteCollision.GuardsDiffer: a collision whose sides
// carry the same guards has nothing to bleed. Making the separation
// conditional too would be the trap — where extraction recognizes no
// guards at all, both sides look identical, and that is precisely the case
// in which an endpoint would silently vanish.
func Resolve(in Input) {
	m := in.Model

	byID := make(map[model.ID][]int, len(m.Endpoints))
	for i := range m.Endpoints {
		byID[m.Endpoints[i].ID] = append(byID[m.Endpoints[i].ID], i)
	}

	controllerName := make(map[model.ID]string, len(m.Controllers))
	for _, c := range m.Controllers {
		controllerName[c.ID] = c.Name
	}
	signatures := guardSignatures(m, in.GuardOwner)

	newID := make(map[int]model.ID)
	var collisions []model.RouteCollision

	for _, idxs := range byID {
		if len(idxs) < 2 {
			continue
		}
		controllers := make(map[model.ID]bool, len(idxs))
		for _, i := range idxs {
			controllers[m.Endpoints[i].ControllerID] = true
		}
		if len(controllers) < 2 {
			continue
		}

		names := make([]string, 0, len(controllers))
		for id := range controllers {
			names = append(names, controllerName[id])
		}
		sort.Strings(names)

		differ := false
		for _, i := range idxs[1:] {
			if signatures[i] != signatures[idxs[0]] {
				differ = true
				break
			}
		}

		first := m.Endpoints[idxs[0]]
		collisions = append(collisions, model.RouteCollision{
			HTTPMethod:   first.HTTPMethod,
			Path:         first.Path,
			Controllers:  names,
			GuardsDiffer: differ,
		})

		for _, i := range idxs {
			e := &m.Endpoints[i]
			e.RouteCollision = true
			newID[i] = model.NewCollidingRouteEndpointID(e.HTTPMethod, e.File, controllerName[e.ControllerID], e.HandlerName)
		}
	}

	if len(newID) == 0 {
		return
	}

	for i, id := range newID {
		m.Endpoints[i].ID = id
	}
	for j, owner := range in.GuardOwner {
		if j >= len(m.GuardApplications) {
			break
		}
		if id, ok := newID[owner]; ok {
			m.GuardApplications[j].EndpointID = id
		}
	}
	for k, owner := range in.AnchorOwner {
		if k >= len(in.Anchors) {
			break
		}
		if id, ok := newID[owner]; ok {
			in.Anchors[k].EndpointID = id
		}
	}

	sort.Slice(collisions, func(i, j int) bool {
		if collisions[i].Path != collisions[j].Path {
			return collisions[i].Path < collisions[j].Path
		}
		return collisions[i].HTTPMethod < collisions[j].HTTPMethod
	})
	m.RouteCollisions = collisions
}

// guardSignatures renders each endpoint's guards and roles as one
// comparable string, so two colliding endpoints can be asked whether they
// actually differ.
//
// It compares what extraction *sees*. An endpoint whose protection is
// invisible here (an unrecognized composite decorator, a global guard)
// signs as unguarded, so two such endpoints compare equal and no warning
// is raised — stated and accepted in Amendment 2 §8, since the criterion
// can only track what the tool knows.
func guardSignatures(m *model.Model, guardOwner []int) map[int]string {
	rolesByGuard := make(map[model.ID][]string)
	for _, r := range m.RoleReferences {
		rolesByGuard[r.GuardApplicationID] = append(rolesByGuard[r.GuardApplicationID], r.RawLiteral)
	}

	parts := make(map[int][]string)
	for j, owner := range guardOwner {
		if j >= len(m.GuardApplications) {
			break
		}
		g := m.GuardApplications[j]
		roles := append([]string(nil), rolesByGuard[g.ID]...)
		sort.Strings(roles)
		parts[owner] = append(parts[owner], g.GuardName+"("+strings.Join(roles, "|")+")")
	}

	out := make(map[int]string, len(parts))
	for owner, p := range parts {
		sort.Strings(p)
		out[owner] = strings.Join(dedupe(p), ",")
	}
	return out
}

// dedupe collapses repeated entries in a sorted slice.
//
// Protection is a set of requirements, not a multiset: the same guard
// recorded twice for one endpoint is the same protection. It happens for
// real — an endpoint merged from two handlers under ADR 0014 keeps each
// handler's own annotations, so an identical @PreAuthorize can appear
// twice. Comparing multisets made such an endpoint look different from an
// otherwise identical sibling, and raised a route-collision warning where
// both sides require exactly the same thing. Caught by re-measuring the
// survey corpus after implementation: it was the one warning in
// eugenp/tutorials that the pre-implementation count had not predicted.
func dedupe(sorted []string) []string {
	out := sorted[:0:0]
	for i, v := range sorted {
		if i == 0 || v != sorted[i-1] {
			out = append(out, v)
		}
	}
	return out
}
