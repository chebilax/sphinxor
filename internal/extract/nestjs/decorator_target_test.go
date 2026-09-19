package nestjs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// writeTSProject lays out a minimal NestJS source tree in a temp dir.
func writeTSProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, "src", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// endpointNamed finds an endpoint by method and path.
func endpointNamed(m *model.Model, method model.HTTPMethod, path string) (model.Endpoint, bool) {
	for _, e := range m.Endpoints {
		if e.HTTPMethod == method && e.Path == path {
			return e, true
		}
	}
	return model.Endpoint{}, false
}

// guardNamesFor returns the guard names recorded against an endpoint.
func guardNamesFor(m *model.Model, id model.ID) []string {
	var out []string
	for _, g := range m.GuardApplications {
		if g.EndpointID == id {
			out = append(out, g.GuardName)
		}
	}
	return out
}

// TestCommentDoesNotAbsorbDecorators is ADR 0020 Amendment 1 §6's
// regression test.
//
// groupDecorators attached a run of decorators to "the next non-decorator
// node", and in tree-sitter-typescript a `comment` is a *named* sibling —
// so a comment sat in the slot where the decorated declaration belonged.
// The decorators were attached to the comment and the declaration got
// none, which did not degrade the endpoint's data: it removed the
// endpoint from the model entirely. No row, and therefore no lint rule to
// fire, on routes that included unguarded POST/PUT/DELETE handlers.
//
// All three shapes are covered because they were found in that order and
// only the first was obvious; the @Controller one costs a whole
// controller, which is the most expensive of the three.
func TestCommentDoesNotAbsorbDecorators(t *testing.T) {
	t.Run("trailing comment on a route decorator", func(t *testing.T) {
		m := extractTS(t, `import { Controller, Post } from '@nestjs/common';
@Controller('crud')
export class CrudController {
  @Post('create') // POST http://localhost:3000/test/crud
  public create(): void {}
}
`)
		if _, ok := endpointNamed(m, model.MethodPost, "/crud/create"); !ok {
			t.Fatalf("POST /crud/create is missing — a comment must not delete an endpoint; got %+v", m.Endpoints)
		}
	})

	t.Run("trailing comment on the controller decorator", func(t *testing.T) {
		// The expensive one: every route in the class disappears at once.
		m := extractTS(t, `import { Controller, Get, Delete } from '@nestjs/common';
@Controller('crud') // route /test/crud/*
export class CrudController {
  @Get(':id')
  public read(): void {}

  @Delete(':id')
  public remove(): void {}
}
`)
		if len(m.Endpoints) != 2 {
			t.Fatalf("got %d endpoint(s), want 2 — a comment on @Controller must not delete the whole controller: %+v",
				len(m.Endpoints), m.Endpoints)
		}
	})

	t.Run("comment between a guard and the route decorator", func(t *testing.T) {
		// Here the endpoint survives but its protection does not, which
		// reports a guarded route as unguarded.
		m := extractTS(t, `import { Controller, Post, UseGuards } from '@nestjs/common';
@Controller('a')
export class C {
  @UseGuards(AuthGuard)
  // TODO: tighten this
  @Post('x')
  public x(): void {}
}
`)
		e, ok := endpointNamed(m, model.MethodPost, "/a/x")
		if !ok {
			t.Fatalf("POST /a/x is missing: %+v", m.Endpoints)
		}
		guards := guardNamesFor(m, e.ID)
		if len(guards) != 1 || guards[0] != "AuthGuard" {
			t.Errorf("guards = %v, want [AuthGuard] — decorators before an interleaved comment must not be dropped", guards)
		}
	})

	t.Run("comment before the decorators still works", func(t *testing.T) {
		// The ordinary documented-handler shape, which already worked and
		// must keep working: the fix must skip comments looking forward,
		// not swallow the ones that legitimately precede a run.
		m := extractTS(t, `import { Controller, Get } from '@nestjs/common';
@Controller('a')
export class C {
  // returns the thing
  @Get('x')
  public x(): void {}
}
`)
		if _, ok := endpointNamed(m, model.MethodGet, "/a/x"); !ok {
			t.Fatalf("GET /a/x is missing: %+v", m.Endpoints)
		}
	})
}

func extractTS(t *testing.T, src string) *model.Model {
	t.Helper()
	dir := writeTSProject(t, map[string]string{"c.controller.ts": src})
	m, _, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	return m
}
