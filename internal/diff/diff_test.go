package diff

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

func TestDiffEndpoints_AddedAndRemoved(t *testing.T) {
	base := Snapshot{Model: &model.Model{
		Endpoints: []model.Endpoint{
			{ID: "GET /a", HTTPMethod: model.MethodGet, Path: "/a"},
			{ID: "POST /b", HTTPMethod: model.MethodPost, Path: "/b"},
		},
	}}
	head := Snapshot{Model: &model.Model{
		Endpoints: []model.Endpoint{
			{ID: "GET /a", HTTPMethod: model.MethodGet, Path: "/a"},
			{ID: "DELETE /c", HTTPMethod: model.MethodDelete, Path: "/c"},
		},
	}}

	r := Compare(base, head)

	if len(r.AddedEndpoints) != 1 || r.AddedEndpoints[0].ID != "DELETE /c" {
		t.Errorf("added = %+v, want [DELETE /c]", r.AddedEndpoints)
	}
	if len(r.RemovedEndpoints) != 1 || r.RemovedEndpoints[0].ID != "POST /b" {
		t.Errorf("removed = %+v, want [POST /b]", r.RemovedEndpoints)
	}
}

func TestDiffGuardApplications_IgnoresFromComposite(t *testing.T) {
	// The same logical guard, but resolved literally in base and via a
	// composite decorator in head — ADR 0007 §2 says this must diff as
	// unchanged, since FromComposite is an extraction-mechanism detail,
	// not a fact about the endpoint's authorization surface.
	base := Snapshot{Model: &model.Model{
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "ep1", GuardName: "AuthGuard", AppliedAt: model.ScopeMethod, FromComposite: false},
		},
	}}
	head := Snapshot{Model: &model.Model{
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "ep1", GuardName: "AuthGuard", AppliedAt: model.ScopeMethod, FromComposite: true},
		},
	}}

	r := Compare(base, head)

	if len(r.AddedGuardApplications) != 0 || len(r.RemovedGuardApplications) != 0 {
		t.Errorf("a guard flipping FromComposite should diff as unchanged, got added=%+v removed=%+v", r.AddedGuardApplications, r.RemovedGuardApplications)
	}
}

func TestDiffRoleReferences_NormalizesWhitespaceForKeying(t *testing.T) {
	// A pure reformat (e.g. Prettier) of a fallback-case RawLiteral
	// shouldn't diff as removed-then-added — see normalizeRawLiteral.
	base := Snapshot{Model: &model.Model{
		GuardApplications: []model.GuardApplication{{ID: "g1", EndpointID: "ep1", GuardName: "Roles"}},
		RoleReferences: []model.RoleReference{
			{ID: "r1", GuardApplicationID: "g1", RawLiteral: "condition(\n  x,\n  y,\n)"},
		},
	}}
	head := Snapshot{Model: &model.Model{
		GuardApplications: []model.GuardApplication{{ID: "g1", EndpointID: "ep1", GuardName: "Roles"}},
		RoleReferences: []model.RoleReference{
			{ID: "r1", GuardApplicationID: "g1", RawLiteral: "condition( x, y, )"},
		},
	}}

	r := Compare(base, head)

	if len(r.AddedRoleReferences) != 0 || len(r.RemovedRoleReferences) != 0 {
		t.Errorf("whitespace-only reformat should diff as unchanged, got added=%+v removed=%+v", r.AddedRoleReferences, r.RemovedRoleReferences)
	}
}

func TestBecamePublic(t *testing.T) {
	base := Snapshot{Model: &model.Model{
		Endpoints:         []model.Endpoint{{ID: "POST /a", HTTPMethod: model.MethodPost, Path: "/a"}},
		GuardApplications: []model.GuardApplication{{ID: "g1", EndpointID: "POST /a", GuardName: "AuthGuard"}},
	}}
	head := Snapshot{Model: &model.Model{
		Endpoints:         []model.Endpoint{{ID: "POST /a", HTTPMethod: model.MethodPost, Path: "/a"}},
		GuardApplications: nil, // guard removed
	}}

	r := Compare(base, head)

	if len(r.BecamePublic) != 1 || r.BecamePublic[0].ID != "POST /a" {
		t.Errorf("BecamePublic = %+v, want [POST /a]", r.BecamePublic)
	}
}

