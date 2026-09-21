package spring

import (
	"sort"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chebilax/sphinxor/internal/model"
)

// chainTerminalKind classifies what one SecurityFilterChain rule's
// terminal call establishes, per docs/decisions/0012-securityfilterchain-effective-policy.md §1.
type chainTerminalKind int

const (
	// chainUnrecognized covers .access(AuthorizationManager) and anything
	// else outside the recognized set — the rule is real and, per ADR
	// 0018, still governs first-match-wins evaluation order; extraction
	// simply can't say what it grants.
	chainUnrecognized chainTerminalKind = iota
	chainRoles
	chainAuthenticated
	// chainNoRequirement is .permitAll()/.denyAll() — a real, recognized
	// terminal that contributes no requirement (ADR 0012 §1: "permitAll()
	// contributes no requirement... this layer has nothing to add").
	chainNoRequirement
)

// chainRoleFuncs are .hasRole/.hasAnyRole/.hasAuthority/.hasAnyAuthority —
// real Java method calls here (not SpEL: these are direct
// HttpSecurity/AuthorizeHttpRequestsConfigurer method invocations with
// literal string arguments), despite sharing names with @PreAuthorize's
// SpEL predicates.
var chainRoleFuncs = map[string]bool{
	"hasRole":         true,
	"hasAnyRole":      true,
	"hasAuthority":    true,
	"hasAnyAuthority": true,
}

// chainHTTPMethodNames maps the identifier text Spring code uses for an
// HttpMethod constant (bare, via static import, or as HttpMethod.X's
// field) to the model.HTTPMethod it names — confirmed against both real
// vendored shapes: Pharmacy uses HttpMethod.DELETE (field_access);
// blog-api uses a bare `DELETE` (static import, plain identifier).
var chainHTTPMethodNames = map[string]model.HTTPMethod{
	"GET":    model.MethodGet,
	"POST":   model.MethodPost,
	"PUT":    model.MethodPut,
	"PATCH":  model.MethodPatch,
	"DELETE": model.MethodDelete,
}

// filterChainRule is one recognized-or-not authorizeHttpRequests rule, in
// source order.
type filterChainRule struct {
	method   *model.HTTPMethod // nil = not method-scoped, applies to every HTTP method
	patterns []string          // non-empty unless anyRequest or matcherUnreadable
	kind     chainTerminalKind
	roles    []string // populated only for chainRoles
	file     string
	line     int

	// anyRequest marks an explicit .anyRequest() rule: it genuinely
	// matches every path. Before docs/decisions/0020-unanalyzable-is-unknown-not-absent.md
	// this was encoded as `patterns == nil`, which a requestMatchers(...)
	// call with no extractable pattern produced too — so a matcher that
	// couldn't be read silently became one that matched everything.
	anyRequest bool

	// matcherUnreadable marks a requestMatchers(...) whose arguments
	// yielded no pattern (a regex matcher, a custom RequestMatcher bean).
	// ADR 0020 §1: unreadable is *unknown*, never universal and never
	// absent — the rule may match any path, so it stops evaluation like
	// any other opaque rule (ADR 0018) and contributes nothing.
	matcherUnreadable bool
}

// findSecurityFilterChainRules locates the project's SecurityFilterChain
// configuration and returns its rules in source (evaluation) order, plus
// the file they were found in. ok is false if there isn't exactly one
// @Bean method returning SecurityFilterChain project-wide.
//
// Exactly one, not "at least one": docs/decisions/0012-securityfilterchain-effective-policy.md §1
// explicitly excludes multiple securityMatcher-scoped SecurityFilterChain
// beans (chain selection by @Order/securityMatcher is its own,
// unaddressed problem) — extracting rules from one chain while silently
// ignoring that a second chain might apply instead (or first) would risk
// a confidently wrong answer, the same category of danger ADR 0018 closed
// for a single chain's own internal evaluation order. Neither vendored
// fixture has more than one SecurityFilterChain bean, so this exclusion
// isn't itself exercised by real-fixture tests, but it's what ADR 0012 §1
// already scoped out, implemented rather than silently assumed away.
func findSecurityFilterChainRules(files []parsedFile) (rules []filterChainRule, file string, ok bool) {
	var lambdas []*sitter.Node
	var srcs [][]byte
	var relPaths []string
	for _, f := range files {
		for _, lam := range securityFilterChainLambdas(f.tree.RootNode(), f.src) {
			lambdas = append(lambdas, lam)
			srcs = append(srcs, f.src)
			relPaths = append(relPaths, f.relPath)
		}
	}
	if len(lambdas) != 1 {
		return nil, "", false
	}

	rules = collectChainRules(lambdas[0], srcs[0])
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].line < rules[j].line })
	return rules, relPaths[0], true
}

