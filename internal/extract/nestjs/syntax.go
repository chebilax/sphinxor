package nestjs

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// namedChildren returns n's named children as a slice, for callers that
// need to look ahead/behind (e.g. pairing decorators with the declaration
// they precede) rather than just iterating.
func namedChildren(n *sitter.Node) []*sitter.Node {
	if n == nil {
		return nil
	}
	out := make([]*sitter.Node, n.NamedChildCount())
	for i := range out {
		out[i] = n.NamedChild(i)
	}
	return out
}

// flattenTopLevel returns program's top-level statements with any
// export_statement wrapper removed, splicing its children in place — an
// exported class/enum's decorators and declaration are children of the
// export_statement, not of program directly.
func flattenTopLevel(program *sitter.Node) []*sitter.Node {
	var out []*sitter.Node
	for _, c := range namedChildren(program) {
		if c.Type() == "export_statement" {
			out = append(out, namedChildren(c)...)
			continue
		}
		out = append(out, c)
	}
	return out
}

// declGroup is a declaration (a class_declaration or method_definition)
// together with the decorator nodes immediately preceding it in source —
// tree-sitter-typescript represents decorators as preceding siblings, not
// as children of the node they decorate.
type declGroup struct {
	decorators []*sitter.Node
	decl       *sitter.Node
}

// groupDecorators walks siblings in source order, attaching each run of
// consecutive `decorator` nodes to the next non-decorator node.
func groupDecorators(siblings []*sitter.Node) []declGroup {
	var groups []declGroup
	var pending []*sitter.Node
	for _, n := range siblings {
		if n.Type() == "decorator" {
			pending = append(pending, n)
			continue
		}
		groups = append(groups, declGroup{decorators: pending, decl: n})
		pending = nil
	}
	return groups
}

// decoratorCall describes one @Name(...) or bare @Name decorator.
type decoratorCall struct {
	Name string
	Args *sitter.Node // the `arguments` node; nil for a bare decorator with no call
	Node *sitter.Node // the decorator node itself, for position info
}

// parseDecorator extracts the callee name and arguments node from a
// `decorator` node, e.g. `@Roles(RoleEnum.admin)` -> Name: "Roles".
// ok is false for shapes this extractor doesn't recognize (e.g. a
// decorator whose callee isn't a plain identifier).
func parseDecorator(n *sitter.Node, src []byte) (decoratorCall, bool) {
	inner := n.NamedChild(0)
	if inner == nil {
		return decoratorCall{}, false
	}
	switch inner.Type() {
	case "call_expression":
		fn := inner.ChildByFieldName("function")
		if fn == nil || fn.Type() != "identifier" {
			return decoratorCall{}, false
		}
		return decoratorCall{Name: fn.Content(src), Args: inner.ChildByFieldName("arguments"), Node: n}, true
	case "identifier":
		return decoratorCall{Name: inner.Content(src), Args: nil, Node: n}, true
	default:
		return decoratorCall{}, false
	}
}

// argumentNodes returns the named argument expressions inside an
// `arguments` node, in order. Safe to call with nil.
func argumentNodes(args *sitter.Node) []*sitter.Node {
	return namedChildren(args)
}

// stringLiteralValue returns a `string` node's content with its quotes
// removed, via its string_fragment child. Empty strings (`”`) have no
// string_fragment child, so ok is true with an empty value in that case.
func stringLiteralValue(n *sitter.Node, src []byte) (value string, ok bool) {
	if n == nil || n.Type() != "string" {
		return "", false
	}
	if n.NamedChildCount() == 0 {
		return "", true
	}
	return n.NamedChild(0).Content(src), true
}

// guardArgName derives a display name for one argument to @UseGuards(...),
// e.g. `RolesGuard` -> "RolesGuard", `AuthGuard('jwt')` -> "AuthGuard" (the
// factory being called; its own arguments aren't guard identity, just
// configuration). Anything else falls back to its raw source text so it's
// still visible in output even if not specially understood.
func guardArgName(n *sitter.Node, src []byte) string {
	switch n.Type() {
	case "identifier":
		return n.Content(src)
	case "call_expression":
		if fn := n.ChildByFieldName("function"); fn != nil {
			return fn.Content(src)
		}
	}
	return n.Content(src)
}

// memberExpressionName returns "Object.property" for a member_expression
// node like `RoleEnum.admin`, or ok=false if n isn't a simple
// identifier.property_identifier member expression.
func memberExpressionName(n *sitter.Node, src []byte) (name string, ok bool) {
	if n == nil || n.Type() != "member_expression" {
		return "", false
	}
	object := n.ChildByFieldName("object")
	property := n.ChildByFieldName("property")
	if object == nil || property == nil || object.Type() != "identifier" {
		return "", false
	}
	return object.Content(src) + "." + property.Content(src), true
}

// controllerBasePath extracts the base path from a @Controller(...)
// decorator's arguments: either a bare string (`@Controller('users')`) or
// an object literal with a `path` property
// (`@Controller({ path: 'users', version: '1' })`), the latter being
// common alongside API versioning.
func controllerBasePath(args *sitter.Node, src []byte) string {
	argNodes := argumentNodes(args)
	if len(argNodes) == 0 {
		return ""
	}
	first := argNodes[0]
	if v, ok := stringLiteralValue(first, src); ok {
		return v
	}
	if first.Type() == "object" {
		for _, pair := range namedChildren(first) {
			if pair.Type() != "pair" {
				continue
			}
			key := pair.ChildByFieldName("key")
			if key == nil || key.Content(src) != "path" {
				continue
			}
			if v, ok := stringLiteralValue(pair.ChildByFieldName("value"), src); ok {
				return v
			}
		}
	}
	return ""
}

// joinPath combines a controller's base path with a route's own path into
// a single, normalized leading-slash path.
func joinPath(base, sub string) string {
	base = strings.Trim(base, "/")
	sub = strings.Trim(sub, "/")
	switch {
	case base == "" && sub == "":
		return "/"
	case base == "":
		return "/" + sub
	case sub == "":
		return "/" + base
	default:
		return "/" + base + "/" + sub
	}
}
