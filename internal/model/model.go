// Package model defines Sphinxor's intermediate model: the
// framework-independent representation of an application's authorization
// surface, extracted from source code.
//
// The shape here is the normalized, ID-referenced collection design
// accepted in docs/decisions/0002-intermediate-model-structure.md — flat
// collections referencing each other by ID, rather than a nested tree.
package model

// ID identifies an entity within a single Model. IDs are only meaningful
// within the Model they were produced by; nothing here assumes global
// uniqueness across separate analysis runs.
//
// Endpoint IDs are the exception: they are derived deterministically from
// HTTPMethod and Path (see NewEndpointID), so the same route produces the
// same ID across two separate runs. That stability is what the allowlist
// matcher (docs/decisions/0003-allowlist-format.md) and, later, v1's drift
// diffing rely on to recognize "the same endpoint" across two points in
// time.
type ID string

// HTTPMethod is an HTTP verb as declared on a NestJS route decorator.
type HTTPMethod string

const (
	MethodGet    HTTPMethod = "GET"
	MethodPost   HTTPMethod = "POST"
	MethodPut    HTTPMethod = "PUT"
	MethodPatch  HTTPMethod = "PATCH"
	MethodDelete HTTPMethod = "DELETE"
)

// Model is the full result of analyzing one project at one point in time:
// every entity extraction found, plus every finding the lint rules
// produced from them.
type Model struct {
	Controllers                []Controller
	Endpoints                  []Endpoint
	GuardApplications          []GuardApplication
	RoleDeclarations           []RoleDeclaration
	RoleReferences             []RoleReference
	AuthenticationRequirements []AuthenticationRequirement
	Findings                   []Finding
	// MethodSecurity is the project-wide fact of whether, and how,
	// method-security annotations are actually enforced —
	// docs/decisions/0015-inert-method-security-guard.md. Its zero value
	// (Found: false) is correct for a framework with no such concept
	// (e.g. NestJS): every consumer treats Found == false as "unknown,"
	// never as "confirmed disabled."
	MethodSecurity MethodSecurityStatus
	URLLayer       URLLayerStatus
	GlobalGuards   GlobalGuardStatus
	GraphQL        GraphQLStatus
	// RouteCollisions records every route declared by more than one
	// controller in the analyzed tree — ADR 0020 Amendment 2 §8.
	RouteCollisions []RouteCollision
	// UnrecognizedAuthAnnotations records access-control annotations that
	// were found on an endpoint and could not be identified — ADR 0022.
	UnrecognizedAuthAnnotations []UnrecognizedAuthAnnotation
	// ThirdPartyAuth records, per third-party authorization framework
	// actually seen in this project, whether the wiring that switches its
	// annotations on was located — ADR 0023 §3.
	ThirdPartyAuth []ThirdPartyAuthStatus
}

// ThirdPartyAuthStatus says whether a third-party authorization
// framework's annotations are actually switched on, for a framework whose
// annotations were found in this project —
// docs/decisions/0023-third-party-authorization-annotations.md §3.
//
// It exists because ADR 0023 §2 suppresses
// mutating-endpoint-without-access-control on every endpoint carrying one
// of those annotations, and an annotation only protects anything if its
// framework's interceptor is wired in. Shiro's
// AuthorizationAttributeSourceAdvisor is the exact counterpart of Spring's
// @EnableMethodSecurity, and this is ADR 0015's treatment of that
// question applied unchanged.
//
// EnablerFound == false is "not located", never "confirmed off" — the
// same distinction MethodSecurityStatus.Found draws, and for a stronger
// reason here: Shiro's spring-boot starter switches annotation support on
// by auto-configuration, leaving no Java bean to find, and this extractor
// does not read build files.
type ThirdPartyAuthStatus struct {
	// Framework is the human name, e.g. "Apache Shiro".
	Framework string
	// Package is the annotation package that was seen, e.g.
	// "org.apache.shiro.authz.annotation".
	Package string
	// Enabler names the wiring looked for, e.g.
	// "AuthorizationAttributeSourceAdvisor", so the warning can tell a
	// reader what to search for.
	Enabler string
	// EnablerFound is true when that wiring was located in the analyzed
	// source.
	EnablerFound bool
}

