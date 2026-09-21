# Known limitations

`vision.md` commits Sphinxor to heuristic, confidence-graded analysis, not formal verification — and to owning that openly rather than promising completeness general-purpose SAST tools already failed to deliver. This file is where that commitment gets kept concretely: real gaps in what the current static analysis can see, found empirically against real code, not hypothesized in advance.

This is not the roadmap. `roadmap-long-term.md` is about what's planned; this is about what the current release honestly cannot see, whether or not fixing it is ever planned. An entry here can outlive several roadmap cycles without becoming wrong.

## Global guards (`APP_GUARD` providers, `app.useGlobalGuards()`)

Sphinxor does not parse NestJS module provider wiring. A guard registered globally — via an `APP_GUARD`-token provider in a module, or via `app.useGlobalGuards()` in `main.ts` — protects every endpoint in the application without any decorator appearing at the endpoint or its controller. This extractor only sees decorators, so it cannot see *what* that guard requires.

Since [ADR 0020](decisions/0020-unanalyzable-is-unknown-not-absent.md) §4 it does see *that* one is registered, and says so. Both forms are detected, and a run on such a project opens with a warning that endpoint-level results understate protection across the board. Verified against `nestjs/nest`'s own `19-auth-jwt` sample, where the recommended pattern — a global guard with `@Public()` opting out — inverts the default this extractor assumes and made every endpoint report as unguarded.

**Consequence**: an endpoint protected exclusively by a global guard is still flagged by `mutating-endpoint-without-access-control`, at Low confidence — the rule's Low grade exists specifically because of this gap (see `internal/lint/mutating_endpoint.go`) — but the report no longer presents that flag without saying the whole project's results lean that way. The error direction was always the safe one; it being unsignalled was not.

**What to do about it today**: mark the affected endpoint(s) with a `// sphinxor-allow: <reason>` comment (`docs/decisions/0003-allowlist-format.md`), same as any other endpoint the tool gets wrong for a reason a human can verify.

## Composite decorators built with `applyDecorators()` — narrowed, not closed

Confirmed common in real code, not a hypothetical edge case: a project can define its own decorator (e.g. `@Auth(roles)`) that internally calls NestJS's `applyDecorators()` to bundle `UseGuards(...)`, `Roles(...)`, and other decorators together into one. Found and hand-verified on [`NarHakobyan/awesome-nest-boilerplate`](https://github.com/NarHakobyan/awesome-nest-boilerplate): `POST /posts` is genuinely guarded via `@Auth([RoleType.USER])`, confirmed no global guard was doing the protection instead.

As of [ADR 0006](decisions/0006-composite-decorator-resolution.md), extraction follows **one level** of this indirection: a composite matching a specific, bounded shape (a single, unconditional return path calling `applyDecorators(...)`, undestructured parameters, direct pass-through argument substitution) is resolved, and `POST /posts` above no longer produces a false positive. What's still invisible, deliberately, per that ADR's stated non-goals:

- **Multi-level composite chains** — a composite calling another composite that calls `applyDecorators(...)`.
- **Composites built by assembling an array imperatively**, rather than by a single flat `applyDecorators(...)` call. Confirmed on [`immich-app/immich`](https://github.com/immich-app/immich), which authorizes with `@Authenticated({ permission: Permission.AssetUpdate })`: the decorator pushes onto a `MethodDecorator[]` through a series of `if` branches, and enforcement happens in a global guard reading the metadata it sets. Extraction recognizes none of it, so a 302-endpoint production application reports 170 findings, every one a Low-confidence false positive in the safe direction, with no permission extracted. The [ADR 0020](decisions/0020-unanalyzable-is-unknown-not-absent.md) §4 global-guard warning does fire on it, so the run says endpoint-level results understate protection.

  **Measured, and deliberately not fixed.** A survey of 11 production repositories found this shape in exactly one other project (`amplication`, on its GraphQL side), and found something more decisive about immich itself: the words `UseGuards` and `Roles(` do not occur **anywhere** in its source tree (re-measured at a later upstream commit than the figures above: 444 files, 303 endpoints, 171 findings — the drift is upstream's, not a correction). Since ADR 0006 extracts only nested calls by those two names, a perfect array-push extension would walk immich's arrays and build zero guards. The shape is not what hides immich's authorization — see *Permissions as metadata* below, which is.
- **Conditional or branching decorator construction** — a composite with more than one `return` path (e.g. `if (...) return SkipAuth(); return applyDecorators(...)`).
- **Destructured parameters** — `function Auth({ roles }: { roles: RoleType[] })` rather than a plain `roles` parameter.
- **Non-trivial dataflow** — a parameter transformed before being passed to the inner `Roles`/`UseGuards` call (e.g. `Roles(roles.map(...))`), or passed via spread (`Roles(...roles)`).

A composite outside this bounded shape isn't guessed at — it falls back to exactly the behavior described above (invisible, Low-confidence flag on the endpoint using it), the same honest default as before ADR 0006, not a new failure mode.

**Consequence for what's still invisible**: same as the global-guard case — a Low-confidence, hedged false positive rather than a confident wrong claim. A connected, secondary consequence: a role enum referenced *only* through a wrapped decorator outside this bounded shape is invisible to the role-declaration usage filter (`internal/extract/nestjs/roles.go`), so `permission-declared-but-unreferenced` and `empty-role` cannot fire on those roles either.

**One item that was on this list by accident, and has been removed.** A **rest parameter in the composite's own signature** — `function RequiresScope(...requiredScopes: Scope[])` — used to disqualify the whole composite, not by decision but because tree-sitter models it as a `required_parameter` whose `pattern` is a `rest_pattern`, so it failed the check written to exclude *destructuring*. `ghostfolio/ghostfolio` declares precisely ADR 0006's bounded shape this way, wrapping `UseGuards(AuthGuard('jwt'), HasPermissionGuard, ImpersonationGuard, ScopeGuard)`, and none of it resolved; the cause was isolated to the single `...`. Fixed under [ADR 0006](decisions/0006-composite-decorator-resolution.md) Amendment 1: ghostfolio's guarded endpoints rise 78 → 105 and its `mutating-endpoint-without-access-control` findings fall 13 → 0, with no change to any other repository in the corpus. What that amendment still does not do, now stated rather than incidental, is resolve a rest parameter's *value*: an inner `Roles(roles)` referencing it resolves to nothing rather than to a wrong subset.

**What to do about it today**: for anything outside the resolved shape, same as above — `sphinxor-allow` on endpoints known to be protected this way. There is no plan to extend beyond one level of indirection or direct pass-through substitution in v0.1; whether it's worth the extraction complexity in a later version is an open question, not a commitment made here.

## Permissions as metadata: the model has no concept for how production code actually authorizes

This is the largest gap recorded in this file, and it is **not an extraction gap**. Extending composite-decorator resolution cannot close it at any depth. It is a question about [ADR 0002](decisions/0002-intermediate-model-structure.md)'s model, and it is deliberately left open here rather than answered — **no ADR closes it, and nothing below proposes one.** [ADR 0020](decisions/0020-unanalyzable-is-unknown-not-absent.md) Amendment 3, referenced under *Consequence today*, fixed how one consequence of this gap was *reported*; it did not make the tool understand a permission, and it was never meant to.

It was first found on NestJS and recorded here as a NestJS finding, with whether it generalized left explicitly unanswered. It has since been surveyed on Spring as well. **It generalizes**: the same model gap, under syntax that looks nothing alike. Both surveys are below, along with the one place the two frameworks genuinely differ, which is not smoothed over.

### The number, in both frameworks

A survey of 11 production NestJS repositories — 2,435 endpoints:

> **The model records zero roles, on zero endpoints, in all 11 repositories.**

A later survey of 14 production Spring repositories — 2,959 endpoints:

> **The model records zero roles, on zero endpoints, in 13 of the 14.**

Not "few". None — literally zero role-bearing endpoints in 11 of 11 NestJS repositories and in 13 of 14 Spring ones, the fourteenth being the exception discussed below. Each survey has a positive control run by the same binary in the same session: the vendored `awesome-nest-boilerplate` fixture reports three role-carrying endpoints, and the vendored `Pharmacy` fixture reports 8 endpoints, 7 of them role-carrying, across 2 distinct roles. The zeros are facts about the corpora, not artifacts of a harness.

### Why — NestJS

Production NestJS does not authorize with `@UseGuards(RolesGuard)` + `@Roles(Role.Admin)`, which is the pair this extractor is built to read. It declares a **requirement as metadata** and lets a globally registered guard enforce it. The decorator does not say *"this guard protects this endpoint"*; it says *"this endpoint requires permission X"* — and the model has no field that means that.

| Repository | How authorization is declared | Uses | What the model records |
|---|---|---:|---|
| `immich-app/immich` | `@Authenticated({ permission: Permission.AssetUpdate })` → `SetMetadata` | 292 | nothing |
| `vendure-ecommerce/vendure` | `@Allow(Permission.ReadProduct)` → `SetMetadata` | 363 | nothing |
| `teableio/teable` | `@Permissions(Action.TableRead)` → `SetMetadata` | 296 | `PermissionGuard` |
| `nocodb/nocodb` | `@Acl(name, { allowedRoles })` → hand-written `MethodDecorator` | 279 | `GlobalGuard` + rate limiters |
| `ToolJet/ToolJet` | `@InitFeature(featureId)` → `SetMetadata` | 73 | `FeatureAbilityGuard` |
| `calcom/cal.com` | `@Permissions([...])` via `Reflector.createDecorator()` | 54 | `PermissionsGuard` |
| `ghostfolio/ghostfolio` | `@RequiresScope(...scopes)` → `SetMetadata` + `UseGuards` | 33 | the four guard names, not the scopes |
| `amplication/amplication` | `AUTHORIZE_CONTEXT` `requiredPermissions` → `SetMetadata` | 157 | nothing |

Three things this table is saying, each of which matters on its own:

- **Only two of the eight route through `applyDecorators` at all.** The others are a bare `SetMetadata` arrow, a hand-written `(target, key, descriptor) => {…}`, or NestJS's own `Reflector.createDecorator()` factory. Composite-decorator resolution cannot reach most of them by construction — it is the wrong instrument, not an insufficiently powerful one.
- **A high guard count is not understanding.** `nocodb` reports guards on 339 of 344 endpoints, which looks like near-total coverage; those guards are `GlobalGuard` and three rate limiters applied at class level, while all 279 `@Acl` permissions — *including their `allowedRoles` lists* — are invisible. `teable`, `ToolJet`, `cal.com` and `ghostfolio` are the same story: the enforcer is visible, the requirement it enforces is not.
- **Extraction is the easy half.** immich's `SetMetadata(MetadataKey.AuthRoute, options)` passes the composite's bare parameter, so ADR 0006's existing positional pass-through would already hand back the call site's `{ permission: Permission.AssetUpdate }` literal. The value is mechanically reachable today. There is nowhere in the model to put it: `GuardApplication{GuardName}` plus `RoleReference{RawLiteral}` has no slot for a permission, and immich's option object carries `permission`, `admin`, `sharedLink`, `public` and `setup`, of which only `admin` is even role-shaped.

### Why — Spring: the same finding, under syntax that looks nothing alike

Spring's version of "requirement as metadata" is a permission string in an annotation the extractor either doesn't recognize or recognizes without being able to read, enforced by a Shiro realm, a Spring bean, or the application's own filter.

| Repository | How authorization is declared | Uses | What the model records |
|---|---|---:|---|
| `thingsboard/thingsboard` | `@PreAuthorize("hasAnyAuthority('TENANT_ADMIN','CUSTOMER_USER')")` | 536 | **439 endpoints, 5 roles** — works |
| `alibaba/nacos` | `@Secured(resource=…, action=ActionTypes.WRITE)` — alibaba's own annotation, not Spring's | 419 | 392 unrecognized annotations, no guard, zero roles |
| `jeecgboot/JeecgBoot` | Shiro `@RequiresPermissions("airag:knowledge:add")`, `@RequiresRoles("admin")` | 223 + 27 | recorded as unrecognized (ADR 0023); no permission read |
| `apolloconfig/apollo` | `@PreAuthorize(value = "@unifiedPermissionValidator.hasCreateNamespacePermission(#appId)")` | 140 | 68 guards, zero roles |
| `yangzongzhuan/RuoYi-Vue` | `@PreAuthorize("@ss.hasPermi('system:user:edit')")` | 116 | 116 guards, zero roles |
| `apache/streampark` | Shiro `@RequiresPermissions("yarnQueue:create")` | 101 | recorded as unrecognized (ADR 0023); no permission read |
| `apache/shenyu` | Shiro `@RequiresPermissions("system:pluginHandler:edit")` | 100 | nothing — all 100 sit in controllers extraction never recognizes, so even ADR 0023 cannot see them |
| `elunez/eladmin` | `@PreAuthorize("@el.check('deploy:edit')")` | 99 | 99 guards, zero roles |
| `spring-cloud/spring-cloud-dataflow` | YAML: `- POST /apps => hasRole('ROLE_CREATE')` | 66 | nothing — it is not a `.java` file |
| `apache/fineract` | in-handler `context.authenticatedUser().validateHasReadPermission("LOAN")` | 389 | nothing — and only 1 of its ~966 routes is even seen (JAX-RS) |
| `apache/dolphinscheduler` | in-handler/service `canOperator(...)`, `resourcePermissionCheckService.*` | 60 | nothing |
| `zalando/nakadi` | legacy `WebSecurityConfigurerAdapter` + `.access(hasScope('nakadi.event_stream.read'))` | 23 | nothing |
| `microcks/microcks` | URL layer `requestMatchers(…).hasAnyRole(ROLE_ADMIN)` + in-handler `authorizationChecker.hasRole(…)` | 12 + 22 | nothing |
| `conductor-oss/conductor` | none — the OSS build ships no endpoint authorization | 0 | nothing, correctly |

Three things this table is saying:

- **It is the same gap, not an analogous one.** `system:user:edit`, `yarnQueue:create`, and `resource=…, action=WRITE` are immich's `@Authenticated({ permission: Permission.AssetUpdate })` in Java. A requirement is named at the endpoint; something the extractor does not read enforces it; `GuardApplication{GuardName}` + `RoleReference{RawLiteral}` has no field that means "requires permission P". Counted across the Spring corpus: **1,133 permission declarations with nowhere in the model to go** — 416 Shiro permission literals, 27 Shiro role literals, 205 permission literals inside bean-call SpEL, 419 nacos `resource`/`action` pairs, 66 SCDF YAML rules.
- **Extraction is the easy half here too.** `@ss.hasPermi('system:user:edit')` and `@RequiresPermissions("system:pluginHandler:edit")` are quoted literals in fixed positions — a dozen lines of pattern-matching each, no dataflow. What stops them is the absence of a slot, exactly as on the NestJS side. The one Spring shape where extraction is *not* the easy half is apollo's: its 140 bean calls carry no literal at all, the requirement being encoded in the method name and its arguments (`hasCreateNamespacePermission(#appId)`), which is also ABAC rather than RBAC.
- **A guard count is not understanding, again.** RuoYi-Vue, eladmin and apollo each report a guard on most of their endpoints — 116, 99 and 68 respectively — and a role on none. The enforcer is visible, the requirement it enforces is not. nacos used to be the extreme version of this, reporting a guard on 392 of its 422 endpoints; since [ADR 0022](decisions/0022-annotation-identity-and-unrecognized-authorization.md) it reports those as *unrecognized annotations* rather than as guards, because they are not Spring's at all — see the entry below.

**The URL layer contributed roles in zero of the fourteen.** Nine projects had a `SecurityFilterChain` that [ADR 0020](decisions/0020-unanalyzable-is-unknown-not-absent.md) §2 correctly suppressed and warned about: six for declaring more than one chain (apollo, fineract, microcks, nacos, `spring-cloud-dataflow`, thingsboard), two for a single chain whose rules would not parse (dolphinscheduler, eladmin), and one for being reactive (JeecgBoot). Four had no chain this extractor recognizes at all, and warn about nothing — conductor, plus nakadi's pre-5.7 `WebSecurityConfigurerAdapter` and Shiro's `ShiroFilterFactoryBean` in shenyu and streampark. The fourteenth, RuoYi-Vue, is the one URL-layer result in the corpus that is right rather than absent: exactly one chain, parsed correctly, and it genuinely grants no roles (`permitAll` plus `anyRequest().authenticated()`).

### The asymmetry: Spring found one repository where the model works, and NestJS found none

This is the one place the two surveys disagree, and it is not flattened into "both are equally broken".

`thingsboard/thingsboard` records **439 role-carrying endpoints across 5 roles**. All 15 endpoints it reports without roles carry no method-security annotation in source either: 13 are explicitly public by route (`/api/noauth/*`, `/.well-known/*`, `/api/images/public/*`) and the other two are an OAuth2 redirect callback and a readiness probe, both unauthenticated by construction. The empty cells are correct, not missing. On that repository the model does the job it was designed for, end to end.

So `hasRole`/`hasAuthority` is **a live idiom that real production code still uses**, and ADR 0011 §1's deliberately narrow SpEL subset is a real, load-bearing capability rather than a bet that missed. The NestJS zero was a *total mismatch* — no surveyed repository authorized the way the extractor reads, and `@UseGuards` + `@Roles` behaved like a dead idiom. The Spring zero is a *distribution problem*: the idiom the extractor reads is alive, and most of this corpus happens not to use it. Those are different findings and they license different responses, so they are recorded as different findings.

### What the Spring corpus does not prove

Two caveats, both material:

- **Four of the fourteen share a lineage.** `RuoYi-Vue`, `eladmin`, `JeecgBoot` and arguably `nacos` come from an overlapping Chinese admin-framework tradition and may not be independent samples. What keeps the finding standing is that the same permission-string idiom appears independently outside that tradition: via Apache Shiro in two Apache projects (`shenyu`, `streampark`), and via YAML in `spring-cloud-dataflow`.
- **Spring's zeros are confounded in a way NestJS's were not.** Several of them are caused by route shapes extraction does not recognize rather than by the model gap — see *Spring route shapes outside ADR 0011 §1's scope* below. Those are **separable problems**: fixing endpoint discovery would surface more endpoints without recording a single additional role, and closing the model gap would record permissions on endpoints extraction already sees. Neither subsumes the other, and this entry claims only the second.

And one result that is not a gap at all: **`conductor-oss/conductor`'s zero is a true negative, verified rather than assumed.** Its OSS build ships no endpoint authorization — one `SecurityContextHolder` read in a scheduler service, no filter chain, no security annotations anywhere in main source. Reporting no roles for it is correct. Not every zero in this corpus is a failure to see something.

### Why an extraction answer could not be complete anyway

Even granted a model concept, the link from a metadata key to the guard that enforces it is **wiring, not annotation syntax**. immich registers two different `APP_GUARD` providers in two different modules — `AuthGuard` for the API, `MaintenanceAuthGuard` for the maintenance worker — and **both read the same key**, `MetadataKey.AuthRoute`. The same decorator on the same handler therefore means two different things depending on which module mounted the controller. No decorator-level analysis resolves that, and this extractor does not parse module provider wiring at all (see the global-guard entry above).

Spring reproduces the same problem with different plumbing: nacos's `signType` attribute selects which enforcer interprets a `@Secured`, SCDF's method-and-path-keyed rules live in a YAML file the extractor never opens, and Shiro's realm is configured entirely outside Spring Security. In every case the requirement and its enforcer are declared in different places, and only one of them is an annotation.

So any future answer has at least three parts, and only the first is extraction: read the requirement; represent it in the model; and know, or honestly refuse to know, which guard enforces it.

### Consequence today

Every endpoint in every one of these projects is reported with an empty `Roles` column. On NestJS the mutating ones among them are flagged by `mutating-endpoint-without-access-control` at Low confidence — the safe direction, and the [ADR 0020](decisions/0020-unanalyzable-is-unknown-not-absent.md) §4 global-guard warning fires on the projects that register one, so a run does not present these results as a complete picture.

**On Spring this gap used to do something worse than under-report — it broke the build, and that is fixed.** ADR 0011 §1 fuses guard and role-carrier — `@PreAuthorize`/`@Secured` is simultaneously "this endpoint is protected" and "this is what it requires" — so a recognized-but-unresolved annotation was recorded with `DeclaresRoles: true` and zero `RoleReference`s, which is precisely `empty-role`'s trigger, at **High** confidence, which gates CI. The survey measured **675 High-confidence findings across four repositories** (nacos 392, RuoYi-Vue 116, eladmin 99, apollo 68), every one invented:

```
@PreAuthorize() on GET /monitor/cache declares no roles      # really: @ss.hasPermi('monitor:cache:list')
@PreAuthorize() on GET /api/logs/download declares no roles  # really: @el.check('logs:list')
@Secured() on POST / declares no roles                       # really: resource=…, action=WRITE
```

Since [ADR 0020](decisions/0020-unanalyzable-is-unknown-not-absent.md) Amendment 3, a role list that could not be *read* is distinguished from one the source declares *empty*. `empty-role` fires only on the latter — `@Secured({})`, NestJS's `@Roles()` — so all 675 are gone and all four projects exit zero. What replaces them is a warning naming the count and a `?` in the Roles cell, because suppressing the finding without saying anything would leave the same absent-looking output that ADR 0020 exists to stop. A `-` there would claim no role is required; `?` says the requirement exists and was not recovered.

The gap itself is unchanged, and that is the point: the tool now under-reports honestly instead of accusing the code of something the file in front of you disproves. The same fix removed 5 instances of that false positive from this repository's own vendored `ruoyi-vue-pro` fixture, where they had sat unnoticed since it was added for an unrelated decision.

But the scale should be stated plainly for both: on a production application in either framework, the RBAC matrix this tool exists to produce currently has **no role data in it at all** — with `thingsboard` as the single surveyed exception. Amendment 3 changed how honestly that emptiness is presented, not how empty it is.

**What to do about it today**: nothing at the endpoint level recovers the permission — `sphinxor-allow` marks an endpoint as reviewed, it does not read what the endpoint requires. Treat the matrix for such a project as a route inventory with authentication hints, not as an authorization model.

**Status**: open, and framed here rather than decided. Closing it means amending ADR 0002 to carry a requirement that is neither a guard nor a role. That amendment has not been written. What the Spring survey changes is the scope of the question, not its answer: it is now known to be a model-level gap that surfaces in both supported frameworks, not a NestJS-specific one.

## Spring route shapes outside ADR 0011 §1's scope — and the recognized-endpoint count is not the API surface

Found by the same Spring survey as the entry above, and recorded separately because it is a different gap: this is about **which endpoints exist at all**, not about what authorizes them. Fixing it would surface more endpoints without recording one additional role.

[ADR 0011](decisions/0011-spring-second-framework.md) §1 recognizes `@RestController`/`@Controller` classes — literally, or composed by a project-declared annotation since [ADR 0024](decisions/0024-controller-meta-annotations.md) — and `@GetMapping`/`@PostMapping`/`@PutMapping`/`@DeleteMapping`/`@PatchMapping` methods. Everything below is real production Spring that declares routes some other way. The measured consequence is that a run reports a matrix that looks complete and is not.

**`@RequestMapping(method = …)` is now the load-bearing one**, though not in the way this entry first put it. It is a **necessary but not sufficient** condition for three things: the 415 method-level declarations below, `eladmin`'s `@Anonymous*Mapping`, and `shenyu`'s `@Shenyu*Mapping`.

For the two meta-annotation families, reading `@RequestMapping(method = …)` alone would change nothing — verified: **zero** handlers carrying an `@Anonymous*Mapping` also carry a literal `@RequestMapping`. The handler carries only the meta-annotation, and [ADR 0024](decisions/0024-controller-meta-annotations.md) §1 resolves meta-annotations at the class level only. Each of those families needs **both** gaps closed: this one, and method-level meta-annotation resolution (plus depth 2 for shenyu's).

- **A controller-level meta-annotation — fixed.** `apache/shenyu`'s `@RestApi` composes `@RestController` + `@RequestMapping` with `@AliasFor`, and sits on 35 of shenyu-admin's 41 controller classes. It hid **179 route declarations and all 100 `@RequiresPermissions`**, silently: the run reported 192 endpoints, of which 155 were `shenyu-examples/` demo applications and **11 were the real admin API**. Since [ADR 0024](decisions/0024-controller-meta-annotations.md) a class carrying an annotation that composes `@RestController` is a controller, resolved one level, and shenyu reports 371 endpoints with 190 from shenyu-admin. What remains deliberately unresolved is a **two-level** chain and a **method-level** mapping meta-annotation — both measured to surface zero endpoints, because they bottom out at `@RequestMapping(method = …)`, the next bullet.
- **Routes declared on an inherited interface — announced since [ADR 0032](decisions/0032-controllers-that-yield-no-routes.md).** The mappings live on an interface and the handlers are `@Override` methods, so the class carries no mapping annotation this extractor reads. Measured across the corpus:

  | Repository | Controllers affected | Interfaces | Authorization lost |
  |---|---|---|---|
  | `dataease` | **31** produce no routes, 1 produces fewer | **32 of 33 in-repo** | 97 `@DePermit`, project-local |
  | `apolloconfig/apollo` | 13 produce no routes, 1 produces fewer | external artifact | **75 `@PreAuthorize`** |
  | `apache/shenyu` | 8 produce fewer | in-repo `PagedController` | none |

  apollo's 75 breaks down as 46 on the 13 controllers that vanish entirely and 29 on `PortalManagementController`, which does **not** vanish: it has 47 `@Override` methods, 9 with an inline mapping and 38 without, so it reports 9 endpoints and looks complete. *This entry previously said "14 controllers, 72 `@PreAuthorize`", which no measurement reproduces; 46 + 29 = 75 across 14 controllers is what the extractor and the source actually show.*

  shenyu's is a different shape again: `PagedController` declares two `@PostMapping`s as **`default` methods**, so its eight implementors each serve two routes without overriding anything. **16 hidden routes behind classes that look ordinary.**

  `dataease` was not recorded here at all before ADR 0032, and is the largest instance in the corpus.
- **`@RequestMapping(method = RequestMethod.X)` — fixed.** ADR 0011 §1 cut this shape deliberately, on the grounds that no fixture used it. It turned out to hide **611 routes** across seven repositories: JeecgBoot 268, inlong 175, thingsboard 93, nakadi 49, microcks 17, nacos 6, metersphere 3. It also meant **the one repository where the model works was only partly seen** — thingsboard mixes both shapes, so `POST /api/customer`, carrying a plainly readable `@PreAuthorize("hasAuthority('TENANT_ADMIN')")`, was absent from its model entirely. Read since [ADR 0026](decisions/0026-requestmapping-method-attribute.md), including `method = {GET, POST}` expanding into one endpoint per verb, and `HEAD`/`OPTIONS`/`TRACE` added to the model since the corpus declares them.

  *This entry previously carried 381, then 415/593, then 429/588, and all three were estimates from regex scans.* 381 counted meta-annotation declarations as route declarations and did not expand multi-verb lists; 415/593 predated [ADR 0025](decisions/0025-qualified-annotation-names.md) and did not require method-level position; 429/588 used a fixed lookahead window to decide what was method-level. **611 is what the extractor actually recovers**, and is measured rather than estimated. Of the routes recovered, well over half needed no finding: they carry a Spring Security guard or an Apache Shiro annotation that [ADR 0023](decisions/0023-third-party-authorization-annotations.md) already recognizes.

  **What the 209 new findings mean, and do not.** Every repository gaining findings here has a URL layer this extractor cannot read, and since [ADR 0027](decisions/0027-unannounced-url-layers.md) every one of them says so. `inlong`'s was read by hand: its Shiro chain ends in a `/**` catch-all applying an `AuthenticationFilter` and a tenant filter, and **114 of its mutating routes fall under it with zero `anon`**. These are endpoints with no *per-endpoint authorization* in applications that authenticate by default — not endpoints anyone can call.

- **A method-level `@RequestMapping` with no `method` attribute — fixed.** In Spring each maps *every* HTTP verb. Measured with tree-sitter (a windowed regex had said 164 across 17 repositories, counting class-level annotations as method-level): **141 method-level across 12 repositories**, alongside 735 class-level ones that are path prefixes and correctly produce nothing. Since [ADR 0028](decisions/0028-verbless-request-mapping.md) each is **one endpoint marked `ANY`**, not eight rows: expanding would have produced 504 `mutating-endpoint-without-access-control` findings restating 126 facts, four per handler, plus roughly a thousand matrix rows of `HEAD`, `OPTIONS` and `TRACE`. Six consumers were enumerated before the model term was added; the two non-obvious ones are the Cerbos exporter, which omits such an endpoint rather than writing a rule on an `any` action no request carries, and URL-layer rule matching, where a verb-scoped rule covers one of eight verbs and so resolves to unknown per [ADR 0018](decisions/0018-unrecognized-rule-stops-evaluation.md).

  **What an `ANY` row over-states, and what it does not.** Where a path carries both a verb-less mapping and a verb-specific one — `spring-cloud-dataflow`'s `TaskSchedulerController` is the corpus's only instance, with `DELETE` going to one handler and every other verb to the other — the model records two endpoints and does not compute the complement. So the `ANY` row claims to cover `DELETE` when at that path it does not. That over-states *which verbs the row covers*, never *what protects them*: each handler's guards attach to its own row, so neither endpoint can report the other's protection.

- **A controller declared as a nested class.** `extractControllers` walks top-level `class_declaration` nodes only, so a `@RestController` nested inside another class contributes no endpoints at all — and because it yields no *controller* either, [ADR 0032](decisions/0032-controllers-that-yield-no-routes.md)'s detection does not see it. *This entry previously claimed the corpus contains no instance. It contains **33**, across apollo, conductor, nacos, shenyu, spring-cloud-dataflow and halo — all of them in test sources.* Production instances: zero. Still silent.
- **JAX-RS in a Spring Boot application.** `apache/fineract` is a Spring Boot application that declares its routes with `@Path` plus JAX-RS verb annotations — 966 of them. Extraction recognizes **one endpoint**, and its 389 in-handler `validateHasReadPermission("LOAN")`-style checks go with the endpoints that never appeared.
- **Functional routing (`RouterFunction`).** Spring WebFlux's alternative to an annotated controller: routes are built in a `@Bean` method with `route(GET("/x"), handler::method)` rather than declared by annotation. Extraction reads none of it. Three corpus repositories use it — **halo** (151 files), **apache/shenyu** (6) and **JeecgBoot** (1) — and shenyu's are real routes, not scaffolding: `POST /helloWorld2`, `GET /rewrite`, `GET /pdm`, `GET /oms`, `GET /timeout`.

  **halo makes this look announced, and it is not.** halo yields zero endpoints from 1,001 parsed files, so ADR 0019 §2's notice fires and the run does say something. But that notice tests `len(m.Endpoints) == 0` across the whole project — it reports *emptiness*, not an unread route shape. shenyu and JeecgBoot, which recognize 394 and 931 endpoints respectively, get no warning at all and lose their functional routes in silence. Verified directly: adding a single annotated controller to a `RouterFunction`-only project removes the warning while the functional routes stay invisible. This is Spring's own routing rather than another framework, so unlike the JAX-RS entry above it is a gap and not a non-goal.

- **Method-level meta-annotations over `@RequestMapping`.** `elunez/eladmin`'s `@AnonymousGetMapping` and siblings (5 declarations, 11 uses) and `apache/shenyu`'s `@Shenyu*Mapping` in `shenyu-client`/`shenyu-examples`, the latter a genuine two-level chain. ADR 0024 resolves neither, and the reason is measured rather than conservative: every one composes `@RequestMapping(method = …)`, so resolving them at any depth surfaces **zero** endpoints until that shape is read. When that scope cut is revisited these should be revisited with it — they are the same blocker.

A negative result from the same survey, worth recording because it was specifically looked for: **no repository in the Spring corpus wraps `@PreAuthorize` in a meta-annotation.** The NestJS-style "custom decorator bundling the security annotation" does not appear on the Spring side at all. The wrapping happens at the routing layer instead — which is the worse of the two, since an unreadable guard leaves a visible endpoint while an unreadable controller leaves nothing.

**Consequence**: for a project using any of these, the matrix under-reports the API surface, and in the apollo case it does so without saying anything. That silence is the specific failure class [ADR 0020](decisions/0020-unanalyzable-is-unknown-not-absent.md) exists to eliminate — an unanalyzable thing presenting as an absent thing — reached here through endpoint discovery rather than through paths, versions or filter chains. ADR 0019 §2's "recognized no endpoints" notice cannot fire on almost any of them, because almost none of them recognizes *zero* — the single exception, halo, is announced by accident of having no annotated controllers at all, not because anything detected its `RouterFunction`s.

**Two open questions this raised, recorded rather than answered:**

- **Should test sources be analyzed at all?** `parseProject` already skips directories named `test` and files ending `Test`/`Tests`/`IT`, which is a partial and undeclared policy rather than a decision — top-level controllers under other test layouts are extracted and counted today. The nested-controller measurement above (33 instances, all in test code) is what surfaced it. It exists independently of any one item here and is not settled by [ADR 0032](decisions/0032-controllers-that-yield-no-routes.md).
- **An interface name declared twice, with different answers.** `JeecgBoot` declares `org.jeecg.common.airag.api.IAiragBaseApi` in both its `local-api` module (no mappings) and its `cloud-api` module (a Feign client with seven). Same fully-qualified name, two modules, and which one is on the classpath is a build profile no static read settles. ADR 0032 treats such a name as ambiguous and stays quiet, which is a deliberate under-report.

**What to do about it today**: check the endpoint count against what the application actually serves before trusting a matrix as complete. Since ADR 0032 the run says how many controllers produced no routes, which is the missing companion to the endpoint count — but it cannot say how many routes are missing.

**Status**: the controller meta-annotation is fixed ([ADR 0024](decisions/0024-controller-meta-annotations.md)). The rest is measured and recorded, not fixed.

**A note on what surfacing shenyu's admin API actually established.** The 179 routes brought 35 new `mutating-endpoint-without-access-control` findings, not the ~101 the raw count implied — [ADR 0023](decisions/0023-third-party-authorization-annotations.md) absorbed 66 of them the moment the endpoints became visible. And the 35 are **not** 35 endpoints established to be unguarded: shenyu-admin configures a Shiro URL layer (`ShiroConfiguration`, a `ShiroFilterFactoryBean` and a filter chain definition map) that this extractor does not parse, so some are plausibly covered by a path rule there. The honest statement is *35 endpoints surfaced with no method-level access control, in a project whose URL layer is unread* — the same category as the `SecurityFilterChain` blind spot below, and not a conclusion.

## An authorization annotation this extractor cannot identify

A project can declare its own annotation whose simple name is one Spring Security also uses. [`alibaba/nacos`](https://github.com/alibaba/nacos) does: `com.alibaba.nacos.auth.annotation.Secured`, carrying `resource`, `action`, `signType` and a pluggable `parser`, enforced by nacos's own `AuthFilter` and unrelated to `org.springframework.security.access.annotation.Secured`. 108 non-test files import it, 428 uses.

Extraction used to match on the simple name alone and read no imports at all, so it recorded **392 Spring method-security guards** from it — and `mutating-endpoint-without-access-control` was suppressed on **242 of nacos's 245 mutating endpoints** on that basis, none of which carries any other guard. The endpoints really are protected, so the outcome was safe; it was safe by luck, on the strength of a nine-letter word. [ADR 0015](decisions/0015-inert-method-security-guard.md)'s warning also fired, advising that annotations which are not Spring's might be inert for want of `@EnableMethodSecurity`.

Since [ADR 0022](decisions/0022-annotation-identity-and-unrecognized-authorization.md), a recognized name counts as a Spring annotation only when the same file's imports bind it to an accepted package. An annotation that fails that test is recorded as an **unrecognized authorization annotation** — not as a guard, and not as nothing. The scan behind that decision covered 20 real Java repositories: **every** use of `@PreAuthorize`/`@Secured`/`@RolesAllowed` has a binding import in its own file, with no same-package declarations, no wildcards standing in and no fully-qualified inline uses, so per-file extraction answers the question without a classpath.

**Consequence**: an endpoint carrying one is neither confirmed protected nor confirmed unprotected. `mutating-endpoint-without-access-control` deliberately does **not** fire on it — its message, "has no detected guard or role decorator", would be false with an authorization annotation one line above the handler — so the Guards column shows `?` and the run warns, naming the package it actually resolved to and how many endpoints depend on it. Naming the package rather than the simple name is the point: `@Secured` alone reads as Spring's.

**What is still unknown, and it is the important part**: *what that annotation requires*. nacos's 419 `resource`/`action` pairs remain unextracted, for the reason given in *Permissions as metadata* above — the model has no field for a permission. ADR 0022 changed what Sphinxor claims, not what it understands.

**The accepted risk, stated rather than buried**: if such an annotation is decorative — declared but never enforced — Sphinxor now stays quiet where it previously accused. That is a real lost finding, traded deliberately: a false negative on a rare case (a home-grown authorization annotation that enforces nothing) against a false positive on a common one (a home-grown authorization annotation that works), 242 times on nacos alone. It is not silence — the annotation is recorded, the row is marked, and the warning names the package so a reader can check the one thing the tool cannot.

**One cosmetic gap, unexercised and deliberately not fixed**: `sphinxor export cerbos` omits such an endpoint under the `no-guard` reason, which understates what is known about it. No project in the corpus reaches that path — nacos's export is already omitted wholesale for its unreadable URL layer — and inventing a reason code for a case no measured project exhibits would be building ahead of evidence, which this project declines to do elsewhere for the same reason.

**What to do about it today**: read the annotation the warning names. If it genuinely enforces authorization, the affected endpoints are protected and Sphinxor simply cannot say by what; if it does not, they are unprotected and nothing in the report will tell you so.

## Authorization by a framework other than Spring Security — Apache Shiro

Apache Shiro is a different authorization framework, and `@RequiresPermissions("system:user:edit")` is how a large amount of real Java declares access control. It shares no name with anything Spring Security uses, so before [ADR 0023](decisions/0023-third-party-authorization-annotations.md) it never entered any recognized set and the endpoint fell straight through to *no access control found* — the state [ADR 0022](decisions/0022-annotation-identity-and-unrecognized-authorization.md) §3 had already established is the wrong thing to report about an endpoint that has some. nacos was caught only because its annotation happened to be spelled like Spring's.

Measured across the 20-repository Java corpus: **907 mutating routes carried a Shiro annotation and were reported as having no access control** — around a fifth of all `mutating-endpoint-without-access-control` findings in the Spring corpus.

Since ADR 0023, an annotation bound by import to `org.apache.shiro.authz.annotation` is recorded as an unrecognized authorization annotation, exactly as nacos's `@Secured` is: no guard, no role, `?` in the Guards column, the mutating-endpoint finding suppressed, and a warning naming the package and the endpoint count. **845 findings disappeared across the corpus** — metersphere 555, streampark 101, JeecgBoot 81, litemall 65, inlong 43 — with no other repository changing in any respect.

**This does not mean Shiro is supported.** Nothing reads what it requires. `"system:user:edit"` is not extracted, modelled or reported, for the same reason nacos's `resource`/`action` pairs are not — the model has no field for a permission (*Permissions as metadata* above). Shiro's URL layer (`ShiroFilterFactoryBean`, in shenyu and streampark) is not parsed either. One claim was withdrawn: that these endpoints have no access control.

**Whether the annotations are switched on is now reported.** Shiro annotations do nothing unless `AuthorizationAttributeSourceAdvisor` is wired in — the counterpart of Spring's `@EnableMethodSecurity` — and the run says whether it was located, per [ADR 0015](decisions/0015-inert-method-security-guard.md)'s treatment applied unchanged. It is not downgraded when absent: Shiro's spring-boot starter enables annotation support by auto-configuration, leaving no Java bean to find, and build files are not parsed. In all six corpus repositories using Shiro the wiring *was* located.

**What remains, and it is substantial: project-local authorization annotations.** No package list can enumerate an annotation a project invented. Checked individually rather than assumed:

- metersphere's `@CheckOwner` (515 uses) — enforcement, read by `CheckOwnerAspect`, `CheckProjectOwnerAspect` and `CheckOrgOwnerAspect`.
- streampark's `@Permission` (78) — enforcement, read by `PermissionAspect`.
- `@AuthAction` (28, Sentinel vendored inside JeecgBoot) — reader lives in the dependency; not confirmable from source.
- dataease's `@DePermit` (97) — **no reader anywhere in the analyzed source**, only its own declaration. It may be enforced outside the tree, or it may be dead.

Endpoints carrying these keep their findings. That is the status quo, not a regression — and `@DePermit` is why the obvious shortcut was rejected: a rule matching annotation *names* would have suppressed 97 findings on the strength of a word, with no more evidence of protection than the word itself.

**Two further shapes deliberately excluded**, both measured:

- **A name heuristic** would capture 1,615 mutating routes to the package rule's 907, but it catches JeecgBoot's `@IgnoreAuth` — an annotation that *skips* authentication — and so would suppress the finding precisely where it is correct, on genuinely public endpoints. It also catches litemall's `@RequiresPermissionsDesc`, which declares `menu()` and `button()` for an admin console, and still misses `@CheckOwner`.
- **An on-demand (`.*`) import** of the Shiro package binds nothing here. A wildcard says a package is in scope, not which names come from it, and honoring one bound every unimported annotation in the file to Shiro — an early implementation recorded `@RestController` as a Shiro authorization annotation. 274 of the corpus's 275 Shiro imports are single-type, and the one on-demand file has a single mutating route, so the exclusion costs one route in the safe direction.

**Other frameworks are not covered.** Sa-Token (`cn.dev33.satoken.annotation`) is the obvious next candidate and is deliberately absent: zero occurrences across the 20 repositories scanned, and adding a package the corpus does not exercise would be guessing.

**What to do about it today**: the warning names the package. Confirm the framework is wired up — for Shiro, that `AuthorizationAttributeSourceAdvisor` or the spring-boot starter is present — and read the annotations themselves for what they require, which Sphinxor will not tell you.

## A Spring annotation written fully qualified — fixed

Java lets an annotation be written out in full at the use site, and sometimes a project must. `microcks/microcks` declares its own class `io.github.microcks.web.RestController`, so Spring's cannot be imported into that package; all **13 files** there write `@org.springframework.web.bind.annotation.RestController`.

Extraction matched the text as written against `"RestController"`, so none of those classes was a controller. microcks reported **50 endpoints** while those files held **30 more in a shape the extractor already reads** — roughly 40% of its API surface, missing for a reason unrelated to any other gap, and silent, since [ADR 0019](decisions/0019-cli-framework-selection.md) §2's notice cannot fire when 50 endpoints are recognized.

**This was the nacos collision with the sign reversed.** [ADR 0022](decisions/0022-annotation-identity-and-unrecognized-authorization.md) fixed a case where a matching *simple name* made a foreign annotation count; here a *qualified name* made Spring's own annotation not count. Both treat the string as written as the annotation's identity.

Since [ADR 0025](decisions/0025-qualified-annotation-names.md) the identity is the simple name however it is written, and microcks reports **80 endpoints**. A dotted name must match a known package **in full**: `@com.example.RestController` is not Spring's, and taking the last segment would reintroduce the nacos fault in the change meant to fix its mirror image. The corpus makes that concrete in an unexpected way — its only short dotted annotation names are `@lombok.Data` and `@feign.Headers`, which are *complete* paths rather than truncations, so a truncated `@annotation.RestController` and a complete `@lombok.Data` are indistinguishable to anything matching on the last segment.

**Consequence**: the 30 recovered routes brought 11 new `mutating-endpoint-without-access-control` findings and no roles or guards at all, because those files carry no Spring Security annotations. Nothing about microcks's authorization picture changed; its API surface stopped being under-reported.

**What remains for microcks**: 4 more of its routes are still invisible, behind `@RequestMapping(method = …)` — the entry above, a separate gap.

**Not covered**: a *partially* qualified name, `@annotation.RestController` from a wildcard import plus a nested reference. Zero occurrences across 20 repositories, and resolving one needs the imports and package structure rather than the use site alone.

## Spring: `SecurityFilterChain` beyond simple, single-chain patterns

Not "`SecurityFilterChain` is unsupported" — as of [ADR 0012](decisions/0012-securityfilterchain-effective-policy.md), recognized simple-pattern `authorizeHttpRequests` rules are supported and correctly AND-combined with method-level `@PreAuthorize`/`@Secured`/`@RolesAllowed` (the set intersection described there), verified against the real mismatch that drove that ADR: `Kitty-Hivens/Pharmacy`'s `SupplierController.GET` allows `ADMIN` or `PHARMACIST` at the method layer but only `ADMIN` at the URL layer, and Sphinxor now reports the real, `ADMIN`-only effective policy rather than the method layer's broader claim.

What's still invisible, per that ADR's stated scope and [ADR 0018](decisions/0018-unrecognized-rule-stops-evaluation.md)'s correction to it:

- **A custom `AuthorizationManager`** (`.access(...)`) — executing arbitrary Java to know the real answer is categorically out of scope. Confirmed common in real code, not hypothetical: `categolj/blog-api`'s entire tenant-scoped rule set uses this, alongside a handful of ordinary `.hasAuthority(...)` rules in the *same* chain — extraction recognizes the `.hasAuthority(...)` ones individually and correctly stops evaluating at the first `.access(...)` rule that matches a given endpoint (per ADR 0018), rather than skipping past it to a later, more permissive rule.
- **More than one `SecurityFilterChain` bean in the same project** (chain selection by `@Order`/`securityMatcher` scoping) — extraction requires finding exactly one `@Bean`-annotated method returning `SecurityFilterChain` project-wide, rather than guessing which chain (or ordering) actually applies to a given request. Per ADR 0020 §2 this is now *announced*, not silently skipped: the project's URL layer is recorded as present-but-unknown, `sphinxor lint` warns that the roles it shows may be broader than what the application enforces, and `sphinxor export cerbos` omits every endpoint rather than exporting a method-layer-only policy. Previously the layer just vanished, and the audit's two-chain reproduction exported `roles: [ADMIN, ANALYST]` for an endpoint the running application restricted to `ADMIN` — a grant the application itself denies.
- **Reactive `SecurityWebFilterChain` / `ServerHttpSecurity`** (Spring WebFlux) — not parsed. Its rules use a different builder API (`authorizeExchange`, `pathMatchers`) that extraction does not read at all. Like the multi-chain case, and for the same reason, it is detected so it cannot be mistaken for "this project has no URL layer", and produces the same warning and the same export omission.
- **A pre-Spring-Security-5.7 `WebSecurityConfigurerAdapter`** — the older configuration base class, in scope but not parsed. `zalando/nakadi` uses one, and its rules are `.access(hasScope(…))`, which [ADR 0012](decisions/0012-securityfilterchain-effective-policy.md) puts out of scope even in the modern API. Detected and announced since [ADR 0027](decisions/0027-unannounced-url-layers.md).
- **An Apache Shiro `ShiroFilterFactoryBean`** — a different framework's URL layer, and nothing about it is parsed. Six corpus projects declare one (JeecgBoot, shenyu, streampark, inlong, litemall, metersphere). Announcing it is not interpreting it, the narrower half of [ADR 0023](decisions/0023-third-party-authorization-annotations.md)'s argument: what changes is that a project with a Shiro URL layer stops being reported as though it had none. `inlong`'s, read by hand, ends in a `/**` catch-all applying an `AuthenticationFilter` and a tenant filter — **114 of its mutating routes fall under it and zero are `anon`** — which is why its endpoints showing no per-endpoint authorization must not be read as "anyone can call this".
- **Non-Ant-pattern matchers**: regex or character-class syntax, a custom `RequestMatcher` bean, `mvc.matcher(...)`, `dispatcherTypeMatchers`. Only literal segments, `*`, `**`, and `{var}`-as-wildcard are matched. A matcher whose pattern cannot be read makes that *rule* unknown (ADR 0020 §1) — it stops evaluation for endpoints it might cover and grants its roles to none of them. The wrapper forms that carry an ordinary readable pattern one call deeper, `requestMatchers(antMatcher("/admin/**"))`, are read normally.
- **`@PostAuthorize`/`@PreFilter`/`@PostFilter`**, composed/meta-annotations, `RoleHierarchy` resolution, and Kotlin source remain out of scope, per ADR 0011.

**Consequence**: an endpoint whose real access control depends on any of the above is reported using whatever the method layer alone establishes (or as unguarded, if the method layer has nothing either) — under-reporting relative to a rule Sphinxor can't read, never over-reporting a grant that isn't real, consistent with the intersection's own soundness-not-completeness property. Where the gap is project-wide rather than rule-local — the multi-chain and reactive cases — the run says so explicitly instead of leaving the reader to infer it from this file.

**What to do about it today**: `sphinxor-allow` on endpoints known to be protected by one of the above, same as any other endpoint the tool cannot see into.

## Spring: method-security annotations that may never be switched on

`@PreAuthorize`, `@Secured` and `@RolesAllowed` do nothing at runtime unless `@EnableMethodSecurity` (or the older `@EnableGlobalMethodSecurity`) is present. An application can carry a full set of annotations and enforce none of them.

Sphinxor deliberately does not downgrade those annotations when it can't find the enabling one, per [ADR 0015](decisions/0015-inert-method-security-guard.md): absence is not evidence, since the configuration can live in a parent module, an imported starter, or Kotlin source this extractor doesn't parse. Guessing "disabled" would invent findings; guessing "enabled" is what it does, and that is the safer of the two guesses to make out loud.

What changed with ADR 0020 §4 is that it is now made out loud. When method-security annotations are found and no enabling annotation is located anywhere in the analyzed source, the run warns that those annotations may be inert — in which case every role shown for the endpoints they appear to protect is imaginary, and the endpoints are not protected at all.

**Consequence**: unchanged in the model and the findings; the roles are still reported. The difference is that a report which is confidently wrong in this specific way now carries the one sentence that lets a reader check.

**What to do about it today**: confirm where method security is enabled for the application. If it's outside the analyzed source, nothing is wrong with the report; if it's nowhere, the finding is that the annotations are decorative.

## A route path declared with a constant rather than a string literal

`@Controller(RouteKey.Asset)`, `@Controller(BASE)`, `@Controller(['cats', 'kittens'])`, a template literal, and Spring's `@RequestMapping(Routes.ADMIN)` all declare a real path that extraction cannot read: it collects string literals, and does not resolve a constant to its declaration. Route constants and route enums are ordinary good practice, not an anti-pattern — immich uses `@Controller(RouteKey.X)` in 4 of its 47 controllers, and ToolJet in 4 of its own.

Since [ADR 0020](decisions/0020-unanalyzable-is-unknown-not-absent.md) Amendment 1 §5 this is loud, and — more importantly — no longer dangerous. Before it, the unreadable prefix silently became the empty string, and since endpoint identity is derived from `(method, path)`, unrelated endpoints collided on one identity. NestJS merged them, reporting one endpoint's guards and roles against another: a wide-open `DELETE` was shown as `ADMIN`-protected with the `mutating-endpoint-without-access-control` finding suppressed. Spring dropped one of the pair outright, so the unguarded endpoint vanished from the report.

Those endpoints now keep an identity synthesized from their controller and handler, so nothing merges or disappears; the path shown for them is marked with a leading `…`, the run warns and names the affected controllers, and `sphinxor export cerbos` omits them, since a policy cannot be named after a fragment of a route.

**Consequence**: the affected endpoints are analyzed and linted normally, and their findings are real. What is not available is their full path — so they cannot be exported, and if such a path later becomes readable, `sphinxor diff` reports the endpoint once as removed and once as added, because its identity changed.

**What to do about it today**: nothing is required — the analysis is sound. If you want these endpoints exportable, use a string literal in the decorator. Resolving constants to their declarations is a possible future improvement, recorded as the rejected-for-now alternative in that ADR.

## GraphQL resolvers are not analyzed

`@Resolver` classes and the `@Query`/`@Mutation`/`@Subscription`/`@ResolveField` operations they carry are outside Sphinxor's scope, per [ADR 0021](decisions/0021-graphql-out-of-scope-but-detected.md). Authorization declared on them — including `@UseGuards` and `@Roles`, which are the same decorators the REST side uses and are perfectly readable — is not extracted, and no operation appears in the matrix.

The reason it is not simply added: a GraphQL operation has no HTTP method and no path, and endpoint identity, the matrix's columns, allowlist anchoring and the Cerbos exporter's resource/action mapping are all built on those ([ADR 0002](decisions/0002-intermediate-model-structure.md), [ADR 0009](decisions/0009-cerbos-exporter.md)). `@ResolveField` authorizes a field on a type, which the model has no concept of at all. Supporting GraphQL honestly is a model decision, not an extraction tweak.

Since ADR 0021 this is loud. A project containing resolvers is detected and the run warns, naming the number of operations it did not analyze. Before that it was silent in the worst way: `notiz-dev/nestjs-prisma-starter`'s whole API is 16 GraphQL operations, 10 of them guarded, and Sphinxor reported the project's two hello-world REST routes with 0 findings and no caveat — [ADR 0019](decisions/0019-cli-framework-selection.md) §2's "recognized no endpoints" notice could not fire, because two is not zero.

**Consequence**: the HTTP matrix for such a project is correct as far as it goes, and the warning says how far that is. A mixed REST + GraphQL project is the case to watch, not the GraphQL-only one — its REST results are complete and accurate, and that apparent completeness is what makes the unanalyzed half easy to overlook.

**What to do about it today**: review resolver authorization by hand. Sphinxor will tell you it exists; it will not tell you what it requires.

**Not covered**: Spring's `@QueryMapping`/`@MutationMapping`/`@SchemaMapping`. It was not surveyed during the audit that produced this entry, and it is not claimed as handled on the strength of symmetry with NestJS.

## Two controllers declaring the same route — now loud, no longer over-reports access

Endpoint identity was `(method, path)`. If two controllers in the same analyzed tree declared the same absolute route, they shared one identity, and the same damage followed as in the unreadable-path case above — except here nothing is unanalyzable. Both paths read perfectly. **Consequence**, reproduced on a minimal case: in NestJS the two endpoints merged, so an unguarded route showed the other's guards and roles — a `DELETE` with no guard at all reported as `ADMIN`-protected, with `mutating-endpoint-without-access-control` suppressed. In Spring one of the pair was dropped from the report entirely, and the survivor was whichever was extracted first.

Since [ADR 0020](decisions/0020-unanalyzable-is-unknown-not-absent.md) Amendment 2 §8, colliding endpoints are kept apart unconditionally — each keeps its own guards, neither is merged or dropped — and `sphinxor export cerbos` omits them, since a policy written for one might govern the other's traffic. The run warns only when the two sides' guards actually differ, which is the case where merging them would have reported one endpoint's protection against another. **What remains unknowable** is the thing the warning names: whether the two are one route or two. A runtime path prefix, a conditional controller registration, or a second application mounted from the same tree would separate them, and none of those is visible to this extractor.

This entry previously named the failing assumption as *"one analyzed tree is one application"*, on the strength of a single sighting in immich. A survey of 23 real repositories (17 measurable; the rest use route shapes this extractor doesn't recognize, so they can't answer either way) found that diagnosis to be the benign case, and found two other causes that do the damage. What follows is what was measured.

### The two causes that actually over-report access

**A route version that was read past — fixed.** `@Controller({ path, version })` was walked for its `path` key while every other key was stepped over, so the `version` — a route discriminator the running application routes on — was ignored, and two distinct endpoints collapsed into one. Spring's `version` attribute on a mapping annotation was ignored the same way. Hand-verified on [`calcom/cal.com`](https://github.com/calcom/cal.com), where one `GET /v2/event-types` requires authentication (`ApiAuthGuard`) and the other makes it optional (`OptionalApiAuthGuard`): merged, each was reported carrying the other's guard. 15 of cal.com's collisions were version pairs, 4 of them with differing guards; 11 of novu's were the same.

Since [ADR 0020](decisions/0020-unanalyzable-is-unknown-not-absent.md) Amendment 2 §7 the version is part of endpoint identity, in both frameworks and in all of NestJS's declaration forms. What remains, deliberately: a version that cannot be read — a constant reference, or an array of them, which is the *majority* shape in real versioned code — makes that endpoint's identity synthesized from its controller and handler rather than its route, and `sphinxor export cerbos` omits it, since a policy there could not say which of two same-path routes it governs. And the version's effect on the URL is still not known: NestJS's URI versioning puts it in the path while header and media-type versioning do not, and the `enableVersioning(...)` call that decides this is not parsed, so a URI-versioned path is shown as declared rather than as the application serves it.

A related fact about how long a readable discriminator can sit unread: **both NestJS fixtures vendored since v0.1 already declared versions** — `nestjs-boilerplate` on every one of its controllers, `awesome-nest-boilerplate` on `GET /auth/me` — and nothing noticed until §7's control test asserted their absence and failed. Unlike a global guard or a composite decorator, nothing about this one was hard to see; it was one object key beside a key already being read.

**A path prefix applied by runtime configuration.** [`YunaiV/ruoyi-vue-pro`](https://github.com/YunaiV/ruoyi-vue-pro) declares `@RequestMapping("/member/address")` on both an admin controller and an app controller, in one application and one Maven module. They differ at runtime only because a `WebMvcRegistrations` bean calls `RequestMappingHandlerMapping.setPathPrefixes(...)`, mapping `/admin-api` and `/app-api` by matching the controller's Java package name — an arbitrary runtime predicate no static analysis resolves. 59 collisions, **47 with differing guards**: the admin side carries `@PreAuthorize`, the app side carries nothing.

Reproduced end to end with the CLI on `yudao-module-member`: with both controllers in the tree, `PUT /member/user/update` appears once, attributed to the admin controller and carrying `@PreAuthorize`, while `AppMemberUserController.updateUser` — which has no access control at all — is **absent from the report** and produces no `mutating-endpoint-without-access-control` finding. Linting the app subtree alone flags it correctly. Three mutating endpoints in that repository lose their finding this way.

### Multi-application trees: observed, common, and benign so far

The pattern is real — 7 of the 17 measurable repositories — and in every production instance, hand-verified, the colliding endpoints carried no differing protection:

- [`immich-app/immich`](https://github.com/immich-app/immich)'s `MaintenanceWorkerController` deliberately re-declares `/server/version`, `/admin/maintenance` and others for a separate worker process; 11 endpoints pair up.
- `amplication`'s `POST /login` pair are byte-identical generated files; `novu`'s `GET /health-check` (api vs. worker) are both unguarded.
- `ever-gauzy`'s `GET /` pair does differ — `@Public()` on one side — but that is an opt-out decorator extraction reads on neither side.
- `apache/shenyu`'s 43 collisions are entirely inside `shenyu-examples/`, each its own `@SpringBootApplication`; `shenyu-admin` has none.
- `spring-petclinic-microservices` is a four-service tree with zero collisions: a multi-application tree does not even imply a collision.

**The "no bleed" here is measurement-limited, but the safety is not.** For immich and novu, extraction recognizes no guards at all on either side of the pair — their authorization runs through decorators it cannot read (see the composite-decorator entry above) — so "the guards are the same" means "none were seen". The equality is an artifact, and Amendment 2 §8 says so itself.

What does *not* follow, and what an earlier version of this entry wrongly implied, is that danger is queued up behind better extraction. §8 has two halves and only the warning is conditional: **keeping the colliding endpoints apart is unconditional**, so neither one is merged into the other, neither is dropped, and neither can report the other's protection — whatever extraction can or cannot see. Verified on real immich rather than reasoned about: all 11 pairs are present in the report today, each with its own identity, and `sphinxor export cerbos` omits all 11. The same run with immich's `@Authenticated({...})` options rewritten into literal guards — a stand-in for extraction learning to read them — raises the §8 warning on all 11, naming each route, because all 11 genuinely differ (the worker side carries `@MaintenanceRoute()` or nothing; the main-app side carries `permission: …, admin: true` or `public: true`). That is the warning waking up on its own, which is what §8 predicted, and it arrives in the safe direction: late, never wrong.

A fourth shape, recorded without a verdict: **conditional controller registration**. `novu`'s `organization.module.ts` returns `[EEOrganizationController]` or `[OrganizationController]` depending on a runtime check, so only one is ever mounted. Three collisions, identical class-level guards, no bleed observed.

**Status of a fix**: [ADR 0020](decisions/0020-unanalyzable-is-unknown-not-absent.md) Amendment 2 is **Accepted and implemented**, in both halves. The rest of this entry describes what the tool did before it, and is kept because the mechanism it describes is still the reason the current behaviour looks the way it does.

**What to do about it today**: when the run warns that one route is declared by two controllers with different access control, check which of the two your request actually reaches — the tool cannot, and the answer decides which row's guards apply. Pointing `sphinxor` at a single application's source root removes the multi-application case, but not the two causes above, which occur inside one application.

## A `sphinxor-allow` marker separated from its endpoint by a block comment

The allowlist matcher (`internal/allowlist`, shared by both extractors since the Spring port) skips blank lines and `//` line comments between a marker and the endpoint it exempts, but not `/* */` blocks. So a marker placed above a Javadoc/JSDoc block, rather than directly above the endpoint's first decorator/annotation, exempts nothing.

This is recorded as benign, deferred debt rather than a latent false negative, on two grounds — both checked rather than assumed:

- **The dominant Spring idiom is immune by construction, not by luck.** Java annotations are part of tree-sitter's `method_declaration` node, so the endpoint's anchor line is its *first annotation*. A handler documented with OpenAPI annotations (`@Operation(summary = ...)`, the pattern in `Kitty-Hivens/Pharmacy`) therefore has its documentation *inside* the anchor, never between the marker and it. Across the whole vendored Java corpus — both `SecurityConfig`s, all five controllers, the role enum — there is not one block comment; handlers are either bare (`categolj/blog-api`) or annotation-documented (Pharmacy). Javadoc above handlers is common in Java generally, but not in the Spring REST controllers this targets.
- **When it does occur, it fails loudly, not silently.** A marker that matches nothing produces the `stale-allow-marker` finding ([ADR 0003](decisions/0003-allowlist-format.md)) at `High` confidence, naming the marker's file and line and stating that it does not sit directly above a recognized endpoint — and `High` findings gate CI. Verified directly against the real Pharmacy fixture with a Javadoc inserted between marker and handler. A developer never silently believes an exemption applied; their build fails telling them it didn't. That is the self-announcing refusal the stale-marker finding exists to produce, the inverse of this project's reassuring-false-negative failure class.

**What to do about it today**: put the marker directly above the endpoint's first decorator or annotation, above any Javadoc/JSDoc block. The stale-marker finding will tell you if you haven't.

**Why it isn't fixed yet**: the matcher is now shared by both extractors, so widening it to skip block comments is a two-framework behavior change affecting existing NestJS results, and it deserves its own tests rather than being folded into an unrelated change. Deferred deliberately, not overlooked.
