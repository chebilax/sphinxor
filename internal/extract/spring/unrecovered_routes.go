package spring

import (
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chebilax/sphinxor/internal/model"
)

// frameworkInterfacePackages are packages whose interfaces are framework
// or JDK types rather than a project's own API — ADR 0032 §1.
//
// A controller implementing one of these is implementing a lifecycle or
// functional interface, not declaring routes. Without this, condition B
// fires on InitializingBean (JeecgBoot), Function (spring-cloud-dataflow)
// and Predicate (halo), all measured false positives.
var frameworkInterfacePackages = []string{
	"java.", "javax.", "jakarta.", "org.springframework.", "reactor.", "io.swagger.",
}

// interfaceIndex answers, for an interface's simple name, whether this
// repository declares it and whether that declaration carries mapping
// annotations.
//
// Built project-wide before controllers are examined, the same staging
// ADR 0024's meta-annotation pass uses and for the same reason: dataease
// declares its interfaces in a separate `sdk/api` module from the
// controllers that implement them.
// Keyed by FULLY-QUALIFIED name, never the simple one. ADR 0022's
// "spelling is not identity" applies to interfaces too, and JeecgBoot
// proves it: it declares org.jeecg.common.airag.api.IAiragBaseApi twice,
// once in its local-api module with no mappings and once in its cloud-api
// module as a Feign client with seven. A simple-name index conflated
// them and flagged a controller on the strength of the wrong one.
type interfaceIndex struct {
	declared map[string]bool // fqn -> declared in this repository
	routing  map[string]bool // fqn -> at least one declaration bears mappings
	plain    map[string]bool // fqn -> at least one declaration bears none
}

func newInterfaceIndex() *interfaceIndex {
	return &interfaceIndex{declared: map[string]bool{}, routing: map[string]bool{}, plain: map[string]bool{}}
}

// ambiguous reports whether this repository declares the same interface
// twice, once bearing routes and once not. JeecgBoot's local/cloud module
// split does exactly that, and which one is on the classpath is a build
// profile no static read can settle.
func (ix *interfaceIndex) ambiguous(fqn string) bool {
	return ix.routing[fqn] && ix.plain[fqn]
}

// scanInterfaceDeclarations adds this file's interface declarations to
// the index.
func (ix *interfaceIndex) scan(root *sitter.Node, src []byte) {
	pkg := packageOf(root, src)
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "interface_declaration" {
			name := n.ChildByFieldName("name")
			if name != nil {
				fqn := name.Content(src)
				if pkg != "" {
					fqn = pkg + "." + fqn
				}
				ix.declared[fqn] = true
				if interfaceDeclaresMappings(n, src) {
					ix.routing[fqn] = true
				} else {
					ix.plain[fqn] = true
				}
			}
		}
		for _, c := range namedChildren(n) {
			walk(c)
		}
	}
	walk(root)
}

// interfaceDeclaresMappings reports whether any method on this interface
// carries a recognized mapping annotation — what makes it a routing
// interface rather than an ordinary one.
func interfaceDeclaresMappings(decl *sitter.Node, src []byte) bool {
	body := decl.ChildByFieldName("body")
	if body == nil {
		return false
	}
	found := false
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if found {
			return
		}
		if n.Type() == "method_declaration" {
			for _, ann := range annotationsOf(n, src) {
				if isMappingAnnotation(ann) {
					found = true
					return
				}
			}
		}
		for _, c := range namedChildren(n) {
			walk(c)
		}
	}
	walk(body)
	return found
}

// interfaceIsRouteBearing classifies one implemented interface per ADR
// 0032 §1's table.
//
// Announced when the interface is route-bearing in this repository, or
// when it is not in this repository at all and is not a framework type —
// apollo's PortalManagementApi comes from an external artifact, so
// nothing here can rule it out, and "unknown" is announced rather than
// assumed empty (ADR 0020's principle).
// interfaceKind is how ADR 0032 §1 condition B classifies one implemented
// interface.
type interfaceKind int