func TestBecamePublic_RemovedEndpointIsNotBecamePublic(t *testing.T) {
	base := Snapshot{Model: &model.Model{
		Endpoints:         []model.Endpoint{{ID: "POST /a", HTTPMethod: model.MethodPost, Path: "/a"}},
		GuardApplications: []model.GuardApplication{{ID: "g1", EndpointID: "POST /a", GuardName: "AuthGuard"}},
	}}
	head := Snapshot{Model: &model.Model{}} // endpoint removed entirely

	r := Compare(base, head)

	if len(r.BecamePublic) != 0 {
		t.Errorf("a removed endpoint must not appear in BecamePublic, got %+v", r.BecamePublic)
	}
	if len(r.RemovedEndpoints) != 1 {
		t.Errorf("expected the endpoint in RemovedEndpoints instead, got %+v", r.RemovedEndpoints)
	}
}

func TestDiffRegressions_NewHighConfidenceFinding(t *testing.T) {
	base := Snapshot{Model: &model.Model{}, Findings: nil}
	head := Snapshot{Model: &model.Model{}, Findings: []model.Finding{
		{RuleID: "empty-role", Confidence: model.ConfidenceHigh, SubjectID: "ep1", SubjectKind: model.SubjectEndpoint},
	}}

	r := Compare(base, head)

	if len(r.Regressions) != 1 || r.Regressions[0].Reason != ReasonNew {
		t.Fatalf("regressions = %+v, want one ReasonNew", r.Regressions)
	}
}

func TestDiffRegressions_PersistingFindingIsNotARegression(t *testing.T) {
	f := model.Finding{RuleID: "empty-role", Confidence: model.ConfidenceHigh, SubjectID: "ep1", SubjectKind: model.SubjectEndpoint}
	base := Snapshot{Model: &model.Model{}, Findings: []model.Finding{f}}
	head := Snapshot{Model: &model.Model{}, Findings: []model.Finding{f}}

	r := Compare(base, head)

	if len(r.Regressions) != 0 {
		t.Errorf("an unchanged High-confidence finding must not be re-reported as a new regression, got %+v", r.Regressions)
	}
}

func TestDiffRegressions_LowConfidenceNeverGates(t *testing.T) {
	base := Snapshot{Model: &model.Model{}, Findings: nil}
	head := Snapshot{Model: &model.Model{}, Findings: []model.Finding{
		{RuleID: "mutating-endpoint-without-access-control", Confidence: model.ConfidenceLow, SubjectID: "ep1", SubjectKind: model.SubjectEndpoint},
	}}

	r := Compare(base, head)

	if len(r.Regressions) != 0 {
		t.Errorf("a new Low-confidence finding must never gate, got %+v", r.Regressions)
	}
}

// TestDiffRegressions_LowToHighTransition is the test ADR 0007 §3
// explicitly requires: if base has a Low-confidence finding at a
// subject+rule and head has a High-confidence finding at the SAME
// subject+rule, that must register as a regression — the match set is
// base's High-confidence findings only, not "any base finding regardless
// of confidence". Constructed directly via model.Finding literals since
// no current rule varies its own confidence per instance; the matching
// logic still has to be correct on its own terms, not correct by
// accident because nothing exercises this today.
func TestDiffRegressions_LowToHighTransition(t *testing.T) {
	base := Snapshot{Model: &model.Model{}, Findings: []model.Finding{
		{RuleID: "some-rule", Confidence: model.ConfidenceLow, SubjectID: "ep1", SubjectKind: model.SubjectEndpoint},
	}}
	head := Snapshot{Model: &model.Model{}, Findings: []model.Finding{
		{RuleID: "some-rule", Confidence: model.ConfidenceHigh, SubjectID: "ep1", SubjectKind: model.SubjectEndpoint},
	}}

	r := Compare(base, head)

	if len(r.Regressions) != 1 {
		t.Fatalf("got %d regressions, want 1 — a Low-in-base/High-in-head finding at the same subject must gate, not be silently matched against its Low ancestor", len(r.Regressions))
	}
	if r.Regressions[0].Reason != ReasonNew {
		t.Errorf("reason = %q, want %q", r.Regressions[0].Reason, ReasonNew)
	}
}

