package nestjs

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// guardSet collects every GuardApplication name attached to endpointID.
func guardSet(m *model.Model, endpointID model.ID) map[string]bool {
	names := make(map[string]bool)
	for _, g := range m.GuardApplications {
		if g.EndpointID == endpointID {
			names[g.GuardName] = true
		}
	}
	return names
}

// endpointsAt returns every endpoint matching method and path, in
// extraction order — more than one means they collided on a path but were
// kept apart by something else (here, the version).
func endpointsAt(m *model.Model, method model.HTTPMethod, path string) []model.Endpoint {
	var out []model.Endpoint
	for _, e := range m.Endpoints {
		if e.HTTPMethod == method && e.Path == path {
			out = append(out, e)
		}
	}
	return out
}

// TestExtract_NovuReadableVersionSeparatesIdentity is §7's readable-version
// case, against the vendored novu pair (testdata/novu/NOTICE.md).
//
// /topics is declared twice: once as @Controller('/topics') with no version,
// and once as @Controller({ path: '/topics', version: '2' }). novu bootstraps
// VersioningType.URI with defaultVersion '1', so these are two real routes.
// Before §7 the version key was stepped over, both collapsed onto
// "GET /topics", and each was reported carrying the other's class-level
// decorators.
func TestExtract_NovuReadableVersionSeparatesIdentity(t *testing.T) {
	m, _, err := Extract("testdata/novu")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	got := endpointsAt(m, model.MethodGet, "/topics")
	if len(got) != 2 {
		t.Fatalf("GET /topics: got %d endpoints, want 2 (v1 and v2 are distinct routes)", len(got))
	}
	if got[0].ID == got[1].ID {
		t.Fatalf("GET /topics: both endpoints share ID %q — the version did not separate them", got[0].ID)
	}

	byVersion := make(map[string]model.Endpoint, 2)
	for _, e := range got {
		if e.VersionUnresolved {
			t.Errorf("GET /topics on %s: version marked unresolved, but both declarations are readable", e.HandlerName)
		}
		byVersion[e.Version] = e
	}
	if _, ok := byVersion[""]; !ok {
		t.Errorf("no unversioned GET /topics found; versions present: %v", byVersion)
	}
	if _, ok := byVersion["2"]; !ok {
		t.Errorf("no version-2 GET /topics found; versions present: %v", byVersion)
	}

	// The unversioned half must keep exactly the identity it has today.
	// §7's bounding rule: "no version declared" is absent, not unknown.
	if want := model.NewEndpointID(model.MethodGet, "/topics"); byVersion[""].ID != want {
		t.Errorf("unversioned endpoint ID = %q, want %q unchanged", byVersion[""].ID, want)
	}
}

// TestExtract_CalcomUnreadableVersionSeparatesIdentity is §7's
// unreadable-version case *and* the guard-bleed regression, against the
// vendored cal.com pair (testdata/cal.com/NOTICE.md).
//
// Both controllers declare path "/v2/event-types"; their versions are
// constant references (one an array of them), so neither is readable. They
// must still not be assumed equal. GET / carries @UseGuards(ApiAuthGuard) on
// the 2024-04-15 side and @UseGuards(OptionalApiAuthGuard) on the 2024-06-14
// side: merged, each is reported carrying the other's guard.
func TestExtract_CalcomUnreadableVersionSeparatesIdentity(t *testing.T) {
	m, _, err := Extract("testdata/cal.com")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	got := endpointsAt(m, model.MethodGet, "/v2/event-types")
	if len(got) != 2 {
		t.Fatalf("GET /v2/event-types: got %d endpoints, want 2", len(got))
	}
	if got[0].ID == got[1].ID {
		t.Fatalf("GET /v2/event-types: both endpoints share ID %q — unreadable versions were assumed equal", got[0].ID)
	}

	byController := make(map[string]model.Endpoint, 2)
	for _, e := range got {
		if !e.VersionUnresolved {
			t.Errorf("GET /v2/event-types on %s: version should be marked unresolved (constant reference)", e.HandlerName)
		}
		for _, c := range m.Controllers {
			if c.ID == e.ControllerID {
				byController[c.Name] = e
			}
		}
	}

	older, ok := byController["EventTypesController_2024_04_15"]
	if !ok {
		t.Fatalf("EventTypesController_2024_04_15 endpoint not found, got %v", byController)
	}
	newer, ok := byController["EventTypesController_2024_06_14"]
	if !ok {
		t.Fatalf("EventTypesController_2024_06_14 endpoint not found, got %v", byController)
	}

	olderGuards := guardSet(m, older.ID)
	if !olderGuards["ApiAuthGuard"] {
		t.Errorf("2024-04-15 GET /v2/event-types lost its own ApiAuthGuard, guards = %v", olderGuards)
	}
	if olderGuards["OptionalApiAuthGuard"] {
		t.Errorf("2024-04-15 GET /v2/event-types carries the other controller's OptionalApiAuthGuard, guards = %v", olderGuards)
	}

	newerGuards := guardSet(m, newer.ID)
	if !newerGuards["OptionalApiAuthGuard"] {
		t.Errorf("2024-06-14 GET /v2/event-types lost its own OptionalApiAuthGuard, guards = %v", newerGuards)
	}
	if newerGuards["ApiAuthGuard"] {
		t.Errorf("2024-06-14 GET /v2/event-types carries the other controller's mandatory ApiAuthGuard, guards = %v", newerGuards)
	}
}

