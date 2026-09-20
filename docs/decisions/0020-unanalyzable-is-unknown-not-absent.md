# 0020. A rule, matcher, or layer that could not be analyzed is unknown, never absent — generalizing ADR 0018

## Status

Accepted (§1–§4, implemented).

**Amendment 1 (§5–§6): Accepted.** See *Amendment 1* below. The accepted decision
is unchanged; the amendment extends its reach to two mechanisms the original text
did not examine.

**Amendment 2 (§7–§8): Accepted.** See *Amendment 2* below. Same principle again,
at a seventh and eighth mechanism — a route-discriminating `version` that is read
past, and a path prefix applied by runtime configuration. Both were found by
measuring the duplicate-route limitation across 17 real repositories.

**Amendment 3 (§9–§11): Accepted.** See *Amendment 3* below. A ninth mechanism — a
role list that could not be read, recorded as a role list declared empty. Found by
surveying 14 production Spring repositories, where it was producing 675
High-confidence, CI-gating false positives across four of them. This is the first
member of the family to reach a **blocking** finding, and the first to require a
lint rule's stated rationale be rewritten rather than only its behaviour changed.

## Context

A blind-spot audit across the documented limitations and several new real
repositories found three silent failures. They are not three bugs. They are one
conceptual error at three scopes, and it is an error this project already
identified and corrected once.

[ADR 0018](0018-unrecognized-rule-stops-evaluation.md) found that ADR 0012 §1 had
conflated *unrecognized* with *absent*: a `SecurityFilterChain` rule whose **shape**
Sphinxor couldn't read was treated as if it weren't there, so evaluation fell
through to a later, more permissive rule. ADR 0018 fixed that — for a rule's shape.

The principle behind it was never applied to the two neighbouring cases:

- a **matcher** whose pattern can't be read, and
- an entire **layer** that can't be analyzed.

Both still resolve to "absent," and both produce silently wrong answers.

### A. An unreadable matcher becomes match-all

`requestMatchersArgs` (`internal/extract/spring/securityfilterchain.go`) collects
only string-literal arguments. When none are found it returns `nil` patterns — and
upstream, `nil` patterns is *also* the sentinel meaning `anyRequest()`, "matches
every path." So a matcher Sphinxor cannot read silently becomes a rule that matches
**everything**.

Verified against a real run:

```
.requestMatchers(antMatcher("/admin/**")).hasRole("ADMIN")
.anyRequest().permitAll()

| DELETE | /admin/wipe  | ... | ADMIN | - |
| DELETE | /public/wipe | ... | ADMIN | - |   <- really permitAll(): anyone can call it
0 finding(s), 0 blocking, 0 warning
```

**State this plainly, because it is the worst defect found in this project to
date**: Sphinxor asserts protection that does not exist, on a destructive endpoint,
*and* suppresses the `mutating-endpoint-without-access-control` finding that exists
precisely to catch an unguarded `DELETE`. Both the claim and the safety net fail
together, in the same direction, with a clean report. That is not incomplete
coverage — it is a false security assurance, the single failure mode
`docs/vision.md` and [ADR 0009](0009-cerbos-exporter.md) §3 are written to prevent.

The trigger is not exotic. `requestMatchers(antMatcher("/admin/**"))` is idiomatic
Spring Security 6 — the officially recommended form when mixing matcher types —
and `RegexRequestMatcher`, `mvc.pattern(...)`, and custom `RequestMatcher` beans
all take the same path.

### B. An unanalyzable URL layer is treated as no URL layer

`findSecurityFilterChainRules` requires exactly one `SecurityFilterChain` bean
(`if len(lambdas) != 1 { return nil, "", false }`); `Extract` then skips the URL
layer entirely. Nothing records that a layer existed and went unread.

The consequence reaches the **deployable artifact**. The same application, differing
only in bean count:

| | exported Cerbos policy | reality |
|---|---|---|
| one chain | `roles: [ADMIN]` | ADMIN |
| two chains | `roles: [ADMIN, ANALYST]` | ADMIN |

The two-chain export grants `ANALYST` an access the running application denies, and
the export report says nothing. Multi-chain configurations are common (a separate
chain for actuator, or API versus form login).

### C. The reactive URL layer is not looked for at all

`internal/extract/spring` contains no reference to `SecurityWebFilterChain`,
`ServerHttpSecurity`, or `pathMatchers`. Confirmed by A/B against the *same* official
sample in both flavours: the servlet version reports `SCOPE_message:read` /
`SCOPE_message:write` correctly; the reactive twin reports nothing for either.

In a `pathMatchers`-only application this fails loudly and in the safe direction (the
mutating endpoint gets flagged). Combined with `@PreAuthorize` — the ordinary case in
reactive applications — it becomes case B: the method layer is reported as the whole
story, and over-reports.

### What the three share

In all three, something that could not be analyzed was recorded as something that
was not there. An unreadable matcher became "matches everything." An unread layer
became "no such layer." Neither is a guess the code knows it is making, which is
why nothing warns.

The distinction that matters, and that the current code cannot express:

- **absent** — genuinely no such rule/matcher/layer. A method-security-only Spring
  app has no URL layer, and the method layer really is the complete picture.
- **unknown** — it exists and could not be read. The complete picture is not
  available, and any answer derived as though it were is unearned.

## Decision

### The principle

**A rule, matcher, or layer that Sphinxor could not analyze is recorded as
*unknown*, never as *absent*, and never as a permissive default. Any result derived
from an incomplete picture must say so.**

This is ADR 0018's correction stated at the level it should have been stated
originally. It governs the three cases below and is intended to pre-decide the next
member of this family rather than waiting for it to be found in an audit.

### §1 An unreadable matcher is an unrecognized rule, not a universal one

`nil` patterns stops doubling as the `anyRequest()` sentinel. A `requestMatchers(...)`
call from which no pattern could be extracted becomes the existing
`chainUnrecognized` kind, which ADR 0018 already gives the correct behavior: it is
opaque, it may match, so evaluation stops and the endpoint's URL layer is
unresolved. `anyRequest()` keeps its own explicit representation.

Conservative, because an unreadable matcher *might* match any path — so every
endpoint reaching that rule has an unknown URL-layer outcome rather than a
confidently wrong one.

**Paired with widening what can be read**, so "unknown" stays rare rather than
becoming the common answer: the wrapper forms that triggered this — `antMatcher("…")`,
`AntPathRequestMatcher.antMatcher("…")` — carry a perfectly readable string literal
one call deeper, and are recognized. Genuinely unreadable matchers (regex, custom
beans) resolve to unknown. Fixing the dangerous default without this would trade a
silent wrong answer for a large, honest, and needless loss of coverage on idiomatic
configurations.

### §2 An existing-but-unanalyzable URL layer is recorded as unknown

Presence of a URL layer is detected separately from parsing it. Detection alone is
what converts silent into loud, and it is cheap: the tokens are `SecurityFilterChain`
and `SecurityWebFilterChain`.

- **No URL layer found at all** → absent. The method layer is complete. Current
  behavior is correct and must not regress: a method-security-only application must
  not start reporting uncertainty it doesn't have.
- **A URL layer found but not analyzable** — more than one `SecurityFilterChain`
  bean, zero parseable ones, or a reactive `SecurityWebFilterChain` (§3) — →
  **unknown**, recorded on the model, not silently skipped.

Consumers then differ, matching what each output already means:

- `sphinxor lint`'s matrix is an inventory, so it still shows the method-layer roles
  it found, plus a prominent warning that the URL layer exists and was not analyzed,
  so the effective policy may be narrower than shown.
- `sphinxor export cerbos` emits **grants**, so per ADR 0009 §3 it must not export a
  method-layer-only rule as though it were the effective policy. Those endpoints are
  omitted and flagged in the companion report. Exporting the broader set is guessing
  in the permissive direction, which that ADR forbids.