// UnrecognizedAuthAnnotation is an annotation that carries a recognized
// method-security name but is not bound to a package that makes it the
// real thing — docs/decisions/0022-annotation-identity-and-unrecognized-authorization.md
// §2. alibaba/nacos's com.alibaba.nacos.auth.annotation.Secured is the
// case that produced this type: 428 uses of a name Spring also uses, for
// an unrelated annotation enforced by nacos's own filter.
//
// It is deliberately NOT a GuardApplication with a "recognized" flag.
// ADR 0011 §1 found two consumers silently depending on a GuardApplication
// convention they did not check; a flag here would repeat that exactly,
// since every rule, exporter and report that did not know to test it would
// count the annotation as protection — which is the defect ADR 0022 fixes.
// A separate collection cannot be mistaken for a guard by code that has
// never heard of it.
//
// What it means: an endpoint carrying one is neither confirmed protected
// nor confirmed unprotected. It is specifically NOT the same state as an
// endpoint with nothing on it, which is why
// internal/lint/mutating_endpoint.go skips these (ADR 0022 §3) — that
// rule's message would be false on its face.
type UnrecognizedAuthAnnotation struct {
	ID         ID
	EndpointID ID
	// Name is the annotation's simple name as written, e.g. "Secured".
	Name string
	// BoundTo is the fully-qualified name this file's imports bound Name
	// to, e.g. "com.alibaba.nacos.auth.annotation.Secured". Empty when
	// nothing in the file bound it at all, which is itself a reason not
	// to treat it as Spring's.
	//
	// The warning names this rather than Name, deliberately: "@Secured"
	// alone reads as Spring's, which is the confusion this whole decision
	// exists to remove (ADR 0022 §3a).
	BoundTo   string
	AppliedAt GuardScope
	File      string
	Line      int
}

// RouteCollision is one route declared by two or more different
// controllers in the same analyzed tree, per
// docs/decisions/0020-unanalyzable-is-unknown-not-absent.md Amendment 2 §8.
//
// It is recorded for every collision, but only reported to the user when
// GuardsDiffer. A collision whose sides carry identical guards has nothing
// to bleed, and the measurement behind that amendment found those to be
// the overwhelming majority — warning on all of them would fire 324 times
// across the survey corpus, almost entirely where nothing is wrong, and
// cost the caveat mechanism its meaning.
type RouteCollision struct {
	HTTPMethod HTTPMethod
	Path       string
	// Controllers names every controller declaring this route, sorted.
	Controllers []string
	// GuardsDiffer is true when the colliding endpoints do not all carry
	// the same guards and roles — the case where merging them would have
	// reported one endpoint's protection against another.
	//
	// It reflects what extraction can *see*. Where guards are invisible to
	// it (an unrecognized composite decorator, say), two endpoints look
	// identical and this stays false. That is deliberate: the criterion
	// tracks what the tool actually knows, and if guard coverage later
	// improves and a real difference surfaces, the warning starts firing
	// on its own.
	GuardsDiffer bool
}

