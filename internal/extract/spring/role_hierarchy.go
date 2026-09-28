package spring

import (
	"regexp"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chebilax/sphinxor/internal/model"
)

// hierarchyScan is what the per-file pass gathers for ADR 0038 Stage 1,
// resolved once every file has been seen (resolveRoleHierarchy): the
// string a hierarchy is built from may be a constant declared in another
// file, and the version evidence is project-wide.
type hierarchyScan struct {
	sites []hierarchySite
	// builder: RoleHierarchyImpl.withDefaultRolePrefix()/withRolePrefix(),
	// the 6.3 builder, which is not read (ADR 0038 §1).
	builder bool
	// floor63: an API that exists only from Spring Security 6.3
	// (fromHierarchy, the builder) — ADR 0038 §13.
	floor63 bool
	// methodWired: setRoleHierarchy(...) on a receiver declared as a
	// MethodSecurityExpressionHandler, which reaches @PreAuthorize on every
	// version (ADR 0038 Finding 2).
	methodWired bool
	// condition: a @Profile/@Conditional… on a class declaring a
	// hierarchy, verbatim.
	condition string
}

// hierarchySite is one place a hierarchy string is handed to Spring:
// RoleHierarchyImpl.fromHierarchy(arg) or setHierarchy(arg).
type hierarchySite struct {
	arg   *sitter.Node
	src   []byte
	class string
}

// scanRoleHierarchy records whether this file declares a Spring Security
// role hierarchy — docs/decisions/0031-role-hierarchy.md §1/§3 — and, for
// docs/decisions/0038-role-hierarchy-read.md Stage 1, where its rules are
// handed over, so resolveRoleHierarchy can read them.
//
// Three shapes are detected, per ADR 0031 §3:
//
//   - a @Bean method whose return type is RoleHierarchy;
//   - a RoleHierarchyImpl.fromHierarchy(...) / .withDefaultRolePrefix()
//     call, which is how one is built in 6.3+;
//   - a setRoleHierarchy(...) call on an expression handler.
//
// Reading the rules changes no grant: Stage 1 states them in the warning
// and expands nothing (ADR 0038 §12).
//
// What this misses is stated rather than implied: a hierarchy configured
// in YAML, in properties, or in Kotlin. Both are already out of scope
// (ADR 0011 §1), and neither becomes newly hidden by this scan — but a
// project without the warning is "none located", never "confirmed none",
// the same boundary ADR 0015 draws for @EnableMethodSecurity.
func scanRoleHierarchy(root *sitter.Node, src []byte, status *model.RoleHierarchyStatus, scan *hierarchyScan) {
	seen := map[string]bool{}
	for _, c := range status.DeclaredIn {
		seen[c] = true
	}

	record := func(class string, classNode *sitter.Node) {
		status.Found = true
		if class != "" && !seen[class] {
			seen[class] = true
			status.DeclaredIn = append(status.DeclaredIn, class)
		}
		if scan.condition == "" && classNode != nil {
			scan.condition = conditionOf(classNode, src)
		}
	}

	// setHierarchy is a generic name. It is only taken as a hierarchy
	// site in a file that names RoleHierarchyImpl at all.
	namesImpl := strings.Contains(string(src), "RoleHierarchyImpl")

	var walk func(n *sitter.Node, enclosing string, classNode *sitter.Node)
	walk = func(n *sitter.Node, enclosing string, classNode *sitter.Node) {
		switch n.Type() {
		case "class_declaration":
			if name := n.ChildByFieldName("name"); name != nil {
				enclosing = name.Content(src)
				classNode = n
			}
		case "method_declaration":
			// A @Bean method returning RoleHierarchy. The return type is
			// the declaration, so this catches the bean however its body
			// is written.
			if t := n.ChildByFieldName("type"); t != nil && isRoleHierarchyType(t.Content(src)) {
				if hasAny(annotationsOf(n, src), map[string]bool{"Bean": true}) {
					record(enclosing, classNode)
				}
			}
		case "method_invocation":
			if name := n.ChildByFieldName("name"); name != nil {
				obj := n.ChildByFieldName("object")
				switch name.Content(src) {
				case "fromHierarchy", "withDefaultRolePrefix", "withRolePrefix":
					// Only when invoked on RoleHierarchyImpl — these are
					// generic enough names that the receiver matters.
					if obj != nil && strings.Contains(obj.Content(src), "RoleHierarchy") {
						record(enclosing, classNode)
						scan.floor63 = true
						if name.Content(src) == "fromHierarchy" {
							scan.addSite(n, src, enclosing)
						} else {
							scan.builder = true
						}
					}
				case "setHierarchy":
					if namesImpl {
						scan.addSite(n, src, enclosing)
					}
				case "setRoleHierarchy":
					record(enclosing, classNode)
					if obj != nil && obj.Type() == "identifier" &&
						strings.Contains(localType(n, obj.Content(src), src), "MethodSecurityExpressionHandler") {
						scan.methodWired = true
					}
				}
			}
		}
		for _, c := range namedChildren(n) {
			walk(c, enclosing, classNode)
		}
	}
	walk(root, "", nil)
}

