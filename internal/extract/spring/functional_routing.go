package spring

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chebilax/sphinxor/internal/model"
)

// routerFunctionPackage is where Spring WebFlux declares RouterFunction.
// Identity comes from the import, never the spelling (ADR 0033 §5,
// applying ADR 0022 §1 to a type rather than an annotation).
const routerFunctionPackage = "org.springframework.web.reactive.function.server"

// scanFunctionalRouting counts methods in this file whose declared return
// type is RouterFunction — docs/decisions/0033-functional-routing.md §1.
//
// The method is the unit because it is the only one spanning all three
// corpus idioms: a @Bean returning RouterFunction (JeecgBoot, shenyu,
// halo's WebFluxConfig), an implementation of a project interface that
// returns one (halo's CustomEndpoint, ~68 classes), and the interface
// declaration itself. Counting @Bean methods finds 14 of halo's 82
// files; counting RouterFunctions.route( calls misses halo's
// SpringdocRouteBuilder entirely.
//
// The routes inside are deliberately not read (§1), and the count is not
// a route count (§2).
func scanFunctionalRouting(root *sitter.Node, src []byte, status *model.FunctionalRoutingStatus) {
	imports := parseImports(root, src)
	if !bindsRouterFunction(imports) {
		return
	}
	seen := map[string]bool{}
	for _, c := range status.Classes {
		seen[c] = true
	}

	var walk func(n *sitter.Node, enclosing string)
	walk = func(n *sitter.Node, enclosing string) {
		switch n.Type() {
		case "class_declaration", "interface_declaration":
			if name := n.ChildByFieldName("name"); name != nil {
				enclosing = name.Content(src)
			}
		case "method_declaration":
			if returnsRouterFunction(n, src) {
				status.Builders++
				if enclosing != "" && !seen[enclosing] {
					seen[enclosing] = true
					status.Classes = append(status.Classes, enclosing)
				}
			}
		}
		for _, c := range namedChildren(n) {
			walk(c, enclosing)
		}
	}
	walk(root, "")
}

// bindsRouterFunction reports whether this file imports Spring WebFlux's
// RouterFunction, by single-type or on-demand import.
//
// A file that never imports it can still mention the name fully
// qualified, which returnsRouterFunction handles on its own; this is the
// cheap gate for the overwhelming majority of files that mention it not
// at all.
func bindsRouterFunction(t importTable) bool {
	if fqn, ok := t.exact["RouterFunction"]; ok {
		return fqn == routerFunctionPackage+".RouterFunction"
	}
	for _, w := range t.wildcards {
		if w == routerFunctionPackage {
			return true
		}
	}
	return false
}

// returnsRouterFunction reports whether a method declares RouterFunction
// as its return type, with or without type arguments.
func returnsRouterFunction(m *sitter.Node, src []byte) bool {
	t := m.ChildByFieldName("type")
	if t == nil {
		return false
	}
	return isRouterFunctionType(t, src)
}

func isRouterFunctionType(t *sitter.Node, src []byte) bool {
	switch t.Type() {
	case "generic_type":
		// RouterFunction<ServerResponse>: the raw type is the first child.
		kids := namedChildren(t)
		if len(kids) == 0 {
			return false
		}
		return isRouterFunctionType(kids[0], src)
	case "type_identifier":
		return t.Content(src) == "RouterFunction"
	case "scoped_type_identifier":
		text := t.Content(src)
		return text == routerFunctionPackage+".RouterFunction" ||
			strings.HasSuffix(text, ".RouterFunction") && strings.HasPrefix(text, routerFunctionPackage)
	}
	return false
}