### §3 Reactive chains are detected, and parsing them is explicitly out of scope here

`SecurityWebFilterChain` / `ServerHttpSecurity` presence is detected and feeds §2's
unknown state. Actually parsing `pathMatchers(...)` is **not** part of this ADR —
that is new framework coverage, and it belongs in its own decision with its own
real-fixture validation. This ADR's obligation is only that a reactive application
stops being told a method-layer-only answer is complete.

### §4 Project-level uncertainty is stated, not resolved

Two further audit findings are *not* wrong answers; they are correct answers
presented with more confidence than earned. Both keep their current analysis and
gain a project-level warning, using the notice mechanism
[ADR 0019](0019-cli-framework-selection.md) §2 already established (stderr, so
`--format json` on stdout stays clean).

- **Method-security annotations found, no `@EnableMethodSecurity` located.**
  `isConfirmedInert` deliberately treats `Found == false` as "no evidence either
  way," because the enabling configuration can live in a parent module or unparsed
  Kotlin. That reasoning stands and does not change. But the consequence today is
  that `@PreAuthorize("hasRole('ADMIN')")` on a wide-open endpoint is reported as
  `Roles: ADMIN` with zero findings. The warning says the annotations were found,
  the enabling configuration was not seen, and if it is not enabled elsewhere these
  annotations are inert and the endpoints are unprotected.
- **NestJS `APP_GUARD` provider detected.** On the inverted-default pattern NestJS's
  own documentation recommends — a global guard protecting everything, `@Public()`
  opting out — endpoint-level results systematically understate protection. Verified
  on `nestjs/nest`'s official `19-auth-jwt` sample. The direction is safe, but the
  distortion is silent; the warning says a global guard was registered and
  endpoint-level results may understate protection.

Neither warning changes a finding or a grant. They change what the user believes the
output means, which is the whole point.

## Alternatives considered

- **Three separate ADRs, one per defect** — rejected, and this is the substantive
  choice in this document. The audit's actual finding is that ADR 0018's lesson was
  applied to the case that revealed it rather than to the principle behind it.
  Writing three patches would repeat exactly that mistake and leave the fourth
  member of the family — whatever it turns out to be — undecided.
- **Treat an unreadable matcher as matching nothing** (skip, continue) — rejected:
  that is ADR 0012 §1's original error, re-entered through a different door. A
  skipped rule lets evaluation reach a later, more permissive one.
- **Keep exporting method-layer-only rules when the URL layer is unknown, with a
  warning in the report** — rejected. A warning in a companion document does not
  make a deployed policy less permissive, and ADR 0009 §3 already settled that
  omission is the safe state because Cerbos denies by default.
- **Treat "no `@EnableMethodSecurity` found" as confirmed-inert** (downgrade the
  guards) — rejected, unchanged from ADR 0015: it would produce false positives on
  correctly configured applications whose enabling config is outside the scanned
  tree. The defect is the missing signal, not the analysis.
- **Parse reactive `pathMatchers` now** — rejected for this ADR as scope creep;
  detection is what closes the silent failure, parsing is a coverage feature.

## Consequences

- `internal/extract/spring` gains a distinct representation for an unreadable
  matcher and for an unknown URL layer; `internal/export/cerbos` gains the
  corresponding omission reason. Both are additive to the model.
- **Coverage will visibly drop in places, and that is the intended outcome**: some
  endpoints that today carry confident roles will become unresolved-and-flagged.
  Every such case is one where the previous answer was not actually earned. The
  `antMatcher` recognition in §1 keeps this from being gratuitous.
- Real-fixture validation: the audit's reproductions become regression tests —
  the `antMatcher`/regex match-all case, the two-chain export divergence, and the
  reactive/servlet A/B on the same upstream sample. Each must be confirmed to fail
  against today's behavior before being kept, the standard set by ADR 0014's
  regression test and reaffirmed for ADR 0018's.
- `docs/limitations.md` is updated: the multi-chain and non-Ant-matcher entries stop
  describing silent gaps and start describing loud ones; a reactive entry is added.
- A dedicated NestJS blind-spot hunt across several real repositories remains
  outstanding. The audit behind this ADR probed Spring considerably harder, and
  given the hit rate there, parity should not be assumed.

---

# Amendment 1 — an unreadable **path**, and an unanalyzable **decorator target**

## Status

Accepted.

## Context

The NestJS hunt this ADR's Consequences left outstanding has been carried out:
six real repositories, including two production applications (immich, 302
endpoints; ToolJet, 397), plus every sample in `nestjs/nest`. It found two more
silent failures.

They are not new bugs in the sense of being unrelated to what is above. They are
the same conceptual error — *unanalyzable recorded as absent* — reached through a
fourth and fifth mechanism. The accepted text covers a rule's **shape** (ADR 0018),
a matcher's **readability** (§1), and a layer's **availability** (§2/§3). It does
not cover a route's **path**, or the **target** a decorator attaches to. Recording
these here rather than in a new ADR is deliberate: the principle's reach is the
finding, and a fresh number would present the fourth instance of one error as a
separate bug for the fourth time.

### E. An unreadable route path becomes the empty string, and endpoint identity collapses

`model.NewEndpointID(method, path)` derives an endpoint's identity **from its
path**. Extraction reads only string-literal arguments out of `@Controller(...)`,
so any other form silently yields an empty prefix:

```ts
@Controller(RouteKey.Asset)      // enum member   -> ""
@Controller(BASE)                // const ref     -> ""
@Controller(['cats','kittens'])  // array form    -> ""
@Controller(`${base}/v2`)        // template lit  -> ""
@Get(P)                          // method level  -> segment dropped
```

Spring has the identical defect: `@RequestMapping(Routes.ADMIN)` resolves to `""`.

Two endpoints whose prefixes both vanish then collide on one ID, and the two
frameworks fail in opposite directions — both wrong, both silent:

**NestJS merges them.** Reduced from the immich shape:

```
@Controller(RouteKey.Admin)  @Roles('ADMIN') @UseGuards(...) DELETE /wipe
@Controller(RouteKey.Public)                                 DELETE /wipe   <- no guard at all

| DELETE | /wipe | AdminController  | AuthGuard, RolesGuard | ADMIN |  -  |
| DELETE | /wipe | PublicController | AuthGuard, RolesGuard | ADMIN |  -  |
0 finding(s)
```

The wide-open `DELETE /public/wipe` is reported as ADMIN-protected, and
`mutating-endpoint-without-access-control` — the rule written to catch exactly an
unguarded `DELETE` — does not fire. `sphinxor export cerbos` then writes a
`public.yaml` granting `delete` to `ADMIN`.

That is, line for line, §1's failure: a false security assurance on a destructive
endpoint with the safety net suppressed at the same moment. §1 reached it by
widening a matcher; this reaches it by collapsing an identity. The recurrence of
the *same* output from a different cause is the argument for treating the
principle, not the instance, as the thing being decided.

**Spring drops one.** The same construction in Java reports **one** endpoint where
there are two, and the one that disappears is the unguarded one.

The trigger is not an anti-pattern. Route constants and route enums are ordinary
good practice; immich uses `@Controller(RouteKey.X)` in 4 of its 47 controllers.
Its report contains 13 colliding `(method, path)` pairs covering 26 endpoints; two
of those collisions are traced to this cause specifically — `GET /assets/:id`
reported as `GET /:id` and colliding with `UserController`, and `POST /assets/jobs`
colliding with `JobController`. The remaining pairs were not attributed and may
have other causes. immich shows no bleed today only because none of its guards
are recognized at all, so the precondition is proven in production code while the
damage there is latent.

