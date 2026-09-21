# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **No enumerated Spring Security mechanism is silent any more.**
  [ADR 0029](docs/decisions/0029-spring-security-scope.md) listed every mechanism
  the Spring extractor deals with and defined done as none of them being
  *present, uninterpreted, and unmentioned*. As of 2026-09-21 that holds: each is
  either read or detected and announced. It does **not** mean everything is read —
  an unreadable SpEL expression, a `RouterFunction` builder's routes and an
  external interface's mappings are announced and not understood, and
  `docs/limitations.md` says which.

- **Functional routing is announced.** WebFlux `RouterFunction` routes are built
  in code rather than declared by annotation, and were read by nothing. Measured:
  **82 builder methods across three repositories** (halo 77, shenyu 4, JeecgBoot
  1). The unit is a method whose return type is `RouterFunction` — the only
  criterion spanning all three idioms found; counting `@Bean` methods finds 14 of
  halo's 82 files, and counting `RouterFunctions.route(` calls misses its
  springdoc builder.

  The warning counts **methods, not routes**, and says so: one chain declares any
  number of routes, and a method may be a fragment composed elsewhere. halo now
  shows this alongside the existing "recognized no endpoints" notice, which is
  correct — shenyu and JeecgBoot recognize 394 and 931 endpoints and trigger only
  the new one. See [ADR 0033](docs/decisions/0033-functional-routing.md).

### Fixed

- **A `@RestController` nested inside another class is now extracted.**
  `extractControllers` walked top-level classes only, so a nested controller
  yielded no endpoints *and no controller* — invisible rather than incomplete, so
  even the "produced no routes" warning could not see it. It now walks classes at
  any depth, and guards attach as they do for a top-level controller.

  Zero corpus delta: all 33 nested controllers in the 20-repository corpus sit in
  paths already skipped as test sources, so this exists to protect production
  code. See [ADR 0034](docs/decisions/0034-nested-controllers.md).

- **Controllers whose routes were not all recovered are now announced.** The matrix
  reported `N endpoint(s)` with no way to tell whether N covered the application.
  For four corpus repositories it did not, and dataease reported **16 endpoints
  from 38 controllers** in silence.

  Two conditions, one warning. A recognized controller that yielded **no**
  endpoints; or one that implements a route-bearing interface, or an unknown
  external interface plus unmapped `@Override` handlers. Together they cover
  routes declared on an inherited interface *and* routes behind a method-level
  mapping meta-annotation, without diagnosing which — a cause-specific detector
  would cover the shapes measured today and stay silent on the next one.

  | Repository | Announced |
  |---|---|
  | dataease | 31 produced no routes, 1 produced fewer |
  | apollo | 13 produced no routes, 1 produced fewer |
  | shenyu | 2 produced no routes, 8 produced fewer |
  | eladmin | 2 produced no routes |

  The warning says routes were **not recovered**, never that routes exist — a
  controller of pure `@ExceptionHandler`s is a correct zero. It deliberately
  reports no count of the missing routes: shenyu's `PagedController` hides two
  behind classes that override one unrelated method, so any count read off the
  class is wrong.

  No endpoint, finding or export changes anywhere; the other sixteen repositories
  are byte-identical. See
  [ADR 0032](docs/decisions/0032-controllers-that-yield-no-routes.md).

- **A role hierarchy is announced, with the direction of the error.** A project
  declaring one (`@Bean RoleHierarchy`, `RoleHierarchyImpl.fromHierarchy(...)`, or
  `setRoleHierarchy(...)`) now produces a warning saying the roles shown are
  **narrower** than what the application grants: a role that implies another
  reaches every endpoint the implied one does.

  Stating the direction is the point — a bare "this project has a role hierarchy"
  leaves the reader to work out which way the numbers are wrong, and the two
  directions call for opposite responses. It is a warning and never a finding,
  because the error under-reports access: a reader acting on the matrix
  over-restricts rather than under-restricts.

  The hierarchy's **content is not parsed** and no grant is expanded — that would
  change reported access rather than annotate it, and stays a separate decision.
  See [ADR 0031](docs/decisions/0031-role-hierarchy.md). Zero corpus occurrences,
  so `sphinxor lint` is byte-identical across all 20 repositories.

