package spring

import "strings"

// matchesAntPattern reports whether path matches pattern, using the
// bounded Ant-style subset docs/decisions/0012-securityfilterchain-effective-policy.md
// §1 recognizes: a literal segment matches only itself; `*` matches
// exactly one path segment; `**` matches zero or more path segments;
// `{name}` (Spring's path-variable placeholder, used here as a wildcard)
// matches exactly one path segment. No regex, no character classes, no
// `AntPathMatcher`/`PathPatternParser` parity beyond this set — an
// unrecognized pattern shape simply doesn't match, the same "can't parse
// it, don't guess" default used everywhere else in this project.
//
// path is an extracted Endpoint.Path, which may itself contain a
// `{var}` segment (a route's own path variable, e.g. "/api/customers/{id}").
// A pattern's literal segment is compared against such a path segment by
// plain string equality — it matches only when the pattern spells out the
// exact same placeholder text (e.g. both write `{id}`), which is a real,
// valid Spring pattern. A pattern literal can never match a *different*
// path variable name, and deliberately doesn't try to reason about
// whether they'd coincide for some runtime value — the conservative,
// omit-rather-than-guess direction, achieved here for free by ordinary
// string comparison rather than by any special-cased logic.
func matchesAntPattern(pattern, path string) bool {
	patSegs := splitPath(pattern)
	pathSegs := splitPath(path)
	return matchSegments(patSegs, pathSegs)
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func matchSegments(pat, path []string) bool {
	if len(pat) == 0 {
		return len(path) == 0
	}
	switch pat[0] {
	case "**":
		if matchSegments(pat[1:], path) {
			return true
		}
		if len(path) == 0 {
			return false
		}
		return matchSegments(pat, path[1:])
	case "*":
		if len(path) == 0 {
			return false
		}
		return matchSegments(pat[1:], path[1:])
	default:
		if len(path) == 0 {
			return false
		}
		if isPlaceholderSegment(pat[0]) {
			return matchSegments(pat[1:], path[1:])
		}
		if pat[0] != path[0] {
			return false
		}
		return matchSegments(pat[1:], path[1:])
	}
}

func isPlaceholderSegment(s string) bool {
	return len(s) >= 2 && s[0] == '{' && s[len(s)-1] == '}'
}
