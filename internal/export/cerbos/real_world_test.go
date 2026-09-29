package cerbos

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/cerbostest"
	"github.com/chebilax/sphinxor/internal/extract/nestjs"
)

// cerbosBinary locates the real cerbos CLI on PATH. Per ADR 0009 §5 and
// Consequences, this is a test-time dependency, not a runtime one — a
// contributor without it installed locally still gets everything else
// `go test ./...` covers; CI installs it explicitly (.github/workflows/ci.yml)
// so this validation is never silently skipped where it actually gates a
// merge.
func cerbosBinary(t *testing.T) string {
	t.Helper()
	return cerbostest.Binary(t)
}

// compileWithCerbos runs the real `cerbos compile` against dir and fails
// t with the engine's own diagnostic output if it doesn't accept the
// generated policies — per ADR 0009 §5, a policy that doesn't compile in
// the real engine is this feature's equivalent of a silently-not-firing
// diff regression: the kind of bug "it looks right" doesn't catch.
func compileWithCerbos(t *testing.T, dir string) {
	t.Helper()
	cerbos := cerbosBinary(t)
	out, err := exec.Command(cerbos, "compile", "--skip-tests", dir).CombinedOutput()
	if err != nil {
		t.Fatalf("cerbos compile %s failed:\n%s", dir, out)
	}
}

// TestRealWorldExport_NestjsBoilerplate exports the vendored
// brocoders/nestjs-boilerplate fixture and validates the result against
// the real Cerbos engine, not just against Sphinxor's own translation
// logic. UsersController has a uniform class-level @Roles(RoleEnum.admin)
// guard, so every one of its endpoints should be exported — the exact
// opposite case from the collision test below, and worth its own
// real-repo check since a class-level guard expands differently at
// extraction time (one GuardApplication per endpoint, ADR 0002) than a
// method-level one.
func TestRealWorldExport_NestjsBoilerplate(t *testing.T) {
	m, _, err := nestjs.Extract("../../extract/nestjs/testdata/nestjs-boilerplate/src")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	result := Translate(m)

	usersRules := 0
	for _, r := range result.Rules {
		if r.Resource == "users" {
			usersRules++
		}
	}
	if usersRules != 4 {
		t.Errorf("got %d rules for \"users\", want 4 (one per distinct HTTP method: post, get, patch, delete): %+v", usersRules, result.Rules)
	}

	// AuthController's get/patch/delete (all AuthGuard('jwt'), no @Roles())
	// export as "authenticated, any role" grants (ADR 0010). Its "post"
	// shares the action with six unguarded siblings (login, register,
	// confirm, confirm/new, forgot/password, reset/password): since ADR
	// 0044 the guarded ones (logout, refresh) become route-conditioned
	// rules and the six are omitted as no-guard, never covered by a rule.
	authActions := map[string][]string{}
	var authPostRoutes []string
	for _, r := range result.Rules {
		if r.Resource == "auth" && r.Route == "" {
			authActions[r.Action] = r.Roles
		}
		if r.Resource == "auth" && r.Action == "post" {
			if r.Route == "" {
				t.Errorf("auth post rule without a route condition would govern its unguarded siblings: %+v", r)
			}
			authPostRoutes = append(authPostRoutes, r.Route)
		}
	}
	for _, o := range result.Omissions {
		if o.Resource == "auth" && o.Reason != ReasonNoGuard {
			t.Errorf("auth omission %s %s: reason %s, want no-guard", o.Endpoint.HTTPMethod, o.Endpoint.Path, o.Reason)
		}
	}
	if len(authPostRoutes) != 2 {
		t.Errorf("auth post routes = %v, want the two guarded ones (logout, refresh)", authPostRoutes)
	}
	for _, action := range []string{"get", "patch", "delete"} {
		roles, ok := authActions[action]
		if !ok || len(roles) != 1 || roles[0] != anyAuthenticatedRole {
			t.Errorf("auth %s roles = %v, want [%q] (AuthenticationRequirement)", action, roles, anyAuthenticatedRole)
		}
	}

	dir := t.TempDir()
	if _, err := WritePolicies(dir, result); err != nil {
		t.Fatalf("WritePolicies: %v", err)
	}
	compileWithCerbos(t, dir)
}

