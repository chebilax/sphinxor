package nestjs

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

const routeKeyEnum = `export enum RouteKey {
  Admin = 'admin',
  Public = 'public',
}
`

// TestUnreadableControllerPath_DoesNotMergeEndpoints is ADR 0020
// Amendment 1 §5's regression test, and the most important one in this
// package.
//
// Endpoint identity is derived from (method, path). Extraction read only
// string literals out of @Controller(...), so a route enum — ordinary
// good practice, used by immich in 4 of its 47 controllers — silently
// produced an empty prefix. Two unrelated endpoints then collided on one
// ID and were merged: the guards and roles of the protected one were
// reported against the wide-open one, and the
// mutating-endpoint-without-access-control finding that exists to catch
// exactly an unguarded DELETE did not fire.
//
// That is §1's failure reached through a different door — a false
// assurance on a destructive endpoint with the safety net suppressed at
// the same moment. Confirmed to fail against that behavior before being
// kept.
func TestUnreadableControllerPath_DoesNotMergeEndpoints(t *testing.T) {
	dir := writeTSProject(t, map[string]string{
		"routes.ts": routeKeyEnum,
		"admin.controller.ts": `import { Controller, Delete, UseGuards } from '@nestjs/common';
import { Roles } from './roles.decorator';
import { RouteKey } from './routes';

@Controller(RouteKey.Admin)
export class AdminController {
  @UseGuards(AuthGuard, RolesGuard)
  @Roles('ADMIN')
  @Delete('wipe')
  public wipe(): void {}
}
`,
		"public.controller.ts": `import { Controller, Delete } from '@nestjs/common';
import { RouteKey } from './routes';

@Controller(RouteKey.Public)
export class PublicController {
  @Delete('wipe')
  public wipe(): void {}
}
`,
	})

	m, _, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(m.Endpoints) != 2 {
		t.Fatalf("got %d endpoint(s), want 2: %+v", len(m.Endpoints), m.Endpoints)
	}

	byController := map[string]model.Endpoint{}
	for _, e := range m.Endpoints {
		for _, c := range m.Controllers {
			if c.ID == e.ControllerID {
				byController[c.Name] = e
			}
		}
	}

	admin, okA := byController["AdminController"]
	public, okP := byController["PublicController"]
	if !okA || !okP {
		t.Fatalf("expected one endpoint per controller, got %v", byController)
	}

	// The assertion that bites: distinct identities, so nothing merges.
	if admin.ID == public.ID {
		t.Fatalf("both endpoints share ID %q — an unreadable path must not collapse identity", admin.ID)
	}

	// The wide-open DELETE must carry no protection it doesn't have.
	if roles := rolesOnEndpoint(m, public.ID); len(roles) != 0 {
		t.Errorf("DELETE /public/wipe roles = %v, want none — it has no guard at all", roles)
	}
	if guards := guardNamesFor(m, public.ID); len(guards) != 0 {
		t.Errorf("DELETE /public/wipe guards = %v, want none", guards)
	}

	// And the protected one keeps what it does have.
	if roles := rolesOnEndpoint(m, admin.ID); len(roles) != 1 || roles[0] != "ADMIN" {
		t.Errorf("DELETE /admin/wipe roles = %v, want [ADMIN]", roles)
	}

	// Both paths are unresolved, so consumers can say so rather than
	// presenting "/wipe" as a real route.
	for name, e := range map[string]model.Endpoint{"admin": admin, "public": public} {
		if !e.PathUnresolved {
			t.Errorf("%s endpoint: PathUnresolved = false, want true — its prefix was never read", name)
		}
	}
}

// TestUnreadableControllerPath_Shapes covers the argument forms that
// silently produced an empty prefix, against the bare @Controller() that
// legitimately has none and must not be marked uncertain.
func TestUnreadableControllerPath_Shapes(t *testing.T) {
	cases := []struct {
		name           string
		decorator      string
		wantUnresolved bool
	}{
		{"string literal", `@Controller('cats')`, false},
		{"object form with path", `@Controller({ path: 'cats', version: '1' })`, false},
		{"bare, genuinely no prefix", `@Controller()`, false},
		{"enum member", `@Controller(RouteKey.Asset)`, true},
		{"const reference", `@Controller(BASE)`, true},
		{"array form", `@Controller(['cats', 'kittens'])`, true},
		{"template literal", "@Controller(`${base}/v2`)", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := extractTS(t, `import { Controller, Get } from '@nestjs/common';
`+tc.decorator+`
export class C {
  @Get('x')
  public x(): void {}
}
`)
			if len(m.Endpoints) != 1 {
				t.Fatalf("got %d endpoint(s), want 1: %+v", len(m.Endpoints), m.Endpoints)
			}
			if got := m.Endpoints[0].PathUnresolved; got != tc.wantUnresolved {
				t.Errorf("PathUnresolved = %v, want %v", got, tc.wantUnresolved)
			}
		})
	}
}

// TestUnreadableMethodPath covers the same defect one level down.
func TestUnreadableMethodPath(t *testing.T) {
	m := extractTS(t, `import { Controller, Get } from '@nestjs/common';
const P = 'detail';
@Controller('cats')
export class C {
  @Get(P)
  public a(): void {}

  @Get('other')
  public b(): void {}
}
`)
	if len(m.Endpoints) != 2 {
		t.Fatalf("got %d endpoint(s), want 2: %+v", len(m.Endpoints), m.Endpoints)
	}
	for _, e := range m.Endpoints {
		want := e.HandlerName == "a"
		if e.PathUnresolved != want {
			t.Errorf("handler %s: PathUnresolved = %v, want %v", e.HandlerName, e.PathUnresolved, want)
		}
	}
}

func rolesOnEndpoint(m *model.Model, id model.ID) []string {
	guards := map[model.ID]bool{}
	for _, g := range m.GuardApplications {
		if g.EndpointID == id {
			guards[g.ID] = true
		}
	}
	var out []string
	for _, r := range m.RoleReferences {
		if guards[r.GuardApplicationID] {
			out = append(out, r.RawLiteral)
		}
	}
	return out
}
