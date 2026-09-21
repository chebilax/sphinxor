package spring

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chebilax/sphinxor/internal/model"
)

// methodSecurityFilterAnnotations are Spring Security's collection-filtering
// annotations — docs/decisions/0030-post-authorize-and-method-security-filters.md §4.
//
// They are counted, never recorded against an endpoint. Neither one denies
// a call: a caller with no matching authority invokes the method
// successfully and gets a shorter collection back. Treating them as
// authorization would mark an endpoint's Guards column ? and suppress
// mutating-endpoint-without-access-control on a mutation nothing stops.
var methodSecurityFilterAnnotations = map[string]bool{
	"PreFilter":  true,
	"PostFilter": true,
}

// scanMethodSecurityFilters counts @PreFilter/@PostFilter in one file and
// records the declaring classes, OR-accumulating into status across the
// project the same way scanMethodSecurityStatus does.
//
// Identity is by import, per ADR 0022 §1, with the fully-qualified
// spelling handled per ADR 0025 §3 — the same two paths the guard
// dispatch uses, because "PostFilter" is a plausible method name (shenyu
// has definitionPostFilter) and a bare name match would count it.
func scanMethodSecurityFilters(root *sitter.Node, src []byte, status *model.MethodSecurityFilterStatus) {
	imports := parseImports(root, src)
	seen := map[string]bool{}
	for _, c := range status.Classes {
		seen[c] = true
	}

	var walk func(n *sitter.Node, enclosing string)
	walk = func(n *sitter.Node, enclosing string) {
		if n.Type() == "class_declaration" {
			if name := n.ChildByFieldName("name"); name != nil {
				enclosing = name.Content(src)
			}
		}
		if n.Type() == "method_declaration" {
			for _, ann := range annotationsOf(n, src) {
				if !methodSecurityFilterAnnotations[ann.Name] || !isSpringFilter(ann, imports) {
					continue
				}
				switch ann.Name {
				case "PreFilter":
					status.PreFilter++
				case "PostFilter":
					status.PostFilter++
				}
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

// isSpringFilter reports whether this use of @PreFilter/@PostFilter is
// bound to Spring Security's prepost package, either by a fully-qualified
// spelling or by the file's imports.
func isSpringFilter(ann annotationCall, imports importTable) bool {
	const pkg = "org.springframework.security.access.prepost"
	if ann.Qualifier != "" {
		return ann.Qualifier == pkg
	}
	_, accepted := imports.resolveAnnotation(ann.Name)
	return accepted
}
