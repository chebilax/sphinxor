package spring

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chebilax/sphinxor/internal/model"
)

// scanProjectEnforcement records endpoints carrying an annotation the
// project declares and enforces with its own code — ADR 0023 Amendment 1.
//
// Detected by structure, never by name: the annotation type is declared in
// the analyzed source, and it is read by
//   - an @Aspect whose pointcut expression names it, @annotation(...) or
//     @within(...), directly or through a bound advice parameter; or
//   - a Spring MVC HandlerInterceptor calling get…/find…/has…Annotation
//     with its class literal.
//
// A reader counts only if it can stop the request (canStop). That is for
// the export, where over-inclusion omits more, the safe direction; it
// still counts loggers that can throw, and findings must not use it
// (ADR 0047).
func scanProjectEnforcement(files []parsedFile, b *builder) {
	declared := map[string]bool{}
	for _, f := range files {
		walkTree(f.tree.RootNode(), func(n *sitter.Node) {
			if n.Type() == "annotation_type_declaration" {
				if name := n.ChildByFieldName("name"); name != nil {
					declared[name.Content(f.src)] = true
				}
			}
		})
	}
	if len(declared) == 0 {
		return
	}

	idx := newStopIndex(files)
	readers := map[string][]string{} // annotation simple name -> reader classes
	addReader := func(name, reader string) {
		name = lastSegment(name)
		if !declared[name] {
			return
		}
		for _, r := range readers[name] {
			if r == reader {
				return
			}
		}
		readers[name] = append(readers[name], reader)
	}
	for _, f := range files {
		walkTree(f.tree.RootNode(), func(n *sitter.Node) {
			if n.Type() != "class_declaration" {
				return
			}
			cls := n.ChildByFieldName("name").Content(f.src)
			reader := cls + " (" + filepath.Base(f.relPath) + ")"
			ci := idx.classes[cls]
			if hasAnnotationNamed(n, f.src, "Aspect") {
				for _, name := range pointcutAnnotations(n, f.src, idx, ci) {
					addReader(name, reader)
				}
			}
			if isHandlerInterceptor(n, f.src) {
				for _, name := range annotationLookups(n, f.src, idx, ci) {
					addReader(name, reader)
				}
			}
		})
	}
	if len(readers) == 0 {
		return
	}

	// Handlers by (file, method name): overloads share their annotations,
	// which over-includes, the safe direction for the export.
	carried := map[string]map[string]bool{}
	for _, f := range files {
		var walk func(n *sitter.Node, cls []annotationCall)
		walk = func(n *sitter.Node, cls []annotationCall) {
			switch n.Type() {
			case "class_declaration", "interface_declaration":
				cls = annotationsOf(n, f.src)
			case "method_declaration":
				k := f.relPath + "|" + n.ChildByFieldName("name").Content(f.src)
				for _, a := range append(annotationsOf(n, f.src), cls...) {
					if len(readers[a.Name]) > 0 {
						if carried[k] == nil {
							carried[k] = map[string]bool{}
						}
						carried[k][a.Name] = true
					}
				}
			}
			for _, c := range namedChildren(n) {
				walk(c, cls)
			}
		}
		walk(f.tree.RootNode(), nil)
	}
	for _, e := range b.model.Endpoints {
		names := make([]string, 0, len(carried[e.File+"|"+e.HandlerName]))
		for name := range carried[e.File+"|"+e.HandlerName] {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			b.model.ProjectEnforcements = append(b.model.ProjectEnforcements, model.ProjectEnforcement{
				EndpointID: e.ID,
				Annotation: name,
				Readers:    readers[name],
			})
		}
	}
}

func walkTree(n *sitter.Node, visit func(*sitter.Node)) {
	visit(n)
	for _, c := range namedChildren(n) {
		walkTree(c, visit)
	}
}

func hasAnnotationNamed(decl *sitter.Node, src []byte, name string) bool {
	for _, a := range annotationsOf(decl, src) {
		if a.Name == name {
			return true
		}
	}
	return false
}

var adviceAnnotations = map[string]bool{
	"Pointcut": true, "Around": true, "Before": true, "After": true,
	"AfterReturning": true, "AfterThrowing": true,
}

var pointcutDesignator = regexp.MustCompile(`@(?:annotation|within)\(\s*([\w.]+)\s*\)`)

