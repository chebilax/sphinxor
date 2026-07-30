package nestjs

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chebilax/sphinxor/internal/model"
)

// httpVerbDecorators maps a NestJS route decorator name to the HTTP
// method it declares. @Options, @Head, and @All are not recognized in
// v0.1 — a scope cut, not a correctness claim about those routes.
var httpVerbDecorators = map[string]model.HTTPMethod{
	"Get":    model.MethodGet,
	"Post":   model.MethodPost,
	"Put":    model.MethodPut,
	"Patch":  model.MethodPatch,
	"Delete": model.MethodDelete,
}

// endpointAnchor is an Endpoint's textual starting line — the earliest
// line among its decorators and its method_definition — used to match a
// sphinxor-allow marker to "the endpoint below it" (docs/decisions/0003).
type endpointAnchor struct {
	EndpointID model.ID
	File       string
	Line       int
}

// pendingGuard is a guard or role decorator found on a controller class or
// a route handler, not yet tied to a specific Endpoint — class-level ones
// get applied to every endpoint in the controller (model.GuardScope).
type pendingGuard struct {
	kind  string // "guard" | "roles"
	name  string // kind == "guard"
	roles []roleArg
	file  string
	line  int
}

type roleArg struct {
	raw    string
	declID *model.ID
}

// extractControllers finds every @Controller() class in root, and every
// route handler method within it, populating b.model and returning the
// endpoint anchors needed for allowlist matching.
func extractControllers(root *sitter.Node, src []byte, file string, b *builder, roleByName map[string]model.ID) []endpointAnchor {
	var anchors []endpointAnchor

	for _, group := range groupDecorators(flattenTopLevel(root)) {
		if group.decl == nil || group.decl.Type() != "class_declaration" {
			continue
		}

		controllerCall, isController := findDecoratorCall(group.decorators, src, "Controller")
		if !isController {
			continue
		}

		nameNode := group.decl.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}

		controllerID := b.nextIDFor("controller")
		basePath := controllerBasePath(controllerCall.Args, src)
		b.model.Controllers = append(b.model.Controllers, model.Controller{
			ID:       controllerID,
			Name:     nameNode.Content(src),
			BasePath: basePath,
			File:     file,
			Line:     int(group.decl.StartPoint().Row) + 1,
		})

		classGuards := pendingGuardsFromDecorators(group.decorators, src, file, roleByName)

		body := group.decl.ChildByFieldName("body")
		for _, methodGroup := range groupDecorators(namedChildren(body)) {
			if methodGroup.decl == nil || methodGroup.decl.Type() != "method_definition" {
				continue
			}

			httpCall, httpMethod, isRoute := findHTTPVerbDecorator(methodGroup.decorators, src)
			if !isRoute {
				continue
			}

			handlerNameNode := methodGroup.decl.ChildByFieldName("name")
			if handlerNameNode == nil {
				continue
			}

			subPath := ""
			if args := argumentNodes(httpCall.Args); len(args) > 0 {
				if v, ok := stringLiteralValue(args[0], src); ok {
					subPath = v
				}
			}

			endpointID := b.nextIDFor("endpoint")
			anchorLine := anchorLineOf(methodGroup)

			b.model.Endpoints = append(b.model.Endpoints, model.Endpoint{
				ID:           endpointID,
				HTTPMethod:   httpMethod,
				Path:         joinPath(basePath, subPath),
				HandlerName:  handlerNameNode.Content(src),
				ControllerID: controllerID,
				File:         file,
				Line:         anchorLine,
			})
			anchors = append(anchors, endpointAnchor{EndpointID: endpointID, File: file, Line: anchorLine})

			methodGuards := pendingGuardsFromDecorators(methodGroup.decorators, src, file, roleByName)
			b.applyGuards(endpointID, classGuards, model.ScopeClass)
			b.applyGuards(endpointID, methodGuards, model.ScopeMethod)
		}
	}

	return anchors
}

