package spring

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// acceptedAnnotationPackages maps each recognized method-security
// annotation's simple name to the package(s) that make it the real thing,
// per docs/decisions/0022-annotation-identity-and-unrecognized-authorization.md §1.
//
// Every string here was verified rather than recalled, because a typo
// would silently reject the genuine annotation — this file's own defect
// inverted, and one the survey corpus cannot catch:
//
//   - PreAuthorize: confirmed by all 122 imports across the 20-repository
//     corpus, which show no second spelling.
//   - Secured: verified against Spring Security's current API
//     documentation. No project in the corpus imports Spring's @Secured at
//     all, so nothing here is corpus-backed — see
//     TestAnnotationIdentity_SpringSecuredIsRecognized.
//   - RolesAllowed: jakarta form verified against the Jakarta Annotations
//     specification; javax is its pre-rename twin. Both are the real
//     JSR-250 annotation and Spring reads both, so accepting both is a
//     binding fact, not a preference. Also absent from the corpus — see
//     TestAnnotationIdentity_RolesAllowedBothNamespaces.
var acceptedAnnotationPackages = map[string][]string{
	"PreAuthorize": {"org.springframework.security.access.prepost"},
	"Secured":      {"org.springframework.security.access.annotation"},
	"RolesAllowed": {"javax.annotation.security", "jakarta.annotation.security"},
}

// importTable is one file's import declarations, indexed the two ways a
// Java simple name can be bound: a single-type import naming it directly,
// or an on-demand (wildcard) import of a package containing it.
//
// Per-file is the whole scope, and that is what makes ADR 0022 tractable:
// measured across 20 real repositories and every vendored fixture, each
// use of a recognized annotation name has a binding import in its own
// file — no same-package declarations, no fully-qualified inline uses. So
// no cross-file symbol table and no classpath is needed to answer "is
// this Spring's annotation?".
type importTable struct {
	// exact maps a simple name to the fully-qualified name a single-type
	// import bound it to, e.g. "Secured" ->
	// "com.alibaba.nacos.auth.annotation.Secured".
	exact map[string]string
	// wildcards are packages imported on demand, e.g.
	// "org.springframework.security.access.prepost" from
	// `import org.springframework.security.access.prepost.*;`.
	wildcards []string
}

// parseImports reads root's import declarations into an importTable.
//
// tree-sitter-java's shape, confirmed against a real parse rather than
// assumed from the grammar file: an `import_declaration` has a
// `scoped_identifier` named child holding the dotted path, plus — only for
// an on-demand import — a second named child of type `asterisk`, in which
// case the scoped_identifier is the *package* rather than a type. A
// `static` import carries the keyword as an unnamed child; those bind
// members, never annotation types, so they are skipped.
func parseImports(root *sitter.Node, src []byte) importTable {
	t := importTable{exact: make(map[string]string)}
	for _, n := range namedChildren(root) {
		if n.Type() != "import_declaration" {
			continue
		}
		if isStaticImport(n) {
			continue
		}
		path := findChildByType(n, "scoped_identifier")
		if path == nil {
			continue
		}
		text := path.Content(src)
		if findChildByType(n, "asterisk") != nil {
			t.wildcards = append(t.wildcards, text)
			continue
		}
		if i := strings.LastIndexByte(text, '.'); i >= 0 && i+1 < len(text) {
			t.exact[text[i+1:]] = text
		}
	}
	return t
}

func isStaticImport(n *sitter.Node) bool {
	for i := 0; i < int(n.ChildCount()); i++ {
		if c := n.Child(i); !c.IsNamed() && c.Type() == "static" {
			return true
		}
	}
	return false
}

// resolveAnnotation reports what this file binds simpleName to, and
// whether that binding is one of the packages that make it a real Spring
// method-security annotation (ADR 0022 §1).
//
// boundTo is the fully-qualified name the binding resolves to, for the
// warning to name; it is empty when nothing in the file binds the name at
// all. An unbound name is deliberately *not* accepted: Spring's
// annotations live under org.springframework.*, so a file using @Secured
// with no import cannot be picking Spring's up by same-package
// resolution — it is either a wildcard (handled here) or a project-local
// annotation, which is the foreign case this ADR exists to catch.
func (t importTable) resolveAnnotation(simpleName string) (boundTo string, accepted bool) {
	acceptedPkgs, recognized := acceptedAnnotationPackages[simpleName]
	if !recognized {
		return "", false
	}

	if fqn, ok := t.exact[simpleName]; ok {
		for _, pkg := range acceptedPkgs {
			if fqn == pkg+"."+simpleName {
				return fqn, true
			}
		}
		return fqn, false
	}

	// No single-type import. An on-demand import of an accepted package
	// binds the name just as well; one of any other package binds it to
	// something this extractor cannot identify.
	for _, pkg := range acceptedPkgs {
		for _, w := range t.wildcards {
			if w == pkg {
				return pkg + "." + simpleName, true
			}
		}
	}
	for _, w := range t.wildcards {
		return w + "." + simpleName, false
	}

	return "", false
}
