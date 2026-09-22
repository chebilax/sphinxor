package spring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
)

const yudaoFixture = "testdata/ruoyi-vue-pro"

// TestExtract_CollidingRouteKeepsBothEndpoints is ADR 0020 Amendment 2 §8's
// unconditional half, against the vendored yudao pair
// (testdata/ruoyi-vue-pro/NOTICE.md).
//
// Both controllers declare @RequestMapping("/member/user") literally; a
// runtime path prefix keyed on the Java package separates them, which no
// static analysis can read. Before §8, Spring dropped the second endpoint
// outright: the app-side PUT /member/user/update — which has no access
// control at all — was absent from the report, and
// mutating-endpoint-without-access-control did not fire for it.
func TestExtract_CollidingRouteKeepsBothEndpoints(t *testing.T) {
	m, _, err := Extract(yudaoFixture)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	var got []model.Endpoint
	for _, e := range m.Endpoints {
		if e.HTTPMethod == model.MethodPut && e.Path == "/member/user/update" {
			got = append(got, e)
		}
	}
	if len(got) != 2 {
		t.Fatalf("PUT /member/user/update: got %d endpoints, want 2 (admin and app sides)", len(got))
	}
	if got[0].ID == got[1].ID {
		t.Fatalf("both endpoints share ID %q — they were not kept apart", got[0].ID)
	}

	byController := make(map[string]model.Endpoint, 2)
	for _, e := range got {
		if !e.RouteCollision {
			t.Errorf("endpoint %s is not marked as a route collision", e.HandlerName)
		}
		for _, c := range m.Controllers {
			if c.ID == e.ControllerID {
				byController[c.Name] = e
			}
		}
	}

	admin, ok := byController["MemberUserController"]
	if !ok {
		t.Fatalf("admin-side endpoint missing, got %v", byController)
	}
	app, ok := byController["AppMemberUserController"]
	if !ok {
		t.Fatalf("app-side endpoint missing, got %v", byController)
	}

	// Each side keeps only its own guards.
	adminGuards := 0
	appGuards := 0
	for _, g := range m.GuardApplications {
		switch g.EndpointID {
		case admin.ID:
			adminGuards++
		case app.ID:
			appGuards++
		}
	}
	if adminGuards == 0 {
		t.Errorf("admin PUT /member/user/update lost its own @PreAuthorize")
	}
	if appGuards != 0 {
		t.Errorf("app PUT /member/user/update carries %d guard(s) it does not declare", appGuards)
	}

	// The finding the collision was hiding.
	findings := lint.Run(m, lint.DefaultRules(), nil)
	fired := false
	for _, f := range findings {
		if f.RuleID == "mutating-endpoint-without-access-control" && f.SubjectID == app.ID {
			fired = true
		}
	}
	if !fired {
		t.Errorf("mutating-endpoint-without-access-control did not fire on the unguarded app-side PUT")
	}
}

// TestExtract_CollidingRouteWithDifferingGuardsIsRecorded covers §8's
// conditional half on the side where the warning must fire: the two sides
// of this collision carry different guards, which is exactly the case where
// merging them would have reported one endpoint's protection against
// another.
func TestExtract_CollidingRouteWithDifferingGuardsIsRecorded(t *testing.T) {
	m, _, err := Extract(yudaoFixture)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	var found *model.RouteCollision
	for i, c := range m.RouteCollisions {
		if c.HTTPMethod == model.MethodPut && c.Path == "/member/user/update" {
			found = &m.RouteCollisions[i]
		}
	}
	if found == nil {
		t.Fatalf("PUT /member/user/update not recorded as a collision; got %+v", m.RouteCollisions)
	}
	if !found.GuardsDiffer {
		t.Errorf("collision not marked as guard-differing, though one side has @PreAuthorize and the other has nothing")
	}
	if len(found.Controllers) != 2 {
		t.Errorf("collision controllers = %v, want both declaring classes", found.Controllers)
	}
}

// TestExtract_NoCollisionInSingleControllerProjects pins the noise floor:
// the existing fixtures declare no route twice, so nothing may be recorded
// for them. A collision list that fills up on healthy projects is the
// failure §8's criterion exists to avoid.
func TestExtract_NoCollisionInSingleControllerProjects(t *testing.T) {
	for _, dir := range []string{"testdata/Pharmacy/backend/src/main/java", "testdata/blog-api/src/main/java"} {
		m, _, err := Extract(dir)
		if err != nil {
			t.Fatalf("Extract(%s): %v", dir, err)
		}
		if len(m.RouteCollisions) != 0 {
			t.Errorf("%s: got %d route collisions, want 0: %+v", dir, len(m.RouteCollisions), m.RouteCollisions)
		}
		for _, e := range m.Endpoints {
			if e.RouteCollision {
				t.Errorf("%s: %s %s marked as colliding", dir, e.HTTPMethod, e.Path)
			}
		}
	}
}

