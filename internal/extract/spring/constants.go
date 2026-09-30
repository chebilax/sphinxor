package spring

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// constIndex evaluates Java compile-time String constants across the
// analyzed tree, with Java's own name resolution — ADR 0039 §2, extending
// ADR 0038 §1. It serves a role hierarchy string and a route path.
//
// Its one rule above all others: a name it cannot resolve to exactly one
// declaration in the analyzed source is UNKNOWN, never a pick. Java
// itself would reject a genuinely ambiguous name, and a name that could
// be supplied by a dependency is not something source can settle
// (ADR 0039 §2). So:
//
//   - an import naming a class outside the tree resolves to nothing, and
//     does not fall through to a same-named class elsewhere;
//   - an on-demand import of a package with no class in the tree could
//     supply any name, so a lookup that would reach it is unknown;
//   - two candidates at one step is unknown;
//   - a class with a supertype outside the tree could inherit the field,
//     which shadows a static import, so a bare name not found in the known
//     part of its hierarchy is unknown rather than taken from an import.
type constIndex struct {
	classes  map[string]*fileScope // fully qualified class -> its file
	fields   map[string]map[string]constRef
	enums    map[string]map[string]bool
	supers   map[string][]string // fully qualified class -> supertypes as written
	packages map[string]bool     // packages with at least one class in the tree
	files    map[string]*fileScope
}

// fileScope is what a name means inside one file: its package and imports.
type fileScope struct {
	pkg          string
	single       map[string]string // simple name -> fully qualified class
	wild         []string          // on-demand imported packages
	staticSingle map[string]string // member -> fully qualified class
	staticWild   []string          // fully qualified classes, static on-demand
	src          []byte
}

type constRef struct {
	value *sitter.Node
	scope *fileScope
	class string // fully qualified
}

func buildConstIndex(files []parsedFile) *constIndex {
	idx := newConstIndex()
	for _, f := range files {
		idx.addFile(f.relPath, f.tree.RootNode(), f.src)
	}
	return idx
}

func newConstIndex() *constIndex {
	return &constIndex{
		classes:  map[string]*fileScope{},
		fields:   map[string]map[string]constRef{},
		enums:    map[string]map[string]bool{},
		supers:   map[string][]string{},
		packages: map[string]bool{},
		files:    map[string]*fileScope{},
	}
}

// addFile indexes one parsed file: its import scope and every String
// constant and enum it declares.
func (idx *constIndex) addFile(relPath string, root *sitter.Node, src []byte) {
	scope := newFileScope(root, src)
	idx.files[relPath] = scope
	var walk func(n *sitter.Node, outer string)
	walk = func(n *sitter.Node, outer string) {
		switch n.Type() {
		case "class_declaration", "interface_declaration", "enum_declaration":
			if name := n.ChildByFieldName("name"); name != nil {
				fq := qualify(scope.pkg, outer, name.Content(src))
				idx.classes[fq] = scope
				idx.packages[scope.pkg] = true
				idx.indexBody(n, fq, scope)
				idx.supers[fq] = supertypesOf(n, src)
				for _, c := range namedChildren(n) {
					walk(c, fq)
				}
				return
			}
		}
		for _, c := range namedChildren(n) {
			walk(c, outer)
		}
	}
	walk(root, "")
}

func qualify(pkg, outer, simple string) string {
	switch {
	case outer != "":
		return outer + "." + simple
	case pkg != "":
		return pkg + "." + simple
	}
	return simple
}

// fqClassOf returns the fully qualified name of the class declaration n.
func fqClassOf(n *sitter.Node, src []byte, pkg string) string {
	var names []string
	for p := n; p != nil; p = p.Parent() {
		switch p.Type() {
		case "class_declaration", "interface_declaration", "enum_declaration":
			if name := p.ChildByFieldName("name"); name != nil {
				names = append([]string{name.Content(src)}, names...)
			}
		}
	}
	fq := strings.Join(names, ".")
	if pkg != "" {
		fq = pkg + "." + fq
	}
	return fq
}