### F. A comment makes a decorator's target unanalyzable, and the endpoint vanishes

`groupDecorators` (`internal/extract/nestjs/syntax.go`) walks siblings and attaches
each run of `decorator` nodes to "the next non-decorator node." In
tree-sitter-typescript a `comment` **is a named sibling**, so it occupies that slot.
Confirmed against the AST:

```
export_statement
  decorator
  comment            <- the decorators attach here
  class_declaration
    class_body
      decorator
      comment        <- and here
      method_definition
```

Three consequences, one cause:

| shape | result |
|---|---|
| `@Post('x') // note` | that endpoint vanishes from the matrix |
| `@Controller('a') // note` | the **entire controller** vanishes |
| a comment between `@UseGuards(G)` and `@Post('x')` | the guards are dropped; the endpoint is reported unguarded |

The first two are silent false negatives of the worst kind: there is no endpoint
left for a lint rule to fire on, so an unguarded `POST`/`PUT`/`DELETE` produces
neither a row nor a finding. Hit on `CatsMiaow/nestjs-project-structure` — 14
affected decorators across 4 files, `CrudController` (unguarded `POST`, `PUT`,
`DELETE`) absent from the report entirely, and `SampleController`'s only
role-bearing route, `@Roles('admin') @Get('admin')`, absent with it.

This belongs in the same family: a comment renders the decorator's target
unanalyzable, and the endpoint is recorded as *absent* rather than as anything at
all. It differs from E only in that here the correct answer is not "unknown" but
"analyzable after all" — the information was never missing, only mis-attached.

## Decision

### §5 An endpoint whose path cannot be read keeps its identity and is marked unknown

The endpoint is **still extracted and still analyzed**. Its identity falls back to
being synthesized from its controller and handler, and it carries an explicit mark
that its path was not readable.

**Identity.** When the path is readable, `NewEndpointID(method, path)` is unchanged
— existing IDs, existing allowlist anchors, and existing diff history all keep
working exactly as they do today. The synthesized key is used *only* where the
current one cannot be formed, which bounds the change to the endpoints that are
broken now.

**Stability across runs**, which is the diff's actual requirement (ADR 0007): the
synthesized key is stable for as long as the controller class name and handler
method name are stable. That is at least as stable as a path — renaming a route is
routine, renaming a handler is not — and it is the same property ADR 0007 already
relies on for `GuardApplication` and `RoleReference`, neither of which has a
path-shaped identity either. This is a deliberate, narrow departure from ADR 0002's
preference for structure-*independent* keys: where the structure-independent key
cannot be formed at all, a structure-dependent one that exists beats an identity
collision that silently merges or deletes endpoints.

**Re-identification when a path later becomes readable.** If someone replaces
`@Controller(RouteKey.Asset)` with `@Controller('assets')`, or extraction later
learns to resolve the constant, the endpoint's ID changes and `sphinxor diff` will
report it as one endpoint removed and one added. **This is accepted.** It is rare,
it is visible rather than silent, and the failure mode is a spurious `ReasonNew`
gate — which fails toward asking a human, not toward waving something through. The
alternative, keeping a path-derived identity so the ID stays stable, is precisely
the collision being fixed.

**Marking.** The endpoint records that its path is unresolved, so consumers can say
so rather than presenting a partial path as a real route:

- `sphinxor lint`'s matrix shows the path it could resolve, marked as incomplete,
  and the run carries a project-level warning naming the affected controllers.
- `sphinxor export cerbos` must not emit a rule whose resource or action is derived
  from a path it could not read: those endpoints are omitted and flagged, matching
  §2's treatment and ADR 0009 §3.

Lint rules continue to run against these endpoints. Restoring that is half the
point of choosing this option: it is what makes
`mutating-endpoint-without-access-control` fire again on the wide-open `DELETE`.

### §6 A comment never absorbs a decorator

`groupDecorators` skips `comment` nodes when looking for the declaration a run of
decorators belongs to. A comment is not a declaration; treating it as one is a
plain defect with no design tension in it, and the fix carries no interpretive
choice worth deciding in an ADR beyond recording that it happened and why the class
of bug is the one above.

All three shapes in the table must be covered, not only the one that was found
first.

## Alternatives considered

- **Refuse to emit an endpoint whose path can't be read, and warn** — rejected,
  and this is the substantive choice here. It trades a silent false negative for a
  loud coverage hole on a legitimate idiom: immich would lose 4 of 47 controllers
  from analysis entirely. Loud beats silent, so this would be an improvement — but
  it only *signals* the problem, where §5 both removes the cause and puts the
  endpoints back under the lint rules that protect them. Signalling is the right
  answer when analysis is genuinely impossible (§2, §3); it is the wrong answer
  when the endpoint is perfectly analyzable and only its name is uncertain.
- **Resolve the constants — follow `RouteKey.Asset` to its enum declaration** —
  rejected *for this ADR*, though it is the eventual right answer for the common
  cases, and extraction already does exactly this for role enums. It is coverage
  work with its own failure modes (imports, re-exports, computed members) and its
  own fixtures. §5 is what makes the tool safe while the path is unread; resolution
  makes "unread" rarer. They are independent, and bundling them would let the
  safety fix wait on the coverage feature.
- **Key endpoint identity on controller + handler everywhere**, not just as a
  fallback — rejected. It would silently invalidate every existing allowlist anchor
  and every stored baseline, which is a large, breaking change to buy consistency
  in a case that is currently rare.
- **Treat a comment-interrupted decorator run as unknown** (§6) — rejected as
  nonsense dressed as principle. The decorators are right there and fully readable;
  the only thing wrong is which node they were attached to.
- **Fold the GraphQL finding in** — rejected; see below.

## Out of scope, deliberately

The hunt's third silent finding is that a GraphQL-first project reports a clean
bill of health: `nestjs-prisma-starter`'s entire API is 4 `@Resolver` classes and
15 `@Query`/`@Mutation` operations behind `@UseGuards(GqlAuthGuard)`, and Sphinxor
reports the 2 hello-world REST endpoints with 0 findings **and no warning** — ADR
0019 §2's "recognized no endpoints" notice cannot fire, because 2 is not 0.

That is unacceptable and it is not a bug: it is a scope question. GraphQL stays out
of scope, and the tool must say so when it detects resolvers. Detection plus a
project-level warning, not a GraphQL parser. It gets its own small ADR once §5 and
§6 have landed, so that a scope decision is not settled inside a defect fix.

## Consequences

- `model.Endpoint` gains a mark for an unresolved path, and `internal/model` gains
  the synthesized-identity constructor. `internal/export/cerbos` gains the matching
  omission reason. All additive, in the shape §2 already established.
- **Both fixes increase reported endpoint counts**, unlike §1–§4, which reduced
  confident coverage. §6 restores endpoints that were being dropped; §5 stops
  endpoints from overwriting each other. Any project whose count rises was being
  under-reported, silently, before.
- Regression tests, at the bar set above and in ADR 0014 — each confirmed to fail
  against current behavior before being kept:
  - §5, NestJS: the merge case. `DELETE /public/wipe` must come back as its own
    endpoint with no roles, and `mutating-endpoint-without-access-control` must
    fire on it. Today it reports `ADMIN` with zero findings.
  - §5, Spring: the drop case. Two colliding endpoints must both survive; today
    one is silently replaced.
  - §5: the export must omit an endpoint whose path is unresolved rather than
    naming a resource after a path it could not read.
  - §6: all three shapes — trailing comment on a route decorator, trailing comment
    on `@Controller`, and a comment between `@UseGuards` and the route decorator.
    The vanishing-controller case specifically.