- **`@PostAuthorize` is recognized — and still reported on a write.** Spring
  evaluates it *after* the handler runs, so on a `POST`/`PUT`/`PATCH`/`DELETE` the
  state change has already happened when access is denied. Spring's own docs say
  `@PostAuthorize` "is not recommended for classes that perform database writes".

  So protection is verb-dependent, a first for this model. On any endpoint it is
  recorded, marks the Guards column `?`, and is omitted from the Cerbos export. On
  a **mutating** endpoint it does **not** suppress
  `mutating-endpoint-without-access-control`, which now explains itself:

  > `POST /orders/{id}` has a `@PostAuthorize` but no guard that runs before the
  > method. `@PostAuthorize` is evaluated after the handler executes, so the state
  > change has already happened when access is denied — unless an enclosing
  > transaction rolls it back, which is not visible here.

  When the annotation is confirmed inert the ordinary message is used instead,
  since nothing evaluates it at all. The carve-out is keyed on the annotation's
  *binding*, so a project-local `@PostAuthorize` keeps its existing treatment.

- **`@PreFilter`/`@PostFilter` are announced, and authorize nothing.** Neither ever
  denies a call — a caller without the authority still invokes the handler and
  receives a shorter collection. They are counted project-wide and named in a
  warning, deliberately reaching no rule: recording them per endpoint would mark
  `?` on an endpoint nothing guards.

  See [ADR 0030](docs/decisions/0030-post-authorize-and-method-security-filters.md).
  Zero corpus occurrences, so `sphinxor lint` is byte-identical across all 20
  repositories.

- **`@EnableReactiveMethodSecurity` is now read.** Sphinxor scanned for
  `@EnableMethodSecurity` and `@EnableGlobalMethodSecurity` only, so a WebFlux
  project enabling method security the reactive way was told its annotations
  "are inert at runtime and the endpoints they appear to protect are **NOT
  protected**" — a false statement about a correctly configured application,
  and the only place the tool asserted something untrue rather than staying
  silent.

  The annotation does not resemble its servlet counterpart and has none of
  its three attributes, so the effective families were established by reading
  Spring Security's own reactive configuration classes: `@PreAuthorize` and
  `@PostAuthorize` are **enabled unconditionally**, and `@Secured` and
  `@RolesAllowed` are **not enabled on either `useAuthorizationManager`
  path**. Verified against Spring Security 6.5.11, 7.1.1 and `main`; the two
  negatives are version-bound and are re-checked on each major release.

  Consequently a `@Secured` or `@RolesAllowed` under a reactive-only
  configuration is now correctly treated as inert, and
  `mutating-endpoint-without-access-control` reports its endpoint. A project
  carrying both enablers keeps `@Secured`. The caveat text now names all
  three enablers, since a caveat about what was not found has to say what was
  looked for.

  No corpus report changes: `sphinxor lint` is byte-identical across all 20
  repositories. The one repository using the annotation, **halo**, declares
  no method-security annotations at all. See
  [ADR 0015](docs/decisions/0015-inert-method-security-guard.md) Amendment 1.

- **A handler mapping every HTTP verb is now an endpoint.** A method-level
  `@RequestMapping` with no `method` attribute routes all eight verbs in
  Spring; there are **141 across 12 repositories** in the surveyed corpus,
  and none of them appeared in the matrix.

  Each becomes **one endpoint marked `ANY`**, not eight rows. Expanding
  would have produced 504 `mutating-endpoint-without-access-control`
  findings restating 126 facts — four per handler — plus roughly a
  thousand rows of `HEAD`, `OPTIONS` and `TRACE`. Measured result: **138
  endpoints and 128 findings**.

  `ANY` counts as mutating, since the handler accepts POST, PUT, PATCH and
  DELETE. `sphinxor export cerbos` **omits** such an endpoint rather than
  writing a rule on an `any` action no request carries, and a verb-scoped
  URL-layer rule leaves it unresolved rather than granting its roles
  across seven verbs it does not cover. See
  [ADR 0028](docs/decisions/0028-verbless-request-mapping.md).


