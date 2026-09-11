package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/diff"
	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
)

// TestDiff_Pharmacy_NoChangeAgainstItself sanity-checks internal/diff
// against the real, fully-extracted (method + URL layer) Pharmacy model —
// the other half of ADR 0011/0012's success criterion (lint's rules and
// diff run unchanged against Spring-produced output). Comparing the model
// to itself must report zero changes and zero regressions: confirms diff
// doesn't crash or misbehave on ScopeRequestMatcher-sourced
// GuardApplications/AuthenticationRequirements, which didn't exist in any
// real model until this PR.
func TestDiff_Pharmacy_NoChangeAgainstItself(t *testing.T) {
	m, err := Extract("testdata/Pharmacy/backend/src/main/java")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	findings := lint.Run(m, lint.DefaultRules(), map[model.ID]bool{})

	snap := diff.Snapshot{Model: m, Findings: findings}
	result := diff.Compare(snap, snap)

	if result.HasRegressions() {
		t.Errorf("comparing the same model to itself must report no regressions, got %+v", result)
	}
	if len(result.AddedEndpoints) != 0 || len(result.RemovedEndpoints) != 0 {
		t.Errorf("expected no endpoint changes, got added=%+v removed=%+v", result.AddedEndpoints, result.RemovedEndpoints)
	}
}