- `docs/limitations.md` gains an entry for unresolved route paths (loud after §5),
  and a new recognized shape under the existing composite-decorator entry:
  immich's `@Authenticated({ permission })`, built by pushing onto a
  `MethodDecorator[]` array rather than the flat `applyDecorators()` call ADR 0006
  handles. That one stays loud and documented, not fixed here — 170 Low-confidence
  false positives on 302 endpoints, in the safe direction, with §4's global-guard
  warning already firing on it.
- The NestJS hunt this ADR called for is now done and no longer outstanding. Its
  result argues against assuming the frameworks are equally well covered: the
  official `nestjs/nest` samples surfaced nothing, and all three silent findings
  came from real third-party and production code.

---

# Amendment 2 — a route-discriminating **version**, and a path prefix applied by **configuration**

## Status

Accepted.

§8's warning criterion was narrowed during review, from every same-path collision to
only those whose guards differ. The reasoning is recorded in §8 and in
*Alternatives considered*; the measurement that motivated it is unchanged.

## Context

`docs/limitations.md` carried one open entry describing a *silent,
access-over-reporting* gap: two controllers in one analyzed tree declaring the same
absolute route collide on one `(method, path)` identity, so NestJS merges them and
Spring drops one. That entry named the failing assumption as **"one analyzed tree is
one application"**, cited immich's `MaintenanceWorkerController`, and explicitly
declined to choose a fix — per-application scoping, an analyzed-root option, or
detect-and-warn — on the grounds that one repository is not enough to know whether
the pattern is real.

That question has now been measured, and the measurement changes what the fix should
be.

### What was measured

23 repositories were cloned and analyzed; **17 were measurable**. Six Spring
repositories (`piggymetrics`, `ftgo-application`, `mall`, `mall-swarm`,
`sample-spring-microservices`, `halo`) yield between 0 and 4 handlers because they
use `@RequestMapping(method = RequestMethod.X)` or reactive routers — the ADR 0011
§1 scope cut — and are reported as *not measurable* rather than as zero collisions.

Collisions were counted with temporary instrumentation recording every handler
*before* deduplication, since the Spring extractor drops a colliding endpoint and
the model alone cannot show it. The instrumentation was validated by reproducing
immich's 11 documented pairs exactly before any other repository was trusted, and
removed afterwards.

| Repository | Framework | Handlers | Collisions | Cause |
|---|---|---:|---:|---|
| `eugenp/tutorials` | Spring | 1743 | 204 | multi-app 186, mixed 18 |
| `YunaiV/ruoyi-vue-pro` | Spring | 3253 | 59 | configured path prefix |
| `apache/shenyu` | Spring | 192 | 43 | multi-app (all in `shenyu-examples/`) |
| `novuhq/novu` | NestJS | 465 | 17 | versioning 11, multi-app 3, conditional registration 3 |
| `calcom/cal.com` | NestJS | 175 | 15 | versioning |
| `immich-app/immich` | NestJS | 303 | 11 | multi-app |
| `amplication/amplication` | NestJS | 22 | 2 | multi-app |
| `Ever-co/ever-gauzy` | NestJS | 1196 | 1 | multi-app |
| `ToolJet`, `ghostfolio`, `twenty` | NestJS | 398 / 118 / 146 | 0 | — |
| `JeecgBoot`, `nacos`, `dolphinscheduler`, `streampark`, `hertzbeat`, `spring-petclinic-microservices` | Spring | 587 / 422 / 238 / 232 / 175 / 15 | 0 | — |

### The documented cause is real, common, and benign

Multi-application trees occur in 7 of the 17 measurable repositories. In **every
production instance, guard bleed was zero**, hand-verified rather than inferred:

- `amplication`'s `POST /login` pair — the two `AuthController`s are byte-identical
  generated files.
- `novu`'s `GET /health-check` (api vs. worker) — both unguarded, same shape.
- `ever-gauzy`'s `GET /` pair does differ (`@Public()` on one side), but that is an
  opt-out decorator extraction reads on neither side, so nothing bleeds today.
- `shenyu`'s 43 are entirely inside `shenyu-examples/`, each its own
  `@SpringBootApplication`, all unguarded. `shenyu-admin` has none.
- `spring-petclinic-microservices` is a four-service tree with **zero** collisions:
  a multi-application tree does not even imply a collision.

Amendment 1 recorded 13 colliding pairs in immich and attributed 2 of them to the
unreadable-path cause, leaving the rest unattributed. The survey measures 11, which
is the same number less the 2 that §5 fixed — the remainder are the maintenance
worker, and they are now attributed.

**None of the three options the limitations entry listed would have caught a single
dangerous collision**, because every dangerous collision found is *inside one
application*. That is the finding, and it is why this amendment exists rather than
an ADR choosing among those three.

One honest caveat, carried forward into `docs/limitations.md`: immich's and novu's
"no bleed" is **measurement-limited**. Their authorization runs through composite
decorators extraction does not recognize at all (see the composite-decorator entry),
so there are no guards available to bleed. That is *nothing to bleed*, not *safe* —
the same latency the original entry recorded for immich, now known to apply to novu
as well.

### G. A `version` that is read past, so two distinct routes become one

`controllerBasePath` (`internal/extract/nestjs/syntax.go`) walks the object form of
`@Controller({...})` looking for the `path` key **and ignores every other key**. The
`version` key is seen and stepped over. Spring's `pathAttributeValue`
(`internal/extract/spring/syntax.go`) does the same for `version` on a mapping
annotation.

Version is not decoration. It is a route discriminator: two handlers with the same
path and different versions are two distinct endpoints the running application
routes separately. Collapsing them is §5's failure reached by a new door — *distinct
becomes identical*, so one endpoint is reported carrying another's guards.

Hand-verified on `cal.com`, `POST /v2/bookings`:

```ts
// apps/api/v2/src/platform/bookings/2024-04-15/controllers/bookings.controller.ts
@Controller({ path: "/v2/bookings", version: [VERSION_2024_04_15, VERSION_2024_06_11, VERSION_2024_06_14] })
@UseGuards(PermissionsGuard)
export class BookingsController_2024_04_15 {
  @Post("/")
  async createBooking(...)                      // PermissionsGuard only

// apps/api/v2/src/platform/bookings/2024-08-13/controllers/bookings.controller.ts
@Controller({ path: "/v2/bookings", version: VERSION_2024_08_13_VALUE })
@UseGuards(PermissionsGuard)
export class BookingsController_2024_08_13 {
  @Post("/")
  @UseGuards(OptionalApiAuthGuard)              // one guard more
  async createBooking(...)
```

The two merge, and the 2024-04-15 endpoint is reported carrying an
`OptionalApiAuthGuard` it does not have. 4 of cal.com's 15 collision groups differ
in guards this way; the other 11 are the same defect with matching guards.

Two properties of the real code decide the shape of the fix:

- **The effect of a version on the path is not derivable from the decorator.**
  `novu` bootstraps `VersioningType.URI` with `prefix: '…v'` and
  `defaultVersion: '1'`, so its real routes are `/v1/subscribers` and
  `/v2/subscribers` — the path changes, and the prefix and the default live in
  `bootstrap.ts`, not at the endpoint. `cal.com` bootstraps `VersioningType.CUSTOM`
  with a header extractor, so its paths are genuinely identical and the version is a
  separate runtime discriminator. Any fix that folds version into the path is wrong
  for one of these two.
- **A version is frequently not a readable literal.** novu writes `version: '2'`;
  cal.com writes constant references, and in one case an array of them. So the fix
  must handle a version that is present and unreadable, which is this ADR's own
  central case.

