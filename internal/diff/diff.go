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
)

// Compare diffs base against head.
func Compare(base, head Snapshot) Result {
	var r Result

	r.AddedEndpoints, r.RemovedEndpoints = diffEndpoints(base.Model.Endpoints, head.Model.Endpoints)
	r.AddedRoleDeclarations, r.RemovedRoleDeclarations = diffRoleDeclarations(base.Model.RoleDeclarations, head.Model.RoleDeclarations)

	baseGuardsByKey := indexGuardApplications(base.Model)
	headGuardsByKey := indexGuardApplications(head.Model)
	r.AddedGuardApplications, r.RemovedGuardApplications = diffGuardApplications(baseGuardsByKey, headGuardsByKey)

	baseRefsByKey := indexRoleReferences(base.Model, guardAppByID(base.Model))
	headRefsByKey := indexRoleReferences(head.Model, guardAppByID(head.Model))
	r.AddedRoleReferences, r.RemovedRoleReferences = diffRoleReferences(baseRefsByKey, headRefsByKey)

	r.BecamePublic = becamePublic(base.Model, head.Model, baseGuardsByKey, headGuardsByKey)

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
