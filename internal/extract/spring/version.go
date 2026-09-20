package spring

import (
	sitter "github.com/smacker/go-tree-sitter"
)

// versionDecl is the outcome of reading a route's API version, per
// docs/decisions/0020-unanalyzable-is-unknown-not-absent.md Amendment 2 §7.
// It mirrors the NestJS twin (internal/extract/nestjs/version.go); the
// three states are the amendment's own vocabulary — absent, readable, and
// declared-but-unknown.
type versionDecl struct {
	declared bool
	value    string
	resolved bool
}

// unknownVersion reports whether a version was declared but could not be
// read — the state that forces a synthesized identity.
func (v versionDecl) unknownVersion() bool { return v.declared && !v.resolved }

// versionAttributeValue reads the `version` attribute out of a mapping
// annotation's arguments — Spring Framework 7 / Boot 4 API versioning,
// e.g. @GetMapping(value = "/{id}", version = "1.0").
//
// Only the string-literal form resolves. A constant reference is declared
// but unreadable, and stays unknown rather than being guessed at — the
// same treatment pathAttributeValue's caller already gives an unreadable
// path.
func versionAttributeValue(args *sitter.Node, src []byte) versionDecl {
	if args == nil {
		return versionDecl{}
	}
	for _, arg := range namedChildren(args) {
		if arg.Type() != "element_value_pair" {
			continue
		}
		key := arg.ChildByFieldName("key")
		if key == nil || key.Content(src) != "version" {
			continue
		}
		if v, ok := stringLiteralValue(arg.ChildByFieldName("value"), src); ok {
			return versionDecl{declared: true, value: v, resolved: true}
		}
		return versionDecl{declared: true}
	}
	return versionDecl{}
}