**How long this sat unread, measured on this project's own fixtures**: both NestJS
fixtures vendored since v0.1 already declare versions — `nestjs-boilerplate` uses
`@Controller({ path, version: '1' })` on *every* controller, and
`awesome-nest-boilerplate` carries a `@Version('1')` on `GET /auth/me`. Neither was
noticed, by extraction or by any test, until §7's control test asserted their absence
and failed. A readable route discriminator sat in the validation corpus, unread, for
the project's whole life. It is recorded because it says something about the class of
defect: unlike a global guard or a composite decorator, nothing about this one is
hard to see — it is one object key beside a key already being read.

Spring has the same mechanism, already present in the corpus:
`spring-boot-modules/spring-boot-5/.../apiversions/header/ProductController.java`
declares `@GetMapping(value = "/{id}", version = "1.0")` and
`version = "2.0"` **in one controller, in one file**, and Sphinxor reports one
endpoint. The attribute is a plain string literal there.

### H. A path prefix applied by runtime configuration, which no static analysis can resolve

`YunaiV/ruoyi-vue-pro` (yudao) produced 59 collisions, **47 of them with differing
guards** — the largest concentration of real over-reporting in the survey. Both
controllers declare the same path, literally:

```java
// controller/admin/address/AddressController.java
@RestController @RequestMapping("/member/address")
  @GetMapping("/list") @PreAuthorize("@ss.hasPermission('member:user:query')")

// controller/app/address/AppAddressController.java
@RestController @RequestMapping("/member/address")
  @GetMapping("/list")                            // no access control at all
```

They differ at runtime only because `YudaoWebAutoConfiguration` registers a
`WebMvcRegistrations` bean calling
`RequestMappingHandlerMapping.setPathPrefixes(...)`, mapping `/admin-api` and
`/app-api` by matching the controller's **Java package name** against a configured
pattern. The real routes are `/admin-api/member/address/list` and
`/app-api/member/address/list`.

This is categorically unreadable — an arbitrary `Predicate<Class<?>>` evaluated at
runtime over package names, with the prefixes themselves coming from configuration
properties. It sits with ADR 0012's custom `AuthorizationManager`: executing the
application to know the answer is not static analysis.

The damage was reproduced end to end with the shipped CLI, not with the
instrumentation, on `yudao-module-member/.../controller`:

| | `PUT /member/user/update` |
|---|---|
| both controllers in the tree | one row, attributed to admin's `MemberUserController`, carrying `@PreAuthorize`. `AppMemberUserController.updateUser` — **no access control at all** — is absent from the report, and `mutating-endpoint-without-access-control` does not fire. |
| app subtree alone | correctly flagged `mutating-endpoint-without-access-control`. |

Three mutating endpoints in that repository lose their finding this way:
`PUT /member/user/update`, `POST /promotion/kefu-message/send`,
`PUT /promotion/kefu-message/update-read-status`.

### What G and H share with everything above

G is *readable and not read*: the input is right there in the decorator, so the
answer is to read it. H is *genuinely unreadable*: two controllers declare the same
path and Sphinxor cannot know whether a runtime prefix separates them. What both
currently do is record that uncertainty as a fact — "these are the same endpoint" —
and then silently keep one side's guards. That is this ADR's error, twice more.

## Decision

### §7 A route's version is part of its identity

**NestJS**: the `version` key of `@Controller({ path, version })`, and the
`@Version(...)` method decorator. **Spring**: the `version` attribute on
`@GetMapping`/`@PostMapping`/`@PutMapping`/`@DeleteMapping`/`@PatchMapping`/`@RequestMapping`.

Three states, matching this ADR's own vocabulary:

- **No version declared** → *absent*. Identity is unchanged:
  `NewEndpointID(method, path)` exactly as today. This is what keeps every existing
  allowlist anchor, stored diff baseline, and endpoint ID working — the same
  bounding rule §5 used, and the reason the overwhelming majority of endpoints in
  every surveyed repository are untouched by this amendment.
- **A version declared and readable as a literal** → part of the identity, as a
  separate component beside method and path, and shown in the matrix so two rows
  sharing a path are visibly distinguishable.
- **A version declared but not readable** — a constant reference, an array of
  references, a computed value → *unknown*. Two endpoints whose versions are both
  unknown must not be assumed equal, so identity falls back to §5's
  controller-plus-handler synthesis and the endpoint is marked. This is the cal.com
  case, and it is the majority of the version-bearing code found.

**Version is not folded into the path.** The evidence above is the reason: novu's
URI versioning changes the path using a prefix and a default declared in
`bootstrap.ts`, while cal.com's custom header versioning does not change the path at
all. Reconstructing the real URI path would require parsing `enableVersioning(...)`
and is coverage work, not this fix — the same separation Amendment 1 drew between
§5 and resolving route constants. Until that exists, a URI-versioned path is
displayed as declared and is *incomplete*, and must be marked as such rather than
presented as the whole route.

**The exporter omits an endpoint whose version could not be read, and only that.**
An unreadable version is unknown, and §2 and §5 already settled that an unknown is not
exportable — a policy must not be named after something Sphinxor could not read. A
*readable* version exports exactly as it does today.

This is the lint/export asymmetry [ADR 0012](0012-securityfilterchain-effective-policy.md)
established and §2 restated: the matrix is an inventory and may show what it found
with a caveat, while the exported policy is a deployable artifact, and a warning in a
companion report does not make a wrong policy less wrong.

#### Correction: this clause originally omitted *every* version-bearing endpoint

It was accepted in that broader form on a premise stated in review and not checked:
that "Cerbos resource and action names derive from the path." **They do not.**
`ResourceKind` derives the resource from the **controller class name**, and the action
is the lowercased **HTTP method** ([ADR 0009](0009-cerbos-exporter.md) §2). The path is
used in the exporter only to decide that an endpoint with an unreadable path cannot be
named at all; it never appears in a resource or an action.

Everything the broad clause was meant to prevent therefore had nothing to attach to:

- Two versions declared in **different controllers** — novu's topics pair, cal.com's
  event-types pair, which is the shape the survey actually found — already produce
  **different resource kinds**. There was never a collision to prevent.
- Two versions in the **same controller and the same HTTP method** is the only real
  collision, and the exporter has handled it since ADR 0009: `ReasonActionCollision`
  omits every endpoint in a group whose grants disagree, and emits one correct rule
  where they agree. No over-grant is reachable from a collapsed version in either
  case.

The cost, measured rather than argued: implemented in the accepted form, the clause
emptied `nestjs-boilerplate`'s entire Cerbos export — all seven rules, across both its
controllers — because every controller in it declares `version: '1'`. Any project
using NestJS versioning uniformly would have exported nothing. That is a severe
regression bought for no safety gain, and the narrowing above keeps the principle
("don't export what you could not read") while dropping it.

Recorded here rather than silently revised, because the premise is the part worth
remembering: the argument was sound given what it assumed, and what it assumed was
never verified against `translate.go`.

### §8 Two controllers declaring the same path is unknown-whether-distinct, never silently one

When two controllers in one analyzed tree declare the same absolute route — after
§7, so genuine version pairs no longer reach this case — Sphinxor cannot know
whether a runtime prefix, a conditional registration, or a separate application
mount separates them.

The section has two halves, and they are conditioned differently. Keeping the
endpoints apart is **unconditional**, because it is the correctness fix. Telling the
user about it is **conditional**, because the measurement says most collisions have
nothing to tell.

**Unconditionally — neither endpoint is dropped and neither is merged.** Both are
extracted, both keep an identity (synthesized per §5 where the path-derived one would
collide), and each keeps only its own guards. This is what restores
`mutating-endpoint-without-access-control` on yudao's `AppMemberUserController`, and
it must not be made conditional on anything: a collision whose guards look identical
today because none were recognized is exactly the case where an endpoint would
silently disappear, which is the defect being fixed.