// securityFilterChainLambdas finds every authorizeHttpRequests(lambda)
// call inside a @Bean method returning SecurityFilterChain, returning
// each call's lambda argument.
func securityFilterChainLambdas(root *sitter.Node, src []byte) []*sitter.Node {
	var out []*sitter.Node
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "method_declaration" && isSecurityFilterChainBean(n, src) {
			for _, call := range findMethodInvocations(n, src, "authorizeHttpRequests") {
				args := call.ChildByFieldName("arguments")
				if lam := soleLambdaArg(args); lam != nil {
					out = append(out, lam)
				}
			}
		}
		for _, c := range namedChildren(n) {
			walk(c)
		}
	}
	walk(root)
	return out
}

func isSecurityFilterChainBean(methodDecl *sitter.Node, src []byte) bool {
	return isChainBeanOfType(methodDecl, src, "SecurityFilterChain")
}

// isReactiveChainBean matches WebFlux's SecurityWebFilterChain. Its rules
// (ServerHttpSecurity / pathMatchers) are deliberately NOT parsed — that
// is new framework coverage and belongs in its own decision
// (docs/decisions/0020-unanalyzable-is-unknown-not-absent.md §3). This
// exists only so a reactive URL layer is detected as *present*, and
// therefore recorded as unknown rather than mistaken for absent, which
// is what stops a reactive project from being told a method-layer-only
// answer is the complete picture.
func isReactiveChainBean(methodDecl *sitter.Node, src []byte) bool {
	return isChainBeanOfType(methodDecl, src, "SecurityWebFilterChain")
}

func isChainBeanOfType(methodDecl *sitter.Node, src []byte, typeName string) bool {
	anns := annotationsOf(methodDecl, src)
	if _, ok := findAnnotation(anns, "Bean"); !ok {
		return false
	}
	typeNode := methodDecl.ChildByFieldName("type")
	return typeNode != nil && typeNode.Type() == "type_identifier" && typeNode.Content(src) == typeName
}

// countChainBeans reports how many servlet and reactive chain beans exist
// project-wide, regardless of whether their rules can be parsed. Presence
// and parseability are separate questions (ADR 0020 §2).
// urlLayerForms counts each recognized way of declaring a URL-authorization
// layer. Every one is detected by a *declaration* — a @Bean method's return
// type, or a class's extends clause — never by a mention of the type name,
// per docs/decisions/0027-unannounced-url-layers.md §1.
//
// That distinction is not theoretical. A token-presence check would have
// invented a reactive URL layer for apache/shenyu, where
// SecurityWebFilterChain appears only in an import and a
// @ConditionalOnClass({… SecurityWebFilterChain.class …}) with no @Bean
// returning one.
type urlLayerForms struct {
	// servlet is Spring Security's modern @Bean SecurityFilterChain.
	servlet int
	// reactive is WebFlux's SecurityWebFilterChain (ADR 0020 §3).
	reactive int
	// legacyAdapter is a class extending WebSecurityConfigurerAdapter,
	// Spring Security's own pre-5.7 configuration base class — in scope
	// per ADR 0011, simply an older API (ADR 0027 §1).
	legacyAdapter int
	// shiro is a @Bean returning Apache Shiro's ShiroFilterFactoryBean.
	// Shiro is not Spring Security and nothing here parses it; recording
	// that the layer exists is not interpreting it (ADR 0027, Context).
	shiro int
}

func (f urlLayerForms) any() bool {
	return f.servlet > 0 || f.reactive > 0 || f.legacyAdapter > 0 || f.shiro > 0
}

func countChainBeans(files []parsedFile) urlLayerForms {
	var out urlLayerForms
	for _, f := range files {
		var walk func(n *sitter.Node)
		walk = func(n *sitter.Node) {
			switch n.Type() {
			case "method_declaration":
				switch {
				case isSecurityFilterChainBean(n, f.src):
					out.servlet++
				case isReactiveChainBean(n, f.src):
					out.reactive++
				case isChainBeanOfType(n, f.src, "ShiroFilterFactoryBean"):
					out.shiro++
				}
			case "class_declaration":
				if extendsType(n, f.src, "WebSecurityConfigurerAdapter") {
					out.legacyAdapter++
				}
			}
			for _, c := range namedChildren(n) {
				walk(c)
			}
		}
		walk(f.tree.RootNode())
	}
	return out
}

