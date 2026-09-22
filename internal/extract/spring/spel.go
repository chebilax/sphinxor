package spring

import (
	"strings"
)

// spelKind classifies what a recognized @PreAuthorize SpEL expression
// establishes, per docs/decisions/0011-spring-second-framework.md §1/§2.
type spelKind int

const (
	// spelUnrecognized covers any boolean combination, any bean method
	// call, or anything else outside the small recognized set below —
	// the guard is still real (its presence is evidence), but its role
	// list was *not read*, which is a different state from being read
	// and found empty. Callers mark it RolesUnresolved, per
	// docs/decisions/0020-unanalyzable-is-unknown-not-absent.md
	// Amendment 3 §9.
	spelUnrecognized spelKind = iota
	// spelRoles is hasRole/hasAnyRole/hasAuthority/hasAnyAuthority — one
	// or more role/authority literals.
	spelRoles
	// spelAuthenticated is the literal isAuthenticated() predicate,
	// establishing "authenticated, any role" (ADR 0010) via the URL-less,
	// method-layer path (docs/decisions/0011-spring-second-framework.md §2).
	spelAuthenticated
	// spelNoRole is permitAll() or denyAll(): recognized expressions that
	// resolve to no role list, as opposed to a role list that could not
	// be read.
	//
	// Naming them is what keeps ADR 0017's settled boundary intact under
	// Amendment 3. ADR 0017 decided deliberately that permitAll() keeps
	// DeclaresRoles: true and "still surfaces through empty-role", on the
	// reasoning that a developer's permitAll() could itself be the
	// mistake; TestExtractControllers_PermitAllStillDeclaresRoles pins
	// it. Had these two stayed in spelUnrecognized they would have been
	// swept into RolesUnresolved and silently stopped firing — reversing
	// a documented decision as a side effect of an unrelated fix.
	// Amendment 3 §10 leaves whether empty-role should fire on
	// permitAll() at all as an open question for its own ADR.
	spelNoRole
	// spelPermissions is a whole-expression call to a Spring BEAN whose
	// arguments are all quoted literals — @ss.hasPermi('system:user:edit'),
	// @el.check('user:list','dept:list') —
	// docs/decisions/0035-permissions-in-the-model.md §2. The literals are
	// named requirements that are not roles, and Permissions carries them
	// in written order.
	//
	// 205 sites across the corpus, in RuoYi-Vue and eladmin. Every other
	// bean-call shape stays spelUnrecognized and keeps today's behaviour:
	// no arguments (73 sites), any non-literal argument (77 sites), a
	// negation, or a boolean combination.
	spelPermissions
)

// spelResult is what parseSpEL recognized in one @PreAuthorize string.
type spelResult struct {
	Kind  spelKind
	Roles []string // populated only for spelRoles, in written order
	// Permissions is populated only for spelPermissions, in written
	// order, and Via is the bean call that named them, verbatim
	// ("@ss.hasPermi"). Via is not decoration: ADR 0035 §3 does not
	// interpret the bean, so the callee has to travel with the literal
	// all the way to the matrix.
	Permissions []string
	Via         string
}

// spelRoleFuncs are the recognized role/authority-bearing SpEL calls, per
// ADR 0011 §1 — deliberately small, not a general SpEL parser.
var spelRoleFuncs = map[string]bool{
	"hasRole":         true,
	"hasAnyRole":      true,
	"hasAuthority":    true,
	"hasAnyAuthority": true,
}