- **`@RequestMapping(method = RequestMethod.X)` now declares routes.**
  ADR 0011 cut this older form deliberately, on the grounds that no fixture
  used it. It was hiding **611 routes** across seven repositories —
  JeecgBoot 268, inlong 175, thingsboard 93, nakadi 49, microcks 17,
  nacos 6, metersphere 3 — including thingsboard's `POST /api/customer`,
  which carries a plainly readable `@PreAuthorize("hasAuthority('TENANT_ADMIN')")`
  and now appears with that role.

  `method = {GET, POST}` expands into one endpoint per verb; 40 annotations
  in the corpus declare more than one. **`HEAD`, `OPTIONS` and `TRACE` are
  added to the model**, since the corpus declares them and a declared
  endpoint is a fact; none of the three is treated as mutating, because
  that rule is about state change.

  Well over half the recovered routes needed no finding — they carry a
  Spring Security guard or a Shiro annotation already recognized. The 209
  that do arrive in projects whose URL layer this tool cannot read, and
  since ADR 0027 every one of those projects says so. See
  [ADR 0026](docs/decisions/0026-requestmapping-method-attribute.md).

  A method-level `@RequestMapping` with **no** `method` attribute maps every
  verb in Spring; those 164 uses remain out of scope, recorded as a model
  question rather than decided in passing.

### Security

- **Two more URL-authorization layers are detected and announced.** A class
  extending `WebSecurityConfigurerAdapter` (Spring Security's pre-5.7
  configuration base class) and a `@Bean` returning Apache Shiro's
  `ShiroFilterFactoryBean` were invisible: a project with one was reported
  as though it had no URL layer at all. Seven corpus projects are affected
  — nakadi, and JeecgBoot, shenyu, streampark, inlong, litemall and
  metersphere.

  Both now produce the state [ADR 0020](docs/decisions/0020-unanalyzable-is-unknown-not-absent.md)
  §2 already defined for a present-but-unanalyzed layer: `sphinxor lint`
  warns that the roles shown come from the method layer alone, and
  `sphinxor export cerbos` omits every endpoint. Nothing is parsed —
  recording that a layer exists is not interpreting it.

  Detection matches a **declaration**, never a mention of the type name. A
  project with more than one unreadable layer now names all of them in one
  warning instead of one hiding the others.

  Measured: rule counts are unchanged (all seven already exported zero,
  having no Spring Security guard to export), but 1,655 endpoints across
  six projects stop being reported as `no-guard` in the export report and
  become `url-layer-unknown`, which is the accurate reason. See
  [ADR 0027](docs/decisions/0027-unannounced-url-layers.md).

### Added

- **A Spring annotation written fully qualified is now recognized.**
  `microcks/microcks` declares its own `io.github.microcks.web.RestController`,
  so Spring's cannot be imported there; all 13 files in that package write
  `@org.springframework.web.bind.annotation.RestController`. Extraction
  matched the text as written, so none of those classes was a controller.
  microcks reported **50 endpoints** while those files held **30 more** in a
  shape already supported — roughly 40% of its API surface, and silent.

  It is the nacos collision with the sign reversed: there a matching simple
  name made a foreign annotation count, here a qualified name made Spring's
  own annotation not count. An annotation's identity is now its simple name
  however it is written, and microcks reports **80 endpoints** with 11 new
  `mutating-endpoint-without-access-control` findings and no change to any
  role or guard.

  A dotted name must match a known package **in full** —
  `@com.example.RestController` is not Spring's. A fully-qualified use of a
  method-security annotation counts as its own binding, so Spring's
  `@PreAuthorize` spelled out is a guard rather than an unrecognized
  annotation, while nacos's `@Secured` spelled out still is not. See
  [ADR 0025](docs/decisions/0025-qualified-annotation-names.md).


- **A controller declared by a meta-annotation is now recognized.** Spring
  treats an annotation that is itself annotated `@RestController` as
  composing it; extraction required the literal annotation. `apache/shenyu`
  declares `@RestApi` — `@RestController` + `@RequestMapping` with an
  `@AliasFor`'d path — on 35 of shenyu-admin's 41 controller classes,
  hiding **179 route declarations**. A run reported 192 endpoints, of which
  155 were demo applications and **11 were the real admin API**, with
  nothing in the output suggesting the rest existed.

  shenyu now reports **371 endpoints, 190 of them from shenyu-admin**. No
  other repository in the 20-project corpus changed, since none declares a
  controller-composing meta-annotation — a census of all 115 project-declared
  `@Target(TYPE)` annotations in the corpus found exactly one that composes
  a controller, which is also why a name-based rule was rejected.

  Resolution is **one level**, class-level, and reads only two things: that
  the class is a controller, and its base path (from the declaration's own
  `@RequestMapping`, or the use site via `@AliasFor`). A base path that
  cannot be read is marked unresolved rather than treated as empty. Deeper
  chains and method-level mapping meta-annotations are deliberately not
  resolved: every one measured bottoms out at `@RequestMapping(method = …)`,
  which is not read, so following them surfaces zero endpoints. See
  [ADR 0024](docs/decisions/0024-controller-meta-annotations.md).

  The 179 new routes brought **35** new `mutating-endpoint-without-access-control`
  findings rather than ~101, because ADR 0023 recognized their 100 Shiro
  annotations as soon as the endpoints existed. Those 35 are endpoints with
  no *method-level* access control in a project whose Shiro URL layer is
  unparsed — not endpoints established to be unguarded.

