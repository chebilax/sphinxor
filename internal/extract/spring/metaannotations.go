package spring

import (
	sitter "github.com/smacker/go-tree-sitter"
)

// controllerMeta describes a project-declared annotation that composes
// @RestController or @Controller — docs/decisions/0024-controller-meta-annotations.md.
//
// apache/shenyu's @RestApi is the case this exists for: @RestController +
// @RequestMapping with an @AliasFor'd path, on 35 of shenyu-admin's 41
// controller classes, hiding 179 route declarations from a run that
// reported 192 endpoints and said nothing about the gap.
type controllerMeta struct {
	// declaredPath is a literal path written on the declaration's own
	// @RequestMapping, e.g. `@RequestMapping("/base")` above the
	// `@interface`. Empty unless hasDeclaredPath.
	declaredPath    string
	hasDeclaredPath bool
	// pathAttribute is the name of the meta-annotation's own attribute
	// whose @AliasFor targets @RequestMapping's path (or value), so the
	// base path comes from the *use site*. "" when no attribute does.
	//
	// shenyu's @RestApi sets this to "value":
	//
	//	@AliasFor(attribute = "path", annotation = RequestMapping.class)
	//	String[] value() default {};
	pathAttribute string
	// pkg is the package the annotation type was declared in, so a
	// fully-qualified use can be checked against it — ADR 0025 §4's
	// whole-path rule applied to project-declared annotations.
	//
	// Without it, `@other.pkg.RestApi` would match shenyu's `@RestApi`
	// on its last segment alone, which is the hazard §4 exists to
	// prevent. Found by the post-implementation audit for call sites
	// still matching on a bare name; no corpus project writes a project
	// meta-annotation qualified, so nothing exercises it.
	pkg string
}

// scanControllerMetaAnnotations adds every controller-composing
// annotation declared in this file to out, keyed by its simple name.
//
// Project-wide rather than per-file by necessity: shenyu declares
// @RestApi in aspect/annotation/RestApi.java and uses it 35 files away.
// The caller runs this over every parsed file before any controller is
// extracted.
//
// Only ONE level is resolved (ADR 0024 §1): this looks for a literal
// @RestController/@Controller on the declaration, never for another
// meta-annotation that composes one. The bound is measured rather than
// conventional — the corpus's only multi-level chains are shenyu's
// @Shenyu*Mapping, which bottom out at @RequestMapping(method = …), a
// shape ADR 0011 §1 does not read at any depth.
func scanControllerMetaAnnotations(root *sitter.Node, src []byte, out map[string]controllerMeta) {
	for _, decl := range namedChildren(root) {
		if decl.Type() != "annotation_type_declaration" {
			continue
		}
		anns := annotationsOf(decl, src)
		if !hasAny(anns, controllerAnnotations) {
			continue
		}
		nameNode := decl.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}

		meta := controllerMeta{}
		if reqMapping, ok := findAnnotation(anns, "RequestMapping"); ok {
			// A path written on the declaration itself applies to every
			// use, and takes precedence over the use site (§2).
			if p, ok := pathAttributeValue(reqMapping.Args, src); ok {
				meta.declaredPath = p
				meta.hasDeclaredPath = true
			}
		}
		meta.pathAttribute = pathAliasAttribute(decl, src)
		meta.pkg = packageOf(root, src)

		out[nameNode.Content(src)] = meta
	}
}

// pathAliasAttribute returns the name of the annotation type's own
// attribute that @AliasFor routes to @RequestMapping's path — ADR 0024 §3.
//
// Two forms are recognized, both present in the corpus:
//
//   - explicit, naming both parts:
//     @AliasFor(attribute = "path", annotation = RequestMapping.class)
//   - implicit same-name, on an attribute already called path or value:
//     @AliasFor(annotation = RequestMapping.class)
//
// An @AliasFor with no `annotation =` element aliases two attributes of
// the *same* annotation — shenyu's @ShenyuGetMapping pairs value and path
// that way — and says nothing about @RequestMapping, so it is not
// followed.
func pathAliasAttribute(decl *sitter.Node, src []byte) string {
	body := decl.ChildByFieldName("body")
	if body == nil {
		return ""
	}
	for _, el := range namedChildren(body) {
		if el.Type() != "annotation_type_element_declaration" {
			continue
		}
		nameNode := el.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		attr := nameNode.Content(src)

		alias, ok := findAnnotation(annotationsOf(el, src), "AliasFor")
		if !ok || alias.Args == nil {
			continue
		}
		target, hasTarget := aliasForTargetAnnotation(alias.Args, src)
		if !hasTarget || target != "RequestMapping" {
			continue
		}
		switch aliased, named := aliasForAttribute(alias.Args, src); {
		case named && (aliased == "path" || aliased == "value"):
			return attr
		case !named && (attr == "path" || attr == "value"):
			// Implicit same-name aliasing.
			return attr
		}
	}
	return ""
}