**Conditionally — the run warns only when the colliding endpoints' guards differ.**
Same guards on both sides means there is nothing to bleed and the warning would carry
no information. Differing guards — yudao's `@PreAuthorize` on one side and nothing on
the other — is the dangerous case, and there the warning is essential. It names the
colliding paths and the controllers that declare them, and states that the tool
cannot tell whether they are the same endpoint.

**`sphinxor export cerbos` omits a colliding endpoint** regardless of the warning
criterion, per ADR 0009 §3: a policy must not be named after a route whose real
prefix is unknown, and whether the two sides' guards happen to match says nothing
about whether the path is right.

This is deliberately *not* framed as per-application scoping. The measurement says
application boundaries are neither necessary nor sufficient: yudao's colliding pair
is in one application and one Maven module, and petclinic's four applications
collide on nothing.

### Why the criterion is guard difference, and the objection to it

Warning on every collision was the first draft of this section, and it does not
survive the measurement it was written from. After §7 removes the version pairs it
would fire on `tutorials` (202), `shenyu` (43, all in example applications), immich
(11), novu (6), `amplication` (2) and `ever-gauzy` (1) — every one of them a category
the same survey proved benign, with zero guard bleed in every production instance.
That is not a loud failure, it is a wall nobody reads, and it lands on the noise floor
this project pins deliberately (`TestAnalyzeDirectory_RealProjectStaysQuiet`,
`internal/cli/analyze_test.go`, which exists to catch §1–§4's caveats becoming
meaningless through overuse). A caveat that fires mostly where nothing is wrong
teaches the reader to skip it, and then it is not there when yudao needs it.

Conditioned on guard difference, the same survey leaves **47 warnings in
`ruoyi-vue-pro` and 13 in `tutorials`, and none anywhere else** — which is, precisely,
the set of collisions where an endpoint is reported carrying protection it does not
have.

**The obvious objection, stated rather than left implicit**: for immich and novu,
"the guards are the same" currently means "no guards were recognized on either side",
because their authorization runs through composite decorators extraction cannot read
at all. The equality is a measurement artifact, not a fact about those applications.

This is accepted, and it is arguably the right behaviour rather than a tolerated
weakness. The criterion tracks *what the tool actually knows*, which is the only
honest basis available to it: Sphinxor warns when it can see a difference that would
bleed, and stays quiet when it cannot see one. If composite-decorator support
improves and real differences surface in those repositories, the warning wakes up on
its own, without this decision being revisited. The failure mode is a warning that
arrives late, never one that asserts safety — and the endpoints stay separated
throughout, by the unconditional half above, so nothing is hidden in the meantime.

## Alternatives considered

- **Per-application scoping, an `--analyzed-root` option, or detecting the
  multi-application case specifically** — the three options `docs/limitations.md`
  listed. Rejected on the measurement: multi-application collisions occur in 7 of 17
  repositories and caused real guard bleed in **none** of the production ones, while
  every dangerous collision found is inside a single application, where none of the
  three would apply.
- **Fold version into the path** — rejected; novu and cal.com take opposite paths
  from the same decorator shape, so the decorator does not determine the route.
- **Parse `app.enableVersioning(...)` now, to reconstruct URI-versioned paths** —
  rejected as scope creep, exactly as Amendment 1 rejected resolving route
  constants. §7 makes the tool correct while the real path is unknown; parsing
  bootstrap makes "unknown" rarer. Bundling them would make the safety fix wait on
  the coverage feature.
- **Parse `setPathPrefixes` / `WebMvcRegistrations`** — rejected as categorically
  out of scope, with ADR 0012's custom `AuthorizationManager`. The predicate is
  arbitrary Java over package names and the prefixes come from configuration
  properties.
- **Warn on every same-path collision, not only guard-differing ones** — rejected
  during review; see §8. It fires on 324 collisions across the survey, essentially
  all of them in the category the same survey proved benign, against 60 under the
  accepted criterion. The cost is not noise in the abstract: it is the caveat
  mechanism losing its meaning, which `TestAnalyzeDirectory_RealProjectStaysQuiet`
  exists to prevent.
- **Condition the *structural* fix on guards differing too** — rejected, and this is
  the line that matters in §8. Only the warning is conditioned. Splitting the
  endpoints only when a difference is visible would leave an endpoint silently
  dropped in exactly the case where extraction sees no guards — which is immich's
  and novu's situation today, and which is the defect, not a safe state.
- **Treat a same-path collision as a `High`-confidence finding rather than a
  project-level warning** — rejected. `High` gates CI, and the measurement says the
  common case is benign; a gate that fires on immich's maintenance worker and
  shenyu's example applications is the CI-disabling false positive `docs/vision.md`
  warns about. The uncertainty is about the model, not about a specific endpoint's
  protection, which is what the project-level notice mechanism (ADR 0019 §2) is for.
- **Leave H documented and fix only G** — rejected. H is where the largest measured
  over-reporting is (47 groups in one repository, 3 mutating endpoints losing their
  finding), and "detect and say so" is this project's standing answer for input it
  cannot read. Documenting it a second time without acting would repeat the mistake
  Amendment 1's preamble describes.

## Consequences

- `model.Endpoint` gains a version component and a mark for an unresolved version,
  in the shape §5 already established for paths. `internal/model` gains the identity
  constructor taking it. Both extractors gain version reading. Additive.
- **Endpoint counts rise again**, as in Amendment 1: cal.com gains 15 endpoints,
  novu 19, yudao 59, shenyu 75, `tutorials` 481, none of which exist in today's
  reports. Any project whose count rises was being under-reported silently.
- **Findings appear that did not before**, and they are true: yudao's three unguarded
  mutating endpoints are the concrete ones. Stated honestly, because §8's conditional
  warning does not remove it: the Spring un-drop also surfaces genuinely-unguarded
  mutating endpoints that a same-path sibling was shadowing — up to 18 in `shenyu`
  and 64 in `tutorials`, all `Low` and non-gating. They are correct, they are what
  linting those applications separately already reports, and suppressing them would
  reintroduce the silent drop. The noise floor is defended by the warning criterion,
  not by keeping real endpoints out of the report.
- **The §8 warning fires on 47 collisions in `ruoyi-vue-pro` and 13 in `tutorials`
  across the whole survey, and nowhere else** — the measured effect of the criterion,
  and the number to re-check if the criterion is ever revisited. Re-measured against
  the implementation, it reproduces exactly: 47, 13, and zero in immich, `shenyu`,
  cal.com, novu and `amplication`.
- One defect in the criterion was found by that re-measurement rather than by
  reasoning: comparing guard *multisets* made an endpoint merged from two handlers
  under ADR 0014 — which keeps each handler's own annotations, so an identical
  `@PreAuthorize` is recorded twice — look different from a colliding sibling
  requiring exactly the same thing. Protection is a set, not a multiset. It showed up
  as one unpredicted warning in `tutorials` (`GET /api/authorities`), is fixed, and
  has its own regression test. Worth recording as a reason to re-measure after
  implementing, not only before.
- §7 must land before §8, or §8's warning fires on every version pair — 26 of the
  survey's collisions are version pairs that stop being collisions once §7 exists.
  This ordering is a requirement, not a preference.