// GlobalGuardStatus records a framework-level guard registered away from
// any endpoint — NestJS's APP_GUARD provider or app.useGlobalGuards(),
// per docs/decisions/0020-unanalyzable-is-unknown-not-absent.md §4.
//
// This does not change any finding. It exists because the pattern
// NestJS's own documentation recommends — a global guard protecting
// everything, with @Public() opting out — inverts the default this
// extractor assumes, so endpoint-level results systematically understate
// protection. The direction is safe; the distortion being unsignalled is
// not.
// GraphQLStatus records that a project exposes a GraphQL API, which
// this extractor deliberately does not analyze —
// docs/decisions/0021-graphql-out-of-scope-but-detected.md.
//
// Detection exists so that a scope boundary stops being invisible. A
// GraphQL-first project otherwise produced a clean, confident report
// describing whatever REST routes it happened to have beside its real
// API, and ADR 0019 §2's "recognized no endpoints" notice could not
// fire because the count was not zero.
//
// Operations counts @Query/@Mutation/@Subscription/@ResolveField so the
// warning can state the size of what was skipped. None of them becomes
// an Endpoint: a GraphQL operation has no HTTP method and no path, and
// the model's identity, the matrix's columns, the allowlist's anchoring
// and the Cerbos exporter's action mapping are all built on those.
type GraphQLStatus struct {
	Present    bool
	Operations int
}

type GlobalGuardStatus struct {
	Registered bool
	// Mechanism names how it was registered, for the warning text.
	Mechanism string
}

// URLLayerStatus records what extraction was able to learn about a
// project's URL-layer authorization (Spring's SecurityFilterChain /
// SecurityWebFilterChain), per
// docs/decisions/0020-unanalyzable-is-unknown-not-absent.md §2.
//
// The distinction this type exists to make, which the model previously
// could not express: a layer that is *absent* and a layer that is
// *present but unanalyzable* are different facts. A method-security-only
// project genuinely has no URL layer, and its method layer really is the
// complete picture. A project whose chain could not be read has an
// incomplete picture, and any effective policy derived from the method
// layer alone is unearned.
type URLLayerStatus struct {
	// Present is true if a URL-layer construct was detected at all,
	// whether or not it could be parsed. Detection is deliberately
	// separable from parsing: it is what turns a silent gap into a loud
	// one, and it costs only the token.
	Present bool
	// Analyzed is true if the detected layer was actually parsed into
	// rules. Present && !Analyzed is the unknown state: more than one
	// SecurityFilterChain bean, none parseable, or a reactive
	// SecurityWebFilterChain (whose parsing is out of scope per ADR 0020
	// §3 — detected so it cannot be mistaken for absence).
	Analyzed bool
	// Reason explains Present && !Analyzed in one human-readable clause,
	// for the warning shown to the user and the exporter's omission.
	Reason string
}

// Unknown reports whether a URL layer exists but could not be analyzed —
// the state in which no consumer may treat the method layer as complete.
func (s URLLayerStatus) Unknown() bool { return s.Present && !s.Analyzed }

// MethodSecurityStatus records whether Spring's @EnableMethodSecurity or
// the deprecated @EnableGlobalMethodSecurity was found anywhere in the
// project, and which annotation families it actually enables — each
// gated by its own independent attribute, with its own real default,
// docs/decisions/0015-inert-method-security-guard.md.
type MethodSecurityStatus struct {
	// Found is true if @EnableMethodSecurity or @EnableGlobalMethodSecurity
	// was found anywhere in the parsed project. False means "no evidence
	// either way" — extraction's view of the project is necessarily
	// partial — never "confirmed disabled project-wide."
	Found bool
	// PrePostEnabled/SecuredEnabled/Jsr250Enabled are only meaningful when
	// Found is true: whether at least one located enabling annotation
	// activates @PreAuthorize/@PostAuthorize, @Secured, and
	// @RolesAllowed/@PermitAll/@DenyAll respectively.
	PrePostEnabled bool
	SecuredEnabled bool
	Jsr250Enabled  bool
}

// Controller is a NestJS @Controller() class. It is not itself part of the
// collection set enumerated in ADR 0002, but Endpoint.ControllerID needs
// something to reference, so it's included here to keep the model
// self-contained.
type Controller struct {
	ID       ID
	Name     string
	BasePath string
	File     string
	Line     int
}