func newFileScope(root *sitter.Node, src []byte) *fileScope {
	s := &fileScope{single: map[string]string{}, staticSingle: map[string]string{}, src: src, pkg: packageOf(root, src)}
	for _, n := range namedChildren(root) {
		if n.Type() != "import_declaration" {
			continue
		}
		text := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(n.Content(src), "import")), ";")
		text = strings.Join(strings.Fields(text), " ")
		static := strings.HasPrefix(text, "static ")
		q := strings.ReplaceAll(strings.TrimPrefix(text, "static "), " ", "")
		i := strings.LastIndex(q, ".")
		if i < 0 {
			continue
		}
		head, last := q[:i], q[i+1:]
		switch {
		case static && last == "*":
			s.staticWild = append(s.staticWild, head)
		case static:
			s.staticSingle[last] = head
		case last == "*":
			s.wild = append(s.wild, head)
		default:
			s.single[last] = q
		}
	}
	return s
}

func supertypesOf(decl *sitter.Node, src []byte) []string {
	var out []string
	for _, c := range namedChildren(decl) {
		switch c.Type() {
		case "superclass", "super_interfaces", "extends_interfaces":
			for _, t := range typeNames(c, src) {
				out = append(out, t)
			}
		}
	}
	return out
}

// typeNames lists the type names under a superclass/interfaces clause,
// without type arguments.
func typeNames(n *sitter.Node, src []byte) []string {
	var out []string
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "type_identifier", "scoped_type_identifier":
			out = append(out, n.Content(src))
			return
		case "generic_type":
			if c := n.NamedChild(0); c != nil {
				walk(c)
			}
			return
		}
		for _, c := range namedChildren(n) {
			walk(c)
		}
	}
	walk(n)
	return out
}

func (idx *constIndex) indexBody(decl *sitter.Node, fq string, scope *fileScope) {
	src := scope.src
	body := decl.ChildByFieldName("body")
	if body == nil {
		return
	}
	members := namedChildren(body)
	if decl.Type() == "enum_declaration" {
		consts := map[string]bool{}
		for _, m := range members {
			switch m.Type() {
			case "enum_constant":
				if name := m.ChildByFieldName("name"); name != nil {
					consts[name.Content(src)] = true
				}
			case "enum_body_declarations":
				members = append(members, namedChildren(m)...)
			}
		}
		idx.enums[fq] = consts
	}
	for _, m := range members {
		// An interface's fields are implicitly static final.
		constant := m.Type() == "constant_declaration" ||
			(m.Type() == "field_declaration" && hasModifierKeyword(m, "static") && hasModifierKeyword(m, "final"))
		if !constant {
			continue
		}
		// String, and String[] for chain matcher patterns (Amendment 1).
		// eval reads only the first; evalList reads both.
		if t := m.ChildByFieldName("type"); t == nil || (t.Content(src) != "String" && strings.ReplaceAll(t.Content(src), " ", "") != "String[]") {
			continue
		}
		for _, d := range namedChildren(m) {
			if d.Type() != "variable_declarator" {
				continue
			}
			name, value := d.ChildByFieldName("name"), d.ChildByFieldName("value")
			if name == nil || value == nil {
				continue
			}
			if idx.fields[fq] == nil {
				idx.fields[fq] = map[string]constRef{}
			}
			idx.fields[fq][name.Content(src)] = constRef{value: value, scope: scope, class: fq}
		}
	}
}

// resolveClass resolves a type name as written in scope, inside class
// encl, in the order Java does: a member type of the enclosing chain (or
// the chain itself), a single-type import, the same package, on-demand
// imports; or a fully qualified name. ok is false for "unknown", and
// ambiguous says the unknown is two candidates in the tree rather than a
// class that could lie outside it.
func (idx *constIndex) resolveClass(name string, scope *fileScope, encl string) (fq string, ok, ambiguous bool) {
	fq, ok = idx.resolveClassIn(name, scope, encl, &ambiguous)
	return fq, ok, ambiguous
}

