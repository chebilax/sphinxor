package cli

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/chebilax/sphinxor/internal/extract"
	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
)

// resolveFramework decides which extractor to run against dir, per
// docs/decisions/0019-cli-framework-selection.md §1.
//
// The rule that makes auto-detection safe is that it never resolves its
// own uncertainty: exactly one detected framework is used, and both "none"
// and "more than one" are hard errors asking for --framework.
// Auto-detection isn't the danger; auto-detection that quietly picks is.
//
// The returned string says how the choice was made, so the run can state
// it in its output — a silently wrong analysis becomes a visibly wrong one.
func resolveFramework(dir, override string) (extract.Framework, string, error) {
	if override != "" {
		f, err := extract.ParseFramework(override)
		if err != nil {
			return "", "", err
		}
		return f, "--framework", nil
	}

	detected, err := extract.Detect(dir)
	if err != nil {
		return "", "", fmt.Errorf("detecting framework in %s: %w", dir, err)
	}

	switch len(detected) {
	case 1:
		return detected[0], "detected", nil
	case 0:
		// ADR 0042 §5: a wholly Kotlin Spring project is not an
		// unrecognized project — it is one Sphinxor cannot parse, and the
		// error should say so instead of suggesting --framework.
		if k, err := extract.Kotlin(dir); err == nil && k.ControllerFiles+k.URLSecurityFiles+k.MethodEnablerFiles > 0 {
			return "", "", fmt.Errorf(
				"no supported framework detected in %s: it has %d Kotlin source file(s), %d declaring Spring "+
					"controllers and %d declaring Spring Security configuration, but Kotlin is not parsed "+
					"(docs/decisions/0011-spring-second-framework.md §1), so there is nothing here Sphinxor can analyze",
				dir, k.Files, k.ControllerFiles, k.URLSecurityFiles+k.MethodEnablerFiles)
		}
		return "", "", fmt.Errorf(
			"no supported framework detected in %s (looked for: %s)\n"+
				"If this is a supported project, pass --framework explicitly",
			dir, extract.Join(extract.All))
	default:
		return "", "", fmt.Errorf(
			"more than one framework detected in %s: %s\n"+
				"Pass --framework to say which one to analyze",
			dir, extract.Join(detected))
	}
}

// analyzeDirectory resolves the framework for dir, extracts the model, and
// runs the full rule set against it, including allowlist matching and
// stale-marker findings — the shared pipeline behind `sphinxor lint`,
// `sphinxor diff`, and `sphinxor export`.
//
// It also enforces ADR 0019 §2's distinction between "couldn't look" and
// "looked, found nothing": extracting zero source files is an error, not
// an empty report, because an empty report reads as a clean bill of health
// for a project the tool never actually examined.
//
// Notices (which framework was chosen; parsed files but recognized no
// endpoints) go to w, which callers point at stderr so they never
// contaminate a --format json report on stdout.
func analyzeDirectory(w io.Writer, dir, override string) (*model.Model, []model.Finding, error) {
	m, findings, _, err := analyzeSnapshot(w, dir, override)
	return m, findings, err
}

