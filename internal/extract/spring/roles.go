package spring

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chebilax/sphinxor/internal/model"
)

// collectUsedRoleLiterals scans root for every role/authority literal
// referenced by a recognized method-security annotation — the SpEL
// role-bearing calls inside @PreAuthorize (spel.go), plus @Secured's and
// @RolesAllowed's own plain string-array arguments. Mirrors
// internal/extract/nestjs's usedEnumNames filter (roles.go there): a real
// project has role-unrelated enums and constants, and restricting
// role-declaration extraction to literals actually referenced somewhere
// avoids treating every String constant or enum in the project as a
// candidate role — verified as real noise for NestJS
// (internal/extract/nestjs/roles.go's own doc comment), not assumed to
// generalize without being named here too.
func collectUsedRoleLiterals(root *sitter.Node, src []byte) map[string]bool {
	used := make(map[string]bool)
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "class_declaration" || n.Type() == "method_declaration" {
			for _, ann := range annotationsOf(n, src) {
				for _, lit := range roleLiteralsOf(ann, src) {
					used[lit] = true
				}
			}
		}
		for _, c := range namedChildren(n) {
			walk(c)
		}
	}
	walk(root)
	return used
}

// roleLiteralsOf returns the role/authority literal strings one
// annotation contributes, if it's a recognized method-security
// annotation with a recognized role-bearing shape.
func roleLiteralsOf(ann annotationCall, src []byte) []string {
	switch ann.Name {
	case "PreAuthorize":
		lit, ok := stringLiteralValue(soleStringLiteralArg(ann.Args), src)
		if !ok {
			return nil
		}
		result := parseSpEL(lit)
		if result.Kind != spelRoles {
			return nil
		}
		return result.Roles
	case "Secured", "RolesAllowed":
		return stringArrayValues(ann.Args, src)
	default:
		return nil
	}
}

// soleStringLiteralArg returns args' single positional string_literal
// argument (the shape every real @PreAuthorize in the vendored fixtures
// uses), or nil if args isn't exactly that shape.
func soleStringLiteralArg(args *sitter.Node) *sitter.Node {
	if args == nil || args.NamedChildCount() != 1 {
		return nil
	}
	if first := args.NamedChild(0); first.Type() == "string_literal" {
		return first
	}
	return nil
}

// stringArrayValues reads @Secured/@RolesAllowed's argument: either a bare
// positional string_literal (`@Secured("ROLE_ADMIN")`, Java's single-element
// array shorthand) or an element_value_array_initializer of string_literals
// (`@Secured({"ROLE_ADMIN", "ROLE_MANAGER"})`) — confirmed against a real
// tree-sitter-java parse before writing this, not assumed from the
// annotation's Java API shape alone. Any element that isn't a plain string
// literal is skipped, not guessed at.
func stringArrayValues(args *sitter.Node, src []byte) []string {
	if args == nil || args.NamedChildCount() != 1 {
		return nil
	}
	arg := args.NamedChild(0)
	switch arg.Type() {
	case "string_literal":
		if v, ok := stringLiteralValue(arg, src); ok {
			return []string{v}
		}
		return nil
	case "element_value_array_initializer":
		var out []string
		for _, elem := range namedChildren(arg) {
			if v, ok := stringLiteralValue(elem, src); ok {
				out = append(out, v)
			}
		}
		return out
	default:
		return nil
	}
}

// extractRoleDeclarations finds every RoleDeclaration in root, restricted
// to enum constants and String constants whose exact text appears in
// usedLiterals. Two different kinds, two different matches against a bare
// SpEL/annotation-array literal — docs/decisions/0016-spel-role-declaration-heuristic-resolution.md:
//   - A plain Java enum constant has no separate backing value, so a match
//     is by the constant's own NAME (`enum Role { ADMIN }` matches the
//     literal "ADMIN").
//   - A `public static final String` constant's whole point is a name/value
//     split, so a match is by its VALUE, not its Java field identifier —
//     `public static final String ADMIN_ROLE = "ADMIN"` matches the literal
//     "ADMIN", not "ADMIN_ROLE".
func extractRoleDeclarations(root *sitter.Node, src []byte, file string, next func() model.ID, usedLiterals map[string]bool) []model.RoleDeclaration {
	var out []model.RoleDeclaration
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "enum_declaration":
			out = append(out, enumRoleDeclarations(n, src, file, next, usedLiterals)...)
		case "field_declaration":
			if d, ok := constRoleDeclaration(n, src, file, next, usedLiterals); ok {
				out = append(out, d)
			}
		}
		for _, c := range namedChildren(n) {
			walk(c)
		}
	}
	walk(root)
	return out
}

func enumRoleDeclarations(enumDecl *sitter.Node, src []byte, file string, next func() model.ID, usedLiterals map[string]bool) []model.RoleDeclaration {
	body := enumDecl.ChildByFieldName("body")
	if body == nil {
		return nil
	}
	var out []model.RoleDeclaration
	for _, c := range namedChildren(body) {
		if c.Type() != "enum_constant" {
			continue
		}
		nameNode := c.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := nameNode.Content(src)
		if !usedLiterals[name] {
			continue
		}
		out = append(out, model.RoleDeclaration{
			ID:   next(),
			Name: name,
			Kind: model.RoleDeclarationEnum,
			File: file,
			Line: int(c.StartPoint().Row) + 1,
		})
	}
	return out
}

func constRoleDeclaration(fieldDecl *sitter.Node, src []byte, file string, next func() model.ID, usedLiterals map[string]bool) (model.RoleDeclaration, bool) {
	if !hasModifierKeyword(fieldDecl, "static") || !hasModifierKeyword(fieldDecl, "final") {
		return model.RoleDeclaration{}, false
	}
	typeNode := fieldDecl.ChildByFieldName("type")
	if typeNode == nil || typeNode.Type() != "type_identifier" || typeNode.Content(src) != "String" {
		return model.RoleDeclaration{}, false
	}
	decl := fieldDecl.ChildByFieldName("declarator")
	if decl == nil || decl.Type() != "variable_declarator" {
		return model.RoleDeclaration{}, false
	}
	valueNode := decl.ChildByFieldName("value")
	value, ok := stringLiteralValue(valueNode, src)
	if !ok || !usedLiterals[value] {
		return model.RoleDeclaration{}, false
	}
	return model.RoleDeclaration{
		ID:   next(),
		Name: value,
		Kind: model.RoleDeclarationConst,
		File: file,
		Line: int(fieldDecl.StartPoint().Row) + 1,
	}, true
}

// hasModifierKeyword reports whether decl's `modifiers` node contains the
// given plain keyword (e.g. "static", "final") as an unnamed child —
// confirmed against a real tree-sitter-java parse: these keywords are
// unnamed tokens, unlike annotations, so they're invisible to
// NamedChild/NamedChildCount and have to be found via the full,
// unnamed-inclusive child list instead.
func hasModifierKeyword(decl *sitter.Node, keyword string) bool {
	mods := findChildByType(decl, "modifiers")
	if mods == nil {
		return false
	}
	for i := 0; i < int(mods.ChildCount()); i++ {
		if mods.Child(i).Type() == keyword {
			return true
		}
	}
	return false
}
