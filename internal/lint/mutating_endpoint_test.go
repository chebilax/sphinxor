package lint

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

func TestMutatingEndpointWithoutAccessControl(t *testing.T) {
	m := &model.Model{
		Endpoints: []model.Endpoint{
			{ID: "get", HTTPMethod: model.MethodGet, Path: "/things"},
			{ID: "post-unguarded", HTTPMethod: model.MethodPost, Path: "/things"},
			{ID: "post-guarded", HTTPMethod: model.MethodPost, Path: "/things/guarded"},
			{ID: "delete-unguarded", HTTPMethod: model.MethodDelete, Path: "/things/:id"},
		},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "post-guarded", GuardName: "AuthGuard"},
		},
	}

	findings := MutatingEndpointWithoutAccessControl{}.Check(m)

	flagged := make(map[model.ID]bool, len(findings))
	for _, f := range findings {
		flagged[f.SubjectID] = true
		if f.Confidence != model.ConfidenceLow {
			t.Errorf("finding for %s: confidence = %q, want low", f.SubjectID, f.Confidence)
		}
		if f.SubjectKind != model.SubjectEndpoint {
			t.Errorf("finding for %s: subject kind = %q, want endpoint", f.SubjectID, f.SubjectKind)
		}
	}

	if !flagged["post-unguarded"] || !flagged["delete-unguarded"] {
		t.Errorf("expected post-unguarded and delete-unguarded to be flagged, got %v", flagged)
	}
	if flagged["get"] {
		t.Errorf("GET should never be flagged (not a mutating method)")
	}
	if flagged["post-guarded"] {
		t.Errorf("post-guarded has a GuardApplication, should not be flagged")
	}
	if len(findings) != 2 {
		t.Errorf("got %d findings, want 2", len(findings))
	}
}

// TestMutatingEndpointWithoutAccessControl_ConfirmedInert covers ADR
// 0015's fix: a real annotation is present (a GuardApplication exists),
// but its family is confirmed not enabled project-wide, so it must not
// count as protection. Synthetic (docs/testing.md's own stated principle):
// neither vendored Spring fixture combines a real @Secured/@RolesAllowed
// on a controller method with a confirmed-inert family, so this is the
// pure-logic half of ADR 0015 — MethodSecurityStatus itself is
// real-fixture-extracted (internal/extract/spring/extract_test.go), this
// rule's consumption of it is a pure function of that already-verified
// input.
func TestMutatingEndpointWithoutAccessControl_ConfirmedInert(t *testing.T) {
	m := &model.Model{
		Endpoints: []model.Endpoint{
			{ID: "secured-inert", HTTPMethod: model.MethodPost, Path: "/a"},
			{ID: "secured-live", HTTPMethod: model.MethodPost, Path: "/b"},
			{ID: "unknown-status", HTTPMethod: model.MethodPost, Path: "/c"},
			{ID: "prepost-inert-but-also-secured-live", HTTPMethod: model.MethodPost, Path: "/d"},
		},
		GuardApplications: []model.GuardApplication{
			// securedEnabled confirmed false project-wide: inert, must be flagged.
			{ID: "g1", EndpointID: "secured-inert", GuardName: "Secured", DeclaresRoles: true},
			// jsr250Enabled confirmed true project-wide: live, must not be flagged.
			{ID: "g2", EndpointID: "secured-live", GuardName: "RolesAllowed", DeclaresRoles: true},
			// MethodSecurity.Found == false ("no evidence either way") must
			// never be treated as confirmed-inert — this endpoint keeps
			// today's behavior (guard presence alone is enough).
			{ID: "g3", EndpointID: "unknown-status", GuardName: "PreAuthorize", DeclaresRoles: true},
			// One inert guard (PreAuthorize) and one live guard (RolesAllowed)
			// on the same endpoint: the live one must still count.
			{ID: "g4", EndpointID: "prepost-inert-but-also-secured-live", GuardName: "PreAuthorize", DeclaresRoles: true},
			{ID: "g5", EndpointID: "prepost-inert-but-also-secured-live", GuardName: "RolesAllowed", DeclaresRoles: true},
		},
		MethodSecurity: model.MethodSecurityStatus{
			Found:          true,
			PrePostEnabled: false, // e.g. @EnableMethodSecurity(prePostEnabled = false)
			SecuredEnabled: false, // never turned on
			Jsr250Enabled:  true,  // explicitly turned on
		},
	}

	// unknown-status has MethodSecurity.Found == false semantics tested
	// separately below, since Found is a single project-wide field — this
	// model's MethodSecurity.Found is true, so g3 (PreAuthorize) IS
	// confirmed inert here (PrePostEnabled: false) and unknown-status
	// should be flagged too, for the reason stated on g3 above, not "unknown".
	findings := MutatingEndpointWithoutAccessControl{}.Check(m)
	flagged := make(map[model.ID]bool, len(findings))
	for _, f := range findings {
		flagged[f.SubjectID] = true
	}

	if !flagged["secured-inert"] {
		t.Error("secured-inert: Secured is confirmed not enabled (SecuredEnabled: false), should be flagged as unguarded")
	}
	if flagged["secured-live"] {
		t.Error("secured-live: RolesAllowed is confirmed enabled (Jsr250Enabled: true), should not be flagged")
	}
	if !flagged["unknown-status"] {
		t.Error("unknown-status: PreAuthorize is confirmed inert here (PrePostEnabled: false), should be flagged")
	}
	if flagged["prepost-inert-but-also-secured-live"] {
		t.Error("prepost-inert-but-also-secured-live: the live RolesAllowed guard must still count even though the PreAuthorize one is inert")
	}
}

// TestMutatingEndpointWithoutAccessControl_UnknownStatusNeverDowngrades
// isolates the sharp boundary ADR 0015 depends on: MethodSecurity.Found ==
// false ("no evidence either way") must never be treated the same as a
// confirmed-false flag — that would create the exact symmetric false
// positive the ADR was written to avoid (a real, live guard Sphinxor's
// static view didn't reach, misreported as unguarded).
func TestMutatingEndpointWithoutAccessControl_UnknownStatusNeverDowngrades(t *testing.T) {
	m := &model.Model{
		Endpoints: []model.Endpoint{
			{ID: "e1", HTTPMethod: model.MethodPost, Path: "/a"},
		},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "e1", GuardName: "Secured", DeclaresRoles: true},
		},
		MethodSecurity: model.MethodSecurityStatus{Found: false},
	}

	findings := MutatingEndpointWithoutAccessControl{}.Check(m)
	for _, f := range findings {
		if f.SubjectID == "e1" {
			t.Errorf("MethodSecurity.Found == false must not downgrade a guard to unguarded, got finding: %+v", f)
		}
	}
}
