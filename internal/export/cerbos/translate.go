// Package cerbos translates Sphinxor's intermediate model (internal/model)
// into a Cerbos policy set, per docs/decisions/0009-cerbos-exporter.md.
//
// This package depends only on internal/model — never on
// internal/extract/nestjs or anything NestJS-specific. That boundary is
// deliberate (ADR 0009): everything here works for any future framework's
// extractor automatically, because it only ever reads the normalized
// model.
package cerbos

import (
	"sort"
	"strings"

	"github.com/chebilax/sphinxor/internal/model"
)

// anyAuthenticatedRole is Cerbos's documented special role value: "The
// special value `*` can be used to disregard roles when evaluating the
// rule" (Cerbos resource_policies docs). Used for AuthenticationRequirement
// grants (ADR 0010) — confirmed behaviorally with a real cerbos compile
// and a passing test asserting EFFECT_ALLOW for an unrelated role, not
// just read from the docs.
const anyAuthenticatedRole = "*"

// Rule is one confirmed (resource, action) -> roles grant, ready to become
// a Cerbos resource policy rule. It always has at least one role and at
// least one contributing endpoint — an endpoint with no confirmed role
// never becomes a Rule (see Omission).
type Rule struct {
	Resource  string
	Action    string
	Roles     []string // deduped, sorted
	Endpoints []model.Endpoint
}

// OmissionReason is why an endpoint did not become part of any Rule.
// Per ADR 0009 §3, omission is the safe default whenever the model can't
// establish a grant with certainty — never a guess.
type OmissionReason string

const (
	// ReasonNoGuard: no GuardApplication at all was found for this
	// endpoint. Cerbos denies by default for an action with no matching
	// rule, so omitting it is the structurally safe state.
	ReasonNoGuard OmissionReason = "no-guard"
	// ReasonNoRole: at least one GuardApplication exists, but no
	// RoleReference resolved to a role name — e.g. a bare AuthGuard with
	// no @Roles(), or @Roles()/composite-resolved with an empty role
	// list. This usually means "authenticated, any role" in the source,
	// which the model has no way to express as a Cerbos role grant.
	ReasonNoRole OmissionReason = "guarded-no-role"
	// ReasonActionCollision: two or more endpoints on the same resource
	// map to the same Cerbos action (ADR 0009 §2's controller+method
	// mapping has no path component), and they don't all carry the exact
	// same confirmed role set — including the case where some have roles
	// and others have none. Merging them into one rule would be wrong for
	// at least one of the real endpoints, in either direction (granting
	// too much to the endpoint with fewer roles, or too little to the one
	// with more), so none of the colliding endpoints become a Rule.
	ReasonActionCollision OmissionReason = "action-collision"
	// ReasonNoCommonRole: a single endpoint has role-bearing evidence in
	// more than one independent layer (e.g. a method annotation and a
	// URL-pattern rule — docs/decisions/0012-securityfilterchain-effective-policy.md),
	// and those layers' allowed-role sets share no role in common. Spring
	// AND-combines both layers, so a role is only safely exportable if a
	// principal holding it would pass every layer independently — the
	// empty-intersection case means no single role does. This does not
	// claim the endpoint is unreachable: a principal holding one
	// qualifying role for each layer separately could still pass in the
	// real app even though no role satisfies both at once — the honest
	// claim is narrower, that no role is safe to export here, not that
	// nobody can ever reach it.
	ReasonNoCommonRole OmissionReason = "no-common-role"
)

// Omission records one endpoint that could not become part of any Rule,
// and why — surfaced in both the companion report and inline policy
// comments (ADR 0009 §4).
type Omission struct {
	Endpoint model.Endpoint
	Resource string // "" if the endpoint's controller couldn't be resolved
	Reason   OmissionReason
	Detail   string
}

// UnverifiedRole flags a Role that WAS exported (it's part of a Rule) but
// whose RoleReference never resolved to a known RoleDeclaration — Sphinxor
// itself can't confirm it's a real, canonical role name (ADR 0009 §3).
// Unlike Omission, this doesn't withhold the grant: Cerbos only needs a
// role name string, and withholding a role the source code actually
// checks for would make the exported policy wrong in the denying
// direction. It's flagged, not omitted.
type UnverifiedRole struct {
	Resource string
	Action   string
	Role     string
	Endpoint model.Endpoint
}

