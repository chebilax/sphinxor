package spring

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chebilax/sphinxor/internal/allowlist"
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
// top level of root, every recognized HTTP-mapping method within it, and
// every method-security guard (class- and method-level) applying to each
// endpoint — populating b.model.Controllers, b.model.Endpoints,
// b.model.GuardApplications, and b.model.RoleReferences. Authentication
// requirements are a separate final pass (authentication.go), since an
// endpoint's guards can come from both class- and method-level annotations
// and the "zero resolved roles anywhere on this endpoint" check needs all
// of them known first.
//
// It returns one allowlist.Anchor per handler it recognized — the input
// docs/decisions/0003-allowlist-format.md's marker matching needs.
//
// One anchor per *handler*, not per Endpoint: two handlers that merge into
// a single Endpoint under ADR 0014 (same method+path, differing only in
// `produces`) are two real places in the source a developer could put a
// marker above, and either should exempt the endpoint they share.
func extractControllers(root *sitter.Node, src []byte, file string, b *builder, roleByName map[string]model.ID) []allowlist.Anchor {
	var anchors []allowlist.Anchor
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

		// A @RequestMapping whose path is a constant reference reads as
		// no path at all. Left that way it collapsed endpoint identity,
		// and in Java the colliding endpoint was dropped outright rather
		// than merged (ADR 0020 Amendment 1 §5). A controller with no
		// @RequestMapping genuinely has no prefix and stays resolved.
		basePath := ""
		basePathResolved := true
		classVersion := versionDecl{}
		if reqMapping, ok := findAnnotation(classAnns, "RequestMapping"); ok {
			if p, ok := pathAttributeValue(reqMapping.Args, src); ok {
				basePath = p
			} else if reqMapping.Args != nil {
				basePathResolved = false
			}
			// A class-level version applies to every handler that does not
			// declare its own (ADR 0020 Amendment 2 §7).
			classVersion = versionAttributeValue(reqMapping.Args, src)
		}

		controllerID := b.nextIDFor("controller")
		b.model.Controllers = append(b.model.Controllers, model.Controller{
			ID:       controllerID,
			Name:     nameNode.Content(src),
			BasePath: basePath,
			File:     file,
			Line:     int(decl.StartPoint().Row) + 1,
		})

		classGuards := pendingGuardsFromAnnotations(classAnns, src, file, roleByName)

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

			subPath, subPathRead := pathAttributeValue(mapping.Args, src)
			subPathResolved := subPathRead || mapping.Args == nil
			handlerName := handlerNameNode.Content(src)
			path := joinPath(basePath, subPath)
			pathUnresolved := !basePathResolved || !subPathResolved

			version := classVersion
			if v := versionAttributeValue(mapping.Args, src); v.declared {
				version = v
			}

			// Identity, in order of what is least knowable — the same
			// ordering and the same reasoning as the NestJS extractor's
			// (ADR 0020 Amendment 1 §5, Amendment 2 §7). Java's failure
			// mode differs — a colliding endpoint is dropped below rather
			// than merged — but the cause and the fix are identical.
			endpointID := model.NewEndpointID(httpMethod, path)
			switch {
			case pathUnresolved:
				endpointID = model.NewUnresolvedPathEndpointID(httpMethod, nameNode.Content(src), handlerName)
			case version.unknownVersion():
				endpointID = model.NewUnresolvedVersionEndpointID(httpMethod, nameNode.Content(src), handlerName)
			case version.declared:
				endpointID = model.NewVersionedEndpointID(httpMethod, path, version.value)
			}

			// The handler's own starting line. In Java, annotations are
			// part of method_declaration's modifiers, so the node already
			// starts at the first annotation (@PostMapping, not the
			// signature) — verified against real Pharmacy source rather
			// than assumed from the grammar, and the same line the
			// Endpoint row records below.
			anchors = append(anchors, allowlist.Anchor{
				EndpointID: endpointID,
				File:       file,
				Line:       int(member.StartPoint().Row) + 1,
			})

			// Two real handlers sharing HTTPMethod+Path (differing only in
			// `produces`) merge into one Endpoint —
			// docs/decisions/0014-endpoint-identity-and-content-negotiation.md.
			// The first encountered wins the Endpoint row itself
			// (HandlerName/File/Line), but every merged handler's own
			// guards still get attached below — ADR 0014's Consequences
			// says so explicitly: "it attaches whatever guards each real
			// handler carries to the shared Endpoint.ID exactly as
			// extracted." Skipping guard extraction for a merged-away
			// handler would silently discard a real annotation.
			// Two real handlers sharing HTTPMethod+Path (differing only in
			// `produces`) merge into one Endpoint —
			// docs/decisions/0014-endpoint-identity-and-content-negotiation.md.
			// The first encountered wins the Endpoint row itself
			// (HandlerName/File/Line), but every merged handler's own
			// guards still get attached below — ADR 0014's Consequences
			// says so explicitly: "it attaches whatever guards each real
			// handler carries to the shared Endpoint.ID exactly as
			// extracted." Skipping guard extraction for a merged-away
			// handler would silently discard a real annotation — locked in
			// as a regression test, TestExtractControllers_MergedHandlerRetainsOwnGuard
			// (guards_test.go), confirmed to actually fail against the
			// original buggy shape before being kept.
			if !b.seenEndpoints[endpointID] {
				b.seenEndpoints[endpointID] = true
				b.model.Endpoints = append(b.model.Endpoints, model.Endpoint{
					ID:                endpointID,
					HTTPMethod:        httpMethod,
					Path:              path,
					HandlerName:       handlerName,
					PathUnresolved:    pathUnresolved,
					Version:           version.value,
					VersionUnresolved: version.unknownVersion(),
					ControllerID:      controllerID,
					File:              file,
					Line:              int(member.StartPoint().Row) + 1,
				})
				// Class-level guards apply once per endpoint, not once per
				// merged handler — attaching them again for a second
				// merged handler would duplicate identical
				// GuardApplications for no reason (both variants share the
				// exact same class-level annotations by construction).
				b.applyGuards(endpointID, classGuards, model.ScopeClass)
			}

			methodGuards := pendingGuardsFromAnnotations(methodAnns, src, file, roleByName)
			b.applyGuards(endpointID, methodGuards, model.ScopeMethod)
		}
	}
	return anchors
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