// extendsType reports whether a class declaration's superclass is name.
// tree-sitter-java exposes the extends clause as a `superclass` child
// holding a type node, so this matches a real declaration rather than any
// occurrence of the identifier (ADR 0027 §1).
func extendsType(classDecl *sitter.Node, src []byte, name string) bool {
	super := findChildByType(classDecl, "superclass")
	if super == nil {
		return false
	}
	for _, c := range namedChildren(super) {
		if simpleNameOfClassLiteral(c.Content(src)) == name {
			return true
		}
	}
	return false
}

func soleLambdaArg(args *sitter.Node) *sitter.Node {
	if args == nil || args.NamedChildCount() != 1 {
		return nil
	}
	if first := args.NamedChild(0); first.Type() == "lambda_expression" {
		return first
	}
	return nil
}

// findMethodInvocations returns every method_invocation named name
// anywhere under n, searched recursively (the target call can be nested
// arbitrarily deep inside a fluent chain).
func findMethodInvocations(n *sitter.Node, src []byte, name string) []*sitter.Node {
	var out []*sitter.Node
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "method_invocation" {
			if nm := n.ChildByFieldName("name"); nm != nil && nm.Content(src) == name {
				out = append(out, n)
			}
		}
		for _, c := range namedChildren(n) {
			walk(c)
		}
	}
	walk(n)
	return out
}

// collectChainRules walks lambdaBody for every `.requestMatchers(...).X()`
// or `.anyRequest().X()` pair — a method_invocation named X whose object
// is itself a method_invocation named "requestMatchers" or "anyRequest".
// Source order is recovered by sorting on position afterward
// (findSecurityFilterChainRules), not by the traversal order here: Java's
// fluent chain nests as object fields (the outermost node in the tree is
// the *last* call written), so a naive pre-order walk would visit rules
// in reverse.
func collectChainRules(lambdaBody *sitter.Node, src []byte) []filterChainRule {
	var out []filterChainRule
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "method_invocation" {
			if rule, ok := chainRuleFromTerminal(n, src); ok {
				out = append(out, rule)
			}
		}
		for _, c := range namedChildren(n) {
			walk(c)
		}
	}
	walk(lambdaBody)
	return out
}

func chainRuleFromTerminal(terminal *sitter.Node, src []byte) (filterChainRule, bool) {
	name := terminal.ChildByFieldName("name")
	object := terminal.ChildByFieldName("object")
	if name == nil || object == nil || object.Type() != "method_invocation" {
		return filterChainRule{}, false
	}
	matcherName := object.ChildByFieldName("name")
	if matcherName == nil {
		return filterChainRule{}, false
	}

	var method *model.HTTPMethod
	var patterns []string
	var anyRequest, matcherUnreadable bool
	switch matcherName.Content(src) {
	case "anyRequest":
		anyRequest = true
	case "requestMatchers":
		method, patterns = requestMatchersArgs(object.ChildByFieldName("arguments"), src)
		// No pattern could be read out of a matcher that is definitely
		// present. ADR 0020 §1: record that as unknown, not as universal.
		matcherUnreadable = len(patterns) == 0
	default:
		return filterChainRule{}, false
	}

	// name.StartPoint(), not terminal.StartPoint(): a method_invocation
	// node's own start position is inherited from its leftmost descendant
	// (object, the first field) — in a right-nested fluent chain, every
	// node shares the chain's overall start position. Only the terminal's
	// own method-name token has a position that actually varies per rule,
	// confirmed against real Pharmacy source (every rule reported the same
	// line using terminal.StartPoint() before this fix).
	line := int(name.StartPoint().Row) + 1

	// An unreadable matcher makes the whole rule opaque regardless of how
	// recognizable its terminal is: knowing it grants ADMIN is useless
	// when we can't tell which paths it grants ADMIN *on*. Reading the
	// terminal anyway and attaching those roles is exactly the defect ADR
	// 0020 §1 corrects. chainUnrecognized already carries the right
	// semantics from ADR 0018 — opaque, still governs evaluation order.
	base := filterChainRule{
		method:            method,
		patterns:          patterns,
		anyRequest:        anyRequest,
		matcherUnreadable: matcherUnreadable,
		line:              line,
	}
	if matcherUnreadable {
		base.kind = chainUnrecognized
		return base, true
	}

	switch tname := name.Content(src); {
	case chainRoleFuncs[tname]:
		roles := stringArgValues(terminal.ChildByFieldName("arguments"), src)
		if len(roles) == 0 {
			return filterChainRule{}, false
		}
		base.kind, base.roles = chainRoles, roles
		return base, true
	case tname == "authenticated" && argCount(terminal) == 0:
		base.kind = chainAuthenticated
		return base, true
	case tname == "permitAll" && argCount(terminal) == 0, tname == "denyAll" && argCount(terminal) == 0:
		base.kind = chainNoRequirement
		return base, true
	default:
		// Readable matcher (requestMatchers/anyRequest), unrecognized
		// terminal (.access(...), or anything else) — still a real rule
		// that governs evaluation order, per ADR 0018. kind stays its
		// zero value, chainUnrecognized.
		return base, true
	}
}