func (s *hierarchyScan) addSite(call *sitter.Node, src []byte, class string) {
	args := call.ChildByFieldName("arguments")
	if args == nil || args.NamedChildCount() != 1 {
		return
	}
	s.sites = append(s.sites, hierarchySite{arg: args.NamedChild(0), src: src, class: class})
}

// conditionOf returns a class's @Profile or @Conditional… annotation,
// verbatim, or "".
func conditionOf(classNode *sitter.Node, src []byte) string {
	for _, a := range annotationsOf(classNode, src) {
		if a.Name == "Profile" || strings.HasPrefix(a.Name, "Conditional") {
			return a.Node.Content(src)
		}
	}
	return ""
}

// localType returns the declared type of a local variable named name in
// the method or constructor enclosing n, or "".
func localType(n *sitter.Node, name string, src []byte) string {
	scope := enclosingCallable(n)
	if scope == nil {
		return ""
	}
	if decl := findLocal(scope, name, src); decl != nil {
		if t := decl.ChildByFieldName("type"); t != nil {
			return t.Content(src)
		}
	}
	return ""
}

func enclosingCallable(n *sitter.Node) *sitter.Node {
	for p := n.Parent(); p != nil; p = p.Parent() {
		switch p.Type() {
		case "method_declaration", "constructor_declaration", "lambda_expression":
			return p
		}
	}
	return nil
}

// findLocal returns the one local_variable_declaration in scope declaring
// name, or nil when there is none or more than one.
func findLocal(scope *sitter.Node, name string, src []byte) *sitter.Node {
	var found []*sitter.Node
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "local_variable_declaration" {
			for _, d := range namedChildren(n) {
				if d.Type() == "variable_declarator" {
					if id := d.ChildByFieldName("name"); id != nil && id.Content(src) == name {
						found = append(found, n)
					}
				}
			}
		}
		for _, c := range namedChildren(n) {
			walk(c)
		}
	}
	walk(scope)
	if len(found) != 1 {
		return nil
	}
	return found[0]
}

// isRoleHierarchyType reports whether a declared return type is Spring's
// RoleHierarchy, written plainly or fully qualified.
func isRoleHierarchyType(text string) bool {
	text = strings.TrimSpace(text)
	return text == "RoleHierarchy" ||
		text == "org.springframework.security.access.hierarchicalroles.RoleHierarchy"
}

// resolveRoleHierarchy is ADR 0038 Stage 1: it evaluates the hierarchy
// string, parses it in the grammar §2 allows, and records which layers'
// use of it rests on an assumed version (§12). It runs after the URL
// layer is known, because an analyzed authorizeHttpRequests layer is one
// of those.
//
// The Spring Security facts it encodes were read at the versions ADR
// 0038 §13 lists, and are re-checked on every major version — the parser
// regexes below and the version numbers in AssumedReach are the parts
// that can go stale.
func resolveRoleHierarchy(files []parsedFile, scan *hierarchyScan, ms model.MethodSecurityStatus, urlAnalyzed, servletChain bool, status *model.RoleHierarchyStatus) {
	if !status.Found {
		return
	}
	status.Condition = scan.condition
	status.AssumedReach = assumedReach(scan, ms, urlAnalyzed)

	switch {
	case len(scan.sites) == 0 && scan.builder:
		status.NotRead = "it is built with RoleHierarchyImpl's builder, which is not read"
		return
	case len(scan.sites) == 0:
		status.NotRead = "they are not handed to RoleHierarchyImpl as a string, so they are built by code Sphinxor does not evaluate"
		return
	}

	idx := buildConstIndex(files)
	value := ""
	for i, site := range scan.sites {
		v, ok, unresolved := idx.eval(site.arg, site.src, site.class, 0)
		if !ok && strings.HasPrefix(unresolved, `"""`) {
			status.NotRead = "they are written as a text block, which is not read"
			return
		}
		if !ok {
			status.NotRead = "they come from " + clip(unresolved) + ", which Sphinxor does not evaluate — it may read a configuration property, a database, or another runtime value"
			return
		}
		if i > 0 && v != value {
			status.NotRead = "more than one hierarchy string is declared, and they differ"
			return
		}
		value = v
	}

	// A floor at or above 5.2 fixes the grammar (ADR 0038 §13): the
	// SecurityFilterChain bean type exists from 5.4, @EnableMethodSecurity
	// from 5.6, fromHierarchy from 6.3.
	floor52 := scan.floor63 || ms.Modern || servletChain
	edges, upTo50, from52, ok := parseHierarchy(value, floor52)
	switch {
	case !ok:
		status.UpTo50, status.From52 = upTo50, from52
	case len(edges) == 0:
		status.NotRead = "the string contains no rule"
	case hasCycle(edges):
		status.NotRead = "they contain a cycle, which Spring Security rejects at startup"
	default:
		status.Read, status.Edges = true, edges
	}
}