// Result is the full output of translating one Model: every confirmed
// rule, every omission, and every unverified-but-exported role.
type Result struct {
	Rules           []Rule
	Omissions       []Omission
	UnverifiedRoles []UnverifiedRole
}

// roleGrant is one role name resolved for one endpoint, and whether it
// resolved to a known RoleDeclaration.
type roleGrant struct {
	name     string
	verified bool
}

// layer distinguishes where an endpoint's grant-bearing evidence came
// from, for the method×URL effective-policy reduction (ADR 0012 §2).
type layer int

const (
	layerMethod layer = iota
	layerURL
)

// layerOf classifies a GuardScope into the layer it contributes to.
// Everything except a URL-pattern rule is the method layer — class-level,
// method-level, and composite-resolved guards are all annotation-derived
// facts about the endpoint's own handler, indistinguishable from each
// other for this purpose, unlike a rule declared entirely apart from any
// handler.
func layerOf(scope model.GuardScope) layer {
	if scope == model.ScopeRequestMatcher {
		return layerURL
	}
	return layerMethod
}

// conflictingLayers records what each layer required when an endpoint's
// method-layer and URL-layer grants share no common role (ReasonNoCommonRole).
type conflictingLayers struct {
	method []roleGrant
	url    []roleGrant
}

// endpointEntry is one endpoint together with whatever roleGrants it has
// (possibly none), grouped by (resource, action) before any Rule/Omission
// decision is made — the decision needs every endpoint sharing that key,
// not just the ones that individually look confirmed, or a confirmed
// endpoint's rule could silently also govern an unconfirmed sibling that
// happens to share the same Cerbos action (see ReasonActionCollision).
type endpointEntry struct {
	endpoint model.Endpoint
	grants   []roleGrant
}

