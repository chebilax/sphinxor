package nestjs

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// versionNeutral is NestJS's own constant for a route that matches
// regardless of the requested version. It is a *declared absence* of a
// version discriminator, not an unreadable value: recognizing it by name
// is the same heuristic detectGlobalGuards already applies to APP_GUARD.
const versionNeutral = "VERSION_NEUTRAL"

// versionDecl is the outcome of reading a route's API version, per
// docs/decisions/0020-unanalyzable-is-unknown-not-absent.md Amendment 2 §7.
//
// The three states are the amendment's own vocabulary. `declared == false`
// is *absent* — no version anywhere, so the endpoint keeps exactly the
// path-derived identity it had before §7. `declared && resolved` is a
// readable version that becomes part of the identity. `declared &&
// !resolved` is *unknown* — a constant reference or an array of them,
// which must not be assumed equal to any other unknown.
type versionDecl struct {
	declared bool
	value    string
	resolved bool
}

// unknownVersion reports whether a version was declared but could not be
// read — the state that forces a synthesized identity.
func (v versionDecl) unknownVersion() bool { return v.declared && !v.resolved }

// controllerVersion reads the `version` key out of the object form of
// @Controller({ path, version }). The string form, @Controller('/x'),
// declares no version at all.
func controllerVersion(args *sitter.Node, src []byte) versionDecl {
	argNodes := argumentNodes(args)
	if len(argNodes) == 0 || argNodes[0].Type() != "object" {
		return versionDecl{}
	}
	for _, pair := range namedChildren(argNodes[0]) {
		if pair.Type() != "pair" {
			continue
		}
		key := pair.ChildByFieldName("key")
		if key == nil || key.Content(src) != "version" {
			continue
		}
		return resolveVersionArg(pair.ChildByFieldName("value"), src)
	}
	return versionDecl{}
}

// decoratorVersion reads a @Version(...) decorator out of a run of
// decorators — the method- (or class-) level form, which overrides the
// controller's own version where both are present.
func decoratorVersion(decorators []*sitter.Node, src []byte) versionDecl {
	for _, d := range decorators {
		call, ok := parseDecorator(d, src)
		if !ok || call.Name != "Version" {
			continue
		}
		args := argumentNodes(call.Args)
		if len(args) == 0 {
			return versionDecl{declared: true}
		}
		return resolveVersionArg(args[0], src)
	}
	return versionDecl{}
}

// resolveVersionArg resolves one version argument: a string literal, an
// array of them, or NestJS's VERSION_NEUTRAL. Anything else — a constant
// reference, an array of references, a computed value — is declared but
// unreadable, and stays unknown rather than being guessed at.
func resolveVersionArg(n *sitter.Node, src []byte) versionDecl {
	if n == nil {
		return versionDecl{declared: true}
	}
	if v, ok := stringLiteralValue(n, src); ok {
		return versionDecl{declared: true, value: v, resolved: true}
	}
	if n.Type() == "identifier" && n.Content(src) == versionNeutral {
		// Matches every version, so it discriminates nothing: absent.
		return versionDecl{}
	}
	if n.Type() == "array" {
		var values []string
		for _, el := range namedChildren(n) {
			v, ok := stringLiteralValue(el, src)
			if !ok {
				return versionDecl{declared: true}
			}
			values = append(values, v)
		}
		if len(values) == 0 {
			return versionDecl{declared: true}
		}
		// A controller serving several versions is one endpoint per
		// declaration, keyed by the whole set it answers for.
		return versionDecl{declared: true, value: strings.Join(values, ","), resolved: true}
	}
	return versionDecl{declared: true}
}