### Fixed

- **An Apache Shiro authorization annotation is no longer reported as no
  access control.** `@RequiresPermissions` shares no name with anything
  Spring Security uses, so it never entered any recognized set and the
  endpoint fell straight through to "nothing found". Measured across 20
  Java repositories, **907 mutating routes carried a Shiro annotation and
  were flagged as unguarded** — around a fifth of all
  `mutating-endpoint-without-access-control` findings in the Spring corpus.

  An annotation bound by import to `org.apache.shiro.authz.annotation` is
  now recorded as an unrecognized authorization annotation, the state
  [ADR 0022](docs/decisions/0022-annotation-identity-and-unrecognized-authorization.md)
  introduced: no guard, no role, `?` in the Guards column, the finding
  suppressed, and a warning naming the package and the count. **845
  findings disappeared** — metersphere 555, streampark 101, JeecgBoot 81,
  litemall 65, inlong 43 — with no other repository changing at all.

  Recognition is by import **package**, not annotation name. A name
  heuristic was measured and rejected: it catches `@IgnoreAuth`, which
  *skips* authentication, and so would suppress the finding exactly where
  it is correct.

  **Shiro is still unsupported** — nothing reads what it requires, and its
  URL layer is still unparsed. One claim was withdrawn: that these
  endpoints have no access control. The run also reports whether Shiro's
  `AuthorizationAttributeSourceAdvisor` wiring was located, so a reader
  knows what the suppression rests on. See
  [ADR 0023](docs/decisions/0023-third-party-authorization-annotations.md).

- **A Spring method-security annotation is now identified by its import, not
  just its name.** `@PreAuthorize`, `@Secured` and `@RolesAllowed` were
  matched on simple name alone, so any annotation spelled that way — from
  any package — became a Spring Security guard. `alibaba/nacos` declares its
  own `com.alibaba.nacos.auth.annotation.Secured` (108 non-test files, 428
  uses), and Sphinxor recorded **392 Spring guards** from it, suppressing
  `mutating-endpoint-without-access-control` on **242 of nacos's 245
  mutating endpoints**. Those endpoints are genuinely protected — by nacos's
  own filter — so the result was right for a reason the tool never
  established.

  A recognized name now counts only when the same file's imports bind it to
  an accepted package. Scanned across 20 real Java repositories, every use
  has such an import in its own file, so this needs no classpath.

  An annotation that fails the test is **not** discarded: it is recorded as
  an unrecognized authorization annotation, the Guards column shows `?`, and
  the run warns naming the package it actually came from and how many
  endpoints carry it. `mutating-endpoint-without-access-control` does not
  fire on those endpoints — its message would be false with an authorization
  annotation one line above the handler — so nacos stays at 3 findings
  rather than jumping to 245. ADR 0015's "these may be inert" warning also
  stops firing on annotations that were never Spring's.

  Exactly one repository in the corpus changed. See
  [ADR 0022](docs/decisions/0022-annotation-identity-and-unrecognized-authorization.md).

