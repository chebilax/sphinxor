package nestjs

import (
	sitter "github.com/smacker/go-tree-sitter"
)

// sourceNode pairs a tree-sitter node with the source buffer it was
// parsed from. Necessary once composite-decorator resolution mixes nodes
// from the composite's defining file (e.g. http.decorators.ts) with
// nodes from the call site's file (e.g. post.controller.ts): a node's
// Content() must always be read against its own tree's source bytes —
// reading it against a different file's bytes produces garbage, silently
// (wrong byte offsets into unrelated text), not a crash.
type sourceNode struct {
	node *sitter.Node
	src  []byte
}

// compositeDecorator is a project-defined decorator factory recognized as
// matching the bounded shape docs/decisions/0006-composite-decorator-resolution.md
// resolves: a single return path calling applyDecorators(...), containing
// nested UseGuards()/Roles() calls.
type compositeDecorator struct {
	params     []string       // parameter names, in declaration order
	innerCalls []*sitter.Node // the UseGuards(...)/Roles(...) call_expression nodes inside applyDecorators' arguments
	src        []byte         // the DEFINING file's source — innerCalls (until substituted) must be read against this
}

// collectCompositeDecorators finds every function or arrow-function
// definition in the file matching that shape, keyed by name. A definition
// that doesn't match — more than one return path, a destructured
// parameter, a return expression that isn't a direct applyDecorators(...)
// call — is simply not registered; call sites using it fall back to
// today's behavior (invisible, Low-confidence flag) per the ADR's
// explicit non-goals, rather than being resolved incorrectly.
func collectCompositeDecorators(root *sitter.Node, src []byte) map[string]compositeDecorator {
	out := make(map[string]compositeDecorator)
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "function_declaration":
			if name, cd, ok := parseCompositeFunctionDeclaration(n, src); ok {
				out[name] = cd
			}
		case "variable_declarator":
			if name, cd, ok := parseCompositeArrowVariable(n, src); ok {
				out[name] = cd
			}
		}
		for _, c := range namedChildren(n) {
			walk(c)
		}
	}
	walk(root)
	return out
}

func parseCompositeFunctionDeclaration(n *sitter.Node, src []byte) (string, compositeDecorator, bool) {
	nameNode := n.ChildByFieldName("name")
	paramsNode := n.ChildByFieldName("parameters")
	bodyNode := n.ChildByFieldName("body")
	if nameNode == nil || paramsNode == nil || bodyNode == nil {
		return "", compositeDecorator{}, false
	}
	return buildComposite(nameNode.Content(src), paramsNode, bodyNode, src)
}

func parseCompositeArrowVariable(n *sitter.Node, src []byte) (string, compositeDecorator, bool) {
	nameNode := n.ChildByFieldName("name")
	valueNode := n.ChildByFieldName("value")
	if nameNode == nil || valueNode == nil || valueNode.Type() != "arrow_function" {
		return "", compositeDecorator{}, false
	}
	paramsNode := valueNode.ChildByFieldName("parameters")
	bodyNode := valueNode.ChildByFieldName("body")
	if paramsNode == nil || bodyNode == nil || paramsNode.Type() != "formal_parameters" {
		return "", compositeDecorator{}, false
	}
	return buildComposite(nameNode.Content(src), paramsNode, bodyNode, src)
}

func buildComposite(name string, paramsNode, bodyNode *sitter.Node, src []byte) (string, compositeDecorator, bool) {
	params, ok := simpleParameterNames(paramsNode, src)
	if !ok {
		return "", compositeDecorator{}, false
	}
	applyCall, ok := singleApplyDecoratorsCall(bodyNode, src)
	if !ok {
		return "", compositeDecorator{}, false
	}
	return name, compositeDecorator{params: params, innerCalls: innerAuthCalls(applyCall, src), src: src}, true
}

// simpleParameterNames returns each parameter's plain identifier name, or
// ok=false if any parameter uses a destructuring pattern this resolver
// doesn't attempt to follow.
func simpleParameterNames(paramsNode *sitter.Node, src []byte) ([]string, bool) {
	var names []string
	for _, p := range namedChildren(paramsNode) {
		switch p.Type() {
		case "required_parameter", "optional_parameter":
			pattern := p.ChildByFieldName("pattern")
			if pattern == nil || pattern.Type() != "identifier" {
				return nil, false
			}
			names = append(names, pattern.Content(src))
		default:
			return nil, false
		}
	}
	return names, true
}

// singleApplyDecoratorsCall requires exactly one return path — searched
// recursively, so an `if`-guarded second return correctly disqualifies a
// composite rather than being missed — whose expression is a direct call
// to applyDecorators(...).
func singleApplyDecoratorsCall(bodyNode *sitter.Node, src []byte) (*sitter.Node, bool) {
	var expr *sitter.Node
	if bodyNode.Type() == "statement_block" {
		returns := findReturnStatements(bodyNode)
		if len(returns) != 1 || returns[0].NamedChildCount() == 0 {
			return nil, false
		}
		expr = returns[0].NamedChild(0)
	} else {
		expr = bodyNode // expression-bodied arrow: the body *is* the expression
	}

	if expr == nil || expr.Type() != "call_expression" {
		return nil, false
	}
	fn := expr.ChildByFieldName("function")
	if fn == nil || fn.Type() != "identifier" || fn.Content(src) != "applyDecorators" {
		return nil, false
	}
	return expr, true
}