- Regression tests, at the bar set in ADR 0014 and reaffirmed above — each confirmed
  to fail against current behavior before being kept, with fixtures vendored per
  ADR 0005:
  - §7, NestJS, readable version: novu's `/subscribers` v1/v2 pair must produce two
    endpoints.
  - §7, NestJS, unreadable version: cal.com's `POST /v2/bookings` must produce two
    endpoints, and the 2024-04-15 one must **not** show `OptionalApiAuthGuard`.
  - §7, Spring: `spring-boot-5`'s `ProductController` with `version = "1.0"` and
    `version = "2.0"` on one path must produce two endpoints.
  - §7: no version declared anywhere must leave IDs, allowlist anchoring, and diff
    output byte-identical to today.
  - §8, Spring: yudao's `AddressController` / `AppAddressController` pair — both
    endpoints survive, the app-side one carries no roles, and
    `mutating-endpoint-without-access-control` fires on
    `PUT /member/user/update`. Today the app-side endpoint is absent from the report.
  - §8, NestJS: the immich shape — both endpoints survive, neither carries the
    other's guards.
  - §8, warning criterion, both directions: a collision whose two sides carry the
    same guards produces no warning; yudao's `@PreAuthorize`-versus-nothing pair
    produces one. And `TestAnalyzeDirectory_RealProjectStaysQuiet` must still pass
    unchanged — the existing pin on the noise floor is the regression test for this
    criterion, not a separate one.
- `docs/limitations.md`'s duplicate-route entry is rewritten to name these two
  causes instead of multi-application, to keep multi-application as an
  observed-but-benign case with the evidence, and to carry the measurement-limited
  caveat about immich and novu.
- The survey's remaining uncharacterized shape, recorded so it is not lost:
  **conditional controller registration** — novu's `organization.module.ts` returns
  `[EEOrganizationController]` or `[OrganizationController]` depending on a runtime
  check, so only one is ever mounted. Three collisions, identical class-level guards,
  no bleed observed. §8 covers it correctly by treating it as unknown; nothing
  further is proposed for it here.

---

# Amendment 3 — an **unresolved role list** recorded as an empty one

## Status

Accepted and implemented.

§10 was confirmed during review rather than changed: `permitAll()`/`denyAll()` keep
ADR 0017's behaviour, on the grounds that a settled decision should not be reversed
as a side effect of an unrelated fix, and that whether `empty-role` should fire on
`permitAll()` deserves its own ADR and its own measurement.

## Why this is an amendment to 0020 and not a new ADR, or an amendment to 0011

The mechanism corrected here was created by [ADR 0011](0011-spring-second-framework.md)
§1's guard/role fusion, and [ADR 0017](0017-declaresroles-excludes-isauthenticated.md)
already corrected that same fusion once. So there are three plausible homes, and the
choice is argued rather than assumed.

It belongs here because **the principle being applied is this ADR's, and this ADR
claimed the case in advance.** *The principle* above says a thing Sphinxor could not
analyze is recorded as *unknown*, never as *absent*, and states that it "is intended to
pre-decide the next member of this family rather than waiting for it to be found in
an audit." This is that next member, arriving exactly as predicted: an
*unreadable role list* recorded as an *empty role list*. The family's previous
members were a matcher, a layer, a path and a version; the only new thing here is
which field it happens to.

It does not belong in ADR 0011 because 0011 decides which framework is supported and
how its syntax maps to the model; it does not own what an unreadable thing means, and
the fix reaches into a framework-independent lint rule (`internal/lint/empty_role.go`)
that 0011 has no jurisdiction over. It does not deserve a new number because a new
number would restate this ADR's principle in order to apply it, which is what
amendments exist to avoid.

**How this differs from ADR 0017, which corrected the same fusion.** ADR 0017 moved
one SpEL shape out of `DeclaresRoles` because that shape has a *positive* meaning the
model can express: `isAuthenticated()` means "authenticated, any role", and
`AuthenticationRequirement` exists to say so. This amendment is the opposite case:
these shapes have no known meaning at all, because they were not read. ADR 0017 was
about a misclassification; this is about a missing distinction. That is why it does
not extend 0017's carve-out list — adding shapes to `DeclaresRoles: false` would
assert they require no role, which is precisely what is not known.

## Context

### I. A role list that could not be read is indistinguishable from one declared empty

`GuardApplication.DeclaresRoles` says "this guard carries the endpoint's role
requirement as a `RoleReference` list." The model records the list. It does not record
whether the list is empty *because the source says so* or *because extraction could
not read the source*. `internal/lint/empty_role.go` fires on the conjunction
`DeclaresRoles && zero RoleReferences`, which both states satisfy.

ADR 0011 §1 makes this collision far more likely on Spring than on NestJS, by design:
presence and role-check are fused, so *every* recognized `@PreAuthorize`/`@Secured`/
`@RolesAllowed` sets `DeclaresRoles: true` — including the ones whose content ADR 0011
§1 deliberately declines to parse. That same section is explicit that the narrow SpEL
subset will leave bean method calls and boolean combinations unresolved, and says they
land "in exactly the existing 'guarded, no role' bucket ADR 0010 already defined."
What it did not trace is that this bucket is also `empty-role`'s trigger.

### The rule's stated rationale is falsified, not merely incomplete

`internal/lint/empty_role.go` grades itself High on this reasoning:

> Confidence: High. Unlike the other two v0.1 rules, this doesn't depend on
> assumptions about code this extractor can't see: whether a specific role-declaring
> construct resolved zero roles is a syntactic fact, verifiable by reading that one
> location. There's no global-guard or missed-reference scenario that could fool it.

Every clause of that is wrong for this shape. It *does* depend on code this extractor
cannot see — the SpEL inside the string. "Resolved zero roles" is a fact about
Sphinxor's parser, not about the source. Reading that one location **disproves** the
finding rather than confirming it, since the requirement is sitting there in plain
text. And there is a scenario that fools it, now measured on four real projects.

This matters beyond the fix: a rule whose rationale has been disproved needs its
rationale rewritten, not just its behaviour patched. §9 requires both.

### What was measured

The Spring survey recorded in `docs/limitations.md` — 14 production repositories,
2,959 endpoints — produced **675 `empty-role` findings at High confidence, all false**,
concentrated in four projects:

| Repository | Findings | The shape that produced them |
|---|---:|---|
| `alibaba/nacos` | 392 | `@Secured(resource = …, action = ActionTypes.WRITE)` — named attributes, no string array |
| `yangzongzhuan/RuoYi-Vue` | 116 | `@PreAuthorize("@ss.hasPermi('system:user:edit')")` — bean method call |
| `elunez/eladmin` | 99 | `@PreAuthorize("@el.check('deploy:edit')")` — bean method call |
| `apolloconfig/apollo` | 68 | `@PreAuthorize(value = "@unifiedPermissionValidator.hasCreateNamespacePermission(#appId)")` — bean method call |

High confidence gates CI. **A user pointing `sphinxor lint` at RuoYi-Vue today gets a
failing build on 116 findings, every one of which names a permission the source states
plainly.** This is the reassuring-false-negative failure class inverted: not a missed
risk, but a confident accusation that the code disproves on sight — and the fastest
possible route to the tool being switched off, which `vision.md` names as the trap
that limited adoption of general-purpose SAST in this space.

Counted the other way: across all 14 repositories the run produced exactly two kinds
of finding — `mutating-endpoint-without-access-control` at Low, and this at High. So
this is the only unanalyzable construct in the corpus that **blocks**, where every
other gap in `docs/limitations.md` degrades to a Low-confidence flag or a
project-level warning.

The one other place a limitation yields a blocking finding is the block-comment entry
in `docs/limitations.md`, where a `sphinxor-allow` marker extraction cannot associate
produces `stale-allow-marker` at High. That one is *correct*: the exemption genuinely
did not apply, and the build failing is the intended self-announcing refusal. The
difference is the whole point — there, a High finding tells the developer something
true they need to act on; here, it tells them something the file in front of them
disproves.

### What is not affected, verified rather than assumed

- **`sphinxor export cerbos` is already correct here** and needs no change.
  `ReasonNoRole` already omits every endpoint that has a guard but no resolved role.
  Confirmed by running the exporter on RuoYi-Vue: **0 rules exported, 146 omissions**.
  The export side never trusted this state; only the lint rule did.