// parseSpEL recognizes a small, explicit set of whole-expression SpEL call
// shapes inside a @PreAuthorize string: hasRole('X'), hasAnyRole('X','Y'),
// hasAuthority('X'), hasAnyAuthority('X','Y'), isAuthenticated(), and
// permitAll()/denyAll(). Anything else — a bean method call, a boolean
// combination, a comparison against #parameter or authentication.name,
// extra whitespace-separated junk after the call — is deliberately left
// spelUnrecognized rather than partially parsed; ADR 0011 §1 is explicit
// that this is not a full SpEL parser and never guesses on an ambiguous or
// unfamiliar shape.
func parseSpEL(expr string) spelResult {
	expr = strings.TrimSpace(expr)

	name, argsText, ok := splitCall(expr)
	if !ok {
		return spelResult{Kind: spelUnrecognized}
	}

	if name == "isAuthenticated" && argsText == "" {
		return spelResult{Kind: spelAuthenticated}
	}

	if (name == "permitAll" || name == "denyAll") && argsText == "" {
		return spelResult{Kind: spelNoRole}
	}

	// ADR 0035 §2: a bean call. The leading @ is Spring's bean-reference
	// sigil and is the ONLY thing separating a project bean whose method
	// happens to be called hasPermission from Spring Security's own
	// built-in hasPermission(...) expression, which this parser
	// deliberately does not read. The vendored ruoyi-vue-pro fixture
	// writes @ss.hasPermission('member:user:update') and spel_test.go
	// pins bare hasPermission('ADMIN') as unrecognized; those two are a
	// regression pair and neither may be dropped.
	//
	// splitCall has already anchored the call at both ends, so a boolean
	// combination containing a bean call does not reach here. A negated
	// call, !@bean.method('x'), fails isBeanReference because the name
	// starts with '!' rather than '@' — deliberately, since a negated
	// requirement is not a requirement (ADR 0035 §2).
	if isBeanReference(name) {
		perms, ok := parseQuotedArgList(argsText)
		if !ok || len(perms) == 0 {
			// No arguments, or an argument that is not a clean literal.
			// NOT an empty requirement: eladmin's @el.check() means
			// "requires admin", because its varargs implementation
			// short-circuits on an empty array. Unread, exactly as
			// before this decision (ADR 0020 Amendment 3 §9).
			return spelResult{Kind: spelUnrecognized}
		}
		return spelResult{Kind: spelPermissions, Permissions: perms, Via: name}
	}

	if !spelRoleFuncs[name] {
		return spelResult{Kind: spelUnrecognized}
	}

	roles, ok := parseQuotedArgList(argsText)
	if !ok || len(roles) == 0 {
		return spelResult{Kind: spelUnrecognized}
	}
	return spelResult{Kind: spelRoles, Roles: roles}
}

// splitCall matches expr against the whole-string shape `name(argsText)` —
// anchored at both ends, so "hasRole('A') and #x" (a boolean combination)
// correctly fails to match rather than matching just its hasRole('A')
// prefix.
func splitCall(expr string) (name, argsText string, ok bool) {
	open := strings.IndexByte(expr, '(')
	if open == -1 || !strings.HasSuffix(expr, ")") {
		return "", "", false
	}
	name = expr[:open]
	// A plain identifier (hasRole, isAuthenticated) or a bean reference
	// (@ss.hasPermi). Nothing else: a name carrying anything but those
	// shapes — a leading "!", an operator, stray text — is not a call
	// this parser claims to understand, and the whole expression stays
	// unrecognized rather than being partially read (ADR 0011 §1).
	if !isIdentifier(name) && !isBeanReference(name) {
		return "", "", false
	}
	argsText = strings.TrimSpace(expr[open+1 : len(expr)-1])
	return name, argsText, true
}

// isBeanReference reports whether name is a Spring bean reference of the
// form @bean.method or @bean.nested.method — an "@" followed by at least
// two dot-separated identifiers.
//
// The dot is required. A bare "@bean" is not a call this parser
// understands, and accepting one would mean treating the bean itself as
// the requirement.
func isBeanReference(name string) bool {
	if !strings.HasPrefix(name, "@") {
		return false
	}
	parts := strings.Split(name[1:], ".")
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if !isIdentifier(p) {
			return false
		}
	}
	return true
}

func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// parseQuotedArgList splits a comma-separated list of single-quoted SpEL
// string literals, e.g. `'ADMIN', 'PHARMACIST'` -> ["ADMIN", "PHARMACIST"].
// ok is false if any piece isn't a clean, balanced 'text' literal — a
// partial match here would mean guessing at an argument list this project
// doesn't actually understand (e.g. a nested expression, a bean reference),
// exactly what ADR 0011 §1 rules out.
func parseQuotedArgList(argsText string) ([]string, bool) {
	if argsText == "" {
		return nil, false
	}
	parts := strings.Split(argsText, ",")
	roles := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len(p) < 2 || p[0] != '\'' || p[len(p)-1] != '\'' {
			return nil, false
		}
		inner := p[1 : len(p)-1]
		if strings.ContainsRune(inner, '\'') {
			return nil, false
		}
		roles = append(roles, inner)
	}
	return roles, true
}
