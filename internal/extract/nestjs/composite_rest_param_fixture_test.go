package nestjs

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// TestExtract_CompositeWithRestParameter is the pipeline-level regression
// check for ADR 0006 Amendment 1: a composite whose own signature takes a
// rest parameter is resolved, and the guards it wraps reach the endpoints
// that use it.
//
// Its fixture is a REDUCTION, not a vendored copy, which is a deliberate
// departure from docs/testing.md's real-fixture bar — stated here in the
// test's own comment as that document requires, rather than taken
// silently. The reason is licensing, not convenience: the shape comes
// from ghostfolio, which is AGPL-3.0, while this repository and every
// other fixture in it are permissive. See
// testdata/ghostfolio-shape/NOTICE.md.
//
// The empirical half of the bar was met before the departure was taken.
// The fix was validated against the real repository first — 118
// endpoints, guards recognized on 78 before and 105 after, and
// mutating-endpoint-without-access-control findings 13 before and 0
// after, all 13 of them false positives — with no change to any of the
// other ten repositories in the same survey. Those numbers are recorded
// in ADR 0006 Amendment 1. What this fixture adds is a deterministic,
// offline anchor so the one-character parser bug cannot come back
// unnoticed.
func TestExtract_CompositeWithRestParameter(t *testing.T) {
	m, _, err := Extract("testdata/ghostfolio-shape/src")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(m.Endpoints) != 4 {
		t.Fatalf("got %d endpoints, want 4: %v", len(m.Endpoints), endpointSummaries(m.Endpoints))
	}

	guardsByEndpoint := make(map[model.ID][]string, len(m.Endpoints))
	for _, g := range m.GuardApplications {
		guardsByEndpoint[g.EndpointID] = append(guardsByEndpoint[g.EndpointID], g.GuardName)
		if !g.FromComposite {
			t.Errorf("guard %q came from a composite and must be marked FromComposite", g.GuardName)
		}
	}

	// The four guards RequiresScope wraps, in declaration order:
	// AuthGuard('jwt') is a call-form guard, the other three are plain
	// identifiers. Both forms must resolve through the composite.
	want := []string{"AuthGuard", "HasPermissionGuard", "ImpersonationGuard", "ScopeGuard"}

	byHandler := make(map[string]model.Endpoint, len(m.Endpoints))
	for _, e := range m.Endpoints {
		byHandler[e.HandlerName] = e
	}

	for _, handler := range []string{"createWatchlistItem", "deleteWatchlistItem", "getWatchlistItems"} {
		e, ok := byHandler[handler]
		if !ok {
			t.Fatalf("endpoint %s missing: %v", handler, endpointSummaries(m.Endpoints))
		}
		got := guardsByEndpoint[e.ID]
		if len(got) != len(want) {
			t.Errorf("%s: got guards %v, want %v — the rest parameter must not disqualify the composite",
				handler, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: got guards %v, want %v", handler, got, want)
				break
			}
		}
	}

	// The undecorated sibling proves the fixture can still tell the two
	// cases apart: recognizing the composite must not make every endpoint
	// in the file look guarded.
	bare, ok := byHandler["importWatchlist"]
	if !ok {
		t.Fatalf("endpoint importWatchlist missing: %v", endpointSummaries(m.Endpoints))
	}
	if got := guardsByEndpoint[bare.ID]; len(got) != 0 {
		t.Errorf("importWatchlist carries no decorator and must have no guards, got %v", got)
	}

	// The scopes travel as SetMetadata, which this extractor does not
	// read, and the rest parameter is never passed to Roles(). Pinning
	// zero role references states the boundary of what the amendment
	// fixed: the guards became visible, the scopes did not — which is the
	// permissions-as-metadata gap recorded in docs/limitations.md, not
	// something this fix claims to address.
	if len(m.RoleReferences) != 0 {
		t.Errorf("got %d role references, want 0: the scopes are SetMetadata, not Roles()", len(m.RoleReferences))
	}
}
