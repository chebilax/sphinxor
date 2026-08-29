package spring

import (
	"reflect"
	"testing"
)

// TestParseSpEL covers the bounded recognized-shape set docs/decisions/0011-spring-second-framework.md
// §1 specifies, plus the unrecognized shapes it explicitly names as
// deliberately left unparsed rather than partially guessed at.
func TestParseSpEL(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want spelResult
	}{
		{"hasRole single", `hasRole('ADMIN')`, spelResult{Kind: spelRoles, Roles: []string{"ADMIN"}}},
		{"hasAnyRole two", `hasAnyRole('ADMIN', 'PHARMACIST')`, spelResult{Kind: spelRoles, Roles: []string{"ADMIN", "PHARMACIST"}}},
		{"hasAnyRole no space after comma", `hasAnyRole('ADMIN','PHARMACIST')`, spelResult{Kind: spelRoles, Roles: []string{"ADMIN", "PHARMACIST"}}},
		{"hasAuthority single", `hasAuthority('ROLE_ADMIN')`, spelResult{Kind: spelRoles, Roles: []string{"ROLE_ADMIN"}}},
		{"hasAnyAuthority two", `hasAnyAuthority('A', 'B')`, spelResult{Kind: spelRoles, Roles: []string{"A", "B"}}},
		{"isAuthenticated", `isAuthenticated()`, spelResult{Kind: spelAuthenticated}},
		{"isAuthenticated with whitespace", `  isAuthenticated()  `, spelResult{Kind: spelAuthenticated}},
		{"permitAll", `permitAll()`, spelResult{Kind: spelUnrecognized}},
		{"denyAll", `denyAll()`, spelResult{Kind: spelUnrecognized}},
		{"boolean combination not matched whole", `hasRole('ADMIN') and #id == authentication.name`, spelResult{Kind: spelUnrecognized}},
		{"or combination", `hasRole('A') || hasAuthority('B')`, spelResult{Kind: spelUnrecognized}},
		{"bean method call", `@authService.check(#id)`, spelResult{Kind: spelUnrecognized}},
		{"empty string", ``, spelResult{Kind: spelUnrecognized}},
		{"hasRole empty args", `hasRole()`, spelResult{Kind: spelUnrecognized}},
		{"hasRole unquoted arg", `hasRole(ADMIN)`, spelResult{Kind: spelUnrecognized}},
		{"hasRole malformed quote", `hasRole('ADMIN)`, spelResult{Kind: spelUnrecognized}},
		{"unknown function", `hasPermission('ADMIN')`, spelResult{Kind: spelUnrecognized}},
		{"isAuthenticated with stray arg not matched", `isAuthenticated(true)`, spelResult{Kind: spelUnrecognized}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSpEL(tt.expr)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseSpEL(%q) = %+v, want %+v", tt.expr, got, tt.want)
			}
		})
	}
}