// pointcutAnnotations names the annotation types an aspect's advice selects,
// for advice that can stop the request (canStop). A lower-case designator
// argument is a bound advice parameter, and the parameter's declared type
// is the annotation. A @Pointcut method's own expression counts when some
// advice of the aspect using it can stop the request.
func pointcutAnnotations(aspect *sitter.Node, src []byte, idx *stopIndex, ci *classInfo) []string {
	type advice struct {
		kind  string
		expr  string
		stops bool
		param map[string]string
	}
	var pointcuts = map[string]string{} // pointcut method name -> expression
	var advices []advice
	walkTree(aspect, func(n *sitter.Node) {
		if n.Type() != "method_declaration" {
			return
		}
		params := map[string]string{}
		if p := n.ChildByFieldName("parameters"); p != nil {
			for _, fp := range namedChildren(p) {
				t, name := fp.ChildByFieldName("type"), fp.ChildByFieldName("name")
				if t != nil && name != nil {
					params[name.Content(src)] = t.Content(src)
				}
			}
		}
		for _, a := range annotationsOf(n, src) {
			if !adviceAnnotations[a.Name] {
				continue
			}
			expr := a.Node.Content(src)
			if a.Name == "Pointcut" {
				pointcuts[n.ChildByFieldName("name").Content(src)] = expr
				continue
			}
			advices = append(advices, advice{kind: a.Name, expr: expr, stops: idx.canStop(n, src, ci, a.Name), param: params})
		}
	})
	var out []string
	add := func(expr string, params map[string]string) {
		for _, m := range pointcutDesignator.FindAllStringSubmatch(expr, -1) {
			name := m[1]
			if t, ok := params[name]; ok {
				name = t
			}
			out = append(out, name)
		}
	}
	for _, a := range advices {
		if !a.stops {
			continue
		}
		add(a.expr, a.param)
		// An advice naming a @Pointcut method by name inherits its expression.
		for pc, expr := range pointcuts {
			if strings.Contains(a.expr, pc+"(") {
				add(expr, a.param)
			}
		}
	}
	return out
}

func isHandlerInterceptor(cls *sitter.Node, src []byte) bool {
	for _, t := range append(supertypesOf(cls, src), superclassName(cls, src)) {
		switch lastSegment(t) {
		case "HandlerInterceptor", "AsyncHandlerInterceptor", "HandlerInterceptorAdapter":
			return true
		}
	}
	return false
}

func superclassName(cls *sitter.Node, src []byte) string {
	if s := findChildByType(cls, "superclass"); s != nil {
		return strings.TrimSpace(strings.TrimPrefix(s.Content(src), "extends"))
	}
	return ""
}

var annotationLookup = regexp.MustCompile(`^(?:get|find|has)\w*Annotation$`)

// annotationLookups names the annotation types an interceptor looks up with
// a class literal, when the method doing the lookup, or the interceptor's
// preHandle, can stop the request.
func annotationLookups(cls *sitter.Node, src []byte, idx *stopIndex, ci *classInfo) []string {
	var preHandle *sitter.Node
	walkTree(cls, func(n *sitter.Node) {
		if n.Type() == "method_declaration" && n.ChildByFieldName("name").Content(src) == "preHandle" {
			preHandle = n
		}
	})
	var out []string
	walkTree(cls, func(n *sitter.Node) {
		if n.Type() != "method_invocation" {
			return
		}
		name := n.ChildByFieldName("name")
		if name == nil || !annotationLookup.MatchString(name.Content(src)) {
			return
		}
		m := n
		for m != nil && m.Type() != "method_declaration" {
			m = m.Parent()
		}
		if m == nil || !(idx.canStop(m, src, ci, "interceptor") || preHandle != nil && idx.canStop(preHandle, src, ci, "interceptor")) {
			return
		}
		for _, a := range namedChildren(n.ChildByFieldName("arguments")) {
			if a.Type() == "class_literal" {
				if t := a.NamedChild(0); t != nil {
					out = append(out, t.Content(src))
				}
			}
		}
	})
	return out
}

// stopIndex is the analyzed source's classes, for deciding whether an
// advice or interceptor method can stop the request (ADR 0023 Amendment 1).
type stopIndex struct {
	classes map[string]*classInfo // simple name -> class; a name declared twice keeps the first
}

type classInfo struct {
	src     []byte
	fields  map[string]string         // field name -> declared type
	methods map[string][]*sitter.Node // method name -> declarations
}

