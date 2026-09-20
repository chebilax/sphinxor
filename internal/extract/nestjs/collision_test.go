package nestjs

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
)

// writeProject materializes a throwaway NestJS project for one test.
func writeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// TestExtract_CollidingRouteDoesNotMergeGuards is ADR 0020 Amendment 2 §8
// on the NestJS side, reduced from the immich shape — a second controller
// deliberately re-declaring a route for a separate worker process.
//
// immich is AGPL-3.0 and is not vendored here; this reproduces the shape
// minimally, the same way Amendment 1 §5's merge case did.
//
// Before §8 the two collapsed onto "DELETE /admin/wipe": the wide-open
// endpoint was reported carrying the other's AuthGuard and ADMIN role, and
// mutating-endpoint-without-access-control was suppressed for it.
func TestExtract_CollidingRouteDoesNotMergeGuards(t *testing.T) {
	dir := writeProject(t, map[string]string{
		"admin.controller.ts": `
import { Controller, Delete, UseGuards } from '@nestjs/common';

@Controller('admin')
@UseGuards(AuthGuard)
@Roles('ADMIN')
export class AdminController {
  @Delete('wipe')
  wipe() {}
}
`,
		"worker.controller.ts": `
import { Controller, Delete } from '@nestjs/common';

@Controller('admin')
export class MaintenanceWorkerController {
  @Delete('wipe')
  wipe() {}
}
`,
	})

	m, _, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	got := endpointsAt(m, model.MethodDelete, "/admin/wipe")
	if len(got) != 2 {
		t.Fatalf("DELETE /admin/wipe: got %d endpoints, want 2", len(got))
	}
	if got[0].ID == got[1].ID {
		t.Fatalf("both endpoints share ID %q — they were merged", got[0].ID)
	}

	byController := make(map[string]model.Endpoint, 2)
	for _, e := range got {
		if !e.RouteCollision {
			t.Errorf("%s not marked as a route collision", e.HandlerName)
		}
		for _, c := range m.Controllers {
			if c.ID == e.ControllerID {
				byController[c.Name] = e
			}
		}
	}

	worker := byController["MaintenanceWorkerController"]
	if guards := guardSet(m, worker.ID); len(guards) != 0 {
		t.Errorf("unguarded worker endpoint carries %v", guards)
	}
	admin := byController["AdminController"]
	if guards := guardSet(m, admin.ID); !guards["AuthGuard"] {
		t.Errorf("admin endpoint lost its own AuthGuard, got %v", guards)
	}

	findings := lint.Run(m, lint.DefaultRules(), nil)
	fired := false
	for _, f := range findings {
		if f.RuleID == "mutating-endpoint-without-access-control" && f.SubjectID == worker.ID {
			fired = true
		}
	}
	if !fired {
		t.Errorf("mutating-endpoint-without-access-control did not fire on the unguarded DELETE")
	}

	if len(m.RouteCollisions) != 1 {
		t.Fatalf("got %d recorded collisions, want 1: %+v", len(m.RouteCollisions), m.RouteCollisions)
	}
	if !m.RouteCollisions[0].GuardsDiffer {
		t.Errorf("collision should be marked guard-differing: one side has AuthGuard/ADMIN, the other nothing")
	}
}

// TestExtract_CollidingRouteWithMatchingGuardsStaysQuiet is §8's
// conditional half on the side that must *not* warn. Two controllers
// declaring one route with identical guards have nothing to bleed — the
// common case in the survey behind this amendment — so the endpoints are
// still kept apart, but no guard difference is recorded.
func TestExtract_CollidingRouteWithMatchingGuardsStaysQuiet(t *testing.T) {
	same := `
import { Controller, Get } from '@nestjs/common';

@Controller('health')
export class %sController {
  @Get('live')
  live() {}
}
`
	dir := writeProject(t, map[string]string{
		"api.controller.ts":    fmt.Sprintf(same, "Api"),
		"worker.controller.ts": fmt.Sprintf(same, "Worker"),
	})

	m, _, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if got := endpointsAt(m, model.MethodGet, "/health/live"); len(got) != 2 {
		t.Fatalf("GET /health/live: got %d endpoints, want 2 — both must survive regardless of the warning criterion", len(got))
	}
	if len(m.RouteCollisions) != 1 {
		t.Fatalf("got %d recorded collisions, want 1", len(m.RouteCollisions))
	}
	if m.RouteCollisions[0].GuardsDiffer {
		t.Errorf("identical guards must not be reported as differing")
	}
}

// TestExtract_SameControllerDuplicateIsNotACollision guards ADR 0014's
// deliberate merge: two handlers on one route *within one controller* are
// content negotiation, not two applications, and must keep behaving as
// they do today.
func TestExtract_SameControllerDuplicateIsNotACollision(t *testing.T) {
	dir := writeProject(t, map[string]string{
		"one.controller.ts": `
import { Controller, Get } from '@nestjs/common';

@Controller('items')
export class ItemsController {
  @Get('x')
  asJson() {}

  @Get('x')
  asXml() {}
}
`,
	})

	m, _, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(m.RouteCollisions) != 0 {
		t.Errorf("same-controller duplicate recorded as a collision: %+v", m.RouteCollisions)
	}
	for _, e := range m.Endpoints {
		if e.RouteCollision {
			t.Errorf("%s %s marked as colliding", e.HTTPMethod, e.Path)
		}
	}
}