// Endpoint is one route: an HTTP method bound to a path, on a specific
// controller handler method.
type Endpoint struct {
	ID          ID
	HTTPMethod  HTTPMethod
	Path        string // full path: controller base path + method path
	HandlerName string
	// PathUnresolved marks a route whose declared path could not be read
	// — a route constant, enum member, array or template literal in
	// @Controller(...)/@RequestMapping(...) — per
	// docs/decisions/0020-unanalyzable-is-unknown-not-absent.md
	// Amendment 1 §5. Path then holds only the part that could be
	// resolved, which is a fragment of the real route and must never be
	// presented as the whole of it.
	PathUnresolved bool
	// Version is the route's declared API version — NestJS's
	// @Controller({ version }) / @Version(), Spring's `version` attribute
	// on a mapping annotation — per
	// docs/decisions/0020-unanalyzable-is-unknown-not-absent.md
	// Amendment 2 §7. Empty means no version was declared, which is
	// *absent*, not unknown: such an endpoint keeps exactly the identity
	// it had before that amendment.
	//
	// It is deliberately not folded into Path. Whether a version reaches
	// the URL depends on how the application configures versioning, which
	// is declared away from the endpoint (NestJS's enableVersioning) and
	// is not read here — URI versioning puts it in the path, header and
	// media-type versioning do not.
	Version string
	// RouteCollision marks an endpoint whose declared route is also
	// declared by a *different* controller in the same analyzed tree, per
	// docs/decisions/0020-unanalyzable-is-unknown-not-absent.md
	// Amendment 2 §8.
	//
	// Nothing here is unanalyzable: both paths read perfectly. What is
	// unknown is whether the two are the same route — a runtime path
	// prefix, a conditional controller registration, or a separate
	// application mount may separate them, and none of those is visible
	// to this extractor. The endpoints are kept apart unconditionally,
	// because assuming they are one endpoint is what merged their guards
	// (NestJS) or dropped one of them outright (Spring).
	RouteCollision bool
	// VersionUnresolved marks a route that declares a version whose value
	// could not be read — a constant reference, an array of them, a
	// computed value. Two such endpoints must never be assumed equal, so
	// identity falls back to the controller-and-handler synthesis
	// Amendment 1 §5 introduced.
	VersionUnresolved bool
	ControllerID      ID
	File              string
	Line              int
}

// NewEndpointID derives an Endpoint's stable ID from its method and path,
// per the "shared, structure-independent points" section of ADR 0002. This
// is the identity the allowlist matcher and v1's drift diffing key on.
//
// This identity breaks if a route's path is renamed between two analysis
// runs — an accepted, documented limitation (ADR 0002), not an oversight.
func NewEndpointID(method HTTPMethod, path string) ID {
	return ID(string(method) + " " + path)
}

// NewUnresolvedPathEndpointID derives an Endpoint's ID from its
// controller and handler instead of its path, for the case where the
// declared path could not be read — per
// docs/decisions/0020-unanalyzable-is-unknown-not-absent.md Amendment 1 §5.
//
// It is used only where NewEndpointID cannot form a real identity.
// Endpoints with a readable path keep the path-derived ID, so existing
// allowlist anchors and stored diff baselines are untouched.
//
// Deriving identity from structure is a deliberate, narrow departure
// from ADR 0002's preference for structure-independent keys: where the
// structure-independent key cannot be formed at all, the alternative is
// not a better key but a collision, which silently merges or deletes
// endpoints. Stability across runs — the diff's actual requirement
// (ADR 0007) — holds as long as the class and method names do, which is
// at least as stable as a path, since renaming a route is routine and
// renaming a handler is not.
//
// If the path later becomes readable the ID changes, and `sphinxor diff`
// reports the endpoint as one removed and one added. That is accepted:
// it is rare, it is visible rather than silent, and it fails toward a
// spurious gate that asks a human rather than toward waving something
// through.
//
// The "?" prefix cannot collide with a path-derived ID, since a resolved
// path is always normalized to a leading "/".
func NewUnresolvedPathEndpointID(method HTTPMethod, controllerName, handlerName string) ID {
	return ID(string(method) + " ?unresolved-path " + controllerName + "." + handlerName)
}

