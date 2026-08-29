package spring

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// namedChildren returns n's named children as a slice.
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

// findChildByType returns the first named child of n with the given type,
// or nil. tree-sitter-java's `modifiers` node (holding a declaration's
// annotations) has no field name of its own — confirmed against the real
// grammar (ChildByFieldName("modifiers") returns nil on both
// class_declaration and method_declaration) — so it has to be found by
// type, not by field.
func findChildByType(n *sitter.Node, t string) *sitter.Node {
	if n == nil {
		return nil
	}
	for i := 0; i < int(n.NamedChildCount()); i++ {
		if c := n.NamedChild(i); c.Type() == t {
			return c
		}
	}
	return nil
}

// annotationCall describes one `@Name` (marker_annotation, no arguments) or
// `@Name(...)` (annotation, with an annotation_argument_list) node.
type annotationCall struct {
	Name string
	Args *sitter.Node // the `annotation_argument_list` node; nil for a marker annotation
	Node *sitter.Node
}

// parseAnnotation extracts the name and arguments from a `marker_annotation`
// or `annotation` node. ok is false for any other node type.
func parseAnnotation(n *sitter.Node, src []byte) (annotationCall, bool) {
	if n == nil {
		return annotationCall{}, false
	}
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return annotationCall{}, false
	}
	switch n.Type() {
	case "marker_annotation":
		return annotationCall{Name: nameNode.Content(src), Node: n}, true
	case "annotation":
		return annotationCall{Name: nameNode.Content(src), Args: n.ChildByFieldName("arguments"), Node: n}, true
	default:
		return annotationCall{}, false
	}
}

// annotationsOf returns every marker_annotation/annotation found directly
// under decl's `modifiers` node (present whenever decl carries at least one
// annotation or modifier keyword like `public`), or nil if decl has none.
func annotationsOf(decl *sitter.Node, src []byte) []annotationCall {
	mods := findChildByType(decl, "modifiers")
	if mods == nil {
		return nil
	}
	var out []annotationCall
	for _, c := range namedChildren(mods) {
		if call, ok := parseAnnotation(c, src); ok {
			out = append(out, call)
		}
	}
	return out
}

// findAnnotation returns the first annotation among anns named name.
func findAnnotation(anns []annotationCall, name string) (annotationCall, bool) {
	for _, a := range anns {
		if a.Name == name {
			return a, true
		}
	}
	return annotationCall{}, false
}

// stringLiteralValue returns a `string_literal` node's content with its
// quotes removed, via its `string_fragment` child. An empty string literal
// (`""`) has no string_fragment child, so ok is true with an empty value in
// that case — mirrors internal/extract/nestjs's identical handling of
// tree-sitter-typescript's `string`/`string_fragment` shape.
func stringLiteralValue(n *sitter.Node, src []byte) (value string, ok bool) {
	if n == nil || n.Type() != "string_literal" {
		return "", false
	}
	if n.NamedChildCount() == 0 {
		return "", true
	}
	return n.NamedChild(0).Content(src), true
}

// pathAttributeValue extracts an endpoint-mapping annotation's path, e.g.
// `@RequestMapping("/api/suppliers")` (a bare positional string_literal) or
// `@GetMapping(path = "/categories")` / `@GetMapping(value = "/categories")`
// (an element_value_pair — Spring's mapping annotations alias `value()` and
// `path()` to the same attribute). Returns ok=false if args is nil (a
// marker annotation, i.e. no path given at all) or no recognized shape is
// found — the caller treats that as "no sub-path", not an error.
func pathAttributeValue(args *sitter.Node, src []byte) (path string, ok bool) {
	if args == nil {
		return "", false
	}
	for _, arg := range namedChildren(args) {
		switch arg.Type() {
		case "string_literal":
			// Bare positional argument: @RequestMapping("/x"). Only valid
			// as the sole/first argument in real Spring usage.
			return stringLiteralValue(arg, src)
		case "element_value_pair":
			key := arg.ChildByFieldName("key")
			if key == nil || (key.Content(src) != "path" && key.Content(src) != "value") {
				continue
			}
			return stringLiteralValue(arg.ChildByFieldName("value"), src)
		}
	}
	return "", false
}

// joinPath combines a controller's base path with a route's own path into a
// single, normalized leading-slash path — identical logic to
// internal/extract/nestjs's joinPath; the concept has nothing
// framework-specific about it.
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
