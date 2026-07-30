package nestjs

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

func TestMatchFileAllowlist(t *testing.T) {
	src := `line1
// sphinxor-allow: public healthcheck
@Get('health')
health() {}

line5
// sphinxor-allow: this one matches nothing below

line8
// sphinxor-allow: separated by a blank line and a comment, still matches
// a regular comment in between
@Get('ready')
ready() {}
`
	anchors := []endpointAnchor{
		{EndpointID: "ep-health", File: "f.ts", Line: 3},
		{EndpointID: "ep-ready", File: "f.ts", Line: 12},
	}

	var nextCalls int
	next := func() model.ID {
		nextCalls++
		return model.ID("finding-x")
	}

	allowlisted, stale := matchFileAllowlist([]byte(src), "f.ts", anchors, next)

	if len(allowlisted) != 2 {
		t.Fatalf("got %d allowlisted endpoints, want 2: %v", len(allowlisted), allowlisted)
	}
	want := map[model.ID]bool{"ep-health": true, "ep-ready": true}
	for _, id := range allowlisted {
		if !want[id] {
			t.Errorf("unexpected allowlisted endpoint %q", id)
		}
	}

	if len(stale) != 1 {
		t.Fatalf("got %d stale markers, want 1: %+v", len(stale), stale)
	}
	if stale[0].RuleID != "stale-allow-marker" {
		t.Errorf("stale finding rule = %q, want stale-allow-marker", stale[0].RuleID)
	}
	if stale[0].Confidence != model.ConfidenceHigh {
		t.Errorf("stale finding confidence = %q, want high", stale[0].Confidence)
	}
	if stale[0].SubjectKind != model.SubjectAllowMarker {
		t.Errorf("stale finding subject kind = %q, want allow_marker", stale[0].SubjectKind)
	}
}

func TestMatchFileAllowlist_MarkerFollowedByOtherCode(t *testing.T) {
	src := `// sphinxor-allow: this marker sits above unrelated code
private helper() {}

@Get('health')
health() {}
`
	anchors := []endpointAnchor{
		{EndpointID: "ep-health", File: "f.ts", Line: 4},
	}
	next := func() model.ID { return "finding-x" }

	allowlisted, stale := matchFileAllowlist([]byte(src), "f.ts", anchors, next)

	if len(allowlisted) != 0 {
		t.Errorf("expected no allowlisted endpoints, got %v", allowlisted)
	}
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale marker (marker sits above unrelated code, not the endpoint further down), got %d", len(stale))
	}
}