// requestMatchersArgs reads .requestMatchers(...)'s arguments: an
// optional leading HttpMethod (HttpMethod.X field access, or a bare
// statically-imported identifier — both confirmed against real vendored
// source), followed by one or more string-literal patterns.
func requestMatchersArgs(args *sitter.Node, src []byte) (*model.HTTPMethod, []string) {
	if args == nil {
		return nil, nil
	}
	nodes := namedChildren(args)
	if len(nodes) == 0 {
		return nil, nil
	}

	var method *model.HTTPMethod
	start := 0
	if m, ok := httpMethodOf(nodes[0], src); ok {
		method = &m
		start = 1
	}

	var patterns []string
	for _, n := range nodes[start:] {
		if v, ok := stringLiteralValue(n, src); ok {
			patterns = append(patterns, v)
			continue
		}
		// A matcher-wrapper call carrying the pattern one level deeper.
		if m, ps, ok := matcherWrapperArgs(n, src); ok {
			if m != nil && method == nil {
				method = m
			}
			patterns = append(patterns, ps...)
		}
	}
	return method, patterns
}

// antPatternWrappers are the explicit matcher constructors whose first
// string argument is an ordinary Ant pattern. Recognizing them is the
// other half of ADR 0020 §1: `requestMatchers(antMatcher("/admin/**"))`
// is idiomatic Spring Security 6 — the form its own docs recommend when
// mixing matcher types — so without this, the corrected "unreadable is
// unknown" rule would turn a very common configuration into a
// project-wide unknown for no reason. The pattern is right there and
// perfectly readable; it just sits one call deeper.
//
// Deliberately excludes regexMatcher and any custom RequestMatcher bean:
// those are genuinely unreadable and must stay unknown.
var antPatternWrappers = map[string]bool{
	"antMatcher": true,
}

// matcherWrapperArgs reads a recognized matcher-wrapper invocation,
// e.g. antMatcher("/admin/**") or antMatcher(HttpMethod.GET, "/admin/**"),
// whether called bare (static import) or qualified
// (AntPathRequestMatcher.antMatcher(...)).
func matcherWrapperArgs(n *sitter.Node, src []byte) (*model.HTTPMethod, []string, bool) {
	if n.Type() != "method_invocation" {
		return nil, nil, false
	}
	name := n.ChildByFieldName("name")
	if name == nil || !antPatternWrappers[name.Content(src)] {
		return nil, nil, false
	}
	method, patterns := requestMatchersArgs(n.ChildByFieldName("arguments"), src)
	if len(patterns) == 0 {
		return nil, nil, false
	}
	return method, patterns, true
}

func httpMethodOf(n *sitter.Node, src []byte) (model.HTTPMethod, bool) {
	switch n.Type() {
	case "field_access":
		field := n.ChildByFieldName("field")
		if field == nil {
			return "", false
		}
		m, ok := chainHTTPMethodNames[field.Content(src)]
		return m, ok
	case "identifier":
		m, ok := chainHTTPMethodNames[n.Content(src)]
		return m, ok
	default:
		return "", false
	}
}

func stringArgValues(args *sitter.Node, src []byte) []string {
	var out []string
	for _, n := range namedChildren(args) {
		if v, ok := stringLiteralValue(n, src); ok {
			out = append(out, v)
		}
	}
	return out
}

func argCount(call *sitter.Node) int {
	args := call.ChildByFieldName("arguments")
	if args == nil {
		return 0
	}
	return int(args.NamedChildCount())
}

