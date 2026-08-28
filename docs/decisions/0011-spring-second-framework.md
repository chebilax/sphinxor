# 0011. Spring as the second framework

## Status

Proposed.

## Context

`vision.md`'s v2 names "a second and third framework" alongside the first export
target. Between the two ways to spend that slot — a second exporter (OPA/Rego) or
a second framework (Spring) — Spring is chosen deliberately, and for a reason
specific to risk sequencing, not feature parity: OPA reuses ADR 0009's pipeline
with the model already fixed in place, low structural risk. Spring is the first
real test of ADR 0002's foundational claim — "the model is framework-independent" —
and that claim has never been checked against a second framework, only asserted
while building the first one. If the model needs to change to fit Spring, better to
find that now, with one exporter (Cerbos) downstream to adjust, than later with two.
Same depth-before-breadth asymmetry `vision.md` already applies elsewhere: do the
structurally risky thing while the least is attached to it.

**This moment was explicitly anticipated, not new.** ADR 0001 rejected Spring for
v0.1 on testability grounds — its "global-config override pattern... will need to be
accounted for when the model is generalized in v2 — it should not be designed away
just because NestJS doesn't exhibit it." This ADR is where that account comes due.
ADR 0010 separately anticipated that its recognized-guard-name list "does not grow
into one shared, multi-framework list — it splits, one small recognized set per
extractor package... when a second framework arrives." Both predictions are
confronted directly below, not assumed to have resolved themselves.

**Everything in §1 below was checked against Spring Security's actual current
documentation and a real tree-sitter-java parse, not recalled from general
knowledge.** Two things turned out to matter more than expected, found only by
reading the real docs:

- `github.com/smacker/go-tree-sitter` (Sphinxor's existing Tree-sitter dependency)
  bundles a Java grammar, confirmed by parsing a realistic annotated Spring
  controller: `@RestController`/`@RequestMapping`/`@PreAuthorize`/`@Secured` all
  produce structured `annotation`/`marker_annotation` nodes, the same way NestJS
  decorators do. **But the SpEL expression inside `@PreAuthorize("...")` is not
  further parsed — it's an opaque Java string literal.** SpEL is its own expression
  language, not Java syntax; tree-sitter-java has no reason to parse into it.
  Recognizing `hasRole('ADMIN')` versus `hasAuthority('X')` versus an arbitrary bean
  method call requires Sphinxor to pattern-match the *string content* itself, the
  same way `internal/allowlist/marker.go` already pattern-matches comment text —
  not a further AST-level structural parse. This directly shapes what's tractable
  to recognize in §3.
- Spring's own method-security docs state plainly: **"Spring Boot Starter Security
  does not activate method-level authorization by default,"** and separately,
  **"when you use annotation-based Method Security, then unannotated methods are
  not secured. To protect against this, declare a catch-all authorization rule in
  your `HttpSecurity` instance."** Both are load-bearing for §1 below — they mean
  `@PreAuthorize`/`@Secured`/`@RolesAllowed` can be present in source and still be
  inert, and that request-level config isn't a rare escape hatch but Spring's own
  documented, recommended complement to (or substitute for) method annotations.

## Decision

### §1. Where Spring's shape does and doesn't match NestJS's — enumerated, not assumed

**Absorbs cleanly, no model change:**

- **Endpoint declaration.** `@RestController`/`@Controller` + class-level
  `@RequestMapping("/base")` + method-level `@GetMapping`/`@PostMapping`/
  `@PutMapping`/`@DeleteMapping`/`@PatchMapping` (or `@RequestMapping(method=...)`)
  compose a base path and a per-method path exactly the way NestJS's
  `Controller.BasePath` + `Endpoint.Path` already do. `Controller`/`Endpoint` need
  no new fields.
- **Class-level vs. method-level security.** Spring annotations can sit on the
  class (apply to every method) or the method — the same two-level shape
  `GuardApplication.AppliedAt` (`ScopeClass`/`ScopeMethod`) and the
  expand-class-level-to-every-endpoint convention (ADR 0002) already model.
- **Multiple roles in one annotation.** `@Secured({"ROLE_ADMIN", "ROLE_MANAGER"})`,
  `hasAnyRole('A','B')` — `RoleReference` is already one row per role per guard
  application; multiple roles from one annotation is already representable, not a
  new capability.
- **Role declarations.** Real Spring code declares roles as a Java `enum`, as
  `public static final String` constants, or as bare literals with nothing backing
  them — the same three-way split `RoleDeclarationKind` (`enum`/`const`/
  `none-found`) already names for NestJS's TypeScript-side equivalent. Only the
  Java-side syntax to walk is new; the concept isn't.
- **Path variable syntax** (`{id}` vs. NestJS's `:id`) — cosmetic text stored
  verbatim in `Endpoint.Path` either way, no model concern.

**Needs a small, additive model amendment — found by checking the "framework-
independent" claim against real code, not assumed to hold:**

- **Two consumers already meant to be framework-independent quietly aren't,
  because they hardcode NestJS's own naming convention.** `internal/lint/empty_role.go`
  checks `g.GuardName != "Roles"`; `internal/report/report.go`'s `BuildMatrix`
  checks `g.GuardName == "Roles"` — both to mean "the `GuardApplication` that
  represents an endpoint's role-list declaration, as opposed to a supporting guard
  with no role list of its own (`AuthGuard`, `RolesGuard`)." `"Roles"` is not a
  framework-independent signal; it's the literal string NestJS extraction happens
  to assign (`internal/extract/nestjs/controllers.go`) to the synthetic
  `GuardApplication` it builds from a `@Roles()` decorator. Confirmed by grepping
  every production call site, not assumed: exactly these two files branch on it.
  Had Spring extraction gone ahead using `GuardName` values of `PreAuthorize`/
  `Secured`/`RolesAllowed` (the real, honest annotation names — §1 above), `empty_role`
  would silently never fire on Spring data, and the RBAC matrix would silently list
  every Spring role-check annotation under "Guards" instead of "Roles" — not a
  crash, not a visible failure, just quietly wrong output with no test positioned
  to catch it, exactly because both packages *look* framework-independent (no
  NestJS import) while actually depending on a NestJS extraction convention.
  **This is the finding the "no changes needed" success criterion exists to
  surface** — addressed here, not discovered mid-implementation and patched around.
- **Fix: add `GuardApplication.DeclaresRoles bool`**, set by extraction, replacing
  the string-name convention with an explicit fact: this `GuardApplication` is the
  one whose associated `RoleReference`s (if any) constitute the endpoint's role
  requirement. NestJS extraction sets it `true` for exactly the `GuardApplication`
  it already synthesizes from `@Roles()` (no change to *when* that entity is
  created, only an added field on it) and `false` elsewhere; `empty_role.go` and
  `report.go` are updated to read `DeclaresRoles` instead of the literal name. This
  is done — and NestJS's own existing tests re-verified green — *before* any
  Spring extraction code is written, so Spring starts from a genuinely generalized
  signal on day one rather than inheriting the same trap. For Spring, every
  recognized guard (`PreAuthorize`/`Secured`/`RolesAllowed`) sets `DeclaresRoles:
  true` — there's no NestJS-style split between a role-bearing guard and a
  separate protection-only guard, since presence and role-check are fused (below).

**Needs new extraction logic, not a further model concept:**

- **One Spring method can map multiple HTTP verbs** (`@RequestMapping(method =
  {RequestMethod.GET, RequestMethod.POST})`), unlike NestJS where one decorator is
  one verb. Extraction must expand this into multiple `Endpoint` rows (one per
  verb, same handler) — mechanically similar to how a class-level guard already
  expands into one `GuardApplication` per affected endpoint. No `Endpoint` field
  changes; just more rows created from one source method.
- **The guard/role distinction NestJS's model was built around is fused in
  Spring's common case.** NestJS separates "is this endpoint guarded at all"
  (`@UseGuards`) from "what role does it need" (`@Roles`) — two decorators, two
  concerns. `@PreAuthorize("hasRole('ADMIN')")` is simultaneously both: its mere
  presence establishes a guard, and its SpEL content is usually the role check.
  `GuardApplication` still fits — `GuardName` holds the annotation name
  (`PreAuthorize`/`Secured`/`RolesAllowed`) instead of a guard class name — but
  the fusion means recognizing a role reference now depends on parsing SpEL string
  content (see below) rather than reading a separate decorator's arguments.
- **Recognizing a `RoleReference` inside SpEL is deliberately narrow, not a full
  SpEL parser.** Per the opaque-string finding above, extraction pattern-matches
  a small, explicit set of SpEL call shapes: `hasRole('X')`, `hasAnyRole('X','Y')`,
  `hasAuthority('X')`, `hasAnyAuthority('X','Y')`. Anything else — a bean method
  call (`@authService.check(#id)`), a boolean combination
  (`hasRole('A') || hasAuthority('B')`), a comparison against `#parameter` or
  `authentication.name` — is **not** parsed into a `RoleReference`; the guard is
  still recorded (the annotation is real, its presence is evidence), but with no
  resolved role, landing in exactly the existing "guarded, no role" bucket ADR 0010
  already defined. No new model state for "SpEL too complex to parse" — the
  existing honest-unresolved shape already says it correctly.
- **Role naming convention is recorded, not normalized.** `hasRole('ADMIN')`
  implicitly checks authority `ROLE_ADMIN`; `hasAuthority('ADMIN')` does not add
  that prefix. `RawLiteral` records exactly what was written (`"ADMIN"` either
  way) — Sphinxor does not infer the prefix or treat `hasRole('ADMIN')` and
  `hasAuthority('ROLE_ADMIN')` as "the same role," the same discipline already
  applied to every other `RawLiteral` field. A real, stated limitation, not
  silently smoothed over: two references that are semantically identical in a real
  running app can look like different roles here.
- **`@EnableMethodSecurity`/`@EnableGlobalMethodSecurity` gates whether method
  annotations do anything at all — extraction must check for it, project-wide,
  and this is a genuinely new consideration, not present in NestJS's model.**
  Unlike a NestJS guard (active the moment it's applied, modulo the separate
  global-guard blind spot), a Spring method-security annotation can be present in
  source and be **entirely inert** if neither `@EnableMethodSecurity` (current) nor
  the deprecated `@EnableGlobalMethodSecurity` appears on any `@Configuration`
  class in the project. Reporting a `RoleReference`-backed finding as confidently
  as NestJS's equivalent, without checking this, would be a false-confidence risk
  specific to Spring — the tool asserting protection that may not be enforced,
  which is arguably worse for an *auditing* tool than a missed finding. Extraction
  scans the whole project once for either enabling annotation; if neither is
  found, every method-security-derived `GuardApplication`/`RoleReference` in that
  project is still recorded (the facts about the source are still true), but
  findings built on them get an explicit confidence caveat rather than being
  treated the same as a project where method security is confirmed active. Exact
  confidence-level plumbing is implementation, not decided further here.

**Real, documented blind spot for v1 — not absorbed, not designed around:**

- **`SecurityFilterChain`/`HttpSecurity`/`authorizeHttpRequests` URL-pattern-based
  authorization is out of scope for this version, and that costs more here than
  NestJS's equivalent gap did.** This is NestJS's `APP_GUARD`/global-guard blind
  spot's structural twin — authorization declared entirely apart from any
  annotated method, matched by URL pattern rather than by reference to a specific
  handler. But where NestJS's global guard is a real, occasionally-used escape
  hatch, Spring's own documentation names request-level config as a first-class,
  *recommended* mechanism, explicitly complementary to method security ("unannotated
  methods are not secured... declare a catch-all rule in your `HttpSecurity`
  instance"). Matching `.requestMatchers("/admin/**").hasRole(...)` (Ant-style glob
  patterns, optional HTTP-method scoping, ordered rules where the first match
  wins) against a specific extracted `Endpoint` is a materially different, harder
  problem than anything the model currently does — pattern specificity and
  evaluation order both affect the real outcome, and getting them wrong would
  produce a confidently *wrong* answer, not just a missed one. Building that
  matching engine is deferred, deliberately, not attempted narrowly and badly.
  **This is not assumed to be a minor gap.** It's flagged here as the biggest open
  risk to this whole effort's practical usefulness, and §3 below commits to
  measuring — not guessing — how much of a real Spring app's authorization surface
  it actually leaves invisible.
- **`@PostAuthorize`, `@PreFilter`, `@PostFilter`** evaluate against a method's
  return value or filter a collection after/before invocation — object-level,
  ABAC-shaped checks with no RBAC equivalent in the current model, the same reason
  ADR 0009 excluded Cerbos's `derivedRoles`/`condition` from the exporter. Not
  modeled, not approximated; a documented non-goal.
- **Composed (meta-)annotations** — a project defining its own `@IsAdmin`, itself
  annotated with `@PreAuthorize(...)`, is Java's equivalent of NestJS's
  `applyDecorators()` composites (ADR 0006). The *concept* generalizes, but
  resolving it is scoped out of this first cut, per §3 below — an endpoint using
  an unrecognized composed annotation is treated as unguarded (the same honest
  default as any other unrecognized construct), not a special case.
- **`RoleHierarchy`** (e.g. `ROLE_ADMIN > permission:read`, letting one role imply
  another) is never resolved — every role name is treated as exactly what's
  written, consistent with never inferring a relationship the source doesn't state
  syntactically at the point being read.
- **Kotlin** is a real, supported way to write Spring Security code (every example
  in Spring's own docs is dual Java/Kotlin) but is out of scope here — Java only,
  the same "one language surface at a time" discipline already applied to NestJS
  (TypeScript, not also plain JavaScript).

### §2. The recognized-guard/authentication list, framework-scoped for real

ADR 0010 anticipated that its recognized-authentication-guard concept would split
per framework rather than grow into one shared list. That's now real, and the
split is deeper than a name swap: NestJS's recognized set is a guard **class
name** (`AuthGuard`); Spring has no equivalent single marker, because presence and
role-check are fused (§1). Spring's recognized set for "authenticated, any role"
(`AuthenticationRequirement`) is instead a **specific SpEL predicate literal**:
`isAuthenticated()` inside `@PreAuthorize`. `permitAll()` is a different thing
entirely — an explicit, positive "public on purpose" signal — and per ADR 0003's
own precedent for NestJS's `@Public()`-shaped decorators, an application-level
"this is public" signal is **not** treated as Sphinxor's own allowlist by
inference; only an explicit `sphinxor-allow` marker suppresses a finding. An
endpoint marked `permitAll()` still surfaces through the normal unguarded/no-role
path if a lint rule would otherwise flag it — the same reasoning ADR 0003 already
settled, applied here rather than re-litigated.

`recognizedAuthenticationGuards` (`internal/extract/nestjs/authentication.go`)
gets a direct counterpart in a new `internal/extract/spring` package with its own
recognized SpEL predicate set — not an extension of the NestJS one, not a shared
list in `internal/model`, exactly as ADR 0010 said this split should look.

### §3. Scope for the first Spring cut

**In scope:**

- Endpoint extraction: `@RestController`/`@Controller`, `@RequestMapping` and the
  `@GetMapping`/`@PostMapping`/`@PutMapping`/`@DeleteMapping`/`@PatchMapping`
  family, class+method path composition, multi-verb expansion (§1).
- `@PreAuthorize`, `@Secured`, `@RolesAllowed` — class- and method-level.
- The recognized SpEL subset: `hasRole`/`hasAnyRole`/`hasAuthority`/
  `hasAnyAuthority` → `RoleReference`(s); `isAuthenticated()` →
  `AuthenticationRequirement` (§2). `permitAll()`, `denyAll()`, and anything more
  complex are recognized as *present* (a real `GuardApplication` exists) but
  resolve no role — the existing "guarded, no role" shape, not a new one.
- The `@EnableMethodSecurity`/`@EnableGlobalMethodSecurity` project-wide presence
  check and its confidence implication (§1).
- Role declarations: Java `enum`, `public static final String` constants, or
  none-found — mirroring `RoleDeclarationKind` as-is.

**Explicitly deferred, documented as blind spots (`docs/limitations.md`), not
attempted narrowly:**

- `SecurityFilterChain`/`HttpSecurity`/`authorizeHttpRequests` (§1) — the
  headline gap, measured empirically per §"Consequences" below, not assumed small.
- `@PostAuthorize`/`@PreFilter`/`@PostFilter`.
- Composed/meta-annotations.
- `RoleHierarchy` resolution.
- Kotlin source.
- Custom `AuthorizationManager` beans / any other programmatic authorization path.

This mirrors v0.1's own discipline for NestJS — one framework, narrow, done
properly — applied to the *surface within* Spring rather than to Spring as a
whole: annotation-based method security, in depth, correctly; everything else,
honestly out of scope rather than guessed at.

## Alternatives considered

- **Attempt `SecurityFilterChain` parsing in this same pass** — rejected: a
  URL-glob-pattern matcher with method scoping and first-match-wins ordering
  semantics is a different, harder problem than anything currently in the
  extractor, and bolting it on to hit "more coverage" is exactly the
  broad-and-leaky outcome this ADR is explicit about avoiding. It deserves its own
  design pass once real corpus data shows how much it's actually worth.
- **Assume an unannotated Spring endpoint is protected by `SecurityFilterChain`
  and skip flagging it** — rejected outright: an optimistic default in the
  direction of "probably fine" is a guess Sphinxor doesn't make anywhere else, and
  doing it here specifically to paper over Spring's biggest blind spot would
  quietly turn a stated limitation into a hidden false negative. The existing
  `mutating-endpoint-without-access-control` rule (Low confidence, framework-
  independent already since it only reads `GuardApplication` presence) continues
  to flag these exactly as it does for NestJS's own global-guard gap — same
  honest hedge, not a special case for Spring.
- **Skip the `@EnableMethodSecurity` presence check, treat every annotation as
  live** — rejected: found via real documentation, not hypothesized; skipping it
  would let Sphinxor confidently assert protection Spring itself might not be
  enforcing, the specific false-confidence failure this ADR exists to prevent one
  layer earlier than usual (before the finding is even generated, not after).
- **Normalize `hasRole('X')`/`hasAuthority('ROLE_X')` as the same role** —
  rejected: would require inferring the `ROLE_` convention is actually in effect
  for a given project, which isn't guaranteed (`GrantedAuthority` schemes are
  configurable) — recording both literally and letting a human notice the overlap
  is the honest default already used everywhere else in this model.

## Consequences

- A new `internal/extract/spring` package, Java via `github.com/smacker/go-tree-sitter`'s
  bundled Java grammar (confirmed parseable against realistic annotated Spring
  source, not assumed) — no new Tree-sitter dependency.
- **One small, additive `internal/model` change: `GuardApplication.DeclaresRoles
  bool` (§1).** Everything else Spring-side — endpoints, guard applications, role
  references, authentication requirements, role declarations — fits the
  collections ADR 0002 and ADR 0010 already define, with no further amendment.
  This is the honest shape of the finding this ADR was written to surface: the
  model's framework-independence claim mostly holds, but checking it for real —
  not just against the schema, but against what every consumer actually branches
  on — found one place it was quietly propped up by a NestJS naming convention
  leaking into two nominally framework-independent packages. Fixing that generalizes
  a signal the model always needed, rather than patching two packages to
  special-case Spring's naming underneath an unchanged, still-dishonest field. It
  does not hold universally — `SecurityFilterChain` is real authorization data the
  model has no representation for — but that gap is being kept out by scope
  (§1, §3), not discovered as a forced, awkward model change.
- **Sequencing matters for the `DeclaresRoles` fix**: it lands first, against
  NestJS data only, with the existing NestJS test suite re-verified green before
  any Spring extraction code exists — a generalization checked against the
  framework it was extracted from, not assumed correct because it compiles.
- **The real test of this ADR, from that point on**: `internal/lint`'s three
  rules, `internal/diff`, and `internal/export/cerbos` all run against a
  Spring-produced `*model.Model` with zero *further* code changes — the
  `DeclaresRoles` fix above is the one change this ADR already knows it needs;
  finding a second one during implementation would mean the framework-independence
  claim needs more than this ADR currently accounts for, and that gets reported
  here honestly, not patched around silently the way the first one almost was.
- `docs/limitations.md` gains the `SecurityFilterChain` blind spot as its own
  entry, parallel to the existing NestJS global-guard entry — including, once real
  Spring repos are examined, a measured statement of how much of the corpus's real
  authorization surface it actually leaves invisible (per `docs/testing.md`:
  measured, not assumed). If that number turns out to be large, that's a finding
  for a future ADR to weigh, not something to soften here in advance.
- Real Spring repository selection and hand-verified ground truth (mirroring ADR
  0005's provenance discipline for NestJS) is execution, not designed in this ADR.
