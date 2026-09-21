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
	// Name is always the annotation's SIMPLE name, whether it was
	// written `@RestController` or
	// `@org.springframework.web.bind.annotation.RestController` —
	// docs/decisions/0025-qualified-annotation-names.md §1.
	//
	// microcks writes the second form in 13 files, because its own class
	// in io.github.microcks.web is named RestController and Spring's
	// cannot be imported there. Matching the text as written meant none
	// of those classes was a controller.
	Name string
	// Qualifier is the package part of a fully-qualified use
	// ("org.springframework.web.bind.annotation"), empty when the
	// annotation was written unqualified.
	//
	// It is kept because the simple name alone cannot say whether a
	// dotted annotation is Spring's: §4's whole-path rule and §3's
	// "a qualified use is its own binding" both need it.
	Qualifier string
	Args      *sitter.Node // the `annotation_argument_list` node; nil for a marker annotation
	Node      *sitter.Node
}

// springWebPackages are the packages that make a qualified controller or
// mapping annotation Spring's, per ADR 0025 §4.
//
// The whole path must match. Accepting any dotted name by its last
// segment would read `@com.example.RestController` as Spring's — the
// nacos collision (ADR 0022) reintroduced by the change meant to fix its
// mirror image.
var springWebPackages = map[string]bool{
	"org.springframework.web.bind.annotation": true,
	"org.springframework.stereotype":          true,
}

// isSpringWeb reports whether this annotation can be Spring's controller
// or mapping annotation: either written unqualified, or qualified with a
// package that really is Spring's.
//
// The corpus's only short dotted annotation names are @lombok.Data and
// @feign.Headers, and those are complete paths rather than truncations —
// which is exactly why the check is on the whole path. A truncated
// `@annotation.RestController` and a complete `@lombok.Data` are
// indistinguishable to a matcher looking at the last segment.
func (a annotationCall) isSpringWeb() bool {
	return a.Qualifier == "" || springWebPackages[a.Qualifier]
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
	name, qualifier := splitAnnotationName(nameNode.Content(src))
	switch n.Type() {
	case "marker_annotation":
		return annotationCall{Name: name, Qualifier: qualifier, Node: n}, true
	case "annotation":
		return annotationCall{Name: name, Qualifier: qualifier, Args: n.ChildByFieldName("arguments"), Node: n}, true
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
//
// Used for Spring's own @RequestMapping and for @AliasFor, so a qualified
// use has to be qualified with a Spring package to count (ADR 0025 §4).
// @AliasFor lives in org.springframework.core.annotation rather than the
// web packages, and has no qualified use anywhere in the corpus; it is
// matched unqualified only, which is what isSpringWeb already allows.
func findAnnotation(anns []annotationCall, name string) (annotationCall, bool) {
	for _, a := range anns {
		if a.Name == name && a.isSpringWeb() {
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

// splitAnnotationName separates a written annotation name into its simple
// name and its package qualifier — ADR 0025 §1.
//
// tree-sitter-java gives the name node as an `identifier` for `@Name` and
// a `scoped_identifier` for `@a.b.C`, so the split is on the last dot.
func splitAnnotationName(written string) (name, qualifier string) {
	if i := strings.LastIndexByte(written, '.'); i >= 0 {
		return written[i+1:], written[:i]
	}
	return written, ""
}