// assumedReach is ADR 0038 §12's list: the layers whose use of a
// hierarchy depends on a version nothing in the source fixes.
func assumedReach(scan *hierarchyScan, ms model.MethodSecurityStatus, urlAnalyzed bool) []string {
	if scan.floor63 {
		return nil
	}
	var out []string
	if ms.Modern {
		if ms.PrePostEnabled && !scan.methodWired && !ms.Legacy {
			out = append(out, "@PreAuthorize")
		}
		if ms.SecuredEnabled {
			out = append(out, "@Secured")
		}
		if ms.Jsr250Enabled {
			out = append(out, "@RolesAllowed")
		}
	}
	if urlAnalyzed {
		out = append(out, "authorizeHttpRequests")
	}
	return out
}

var (
	// universalLine is the one form Spring Security ≤5.0, 5.1 and ≥5.2
	// all read the same way: one pair, single spaces (ADR 0038 §13).
	universalLine = regexp.MustCompile(`^[^\s>]+ > [^\s>]+$`)
	// legacyPair is RoleHierarchyImpl's parser up to 5.0, verbatim.
	legacyPair = regexp.MustCompile(`\s*([^\s>]+)\s*>\s*([^\s>]+)`)
	// modernSeparator is its per-line separator from 5.2 (commit
	// be0ad673c2), still the parser in 6.5.11, 7.1.1 and main.
	modernSeparator = regexp.MustCompile(`\s+>\s+`)
)

// parseHierarchy applies ADR 0038 §2. ok is false when the string is not
// in the universal form, nothing fixes the grammar, and the ≤5.0 and ≥5.2
// readings differ — then both are returned and neither is chosen.
//
// 5.1 alone is not distinguished: where ≤5.0 and ≥5.2 agree, that is the
// reading. 5.1 was one minor line, and §13 records the choice.
func parseHierarchy(s string, floor52 bool) (edges, upTo50, from52 []model.RoleHierarchyEdge, ok bool) {
	universal := true
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !universalLine.MatchString(line) {
			universal = false
		}
		roles := modernSeparator.Split(line, -1)
		for i := 1; i < len(roles); i++ {
			from52 = append(from52, model.RoleHierarchyEdge{Higher: roles[i-1], Lower: roles[i]})
		}
	}
	if universal || floor52 {
		return from52, nil, nil, true
	}
	for _, m := range legacyPair.FindAllStringSubmatch(s, -1) {
		upTo50 = append(upTo50, model.RoleHierarchyEdge{Higher: m[1], Lower: m[2]})
	}
	if equalEdges(upTo50, from52) {
		return from52, nil, nil, true
	}
	return nil, upTo50, from52, false
}

func equalEdges(a, b []model.RoleHierarchyEdge) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasCycle(edges []model.RoleHierarchyEdge) bool {
	next := map[string][]string{}
	for _, e := range edges {
		next[e.Higher] = append(next[e.Higher], e.Lower)
	}
	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	var visit func(string) bool
	visit = func(r string) bool {
		switch state[r] {
		case visiting:
			return true
		case done:
			return false
		}
		state[r] = visiting
		for _, n := range next[r] {
			if visit(n) {
				return true
			}
		}
		state[r] = done
		return false
	}
	for _, e := range edges {
		if visit(e.Higher) {
			return true
		}
	}
	return false
}

func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 60 {
		s = s[:57] + "..."
	}
	return s
}

// constIndex is the project's static final String fields and enum
// constants, by simple class name, for evaluating a hierarchy string built
// from them (ADR 0038 §1). A simple name declared twice is ambiguous and
// resolves to nothing.
type constIndex struct {
	fields    map[string]map[string]constRef
	enums     map[string]map[string]bool
	ambiguous map[string]bool
}

type constRef struct {
	value *sitter.Node
	src   []byte
}

func buildConstIndex(files []parsedFile) *constIndex {
	idx := &constIndex{
		fields:    map[string]map[string]constRef{},
		enums:     map[string]map[string]bool{},
		ambiguous: map[string]bool{},
	}
	declared := map[string]bool{}
	for _, f := range files {
		var walk func(n *sitter.Node)
		walk = func(n *sitter.Node) {
			switch n.Type() {
			case "class_declaration", "interface_declaration", "enum_declaration":
				name := n.ChildByFieldName("name")
				if name == nil {
					break
				}
				cls := name.Content(f.src)
				if declared[cls] {
					idx.ambiguous[cls] = true
				}
				declared[cls] = true
				idx.indexBody(n, cls, f.src)
			}
			for _, c := range namedChildren(n) {
				walk(c)
			}
		}
		walk(f.tree.RootNode())
	}
	return idx
}