// matchesRule reports whether rule applies to e: method-scoped rules only
// match the same HTTP method; patterns == nil (.anyRequest()) matches
// every path; otherwise at least one pattern must match via
// matchesAntPattern (antpattern.go).
func matchesRule(rule filterChainRule, e model.Endpoint) bool {
	// A verb-scoped rule is *considered* against an ANY endpoint rather
	// than skipped — firstMatch neutralises it (ADR 0028 §2). Skipping
	// here would let a later, more permissive rule apply to an endpoint
	// an earlier one really does govern for one of its verbs.
	if rule.method != nil && e.HTTPMethod != model.MethodAny && *rule.method != e.HTTPMethod {
		return false
	}
	if rule.anyRequest {
		return true
	}
	// An unreadable matcher might match any path, so it is treated as
	// matching: evaluation stops here and this endpoint's URL layer is
	// unresolved (ADR 0020 §1). Treating it as *not* matching would let
	// evaluation continue to a later, possibly more permissive rule —
	// re-entering ADR 0012 §1's original error by a different door.
	if rule.matcherUnreadable {
		return true
	}
	for _, p := range rule.patterns {
		if matchesAntPattern(p, e.Path) {
			return true
		}
	}
	return false
}

// firstMatch returns the first rule in rules (source order) that matches
// e, per docs/decisions/0018-unrecognized-rule-stops-evaluation.md: the
// first *matching* rule wins regardless of recognition — an unrecognized
// rule that matches stops evaluation exactly as a recognized one would,
// it just can't say what it grants. A rule is only skipped when it
// doesn't match e at all.
func firstMatch(rules []filterChainRule, e model.Endpoint) (filterChainRule, bool) {
	for _, r := range rules {
		if !matchesRule(r, e) {
			continue
		}
		// A verb-scoped rule covers one of the eight verbs an ANY
		// endpoint answers, so it neither clearly applies nor clearly
		// does not (ADR 0028 §2). Treating it as applying would grant
		// its roles across seven verbs it does not cover; treating it as
		// not applying would drop a rule that really does govern one.
		//
		// So it matches — stopping evaluation, so no later and more
		// permissive rule is reached — but as chainUnrecognized, which
		// is exactly ADR 0018's existing "opaque, may match, contributes
		// nothing" outcome. No new state was needed.
		if r.method != nil && e.HTTPMethod == model.MethodAny {
			r.kind = chainUnrecognized
			r.roles = nil
		}
		return r, true
	}
	return filterChainRule{}, false
}

// applySecurityFilterChain evaluates rules against every endpoint in
// b.model.Endpoints and attaches the URL layer's GuardApplication/
// RoleReferences/authCandidate, mirroring guards.go's applyGuards for the
// method layer. File/Line point at the rule's own location (the
// SecurityConfig class), not the endpoint's controller — ADR 0012 §1: an
// honest pointer to where the evidence actually lives.
func (b *builder) applySecurityFilterChain(rules []filterChainRule, file string, roleByName map[string]model.ID) {
	for _, e := range b.model.Endpoints {
		rule, matched := firstMatch(rules, e)
		if !matched {
			continue // no rule matches at all: URL layer contributes nothing, same as no SecurityFilterChain
		}
		switch rule.kind {
		case chainUnrecognized, chainNoRequirement:
			// Nothing contributed — unresolved (ADR 0018) or an explicit
			// non-requirement (permitAll/denyAll, ADR 0012 §1).
		case chainAuthenticated:
			b.authCandidates = append(b.authCandidates, authCandidate{
				EndpointID: e.ID,
				File:       file,
				Line:       rule.line,
				AppliedAt:  model.ScopeRequestMatcher,
			})
		case chainRoles:
			appID := b.nextIDFor("guardapp")
			b.model.GuardApplications = append(b.model.GuardApplications, model.GuardApplication{
				ID:            appID,
				EndpointID:    e.ID,
				GuardName:     "requestMatcher",
				AppliedAt:     model.ScopeRequestMatcher,
				File:          file,
				Line:          rule.line,
				DeclaresRoles: true,
			})
			for _, roleLit := range rule.roles {
				var declID *model.ID
				if id, ok := roleByName[roleLit]; ok {
					idCopy := id
					declID = &idCopy
				}
				b.model.RoleReferences = append(b.model.RoleReferences, model.RoleReference{
					ID:                 b.nextIDFor("roleref"),
					GuardApplicationID: appID,
					RoleDeclarationID:  declID,
					RawLiteral:         roleLit,
					File:               file,
					Line:               rule.line,
				})
			}
		}
	}
}