- **`empty-role` no longer fails the build on a role requirement Sphinxor
  could not read.** On Spring, presence and role-check are fused into one
  annotation (ADR 0011 §1), so an annotation whose content was outside the
  recognized SpEL subset — a bean call like
  `@PreAuthorize("@ss.hasPermi('system:user:edit')")`, or a same-named
  annotation from another framework such as alibaba's
  `@Secured(resource = …, action = …)` — was recorded as declaring *zero*
  roles rather than as declaring a requirement that went unread. That is
  `empty-role`'s trigger, at High confidence, which gates CI.

  A survey of 14 production Spring repositories measured **675 such
  findings across four of them** (nacos 392, RuoYi-Vue 116, eladmin 99,
  apollo 68), every one naming a permission the source states plainly. All
  four failed CI; all four now pass, with no other change to any
  repository in the corpus. The same fix cleared 5 instances from this
  project's own vendored `ruoyi-vue-pro` fixture.

  A role list that was *read and found empty* — `@Secured({})`, NestJS's
  `@Roles()` — still fires, unchanged: that is the case the rule exists
  for. `permitAll()`/`denyAll()` also still fire, deliberately, per
  ADR 0017. Affected endpoints now show `?` in the Roles column instead of
  `-`, and the run warns how many there are, because `-` claims no role is
  required while `?` says the requirement was not recovered. See
  [ADR 0020](docs/decisions/0020-unanalyzable-is-unknown-not-absent.md)
  Amendment 3.

### Security

Six blind spots, found by an audit of Spring and NestJS extraction against
real projects and by the NestJS hunt that followed it across six more
repositories (including immich and ToolJet). All of the same kind: a
construct Sphinxor could not analyze was recorded as *absent* rather than
*unknown*, and the report presented the result with confidence it hadn't
earned. See [ADR 0020](docs/decisions/0020-unanalyzable-is-unknown-not-absent.md)
and its Amendment 1.

- **A route's declared API version is now part of its identity.** NestJS's
  `@Controller({ path, version })` and `@Version()`, and Spring's `version`
  attribute on a mapping annotation, are route discriminators: two handlers
  sharing a path and differing only in version are two endpoints the running
  application routes separately. Extraction read the `path` key and stepped
  over `version`, so they collapsed onto one identity — NestJS merging them,
  so each was reported carrying the other's guards, and Spring dropping one
  outright. Found by measuring `docs/limitations.md`'s duplicate-route gap
  across 17 real repositories; hand-verified on cal.com, where one
  `GET /v2/event-types` requires authentication (`ApiAuthGuard`) and the other
  makes it optional (`OptionalApiAuthGuard`), and the merge misreported both.
  A version that cannot be read — a constant reference, or an array of them,
  which is the majority shape in real code — is treated as *unknown* rather
  than assumed equal to another unknown. An endpoint declaring no version
  keeps exactly the identity it had before, so existing allowlist anchors and
  diff baselines are untouched. See ADR 0020 Amendment 2 §7.

- **One route declared by two controllers no longer merges or disappears.** Endpoint
  identity was `(method, path)`, so two controllers declaring the same absolute route
  shared one: NestJS merged them, and an unguarded endpoint was reported carrying the
  other's guards with `mutating-endpoint-without-access-control` suppressed; Spring
  dropped one of the pair outright. Both endpoints are now kept, each with its own
  guards, and `sphinxor export cerbos` omits them, since a policy written for one
  might govern the other's traffic. Verified on `YunaiV/ruoyi-vue-pro`, where a
  runtime path prefix keyed on the Java package separates an admin API from an app
  API: `PUT /member/user/update` exists twice, one side carrying `@PreAuthorize` and
  the other no access control at all, and the unguarded side was absent from the
  report entirely. It is now reported and flagged.

  The run warns about such a collision only when the two sides' guards actually
  differ. Warning on every collision would have fired 324 times across the 17-repo
  survey behind this work, almost entirely on cases it proved harmless — a monorepo's
  per-service health check, a worker deliberately re-declaring a route. Measured
  against the implementation, the criterion fires 47 times in `ruoyi-vue-pro`, 13 in
  `eugenp/tutorials`, and not at all in immich, `shenyu`, cal.com, novu or
  `amplication`. See ADR 0020 Amendment 2 §8.

- **A project exposing a GraphQL API is now told that it was not analyzed.**
  GraphQL stays out of scope
  ([ADR 0021](docs/decisions/0021-graphql-out-of-scope-but-detected.md)), but a
  scope boundary the output never mentioned was indistinguishable from having
  nothing to report. `notiz-dev/nestjs-prisma-starter`'s entire API is 16
  resolver operations, 10 of them behind `GqlAuthGuard`; Sphinxor reported its
  two hello-world REST routes with 0 findings and no caveat, because ADR 0019
  §2's "recognized no endpoints" notice keys on zero endpoints and two is not
  zero. Resolvers are now detected and the run warns, naming how many
  operations it did not analyze. No resolver is parsed and no finding changes.