// Translate builds a Result from m. It never consults m.Findings for
// grant decisions — a Finding documents Sphinxor's own uncertainty about
// the source, not a fact to translate into a policy; the safety posture
// (ADR 0009 §3) is built entirely from GuardApplication/RoleReference,
// the same evidence a human reviewer could check by hand.
func Translate(m *model.Model) Result {
	controllerByID := make(map[model.ID]model.Controller, len(m.Controllers))
	for _, c := range m.Controllers {
		controllerByID[c.ID] = c
	}

	guardAppByID := make(map[model.ID]model.GuardApplication, len(m.GuardApplications))
	guardedEndpoints := make(map[model.ID]bool, len(m.GuardApplications))
	for _, g := range m.GuardApplications {
		guardAppByID[g.ID] = g
		guardedEndpoints[g.EndpointID] = true
	}

	// Grants are collected per layer, not into one flat set, before being
	// reduced to each endpoint's final grant list (ADR 0012 §2). A single
	// layer's grants are still just a union (a class-level and a
	// method-level guard on the same endpoint remain alternatives, as
	// always) — the layer split only matters once an endpoint has
	// role-bearing evidence in more than one layer, which every NestJS
	// endpoint and most Spring endpoints never do.
	grantsByEndpointLayer := make(map[model.ID]*[2][]roleGrant)
	layerGrants := func(endpointID model.ID, l layer) *[]roleGrant {
		lg, ok := grantsByEndpointLayer[endpointID]
		if !ok {
			lg = &[2][]roleGrant{}
			grantsByEndpointLayer[endpointID] = lg
		}
		return &lg[l]
	}

	for _, ref := range m.RoleReferences {
		app, ok := guardAppByID[ref.GuardApplicationID]
		if !ok {
			continue
		}
		l := layerGrants(app.EndpointID, layerOf(app.AppliedAt))
		*l = appendUniqueGrant(*l, roleGrant{
			name:     ref.RawLiteral,
			verified: ref.RoleDeclarationID != nil,
		})
	}
	// AuthenticationRequirement (ADR 0010) is a second, independent grant
	// source: "authenticated, any role" in the source, confirmed by a
	// recognized authentication guard with no resolved role — Cerbos's
	// documented `*` role disregards roles when evaluating a rule. Always
	// verified=true: this is a positive, confirmed fact extraction itself
	// establishes, not a reference resolved against a declaration, so the
	// "could not be verified against a known declaration" flagging that
	// applies to RoleReference has nothing to check here.
	for _, req := range m.AuthenticationRequirements {
		l := layerGrants(req.EndpointID, layerOf(req.Scope))
		*l = appendUniqueGrant(*l, roleGrant{
			name:     anyAuthenticatedRole,
			verified: true,
		})
	}

	// Reduce each endpoint's per-layer grants to one final grant list.
	// When only one layer has role-bearing evidence, that layer's grants
	// are already the answer — today's behavior, unchanged. When both
	// layers do, Spring AND-combines them at runtime, so the effective,
	// safely-exportable grant is the set INTERSECTION, not a subset check
	// (ADR 0012 §2): sound (never grants a role unless a principal holding
	// it is provably allowed by both layers) but not complete (a
	// principal separately holding one qualifying role per layer can
	// still pass without holding anything in the intersection) — the
	// accepted, safe-direction incompleteness this project takes
	// everywhere else, not a new kind of guess.
	rolesByEndpoint := make(map[model.ID][]roleGrant, len(grantsByEndpointLayer))
	noCommonRole := make(map[model.ID]conflictingLayers)
	for endpointID, lg := range grantsByEndpointLayer {
		method, url := lg[layerMethod], lg[layerURL]
		switch {
		case len(method) == 0 && len(url) == 0:
			// Shouldn't happen — layerGrants is only ever allocated when a
			// grant is about to be appended — but if it did, there's
			// nothing to record either way.
		case len(url) == 0:
			rolesByEndpoint[endpointID] = method
		case len(method) == 0:
			rolesByEndpoint[endpointID] = url
		default:
			inter := intersectGrants(method, url)
			if len(inter) == 0 {
				noCommonRole[endpointID] = conflictingLayers{method: method, url: url}
				continue
			}
			rolesByEndpoint[endpointID] = inter
		}
	}

	type groupKey = [2]string // [resource, action]
	groups := make(map[groupKey][]endpointEntry)
	var order []groupKey

	for _, e := range m.Endpoints {
		resource := ""
		if c, ok := controllerByID[e.ControllerID]; ok {
			resource = ResourceKind(c.Name)
		}
		key := groupKey{resource, strings.ToLower(string(e.HTTPMethod))}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], endpointEntry{endpoint: e, grants: rolesByEndpoint[e.ID]})
	}

	var rules []Rule
	var omissions []Omission
	var unverified []UnverifiedRole

	for _, key := range order {
		resource, action := key[0], key[1]
		entries := groups[key]

		agree := true
		for _, en := range entries[1:] {
			if !sameGrantSet(entries[0].grants, en.grants) {
				agree = false
				break
			}
		}

		switch {
		case agree && len(entries[0].grants) == 0:
			// Nobody in this action shares any evidence at all — no
			// collision to report, just each endpoint's own plain reason.
			for _, en := range entries {
				if conflict, ok := noCommonRole[en.endpoint.ID]; ok {
					// This endpoint isn't unguarded — it has role-bearing
					// evidence in two independent layers that share no
					// common role (ADR 0012 §2), a more specific and more
					// useful fact than the generic "guarded, no role"
					// reason below.
					omissions = append(omissions, Omission{
						Endpoint: en.endpoint,
						Resource: resource,
						Reason:   ReasonNoCommonRole,
						Detail:   noCommonRoleDetail(conflict),
					})
					continue
				}
				reason, detail := ReasonNoGuard, "no access control detected for this endpoint"
				if guardedEndpoints[en.endpoint.ID] {
					// Endpoints genuinely "authenticated, any role" export
					// via AuthenticationRequirement (ADR 0010) instead of
					// landing here — this branch is only reached now by a
					// guard extraction doesn't recognize (unknown
					// authentication semantics) or a literal empty
					// @Roles() (flagged separately, by empty-role, as a
					// likely mistake, not exported as if it were correct).
					reason, detail = ReasonNoRole, "has a guard, but Sphinxor could not determine what access it actually requires — either the guard isn't one Sphinxor recognizes, or the @Roles() call is empty (flagged separately as a likely mistake) — nothing was granted rather than guess"
				}
				omissions = append(omissions, Omission{Endpoint: en.endpoint, Resource: resource, Reason: reason, Detail: detail})
			}

		case agree:
			// Every endpoint sharing this action agrees on the same
			// non-empty role set — safe to merge into one Rule.
			var merged []roleGrant
			for _, en := range entries {
				for _, g := range en.grants {
					merged = appendUniqueGrant(merged, g)
				}
			}
			roleNames := make([]string, 0, len(merged))
			for _, gr := range merged {
				roleNames = append(roleNames, gr.name)
				if !gr.verified {
					for _, en := range entries {
						unverified = append(unverified, UnverifiedRole{Resource: resource, Action: action, Role: gr.name, Endpoint: en.endpoint})
					}
				}
			}
			sort.Strings(roleNames)

			endpoints := make([]model.Endpoint, len(entries))
			for i, en := range entries {
				endpoints[i] = en.endpoint
			}
			sortEndpoints(endpoints)

			rules = append(rules, Rule{Resource: resource, Action: action, Roles: roleNames, Endpoints: endpoints})

		default:
			// Endpoints sharing this action disagree — some have roles
			// and others don't, or they have different roles. The
			// controller+method mapping can't tell them apart, so none
			// of them can be safely represented.
			for _, en := range entries {
				omissions = append(omissions, Omission{
					Endpoint: en.endpoint,
					Resource: resource,
					Reason:   ReasonActionCollision,
					Detail:   collisionDetail(resource, action, entries, en),
				})
			}
		}
	}

	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Resource != rules[j].Resource {
			return rules[i].Resource < rules[j].Resource
		}
		return rules[i].Action < rules[j].Action
	})
	sort.Slice(omissions, func(i, j int) bool {
		if omissions[i].Endpoint.Path != omissions[j].Endpoint.Path {
			return omissions[i].Endpoint.Path < omissions[j].Endpoint.Path
		}
		return omissions[i].Endpoint.HTTPMethod < omissions[j].Endpoint.HTTPMethod
	})
	sort.Slice(unverified, func(i, j int) bool {
		if unverified[i].Resource != unverified[j].Resource {
			return unverified[i].Resource < unverified[j].Resource
		}
		if unverified[i].Action != unverified[j].Action {
			return unverified[i].Action < unverified[j].Action
		}
		return unverified[i].Role < unverified[j].Role
	})

	return Result{Rules: rules, Omissions: omissions, UnverifiedRoles: unverified}
}

