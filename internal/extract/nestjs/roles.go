package nestjs

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chebilax/sphinxor/internal/model"
)

// extractRoleDeclarations finds every TypeScript enum *referenced by at
// least one @Roles() call somewhere in the project* (usedEnumNames) and
// records one RoleDeclaration per member, keyed "EnumName.MemberName" —
// the qualified form a @Roles(EnumName.MemberName) reference uses.
//
// The usedEnumNames filter exists because a real codebase has plenty of
// enums with nothing to do with authorization (environment config, file
// storage driver, soft-delete status...) — extracting every enum in the
// project as a candidate "role declaration" produced overwhelming noise
// on real code (verified against brocoders/nestjs-boilerplate: 5 enums
// total, 1 of them actually role-related). Restricting to enums a
// @Roles() call actually names is far more precise.
//
// The trade-off, stated plainly: a role enum with *zero* @Roles()
// references anywhere in the project — a role system that's entirely
// disconnected from any guard — won't be seen at all, since nothing
// connects it to authorization in the first place. That's a narrower,
// rarer miss than the noise the unfiltered version produced, and the
// honest v0.1 choice given the two failure modes traded off against each
// other.
//
// Only enum member names matter here, not their underlying values (numeric
// or string) — resolution (this package's companion in extract.go) matches
// RoleReferences against this qualified name, not against the value. A
// bare string-literal argument to @Roles() that happens to equal a
// member's value (rather than using the enum symbol) will not resolve —
// a documented limitation, not an oversight (see extract.go).
//
// This walks the whole tree recursively rather than only top-level
// statements, since an enum can be exported, nested, or declared inline;
// unlike controller detection, there's no decorator-association step that
// requires knowing an enum's siblings.
func extractRoleDeclarations(root *sitter.Node, src []byte, file string, next func() model.ID, usedEnumNames map[string]bool) []model.RoleDeclaration {
	var out []model.RoleDeclaration
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "enum_declaration" {
			if nameNode := n.ChildByFieldName("name"); nameNode != nil && usedEnumNames[nameNode.Content(src)] {
				out = append(out, roleDeclarationsFromEnum(n, src, file, next)...)
			}
		}
		for _, c := range namedChildren(n) {
			walk(c)
		}
	}
	walk(root)
	return out
}

// collectRoleEnumNames scans the whole tree for @Roles(...) calls and
// returns the set of enum names appearing as the object of a qualified
// member-expression argument, e.g. @Roles(RoleEnum.admin) contributes
// "RoleEnum". This is the filter extractRoleDeclarations applies.
func collectRoleEnumNames(root *sitter.Node, src []byte) map[string]bool {
	names := make(map[string]bool)
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "decorator" {
			if call, ok := parseDecorator(n, src); ok && call.Name == "Roles" {
				for _, arg := range argumentNodes(call.Args) {
					if qualified, ok := memberExpressionName(arg, src); ok {
						if dot := strings.IndexByte(qualified, '.'); dot != -1 {
							names[qualified[:dot]] = true
						}
					}
				}
			}
		}
		for _, c := range namedChildren(n) {
			walk(c)
		}
	}
	walk(root)
	return names
}

func roleDeclarationsFromEnum(enumDecl *sitter.Node, src []byte, file string, next func() model.ID) []model.RoleDeclaration {
	nameNode := enumDecl.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	enumName := nameNode.Content(src)

	body := enumDecl.ChildByFieldName("body")
	if body == nil {
		return nil
	}

	var out []model.RoleDeclaration
	for _, assignment := range namedChildren(body) {
		if assignment.Type() != "enum_assignment" {
			continue
		}
		memberNameNode := assignment.ChildByFieldName("name")
		if memberNameNode == nil {
			continue
		}

		var memberName string
		switch memberNameNode.Type() {
		case "property_identifier":
			memberName = memberNameNode.Content(src)
		case "string":
			v, ok := stringLiteralValue(memberNameNode, src)
			if !ok {
				continue
			}
			memberName = v
		default:
			continue
		}

		out = append(out, model.RoleDeclaration{
			ID:   next(),
			Name: enumName + "." + memberName,
			Kind: model.RoleDeclarationEnum,
			File: file,
			Line: int(assignment.StartPoint().Row) + 1,
		})
	}
	return out
}