// aliasForTargetAnnotation reads @AliasFor's `annotation = X.class`
// element, returning "X".
func aliasForTargetAnnotation(args *sitter.Node, src []byte) (string, bool) {
	for _, arg := range namedChildren(args) {
		if arg.Type() != "element_value_pair" {
			continue
		}
		key := arg.ChildByFieldName("key")
		if key == nil || key.Content(src) != "annotation" {
			continue
		}
		v := arg.ChildByFieldName("value")
		if v == nil {
			return "", false
		}
		// `RequestMapping.class` parses as a class_literal / field_access
		// depending on the form; the simple name is what matters.
		return simpleNameOfClassLiteral(v.Content(src)), true
	}
	return "", false
}

// aliasForAttribute reads @AliasFor's `attribute = "x"` element. named is
// false when the element is absent, which is the implicit same-name form.
func aliasForAttribute(args *sitter.Node, src []byte) (string, bool) {
	for _, arg := range namedChildren(args) {
		if arg.Type() != "element_value_pair" {
			continue
		}
		key := arg.ChildByFieldName("key")
		if key == nil {
			continue
		}
		if k := key.Content(src); k != "attribute" && k != "value" {
			continue
		}
		if v, ok := stringLiteralValue(arg.ChildByFieldName("value"), src); ok {
			return v, true
		}
	}
	return "", false
}

// simpleNameOfClassLiteral turns "RequestMapping.class" or
// "org.springframework.web.bind.annotation.RequestMapping.class" into
// "RequestMapping".
func simpleNameOfClassLiteral(text string) string {
	if len(text) > len(".class") && text[len(text)-len(".class"):] == ".class" {
		text = text[:len(text)-len(".class")]
	}
	for i := len(text) - 1; i >= 0; i-- {
		if text[i] == '.' {
			return text[i+1:]
		}
	}
	return text
}

// metaBasePath resolves a controller's base path from a meta-annotation
// use site — ADR 0024 §2/§4.
//
// resolved is false only when an argument for the aliased attribute is
// present and could not be read (a constant reference, a concatenation).
// That case takes ADR 0020 Amendment 1 §5's existing treatment: the
// endpoint keeps a synthesized identity, its path is marked …, the run
// warns, and the Cerbos export omits it. Nothing in the 20-repository
// corpus exercises it — all 35 of shenyu's uses are string literals —
// which is precisely why it is specified rather than left to whatever the
// code happens to do on the first project that differs.
//
// No argument at all is *not* unresolved: an annotation used bare, or one
// whose aliased attribute is simply not passed, declares no base path, the
// same as a @RestController with no @RequestMapping.
func metaBasePath(meta controllerMeta, use annotationCall, src []byte) (path string, resolved bool) {
	if meta.hasDeclaredPath {
		return meta.declaredPath, true
	}
	if meta.pathAttribute == "" || use.Args == nil {
		return "", true
	}
	return namedArgumentValue(use.Args, meta.pathAttribute, src)
}

// namedArgumentValue reads the argument bound to attr at an annotation's
// use site. A single positional argument binds to `value`, which is how
// `@RestApi("/plugin-handle")` reaches the attribute named value.
//
// resolved is false when an argument for attr exists but is not a string
// literal.
func namedArgumentValue(args *sitter.Node, attr string, src []byte) (value string, resolved bool) {
	for _, arg := range namedChildren(args) {
		switch arg.Type() {
		case "element_value_pair":
			key := arg.ChildByFieldName("key")
			if key == nil || key.Content(src) != attr {
				continue
			}
			v, ok := stringLiteralValue(arg.ChildByFieldName("value"), src)
			return v, ok
		default:
			// A positional argument. Java binds it to `value`, so it only
			// answers for that attribute.
			if attr != "value" {
				continue
			}
			if arg.Type() == "string_literal" {
				v, ok := stringLiteralValue(arg, src)
				return v, ok
			}
			// Present, and not a literal — an array of them, a constant
			// reference, a concatenation.
			return "", false
		}
	}
	return "", true
}

// packageOf returns a compilation unit's declared package, or "" for the
// default package.
func packageOf(root *sitter.Node, src []byte) string {
	for _, n := range namedChildren(root) {
		if n.Type() != "package_declaration" {
			continue
		}
		for _, c := range namedChildren(n) {
			switch c.Type() {
			case "scoped_identifier", "identifier":
				return c.Content(src)
			}
		}
	}
	return ""
}

// matchesUse reports whether a use site written with this qualifier
// refers to this meta-annotation. An unqualified use always does; a
// qualified one must name the package the annotation was declared in
// (ADR 0025 §4).
func (m controllerMeta) matchesUse(qualifier string) bool {
	return qualifier == "" || qualifier == m.pkg
}
