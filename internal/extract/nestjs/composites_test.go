package nestjs

import (
	"testing"

	sitter "github.com/smacker/go-tree-sitter"

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

// TestCollectCompositeDecorators_RestParameterRecognized pins the
// correction recorded in ADR 0006's amendment: a rest parameter in the
// composite's OWN signature never disqualified the composite by
// decision — it shared a tree-sitter code path with destructuring
// (a rest parameter is a `required_parameter` whose `pattern` is a
// `rest_pattern`, not an `identifier`), and so was rejected by the check
// written for destructuring.
//
// Measured on ghostfolio, whose `RequiresScope(...requiredScopes: Scope[])`
// is otherwise exactly ADR 0006's bounded shape — a single return path
// calling applyDecorators(...) with a literal UseGuards(...) inside — and
// which carries the real guards of 33 endpoints.
func TestCollectCompositeDecorators_RestParameterRecognized(t *testing.T) {
	root, src := parseTS(t, `
export function RequiresScope(...requiredScopes: Scope[]) {
  return applyDecorators(
    SetMetadata(REQUIRES_SCOPE_KEY, requiredScopes),
    UseGuards(AuthGuard('jwt'), HasPermissionGuard, ImpersonationGuard, ScopeGuard),
  );
}
`)
	composites := collectCompositeDecorators(root, src)
	comp, ok := composites["RequiresScope"]
	if !ok {
		t.Fatalf("a composite whose only parameter is a rest parameter must be recognized: %+v", composites)
	}
	if len(comp.params) != 0 {
		t.Errorf("a rest parameter must not be bound positionally, got params %v", comp.params)
	}

	guardArgs, _, _, ok := resolveCompositeArgs(
		decoratorCall{Name: "RequiresScope"}, src, composites)
	if !ok {
		t.Fatalf("resolveCompositeArgs did not resolve a registered composite")
	}
	var names []string
	for _, g := range guardArgs {
		names = append(names, guardArgName(g.node, g.src))
	}
	want := []string{"AuthGuard", "HasPermissionGuard", "ImpersonationGuard", "ScopeGuard"}
	if len(names) != len(want) {
		t.Fatalf("got guards %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("got guards %v, want %v", names, want)
		}
	}
}

// TestResolveCompositeArgs_RestParameterNotSubstituted states the one
// thing the correction above deliberately does NOT do. A rest parameter
// binds to the *list* of remaining call-site arguments, not to one of
// them, so substituting it positionally would silently drop every
// argument after the first. Recognizing the composite is the fix;
// guessing at the rest parameter's value is not, and an inner call that
// references it resolves to nothing rather than to something wrong.
func TestResolveCompositeArgs_RestParameterNotSubstituted(t *testing.T) {
	defRoot, defSrc := parseTS(t, `
export function Auth(...roles: RoleType[]) {
  return applyDecorators(Roles(roles), UseGuards(RolesGuard));
}
`)
	composites := collectCompositeDecorators(defRoot, defSrc)
	if _, ok := composites["Auth"]; !ok {
		t.Fatalf("Auth not recognized as a composite: %+v", composites)
	}

	callRoot, callSrc := parseTS(t, `
@Auth(RoleType.User, RoleType.Admin)
class C {}
`)
	call := firstDecoratorCall(t, callRoot, callSrc, "Auth")
	guardArgs, roleArgs, hasRoles, ok := resolveCompositeArgs(call, callSrc, composites)
	if !ok {
		t.Fatalf("resolveCompositeArgs did not resolve a registered composite")
	}
	if !hasRoles {
		t.Errorf("the composite does contain a Roles() call, so hasRoles must stay true")
	}
	if len(roleArgs) != 0 {
		var got []string
		for _, r := range roleArgs {
			got = append(got, r.node.Content(r.src))
		}
		t.Errorf("a rest parameter must resolve to no role arguments rather than to a wrong subset, got %v", got)
	}
	if len(guardArgs) != 1 || guardArgName(guardArgs[0].node, guardArgs[0].src) != "RolesGuard" {
		t.Errorf("guards referencing no parameter must still resolve, got %d", len(guardArgs))
	}
}

// firstDecoratorCall finds the first @Name(...) decorator in a parsed
// snippet, so a test can hand a real call site to resolveCompositeArgs
// rather than hand-building one.
func firstDecoratorCall(t *testing.T, root *sitter.Node, src []byte, name string) decoratorCall {
	t.Helper()
	var found *decoratorCall
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if found != nil {
			return
		}
		if n.Type() == "decorator" {
			if call, ok := parseDecorator(n, src); ok && call.Name == name {
				found = &call
				return
			}
		}
		for _, c := range namedChildren(n) {
			walk(c)
		}
	}
	walk(root)
	if found == nil {
		t.Fatalf("no @%s(...) decorator found in snippet", name)
	}
	return *found
}