// NewVersionedEndpointID derives an Endpoint's ID from its method, path,
// and declared API version, per
// docs/decisions/0020-unanalyzable-is-unknown-not-absent.md Amendment 2 §7.
//
// It is used only when a version is declared *and* readable. An endpoint
// declaring no version keeps NewEndpointID, unchanged — that is the
// bounding rule the amendment rests on, and it is what leaves existing
// allowlist anchors and stored diff baselines untouched.
//
// An endpoint declaring a version that could not be read does not come
// here: an unreadable version is unknown, and two unknowns must not
// collapse onto one key, so those fall back to
// NewUnresolvedVersionEndpointID.
//
// The "@" separator cannot collide with a plain path-derived ID, since a
// bare NewEndpointID never contains one.
func NewVersionedEndpointID(method HTTPMethod, path, version string) ID {
	return ID(string(method) + " " + path + " @" + version)
}

// NewUnresolvedVersionEndpointID derives an Endpoint's ID from its
// controller and handler for the case where a version is declared but its
// value could not be read — the cal.com shape, where the version is a
// constant reference or an array of them.
//
// It is the same synthesis NewUnresolvedPathEndpointID performs, under a
// distinct marker so the two causes stay distinguishable in output and in
// a diff. The reasoning for structure-derived identity is identical, and
// recorded there.
func NewUnresolvedVersionEndpointID(method HTTPMethod, controllerName, handlerName string) ID {
	return ID(string(method) + " ?unresolved-version " + controllerName + "." + handlerName)
}

// NewCollidingRouteEndpointID derives an Endpoint's ID from its controller
// and handler for the case where a *different* controller in the same tree
// declares the same route, per
// docs/decisions/0020-unanalyzable-is-unknown-not-absent.md Amendment 2 §8.
//
// It is the same synthesis the two constructors above perform, under its
// own marker so the cause stays distinguishable in output and in a diff.
// Unlike those two, nothing here was unreadable: the identity is
// synthesized because a shared one is *wrong*, not because the real one
// could not be formed.
//
// The file is part of the key here, where the other two constructors need
// only the class and handler names. A collision frequently *is* the same
// class name declared twice — a HealthController in each of a monorepo's
// services, a worker re-declaring a controller — so class and handler
// alone would reproduce the very collision this is resolving. The cost is
// that moving such a file changes the endpoint's ID and `sphinxor diff`
// reports a removal and an addition, on the same terms §5 already accepts.
func NewCollidingRouteEndpointID(method HTTPMethod, file, controllerName, handlerName string) ID {
	return ID(string(method) + " ?colliding-route " + file + ":" + controllerName + "." + handlerName)
}

// GuardScope records where a GuardApplication's evidence was found in
// source: the controller (class) level, the handler (method) level, or —
// for a framework whose authorization can be declared apart from any
// annotated method entirely — a request-matcher rule.
//
// A class-level guard applies to every endpoint on that controller.
// Extraction is expected to expand a class-level @UseGuards() into one
// GuardApplication per affected Endpoint (each carrying ScopeClass), rather
// than a separate controller-scoped collection — this keeps "does this
// endpoint have a guard" a direct filter over GuardApplications, at the
// cost of the same class-level guard appearing once per endpoint. That
// duplication is regenerated on every analysis run, not hand-maintained,
// so it isn't a drift risk the way a hand-maintained file would be.
type GuardScope string

