package nestjs

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// TestExtract_RealAwesomeNestBoilerplate runs the full extraction pipeline
// against a second real, vendored NestJS project (see
// testdata/awesome-nest-boilerplate/NOTICE.md), chosen specifically for a
// different guard style than testdata/nestjs-boilerplate/: a custom
// composite @Auth([...]) decorator instead of separate
// @UseGuards()/@Roles() decorators.
//
// This is expected to — and does — demonstrate the known blind spot
// documented in internal/lint/mutating_endpoint.go: POST /posts is
// genuinely guarded via @Auth(), but that's invisible to this extractor,
// so it's flagged anyway, at Low confidence. Expected values below were
// verified by hand against the vendored source and app.module.ts's empty
// providers list (no global APP_GUARD), not assumed.
func TestExtract_RealAwesomeNestBoilerplate(t *testing.T) {
	m, outcome, err := Extract("testdata/awesome-nest-boilerplate/src")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(outcome.AllowlistedEndpoints) != 0 || len(outcome.StaleMarkers) != 0 {
		t.Errorf("vendored fixtures contain no sphinxor-allow markers, got %+v", outcome)
	}

	// 3 AuthController endpoints + 5 PostController endpoints.
	if len(m.Endpoints) != 8 {
		t.Fatalf("got %d endpoints, want 8: %v", len(m.Endpoints), endpointSummaries(m.Endpoints))
	}

	// Neither vendored file contains a literal @Roles() decorator call —
	// only the custom @Auth() wrapper, which this extractor does not
	// unwrap — so no role declarations should be extracted at all, even
	// though RoleType is a real role enum in the app.
	if len(m.RoleDeclarations) != 0 {
		t.Errorf("expected zero role declarations (RoleType is never referenced via a literal @Roles()), got %+v", m.RoleDeclarations)
	}
	if len(m.RoleReferences) != 0 {
		t.Errorf("expected zero role references, got %+v", m.RoleReferences)
	}

	// No GuardApplication at all should exist for POST /posts — this is
	// the blind spot, not a bug: @Auth([RoleType.USER]) is genuinely
	// present in source but this extractor doesn't recognize it as a
	// guard or role decorator.
	endpointByPath := make(map[string]model.Endpoint, len(m.Endpoints))
	for _, e := range m.Endpoints {
		endpointByPath[string(e.HTTPMethod)+" "+e.Path] = e
	}
	createPost, ok := endpointByPath["POST /posts"]
	if !ok {
		t.Fatalf("POST /posts not found: %v", endpointByPath)
	}
	if guards := guardNamesForEndpoint(*m, createPost.ID); len(guards) != 0 {
		t.Errorf("POST /posts: got guards %v, want none (this is the documented @Auth() blind spot)", guards)
	}

	findings := runAllRules(t, m)

	mutating := findingsByRule(findings, "mutating-endpoint-without-access-control")
	wantFlagged := map[string]bool{
		"POST /auth/login":    true, // genuinely unguarded in source
		"POST /auth/register": true, // genuinely unguarded in source
		"POST /posts":         true, // guarded via @Auth(), invisible to this extractor — the blind spot
		"PUT /posts/:id":      true, // genuinely unguarded in source
		"DELETE /posts/:id":   true, // genuinely unguarded in source
	}
	if len(mutating) != len(wantFlagged) {
		t.Errorf("got %d mutating-endpoint-without-access-control findings, want %d: %+v", len(mutating), len(wantFlagged), mutating)
	}
	for _, f := range mutating {
		e := endpointByID(m.Endpoints, f.SubjectID)
		key := string(e.HTTPMethod) + " " + e.Path
		if !wantFlagged[key] {
			t.Errorf("unexpected mutating-endpoint finding for %s", key)
		}
		if f.Confidence != model.ConfidenceLow {
			t.Errorf("%s: confidence = %q, want low", key, f.Confidence)
		}
	}

	if unreferenced := findingsByRule(findings, "permission-declared-but-unreferenced"); len(unreferenced) != 0 {
		t.Errorf("expected no permission-declared-but-unreferenced findings (no role declarations exist to check), got %+v", unreferenced)
	}
	if empty := findingsByRule(findings, "empty-role"); len(empty) != 0 {
		t.Errorf("expected no empty-role findings (no literal @Roles() decorator in this fixture set), got %+v", empty)
	}
}