// TestRealWorldExport_AwesomeNestBoilerplate covers the case that drove
// the action-collision handling: PostController's GET /posts (role
// RoleType.USER) and GET /posts/:id (@Auth([]), no role) share a Cerbos
// action but differ. Since ADR 0044 each becomes a rule conditioned on its
// own route. The real engine checks the contract ADR 0044 states: the
// route TEMPLATE is allowed, and a concrete path or a missing route is
// denied.
func TestRealWorldExport_AwesomeNestBoilerplate(t *testing.T) {
	m, _, err := nestjs.Extract("../../extract/nestjs/testdata/awesome-nest-boilerplate/src")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	result := Translate(m)

	routes := map[string]bool{}
	for _, r := range result.Rules {
		if r.Resource == "post" && r.Action == "get" {
			if r.Route == "" {
				t.Fatalf("post get rule without a route condition: %+v", r)
			}
			routes[r.Route] = true
		}
	}
	if !routes["/posts"] || !routes["/posts/:id"] {
		t.Errorf("post get routes = %v, want /posts and /posts/:id", routes)
	}
	for _, o := range result.Omissions {
		if o.Reason == ReasonActionCollision {
			t.Errorf("no action collision may remain: %+v", o)
		}
	}
	dirRoute := t.TempDir()
	if _, err := WritePolicies(dirRoute, result); err != nil {
		t.Fatal(err)
	}
	policy, err := os.ReadFile(filepath.Join(dirRoute, "post.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(policy), "ROUTE CONDITION") || !strings.Contains(string(policy), "never\n# the concrete request path") {
		t.Errorf("post.yaml lacks the route-contract header:\n%s", policy)
	}
	testRouteContractInEngine(t, dirRoute)

	dir := t.TempDir()
	if _, err := WritePolicies(dir, result); err != nil {
		t.Fatalf("WritePolicies: %v", err)
	}
	compileWithCerbos(t, dir)
}

// TestRealWorldExport_EmptyDirCompiles is a minimal sanity check that an
// entirely unguarded, single-endpoint project still produces a
// cerbos-compile-clean output (an explicit `rules: []`, not a parse
// failure) — the exact regression a hand-check of the YAML shape alone
// wouldn't have caught (see policy.go's handling of the zero-rules case).
func TestRealWorldExport_EmptyDirCompiles(t *testing.T) {
	src := `
@Controller('health')
export class HealthController {
  @Get()
  check() {}
}
`
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/health.controller.ts", []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	m, _, err := nestjs.Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	result := Translate(m)

	out := t.TempDir()
	if _, err := WritePolicies(out, result); err != nil {
		t.Fatalf("WritePolicies: %v", err)
	}
	compileWithCerbos(t, out)
}

// testRouteContractInEngine runs a Cerbos policy test suite against the
// generated post policy: GET /posts requires RoleType.USER on the route
// template "/posts". A concrete path, or no route, is denied — the
// fail-closed behaviour ADR 0044's contract relies on.
func testRouteContractInEngine(t *testing.T, dir string) {
	t.Helper()
	cerbos := cerbosBinary(t)
	if err := os.MkdirAll(filepath.Join(dir, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	suite := `name: RouteContract
principals:
  user: {id: u, roles: ["RoleType.USER"]}
resources:
  template: {kind: post, id: "1", attr: {route: "/posts"}}
  concrete: {kind: post, id: "2", attr: {route: "/posts?page=1"}}
  concreteId: {kind: post, id: "3", attr: {route: "/posts/42"}}
  noroute: {kind: post, id: "4"}
tests:
  - name: the template is allowed, a concrete path or none is denied
    input: {principals: [user], resources: [template, concrete, concreteId, noroute], actions: [get]}
    expected:
      - {principal: user, resource: template,   actions: {get: EFFECT_ALLOW}}
      - {principal: user, resource: concrete,   actions: {get: EFFECT_DENY}}
      - {principal: user, resource: concreteId, actions: {get: EFFECT_DENY}}
      - {principal: user, resource: noroute,    actions: {get: EFFECT_DENY}}
`
	if err := os.WriteFile(filepath.Join(dir, "tests", "route_test.yaml"), []byte(suite), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(cerbos, "compile", dir).CombinedOutput()
	if err != nil {
		t.Fatalf("cerbos compile with the route test suite failed:\n%s", out)
	}
}
