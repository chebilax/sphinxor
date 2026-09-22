// Package diff compares two Sphinxor model snapshots — typically the
// same NestJS project at two points in git history — per
// docs/decisions/0007-model-diff-design.md.
package diff

import (
	"sort"

	"github.com/chebilax/sphinxor/internal/model"
)

// Snapshot is one side of a diff: an extracted model together with the
// findings its lint rules produced against it (including allowlist
// status). Both are needed together — findings alone can't resolve
// stable cross-run subject keys (e.g. a role-declaration finding's
// SubjectID is a per-run sequential ID, not the declaration's stable
// Name), and the model alone says nothing about what gates CI.
type Snapshot struct {
	Model    *model.Model
	Findings []model.Finding
	// AllowlistedEndpoints is the set of endpoints carrying a
	// sphinxor-allow marker in this snapshot, as
	// internal/allowlist.Outcome reports it.
	//
	// Carried explicitly rather than derived from Findings, and that is
	// measured rather than assumed — docs/decisions/0036-became-public-gates-ci.md
	// §4. A GET endpoint that loses its @PreAuthorize and gains a marker
	// produces ZERO findings: the marker is matched (no
	// stale-allow-marker is emitted for it) but
	// mutating-endpoint-without-access-control does not fire on a read,
	// so no finding exists to carry Allowlisted: true. Deriving the set
	// from findings would gate a deliberately-public GET with the
	// author's marker one line above the handler.
	//
	// nil is a valid empty set: a caller that does not populate it gets
	// today's behaviour for cases (a) and (b), which do not consult it.
	AllowlistedEndpoints map[model.ID]bool
}

// Result is the outcome of comparing a base Snapshot to a head one —
// "base" and "head" matching CI terminology (vision.md: "relative to the
// reference branch"), not "old"/"new", to avoid the sense that either
// side is more current in general, only in this comparison.
type Result struct {
	AddedEndpoints   []model.Endpoint
	RemovedEndpoints []model.Endpoint

	AddedRoleDeclarations   []model.RoleDeclaration
	RemovedRoleDeclarations []model.RoleDeclaration

	AddedGuardApplications   []model.GuardApplication
	RemovedGuardApplications []model.GuardApplication

	AddedRoleReferences   []model.RoleReference
	RemovedRoleReferences []model.RoleReference

	// Added/RemovedPermissionReferences are the permission half of the
	// structural diff (docs/decisions/0037-permissions-in-the-diff.md
	// §2). Before them, a permission added, removed or changed between
	// two runs produced a report whose every section read "No change" —
	// confirmed on RuoYi-Vue, which reports zero roles and whose
	// endpoints' requirements are entirely permissions.
	//
	// There is deliberately no third "changed" list: a changed
	// permission is a removal plus an addition. Reporting it as one
	// entry means pairing a removal with an addition, which is matching
	// across an identity change — refused by ADR 0002, by ADR 0007 §2
	// and by ADR 0036 §5, and a guess as soon as two literals change on
	// one guard in the same commit.
	AddedPermissionReferences   []model.PermissionReference
	RemovedPermissionReferences []model.PermissionReference

	// BecamePublic lists endpoints present on both sides that had at
	// least one guard application in base and none in head — vision.md's
	// "endpoints that became public", derived from the guard-application
	// diff above rather than computed independently. An endpoint removed
	// entirely is not "became public" — that's RemovedEndpoints, a
	// distinct fact.
	BecamePublic []model.Endpoint

	Regressions []Regression
}

// HasRegressions is the CI-gating condition for `sphinxor diff`.
func (r Result) HasRegressions() bool {
	return len(r.Regressions) > 0
}

// Regression is a High-confidence finding that gates CI, per ADR 0007 §3.
type Regression struct {
	Finding model.Finding
	Reason  RegressionReason
}

// RegressionReason distinguishes the two ways a High-confidence finding
// can gate CI: it's genuinely new, or it lost the allowlist protection
// it had — the "someone removed a sphinxor-allow marker" case a
// point-in-time `sphinxor lint` run can't see.
type RegressionReason string