// TestExtract_MergedHandlerDoesNotFakeAGuardDifference is the regression
// for a defect in §8's criterion found by re-measuring the survey corpus
// after implementation, not by reasoning about it.
//
// An endpoint merged from two handlers under ADR 0014 keeps each handler's
// own annotations, so an identical @PreAuthorize can be recorded twice for
// it. Comparing guard *multisets* made that endpoint look different from a
// colliding sibling requiring exactly the same thing, and raised a warning
// with nothing behind it. Protection is a set, not a multiset.
//
// Hit in eugenp/tutorials on GET /api/authorities, which was the one
// warning the pre-implementation count had not predicted.
func TestExtract_MergedHandlerDoesNotFakeAGuardDifference(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"A.java": `package a;
import org.springframework.web.bind.annotation.*;
import org.springframework.security.access.prepost.PreAuthorize;

@RestController
@RequestMapping("/api/things")
public class AResource {
    @GetMapping(value = "", produces = "application/json")
    @PreAuthorize("hasRole('ADMIN')")
    public String asJson() { return ""; }

    @GetMapping(value = "", produces = "application/xml")
    @PreAuthorize("hasRole('ADMIN')")
    public String asXml() { return ""; }
}
`,
		"B.java": `package b;
import org.springframework.web.bind.annotation.*;
import org.springframework.security.access.prepost.PreAuthorize;

@RestController
@RequestMapping("/api/things")
public class BResource {
    @GetMapping("")
    @PreAuthorize("hasRole('ADMIN')")
    public String get() { return ""; }
}
`,
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	m, _, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(m.RouteCollisions) != 1 {
		t.Fatalf("got %d collisions, want 1: %+v", len(m.RouteCollisions), m.RouteCollisions)
	}
	if m.RouteCollisions[0].GuardsDiffer {
		t.Errorf("both sides require ROLE_ADMIN; the merged handler's duplicate annotation must not read as a difference")
	}
}

// TestExtract_CollidingRoutesDifferingOnlyByPermission is ADR 0035 §7's
// collide.go consumer, and the reason it needs its own test is that
// nothing else reaches it.
//
// guardSignatures compares what protects each side of a collision. Before
// ADR 0035 it compared guard names plus role literals; two sides both
// carrying @PreAuthorize but requiring DIFFERENT permissions signed
// identically as "PreAuthorize()" and the guard-differing warning was
// silently suppressed — reporting one endpoint's protection against
// another, which is exactly what §8's criterion exists to prevent.
//
// SYNTHETIC, and labelled as such per docs/testing.md rather than left to
// look like an oversight. No repository in the 20-project corpus and no
// vendored fixture contains this shape: ruoyi-vue-pro's colliding sides
// differ by one carrying an annotation and the other carrying none, so
// TestExtract_CollidingRouteWithDifferingGuardsIsRecorded passes with or
// without the permission term. A real fixture can reveal nothing here that
// this pair of controllers does not, because by the time guardSignatures
// runs its input is already a set of literal strings — testing.md's own
// carve-out for downstream logic whose input is framework-independent.
func TestExtract_CollidingRoutesDifferingOnlyByPermission(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"A.java": `package a;
import org.springframework.web.bind.annotation.*;
import org.springframework.security.access.prepost.PreAuthorize;

@RestController
@RequestMapping("/api/things")
public class AResource {
    @PostMapping("/update")
    @PreAuthorize("@ss.hasPermi('thing:update')")
    public String update() { return ""; }
}
`,
		"B.java": `package b;
import org.springframework.web.bind.annotation.*;
import org.springframework.security.access.prepost.PreAuthorize;

@RestController
@RequestMapping("/api/things")
public class BResource {
    @PostMapping("/update")
    @PreAuthorize("@ss.hasPermi('thing:admin')")
    public String update() { return ""; }
}
`,
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	m, _, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(m.RouteCollisions) != 1 {
		t.Fatalf("got %d collisions, want 1: %+v", len(m.RouteCollisions), m.RouteCollisions)
	}
	if !m.RouteCollisions[0].GuardsDiffer {
		t.Error("two sides requiring different permissions must read as differing protection; without permissions in the signature both sides sign as \"PreAuthorize()\" and the warning is silently lost (ADR 0035 §7)")
	}
}

// TestExtract_CollidingRoutesSharingAPermissionStayQuiet is the other side
// of the criterion, so the term added above cannot make every permission
// collision noisy. Synthetic for the same stated reason.
func TestExtract_CollidingRoutesSharingAPermissionStayQuiet(t *testing.T) {
	dir := t.TempDir()
	for name, pkg := range map[string]string{"A.java": "a", "B.java": "b"} {
		src := "package " + pkg + `;
import org.springframework.web.bind.annotation.*;
import org.springframework.security.access.prepost.PreAuthorize;

@RestController
@RequestMapping("/api/things")
public class ` + strings.ToUpper(pkg) + `Resource {
    @PostMapping("/update")
    @PreAuthorize("@ss.hasPermi('thing:update')")
    public String update() { return ""; }
}
`
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	m, _, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(m.RouteCollisions) != 1 {
		t.Fatalf("got %d collisions, want 1: %+v", len(m.RouteCollisions), m.RouteCollisions)
	}
	if m.RouteCollisions[0].GuardsDiffer {
		t.Error("both sides require the same permission; that must not read as a difference")
	}
}