- **`mutating-endpoint-without-access-control` is unaffected.** It keys on the
  existence of a `GuardApplication`, not on its role list, and these guards are real.
- **NestJS is unaffected.** `@Roles()` with no arguments is a genuinely empty list and
  remains the rule's intended target, as does ADR 0006's composite exclusion.

## Decision

### §9 A role list that could not be read is unknown, not empty

**Add `GuardApplication.RolesUnresolved bool`.** It is `true` when a guard sets
`DeclaresRoles: true` and extraction could not resolve its role list to a set of role
literals — as distinct from resolving it to the empty set. It is a statement about
what extraction could read, never about what the application requires.

Set `true` for:

- `@PreAuthorize` whose argument is not a single string literal — a constant
  reference, a concatenation, anything the SpEL text cannot be recovered from.
- `@PreAuthorize` whose SpEL content is recognized as neither a role call
  (`hasRole`/`hasAnyRole`/`hasAuthority`/`hasAnyAuthority`), nor `isAuthenticated()`,
  nor `permitAll()`/`denyAll()` — that is, bean method calls, boolean combinations,
  and comparisons against `#parameter` or `authentication.name`.
- `@Secured`/`@RolesAllowed` whose arguments exist but yield no string literals —
  named attributes (nacos's `resource = …, action = …`), a constant reference, or an
  array none of whose elements are readable.

Set `false` — so `empty-role` still fires — for:

- `@Secured({})` / `@RolesAllowed({})`: an empty array literal is a role list the
  source genuinely declares empty. **This is the case the rule exists for, and it must
  keep firing.**
- A bare `@Secured` / `@RolesAllowed` marker annotation with no arguments at all.
- NestJS's `@Roles()` — unchanged in every respect.

**`empty-role` skips any `GuardApplication` with `RolesUnresolved`**, and its doc
comment is rewritten: the High grade is re-justified on the narrowed trigger (an
empty list the source states, which reading that one location does confirm), and the
falsification above is recorded there so the rationale cannot quietly drift back.

### §10 `permitAll()` / `denyAll()` keep their current treatment, deliberately

ADR 0017 drew a boundary and `TestExtractControllers_PermitAllStillDeclaresRoles`
pins it: `permitAll()` keeps `DeclaresRoles: true` and "still surfaces through
empty-role", on the audit's reasoning that a developer's `permitAll()` could itself
be the mistake. Folding it into §9 would overturn a settled decision as a side effect
of fixing an unrelated one.

It is therefore **excluded from §9**: `permitAll()` and `denyAll()` are recognized as
resolving to no role list rather than falling through as unreadable, which requires
naming them in the SpEL parser instead of letting them land in the unrecognized
bucket. Their behaviour does not change, and the existing test passes unmodified.

**This costs nothing, which was measured, not assumed**: across the four affected
repositories there are **zero** `@PreAuthorize("permitAll()")` or `denyAll()`
occurrences, so the 675 still go to zero with the boundary intact. Whether firing
`empty-role` on `permitAll()` is right at all is a live question and is left open
here rather than answered in passing.

### §11 The unknown is announced, not merely silenced

Suppressing the finding alone would satisfy this ADR's letter and break its spirit:
"unanalyzable is unknown, not absent" requires the run to *say* the picture is
incomplete, which is what §4 established for project-level uncertainty. A silently
empty `Roles` column is exactly the absent-looking output this ADR exists to stop.

So a run whose model contains any `RolesUnresolved` guard emits a project-level
warning in the established shape, naming the count and what it means: these endpoints
carry an access-control annotation whose requirement Sphinxor could not read, so the
`Roles` column **understates** them, and their emptiness is not evidence of anything.

The matrix additionally marks such a row's `Roles` cell `?` rather than `-`, following
the `…` and `@?` precedents from Amendments 1 and 2 — `-` reads as "none required",
which is the wrong claim.

## Alternatives considered

- **Downgrade `empty-role` to Low.** Rejected. It would unblock CI while leaving 675
  wrong findings in the report, and it would weaken the rule on the genuine
  `@Roles()`/`@Secured({})` case it was built for and is correct about. The problem is
  the trigger, not the grade.
- **Stop setting `DeclaresRoles: true` for unresolved annotations.** Rejected, and it
  is the tempting one. It would fix `empty-role` for free, but `DeclaresRoles` also
  drives the RBAC matrix's Guards/Roles split (ADR 0011 §1), so these annotations
  would migrate into the Guards column and read as protection-only guards carrying no
  role requirement — asserting something false instead of admitting something unknown.
  It would also silently change what ADR 0017 fixed.
- **Extend the SpEL subset to read the permission strings.** Rejected here as the
  wrong scope: `@ss.hasPermi('system:user:edit')` is mechanically trivial to read, but
  there is nowhere in ADR 0002's model to put a permission, which is the open question
  `docs/limitations.md` records and deliberately does not answer. This amendment must
  not depend on that being settled; it makes the tool honest about the gap, and closing
  the gap stays a separate decision.
- **Suppress the finding without warning.** Rejected — see §11.

## Consequences

- `GuardApplication` gains one boolean. No entity, no relationship, no export format
  changes. ADR 0002's model is untouched in shape.
- Four real projects stop failing CI for the wrong reason, and start carrying a
  warning that states the real one.
- One new way for the tool to be wrong, stated plainly: if extraction later learns to
  read a shape that currently sets `RolesUnresolved`, a genuinely empty role list
  inside that shape would begin firing `empty-role` where it previously did not. That
  is the safe direction — a finding appearing late rather than a wrong one appearing
  early — and it is the same late-but-never-wrong property Amendment 2 §8 accepted.
### Validation — measured, not predicted

Run against the real 14-repository Spring corpus rather than synthetic cases, per
`docs/testing.md`:

- **675 → 0** across nacos (392), RuoYi-Vue (116), eladmin (99) and apollo (68).
  All four went from exit code 1 to exit code 0: they no longer fail CI.
- Each of the four emits the §11 warning naming exactly the count it previously
  mis-reported, and the ten unaffected repositories stay silent.
- **No other repository changed in any respect** — endpoint counts, roles and every
  other finding are identical across all 14. thingsboard still records its 439
  role-carrying endpoints.
- `Pharmacy` and `blog-api` produce **byte-identical output** before and after.
- Every existing test passes **unmodified**, including the NestJS `@Roles()` case
  and `TestExtractControllers_PermitAllStillDeclaresRoles`.

**One vendored fixture did change, and it is confirmation rather than regression.**
`ruoyi-vue-pro` — vendored for Amendment 2's duplicate-route work, for reasons having
nothing to do with this — was itself carrying **5 instances of this false positive**,
all `@PreAuthorize("@ss.hasPermission('member:user:update')")`, and that fixture was
failing CI on them. No test asserted those findings, so nothing caught it. The shape
was sitting inside this repository's own corpus for as long as the fixture has
existed.

### On the regression test's form

The ADR draft called for a *fixture-backed* test. It is instead a unit test over
inline Java (`internal/extract/spring/roles_unresolved_test.go`), matching the
established pattern in `guards_test.go`, and this is a deliberate departure worth
recording. [ADR 0005](0005-test-fixture-provenance.md) notes the vendored corpus has
reached roughly 2,500 lines against a stated re-evaluation threshold of 3,000;
vendoring a new repository to pin a two-line syntactic distinction would spend that
budget badly. The empirical backing that a fixture would have provided is supplied by
the 14-repository corpus run above and by `ruoyi-vue-pro`, which already contains the
real shape.

The test keeps both halves of §9 in **one source file** — `@Secured({})` fires,
`@Secured(resource = …, action = …)` does not — because the failure being guarded
against is not "empty-role fires too often" but "the two cases get conflated again",
and a test covering only the unread side would pass just as happily if the fix
silenced both.
