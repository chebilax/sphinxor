package cerbos

import (
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// TestWildcardMeaningIsStated: a policy file granting "*" says what it
// means — no restriction detected beyond authentication, never that every
// authenticated user really has access — and a file without "*" does not.
func TestWildcardMeaningIsStated(t *testing.T) {
	e := model.Endpoint{ID: "GET /a", HTTPMethod: model.MethodGet, Path: "/a"}
	withStar := Result{Rules: []Rule{{Resource: "a", Action: "get", Roles: []string{anyAuthenticatedRole}, Endpoints: []model.Endpoint{e}}}}
	withRole := Result{Rules: []Rule{{Resource: "a", Action: "get", Roles: []string{"ADMIN"}, Endpoints: []model.Endpoint{e}}}}

	if p := RenderPolicy("a", withStar); !strings.Contains(p, `# ANY AUTHENTICATED ("*"):`) || !strings.Contains(p, "# really has access") {
		t.Errorf("policy granting \"*\" does not state what it means:\n%s", p)
	}
	if p := RenderPolicy("a", withRole); strings.Contains(p, "ANY AUTHENTICATED") {
		t.Errorf("policy without \"*\" carries the wildcard statement:\n%s", p)
	}
}
