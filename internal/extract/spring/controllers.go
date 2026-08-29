package spring

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chebilax/sphinxor/internal/model"
)

// controllerAnnotations are the class-level annotations that mark a Spring
// MVC/REST controller, per docs/decisions/0011-spring-second-framework.md §1.
// @RestController implies @Controller + @ResponseBody; both are recognized
// as "this class is a controller" identically — the distinction doesn't
// matter for endpoint discovery.
var controllerAnnotations = map[string]bool{
	"RestController": true,
	"Controller":     true,
}

// httpMappingAnnotations maps a Spring method-mapping annotation name to
// the single HTTP verb it declares. `@RequestMapping(method = {GET, POST})`
// (ADR 0011 §1's "one Spring method can map multiple HTTP verbs" case) is
// deliberately not handled here — none of the vendored fixtures use it, and
// per this project's standing discipline (docs/testing.md), extraction
// logic isn't built ahead of real evidence that it's needed. A scope cut,
// not a correctness claim about methods using that shape: such a method is
// simply not recognized as an endpoint yet, the same "can't parse it,
// don't guess" default used everywhere else in this project.
var httpMappingAnnotations = map[string]model.HTTPMethod{
	"GetMapping":    model.MethodGet,
	"PostMapping":   model.MethodPost,
	"PutMapping":    model.MethodPut,
	"DeleteMapping": model.MethodDelete,
	"PatchMapping":  model.MethodPatch,
}

// extractControllers finds every @RestController/@Controller class at the
// top level of root, and every recognized HTTP-mapping method within it,
// populating b.model.Controllers and b.model.Endpoints. No guard, role, or
// authentication extraction happens here — that's a separate, later pass
// (docs/decisions/0011-spring-second-framework.md, docs/decisions/0012-securityfilterchain-effective-policy.md),
// verified independently against the same fixtures once it exists.
func extractControllers(root *sitter.Node, src []byte, file string, b *builder) {
	for _, decl := range namedChildren(root) {
		if decl.Type() != "class_declaration" {
			continue
		}

		classAnns := annotationsOf(decl, src)
		if !hasAny(classAnns, controllerAnnotations) {
			continue
		}

		nameNode := decl.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}

		basePath := ""
		if reqMapping, ok := findAnnotation(classAnns, "RequestMapping"); ok {
			if p, ok := pathAttributeValue(reqMapping.Args, src); ok {
				basePath = p
			}
		}

		controllerID := b.nextIDFor("controller")
		b.model.Controllers = append(b.model.Controllers, model.Controller{
			ID:       controllerID,
			Name:     nameNode.Content(src),
			BasePath: basePath,
			File:     file,
			Line:     int(decl.StartPoint().Row) + 1,
		})

		body := decl.ChildByFieldName("body")
		for _, member := range namedChildren(body) {
			if member.Type() != "method_declaration" {
				continue
			}

			methodAnns := annotationsOf(member, src)
			mapping, httpMethod, ok := findHTTPMapping(methodAnns)
			if !ok {
				continue
			}

			handlerNameNode := member.ChildByFieldName("name")
			if handlerNameNode == nil {
				continue
			}

			subPath, _ := pathAttributeValue(mapping.Args, src)
			path := joinPath(basePath, subPath)
			endpointID := model.NewEndpointID(httpMethod, path)

			// Two real handlers sharing HTTPMethod+Path (differing only in
			// `produces`) merge into the one already created by whichever
			// was encountered first — docs/decisions/0014-endpoint-identity-and-content-negotiation.md.
			// Not an arbitrary "first wins": the URL layer is architecturally
			// identical for both (Spring's authorizeHttpRequests matches
			// only method+path, before content negotiation resolves which
			// handler runs), so which one's Endpoint row survives doesn't
			// change what the URL layer contributes; only File/Line/HandlerName
			// display differs.
			if b.seenEndpoints[endpointID] {
				continue
			}
			b.seenEndpoints[endpointID] = true

			b.model.Endpoints = append(b.model.Endpoints, model.Endpoint{
				ID:           endpointID,
				HTTPMethod:   httpMethod,
				Path:         path,
				HandlerName:  handlerNameNode.Content(src),
				ControllerID: controllerID,
				File:         file,
				Line:         int(member.StartPoint().Row) + 1,
			})
		}
	}
}

func hasAny(anns []annotationCall, names map[string]bool) bool {
	for _, a := range anns {
		if names[a.Name] {
			return true
		}
	}
	return false
}

func findHTTPMapping(anns []annotationCall) (annotationCall, model.HTTPMethod, bool) {
	for _, a := range anns {
		if method, known := httpMappingAnnotations[a.Name]; known {
			return a, method, true
		}
	}
	return annotationCall{}, "", false
}