// collisionDetail explains one action-collision omission in plain terms,
// self-contained enough to read in the generated report or an inline YAML
// comment without any other context (no internal doc references) —
// confirmed by rereading a real generated report cold, the way a user who
// never saw this project's design conversation would. Names what THIS
// endpoint requires, what each sibling requires, and why sharing a Cerbos
// action with a differently-guarded sibling means neither gets exported.
func collisionDetail(resource, action string, all []endpointEntry, self endpointEntry) string {
	var others []string
	for _, en := range all {
		if en.endpoint.ID == self.endpoint.ID {
			continue
		}
		others = append(others, string(en.endpoint.HTTPMethod)+" "+en.endpoint.Path+" ("+grantSummary(en.grants)+")")
	}
	return "this endpoint " + grantSummary(self.grants) + ", but shares the \"" + action +
		"\" action on the \"" + resource + "\" resource with " + strings.Join(others, ", ") +
		" — Sphinxor maps one Cerbos action per HTTP method on a resource and can't tell these routes" +
		" apart by path, so since they don't all need the same access, none of them were exported for" +
		" this action rather than risk granting the wrong one"
}

// noCommonRoleDetail explains a ReasonNoCommonRole omission in plain
// terms, self-contained per the same "reread cold" standard collisionDetail
// already meets: states what each independent layer required and that
// they share no role — deliberately not claiming the endpoint is
// unreachable (see ReasonNoCommonRole's own doc comment for why that
// would overclaim).
func noCommonRoleDetail(c conflictingLayers) string {
	return "this endpoint is guarded in two independent ways that must both be satisfied — one " +
		grantSummary(c.method) + ", the other " + grantSummary(c.url) +
		" — and they share no role in common, so no single role is safe to export as sufficient here." +
		" A principal holding a different qualifying role for each side separately could still be" +
		" allowed by the real application; review both locations to confirm the intended access."
}