- **A comment between a decorator and what it decorates no longer deletes
  endpoints.** In tree-sitter-typescript a comment is a named sibling, so for
  shapes as ordinary as `@Post('x') // note` the decorators were attached to
  the comment instead of the handler. The handler then had no route decorator
  and stopped being an endpoint at all — no row in the matrix, and so no lint
  rule to fire on it. On a `@Controller` line it cost every route in the class
  at once. On `CatsMiaow/nestjs-project-structure` this hid an entire
  controller whose `POST`, `PUT` and `DELETE` are unguarded, reported as zero
  findings because there was nothing left to report on; that project goes from
  9 endpoints and 0 findings to 23 and 3. A comment was the whole
  difference between a false "all clear" and three real findings on a fully
  unguarded controller, out of a two-line defect — the clearest example in
  this project so far that the dangerous bugs are not the complicated ones.
- **An endpoint whose route path cannot be read no longer collides with
  another endpoint's identity.** Endpoint identity is derived from
  `(method, path)`, and extraction reads only string literals — so
  `@Controller(RouteKey.Asset)`, a route constant, an array form, a template
  literal, and Spring's `@RequestMapping(Routes.ADMIN)` all silently produced
  an empty prefix. Unrelated endpoints then shared one identity. NestJS merged
  them, reporting one endpoint's guards and roles against another: a wide-open
  `DELETE` shown as `ADMIN`-protected with the
  `mutating-endpoint-without-access-control` finding suppressed — the same
  false assurance as the `antMatcher` defect below, reached a different way.
  Spring instead dropped one of the pair, so the unguarded endpoint vanished
  from the report entirely. Such endpoints now keep an identity synthesized
  from their controller and handler, are marked in the matrix with a leading
  `…`, are named in a project-level warning, and are omitted by
  `sphinxor export cerbos`, which cannot name a policy after a fragment of a
  route. They remain fully analyzed and linted.

- **A `SecurityFilterChain` matcher whose pattern couldn't be read no longer
  matches every request.** `requestMatchers(antMatcher("/admin/**"))` — the
  idiomatic Spring Security 6 form — collapsed into the same internal state as
  `.anyRequest()`, so its `hasRole("ADMIN")` was applied to the whole
  application. On the audit's reproduction, a `DELETE /public/wipe` endpoint
  that really falls through to `permitAll()` was reported as ADMIN-protected
  with zero findings: a false assurance on a destructive endpoint, which also
  suppressed the `mutating-endpoint-without-access-control` safety net that
  would otherwise have caught it. Such a matcher now makes its rule *unknown* —
  it grants its roles to no endpoint and stops evaluation for the ones it might
  cover. `antMatcher(...)` wrappers carrying a readable pattern are now read
  properly rather than being lost this way.
- **A URL layer that exists but couldn't be analyzed is no longer treated as
  no URL layer.** A project with more than one `SecurityFilterChain` bean had
  the layer silently skipped, leaving the method layer to stand as the complete
  picture. `sphinxor export cerbos` turned that into a deployable policy
  granting `[ADMIN, ANALYST]` on an endpoint the running application restricted
  to `ADMIN`. Such a project is now reported as having an unanalyzed URL layer:
  `lint` warns that its roles may be broader than reality, and `export` omits
  every endpoint and says why rather than exporting a grant the application
  denies.
- **Reactive Spring Security (`SecurityWebFilterChain` / WebFlux) is now
  detected.** Its rules are still not parsed — that remains out of scope — but
  a reactive project no longer looks like one with no URL-layer authorization
  at all. It gets the same warning and the same export omission as the
  multi-chain case.
- **Two systematic distortions are now stated instead of implied.** A Spring
  project whose method-security annotations were found with no
  `@EnableMethodSecurity` anywhere in the analyzed source now warns that those
  annotations may be inert at runtime, in which case the roles shown for them
  are imaginary. A NestJS project registering a global guard (`APP_GUARD`
  provider or `app.useGlobalGuards()`) now warns that endpoint-level results
  understate protection, since every route is protected by default. Neither
  changes a finding; both change what the output means.

### Fixed