// analyzeSnapshot is analyzeDirectory plus the set of endpoints a
// sphinxor-allow marker exempts.
//
// It exists for `sphinxor diff` alone (ADR 0036 §4). That gate has to know
// whether an endpoint made public was made public DELIBERATELY, and the
// allowlist set is the only complete source of that: a GET endpoint that
// loses its guard and gains a marker produces no finding at all, so the
// Allowlisted flag on findings has nothing to ride on. Measured, not
// assumed — see Snapshot.AllowlistedEndpoints.
//
// Kept as a separate entry point rather than widening analyzeDirectory's
// signature, so the dozen callers that do not need the set are not made
// to discard it at every call.
func analyzeSnapshot(w io.Writer, dir, override string) (*model.Model, []model.Finding, map[model.ID]bool, error) {
	framework, how, err := resolveFramework(dir, override)
	if err != nil {
		return nil, nil, nil, err
	}

	sourceFiles, err := extract.SourceFileCount(dir, framework)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("scanning %s: %w", dir, err)
	}
	if sourceFiles == 0 {
		return nil, nil, nil, fmt.Errorf(
			"no %s source files found under %s — nothing was analyzed.\n"+
				"Check the path, or pass --framework if the framework was misidentified",
			framework, dir)
	}

	fmt.Fprintf(w, "Analyzing %s as %s (%s), %d source file(s).\n", dir, framework, how, sourceFiles)

	m, allow, err := extract.Run(dir, framework)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("extracting model at %s: %w", dir, err)
	}

	// Parsed real files but recognized no routes. Unlike the zero-files
	// case this is not an error: a library package or a monorepo subpath
	// legitimately has no controllers, and failing those runs is the
	// noise that gets a CI tool switched off. It is still said out loud,
	// because the other cause is extraction not recognizing this
	// project's route shapes — which an empty matrix alone would present
	// as a clean run.
	if len(m.Endpoints) == 0 {
		fmt.Fprintf(w,
			"warning: parsed %d %s source file(s) but recognized no endpoints.\n"+
				"         If this project does define routes, their shape may not be recognized —\n"+
				"         see docs/limitations.md. This is not treated as a failure.\n",
			sourceFiles, framework)
	}

	// Project-level caveats (ADR 0020 §2/§4). These don't change a finding
	// or a grant; they change what the reader should believe the output
	// means. They go to w (stderr) for the same reason the framework
	// notice does: stdout stays clean for --format json.
	for _, warning := range projectWarnings(m) {
		fmt.Fprintf(w, "warning: %s\n", warning)
	}

	findings := lint.Run(m, lint.DefaultRules(), allow.AllowlistedEndpoints)
	findings = append(findings, allow.StaleMarkers...)

	return m, findings, allow.AllowlistedEndpoints, nil
}

// projectWarnings returns the project-level caveats that change how the
// whole report should be read, per docs/decisions/0020-unanalyzable-is-unknown-not-absent.md
// §2 and §4.
//
// None of these changes a finding or a grant. They exist because a result
// can be correct as far as it goes and still be presented with more
// confidence than the analysis earned — and an audit tool that does that
// is the failure this project is built to avoid.
// guardDifferingCollisions renders the route collisions worth telling the
// user about — those whose sides do not carry the same guards (ADR 0020
// Amendment 2 §8).
func guardDifferingCollisions(m *model.Model) []string {
	var out []string
	for _, c := range m.RouteCollisions {
		if !c.GuardsDiffer {
			continue
		}
		out = append(out, string(c.HTTPMethod)+" "+c.Path+" ("+strings.Join(c.Controllers, ", ")+")")
	}
	return out
}

