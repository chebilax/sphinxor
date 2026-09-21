package spring

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chebilax/sphinxor/internal/model"
)

// scanRoleHierarchy records whether this file declares a Spring Security
// role hierarchy — docs/decisions/0031-role-hierarchy.md §1/§3.
//
// Three shapes are looked for, per §3:
//
//   - a @Bean method whose return type is RoleHierarchy;
//   - a RoleHierarchyImpl.fromHierarchy(...) / .withDefaultRolePrefix()
//     call, which is how one is built in 6.x;
//   - a setRoleHierarchy(...) call on an expression handler.
//
// Only the EXISTENCE is recorded. The rules inside — ROLE_ADMIN >
// ROLE_USER — are not read, and no grant is expanded: that would change
// reported grants rather than annotate them, reopening ADR 0011 §1's
// scope cut, and ADR 0031 keeps it a separate decision.
//
// What this misses is stated rather than implied: a hierarchy configured
// in YAML, in properties, or in Kotlin. Both are already out of scope
// (ADR 0011 §1), and neither becomes newly hidden by this scan — but a
// project without the warning is "none located", never "confirmed none",
// the same boundary ADR 0015 draws for @EnableMethodSecurity.
func scanRoleHierarchy(root *sitter.Node, src []byte, status *model.RoleHierarchyStatus) {
	seen := map[string]bool{}
	for _, c := range status.DeclaredIn {
		seen[c] = true
	}

	record := func(class string) {
		status.Found = true
		if class != "" && !seen[class] {
			seen[class] = true
			status.DeclaredIn = append(status.DeclaredIn, class)
		}
	}

	var walk func(n *sitter.Node, enclosing string)
	walk = func(n *sitter.Node, enclosing string) {
		switch n.Type() {
		case "class_declaration":
			if name := n.ChildByFieldName("name"); name != nil {
				enclosing = name.Content(src)
			}
		case "method_declaration":
			// A @Bean method returning RoleHierarchy. The return type is
			// the declaration, so this catches the bean however its body
			// is written.
			if t := n.ChildByFieldName("type"); t != nil && isRoleHierarchyType(t.Content(src)) {
				if hasAny(annotationsOf(n, src), map[string]bool{"Bean": true}) {
					record(enclosing)
				}
			}
		case "method_invocation":
			if name := n.ChildByFieldName("name"); name != nil {
				switch name.Content(src) {
				case "fromHierarchy", "withDefaultRolePrefix":
					// Only when invoked on RoleHierarchyImpl — these are
					// generic enough names that the receiver matters.
					if obj := n.ChildByFieldName("object"); obj != nil &&
						strings.Contains(obj.Content(src), "RoleHierarchy") {
						record(enclosing)
					}
				case "setRoleHierarchy":
					record(enclosing)
				}
			}
		}
		for _, c := range namedChildren(n) {
			walk(c, enclosing)
		}
	}
	walk(root, "")
}

// isRoleHierarchyType reports whether a declared return type is Spring's
// RoleHierarchy, written plainly or fully qualified.
func isRoleHierarchyType(text string) bool {
	text = strings.TrimSpace(text)
	return text == "RoleHierarchy" ||
		text == "org.springframework.security.access.hierarchicalroles.RoleHierarchy"
}
