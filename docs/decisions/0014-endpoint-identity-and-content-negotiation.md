# 0014. `produces` does not participate in Endpoint identity — merge at discovery

## Status

Accepted.

## Context

While building `internal/extract/spring`'s first cut (controller/endpoint
discovery), the vendored `blog-api` fixture's `CategoryRestController`
surfaced a real shape neither ADR 0002 nor ADR 0011 anticipated: two
distinct real Java methods, `categories()` and `categoriesAsProtobuf()`
(and their per-tenant counterparts `categoriesForTenant()`/
`categoriesAsProtobufForTenant()`), each carrying its own
`@GetMapping(path = "/categories", ...)`, differing only in the
annotation's `produces` attribute (`APPLICATION_JSON_VALUE` vs.
`APPLICATION_PROTOBUF_VALUE`). `model.NewEndpointID` (ADR 0002) derives
identity from `HTTPMethod` + `Path` alone, so these two real,
independently-declared handlers computed the same `Endpoint.ID`.

The first cut extracted both as separate `model.Endpoint` entries sharing
one ID and shipped that as a passing test — flagged as "no dedup policy
invented," but that framing understated the problem. A shared ID across
two distinct real entities is an identity collision, and identity is what
two already-merged, already-shipped features depend on:

- **`internal/diff`** (ADR 0007) identifies an endpoint across two analysis
  runs by this same key. Two real endpoints sharing an ID can't be told
  apart — a regression on one (e.g. the protobuf handler silently loses a
  guard the JSON handler keeps) could be masked by the other, exactly the
  silent false-negative ADR 0007 exists to prevent.
- **`internal/export/cerbos`'s `Translate`** (ADR 0009) groups by endpoint.
  Two same-ID endpoints with different guards could collapse into one rule
  or collide in a way `ReasonActionCollision` wasn't designed for — that
  reason arbitrates *sibling* endpoints that both know they're distinct and
  happen to share a Cerbos action; it was never meant to arbitrate two
  entities that already believe, incorrectly, that they're the same
  endpoint.

Leaving two entries sharing one ID is the worst of both available shapes:
neither genuinely distinct (nothing downstream can tell them apart) nor
merged (nothing combines what each contributes). That ambiguity needed its
own decision, not a note in a discovery PR.

## Decision

**`produces` does not participate in `Endpoint` identity. Handlers that
share `HTTPMethod` + `Path` and differ only in `produces` are merged into
one `Endpoint` at discovery time, in `internal/extract/spring`.**

Reasoning:

- **Architecturally, in Spring itself, content negotiation cannot be an
  authorization dimension.** `authorizeHttpRequests`'s `RequestMatcher`s
  (and its predecessor `AntPathRequestMatcher`/`PathPatternRequestMatcher`)
  evaluate only `HttpServletRequest`'s method and path — the security
  filter chain runs and reaches an allow/deny decision *before*
  `DispatcherServlet`'s `HandlerMapping` has resolved which specific
  `produces`-differentiated method will service the request. There is no
  real Spring mechanism by which the URL layer's decision could differ
  between two `produces` variants of the same path — this is a structural
  fact about the framework, not an empirical observation limited to the
  fixture at hand.
- **`produces` decides response representation, not who may access it.**
  Two representations of one resource at one path are, in the ordinary
  case, one resource for authorization purposes — the same reasoning that
  already lets `RoleReference` treat multiple roles from one annotation as
  one requirement rather than a new concept (ADR 0011 §1).
- **Checked, not assumed, before deciding this is safe for the case that
  drove it**: `CategoryRestController`'s four handlers carry zero
  `@PreAuthorize`/`@Secured`/`@RolesAllowed` annotations (confirmed by
  grep, not absence-of-evidence-as-evidence), and `blog-api`'s
  `SecurityConfig` sets `@EnableMethodSecurity(prePostEnabled = false)`
  project-wide, so even a stray annotation would be inert. Neither
  `/categories` nor `/tenants/{tenantId}/categories` matches any explicit
  `requestMatchers` rule (both fall to the trailing
  `.anyRequest().permitAll()`), so the URL layer is trivially identical for
  both merged pairs too. There is no divergence in the fixture that drove
  this decision, on either layer.

**Residual risk, named rather than silently designed around**: the method
layer's independence from `produces` is a real-code observation for this
fixture, not a Spring architectural guarantee the way the URL-layer claim
above is — a project could, in principle, put a different
`@PreAuthorize` on two `produces`-differentiated handlers of the same
route, and Spring's method security *would* enforce them independently
per handler (method interceptors run after the handler is resolved, unlike
the URL-layer filter). Merging would then need to combine two genuinely
different guard sets on one `Endpoint`, and the existing model has no
"these are alternatives for different representations, not both required"
layer for that — naively unioning them, the way today's `appendUniqueGrant`
already unions same-layer guards, would report the merged endpoint as
requiring *either* role, an over-grant relative to any single real
representation. This project's standing discipline is not designing
extraction logic ahead of real evidence it's needed (docs/testing.md); no
vendored fixture exhibits this, so no resolution is designed here. When
`internal/extract/spring`'s guard-extraction pass (PR 2) is built, it
attaches whatever guards each real handler carries to the shared
`Endpoint.ID` exactly as extracted; if a future real fixture ever shows
divergent guards across `produces` variants of one route, that's the
trigger for a further decision — the same pattern that added
`DeclaresRoles` and `AppliedAt` exactly when real evidence demanded them,
not before.

## Alternatives considered

- **Keep two entries sharing one `Endpoint.ID`** (the first cut's
  shipped state) — rejected: this ADR's own motivating problem. Neither
  distinct nor merged; `internal/diff` and `internal/export/cerbos` both
  depend on ID uniqueness in ways this silently violates.
- **Add a `produces`/content-type dimension to `Endpoint` identity**
  (`NewEndpointID` keyed on method + path + produces) — rejected: would
  make `produces` participate in identity for every framework, including
  NestJS (which has no equivalent concept at all), for a distinction that
  doesn't decide who may access a resource, only how the response is
  encoded. Also multiplies `Endpoint` rows for what a reviewer of the RBAC
  matrix or a Cerbos policy would reasonably expect to see as one route.
- **Pick one handler arbitrarily, drop the other** — rejected: silently
  discards a real, independently-declared handler's own annotations;
  if that handler ever carried a guard the kept one didn't, this would
  be a silent, undetectable coverage loss, not merely an unusual case.

## Consequences

- `internal/extract/spring/controllers.go`: handlers sharing `HTTPMethod` +
  `Path` are merged into one `Endpoint` at discovery, not appended as
  separate entries. `HandlerName` for a merged endpoint needs a defined,
  documented shape (e.g. the first handler encountered, in source order) —
  an implementation detail for the fix, not a further identity question.
- `internal/extract/spring/extract_test.go`: `TestExtract_BlogAPI`'s
  current assertion (two entries sharing one `Endpoint.ID`, checked
  explicitly) is now the case this ADR says shouldn't ship — updated to
  assert exactly one merged `Endpoint` per shared method+path instead.
- No change to `model.Endpoint`, `model.NewEndpointID`, `internal/diff`, or
  `internal/export/cerbos` — this ADR keeps `Endpoint` identity exactly as
  ADR 0002 already defined it (method + path), rather than amending it.
- `internal/extract/nestjs` is unaffected: NestJS route decorators have no
  `produces`-equivalent concept, so this shape cannot occur there.