const (
	ScopeClass  GuardScope = "class"
	ScopeMethod GuardScope = "method"
	// ScopeRequestMatcher is a GuardApplication derived from a recognized
	// URL-pattern-based authorization rule (e.g. Spring's
	// authorizeHttpRequests) rather than an annotation on the endpoint's
	// own handler or class — docs/decisions/0012-securityfilterchain-effective-policy.md.
	// File/Line point at the rule's own location (e.g. a SecurityConfig
	// class), not the endpoint's controller, since that's where the
	// evidence actually lives.
	ScopeRequestMatcher GuardScope = "request_matcher"
)

// GuardApplication is one authorization guard found protecting one
// Endpoint — e.g. a NestJS @UseGuards(RolesGuard) application.
type GuardApplication struct {
	ID         ID
	EndpointID ID
	GuardName  string
	AppliedAt  GuardScope
	File       string
	Line       int
	// FromComposite is true when this GuardApplication was produced by
	// resolving a project-defined composite decorator (one built with
	// applyDecorators(), e.g. @Auth([...])) rather than a literal
	// @UseGuards()/@Roles() call — see
	// docs/decisions/0006-composite-decorator-resolution.md.
	//
	// The zero value, false, is the normal literal-decorator path: every
	// GuardApplication built the way extraction has always built them
	// gets the correct behavior with no explicit initialization. Only
	// composite resolution sets this true.
	FromComposite bool
	// DeclaresRoles is true when this GuardApplication is the one whose
	// associated RoleReferences (if any) constitute the endpoint's role
	// requirement, as opposed to a supporting guard with no role list of
	// its own (e.g. NestJS's AuthGuard/RolesGuard, which only establish
	// that a check happens elsewhere).
	//
	// Framework-independent by construction — added specifically because
	// docs/decisions/0011-spring-second-framework.md found two consumers
	// (internal/lint/empty_role.go, internal/report/report.go) inferring
	// this fact by comparing GuardName against the literal string "Roles",
	// which is NestJS's own synthetic naming convention for the
	// GuardApplication it builds from a @Roles() decorator, not a
	// framework-independent signal. A second framework with a different
	// convention (Spring's @PreAuthorize/@Secured/@RolesAllowed fuse
	// presence and role-check into one annotation, with no equivalent
	// "Roles"-named entity at all) would have made both consumers silently
	// wrong rather than visibly broken. This field replaces the string
	// comparison with an explicit fact extraction sets directly.
	DeclaresRoles bool
	// RolesUnresolved is true when this GuardApplication declares a role
	// requirement (DeclaresRoles) whose role list extraction could not
	// read — as distinct from reading it and finding it empty. It is a
	// statement about what extraction could recover from the source,
	// never about what the application requires.
	//
	// The distinction exists because the two states were previously
	// indistinguishable: both produced DeclaresRoles: true with zero
	// RoleReferences, which is internal/lint/empty_role.go's trigger. On
	// Spring, where ADR 0011 §1 fuses presence and role-check into one
	// annotation, that collision made every unreadable @PreAuthorize SpEL
	// expression and every non-Spring @Secured look like a role list a
	// developer had forgotten to fill in — 675 High-confidence, CI-gating
	// false positives across four production repositories. See
	// docs/decisions/0020-unanalyzable-is-unknown-not-absent.md Amendment 3.
	//
	// The zero value, false, is "the role list was read" — so every
	// existing construction path (NestJS's @Roles(), a resolved
	// @Secured({"ROLE_A"}), a genuinely empty @Secured({})) keeps its
	// current behavior with no explicit initialization.
	RolesUnresolved bool
}

// RoleDeclarationKind records how a role's canonical declaration was
// found, if at all. NestJS has no built-in role registry — projects
// declare roles as a TypeScript enum, as const values, or not at all
// (bare string literals passed directly to a decorator).
type RoleDeclarationKind string

const (
	RoleDeclarationEnum      RoleDeclarationKind = "enum"
	RoleDeclarationConst     RoleDeclarationKind = "const"
	RoleDeclarationNoneFound RoleDeclarationKind = "none-found"
)

