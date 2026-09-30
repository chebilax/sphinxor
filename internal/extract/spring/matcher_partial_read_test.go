package spring

import (
	"testing"

	"github.com/chebilax/sphinxor/internal/export/cerbos"
)

// TestMatcherArgs_NeverReadPartially: ADR 0020 §1 — a matcher that cannot
// be read is unknown, never universal and never partial. Two shapes broke
// it by skipping an argument instead:
//   - requestMatchers(HttpMethod.OPTIONS, "/**") was read as
//     requestMatchers("/**"), since OPTIONS was missing from the method
//     table: a CORS preflight rule became a permitAll() on every path and
//     every verb, hiding the ADMIN rule after it (Stirling-PDF's SaaS
//     chain has this shape);
//   - requestMatchers("/open", secretPath()) was read as "/open" alone, so
//     /admin/** endpoints skipped a rule that may govern them.
func TestMatcherArgs_NeverReadPartially(t *testing.T) {
	for _, tc := range []struct {
		name, body     string
		wantURLRoles   map[string]string
		wantExportNone bool
	}{
		{name: "OPTIONS preflight rule scopes to OPTIONS only", body: `
            auth.requestMatchers(HttpMethod.OPTIONS, "/**").permitAll();
            auth.requestMatchers("/reports/**").hasRole("ADMIN");
            auth.anyRequest().authenticated();`,
			wantURLRoles: map[string]string{"GET /reports/{id}": "ADMIN", "DELETE /reports/{id}": "ADMIN"}},
		{name: "an unread argument makes the whole matcher unreadable", body: `
            auth.requestMatchers("/open", secretPath()).hasRole("ADMIN");
            auth.anyRequest().authenticated();`,
			wantURLRoles: map[string]string{}, wantExportNone: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := chainBranchProject(t, tc.body, `
    private String secretPath() { return System.getenv("SECRET_PATH"); }`)
			got := urlRolesByEndpoint(m)
			if len(got) != len(tc.wantURLRoles) {
				t.Fatalf("URL-layer roles %v, want %v", got, tc.wantURLRoles)
			}
			for ep, r := range tc.wantURLRoles {
				if got[ep] != r {
					t.Errorf("%s: URL-layer roles %q, want %q", ep, got[ep], r)
				}
			}
			if tc.wantExportNone {
				if r := cerbos.Translate(m); len(r.Rules) != 0 {
					t.Errorf("exported %+v; the matcher may cover /reports/**, so nothing may be granted", r.Rules)
				}
			}
		})
	}
}
