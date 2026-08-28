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
	ID           ID
	HTTPMethod   HTTPMethod
	Path         string // full path: controller base path + method path
	HandlerName  string
	ControllerID ID
	File         string
	Line         int
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

// GuardScope records whether a GuardApplication's decorator was found at
// the controller (class) level or the handler (method) level in source.
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