// RoleDeclaration is a role's canonical declaration site, when one can be
// found. Its Name is the stable key used to diff role declarations across
// two analysis runs (ADR 0002).
type RoleDeclaration struct {
	ID   ID
	Name string
	Kind RoleDeclarationKind
	File string
	Line int
}

// RoleReference is one place in the code where a role is required —
// typically a string literal argument to a @Roles()-style decorator,
// attached to a GuardApplication.
//
// RoleDeclarationID is nil when no matching RoleDeclaration was found
// (e.g. the project uses bare string literals with no enum or const
// backing them). This is an explicit, honest "no declaration found"
// state — it is never inferred into existence.
type RoleReference struct {
	ID                 ID
	GuardApplicationID ID
	RoleDeclarationID  *ID
	RawLiteral         string
	File               string
	Line               int
}

// AuthenticationRequirement is a positive, confirmed fact about an
// Endpoint: it has at least one GuardApplication the extractor positively
// recognizes as an authentication guard, and none of the endpoint's
// guards resolve to a specific role — "authenticated, any role" in the
// source (docs/decisions/0010-authenticated-any-role.md).
//
// Never inferred from silence, and never inferred from an unrecognized
// guard's mere presence — only created when extraction can point at a
// guard it positively recognizes as doing authentication. Which guard
// names are recognized is framework-specific (per that ADR's Consequences
// note) and lives in the extractor package, not here; this type only
// records the resulting fact, framework-independently, the same way
// RoleDeclaration/RoleReference do for roles.
type AuthenticationRequirement struct {
	ID         ID
	EndpointID ID
	File       string
	Line       int
	// AppliedAt records which layer established this requirement — the
	// same field name and GuardScope vocabulary GuardApplication.AppliedAt
	// already uses, for exactly the same reason: a framework where
	// authorization can be declared apart from any annotated method
	// (docs/decisions/0012-securityfilterchain-effective-policy.md,
	// docs/decisions/0013-authentication-requirement-scope-field.md) can
	// independently produce an AuthenticationRequirement from more than
	// one layer for the same Endpoint, and the method×URL effective-policy
	// reduction (internal/export/cerbos) needs to tell them apart to
	// reconcile them correctly — it cannot assume every
	// AuthenticationRequirement on an endpoint came from the same place
	// the way a single-layer framework like NestJS always does.
	AppliedAt GuardScope
}

// Confidence is the confidence grade attached to a Finding. Sphinxor never
// reports binary vulnerable/not-vulnerable results (docs/vision.md) —
// every finding carries an honestly stated confidence grade instead.
//
// Per docs/decisions/0004-confidence-level-granularity.md, there are
// exactly two grades: ConfidenceHigh gates CI, ConfidenceLow is a
// non-blocking warning. A third, middle tier was deliberately deferred to
// v1, once real cases exist to calibrate it against.
type Confidence string

const (
	ConfidenceHigh Confidence = "high"
	ConfidenceLow  Confidence = "low"
)

// FindingSubjectKind identifies what kind of entity a Finding is about,
// via Finding.SubjectID.
type FindingSubjectKind string

const (
	SubjectEndpoint        FindingSubjectKind = "endpoint"
	SubjectRoleDeclaration FindingSubjectKind = "role_declaration"
	// SubjectAllowMarker is used by the stale-allow-marker finding
	// (docs/decisions/0003-allowlist-format.md), whose subject is the
	// marker's own location rather than any entity in the model — there is,
	// by definition, no recognized endpoint for it to attach to.
	SubjectAllowMarker FindingSubjectKind = "allow_marker"
)

// Finding is one lint result: a single rule's judgment about a single
// subject entity, at a stated confidence.
type Finding struct {
	ID          ID
	RuleID      string
	Confidence  Confidence
	SubjectID   ID
	SubjectKind FindingSubjectKind
	Message     string
	Allowlisted bool
}
