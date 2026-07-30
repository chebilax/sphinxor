package lint

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

func TestRun_AssignsIDsAndMarksAllowlisted(t *testing.T) {
	m := &model.Model{
		Endpoints: []model.Endpoint{
			{ID: "ep-a", HTTPMethod: model.MethodPost, Path: "/a"},
			{ID: "ep-b", HTTPMethod: model.MethodPost, Path: "/b"},
		},
	}

	allowlisted := map[model.ID]bool{"ep-a": true}

	findings := Run(m, []Rule{MutatingEndpointWithoutAccessControl{}}, allowlisted)

	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2", len(findings))
	}

	seenIDs := map[model.ID]bool{}
	for _, f := range findings {
		if f.ID == "" {
			t.Errorf("finding for %s has no ID", f.SubjectID)
		}
		if seenIDs[f.ID] {
			t.Errorf("duplicate finding ID %q", f.ID)
		}
		seenIDs[f.ID] = true

		switch f.SubjectID {
		case "ep-a":
			if !f.Allowlisted {
				t.Errorf("ep-a finding should be marked Allowlisted")
			}
		case "ep-b":
			if f.Allowlisted {
				t.Errorf("ep-b finding should not be marked Allowlisted")
			}
		}
	}
}
