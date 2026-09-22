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
		// permitAll()/denyAll() moved from spelUnrecognized to spelNoRole
		// under ADR 0020 Amendment 3 §10. Their *behaviour* is unchanged
		// — DeclaresRoles stays true and empty-role still fires, pinned
		// by TestExtractControllers_PermitAllStillDeclaresRoles — but
		// they now have to be named rather than falling through, so that
		// §9's RolesUnresolved does not sweep them up and silently
		// reverse ADR 0017's boundary.
		{"permitAll", `permitAll()`, spelResult{Kind: spelNoRole}},
		{"denyAll", `denyAll()`, spelResult{Kind: spelNoRole}},
		{"permitAll with stray arg is unrecognized", `permitAll(true)`, spelResult{Kind: spelUnrecognized}},
		{"boolean combination not matched whole", `hasRole('ADMIN') and #id == authentication.name`, spelResult{Kind: spelUnrecognized}},
		{"or combination", `hasRole('A') || hasAuthority('B')`, spelResult{Kind: spelUnrecognized}},
		// ADR 0035 §2. A bean call whose arguments are all quoted
		// literals is read as a permission; every other bean-call shape
		// keeps today's behaviour.
		{"bean call with one literal", `@ss.hasPermi('system:user:edit')`, spelResult{Kind: spelPermissions, Permissions: []string{"system:user:edit"}, Via: "@ss.hasPermi"}},
		{"bean call with two literals", `@el.check('user:list','dept:list')`, spelResult{Kind: spelPermissions, Permissions: []string{"user:list", "dept:list"}, Via: "@el.check"}},
		{"bean call with three literals and spaces", `@el.check('roles:list', 'user:add', 'user:edit')`, spelResult{Kind: spelPermissions, Permissions: []string{"roles:list", "user:add", "user:edit"}, Via: "@el.check"}},
		{"bean method call with #param", `@authService.check(#id)`, spelResult{Kind: spelUnrecognized}},
		// eladmin's form B: the requirement is real (its varargs
		// implementation means "requires admin"), and an unreadable
		// argument list is never an empty requirement — ADR 0020
		// Amendment 3 §9 applied to the new term.
		{"bean call with no arguments", `@el.check()`, spelResult{Kind: spelUnrecognized}},
		// A negated requirement is not a requirement (ADR 0035 §2).
		// apollo writes exactly this shape once.
		{"negated bean call", `!@validator.shouldHide('x')`, spelResult{Kind: spelUnrecognized}},
		{"bean call inside a boolean combination", `hasRole('A') or @ss.hasPermi('b')`, spelResult{Kind: spelUnrecognized}},
		{"bean reference with no method", `@ss('x')`, spelResult{Kind: spelUnrecognized}},
		{"bean call with a mixed argument list", `@el.check('a', #b)`, spelResult{Kind: spelUnrecognized}},
		{"empty string", ``, spelResult{Kind: spelUnrecognized}},
		{"hasRole empty args", `hasRole()`, spelResult{Kind: spelUnrecognized}},
		{"hasRole unquoted arg", `hasRole(ADMIN)`, spelResult{Kind: spelUnrecognized}},
		{"hasRole malformed quote", `hasRole('ADMIN)`, spelResult{Kind: spelUnrecognized}},
		// THE SIGIL BOUNDARY, and half of a regression pair. This is
		// Spring Security's own built-in hasPermission(...) expression,
		// which ADR 0035 §2 deliberately does not read — zero uses in the
		// 20-repository corpus. Its other half is the bean call named
		// hasPermission that the vendored ruoyi-vue-pro fixture really
		// writes, pinned by TestExtract_YudaoFixturePermissions. The
		// leading "@" is the only thing separating them, so neither case
		// may be dropped without the other losing its meaning.
		{"built-in hasPermission is not read", `hasPermission('ADMIN')`, spelResult{Kind: spelUnrecognized}},
		{"a BEAN named hasPermission is read", `@ss.hasPermission('member:user:update')`, spelResult{Kind: spelPermissions, Permissions: []string{"member:user:update"}, Via: "@ss.hasPermission"}},
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