const (
	// ifaceNotRouting is a framework type, or an in-repo interface that
	// declares no mappings. nacos's Callback, nakadi's SubscriptionOutput,
	// shenyu's ClientRegisterConfig, JeecgBoot's InitializingBean — all
	// measured false positives that this removes.
	ifaceNotRouting interfaceKind = iota
	// ifaceRouting is declared in this repository AND carries mapping
	// annotations, so implementing it means serving those routes —
	// whether the class overrides them or inherits a default method.
	ifaceRouting
	// ifaceUnknown is not in this repository and not a framework type, so
	// nothing here can rule out that it bears routes. apollo's
	// PortalManagementApi comes from an external artifact. Announced
	// rather than assumed empty, per ADR 0020.
	ifaceUnknown
)

func (ix *interfaceIndex) classify(simpleName, samePkg string, imports importTable) interfaceKind {
	fqn, imported := imports.exact[simpleName]
	if imported {
		for _, pkg := range frameworkInterfacePackages {
			if strings.HasPrefix(fqn, pkg) {
				return ifaceNotRouting
			}
		}
	} else if samePkg != "" {
		// No import binds it, so Java resolves it in the using class's
		// own package.
		fqn = samePkg + "." + simpleName
	} else {
		fqn = simpleName
	}
	switch {
	case ix.ambiguous(fqn):
		// Two declarations of one name disagreeing about whether it
		// bears routes. Announcing on the routing one would be a guess
		// about which module is built; recorded in docs/limitations.md
		// rather than turned into a warning nobody can act on.
		return ifaceNotRouting
	case ix.routing[fqn]:
		return ifaceRouting
	case ix.declared[fqn]:
		return ifaceNotRouting
	default:
		return ifaceUnknown
	}
}

// isMappingAnnotation reports whether an annotation is one of Spring's
// route-mapping annotations, including the verb-less @RequestMapping that
// ADR 0028 reads as ANY.
func isMappingAnnotation(a annotationCall) bool {
	if !a.isSpringWeb() {
		return false
	}
	if _, known := httpMappingAnnotations[a.Name]; known {
		return true
	}
	return a.Name == "RequestMapping"
}

// unmappedOverrides counts public @Override methods on a class that carry
// no mapping annotation — ADR 0032 §1 condition B.
func unmappedOverrides(class *sitter.Node, src []byte) int {
	body := class.ChildByFieldName("body")
	if body == nil {
		return 0
	}
	n := 0
	for _, member := range namedChildren(body) {
		if member.Type() != "method_declaration" {
			continue
		}
		if !isPublic(member, src) {
			continue
		}
		anns := annotationsOf(member, src)
		if !hasAny(anns, map[string]bool{"Override": true}) {
			continue
		}
		mapped := false
		for _, a := range anns {
			if isMappingAnnotation(a) {
				mapped = true
				break
			}
		}
		if !mapped {
			n++
		}
	}
	return n
}

// isPublic reports whether a member declares the public modifier.
func isPublic(n *sitter.Node, src []byte) bool {
	for _, c := range namedChildren(n) {
		if c.Type() == "modifiers" {
			return strings.Contains(c.Content(src), "public")
		}
	}
	return false
}

// implementedInterfaces returns the simple names a class declares in its
// implements clause.
func implementedInterfaces(class *sitter.Node, src []byte) []string {
	var out []string
	for _, c := range namedChildren(class) {
		if c.Type() != "super_interfaces" {
			continue
		}
		var walk func(n *sitter.Node)
		walk = func(n *sitter.Node) {
			switch n.Type() {
			case "type_identifier":
				out = append(out, n.Content(src))
				return
			case "scoped_type_identifier":
				text := n.Content(src)
				out = append(out, text[strings.LastIndex(text, ".")+1:])
				return
			case "generic_type":
				// Take the raw type, not its arguments.
				if len(namedChildren(n)) > 0 {
					walk(namedChildren(n)[0])
				}
				return
			}
			for _, k := range namedChildren(n) {
				walk(k)
			}
		}
		walk(c)
	}
	return out
}