func projectWarnings(m *model.Model) []string {
	var out []string

	// §2: a URL layer exists and could not be read, so the method layer
	// is not the whole story for any endpoint in this project.
	if m.URLLayer.Unknown() {
		out = append(out, "this project's URL-layer authorization could not be analyzed ("+m.URLLayer.Reason+").\n"+
			"         Roles shown below come from the method layer alone and may be BROADER than what the\n"+
			"         application actually enforces. `sphinxor export cerbos` omits these endpoints entirely.")
	}

	// §4: method-security annotations that may never be switched on. The
	// analysis deliberately does not downgrade them (ADR 0015: absence of
	// the enabling annotation is no evidence either way, since it can live
	// in a parent module or unparsed Kotlin) — but silence here reads as
	// confirmation, and the consequence if they really are inert is that
	// every role shown for them is imaginary.
	if !m.MethodSecurity.Found && hasMethodSecurityAnnotations(m) && m.Kotlin.MethodEnablerFiles > 0 {
		// ADR 0042 §4: the enabler exists, in Kotlin, and is not read. The
		// truthful statement is that whether it enables these is unknown —
		// not that they are unprotected.
		out = append(out, "method-security annotations were found, and an enabling annotation (@EnableMethodSecurity or a\n"+
			"         sibling) is declared in Kotlin, which is not parsed. Whether it enables these annotations is\n"+
			"         unknown: the endpoints they appear to protect may or may not be protected.")
	} else if !m.MethodSecurity.Found && hasMethodSecurityAnnotations(m) {
		out = append(out, "method-security annotations were found, but no @EnableMethodSecurity /\n"+
			"         @EnableGlobalMethodSecurity / @EnableReactiveMethodSecurity was located in the analyzed\n"+
			"         source. If it isn't enabled elsewhere (a parent module, Kotlin config), those annotations\n"+
			"         are inert at runtime and the endpoints they appear to protect are NOT protected.")
	}

	// ADR 0030 §4: @PreFilter/@PostFilter. Announced so the reader knows
	// the annotations are there, and worded so nobody reads them as
	// access control — neither ever denies a call, so an endpoint
	// carrying one is exactly as protected as it would be without it.
	// ADR 0042 §2: Kotlin source is not parsed, and saying so is what makes
	// that exclusion safe. Named with what the files declare, so a reader
	// knows whether routes or security configuration are missing.
	if k := m.Kotlin; k.Files > 0 {
		msg := strconv.Itoa(k.Files) + " Kotlin source file(s) were found and not analyzed (Kotlin is not parsed, ADR 0011 §1)"
		var parts []string
		if k.ControllerFiles > 0 {
			parts = append(parts, strconv.Itoa(k.ControllerFiles)+" declaring controllers ("+strings.Join(k.ControllerExamples, ", ")+
				"): their routes are absent from the matrix, along with any access control on them")
		}
		if k.URLSecurityFiles+k.MethodEnablerFiles > 0 {
			parts = append(parts, "Spring Security configuration is declared in Kotlin ("+strings.Join(k.SecurityExamples, ", ")+
				"): it is not read")
		}
		if len(parts) > 0 {
			msg += "; " + strings.Join(parts, "; ")
		}
		out = append(out, wrapIndented(msg+".", 9, 100)[9:])
	}

	if f := m.MethodSecurityFilters; f.Total() > 0 {
		msg := "found " + strconv.Itoa(f.Total()) + " @PreFilter/@PostFilter annotation(s)"
		if len(f.Classes) > 0 {
			msg += " in: " + strings.Join(f.Classes, ", ")
		}
		out = append(out, msg+".\n"+
			"         These filter a collection; they never deny a call. A caller without the\n"+
			"         authority still invokes the handler and receives a shorter result, so they are\n"+
			"         NOT access control and no endpoint's protection changes because of them.")
	}

	// ADR 0032 §2/§4: controllers whose routes were not all recovered.
	// The counts lead, because the number is what tells a reader the
	// matrix is incomplete and how incomplete; names follow so they can
	// go and look. It says routes were NOT RECOVERED, never that routes
	// exist — a controller whose methods are all @ExceptionHandlers is a
	// correct zero, and the wording must not make it an alarm.
	if u := m.UnrecoveredRoutes; len(u) > 0 {
		var none, partial int
		var names []string
		for _, c := range u {
			if c.NoRoutesAtAll {
				none++
			} else {
				partial++
			}
			if len(names) < 8 {
				names = append(names, c.Name)
			}
		}
		msg := strconv.Itoa(none) + " recognized controller(s) produced no routes"
		if partial > 0 {
			msg += ", and " + strconv.Itoa(partial) + " produced fewer than they declare"
		}
		msg += ": " + strings.Join(names, ", ")
		if len(u) > len(names) {
			msg += ", …"
		}
		out = append(out, msg+".\n"+
			"         Their handlers' mappings were not recognized — declared on an inherited\n"+
			"         interface, behind a method-level meta-annotation, or in a shape\n"+
			"         docs/limitations.md does not yet list. Any authorization on those handlers is\n"+
			"         missing from this report along with the routes.")
	}

	// ADR 0033 §1/§2: WebFlux functional routing. The count is METHODS,
	// worded as indicative rather than exact — a method returning a
	// RouterFunction may be a fragment composed into a chain elsewhere
	// rather than an independent declaration, so it can overstate the
	// number of route sources just as the absence of a route count
	// understates the routes.
	if fr := m.FunctionalRouting; fr.Builders > 0 {
		msg := strconv.Itoa(fr.Builders) + " method(s) building functional routes (WebFlux RouterFunction)"
		if len(fr.Classes) > 0 {
			shown := fr.Classes
			if len(shown) > 6 {
				shown = shown[:6]
			}
			msg += " in: " + strings.Join(shown, ", ")
			if len(fr.Classes) > len(shown) {
				msg += ", …"
			}
		}
		out = append(out, msg+".\n"+
			"         Routes declared this way are built in code rather than by annotation, and are\n"+
			"         not read at all — they do not appear in the matrix and neither does any\n"+
			"         authorization on them. The count is of methods, not routes: one builder can\n"+
			"         declare many routes, and several can compose into one.")
	}

	// ADR 0031 §2: a role hierarchy makes every role shown narrower than
	// what the application grants. The DIRECTION is the substance of the
	// message — a bare "this project has a role hierarchy" leaves the
	// reader to work out which way the numbers are wrong, and the two
	// directions call for opposite responses.
	if h := m.RoleHierarchy; h.Found {
		out = append(out, roleHierarchyWarning(h))
	}

	// ADR 0035 Amendment 1: a permission read from a bean call is what the
	// call NAMES, not what the bean decides. Every bean behind a read
	// permission in the corpus also admits a superuser — @ss.hasPermi
	// passes *:*:*, @ss.hasRole and @el.check pass admin — so the
	// Permissions column reads stricter than who can call the endpoint.
	// Same error class and same treatment as ADR 0031's hierarchy: the
	// direction is stated, and it is a warning, never a finding.
	if callees := permissionCallees(m); len(callees) > 0 {
		out = append(out, "permissions are read from @PreAuthorize bean calls ("+strings.Join(callees, ", ")+").\n"+
			"         The permission shown is what the call NAMES, not what the bean decides: the bean is\n"+
			"         not read, and may admit principals the permission does not name, such as a superuser.\n"+
			"         Access can be BROADER than the Permissions column suggests; a permission shown is not\n"+
			"         evidence that only its holders can call the endpoint.")
	}

	// Amendment 1 §5: routes whose declared path could not be read. The
	// matrix marks each one with a leading ellipsis; this says what the
	// mark means and which controllers to look at, because a fragment
	// printed bare reads as a route that exists.
	if u := unresolvedPaths(m); u.endpoints > 0 && len(u.reasons) == 0 {
		out = append(out, "the route path could not be read for "+strconv.Itoa(u.endpoints)+" endpoint(s) in: "+
			strings.Join(u.controllers, ", ")+".\n"+
			"         Their @Controller/@RequestMapping argument is not a string literal (a route constant or enum),\n"+
			"         so the paths shown for them are marked \u2026 and are only the part that resolved. They are still\n"+
			"         analyzed and still linted; `sphinxor export cerbos` omits them, since a policy cannot be named\n"+
			"         after a fragment of a route.")
	} else if u.endpoints > 0 {
		// ADR 0039 §7: the Spring extractor now reads constants, so what
		// is left has a reason, and the reason is what tells a reader
		// whether anything could ever read it.
		out = append(out, "the route path could not be read for "+strconv.Itoa(u.endpoints)+" endpoint(s) in: "+
			strings.Join(u.controllers, ", ")+".\n"+
			wrapIndented("Their path argument is "+strings.Join(u.reasons, "; ")+". The paths shown for them are "+
				"marked \u2026 and are only the part that resolved. They are still analyzed and still linted; "+
				"`sphinxor export cerbos` omits them, since a policy cannot be named after a fragment of a route.", 9, 100))
	}

	// Amendment 2 §8: one route declared by two controllers, where the two
	// sides carry different guards. It is conditioned on that difference
	// deliberately: the survey behind that amendment found same-path
	// collisions to be common and almost always harmless (a monorepo's
	// per-service health check, a worker re-declaring a route), and
	// warning on all of them would have fired 324 times across the corpus,
	// nearly all where nothing is wrong. A caveat that mostly fires on
	// healthy projects stops being read — which is what
	// TestAnalyzeDirectory_RealProjectStaysQuiet exists to prevent.
	if c := guardDifferingCollisions(m); len(c) > 0 {
		out = append(out, "the same route is declared by more than one controller, with DIFFERENT access control\n"+
			"         on each side: "+strings.Join(c, "; ")+".\n"+
			"         Sphinxor cannot tell whether these are one route or two — a runtime path prefix, a\n"+
			"         conditional controller registration, or a second application in this tree would separate\n"+
			"         them, and none of those is visible here. Each endpoint is listed with its own guards\n"+
			"         rather than merged; `sphinxor export cerbos` omits them.")
	}

	// ADR 0022 §3a: an access-control annotation that could not be
	// identified. Named by the package it actually resolved to, not by
	// its simple name — "@Secured" alone reads as Spring's, which is the
	// confusion that decision exists to remove — and with a count, so the
	// reader knows how much of the report leans on it.
	for _, u := range unrecognizedAuthSummary(m) {
		out = append(out, u)
	}

	// Amendment 3 §11: annotations whose role requirement could not be
	// read. Suppressing the empty-role finding alone would satisfy that
	// amendment's letter and break its spirit — a silently empty Roles
	// column is exactly the absent-looking output ADR 0020 exists to
	// stop. The matrix marks each such cell "?"; this says what the mark
	// means and in which direction the reader should be wrong.
	if n := unresolvedRoleEndpoints(m); n > 0 {
		out = append(out, "the requirement could not be read for "+strconv.Itoa(n)+" endpoint(s) carrying an\n"+
			"         access-control annotation (a @PreAuthorize bean call naming no readable literal, such\n"+
			"         as @el.check() or @validator.hasPermission(#id), a SpEL expression outside the\n"+
			"         recognized subset, or a same-named annotation from another framework). Their Roles\n"+
			"         and Permissions columns are marked ? and UNDERSTATE what the application requires —\n"+
			"         an empty or partial cell there is not evidence that nothing is needed.")
	}

	// ADR 0021 §2: a GraphQL API this tool deliberately does not analyze.
	// It fires whenever resolvers are present, not only when they
	// outnumber the REST routes: a mixed project is the more dangerous
	// case, because its REST matrix is correct and complete, and that
	// apparent completeness is what makes the missing half easy to miss.
	if g := m.GraphQL; g.Present {
		out = append(out, "this project exposes a GraphQL API ("+strconv.Itoa(g.Operations)+" operation(s) in @Resolver\n"+
			"         classes), which Sphinxor does not analyze. Any authorization on those operations is absent\n"+
			"         from this report — the matrix below covers the project's HTTP routes only.")
	}

	// §4: the inverted default. Verified against nestjs/nest's own
	// 19-auth-jwt sample, where every endpoint shows no guard because the
	// guard is registered globally and @Public() opts out.
	if m.GlobalGuards.Registered {
		out = append(out, "a global guard is registered via "+m.GlobalGuards.Mechanism+", which protects every route\n"+
			"         by default. Endpoint-level results below UNDERSTATE protection: an endpoint showing no\n"+
			"         guard may still be protected globally.")
	}

	return out
}