func newStopIndex(files []parsedFile) *stopIndex {
	idx := &stopIndex{classes: map[string]*classInfo{}}
	for _, f := range files {
		walkTree(f.tree.RootNode(), func(n *sitter.Node) {
			switch n.Type() {
			case "class_declaration", "interface_declaration", "enum_declaration":
			default:
				return
			}
			name := n.ChildByFieldName("name").Content(f.src)
			if _, dup := idx.classes[name]; dup {
				return
			}
			ci := &classInfo{src: f.src, fields: map[string]string{}, methods: map[string][]*sitter.Node{}}
			if body := n.ChildByFieldName("body"); body != nil {
				for _, m := range namedChildren(body) {
					switch m.Type() {
					case "field_declaration":
						t := typeSimpleName(m.ChildByFieldName("type"), f.src)
						for _, d := range namedChildren(m) {
							if d.Type() == "variable_declarator" {
								ci.fields[d.ChildByFieldName("name").Content(f.src)] = t
							}
						}
					case "method_declaration":
						mn := m.ChildByFieldName("name").Content(f.src)
						ci.methods[mn] = append(ci.methods[mn], m)
					}
				}
			}
			idx.classes[name] = ci
		})
	}
	return idx
}

func typeSimpleName(t *sitter.Node, src []byte) string {
	if t == nil {
		return ""
	}
	s := t.Content(src)
	if i := strings.Index(s, "<"); i >= 0 {
		s = s[:i]
	}
	return lastSegment(strings.TrimSpace(s))
}

// canStop reports whether an advice or interceptor method can stop the
// request, per ADR 0023 Amendment 1:
//   - an exception can escape it: a throw, or a call to a project method
//     whose body throws, one level deep; a project call beyond that level,
//     or a call whose receiver cannot be typed, counts as able to stop —
//     in doubt, omit. A throw or call inside a try whose catch of
//     Exception or Throwable does not rethrow cannot escape;
//   - it sends an error response (sendError, setStatus);
//   - an interceptor returns false;
//   - an @Around proceeds only conditionally, or never.
//
// After-advice counts when an exception can escape it: it still replaces
// the response, like @PostAuthorize on a read.
func (idx *stopIndex) canStop(m *sitter.Node, src []byte, ci *classInfo, kind string) bool {
	body := m.ChildByFieldName("body")
	if body == nil {
		return true // abstract or native: nothing to read, in doubt
	}
	if idx.escapes(m, src, ci, 0) {
		return true
	}
	stop := false
	walkTree(body, func(n *sitter.Node) {
		switch n.Type() {
		case "method_invocation":
			switch n.ChildByFieldName("name").Content(src) {
			case "sendError", "setStatus":
				stop = true
			}
		case "return_statement":
			if kind == "interceptor" {
				if v := n.NamedChild(0); v != nil && v.Type() == "false" {
					stop = true
				}
			}
		}
	})
	if stop {
		return true
	}
	if kind == "Around" {
		proceeds, conditional := false, false
		walkTree(body, func(n *sitter.Node) {
			if n.Type() == "method_invocation" && n.ChildByFieldName("name").Content(src) == "proceed" {
				proceeds = true
				for p := n.Parent(); p != nil && p != m; p = p.Parent() {
					switch p.Type() {
					case "if_statement", "switch_expression", "switch_statement", "ternary_expression", "catch_clause":
						conditional = true
					}
				}
			}
		})
		return !proceeds || conditional
	}
	return false
}

