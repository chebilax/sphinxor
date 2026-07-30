package nestjs

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

func TestExtractControllers_ClassLevelGuardsExpandToEveryEndpoint(t *testing.T) {
	src := `
enum RoleEnum {
  admin = 'admin',
  user = 'user',
}

@UseGuards(AuthGuard('jwt'), RolesGuard)
@Roles(RoleEnum.admin)
@Controller('users')
export class UsersController {
  @Get()
  findAll() {}

  @Delete(':id')
  remove() {}
}
`
	root, source := parseTS(t, src)
	b := newBuilder()
	usedEnums := collectRoleEnumNames(root, source)
	roleDecls := extractRoleDeclarations(root, source, "users.controller.ts", b.nextID("role"), usedEnums)
	b.model.RoleDeclarations = roleDecls
	roleByName := map[string]model.ID{}
	for _, d := range roleDecls {
		roleByName[d.Name] = d.ID
	}

	anchors := extractControllers(root, source, "users.controller.ts", b, roleByName)

	if len(b.model.Endpoints) != 2 {
		t.Fatalf("got %d endpoints, want 2: %+v", len(b.model.Endpoints), b.model.Endpoints)
	}
	if len(anchors) != 2 {
		t.Fatalf("got %d anchors, want 2", len(anchors))
	}

	for _, e := range b.model.Endpoints {
		guardNames := guardNamesForEndpoint(b.model, e.ID)
		wantGuards := map[string]bool{"AuthGuard": true, "RolesGuard": true, "Roles": true}
		for _, g := range guardNames {
			if !wantGuards[g] {
				t.Errorf("endpoint %s %s: unexpected guard %q", e.HTTPMethod, e.Path, g)
			}
		}
		if len(guardNames) != 3 {
			t.Errorf("endpoint %s %s: got guards %v, want AuthGuard, RolesGuard, Roles (class-level expansion)", e.HTTPMethod, e.Path, guardNames)
		}
	}

	roleRefs := b.model.RoleReferences
	if len(roleRefs) != 2 { // one per endpoint, class-level Roles(RoleEnum.admin) expanded
		t.Fatalf("got %d role references, want 2 (one per endpoint): %+v", len(roleRefs), roleRefs)
	}
	for _, r := range roleRefs {
		if r.RawLiteral != "RoleEnum.admin" {
			t.Errorf("role reference raw literal = %q, want RoleEnum.admin", r.RawLiteral)
		}
		if r.RoleDeclarationID == nil {
			t.Errorf("role reference for RoleEnum.admin did not resolve to a declaration")
		}
	}
}

func TestExtractControllers_ObjectLiteralControllerPath(t *testing.T) {
	src := `
@Controller({
  path: 'users',
  version: '1',
})
export class UsersController {
  @Post()
  create() {}
}
`
	root, source := parseTS(t, src)
	b := newBuilder()
	anchors := extractControllers(root, source, "f.ts", b, nil)

	if len(anchors) != 1 {
		t.Fatalf("got %d anchors, want 1", len(anchors))
	}
	if got := b.model.Endpoints[0].Path; got != "/users" {
		t.Errorf("path = %q, want /users", got)
	}
}

func TestExtractControllers_EmptyRolesDecoratorProducesNoRoleReferences(t *testing.T) {
	src := `
@Controller('things')
export class ThingsController {
  @UseGuards(RolesGuard)
  @Roles()
  @Delete(':id')
  remove() {}
}
`
	root, source := parseTS(t, src)
	b := newBuilder()
	extractControllers(root, source, "f.ts", b, nil)

	var rolesGuardApp *model.GuardApplication
	for i := range b.model.GuardApplications {
		if b.model.GuardApplications[i].GuardName == "Roles" {
			rolesGuardApp = &b.model.GuardApplications[i]
		}
	}
	if rolesGuardApp == nil {
		t.Fatalf("no Roles guard application found: %+v", b.model.GuardApplications)
	}
	for _, r := range b.model.RoleReferences {
		if r.GuardApplicationID == rolesGuardApp.ID {
			t.Fatalf("expected zero role references for empty @Roles(), got %+v", r)
		}
	}
}

func TestExtractControllers_BareStringRoleLiteralDoesNotResolve(t *testing.T) {
	src := `
enum RoleEnum {
  admin = 'admin',
}

@Controller('things')
export class ThingsController {
  @UseGuards(RolesGuard)
  @Roles('admin')
  @Delete(':id')
  remove() {}
}
`
	root, source := parseTS(t, src)
	b := newBuilder()
	usedEnums := collectRoleEnumNames(root, source)
	if usedEnums["RoleEnum"] {
		t.Fatalf("bare string literal should not mark RoleEnum as used")
	}
	extractControllers(root, source, "f.ts", b, nil)

	if len(b.model.RoleReferences) != 1 {
		t.Fatalf("got %d role references, want 1", len(b.model.RoleReferences))
	}
	ref := b.model.RoleReferences[0]
	if ref.RawLiteral != "admin" {
		t.Errorf("raw literal = %q, want admin", ref.RawLiteral)
	}
	if ref.RoleDeclarationID != nil {
		t.Errorf("bare string literal role should not resolve to a declaration, got %v", *ref.RoleDeclarationID)
	}
}

func TestExtractControllers_NonControllerClassIgnored(t *testing.T) {
	src := `
@Injectable()
export class UsersService {
  @Get()
  findAll() {}
}
`
	root, source := parseTS(t, src)
	b := newBuilder()
	extractControllers(root, source, "f.ts", b, nil)

	if len(b.model.Controllers) != 0 || len(b.model.Endpoints) != 0 {
		t.Fatalf("expected nothing extracted from a non-@Controller class, got %+v / %+v", b.model.Controllers, b.model.Endpoints)
	}
}

func TestExtractControllers_NonRouteMethodIgnored(t *testing.T) {
	src := `
@Controller('users')
export class UsersController {
  private helper() {}

  @Get()
  findAll() {}
}
`
	root, source := parseTS(t, src)
	b := newBuilder()
	extractControllers(root, source, "f.ts", b, nil)

	if len(b.model.Endpoints) != 1 {
		t.Fatalf("got %d endpoints, want 1 (helper() should be ignored): %+v", len(b.model.Endpoints), b.model.Endpoints)
	}
}

func guardNamesForEndpoint(m model.Model, endpointID model.ID) []string {
	var names []string
	for _, g := range m.GuardApplications {
		if g.EndpointID == endpointID {
			names = append(names, g.GuardName)
		}
	}
	return names
}
