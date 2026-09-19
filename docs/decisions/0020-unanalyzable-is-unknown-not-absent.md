# 0020. A rule, matcher, or layer that could not be analyzed is unknown, never absent — generalizing ADR 0018

## Status

Accepted (§1–§4, implemented).

**Amendment 1 (§5–§6): Accepted.** See *Amendment 1* below. The accepted decision
is unchanged; the amendment extends its reach to two mechanisms the original text
did not examine.

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