// TestExtract_NoVersionDeclaredIsUnaffected pins §7's bounding rule: an
// endpoint that declares no version must come out of extraction with
// exactly the identity it had before §7, so existing allowlist anchors and
// stored diff baselines keep working.
//
// It is asserted per endpoint rather than per project deliberately. Both
// vendored boilerplates turn out to declare versions of their own —
// nestjs-boilerplate uses @Controller({ path, version: '1' }) throughout,
// and awesome-nest-boilerplate carries a @Version('1') on GET /auth/me —
// which is itself the finding that this mechanism has been invisible in
// this project's own corpus since v0.1.
func TestExtract_NoVersionDeclaredIsUnaffected(t *testing.T) {
	// nestjs-boilerplate is deliberately absent: every one of its
	// controllers declares version '1', so it has no version-free endpoint
	// to pin. awesome-nest-boilerplate has one versioned handler among
	// seven, and novu's topics-v1 controller declares none at all.
	for _, dir := range []string{"testdata/awesome-nest-boilerplate/src", "testdata/novu"} {
		m, _, err := Extract(dir)
		if err != nil {
			t.Fatalf("Extract(%s): %v", dir, err)
		}
		checked := 0
		for _, e := range m.Endpoints {
			if e.PathUnresolved || e.Version != "" || e.VersionUnresolved {
				continue
			}
			checked++
			if want := model.NewEndpointID(e.HTTPMethod, e.Path); e.ID != want {
				t.Errorf("%s: endpoint ID = %q, want %q unchanged", dir, e.ID, want)
			}
		}
		if checked == 0 {
			t.Errorf("%s: no version-free endpoints left to check — the bounding rule is untested here", dir)
		}
	}
}

// TestExtract_VersionInExistingFixtures records what §7 newly sees in the
// corpus that was already vendored, in both of NestJS's declaration forms.
func TestExtract_VersionInExistingFixtures(t *testing.T) {
	m, _, err := Extract("testdata/nestjs-boilerplate/src")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	// @Controller({ path: 'users', version: '1' }) — the object form.
	got := endpointsAt(m, model.MethodGet, "/users")
	if len(got) != 1 {
		t.Fatalf("GET /users: got %d endpoints, want 1", len(got))
	}
	if got[0].Version != "1" || got[0].VersionUnresolved {
		t.Errorf("GET /users version = %q (unresolved=%v), want \"1\"", got[0].Version, got[0].VersionUnresolved)
	}
	if want := model.NewVersionedEndpointID(model.MethodGet, "/users", "1"); got[0].ID != want {
		t.Errorf("GET /users ID = %q, want %q", got[0].ID, want)
	}

	m2, _, err := Extract("testdata/awesome-nest-boilerplate/src")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	// @Version('1') on the handler — the decorator form, on a controller
	// whose @Controller('auth') declares none.
	got2 := endpointsAt(m2, model.MethodGet, "/auth/me")
	if len(got2) != 1 {
		t.Fatalf("GET /auth/me: got %d endpoints, want 1", len(got2))
	}
	if got2[0].Version != "1" || got2[0].VersionUnresolved {
		t.Errorf("GET /auth/me version = %q (unresolved=%v), want \"1\"", got2[0].Version, got2[0].VersionUnresolved)
	}
}