// springMethodSecurityAnnotations are exactly the annotations that
// @EnableMethodSecurity switches on — and so exactly the ones the §4
// caveat is about.
var springMethodSecurityAnnotations = map[string]bool{
	"PreAuthorize": true,
	"Secured":      true,
	"RolesAllowed": true,
	"DenyAll":      true,
}

// hasMethodSecurityAnnotations reports whether the project declares roles
// through one of those annotations.
//
// The check is by annotation name rather than by layer on purpose.
// MethodSecurity.Found is false for any framework without the concept
// (model.Model documents that zero value as "unknown", never "disabled"),
// so a layer-shaped check fires this Spring-specific caveat — naming an
// annotation that does not exist there — on every NestJS project that
// uses @Roles(). A caveat that is both wrong and unmissable on half the
// supported frameworks trains people to ignore the ones that are right.
func hasMethodSecurityAnnotations(m *model.Model) bool {
	for _, g := range m.GuardApplications {
		// A denyAll() or @DenyAll declares no role list but is method
		// security all the same (ADR 0012 Amendment 1): without it here, a
		// project whose only annotations deny everyone would lose the
		// "may be inert" caveat.
		if (g.DeclaresRoles || g.DeniesAll) && springMethodSecurityAnnotations[g.GuardName] {
			return true
		}
	}
	return false
}

