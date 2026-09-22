package diff

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// snapshotWithPermissions builds one side of a diff: one endpoint, one
// @PreAuthorize guard application on it, and the given permission
// references attached to that guard.
func snapshotWithPermissions(refs ...model.PermissionReference) Snapshot {
	for i := range refs {
		if refs[i].ID == "" {
			refs[i].ID = model.ID("perm-" + string(rune('1'+i)))
		}
		if refs[i].GuardApplicationID == "" {
			refs[i].GuardApplicationID = "g1"
		}
	}
	return Snapshot{Model: &model.Model{
		Endpoints: []model.Endpoint{
			{ID: "PUT /system/user", HTTPMethod: model.MethodPut, Path: "/system/user"},
		},
		GuardApplications: []model.GuardApplication{{
			ID:                  "g1",
			EndpointID:          "PUT /system/user",
			GuardName:           "PreAuthorize",
			AppliedAt:           model.ScopeMethod,
			DeclaresRoles:       true,
			DeclaresPermissions: true,
		}},
		PermissionReferences: refs,
	}}
}

func literals(refs []model.PermissionReference) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Via+"('"+r.RawLiteral+"')")
	}
	return out
}

func assertPermissionDiff(t *testing.T, r Result, wantAdded, wantRemoved []string) {
	t.Helper()
	gotAdded, gotRemoved := literals(r.AddedPermissionReferences), literals(r.RemovedPermissionReferences)
	if !equalStrings(gotAdded, wantAdded) {
		t.Errorf("added = %v, want %v", gotAdded, wantAdded)
	}
	if !equalStrings(gotRemoved, wantRemoved) {
		t.Errorf("removed = %v, want %v", gotRemoved, wantRemoved)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestDiffPermissionReferences_LiteralChanged is the gap ADR 0035 §7
// left open and ADR 0037 closes: before it, editing a @PreAuthorize
// permission produced a diff whose every section read "No change".
// Confirmed on RuoYi-Vue itself, not only here.
func TestDiffPermissionReferences_LiteralChanged(t *testing.T) {
	base := snapshotWithPermissions(model.PermissionReference{Via: "@ss.hasPermi", RawLiteral: "system:user:edit"})
	head := snapshotWithPermissions(model.PermissionReference{Via: "@ss.hasPermi", RawLiteral: "system:user:remove"})

	r := Compare(base, head)

	assertPermissionDiff(t, r,
		[]string{"@ss.hasPermi('system:user:remove')"},
		[]string{"@ss.hasPermi('system:user:edit')"})

	// ADR 0037 §3: a permission changing to a different string is
	// reported and NOT gated. Sphinxor holds no ordering over roles
	// (ADR 0031) and strictly less than that over permissions — under
	// ADR 0035 §3 it does not even know that @ss.hasPermi denotes a
	// permission check. There is no honest direction to rank two opaque
	// strings in.
	if r.HasRegressions() {
		t.Errorf("a permission literal changing must not gate CI, got %+v", r.Regressions)
	}
}

// TestDiffPermissionReferences_CalleeChanged is why Via is in the key
// (ADR 0037 §1). The literal is identical on both sides; only the bean
// method changed. ADR 0035 §3 decided Sphinxor records the literal AND
// the callee and does not decide what the callee means, so keying on the
// literal alone would assert these two are the same requirement — that
// ADR's question answered by accident, through a comparison key.
//
// RuoYi-Vue carries exactly this pair: 115 @ss.hasPermi and one
// @ss.hasRole on the same bean.
func TestDiffPermissionReferences_CalleeChanged(t *testing.T) {
	base := snapshotWithPermissions(model.PermissionReference{Via: "@ss.hasPermi", RawLiteral: "admin"})
	head := snapshotWithPermissions(model.PermissionReference{Via: "@ss.hasRole", RawLiteral: "admin"})

	r := Compare(base, head)

	assertPermissionDiff(t, r,
		[]string{"@ss.hasRole('admin')"},
		[]string{"@ss.hasPermi('admin')"})
	if r.HasRegressions() {
		t.Errorf("a callee change must not gate CI, got %+v", r.Regressions)
	}
}

// TestDiffPermissionReferences_ArgumentsReordered pins ADR 0037 §1's
// decision to leave the argument's ordinal out of the key. A
// permission's position in an argument list is not part of the
// requirement, and including it would report a reorder that changes
// nothing as two removals and two additions.
func TestDiffPermissionReferences_ArgumentsReordered(t *testing.T) {
	base := snapshotWithPermissions(
		model.PermissionReference{Via: "@el.check", RawLiteral: "a"},
		model.PermissionReference{Via: "@el.check", RawLiteral: "b"},
	)
	head := snapshotWithPermissions(
		model.PermissionReference{Via: "@el.check", RawLiteral: "b"},
		model.PermissionReference{Via: "@el.check", RawLiteral: "a"},
	)

	r := Compare(base, head)

	assertPermissionDiff(t, r, nil, nil)
}

// TestDiffPermissionReferences_VerbatimLiteralNotNormalized is the
// counterpart to TestDiffRoleReferences_NormalizesWhitespaceForKeying,
// and it asserts the OPPOSITE behaviour deliberately (ADR 0037 §1).
//
// normalizeRawLiteral exists for NestJS's resolveRoleArg fallback, which
// puts raw source text into RawLiteral so a reformat changes the key
// without changing the meaning. ADR 0035 §2 admits a permission only
// when every argument is a clean single-quoted literal, so that fallback
// has no counterpart here — while normalizing WOULD collapse two
// genuinely different permission strings onto one key. These two
// literals differ by one space and are two different strings to whatever
// enforces them.
func TestDiffPermissionReferences_VerbatimLiteralNotNormalized(t *testing.T) {
	base := snapshotWithPermissions(model.PermissionReference{Via: "@ss.hasPermi", RawLiteral: "report:view all"})
	head := snapshotWithPermissions(model.PermissionReference{Via: "@ss.hasPermi", RawLiteral: "report:view  all"})

	r := Compare(base, head)

	assertPermissionDiff(t, r,
		[]string{"@ss.hasPermi('report:view  all')"},
		[]string{"@ss.hasPermi('report:view all')"})
}

// TestDiffPermissionReferences_SameLiteralDifferentEndpoint pins that
// the key is anchored on the guard application, exactly as roleRefKey
// is: the same permission on two different endpoints is two references,
// and moving it from one to the other is a removal plus an addition.
func TestDiffPermissionReferences_SameLiteralDifferentEndpoint(t *testing.T) {
	twoEndpoints := func(guardedEndpoint model.ID) Snapshot {
		return Snapshot{Model: &model.Model{
			Endpoints: []model.Endpoint{
				{ID: "PUT /a", HTTPMethod: model.MethodPut, Path: "/a"},
				{ID: "PUT /b", HTTPMethod: model.MethodPut, Path: "/b"},
			},
			GuardApplications: []model.GuardApplication{
				{ID: "g1", EndpointID: guardedEndpoint, GuardName: "PreAuthorize", AppliedAt: model.ScopeMethod},
			},
			PermissionReferences: []model.PermissionReference{
				{ID: "perm-1", GuardApplicationID: "g1", Via: "@ss.hasPermi", RawLiteral: "x:y"},
			},
		}}
	}

	r := Compare(twoEndpoints("PUT /a"), twoEndpoints("PUT /b"))

	assertPermissionDiff(t, r,
		[]string{"@ss.hasPermi('x:y')"},
		[]string{"@ss.hasPermi('x:y')"})
}

// TestDiffPermissionReferences_OrphanedReferenceDoesNotPanic covers the
// degrade-gracefully branch in indexPermissionReferences. Measured at
// zero occurrences across the corpus's 212 references (ADR 0037's
// measurement), so this is the only thing exercising it.
func TestDiffPermissionReferences_OrphanedReferenceDoesNotPanic(t *testing.T) {
	orphan := Snapshot{Model: &model.Model{
		PermissionReferences: []model.PermissionReference{
			{ID: "perm-1", GuardApplicationID: "no-such-guard", Via: "@ss.hasPermi", RawLiteral: "x:y"},
		},
	}}

	r := Compare(orphan, orphan)

	assertPermissionDiff(t, r, nil, nil)
}

// TestDiffPermissionReferences_LosingTheGuardGatesOnce checks that the
// permission diff does not add a second gate on top of ADR 0036 case
// (c). An endpoint losing its annotation loses its guard AND its
// permission together, which is how every one of the corpus's 212
// references is attached; it must produce exactly one regression, the
// became-public one.
func TestDiffPermissionReferences_LosingTheGuardGatesOnce(t *testing.T) {
	base := snapshotWithPermissions(model.PermissionReference{Via: "@ss.hasPermi", RawLiteral: "system:user:edit"})
	head := Snapshot{Model: &model.Model{
		Endpoints: []model.Endpoint{
			{ID: "PUT /system/user", HTTPMethod: model.MethodPut, Path: "/system/user"},
		},
	}}

	r := Compare(base, head)

	assertPermissionDiff(t, r, nil, []string{"@ss.hasPermi('system:user:edit')"})
	if len(r.Regressions) != 1 {
		t.Fatalf("want exactly 1 regression, got %d: %+v", len(r.Regressions), r.Regressions)
	}
	if r.Regressions[0].Reason != ReasonBecamePublic {
		t.Errorf("reason = %q, want %q", r.Regressions[0].Reason, ReasonBecamePublic)
	}
}

// TestDiffPermissionReferences_SelfDiffIsANoOp is ADR 0037 §5's first
// regression bar at unit scale: a key that is not stable across two runs
// of the same source shows up as phantom additions and removals. On
// RuoYi-Vue and eladmin it would show up 212 times.
func TestDiffPermissionReferences_SelfDiffIsANoOp(t *testing.T) {
	s := snapshotWithPermissions(
		model.PermissionReference{Via: "@ss.hasPermi", RawLiteral: "system:user:edit", File: "a.java", Line: 1},
		model.PermissionReference{Via: "@el.check", RawLiteral: "deploy:edit", File: "a.java", Line: 9},
	)

	r := Compare(s, s)

	assertPermissionDiff(t, r, nil, nil)
	if r.HasRegressions() {
		t.Errorf("a snapshot compared with itself has no regressions, got %+v", r.Regressions)
	}
}
