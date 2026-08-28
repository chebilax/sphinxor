package cerbos

import (
	"os"
	"os/exec"
	"testing"

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
	path, err := exec.LookPath("cerbos")
	if err != nil {
		t.Skip("cerbos CLI not found on PATH — skipping real-engine validation (see ADR 0009 §5)")
	}
	return path
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
	// should now export as "authenticated, any role" grants (ADR 0010) --
	// confirmed against real data, not just the synthetic
	// authentication_test.go cases. "post" must NOT be among them: it
	// shares its Cerbos action with six genuinely unguarded siblings
	// (login, register, confirm, confirm/new, forgot/password,
	// reset/password), so it still collides -- exactly the corrected
	// worked example in ADR 0010's Consequences, not the original,
	// wrong "post covering both logout and refresh" claim.
	authActions := map[string][]string{}
	for _, r := range result.Rules {
		if r.Resource == "auth" {
			authActions[r.Action] = r.Roles
		}
	}
	for _, action := range []string{"get", "patch", "delete"} {
		roles, ok := authActions[action]
		if !ok || len(roles) != 1 || roles[0] != anyAuthenticatedRole {
			t.Errorf("auth %s roles = %v, want [%q] (AuthenticationRequirement)", action, roles, anyAuthenticatedRole)
		}
	}
	if _, ok := authActions["post"]; ok {
		t.Errorf("auth \"post\" must still collide (shared with six unguarded siblings), got a rule: %+v", authActions["post"])
	}

	dir := t.TempDir()
	if _, err := WritePolicies(dir, result); err != nil {
		t.Fatalf("WritePolicies: %v", err)
	}
	compileWithCerbos(t, dir)
}

// TestRealWorldExport_AwesomeNestBoilerplate covers the case that drove
// this design's action-collision handling: PostController's GET /posts
// (role RoleType.USER) and GET /posts/:id (@Auth([]), no role) share a
// Cerbos action under the controller+method mapping (ADR 0009 §2) but
// have different confirmed roles. Both must be omitted, not merged —
// confirmed here against the real extracted model, not a synthetic one.
func TestRealWorldExport_AwesomeNestBoilerplate(t *testing.T) {
	m, _, err := nestjs.Extract("../../extract/nestjs/testdata/awesome-nest-boilerplate/src")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	result := Translate(m)

	for _, r := range result.Rules {
		if r.Resource == "post" && r.Action == "get" {
			t.Fatalf("expected no \"get\" rule on \"post\" (GET /posts and GET /posts/:id disagree on roles and must collide), got %+v", r)
		}
	}
	collisions := 0
	for _, o := range result.Omissions {
		if o.Reason == ReasonActionCollision {
			collisions++
		}
	}
	if collisions != 2 {
		t.Errorf("got %d action-collision omissions, want 2 (GET /posts and GET /posts/:id): %+v", collisions, result.Omissions)
	}

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