// unresolvedPathCount is which controllers hold endpoints whose route
// path could not be read, and how many such endpoints there are.
type unresolvedPathCount struct {
	controllers []string
	endpoints   int
	// reasons are ADR 0039 §7's, with counts; empty for NestJS, whose
	// extractor records none.
	reasons []string
}

// unresolvedPaths gathers those controllers, sorted, for the §5 warning.
func unresolvedPaths(m *model.Model) unresolvedPathCount {
	var result unresolvedPathCount
	name := make(map[model.ID]string, len(m.Controllers))
	for _, c := range m.Controllers {
		name[c.ID] = c.Name
	}
	seen := map[string]bool{}
	reasonCount := map[string]int{}
	for _, e := range m.Endpoints {
		if !e.PathUnresolved {
			continue
		}
		result.endpoints++
		if e.PathUnresolvedReason != "" {
			reasonCount[e.PathUnresolvedReason]++
		}
		n := name[e.ControllerID]
		if n != "" && !seen[n] {
			seen[n] = true
			result.controllers = append(result.controllers, n)
		}
	}
	sort.Strings(result.controllers)
	for r, n := range reasonCount {
		result.reasons = append(result.reasons, r+" ("+strconv.Itoa(n)+")")
	}
	sort.Strings(result.reasons)
	return result
}

