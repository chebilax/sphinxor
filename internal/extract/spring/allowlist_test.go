package spring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
)

// copyTree copies a real vendored fixture into dst so a test can apply its
// own edit — inserting a sphinxor-allow marker — without touching the
// vendored source every other test reads. Mirrors internal/diff's
// real_world_test.go helper of the same name.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copying %s to %s: %v", src, dst, err)
	}
}

func mustReplace(t *testing.T, path, old, new string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, old) {
		t.Fatalf("expected %q to contain %q — the vendored fixture may have changed", path, old)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(content, old, new, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

const pharmacySrc = "testdata/Pharmacy/backend/src/main/java"
const authControllerRel = "haru/pharmacy/controller/AuthController.java"

// TestAllowlist_Pharmacy_SuppressesRealFinding is the case that made the
// Spring allowlist a release prerequisite (docs/decisions/0019-cli-framework-selection.md):
// Pharmacy's POST /auth/login is deliberately public (SecurityConfig maps
// /auth/** to permitAll()), and ADR 0011 §2 / ADR 0012 §1 decided on
// purpose that a framework-level permitAll() is NOT treated as Sphinxor's
// own allowlist — so lint really does flag it (TestLint_Pharmacy asserts
// exactly that). Before this port, a Spring user had no way to say "yes,
// on purpose."
//
// The marker goes above the real @PostMapping("/login") handler in a copy
// of the real fixture, and the assertion is on lint's actual output, not
// on the extraction internals: the finding must still be produced, but
// marked Allowlisted, exactly as ADR 0003 specifies (a marker suppresses,
// it does not erase).
func TestAllowlist_Pharmacy_SuppressesRealFinding(t *testing.T) {
	dir := t.TempDir()
	copyTree(t, pharmacySrc, dir)

	mustReplace(t, filepath.Join(dir, authControllerRel),
		"    @PostMapping(\"/login\")",
		"    // sphinxor-allow: login is public by design, permitAll() in SecurityConfig\n    @PostMapping(\"/login\")")

	m, outcome, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	login, ok := findEndpoint(m.Endpoints, model.MethodPost, "/auth/login")
	if !ok {
		t.Fatal("missing endpoint POST /auth/login")
	}
	if !outcome.AllowlistedEndpoints[login.ID] {
		t.Fatalf("POST /auth/login was not allowlisted; outcome = %+v", outcome.AllowlistedEndpoints)
	}
	if len(outcome.StaleMarkers) != 0 {
		t.Errorf("expected no stale markers, got %+v", outcome.StaleMarkers)
	}

	findings := lint.Run(m, lint.DefaultRules(), outcome.AllowlistedEndpoints)

	var found bool
	for _, f := range findings {
		if f.RuleID != "mutating-endpoint-without-access-control" || f.SubjectID != login.ID {
			continue
		}
		found = true
		if !f.Allowlisted {
			t.Errorf("POST /auth/login finding is not marked Allowlisted: %+v", f)
		}
	}
	if !found {
		t.Error("expected the mutating-endpoint finding on POST /auth/login to still be produced (allowlisting suppresses, it does not erase)")
	}
}

// TestAllowlist_Pharmacy_MisplacedMarkerIsStale covers the other half of
// ADR 0003: a marker that doesn't sit directly above a recognized endpoint
// is reported, not silently ignored. Placed above a field declaration in
// the same real controller, so the file is otherwise untouched.
func TestAllowlist_Pharmacy_MisplacedMarkerIsStale(t *testing.T) {
	dir := t.TempDir()
	copyTree(t, pharmacySrc, dir)

	mustReplace(t, filepath.Join(dir, authControllerRel),
		"    private final AuthService authService;",
		"    // sphinxor-allow: this sits above a field, not an endpoint\n    private final AuthService authService;")

	m, outcome, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(outcome.AllowlistedEndpoints) != 0 {
		t.Errorf("expected nothing allowlisted, got %+v", outcome.AllowlistedEndpoints)
	}
	if len(outcome.StaleMarkers) != 1 {
		t.Fatalf("expected exactly 1 stale-allow-marker finding, got %+v", outcome.StaleMarkers)
	}
	stale := outcome.StaleMarkers[0]
	if stale.RuleID != "stale-allow-marker" {
		t.Errorf("rule = %q, want stale-allow-marker", stale.RuleID)
	}
	if stale.Confidence != model.ConfidenceHigh {
		t.Errorf("confidence = %q, want high (a stale marker is a syntactic fact, ADR 0003)", stale.Confidence)
	}
	if stale.SubjectKind != model.SubjectAllowMarker {
		t.Errorf("subject kind = %q, want allow_marker", stale.SubjectKind)
	}

	// The unsuppressed finding must still be there: a stale marker exempts
	// nothing.
	login, ok := findEndpoint(m.Endpoints, model.MethodPost, "/auth/login")
	if !ok {
		t.Fatal("missing endpoint POST /auth/login")
	}
	findings := lint.Run(m, lint.DefaultRules(), outcome.AllowlistedEndpoints)
	for _, f := range findings {
		if f.RuleID == "mutating-endpoint-without-access-control" && f.SubjectID == login.ID && f.Allowlisted {
			t.Error("POST /auth/login must not be allowlisted by a marker that matched nothing")
		}
	}
}

// TestAllowlist_Pharmacy_UnmarkedFixtureUnchanged guards the refactor
// itself: the real vendored fixture carries no markers, so extracting it
// untouched must allowlist nothing and report no stale markers. Without
// this, a matcher bug that allowlisted everything would pass the two tests
// above.
func TestAllowlist_Pharmacy_UnmarkedFixtureUnchanged(t *testing.T) {
	_, outcome, err := Extract(pharmacySrc)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(outcome.AllowlistedEndpoints) != 0 {
		t.Errorf("vendored fixture has no markers; expected nothing allowlisted, got %+v", outcome.AllowlistedEndpoints)
	}
	if len(outcome.StaleMarkers) != 0 {
		t.Errorf("vendored fixture has no markers; expected no stale findings, got %+v", outcome.StaleMarkers)
	}
}