const (
	ReasonNew              RegressionReason = "new"
	ReasonAllowlistRemoved RegressionReason = "allowlist-removed"
	// ReasonBecamePublic is ADR 0036 §1's case (c): an endpoint present
	// on both sides, protected in base, unprotected in head, and not
	// allowlisted in head.
	//
	// It is the only regression reason whose Finding no lint rule
	// produces — it is a fact about a transition, meaningless in a single
	// snapshot, so `sphinxor lint` can never emit it (§7).
	ReasonBecamePublic RegressionReason = "became-public"
)

// Compare diffs base against head.
func Compare(base, head Snapshot) Result {
	var r Result

	r.AddedEndpoints, r.RemovedEndpoints = diffEndpoints(base.Model.Endpoints, head.Model.Endpoints)
	r.AddedRoleDeclarations, r.RemovedRoleDeclarations = diffRoleDeclarations(base.Model.RoleDeclarations, head.Model.RoleDeclarations)

	baseGuardsByKey := indexGuardApplications(base.Model)
	headGuardsByKey := indexGuardApplications(head.Model)
	r.AddedGuardApplications, r.RemovedGuardApplications = diffGuardApplications(baseGuardsByKey, headGuardsByKey)

	baseGuardsByID := guardAppByID(base.Model)
	headGuardsByID := guardAppByID(head.Model)

	baseRefsByKey := indexRoleReferences(base.Model, baseGuardsByID)
	headRefsByKey := indexRoleReferences(head.Model, headGuardsByID)
	r.AddedRoleReferences, r.RemovedRoleReferences = diffRoleReferences(baseRefsByKey, headRefsByKey)

	basePermsByKey := indexPermissionReferences(base.Model, baseGuardsByID)
	headPermsByKey := indexPermissionReferences(head.Model, headGuardsByID)
	r.AddedPermissionReferences, r.RemovedPermissionReferences = diffPermissionReferences(basePermsByKey, headPermsByKey)

	r.BecamePublic = becamePublic(base.Model, head.Model)

	r.Regressions = diffRegressions(base, head)

	return r
}

func diffEndpoints(base, head []model.Endpoint) (added, removed []model.Endpoint) {
	baseByID := make(map[model.ID]model.Endpoint, len(base))
	for _, e := range base {
		baseByID[e.ID] = e
	}
	headByID := make(map[model.ID]model.Endpoint, len(head))
	for _, e := range head {
		headByID[e.ID] = e
	}
	for id, e := range headByID {
		if _, ok := baseByID[id]; !ok {
			added = append(added, e)
		}
	}
	for id, e := range baseByID {
		if _, ok := headByID[id]; !ok {
			removed = append(removed, e)
		}
	}
	sortEndpoints(added)
	sortEndpoints(removed)
	return added, removed
}

func sortEndpoints(endpoints []model.Endpoint) {
	sort.Slice(endpoints, func(i, j int) bool { return endpoints[i].ID < endpoints[j].ID })
}

func diffRoleDeclarations(base, head []model.RoleDeclaration) (added, removed []model.RoleDeclaration) {
	baseByName := make(map[string]model.RoleDeclaration, len(base))
	for _, d := range base {
		baseByName[d.Name] = d
	}
	headByName := make(map[string]model.RoleDeclaration, len(head))
	for _, d := range head {
		headByName[d.Name] = d
	}
	for name, d := range headByName {
		if _, ok := baseByName[name]; !ok {
			added = append(added, d)
		}
	}
	for name, d := range baseByName {
		if _, ok := headByName[name]; !ok {
			removed = append(removed, d)
		}
	}
	sortRoleDeclarations(added)
	sortRoleDeclarations(removed)
	return added, removed
}

func sortRoleDeclarations(decls []model.RoleDeclaration) {
	sort.Slice(decls, func(i, j int) bool { return decls[i].Name < decls[j].Name })
}
