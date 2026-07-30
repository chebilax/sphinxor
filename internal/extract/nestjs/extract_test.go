package nestjs

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
)

// TestExtract_RealNestJSBoilerplate runs the full extraction pipeline
// against real, vendored NestJS source (see testdata/nestjs-boilerplate/NOTICE.md),
// per docs/testing.md: empirical validation against real code, not just
// synthetic fixtures. Expected values below were verified by hand against
// the vendored source, not assumed.
func TestExtract_RealNestJSBoilerplate(t *testing.T) {
	m, outcome, err := Extract("testdata/nestjs-boilerplate/src")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(outcome.AllowlistedEndpoints) != 0 {
		t.Errorf("vendored fixtures contain no sphinxor-allow markers, got %v", outcome.AllowlistedEndpoints)
	}
	if len(outcome.StaleMarkers) != 0 {
		t.Errorf("vendored fixtures contain no sphinxor-allow markers, got %d stale markers", len(outcome.StaleMarkers))
	}

	// 5 UsersController endpoints + 11 AuthController endpoints, verified
	// by hand against the vendored source.
	if len(m.Endpoints) != 16 {
		t.Fatalf("got %d endpoints, want 16: %+v", len(m.Endpoints), endpointSummaries(m.Endpoints))
	}

	// RoleEnum.admin and RoleEnum.user only — Environment (app.config.ts)
	// must be filtered out, since nothing in these files passes it to
	// @Roles().
	roleNames := make(map[string]bool, len(m.RoleDeclarations))
	for _, d := range m.RoleDeclarations {
		roleNames[d.Name] = true
	}
	if len(roleNames) != 2 || !roleNames["RoleEnum.admin"] || !roleNames["RoleEnum.user"] {
		t.Errorf("role declarations = %v, want exactly {RoleEnum.admin, RoleEnum.user}", roleNames)
	}

	endpointByPath := make(map[string]model.Endpoint, len(m.Endpoints))
	for _, e := range m.Endpoints {
		endpointByPath[string(e.HTTPMethod)+" "+e.Path] = e
	}

	refresh, ok := endpointByPath["POST /auth/refresh"]
	if !ok {
		t.Fatalf("POST /auth/refresh not found among endpoints: %v", endpointByPath)
	}
	if guards := guardNamesForEndpoint(*m, refresh.ID); len(guards) != 1 || guards[0] != "AuthGuard" {
		t.Errorf("POST /auth/refresh guards = %v, want [AuthGuard] (from AuthGuard('jwt-refresh'))", guards)
	}

	// The three v0.1 rules, run for real against this model.
	findings := runAllRules(t, m)

	mutating := findingsByRule(findings, "mutating-endpoint-without-access-control")
	wantUnguarded := map[string]bool{
		"POST /auth/email/login":       true,
		"POST /auth/email/register":    true,
		"POST /auth/email/confirm":     true,
		"POST /auth/email/confirm/new": true,
		"POST /auth/forgot/password":   true,
		"POST /auth/reset/password":    true,
	}
	if len(mutating) != len(wantUnguarded) {
		t.Errorf("got %d mutating-endpoint-without-access-control findings, want %d: %+v", len(mutating), len(wantUnguarded), mutating)
	}
	for _, f := range mutating {
		e := endpointByID(m.Endpoints, f.SubjectID)
		key := string(e.HTTPMethod) + " " + e.Path
		if !wantUnguarded[key] {
			t.Errorf("unexpected mutating-endpoint finding for %s", key)
		}
		if f.Confidence != model.ConfidenceLow {
			t.Errorf("%s: confidence = %q, want low", key, f.Confidence)
		}
	}

	unreferenced := findingsByRule(findings, "permission-declared-but-unreferenced")
	if len(unreferenced) != 1 {
		t.Fatalf("got %d permission-declared-but-unreferenced findings, want 1: %+v", len(unreferenced), unreferenced)
	}
	if got := declNameByID(m.RoleDeclarations, unreferenced[0].SubjectID); got != "RoleEnum.user" {
		t.Errorf("unreferenced permission = %q, want RoleEnum.user", got)
	}

	if empty := findingsByRule(findings, "empty-role"); len(empty) != 0 {
		t.Errorf("expected no empty-role findings in this fixture set, got %+v", empty)
	}
}

func runAllRules(t *testing.T, m *model.Model) []model.Finding {
	t.Helper()
	return lint.Run(m, lint.DefaultRules(), map[model.ID]bool{})
}

func endpointSummaries(endpoints []model.Endpoint) []string {
	out := make([]string, len(endpoints))
	for i, e := range endpoints {
		out[i] = string(e.HTTPMethod) + " " + e.Path
	}
	return out
}

func endpointByID(endpoints []model.Endpoint, id model.ID) model.Endpoint {
	for _, e := range endpoints {
		if e.ID == id {
			return e
		}
	}
	return model.Endpoint{}
}

func declNameByID(decls []model.RoleDeclaration, id model.ID) string {
	for _, d := range decls {
		if d.ID == id {
			return d.Name
		}
	}
	return ""
}

func findingsByRule(findings []model.Finding, ruleID string) []model.Finding {
	var out []model.Finding
	for _, f := range findings {
		if f.RuleID == ruleID {
			out = append(out, f)
		}
	}
	return out
}
