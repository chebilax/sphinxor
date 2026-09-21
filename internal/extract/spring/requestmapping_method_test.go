package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
)

// TestRequestMappingMethod_SingleAndMultiVerb is ADR 0026 §1.
//
// The multi-verb case is the one that matters: across the corpus 532
// such annotations declare 588 routes, because 40 declare more than one
// verb. An implementation reading only the first would be wrong 40 times
// and look right in aggregate.
func TestRequestMappingMethod_SingleAndMultiVerb(t *testing.T) {
	m, _ := extractOne(t, `
@RestController
@RequestMapping("/api")
public class C {
    @RequestMapping(value = "/one", method = RequestMethod.POST)
    public void one() { }

    @RequestMapping(value = "/two", method = {RequestMethod.GET, RequestMethod.POST})
    public void two() { }
}
`)
	got := map[string]string{}
	for _, e := range m.Endpoints {
		got[string(e.HTTPMethod)+" "+e.Path] = e.HandlerName
	}
	for _, want := range []string{"POST /api/one", "GET /api/two", "POST /api/two"} {
		if got[want] == "" {
			t.Errorf("missing %s; got %v", want, got)
		}
	}
	if len(m.Endpoints) != 3 {
		t.Errorf("got %d endpoints, want 3 (one + two verbs)", len(m.Endpoints))
	}
	// Both rows of the multi-verb mapping name the same handler.
	if got["GET /api/two"] != "two" || got["POST /api/two"] != "two" {
		t.Errorf("both expanded rows should share the handler, got %v", got)
	}
}

// TestRequestMappingMethod_KeepsItsAuthorization is the sharpest check on
// the corpus result: a route recovered by this decision must carry
// whatever its handler already declared. thingsboard's POST /api/customer
// is the real instance — readable @PreAuthorize, invisible until now.
func TestRequestMappingMethod_KeepsItsAuthorization(t *testing.T) {
	m, _ := extractOne(t, `
import org.springframework.security.access.prepost.PreAuthorize;

@RestController
@RequestMapping("/api")
public class CustomerController {
    @PreAuthorize("hasAuthority('TENANT_ADMIN')")
    @RequestMapping(value = "/customer", method = RequestMethod.POST)
    public void saveCustomer() { }
}
`)
	if len(m.Endpoints) != 1 {
		t.Fatalf("got %d endpoints, want 1", len(m.Endpoints))
	}
	if len(m.RoleReferences) != 1 || m.RoleReferences[0].RawLiteral != "TENANT_ADMIN" {
		t.Fatalf("the recovered route must keep its role, got %+v", m.RoleReferences)
	}
	if got := (lint.MutatingEndpointWithoutAccessControl{}).Check(m); len(got) != 0 {
		t.Errorf("a guarded recovered route must not be flagged, got %d findings", len(got))
	}
}

// TestRequestMappingMethod_ExtraVerbs is ADR 0026 §2: HEAD, OPTIONS and
// TRACE are extracted, and none of them is mutating.
func TestRequestMappingMethod_ExtraVerbs(t *testing.T) {
	m, _ := extractOne(t, `
@RestController
public class C {
    @RequestMapping(value = "/h", method = RequestMethod.HEAD)
    public void h() { }

    @RequestMapping(value = "/o", method = RequestMethod.OPTIONS)
    public void o() { }

    @RequestMapping(value = "/t", method = RequestMethod.TRACE)
    public void t() { }
}
`)
	seen := map[model.HTTPMethod]bool{}
	for _, e := range m.Endpoints {
		seen[e.HTTPMethod] = true
	}
	for _, want := range []model.HTTPMethod{model.MethodHead, model.MethodOptions, model.MethodTrace} {
		if !seen[want] {
			t.Errorf("%s should be extracted", want)
		}
	}
	// §2.3: recording them changes what the matrix shows, not what the
	// rules claim. None mutates state.
	if got := (lint.MutatingEndpointWithoutAccessControl{}).Check(m); len(got) != 0 {
		t.Errorf("HEAD/OPTIONS/TRACE must not be mutating, got %d findings: %+v", len(got), got)
	}
}

// TestRequestMappingMethod_VerblessStaysOut pins ADR 0026 §4. A
// method-level @RequestMapping with no method attribute maps every verb
// in Spring; there are 164 across 17 corpus repositories, and
// representing one is a model decision this ADR deliberately does not
// make. The exclusion is pinned so it stays deliberate.
func TestRequestMappingMethod_VerblessStaysOut(t *testing.T) {
	m, _ := extractOne(t, `
@RestController
@RequestMapping("/api")
public class C {
    @RequestMapping("/anything")
    public void anything() { }
}
`)
	if len(m.Endpoints) != 0 {
		t.Errorf("a verb-less @RequestMapping declares no known route yet (§4), got %+v", m.Endpoints)
	}
}

// TestRequestMappingMethod_UnknownVerbIsNotGuessed guards the parser: a
// RequestMethod constant this model has no term for yields no endpoint
// rather than a guessed one.
func TestRequestMappingMethod_UnknownVerbIsNotGuessed(t *testing.T) {
	m, _ := extractOne(t, `
@RestController
public class C {
    @RequestMapping(value = "/x", method = RequestMethod.SOMETHINGNEW)
    public void x() { }
}
`)
	if len(m.Endpoints) != 0 {
		t.Errorf("an unrecognized verb must not be guessed at, got %+v", m.Endpoints)
	}
}
