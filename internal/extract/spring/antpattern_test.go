package spring

import "testing"

// TestMatchesAntPattern covers the bounded pattern-matching rules stated
// in docs/decisions/0012-securityfilterchain-effective-policy.md §1. Pure
// logic, no framework-specific parsing — synthetic per docs/testing.md's
// own principle, though several cases below use the exact real pattern
// strings from the vendored fixtures for grounding.
func TestMatchesAntPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{"exact literal match", "/auth/login", "/auth/login", true},
		{"exact literal mismatch", "/auth/login", "/auth/logout", false},
		{"trailing ** matches nothing extra", "/api/suppliers/**", "/api/suppliers", true},
		{"trailing ** matches one segment", "/api/suppliers/**", "/api/suppliers/5", true},
		{"trailing ** matches many segments", "/api/suppliers/**", "/api/suppliers/5/edit/confirm", true},
		{"** does not match a different prefix", "/api/suppliers/**", "/api/customers", false},
		{"single * matches exactly one segment", "/api/customers/*", "/api/customers/5", true},
		{"single * does not match zero segments", "/api/customers/*", "/api/customers", false},
		{"single * does not match two segments", "/api/customers/*", "/api/customers/5/edit", false},
		{"pattern {var} matches path literal", "/api/customers/{id}", "/api/customers/5", true},
		{"pattern {var} matches path's own {var}, same name", "/api/customers/{id}", "/api/customers/{id}", true},
		{"pattern {var} matches path's own {var}, different name", "/api/customers/{id}", "/api/customers/{otherId}", true},
		{"pattern literal vs path's own {var}: no match (conservative)", "/api/customers/5", "/api/customers/{id}", false},
		{"HttpMethod-scoped pattern, ** over full inventory subtree", "/api/inventory/**", "/api/inventory/restock", true},
		{"root anyRequest-equivalent empty pattern matches root only", "/", "/", true},
		{"leading/trailing slash normalization", "/api/suppliers/**/", "/api/suppliers/5", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesAntPattern(tt.pattern, tt.path); got != tt.want {
				t.Errorf("matchesAntPattern(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

// TestMatchesAntPattern_RealPharmacyRulesAgainstRealEndpoints grounds the
// matcher against the exact pattern strings in Pharmacy's vendored
// SecurityConfig.java and the exact Endpoint.Path values
// TestExtract_Pharmacy already verified real extraction produces — the
// combination this matcher exists to get right (the SupplierController
// mismatch that drove ADR 0012 in the first place).
func TestMatchesAntPattern_RealPharmacyRulesAgainstRealEndpoints(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"/api/suppliers/**", "/api/suppliers", true},
		{"/api/suppliers/**", "/api/customers", false},
		{"/api/users/**", "/api/customers", false},
		{"/api/customers/{id}", "/api/customers/{id}", true},
		{"/api/customers/{id}", "/api/customers", false},
		{"/auth/**", "/auth/login", true},
	}
	for _, tt := range tests {
		if got := matchesAntPattern(tt.pattern, tt.path); got != tt.want {
			t.Errorf("matchesAntPattern(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}