- `sphinxor version` no longer reports `dev` for a binary installed with
  `go install <pkg>@<version>`. `go install` doesn't apply the release
  workflow's `-ldflags`, so a freshly installed tagged release misreported
  itself on the user's first command. The version now falls back to the module
  version Go records in the build info when no `-ldflags` stamp is present —
  release binaries are unaffected, since the stamp still takes precedence. A
  local `go build` inside the repo now reports the VCS pseudo-version (with
  `+dirty` on an uncommitted tree) instead of `dev`, identifying the exact
  commit; `dev` remains for builds with no VCS information at all.

## [0.6.0] - 2026-09-18

### Added

- **Spring is now an analyzable framework.** `sphinxor lint`, `sphinxor diff`, and
  `sphinxor export cerbos` all work on Spring projects, not just NestJS:
  - Endpoints from `@RestController`/`@Controller` classes and
    `@GetMapping`/`@PostMapping`/`@PutMapping`/`@DeleteMapping`/`@PatchMapping`
    handlers.
  - Method-security guards — `@PreAuthorize`, `@Secured`, `@RolesAllowed` —
    including a bounded set of SpEL shapes (`hasRole`, `hasAnyRole`,
    `hasAuthority`, `isAuthenticated`) and roles resolved against Java enum
    constants.
  - `SecurityFilterChain` URL-layer rules (`authorizeHttpRequests`), evaluated in
    real Spring first-match-wins order.
  - **The two layers are combined into the real effective policy**, not reported
    separately: an endpoint allowing `ADMIN` or `PHARMACIST` at the method layer
    but restricted to `ADMIN` by a URL rule is reported as `ADMIN`-only. What each
    layer can and cannot be read from is bounded in
    [ADR 0012](docs/decisions/0012-securityfilterchain-effective-policy.md) and
    [ADR 0018](docs/decisions/0018-unrecognized-rule-stops-evaluation.md), with the
    current blind spots listed in [`docs/limitations.md`](docs/limitations.md).
  - `// sphinxor-allow:` markers work in Java source exactly as in TypeScript,
    including the stale-marker finding when one doesn't sit above a recognized
    endpoint.
- **Framework auto-detection, with `--framework` to override.** Sphinxor picks the
  extractor by looking at your source, and reports which one it chose and why. It
  refuses rather than guesses: if it detects nothing, or detects more than one (a
  monorepo with a Java backend and a Nest BFF), it stops and asks for
  `--framework` instead of silently analyzing half your project
  ([ADR 0019](docs/decisions/0019-cli-framework-selection.md)).

### Changed

These affect existing NestJS users too, including ones with no interest in Spring.

- **A run that couldn't examine anything is now an error instead of an empty,
  clean report.** Pointing `sphinxor lint` at a path with no recognizable source
  previously printed `0 endpoint(s), 0 finding(s)` and exited `0` — indistinguishable
  from "your authorization model is fine." It now exits non-zero with
  `no supported framework detected in <path>`. **This can turn a currently-green CI
  job red if it was pointing somewhere unintended** — which is the point, but here
  is the recourse:
  - *Wrong path* (the usual cause): correct the path.
  - *Right path, but the directory legitimately has no framework import* — a
    DTO-only or service-only sub-path, for instance: pass `--framework nestjs`
    (or `spring`). The run then proceeds and exits `0`, warning that it parsed
    files but recognized no endpoints.
  - *Framework misidentified*: pass `--framework` with the one you want.

  A run that *does* parse source but recognizes no endpoints still exits `0`, with
  a warning — a library package legitimately has no routes.
- **A one-line notice on stderr** reports the framework, how it was chosen, and the
  source-file count. `stdout` is unchanged, so `--format json` output still parses
  exactly as before and existing consumers are unaffected.
- **Errors print once, without a usage dump.** A failed run shows the reason, not
  the command's full flag list.

## [0.5.0] - 2026-08-28

### Added

- **"Authenticated, any role" grants** ([ADR 0010](docs/decisions/0010-authenticated-any-role.md)):
  the model now has a positive `AuthenticationRequirement` fact for an
  endpoint guarded by a recognized authentication guard (`AuthGuard`) with
  no specific role resolved — previously indistinguishable from "extraction
  found nothing." `sphinxor export cerbos` exports these as Cerbos's
  documented `*` role grant (confirmed behaviorally, not just from docs),
  raising real coverage on the two vendored repos from 6 to 9 rules (7 to
  10 of 24 endpoints) without guessing: the recognized-guard-name set is
  small and evidence-based, deliberately excludes the literal-empty-`@Roles()`
  case `empty-role` already flags as a probable mistake, and an endpoint
  sharing a Cerbos action with a differently-guarded sibling still collides
  exactly as before.