func anchorLineOf(g declGroup) int {
	line := int(g.decl.StartPoint().Row) + 1
	for _, d := range g.decorators {
		if l := int(d.StartPoint().Row) + 1; l < line {
			line = l
		}
	}
	return line
}

func findDecoratorCall(decorators []*sitter.Node, src []byte, name string) (decoratorCall, bool) {
	for _, d := range decorators {
		call, ok := parseDecorator(d, src)
		if ok && call.Name == name {
			return call, true
		}
	}
	return decoratorCall{}, false
}

func findHTTPVerbDecorator(decorators []*sitter.Node, src []byte) (decoratorCall, model.HTTPMethod, bool) {
	for _, d := range decorators {
		call, ok := parseDecorator(d, src)
		if !ok {
			continue
		}
		if method, known := httpVerbDecorators[call.Name]; known {
			return call, method, true
		}
	}
	return decoratorCall{}, "", false
}

// pendingGuardsFromDecorators reads @UseGuards(...) and @Roles(...)
// decorators out of decorators, resolving role arguments against
// roleByName immediately since it's already complete by the time
// extraction reaches this pass.
func pendingGuardsFromDecorators(decorators []*sitter.Node, src []byte, file string, roleByName map[string]model.ID) []pendingGuard {
	var out []pendingGuard
	for _, d := range decorators {
		call, ok := parseDecorator(d, src)
		if !ok {
			continue
		}
		line := int(d.StartPoint().Row) + 1

		switch call.Name {
		case "UseGuards":
			for _, arg := range argumentNodes(call.Args) {
				out = append(out, pendingGuard{kind: "guard", name: guardArgName(arg, src), file: file, line: line})
			}
		case "Roles":
			var roles []roleArg
			for _, arg := range argumentNodes(call.Args) {
				raw, declID := resolveRoleArg(arg, src, roleByName)
				roles = append(roles, roleArg{raw: raw, declID: declID})
			}
			out = append(out, pendingGuard{kind: "roles", roles: roles, file: file, line: line})
		}
	}
	return out
}

// resolveRoleArg resolves one @Roles(...) argument to a RoleDeclaration,
// when possible. Only qualified enum-member references (`RoleEnum.admin`)
// resolve; a bare string literal that happens to equal a member's
// underlying value does not, even if an enum with that value exists —
// documented as a known limitation, not silently guessed at.
func resolveRoleArg(n *sitter.Node, src []byte, roleByName map[string]model.ID) (raw string, declID *model.ID) {
	if name, ok := memberExpressionName(n, src); ok {
		if id, found := roleByName[name]; found {
			idCopy := id
			return name, &idCopy
		}
		return name, nil
	}
	if v, ok := stringLiteralValue(n, src); ok {
		return v, nil
	}
	return n.Content(src), nil
}

func (b *builder) applyGuards(endpointID model.ID, guards []pendingGuard, scope model.GuardScope) {
	for _, g := range guards {
		switch g.kind {
		case "guard":
			b.model.GuardApplications = append(b.model.GuardApplications, model.GuardApplication{
				ID:         b.nextIDFor("guardapp"),
				EndpointID: endpointID,
				GuardName:  g.name,
				AppliedAt:  scope,
				File:       g.file,
				Line:       g.line,
			})
		case "roles":
			guardAppID := b.nextIDFor("guardapp")
			b.model.GuardApplications = append(b.model.GuardApplications, model.GuardApplication{
				ID:         guardAppID,
				EndpointID: endpointID,
				GuardName:  "Roles",
				AppliedAt:  scope,
				File:       g.file,
				Line:       g.line,
			})
			for _, r := range g.roles {
				b.model.RoleReferences = append(b.model.RoleReferences, model.RoleReference{
					ID:                 b.nextIDFor("roleref"),
					GuardApplicationID: guardAppID,
					RoleDeclarationID:  r.declID,
					RawLiteral:         r.raw,
					File:               g.file,
					Line:               g.line,
				})
			}
		}
	}
}
