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