func TestDiffRegressions_AllowlistRemoved(t *testing.T) {
	base := Snapshot{Model: &model.Model{}, Findings: []model.Finding{
		{RuleID: "empty-role", Confidence: model.ConfidenceHigh, SubjectID: "ep1", SubjectKind: model.SubjectEndpoint, Allowlisted: true},
	}}
	head := Snapshot{Model: &model.Model{}, Findings: []model.Finding{
		{RuleID: "empty-role", Confidence: model.ConfidenceHigh, SubjectID: "ep1", SubjectKind: model.SubjectEndpoint, Allowlisted: false},
	}}

	r := Compare(base, head)

	if len(r.Regressions) != 1 || r.Regressions[0].Reason != ReasonAllowlistRemoved {
		t.Fatalf("regressions = %+v, want one ReasonAllowlistRemoved", r.Regressions)
	}
}

func TestDiffRegressions_StillAllowlistedIsNotARegression(t *testing.T) {
	base := Snapshot{Model: &model.Model{}, Findings: []model.Finding{
		{RuleID: "empty-role", Confidence: model.ConfidenceHigh, SubjectID: "ep1", SubjectKind: model.SubjectEndpoint, Allowlisted: true},
	}}
	head := Snapshot{Model: &model.Model{}, Findings: []model.Finding{
		{RuleID: "empty-role", Confidence: model.ConfidenceHigh, SubjectID: "ep1", SubjectKind: model.SubjectEndpoint, Allowlisted: true},
	}}

	r := Compare(base, head)

	if len(r.Regressions) != 0 {
		t.Errorf("a finding that stays allowlisted on both sides must not gate, got %+v", r.Regressions)
	}
}

func TestSubjectKey_RoleDeclarationUsesStableName(t *testing.T) {
	// A permission-declared-but-unreferenced finding's SubjectID is a
	// per-run sequential RoleDeclaration.ID ("role-1", "role-2", ...) —
	// not stable across runs. The diff must key on the declaration's
	// Name instead, or every such finding would spuriously look "new"
	// on every single diff, permanently, since the raw SubjectID would
	// never match between two independent extraction runs.
	base := Snapshot{
		Model: &model.Model{RoleDeclarations: []model.RoleDeclaration{{ID: "role-7", Name: "RoleEnum.user"}}},
		Findings: []model.Finding{
			{RuleID: "permission-declared-but-unreferenced", Confidence: model.ConfidenceHigh, SubjectID: "role-7", SubjectKind: model.SubjectRoleDeclaration},
		},
	}
	head := Snapshot{
		// Same role, different sequential ID — exactly what a second,
		// independent extraction run produces.
		Model: &model.Model{RoleDeclarations: []model.RoleDeclaration{{ID: "role-3", Name: "RoleEnum.user"}}},
		Findings: []model.Finding{
			{RuleID: "permission-declared-but-unreferenced", Confidence: model.ConfidenceHigh, SubjectID: "role-3", SubjectKind: model.SubjectRoleDeclaration},
		},
	}

	r := Compare(base, head)

	if len(r.Regressions) != 0 {
		t.Errorf("the same role, found again under a different sequential ID, must not appear as a new regression, got %+v", r.Regressions)
	}
}