// recordUnrecoveredRoutes applies both of ADR 0032 §1's conditions once
// every controller and endpoint is known.
//
// Condition A needs the finished endpoint list, so this runs as a
// post-pass rather than during extraction.
func recordUnrecoveredRoutes(m *model.Model, partial map[model.ID]bool) {
	hasEndpoints := map[model.ID]bool{}
	for _, e := range m.Endpoints {
		hasEndpoints[e.ControllerID] = true
	}
	for _, c := range m.Controllers {
		switch {
		case !hasEndpoints[c.ID]:
			// Condition A: nothing at all came out of it.
			m.UnrecoveredRoutes = append(m.UnrecoveredRoutes, model.ControllerWithUnrecoveredRoutes{
				ControllerID: c.ID, Name: c.Name, File: c.File,
				NoRoutesAtAll: true,
			})
		case partial[c.ID]:
			// Condition B: some routes recovered, others not — the case
			// that looks complete in the matrix and is not.
			m.UnrecoveredRoutes = append(m.UnrecoveredRoutes, model.ControllerWithUnrecoveredRoutes{
				ControllerID: c.ID, Name: c.Name, File: c.File,
				NoRoutesAtAll: false,
			})
		}
	}
	sort.Slice(m.UnrecoveredRoutes, func(i, j int) bool {
		return m.UnrecoveredRoutes[i].Name < m.UnrecoveredRoutes[j].Name
	})
}

// classHasUnrecoveredInterfaceRoutes applies ADR 0032 §1 condition B to
// one controller class. Its two arms exist because the corpus contains
// two genuinely different shapes, and only one of them involves
// @Override at all.
//
//   - A ROUTE-BEARING in-repo interface is enough on its own. Its mapped
//     methods are routes the class serves, and this extractor reads no
//     interfaces, so they were not recovered — whether the class
//     overrides them or inherits a default method. shenyu's
//     PagedController is the case that forced this arm: it declares
//     @PostMapping("list/search") and @PostMapping("list/search/adaptor")
//     as DEFAULT methods, and the eight shenyu-admin controllers
//     implementing it override only pageService(), which is not a route.
//     Counting unmapped @Override methods flagged those eight for a
//     reason that had nothing to do with why their routes are missing.
//   - An UNKNOWN interface needs the @Override signal, because nothing
//     here can tell a routing interface from an ordinary one and
//     flagging every controller that implements any external type would
//     be noise. apollo's PortalManagementController is this arm: 47
//     @Override methods, 9 with an inline mapping, 38 without.
func classHasUnrecoveredInterfaceRoutes(class *sitter.Node, src []byte, ix *interfaceIndex, imports importTable, samePkg string) bool {
	sawUnknown := false
	for _, iface := range implementedInterfaces(class, src) {
		switch ix.classify(iface, samePkg, imports) {
		case ifaceRouting:
			return true
		case ifaceUnknown:
			sawUnknown = true
		}
	}
	return sawUnknown && unmappedOverrides(class, src) > 0
}

// scanPartialInherited flags, for each controller class in this file,
// whether it has routes declared on an interface that were not
// recovered — ADR 0032 §1 condition B.
//
// Controllers are matched back to the model by file and name rather than
// re-derived, so this sees exactly the classes extraction recognized,
// including those a meta-annotation made into controllers (ADR 0024).
func scanPartialInherited(root *sitter.Node, src []byte, relPath string, b *builder, ix *interfaceIndex, out map[model.ID]bool) {
	byName := map[string]model.ID{}
	for _, c := range b.model.Controllers {
		if c.File == relPath {
			byName[c.Name] = c.ID
		}
	}
	if len(byName) == 0 {
		return
	}
	imports := parseImports(root, src)
	pkg := packageOf(root, src)

	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "class_declaration" {
			name := n.ChildByFieldName("name")
			if name != nil {
				if id, ok := byName[name.Content(src)]; ok {
					if classHasUnrecoveredInterfaceRoutes(n, src, ix, imports, pkg) {
						out[id] = true
					}
				}
			}
		}
		for _, c := range namedChildren(n) {
			walk(c)
		}
	}
	walk(root)
}
