package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// TestExtract_SpringVersionAttributeSeparatesIdentity is §7 on the Spring
// side, against the vendored baeldung fixture (testdata/tutorials/NOTICE.md):
// Spring's native API versioning, two handlers on one path in one controller,
// distinguished only by version = "1.0" / "2.0".
//
// Spring drops rather than merges a colliding endpoint, so before §7 this
// controller reported one endpoint where the application routes two, and the
// 2.0 handler was absent from the report entirely.
func TestExtract_SpringVersionAttributeSeparatesIdentity(t *testing.T) {
	m, _, err := Extract("testdata/tutorials")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	var got []model.Endpoint
	for _, e := range m.Endpoints {
		if e.HTTPMethod == model.MethodGet && e.Path == "/api/products/{id}" {
			got = append(got, e)
		}
	}
	if len(got) != 2 {
		t.Fatalf("GET /api/products/{id}: got %d endpoints, want 2 (version 1.0 and 2.0)", len(got))
	}

	byVersion := make(map[string]model.Endpoint, 2)
	for _, e := range got {
		if e.VersionUnresolved {
			t.Errorf("handler %s: version marked unresolved, but both are string literals", e.HandlerName)
		}
		byVersion[e.Version] = e
	}
	for _, want := range []string{"1.0", "2.0"} {
		if _, ok := byVersion[want]; !ok {
			t.Errorf("no endpoint with version %q; got versions %v", want, byVersion)
		}
	}
	if byVersion["1.0"].ID == byVersion["2.0"].ID {
		t.Errorf("both versions share ID %q", byVersion["1.0"].ID)
	}
}

// TestExtract_SpringNoVersionDeclaredIsUnaffected pins §7's bounding rule on
// the Spring side, same reasoning as the NestJS twin.
func TestExtract_SpringNoVersionDeclaredIsUnaffected(t *testing.T) {
	for _, dir := range []string{"testdata/Pharmacy/backend/src/main/java", "testdata/blog-api/src/main/java"} {
		m, _, err := Extract(dir)
		if err != nil {
			t.Fatalf("Extract(%s): %v", dir, err)
		}
		for _, e := range m.Endpoints {
			if e.Version != "" || e.VersionUnresolved {
				t.Errorf("%s: %s %s has version %q, want none", dir, e.HTTPMethod, e.Path, e.Version)
			}
			if e.PathUnresolved {
				continue
			}
			if want := model.NewEndpointID(e.HTTPMethod, e.Path); e.ID != want {
				t.Errorf("%s: endpoint ID = %q, want %q unchanged", dir, e.ID, want)
			}
		}
	}
}