// unresolvedRoleEndpoints counts endpoints carrying at least one
// GuardApplication whose requirement could not be read
// (docs/decisions/0020-unanalyzable-is-unknown-not-absent.md Amendment 3
// §9). Counted per endpoint rather than per annotation, so the warning's
// number matches the rows a reader would go and look at.
//
// The warning's example changed with
// docs/decisions/0035-permissions-in-the-model.md: it used to offer
// @ss.hasPermi('...') as the unreadable case, which is precisely the case
// that decision now READS. Leaving it would have pointed a reader at a
// shape the run no longer reports. The count fell RuoYi-Vue 116 -> 0,
// eladmin 99 -> 10 and the vendored ruoyi-vue-pro fixture 5 -> 0.
func unresolvedRoleEndpoints(m *model.Model) int {
	affected := make(map[model.ID]bool)
	for _, g := range m.GuardApplications {
		if g.RolesUnresolved {
			affected[g.EndpointID] = true
		}
	}
	return len(affected)
}

// roleHierarchyWarning renders ADR 0038 Stage 1's warning: the rules
// when they were read, both readings when the version decides them, the
// reason when they could not be read — and in every case the direction of
// the error ADR 0031 §2 requires, plus any version the run had to assume.
func roleHierarchyWarning(h model.RoleHierarchyStatus) string {
	head := "this project declares a Spring Security role hierarchy"
	if len(h.DeclaredIn) > 0 {
		head += " in: " + strings.Join(h.DeclaredIn, ", ")
	}
	var body []string
	switch {
	case h.Read:
		head += ": " + formatHierarchy(h.Edges) + "."
		body = append(body, "Roles are shown as declared, without it, so they are NARROWER than what the application "+
			"grants wherever it applies: a role placed above another also reaches every endpoint the lower one does.")
	case len(h.From52) > 0 || len(h.UpTo50) > 0:
		head += "."
		body = append(body, "Its rules are written in a form Spring Security reads differently by version: up to 5.0 as "+
			orNothing(formatHierarchy(h.UpTo50))+"; from 5.2 as "+orNothing(formatHierarchy(h.From52))+".")
		if hasSpacedRole(h.From52) || hasSpacedRole(h.UpTo50) {
			body = append(body, "A quoted role containing spaces matches no authority.")
		}
		body = append(body, "Nothing in the analyzed source fixes the version, so the roles below are shown as declared "+
			"and may be NARROWER than what the application grants.")
	default:
		head += "."
		reason := ""
		if h.NotRead != "" {
			reason = ": " + h.NotRead + "."
		} else {
			reason = "."
		}
		body = append(body, "Its rules are not read"+reason+" The roles shown below are therefore NARROWER than what "+
			"the application actually grants: a role that implies another reaches every endpoint the implied one does. "+
			"Every role shown is real; the list is not exhaustive.")
	}
	if h.Condition != "" {
		body = append(body, "It is declared under "+h.Condition+", which Sphinxor does not evaluate: it applies only "+
			"when that condition holds.")
	}
	if len(h.AssumedReach) > 0 {
		body = append(body, assumedReachSentence(h.AssumedReach))
	}
	return head + "\n" + wrapIndented(strings.Join(body, " "), 9, 100)
}