func (idx *constIndex) indexBody(decl *sitter.Node, cls string, src []byte) {
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
		idx.enums[cls] = consts
	}
	for _, m := range members {
		constant := m.Type() == "constant_declaration" ||
			(m.Type() == "field_declaration" && hasModifierKeyword(m, "static") && hasModifierKeyword(m, "final"))
		if !constant {
			continue
		}
		if t := m.ChildByFieldName("type"); t == nil || t.Content(src) != "String" {
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
			if idx.fields[cls] == nil {
				idx.fields[cls] = map[string]constRef{}
			}
			idx.fields[cls][name.Content(src)] = constRef{value: value, src: src}
		}
	}
}

// eval evaluates n to one string, per ADR 0038 §1: string literals, +
// concatenation, a local variable or a same-project static final String
// initialized that way, and E.C.name() on a same-project enum. It stops
// at the first term it cannot evaluate and returns that term's text; a
// partial string is never returned.
func (idx *constIndex) eval(n *sitter.Node, src []byte, class string, depth int) (value string, ok bool, unresolved string) {
	if n == nil {
		return "", false, ""
	}
	if depth > 16 {
		return "", false, n.Content(src)
	}
	switch n.Type() {
	case "string_literal":
		if v, ok := decodeStringLiteral(n, src); ok {
			return v, true, ""
		}
	case "parenthesized_expression":
		return idx.eval(n.NamedChild(0), src, class, depth+1)
	case "binary_expression":
		if op := n.ChildByFieldName("operator"); op != nil && op.Content(src) == "+" {
			l, ok, u := idx.eval(n.ChildByFieldName("left"), src, class, depth+1)
			if !ok {
				return "", false, u
			}
			r, ok, u := idx.eval(n.ChildByFieldName("right"), src, class, depth+1)
			if !ok {
				return "", false, u
			}
			return l + r, true, ""
		}
	case "identifier":
		name := n.Content(src)
		if scope := enclosingCallable(n); scope != nil {
			if decl := findLocal(scope, name, src); decl != nil {
				for _, d := range namedChildren(decl) {
					if d.Type() == "variable_declarator" {
						if id := d.ChildByFieldName("name"); id != nil && id.Content(src) == name {
							return idx.eval(d.ChildByFieldName("value"), src, class, depth+1)
						}
					}
				}
			}
		}
		if ref, ok := idx.field(class, name); ok {
			return idx.eval(ref.value, ref.src, class, depth+1)
		}
	case "field_access":
		obj, field := n.ChildByFieldName("object"), n.ChildByFieldName("field")
		if obj != nil && field != nil {
			cls := lastSegment(obj.Content(src))
			if ref, ok := idx.field(cls, field.Content(src)); ok {
				return idx.eval(ref.value, ref.src, cls, depth+1)
			}
		}
	case "method_invocation":
		if c, ok := idx.enumName(n, src); ok {
			return c, true, ""
		}
	}
	return "", false, n.Content(src)
}

func (idx *constIndex) field(cls, name string) (constRef, bool) {
	if idx.ambiguous[cls] {
		return constRef{}, false
	}
	ref, ok := idx.fields[cls][name]
	return ref, ok
}

// enumName evaluates E.C.name() — or C.name(), where exactly one project
// enum declares C — to "C", which is Java's defined meaning of name().
// toString() is not accepted: an enum may override it.
func (idx *constIndex) enumName(n *sitter.Node, src []byte) (string, bool) {
	name, obj := n.ChildByFieldName("name"), n.ChildByFieldName("object")
	args := n.ChildByFieldName("arguments")
	if name == nil || obj == nil || name.Content(src) != "name" || (args != nil && args.NamedChildCount() != 0) {
		return "", false
	}
	switch obj.Type() {
	case "field_access":
		enum, c := obj.ChildByFieldName("object"), obj.ChildByFieldName("field")
		if enum == nil || c == nil {
			return "", false
		}
		e := lastSegment(enum.Content(src))
		if !idx.ambiguous[e] && idx.enums[e][c.Content(src)] {
			return c.Content(src), true
		}
	case "identifier":
		c, matches := obj.Content(src), 0
		for e, consts := range idx.enums {
			if consts[c] && !idx.ambiguous[e] {
				matches++
			}
		}
		if matches == 1 {
			return c, true
		}
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
// which is right for a route path and wrong here: videochat ends its
// hierarchy with "\n", and a newline is a rule separator (ADR 0038 §2).
// A text block is not read.
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