func findReturnStatements(n *sitter.Node) []*sitter.Node {
	var out []*sitter.Node
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "return_statement" {
			out = append(out, n)
		}
		for _, c := range namedChildren(n) {
			walk(c)
		}
	}
	walk(n)
	return out
}

// innerAuthCalls picks out, from applyDecorators(...)'s arguments, only
// the nested calls literally named UseGuards or Roles — the same two
// names recognized at a literal decorator call site. Everything else
// (ApiBearerAuth(), UseInterceptors(...), ...) is irrelevant to
// authorization and ignored, same as today.
func innerAuthCalls(applyCall *sitter.Node, src []byte) []*sitter.Node {
	var out []*sitter.Node
	for _, arg := range argumentNodes(applyCall.ChildByFieldName("arguments")) {
		if arg.Type() != "call_expression" {
			continue
		}
		fn := arg.ChildByFieldName("function")
		if fn == nil || fn.Type() != "identifier" {
			continue
		}
		if name := fn.Content(src); name == "UseGuards" || name == "Roles" {
			out = append(out, arg)
		}
	}
	return out
}

// resolveCompositeArgs expands a decorator call site matching a
// registered composite decorator into the resolved (parameter-substituted,
// array-unpacked) argument nodes of each UseGuards()/Roles() call the
// composite contains — each tagged with the source buffer it must be read
// against (see sourceNode). hasRoles distinguishes "the composite has a
// Roles() call with these (possibly zero) role args" from "no Roles()
// call at all" — @Auth([]) must still produce a Roles application with
// zero roles, not no application, so mutating-endpoint-without-access-control
// correctly recognizes the endpoint as guarded via UseGuards even when
// empty-role's FromComposite exclusion (docs/decisions/0006) later skips
// it. ok is false if call.Name isn't a registered composite.
//
// Multiple UseGuards()/Roles() calls inside one composite (unusual, but
// not disallowed) are merged into one flat list each — this package's
// only consumers of a "roles" application already treat all of a single
// application's role args as one set, so merging is behavior-preserving,
// not a simplification that loses information callers need.
func resolveCompositeArgs(call decoratorCall, callSrc []byte, composites map[string]compositeDecorator) (guardArgs, roleArgs []sourceNode, hasRoles, ok bool) {
	comp, found := composites[call.Name]
	if !found {
		return nil, nil, false, false
	}

	callArgNodes := argumentNodes(call.Args)
	substitution := make(map[string]sourceNode, len(comp.params))
	for i, p := range comp.params {
		if i < len(callArgNodes) {
			substitution[p] = sourceNode{node: callArgNodes[i], src: callSrc}
		}
	}

	for _, inner := range comp.innerCalls {
		name := inner.ChildByFieldName("function").Content(comp.src) // validated to be UseGuards or Roles by innerAuthCalls
		resolved := resolveAndUnpack(argumentNodes(inner.ChildByFieldName("arguments")), comp.src, substitution)

		switch name {
		case "UseGuards":
			guardArgs = append(guardArgs, resolved...)
		case "Roles":
			hasRoles = true
			roleArgs = append(roleArgs, resolved...)
		}
	}
	return guardArgs, roleArgs, hasRoles, true
}

// resolveAndUnpack substitutes each argument that's a bare identifier
// matching a composite parameter, then unpacks any resulting array
// literal into its individual elements (matching the common
// Roles(...roles: T[]) rest-parameter convention, where passing an array
// achieves the same effect as passing each element individually).
//
// args are read against defSrc (the composite definition's own source)
// until substituted; a substituted node carries whatever source its
// substitution.sourceNode says (the call site's), and any array unpacked
// from it inherits that same source, since its elements belong to
// whichever tree the array literal itself came from.
//
// A bare identifier that ISN'T a composite parameter (e.g. RolesGuard, a
// plain imported guard class referenced directly in the composite's
// body) is kept as-is, not dropped — it's a static reference, exactly
// like any identifier argument at a literal @UseGuards(RolesGuard) call
// site, not a value this resolver needs to substitute anything into.
//
// Only a genuinely transformed or spread argument (e.g. Roles(...roles),
// Roles(roles.map(...))) is left unresolved, per the ADR's explicit
// non-goals — and even then, it degrades to an inert, unmatched
// reference (internal/lint's rules never treat an unresolved reference as
// evidence of anything), not a wrong claim.
func resolveAndUnpack(args []*sitter.Node, defSrc []byte, substitution map[string]sourceNode) []sourceNode {
	var out []sourceNode
	for _, a := range args {
		if a.Type() == "spread_element" {
			continue
		}

		resolved := sourceNode{node: a, src: defSrc}
		if a.Type() == "identifier" {
			if sub, ok := substitution[a.Content(defSrc)]; ok {
				resolved = sub
			}
		}

		if resolved.node.Type() == "array" {
			for _, elem := range namedChildren(resolved.node) {
				out = append(out, sourceNode{node: elem, src: resolved.src})
			}
			continue
		}
		out = append(out, resolved)
	}
	return out
}
