package spring

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chebilax/sphinxor/internal/model"
)

// scanPermitAll counts JSR-250 @PermitAll annotations on classes and
// methods — docs/decisions/0012-securityfilterchain-effective-policy.md
// Amendment 1.
//
// @PermitAll is Spring Security method security (the jsr250Enabled family,
// the package of @RolesAllowed), so ADR 0029 §3 does not allow it to be
// silent. What it should mean for empty-role, for
// mutating-endpoint-without-access-control and for the became-public gate
// is not decided, so it is counted and announced and nothing else. Identity
// is import-based (ADR 0022 §1), as for every recognized annotation: a
// project's own @PermitAll is not counted.
func scanPermitAll(root *sitter.Node, src []byte, status *model.PermitAllStatus) {
	imports := parseImports(root, src)
	seen := map[string]bool{}
	for _, c := range status.Classes {
		seen[c] = true
	}
	var walk func(n *sitter.Node, enclosing string)
	walk = func(n *sitter.Node, enclosing string) {
		switch n.Type() {
		case "class_declaration":
			if name := n.ChildByFieldName("name"); name != nil {
				enclosing = name.Content(src)
			}
			fallthrough
		case "method_declaration":
			for _, ann := range annotationsOf(n, src) {
				if ann.Name != "PermitAll" || !isJSR250(ann, imports) {
					continue
				}
				status.Count++
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

// isJSR250 reports whether ann binds to the JSR-250 package, written
// qualified or through an import.
func isJSR250(ann annotationCall, imports importTable) bool {
	if ann.Qualifier != "" {
		_, kind := resolveQualifiedAuth(ann.Name, ann.Qualifier)
		return kind == "spring"
	}
	_, accepted := imports.resolveAnnotation(ann.Name)
	return accepted
}