// grantSummary renders what an endpoint's confirmed grants require, in
// plain language rather than Sphinxor's internal role-name format alone —
// used in collisionDetail and the companion report's role columns.
func grantSummary(grants []roleGrant) string {
	if len(grants) == 0 {
		return "requires no specific role"
	}
	names := make([]string, len(grants))
	for i, g := range grants {
		if g.name == anyAuthenticatedRole {
			names[i] = "any authenticated user"
		} else {
			names[i] = "role " + g.name
		}
	}
	sort.Strings(names)
	return "requires " + strings.Join(names, " and ")
}

// ResourceKind derives a Cerbos resource kind from a NestJS controller
// class name: strips a trailing "Controller" suffix, then converts to
// snake_case (e.g. "UsersController" -> "users", "PostController" ->
// "post"). Per ADR 0009 §2, this is a stated, coarse limitation, not a
// claim of idiomatic Cerbos naming.
func ResourceKind(controllerName string) string {
	name := strings.TrimSuffix(controllerName, "Controller")
	if name == "" {
		name = controllerName
	}
	var b strings.Builder
	for i, r := range name {
		if i > 0 && isUpper(r) && !isUpper(rune(name[i-1])) {
			b.WriteByte('_')
		}
		b.WriteRune(toLower(r))
	}
	return b.String()
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
func toLower(r rune) rune {
	if isUpper(r) {
		return r + ('a' - 'A')
	}
	return r
}

func appendUniqueGrant(s []roleGrant, v roleGrant) []roleGrant {
	for i, existing := range s {
		if existing.name == v.name {
			// A role can be referenced more than once for the same
			// endpoint (e.g. class-level + method-level guards). If any
			// reference resolved to a declaration, treat the role as
			// verified overall — one confirmed declaration is enough.
			if v.verified && !existing.verified {
				s[i].verified = true
			}
			return s
		}
	}
	return append(s, v)
}

// isUniversalGrant reports whether grants represents "authenticated, any
// role" (AuthenticationRequirement's `*`) and nothing else. By
// construction (ADR 0010's own exclusion: an AuthenticationRequirement is
// never created for an endpoint/layer that also resolved a concrete
// role), a single layer's grants are never a mix of `*` and a named role
// — `*` alone or not at all.
func isUniversalGrant(grants []roleGrant) bool {
	return len(grants) == 1 && grants[0].name == anyAuthenticatedRole
}

// intersectGrants computes the set-intersection effective policy for two
// independent layers that both apply to the same endpoint (ADR 0012 §2):
// sound (a role survives only if it satisfies both layers) but not
// complete (a principal who separately holds one qualifying role per
// layer can still pass the real app without holding anything in the
// intersection) — the same safe-direction incompleteness this project
// accepts everywhere else, not a new kind of guess.
//
// `*` is the identity element: authenticated-any-role imposes no
// constraint beyond what the other layer already requires, so it never
// narrows an intersection with a concrete set, and `*` ∩ `*` = `*`.
func intersectGrants(a, b []roleGrant) []roleGrant {
	if isUniversalGrant(a) {
		return b
	}
	if isUniversalGrant(b) {
		return a
	}

	bByName := make(map[string]roleGrant, len(b))
	for _, g := range b {
		bByName[g.name] = g
	}

	var out []roleGrant
	for _, g := range a {
		other, ok := bByName[g.name]
		if !ok {
			continue
		}
		out = append(out, roleGrant{name: g.name, verified: g.verified || other.verified})
	}
	return out
}

// sameGrantSet compares two grant lists by role name only (not verified
// status) — two endpoints requiring "the same role" agree for merging
// purposes even if one of them resolved it more confidently than the
// other; the merged Rule's UnverifiedRole flagging (see Translate) already
// accounts for that difference separately.
func sameGrantSet(a, b []roleGrant) bool {
	if len(a) != len(b) {
		return false
	}
	an := make(map[string]bool, len(a))
	for _, g := range a {
		an[g.name] = true
	}
	for _, g := range b {
		if !an[g.name] {
			return false
		}
	}
	return true
}

func sortEndpoints(endpoints []model.Endpoint) {
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Path != endpoints[j].Path {
			return endpoints[i].Path < endpoints[j].Path
		}
		return endpoints[i].HTTPMethod < endpoints[j].HTTPMethod
	})
}
