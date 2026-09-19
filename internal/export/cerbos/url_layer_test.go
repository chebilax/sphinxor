package cerbos

import (
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// unknownURLLayerModel is the shape the two-chain reproduction produced:
// a method layer that resolved roles, and a URL layer that exists and
// could not be read.
func unknownURLLayerModel() *model.Model {
	return &model.Model{
		Controllers: []model.Controller{{ID: "c1", Name: "ReportController"}},
		Endpoints:   []model.Endpoint{{ID: "e1", HTTPMethod: model.MethodGet, Path: "/api/reports", ControllerID: "c1"}},
		GuardApplications: []model.GuardApplication{
			{ID: "g1", EndpointID: "e1", GuardName: "PreAuthorize", AppliedAt: model.ScopeMethod, DeclaresRoles: true},
		},
		RoleReferences: []model.RoleReference{
			{ID: "r1", GuardApplicationID: "g1", RawLiteral: "ADMIN"},
			{ID: "r2", GuardApplicationID: "g1", RawLiteral: "ANALYST"},
		},
		URLLayer: model.URLLayerStatus{
			Present:  true,
			Analyzed: false,
			Reason:   "2 SecurityFilterChain beans were found",
		},
	}
}

// TestTranslate_UnknownURLLayerExportsNothing is ADR 0020 §2's regression
// test, and the one that matters most in this package: it guards a
// deployable artifact, not a report.
//
// Before the fix, a project whose URL layer couldn't be analyzed had that
// layer silently skipped, and the exporter treated the method layer as
// the complete picture. On the audit's reproduction that produced
// `roles: [ADMIN, ANALYST]` for an endpoint the running application
// restricted to ADMIN — a Cerbos grant the application itself denies,
// with nothing in the report saying so.
//
// A caveat in the companion report is not a fix here: it protects the
// reader, not the system the policy is applied to. Nothing is exported.
func TestTranslate_UnknownURLLayerExportsNothing(t *testing.T) {
	result := Translate(unknownURLLayerModel())

	if len(result.Rules) != 0 {
		t.Fatalf("exported %d rule(s) from an unknown effective policy, want 0: %+v", len(result.Rules), result.Rules)
	}
	if len(result.Omissions) != 1 {
		t.Fatalf("got %d omissions, want 1 (every endpoint is omitted, not just guarded ones): %+v",
			len(result.Omissions), result.Omissions)
	}
	if got := result.Omissions[0].Reason; got != ReasonURLLayerUnknown {
		t.Errorf("omission reason = %q, want %q", got, ReasonURLLayerUnknown)
	}
	// The reason the layer couldn't be read must reach the report, so the
	// user can tell a two-chain config from a reactive one.
	if detail := result.Omissions[0].Detail; detail == "" ||
		!strings.Contains(detail, "2 SecurityFilterChain beans were found") {
		t.Errorf("omission detail should carry the URL-layer reason, got %q", detail)
	}
}

// TestTranslate_AnalyzedURLLayerStillExports is the no-regression half:
// the fix must not make every Spring project unexportable. A project
// whose URL layer was read, or that genuinely has none, is unaffected.
func TestTranslate_AnalyzedURLLayerStillExports(t *testing.T) {
	for name, status := range map[string]model.URLLayerStatus{
		"analyzed":       {Present: true, Analyzed: true},
		"genuinely none": {},
	} {
		t.Run(name, func(t *testing.T) {
			m := unknownURLLayerModel()
			m.URLLayer = status

			result := Translate(m)
			if len(result.Rules) != 1 {
				t.Fatalf("got %d rules, want 1 — a known (or absent) URL layer must still export: %+v",
					len(result.Rules), result.Rules)
			}
		})
	}
}