// assumedReachSentence states ADR 0038 §12's assumption: the hierarchy is
// taken to reach every layer whose use of it depends on the Spring
// Security version, with the versions that makes true. The numbers come
// from §13 and are re-checked on every major version.
func assumedReachSentence(reach []string) string {
	var method []string
	url := false
	for _, r := range reach {
		if r == "authorizeHttpRequests" {
			url = true
		} else {
			method = append(method, r)
		}
	}
	var parts []string
	if len(method) > 0 {
		parts = append(parts, strings.Join(method, " and "))
	}
	if url {
		parts = append(parts, "authorizeHttpRequests rules")
	}
	version := "6.3 or later"
	switch {
	case url && len(method) > 0:
		version = "6.3 or later; 6.1 for authorizeHttpRequests"
	case url:
		version = "6.1 or later"
	}
	return "Whether it applies to " + strings.Join(parts, " and to ") + " depends on the Spring Security version, " +
		"which nothing in the analyzed source fixes. This assumes it does (" + version + "). " +
		"On an earlier version the roles shown there are exact."
}

// formatHierarchy renders edges as Spring's own notation, joining a run
// of edges that form a chain ("A > B > C") and quoting a role name that
// contains whitespace, so a malformed reading is visible as one.
func formatHierarchy(edges []model.RoleHierarchyEdge) string {
	role := func(r string) string {
		if strings.ContainsAny(r, " \t") {
			return strconv.Quote(r)
		}
		return r
	}
	var chains []string
	for i := 0; i < len(edges); {
		chain := role(edges[i].Higher) + " > " + role(edges[i].Lower)
		j := i + 1
		for ; j < len(edges) && edges[j].Higher == edges[j-1].Lower; j++ {
			chain += " > " + role(edges[j].Lower)
		}
		chains = append(chains, chain)
		i = j
	}
	return strings.Join(chains, ", ")
}

func orNothing(s string) string {
	if s == "" {
		return "no rule at all"
	}
	return s
}

func hasSpacedRole(edges []model.RoleHierarchyEdge) bool {
	for _, e := range edges {
		if strings.ContainsAny(e.Higher, " \t") || strings.ContainsAny(e.Lower, " \t") {
			return true
		}
	}
	return false
}