func (idx *constIndex) resolveClassIn(name string, scope *fileScope, encl string, ambiguous *bool) (string, bool) {
	first, rest := name, ""
	if i := strings.Index(name, "."); i >= 0 {
		first, rest = name[:i], name[i:]
	}
	finish := func(fq string) (string, bool) {
		fq += rest
		_, ok := idx.classes[fq]
		return fq, ok
	}
	for e := encl; e != "" && e != scope.pkg; {
		if _, ok := idx.classes[e+"."+first]; ok {
			return finish(e + "." + first)
		}
		if lastSegment(e) == first {
			return finish(e)
		}
		i := strings.LastIndex(e, ".")
		if i < 0 {
			break
		}
		e = e[:i]
	}
	if fq, ok := scope.single[first]; ok {
		// Imported by name: that class or nothing. A class outside the
		// tree must not fall through to a same-named one inside it.
		return finish(fq)
	}
	if _, ok := idx.classes[qualify(scope.pkg, "", first)]; ok {
		return finish(qualify(scope.pkg, "", first))
	}
	var cands []string
	for _, w := range scope.wild {
		if !idx.packages[w] {
			// A package this tree does not contain could supply the name.
			return "", false
		}
		if _, ok := idx.classes[w+"."+first]; ok {
			cands = append(cands, w+"."+first)
		}
	}
	if len(cands) == 1 {
		return finish(cands[0])
	}
	if len(cands) > 1 {
		*ambiguous = true
		return "", false
	}
	if rest != "" {
		if _, ok := idx.classes[name]; ok {
			return name, true
		}
	}
	return "", false
}

// fieldIn finds a String constant named name in cls or its supertypes.
// complete is false when some supertype lies outside the tree, so an
// absence is not proof: the field could be inherited from it.
func (idx *constIndex) fieldIn(cls, name string, depth int) (ref constRef, found, complete bool) {
	if depth > 8 {
		return constRef{}, false, false
	}
	if r, ok := idx.fields[cls][name]; ok {
		return r, true, true
	}
	scope := idx.classes[cls]
	if scope == nil {
		return constRef{}, false, false
	}
	complete = true
	for _, s := range idx.supers[cls] {
		fq, ok, _ := idx.resolveClass(s, scope, cls)
		if !ok {
			complete = false
			continue
		}
		r, f, c := idx.fieldIn(fq, name, depth+1)
		if f {
			return r, true, true
		}
		complete = complete && c
	}
	return constRef{}, false, complete
}

// Why a term could not be evaluated, for the warning (ADR 0039 §7).
const (
	unresolvedOutside   = "a constant declared outside the analyzed source"
	unresolvedAmbiguous = "a name that does not resolve to exactly one declaration"
	unresolvedNotConst  = "an expression that is not a String constant"
)

// eval evaluates n to one string, per ADR 0038 §1 and ADR 0039 §2:
// string literals, + concatenation, a local variable or a static final
// String initialized that way, and E.C.name() on an enum in the tree. It
// stops at the first term it cannot evaluate and returns that term's text
// and the reason; a partial string is never returned.
func (idx *constIndex) eval(n *sitter.Node, scope *fileScope, cls string, depth int) (value string, ok bool, term, reason string) {
	if n == nil {
		return "", false, "", unresolvedNotConst
	}
	src := scope.src
	if depth > 24 {
		return "", false, n.Content(src), unresolvedNotConst
	}
	fromRef := func(r constRef) (string, bool, string, string) {
		return idx.eval(r.value, r.scope, r.class, depth+1)
	}
	switch n.Type() {
	case "string_literal":
		if v, ok := decodeStringLiteral(n, src); ok {
			return v, true, "", ""
		}
	case "parenthesized_expression":
		return idx.eval(n.NamedChild(0), scope, cls, depth+1)
	case "character_literal":
		// Only as an operand of a String concatenation, which the
		// binary_expression case below checks: a char on its own is not a
		// String, and char + char is numeric addition in Java.
		if v, ok := decodeCharLiteral(n.Content(src)); ok {
			return v, true, "", ""
		}
	case "binary_expression":
		if op := n.ChildByFieldName("operator"); op != nil && op.Content(src) == "+" {
			if isChar(n.ChildByFieldName("left")) && isChar(n.ChildByFieldName("right")) {
				return "", false, n.Content(src), unresolvedNotConst
			}
			l, ok, t, r := idx.eval(n.ChildByFieldName("left"), scope, cls, depth+1)
			if !ok {
				return "", false, t, r
			}
			rv, ok, t, r := idx.eval(n.ChildByFieldName("right"), scope, cls, depth+1)
			if !ok {
				return "", false, t, r
			}
			return l + rv, true, "", ""
		}
	case "identifier":
		return idx.evalName(n, scope, cls, depth)
	case "field_access":
		obj, field := n.ChildByFieldName("object"), n.ChildByFieldName("field")
		if obj == nil || field == nil {
			break
		}
		fq, ok, ambiguous := idx.resolveClass(obj.Content(src), scope, cls)
		if ambiguous {
			return "", false, n.Content(src), unresolvedAmbiguous
		}
		if !ok {
			return "", false, n.Content(src), unresolvedOutside
		}
		if r, found, _ := idx.fieldIn(fq, field.Content(src), 0); found {
			return fromRef(r)
		}
		return "", false, n.Content(src), unresolvedOutside
	case "method_invocation":
		if c, ok := idx.enumName(n, scope, cls); ok {
			return c, true, "", ""
		}
	}
	return "", false, n.Content(src), unresolvedNotConst
}