### Fixed

- `sphinxor export cerbos --format json`'s companion report no longer
  collides with the generated policy directory — it's written as
  `<out>.report.json`, a sibling of `--out`, not nested inside it (a real
  `cerbos compile` against `--out` used to fail on the report file itself).

## [0.4.0] - 2026-08-27

### Added

- **`sphinxor export cerbos <dir> --out <policy-dir>`** — the first
  authorization-engine exporter ([ADR 0009](docs/decisions/0009-cerbos-exporter.md)):
  translates the normalized model into a Cerbos resource policy set.
  Downstream of extraction by construction (works for any future
  framework automatically). Output is explicitly not deploy-ready:
  a rule is only generated when a real `GuardApplication`/`RoleReference`
  establishes it; anything the model can't confirm (no guard, guarded
  with no specific role, or two endpoints whose confirmed roles disagree
  under the controller+method action mapping) is omitted, never guessed,
  and flagged both inline in the generated YAML and in a companion
  `export-report.md`/`.json`. Validated against the two vendored NestJS
  repos with the real `cerbos compile` CLI, not just structurally.
- `sphinxor version` (and `--version`), reporting the release tag on a
  release build, `dev` on a local build.
- CI: `make check` (build, vet, `gofmt -l`, tests) on every push and pull
  request; a release workflow building the `sphinxor` binary on tag push.
- `SECURITY.md`: private vulnerability disclosure process.

## [0.3.0] - 2026-08-03

### Added

- **`sphinxor diff <base-dir> <head-dir>`** — v1's headline differentiator
  (`docs/vision.md`, [ADR 0007](docs/decisions/0007-model-diff-design.md)):
  drift detection between two versions of a project's authorization model,
  not just a point-in-time audit.
  - Structural diff, always reported: added/removed endpoints, added/removed
    role declarations, added/removed guard applications and role references,
    and endpoints that became public.
  - Regression detection that gates CI: a new `High`-confidence finding, or a
    `High`-confidence finding that lost its `sphinxor-allow` exemption between
    the two sides. `Low`-confidence findings and unchanged pre-existing
    `High` findings never gate, on either side of the comparison.
  - Takes two pre-extracted directories rather than git refs — Sphinxor never
    shells out to `git`; see the README for the `git worktree add` CI pattern.
  - Markdown and JSON output, same convention as `sphinxor lint`.

## [0.2.0] - 2026-07-31

### Added

- Composite decorator resolution (`@Auth(roles)`-style wrappers around
  `applyDecorators()`), one level deep — [ADR 0006](docs/decisions/0006-composite-decorator-resolution.md).
  Closes the false-positive gap on endpoints protected only through such a
  wrapper, within the bounded shape the ADR describes; anything outside
  that shape remains a documented blind spot (`docs/limitations.md`).

## [0.1.0] - 2026-07-31

### Added

- NestJS extraction: controllers, endpoints, `@UseGuards`/role decorators,
  declared roles and permissions, normalized into the intermediate model
  ([ADR 0002](docs/decisions/0002-intermediate-model-structure.md)).
- The three v0.1 lint rules: mutating endpoint (`POST`/`PUT`/`PATCH`/`DELETE`)
  with no detected access control; permission declared but never referenced;
  empty role.
- The `sphinxor-allow` comment-marker allowlist mechanism, plus the
  stale-allow-marker finding for a marker that doesn't sit above a
  recognized endpoint — [ADR 0003](docs/decisions/0003-allowlist-format.md).
- Two-tier `High`/`Low` finding confidence, with `High` gating CI —
  [ADR 0004](docs/decisions/0004-confidence-level-granularity.md).
- `sphinxor lint`: extraction + the rule set + the RBAC matrix report
  (Markdown/JSON).
- Test corpus expanded to a second real NestJS repository with a different
  guard style, validating extraction against more than one project's
  conventions ([ADR 0005](docs/decisions/0005-test-fixture-provenance.md)).
- Initial documentation structure (`docs/`, `CONTRIBUTING.md`, `CHANGELOG.md`).