// wrapIndented word-wraps text to width, every line indented by indent
// spaces — the layout the fixed-text warnings above are written in by
// hand, for a warning whose content is not fixed.
func wrapIndented(text string, indent, width int) string {
	pad := strings.Repeat(" ", indent)
	var lines []string
	line := pad
	for _, w := range strings.Fields(text) {
		if len(line) > indent && len(line)+1+len(w) > width {
			lines = append(lines, line)
			line = pad
		}
		if len(line) > indent {
			line += " "
		}
		line += w
	}
	return strings.Join(append(lines, line), "\n")
}

// permissionCallees returns the distinct callees through which the model's
// permission references were read, sorted — ADR 0035 Amendment 1. Named
// in the warning because the callee is where a reader has to go to find
// out what the bean actually admits; the count would not tell them that.
func permissionCallees(m *model.Model) []string {
	seen := make(map[string]bool)
	var out []string
	for _, p := range m.PermissionReferences {
		if !seen[p.Via] {
			seen[p.Via] = true
			out = append(out, p.Via)
		}
	}
	sort.Strings(out)
	return out
}

// unrecognizedAuthSummary builds one warning per distinct unidentified
// access-control annotation (ADR 0022 §3a), naming the package it
// actually bound to and how many endpoints carry it.
//
// Grouped by binding rather than emitted per endpoint: 392 identical
// lines would bury every other caveat in the run, and the actionable
// facts are which annotation and how much of the report depends on it.
func unrecognizedAuthSummary(m *model.Model) []string {
	endpointsBy := make(map[string]map[model.ID]bool)
	for _, a := range m.UnrecognizedAuthAnnotations {
		key := a.BoundTo
		if key == "" {
			// Nothing in the file bound the name at all, which is itself
			// why it could not be accepted. Say that rather than printing
			// an empty package.
			key = "@" + a.Name + " (no import binds this name in the file where it is used)"
		}
		if endpointsBy[key] == nil {
			endpointsBy[key] = make(map[model.ID]bool)
		}
		endpointsBy[key][a.EndpointID] = true
	}

	keys := make([]string, 0, len(endpointsBy))
	for k := range endpointsBy {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, k := range keys {
		msg := strconv.Itoa(len(endpointsBy[k])) + " endpoint(s) carry " + k + ", which is not a\n" +
			"         Spring Security method-security annotation this tool recognizes. They are NEITHER\n" +
			"         confirmed protected NOR confirmed unprotected: something is guarding them and\n" +
			"         Sphinxor cannot say what it requires. `mutating-endpoint-without-access-control` is\n" +
			"         deliberately not reported for them, and their Guards column is marked ?."
		// ADR 0023 §3: say what that suppression rests on. An annotation
		// only protects anything if its framework's interceptor is wired
		// in, and the run should not stay silent about whether it found
		// the wiring.
		if st, ok := thirdPartyStatusFor(m, k); ok {
			if st.EnablerFound {
				msg += "\n         " + st.Framework + "'s annotation wiring (" + st.Enabler + ") WAS located in the\n" +
					"         analyzed source, so those annotations are active."
			} else {
				msg += "\n         " + st.Framework + "'s annotation wiring (" + st.Enabler + ") was NOT located in the\n" +
					"         analyzed source. If it is not configured elsewhere (a parent module, or Shiro's\n" +
					"         spring-boot starter, which enables it by auto-configuration), those annotations are\n" +
					"         inert and those endpoints are NOT protected."
			}
		}
		out = append(out, msg)
	}
	return out
}

// thirdPartyStatusFor finds the enabling-wiring status for the framework
// whose annotation package boundTo belongs to (ADR 0023 §3).
func thirdPartyStatusFor(m *model.Model, boundTo string) (model.ThirdPartyAuthStatus, bool) {
	for _, st := range m.ThirdPartyAuth {
		if strings.HasPrefix(boundTo, st.Package+".") {
			return st, true
		}
	}
	return model.ThirdPartyAuthStatus{}, false
}