func (idx *constIndex) evalName(n *sitter.Node, scope *fileScope, cls string, depth int) (string, bool, string, string) {
	r, _, ok, term, reason := idx.resolveName(n, scope, cls)
	if !ok {
		return "", false, term, reason
	}
	return idx.eval(r.value, r.scope, r.class, depth+1)
}

// resolveName finds what a bare name refers to, in the order Java does: a
// local variable, a field of the class or an enclosing class (each with
// its supertypes), a single static import, then on-demand static imports.
// local reports that it is a local variable.
func (idx *constIndex) resolveName(n *sitter.Node, scope *fileScope, cls string) (r constRef, local, ok bool, term, reason string) {
	src := scope.src
	name := n.Content(src)
	if callable := enclosingCallable(n); callable != nil {
		if decl := findLocal(callable, name, src); decl != nil {
			for _, d := range namedChildren(decl) {
				if d.Type() == "variable_declarator" {
					if id := d.ChildByFieldName("name"); id != nil && id.Content(src) == name {
						return constRef{value: d.ChildByFieldName("value"), scope: scope, class: cls}, true, true, "", ""
					}
				}
			}
		}
	}
	// The class, then its enclosing classes — each with its supertypes.
	complete := true
	for e := cls; e != ""; {
		r, found, c := idx.fieldIn(e, name, 0)
		if found {
			return r, false, true, "", ""
		}
		complete = complete && c
		i := strings.LastIndex(e, ".")
		if i < 0 || e[:i] == scope.pkg {
			break
		}
		e = e[:i]
	}
	if !complete {
		// An inherited field from outside the tree would shadow any import.
		return constRef{}, false, false, name, unresolvedOutside
	}
	if fq, ok := scope.staticSingle[name]; ok {
		if r, found, _ := idx.fieldIn(fq, name, 0); found {
			return r, false, true, "", ""
		}
		return constRef{}, false, false, name, unresolvedOutside
	}
	var cands []constRef
	for _, w := range scope.staticWild {
		if _, inTree := idx.classes[w]; !inTree {
			return constRef{}, false, false, name, unresolvedOutside
		}
		if r, found, _ := idx.fieldIn(w, name, 0); found {
			cands = append(cands, r)
		}
	}
	switch len(cands) {
	case 1:
		return cands[0], false, true, "", ""
	case 0:
		return constRef{}, false, false, name, unresolvedOutside
	}
	return constRef{}, false, false, name, unresolvedAmbiguous
}

// evalList evaluates a chain matcher argument to its patterns (Amendment 1):
// one string by eval's rules, or an array — inline, or a static final
// String[] field reached by Java's name resolution — whose every element
// evaluates. A local array is not read: its elements can be reassigned.
// ok is false for anything else; a partial list is never returned.
func (idx *constIndex) evalList(n *sitter.Node, scope *fileScope, cls string, depth int) ([]string, bool) {
	if n == nil || depth > 24 {
		return nil, false
	}
	src := scope.src
	switch n.Type() {
	case "array_initializer":
		var out []string
		for _, e := range namedChildren(n) {
			switch e.Type() {
			case "line_comment", "block_comment":
				continue
			}
			v, ok, _, _ := idx.eval(e, scope, cls, depth+1)
			if !ok {
				return nil, false
			}
			out = append(out, v)
		}
		return out, len(out) > 0
	case "array_creation_expression":
		return idx.evalList(findChildByType(n, "array_initializer"), scope, cls, depth+1)
	case "identifier":
		r, local, ok, _, _ := idx.resolveName(n, scope, cls)
		if !ok {
			return nil, false
		}
		if isArrayValue(r.value) {
			if local {
				return nil, false
			}
			return idx.evalList(r.value, r.scope, r.class, depth+1)
		}
	case "field_access":
		obj, field := n.ChildByFieldName("object"), n.ChildByFieldName("field")
		if obj != nil && field != nil {
			if fq, ok, _ := idx.resolveClass(obj.Content(src), scope, cls); ok {
				if r, found, _ := idx.fieldIn(fq, field.Content(src), 0); found && isArrayValue(r.value) {
					return idx.evalList(r.value, r.scope, r.class, depth+1)
				}
			}
		}
	}
	if v, ok, _, _ := idx.eval(n, scope, cls, depth); ok {
		return []string{v}, true
	}
	return nil, false
}

