package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
)

// TestLint_Pharmacy runs internal/lint's three v0.1 rules against the real,
// fully-extracted (method + URL layer) Pharmacy model — the success
// criterion ADR 0011/0012 named: lint's rules run unchanged against
// Spring-produced output. mutating-endpoint-without-access-control and
// empty-role both needed real fixes to hold (ADR 0015, ADR 0017) — this
// test is what "run unchanged" now means concretely: no crash, and the
// findings that do fire are the ones hand-verified expected, not
// leftover NestJS assumptions leaking through.
func TestLint_Pharmacy(t *testing.T) {
	m, err := Extract("testdata/Pharmacy/backend/src/main/java")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	findings := lint.Run(m, lint.DefaultRules(), map[model.ID]bool{})

	// mutating-endpoint-without-access-control fires on exactly one
	// endpoint: POST /auth/login. Not a bug — ADR 0011 §2 / ADR 0012 §1
	// decided this explicitly: permitAll() contributes no GuardApplication
	// at all, and a framework-level "this is public" claim is deliberately
	// not treated as Sphinxor's own allowlist by inference (only an
	// explicit sphinxor-allow marker suppresses a finding, ADR 0003) — a
	// developer's permitAll() could itself be the mistake, so the audit
	// still surfaces it. Every other Pharmacy endpoint is guarded via
	// @PreAuthorize and must not be flagged.
	mutating := make(map[model.ID]bool)
	for _, f := range findings {
		if f.RuleID == "mutating-endpoint-without-access-control" {
			mutating[f.SubjectID] = true
		}
	}
	login, ok := findEndpoint(m.Endpoints, model.MethodPost, "/auth/login")
	if !ok {
		t.Fatal("missing endpoint POST /auth/login")
	}
	if !mutating[login.ID] {
		t.Error("expected mutating-endpoint-without-access-control on POST /auth/login (permitAll() is not treated as Sphinxor's own allowlist)")
	}
	if len(mutating) != 1 {
		t.Errorf("mutating-endpoint-without-access-control fired on %d endpoints, want exactly 1 (POST /auth/login): %v", len(mutating), mutating)
	}

	// empty-role must not fire: every real @PreAuthorize in Pharmacy
	// resolves at least one role (ADR 0017 keeps isAuthenticated() out of
	// this check entirely, but Pharmacy doesn't use isAuthenticated()
	// anyway — every guard here genuinely has roles).
	for _, f := range findings {
		if f.RuleID == "empty-role" {
			t.Errorf("unexpected empty-role finding: %+v", f)
		}
	}
}
