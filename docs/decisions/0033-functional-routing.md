# 0033. Functional routing is announced by the methods that build it

## Status

Accepted.

## Context

[ADR 0029](0029-spring-security-scope.md) §3 lists functional routing —
`RouterFunction`, WebFlux's alternative to an annotated controller — as **silent**,
one of two items left.

Routes are built in a method rather than declared by annotation:

```java
@Bean
RouterFunction<ServerResponse> indexRouter(Resource indexHtml) {
    return route(GET("/"), request -> ok().bodyValue(indexHtml));
}
```

Nothing in the extractor reads any of it. It is Spring's own routing, not another
framework, so [ADR 0029](0029-spring-security-scope.md) §1 makes this a gap rather
than a non-goal.

### Why ADR 0019 §2's warning does not already cover it

It fires on halo, which yields zero endpoints from 1,001 files, and that made the
item look announced. It is not. The condition is `len(m.Endpoints) == 0` across the
whole project, so it reports *emptiness*, never an unread route shape. Measured:

| Repository | Recognized endpoints | ADR 0019 §2 warning |
|---|---|---|
| halo | 0 | fires — by accident of having nothing else |
| apache/shenyu | 394 | **does not fire** |
| JeecgBoot | 931 | **does not fire** |

Two of three repositories lose real routes in silence — shenyu's include
`POST /helloWorld2`, `GET /rewrite`, `GET /pdm`, `GET /oms`, `GET /timeout`.

### What to count

Three idioms appear, and the unit that spans all three is **a method whose
declared return type is `RouterFunction<…>`**:

- a `@Bean` method — JeecgBoot's `indexRouter`, halo's `WebFluxConfig` (14 of
  them), shenyu's three;
- an implementation of a project interface — halo's dominant shape, where
  `CustomEndpoint` declares `RouterFunction<ServerResponse> endpoint()` and ~68
  endpoint classes implement it with `SpringdocRouteBuilder.route()`;
- the interface declaration itself.

Counting `@Bean` methods alone would find 14 of halo's 82 files. Counting
`RouterFunctions.route(` calls would miss halo's springdoc builder, which is a
different entry point.

**Measured: roughly 81 builder methods across 3 repositories** — halo ~75, shenyu
5, JeecgBoot 1 — with 104 `route(`-family calls and 200 verb predicates in halo
alone. These are regex estimates and are flagged as such: the JeecgBoot method was
missed by the first scan because its parameter list contains parentheses, which is
exactly why the implementation uses tree-sitter. Per the exactness rule recorded in
[ADR 0024](0024-controller-meta-annotations.md), the implementation's count is the
one that goes in `docs/limitations.md`, and a divergence from ~81 is explained
rather than absorbed.

## Decision

### §1 Detect methods that build a `RouterFunction`; do not read the routes

A method whose declared return type is `RouterFunction` is recorded, with its
class, in a project-level status. The run announces the count.

**The number is phrased as indicative, not exact.** A method returning a
`RouterFunction` is not necessarily an independent route declaration — it can be a
fragment composed into a larger chain elsewhere, in which case two methods describe
one set of routes. So the warning reads "N method(s) building functional routes",
which is a count of what was found, and never "N route sources" or anything a
reader could take as a census.

The route definitions inside are **not parsed**. Reading them means following a
builder chain across `route(...)`, `andRoute(...)`, `nest(...)`, predicate
combinators and, in halo's case, a third-party springdoc wrapper — and resolving
handler references to methods. That is a resolution decision with its own cost,
and [ADR 0029](0029-spring-security-scope.md) §3 defines done as *not silent*,
explicitly not *read*.

### §2 No count of the routes

The announcement counts **builder methods**, never routes.

This follows [ADR 0032](0032-controllers-that-yield-no-routes.md) §1's rule, for
the same reason and with a sharper case: one `route(...)` chain declares any
number of routes, and halo's 104 chains carry at least 200 verb predicates. Any
number short of parsing the chain would be wrong, and a wrong count is worse than
none — a reader who sees "3 routes" stops looking.

### §3 It changes no finding and no export

No endpoint is created, so no rule fires and the exporter has nothing to omit.
One new project-level fact, one consumer, no per-endpoint state — the
[ADR 0011](0011-spring-second-framework.md) §1 `DeclaresRoles` hazard does not
arise.

### §4 It does not replace ADR 0019 §2's notice, and both may fire

On halo both warnings will appear: "recognized no endpoints" and this one. That is
correct and not redundancy — they answer different questions. The first says the
matrix is empty; the second says why, and would fire just as well on a project
whose matrix is full. Suppressing either would lose information the other does not
carry.

### §5 Identity is the import, as everywhere else

`RouterFunction` counts only when bound to
`org.springframework.web.reactive.function.server`, by import or by a
fully-qualified spelling — [ADR 0022](0022-annotation-identity-and-unrecognized-authorization.md)
§1 and [ADR 0025](0025-qualified-annotation-names.md) §3 applied to a type rather
than an annotation. The corpus contains no same-named type from another package,
so this costs nothing today and is the established rule rather than a new one.

## Alternatives considered

- **Count `@Bean` methods returning `RouterFunction`.** Rejected on measurement:
  14 of halo's 82 files. Its routes live in `CustomEndpoint` implementations,
  which are not beans of that type.
- **Count `RouterFunctions.route(` calls.** Rejected: misses halo's
  `SpringdocRouteBuilder.route()`, a different entry point producing the same
  thing, and counts chains rather than declarations in any case.
- **Parse the route definitions.** Rejected here per §1 — a resolution decision,
  and the definition of done does not require it. Worth revisiting on evidence
  that a project's authorization actually lives there; halo's does not, it lives
  in a reactive `SecurityWebFilterChain` that [ADR 0020](0020-unanalyzable-is-unknown-not-absent.md)
  §3 already announces.
- **Suppress ADR 0019 §2's notice when this fires.** Rejected per §4: the two
  answer different questions and the first is not made wrong by the second.
- **Treat it as out of scope, like JAX-RS.** Rejected per
  [ADR 0029](0029-spring-security-scope.md) §1: `RouterFunction` is Spring's own
  routing, so not reading it is a gap, and §1 reserves "out of scope" for other
  systems.

## Consequences

- `internal/model`: one project-level status — a count and the declaring classes.
  No per-endpoint state.
- `internal/extract/spring`: a scan for methods whose return type is
  `RouterFunction`, in the same per-file pass as the other project-wide scans.
- `internal/cli/analyze.go`: one new project warning.
- `internal/lint`, `internal/export/cerbos`, `internal/diff`: unchanged (§3).
- [ADR 0029](0029-spring-security-scope.md) §2 moves functional routing from
  `silent` to `detected and announced`, leaving **one** item: the nested
  `@RestController`.
- **Non-zero corpus effect**, so the regression bar is again not a no-op: exactly
  halo, shenyu and JeecgBoot gain a warning, the other seventeen stay
  byte-identical, and no endpoint, finding or export changes anywhere. halo gains
  a second warning alongside the one it already has (§4).
- `docs/limitations.md`'s functional-routing entry moves from *silent* to
  *announced*, and its figures are replaced with the implementation's.