func isArrayValue(n *sitter.Node) bool {
	return n != nil && (n.Type() == "array_initializer" || n.Type() == "array_creation_expression")
}

// enumName evaluates E.C.name() to "C", Java's defined meaning of name(),
// where E resolves to an enum in the tree declaring C. toString() is not
// accepted: an enum may override it.
func (idx *constIndex) enumName(n *sitter.Node, scope *fileScope, cls string) (string, bool) {
	src := scope.src
	name, obj := n.ChildByFieldName("name"), n.ChildByFieldName("object")
	args := n.ChildByFieldName("arguments")
	if name == nil || obj == nil || name.Content(src) != "name" || (args != nil && args.NamedChildCount() != 0) {
		return "", false
	}
	var enum, c string
	switch obj.Type() {
	case "field_access":
		e, f := obj.ChildByFieldName("object"), obj.ChildByFieldName("field")
		if e == nil || f == nil {
			return "", false
		}
		fq, ok, _ := idx.resolveClass(e.Content(src), scope, cls)
		if !ok {
			return "", false
		}
		enum, c = fq, f.Content(src)
	case "identifier":
		// C.name() inside the enum itself, or through a static import.
		c = obj.Content(src)
		if fq, ok := scope.staticSingle[c]; ok {
			enum = fq
		} else {
			for e := cls; e != ""; {
				if idx.enums[e][c] {
					enum = e
					break
				}
				i := strings.LastIndex(e, ".")
				if i < 0 {
					break
				}
				e = e[:i]
			}
		}
	default:
		return "", false
	}
	if idx.enums[enum][c] {
		return c, true
	}
	return "", false
}

func lastSegment(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

// decodeStringLiteral returns a string_literal's value with its escape
// sequences decoded. stringLiteralValue reads only the first fragment,
// which drops everything after an escape: videochat ends its hierarchy
// with "\n", a rule separator (ADR 0038 §2). A text block is not read.
func decodeStringLiteral(n *sitter.Node, src []byte) (string, bool) {
	if strings.HasPrefix(n.Content(src), `"""`) {
		return "", false
	}
	var b strings.Builder
	for _, c := range namedChildren(n) {
		switch c.Type() {
		case "string_fragment":
			b.WriteString(c.Content(src))
		case "escape_sequence":
			switch c.Content(src) {
			case `\n`:
				b.WriteByte('\n')
			case `\t`:
				b.WriteByte('\t')
			case `\r`:
				b.WriteByte('\r')
			case `\s`:
				b.WriteByte(' ')
			case `\"`:
				b.WriteByte('"')
			case `\'`:
				b.WriteByte('\'')
			case `\\`:
				b.WriteByte('\\')
			default:
				return "", false
			}
		default:
			return "", false
		}
	}
	return b.String(), true
}

func isChar(n *sitter.Node) bool { return n != nil && n.Type() == "character_literal" }

// decodeCharLiteral returns a Java char literal's character, for a String
// concatenation such as molgenis's API_PATH + '/' + API_VERSION.
func decodeCharLiteral(lit string) (string, bool) {
	if len(lit) < 3 || lit[0] != '\'' || lit[len(lit)-1] != '\'' {
		return "", false
	}
	body := lit[1 : len(lit)-1]
	switch body {
	case `\n`:
		return "\n", true
	case `\t`:
		return "\t", true
	case `\\`:
		return `\`, true
	case `\'`:
		return "'", true
	case `\"`:
		return `"`, true
	}
	if len([]rune(body)) == 1 && body != `\` {
		return body, true
	}
	return "", false
}