// escapes reports whether an exception can leave m (see canStop).
func (idx *stopIndex) escapes(m *sitter.Node, src []byte, ci *classInfo, depth int) bool {
	body := m.ChildByFieldName("body")
	if body == nil {
		return true
	}
	locals := map[string]string{}
	if p := m.ChildByFieldName("parameters"); p != nil {
		for _, fp := range namedChildren(p) {
			if t, n := fp.ChildByFieldName("type"), fp.ChildByFieldName("name"); t != nil && n != nil {
				locals[n.Content(src)] = typeSimpleName(t, src)
			}
		}
	}
	walkTree(body, func(n *sitter.Node) {
		switch n.Type() {
		case "local_variable_declaration":
			t := typeSimpleName(n.ChildByFieldName("type"), src)
			for _, d := range namedChildren(n) {
				if d.Type() == "variable_declarator" {
					locals[d.ChildByFieldName("name").Content(src)] = t
				}
			}
		case "catch_formal_parameter":
			// A multi-catch is typed by its first alternative: every
			// alternative is a Throwable, never a project class to follow.
			if t, name := findChildByType(n, "catch_type"), n.ChildByFieldName("name"); t != nil && name != nil {
				locals[name.Content(src)] = lastSegment(strings.TrimSpace(strings.Split(t.Content(src), "|")[0]))
			}
		case "enhanced_for_statement":
			if t, name := n.ChildByFieldName("type"), n.ChildByFieldName("name"); t != nil && name != nil {
				locals[name.Content(src)] = typeSimpleName(t, src)
			}
		}
	})
	esc := false
	walkTree(body, func(n *sitter.Node) {
		if esc {
			return
		}
		switch n.Type() {
		case "throw_statement":
			if !swallowed(n, m, src) {
				esc = true
			}
		case "method_invocation":
			if swallowed(n, m, src) {
				return
			}
			callees, doubt := idx.callees(n, src, ci, locals)
			if doubt {
				esc = true
				return
			}
			for _, c := range callees {
				if depth >= 1 {
					esc = true // a project call beyond the level followed
					return
				}
				if idx.escapes(c.node, c.ci.src, c.ci, depth+1) {
					esc = true
					return
				}
			}
		}
	})
	return esc
}

type callee struct {
	node *sitter.Node
	ci   *classInfo
}

// callees resolves a call to project method declarations by its receiver's
// declared type. doubt is true when the receiver cannot be typed and could
// be a project class; a call on an external type has no callees.
func (idx *stopIndex) callees(call *sitter.Node, src []byte, ci *classInfo, locals map[string]string) ([]callee, bool) {
	name := call.ChildByFieldName("name").Content(src)
	obj := call.ChildByFieldName("object")
	inClass := func(c *classInfo) ([]callee, bool) {
		if c == nil {
			return nil, true
		}
		var out []callee
		for _, d := range c.methods[name] {
			out = append(out, callee{d, c})
		}
		return out, false
	}
	if obj == nil || obj.Type() == "this" {
		out, doubt := inClass(ci)
		if len(out) > 0 || doubt {
			return out, doubt
		}
		// Not declared in the class: inherited, or a static import. If a
		// project method has that name it may be the one.
		for _, c := range idx.classes {
			if len(c.methods[name]) > 0 {
				return nil, true
			}
		}
		return nil, false
	}
	root := obj
	for root.Type() == "method_invocation" || root.Type() == "field_access" {
		next := root.ChildByFieldName("object")
		if next == nil {
			break
		}
		root = next
	}
	if root.Type() != "identifier" {
		return nil, true
	}
	r := root.Content(src)
	t, ok := locals[r]
	if !ok && ci != nil {
		t, ok = ci.fields[r]
	}
	if !ok {
		t = r // a class name: a static call
	}
	pc, isProject := idx.classes[t]
	if !isProject {
		if ok || (r != "" && r[0] >= 'A' && r[0] <= 'Z') {
			return nil, false // an external type
		}
		return nil, true // an untyped name
	}
	if root != obj {
		return nil, true // a chain on a project object: which method is unknown
	}
	out, _ := inClass(pc)
	if len(out) == 0 {
		return nil, true // inherited from outside the class, in doubt
	}
	return out, false
}

// swallowed reports whether n sits in the body of a try, inside m, whose
// catch of Exception or Throwable does not rethrow: nothing thrown there
// can leave the method.
func swallowed(n, m *sitter.Node, src []byte) bool {
	for child, p := n, n.Parent(); p != nil && p != m; child, p = p, p.Parent() {
		if p.Type() != "try_statement" && p.Type() != "try_with_resources_statement" {
			continue
		}
		if body := p.ChildByFieldName("body"); body == nil || body != child {
			continue
		}
		for _, c := range namedChildren(p) {
			if c.Type() != "catch_clause" {
				continue
			}
			ct := ""
			if fp := findChildByType(c, "catch_formal_parameter"); fp != nil {
				if t := findChildByType(fp, "catch_type"); t != nil {
					ct = t.Content(src)
				}
			}
			catchAll := false
			for _, t := range strings.Split(ct, "|") {
				switch lastSegment(strings.TrimSpace(t)) {
				case "Exception", "Throwable":
					catchAll = true
				}
			}
			rethrows := false
			walkTree(c, func(x *sitter.Node) {
				if x.Type() == "throw_statement" {
					rethrows = true
				}
			})
			if catchAll && !rethrows {
				return true
			}
		}
	}
	return false
}
