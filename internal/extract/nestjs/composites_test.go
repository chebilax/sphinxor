package nestjs

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// TestResolveCompositeArgs_CrossFile is a regression test for a real bug
// found while validating against testdata/awesome-nest-boilerplate: a
// composite decorator's inner call nodes (e.g. Roles(roles) inside
// Auth's body) must be read against the DEFINING file's source bytes,
// never the call site's — mixing them silently produces garbage text
// from the wrong byte offsets, not a crash, which is what made this bug
// easy to miss initially.
func TestResolveCompositeArgs_CrossFile(t *testing.T) {
	defRoot, defSrc := parseTS(t, `
export function Auth(roles: RoleType[] = []): MethodDecorator {
  return applyDecorators(
    Roles(roles),
    UseGuards(AuthGuard(), RolesGuard),
  );
}
`)
	composites := collectCompositeDecorators(defRoot, defSrc)
	if _, ok := composites["Auth"]; !ok {
		t.Fatalf("Auth not recognized as a composite: %+v", composites)
	}

	// A completely different file, with its own unrelated source bytes,
	// using the composite defined above.
	callRoot, callSrc := parseTS(t, `
enum RoleType {
  Admin = 'admin',
}

@Controller('things')
export class ThingsController {
  @Auth([RoleType.Admin])
  @Post()
  create() {}
}
`)

	b := newBuilder()
	usedEnums := collectRoleEnumNames(callRoot, callSrc, composites)
	if !usedEnums["RoleType"] {
		t.Fatalf("expected RoleType to be recognized as used via the cross-file composite, got %v", usedEnums)
	}
	roleDecls := extractRoleDeclarations(callRoot, callSrc, "call-site.ts", b.nextID("role"), usedEnums)
	b.model.RoleDeclarations = roleDecls
	roleByName := map[string]model.ID{}
	for _, d := range roleDecls {
		roleByName[d.Name] = d.ID
	}

	extractControllers(callRoot, callSrc, "call-site.ts", b, roleByName, composites)

	if len(b.model.Endpoints) != 1 {
		t.Fatalf("got %d endpoints, want 1", len(b.model.Endpoints))
	}
	endpointID := b.model.Endpoints[0].ID

	guards := nonRoleGuardNames(b.model, endpointID)
	wantGuards := map[string]bool{"AuthGuard": true, "RolesGuard": true}
	if len(guards) != len(wantGuards) {
		t.Fatalf("guards = %v, want AuthGuard and RolesGuard — if this is garbled or empty, the cross-file source bug has regressed", guards)
	}
	for _, g := range guards {
		if !wantGuards[g] {
			t.Errorf("unexpected guard %q (garbled text is exactly the symptom of the cross-file source bug)", g)
		}
	}

	if len(b.model.RoleReferences) != 1 {
		t.Fatalf("got %d role references, want 1", len(b.model.RoleReferences))
	}
	ref := b.model.RoleReferences[0]
	if ref.RawLiteral != "RoleType.Admin" {
		t.Errorf("role reference raw literal = %q, want RoleType.Admin (garbled text indicates the cross-file source bug)", ref.RawLiteral)
	}
	if ref.RoleDeclarationID == nil {
		t.Errorf("RoleType.Admin should resolve to a declaration")
	}
}

func TestCollectCompositeDecorators_MultipleReturnsNotRecognized(t *testing.T) {
	root, src := parseTS(t, `
export function Auth(roles: RoleType[] = []): MethodDecorator {
  if (roles.length === 0) {
    return SkipAuth();
  }
  return applyDecorators(Roles(roles), UseGuards(RolesGuard));
}
`)
	composites := collectCompositeDecorators(root, src)
	if _, ok := composites["Auth"]; ok {
		t.Errorf("a composite with more than one return path must not be recognized, per ADR 0006's explicit non-goal")
	}
}

func TestCollectCompositeDecorators_DestructuredParameterNotRecognized(t *testing.T) {
	root, src := parseTS(t, `
export function Auth({ roles }: { roles: RoleType[] }): MethodDecorator {
  return applyDecorators(Roles(roles), UseGuards(RolesGuard));
}
`)
	composites := collectCompositeDecorators(root, src)
	if _, ok := composites["Auth"]; ok {
		t.Errorf("a composite with a destructured parameter must not be recognized, per ADR 0006's explicit non-goal")
	}
}

func TestCollectCompositeDecorators_NonApplyDecoratorsReturnNotRecognized(t *testing.T) {
	root, src := parseTS(t, `
export function Auth(roles: RoleType[] = []): MethodDecorator {
  return UseGuards(RolesGuard);
}
`)
	composites := collectCompositeDecorators(root, src)
	if _, ok := composites["Auth"]; ok {
		t.Errorf("a composite whose return isn't a direct applyDecorators(...) call must not be recognized")
	}
}

func TestResolveCompositeArgs_EmptyArraySubstitution(t *testing.T) {
	root, src := parseTS(t, `
export function Auth(roles: RoleType[] = []): MethodDecorator {
  return applyDecorators(Roles(roles), UseGuards(RolesGuard));
}

@Controller('things')
export class ThingsController {
  @Auth([])
  @Get()
  list() {}
}
`)
	composites := collectCompositeDecorators(root, src)
	b := newBuilder()
	extractControllers(root, src, "f.ts", b, nil, composites)

	if len(b.model.Endpoints) != 1 {
		t.Fatalf("got %d endpoints, want 1", len(b.model.Endpoints))
	}
	endpointID := b.model.Endpoints[0].ID

	var rolesApp *model.GuardApplication
	for i := range b.model.GuardApplications {
		if b.model.GuardApplications[i].EndpointID == endpointID && b.model.GuardApplications[i].GuardName == "Roles" {
			rolesApp = &b.model.GuardApplications[i]
		}
	}
	if rolesApp == nil {
		t.Fatalf("expected a Roles GuardApplication (from @Auth([])), found none: %+v", b.model.GuardApplications)
	}
	if !rolesApp.FromComposite {
		t.Errorf("Roles application from @Auth([]) should have FromComposite = true")
	}
	for _, ref := range b.model.RoleReferences {
		if ref.GuardApplicationID == rolesApp.ID {
			t.Errorf("expected zero role references for @Auth([]), got %+v", ref)
		}
	}
}
