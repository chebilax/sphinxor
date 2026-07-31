# 0006. Composite decorator resolution (applyDecorators expansion)

## Status

Accepted.

## Context

`docs/limitations.md` names a confirmed-common, not hypothetical, blind spot: a project-defined decorator built with NestJS's `applyDecorators()` (e.g. `@Auth([RoleType.USER])`, which internally calls `applyDecorators(Roles(roles), UseGuards(AuthGuard(...), RolesGuard), ...)`) is invisible to this extractor. `Roles(roles)` and `UseGuards(...)` are plain function calls inside the composite's own body, not `@Roles()`/`@UseGuards()` decorator syntax at the call site — the extractor only recognizes literal decorator call sites, so an endpoint decorated with `@Auth(...)` looks identical to one with no guard at all. Verified on `internal/extract/nestjs/testdata/awesome-nest-boilerplate`: `POST /posts` is genuinely guarded, and is flagged anyway.

The Low confidence grade on `mutating-endpoint-without-access-control` exists precisely so this kind of gap produces a hedged warning, not a confident wrong claim — the design held up under real-world stress, per the last review. But "the confidence grade is honest about it" and "the signal is clean" are different bars. The goal stated for this work is the second one: the remaining Low findings should be genuine uncertainty, not a systematic miss with a hedge slapped on it.

This ADR is about *how far* to resolve that indirection — general-purpose interprocedural analysis is explicitly out of character for a tool `vision.md` commits to keeping heuristic and syntactic, not formally verified. The scope has to be bounded and stated, not open-ended.

## Decision

**Resolve exactly one level of composite-decorator indirection, with direct parameter pass-through substitution only, and one explicit exclusion to avoid trading one false-positive source for another.**

### What gets resolved

1. **Composite decorator definitions are collected project-wide**, before controller extraction (a new pass, alongside the existing role-declaration collection) — a composite can be defined in a different file than where it's used, same as role enums. A definition is recognized only in this exact shape:
   - A `function Name(...) { ... }` or `const Name = (...) => ...` (arrow, block- or expression-bodied).
   - **Exactly one** `return` (block-bodied) or **exactly one** expression (expression-bodied arrow) — no conditional branches, no multiple return paths. A composite with more than one return statement is not recognized at all; call sites using it fall back to today's behavior (invisible, Low-confidence flag) rather than being resolved incorrectly.
   - That single return expression must be a direct call to `applyDecorators(...)`.
   - Every parameter must be a plain identifier pattern (no destructuring) — a composite with a destructured parameter isn't recognized, same fallback.
2. **Inside `applyDecorators(...)`'s arguments**, only nested calls literally named `UseGuards` or `Roles` are extracted — the same two names already recognized at the top level. Everything else inside `applyDecorators` (`ApiBearerAuth()`, `UseInterceptors(...)`, etc.) is ignored, same as today.
3. **At a call site** using a registered composite (e.g. `@Auth([RoleType.USER])`), arguments are substituted into the composite's parameters **positionally, by direct pass-through only**: if an inner call's argument is a bare identifier matching a parameter name, it's replaced with the call site's corresponding argument node. No other dataflow is attempted — a parameter that's transformed before being passed to `Roles(...)`/`UseGuards(...)` (e.g. `Roles(roles.map(...))`) is not resolved; that inner call is skipped, same fallback as an unrecognized composite.
4. **If a substituted argument is an array literal** (e.g. `roles` resolves to `[RoleType.USER, RoleType.ADMIN]`), its elements are unpacked into individual role-reference candidates, each processed exactly as an individual `@Roles(...)` argument is today (via the existing `resolveRoleArg`). This matches the common `Roles(...roles: number[])` rest-parameter convention, where passing an array achieves the same effect as passing each element individually — this extractor cares about *which roles are named*, not about faithfully replaying the framework's own runtime metadata-array semantics.

Both `internal/extract/nestjs/roles.go`'s enum-usage filter (which enums count as "role enums" — see ADR 0002 and 0005's regression history) and `internal/extract/nestjs/controllers.go`'s guard/role-application extraction go through the same resolution step, so a role referenced only via a composite decorator is now recognized as "used" for both purposes consistently — today, `RoleType` in `awesome-nest-boilerplate` is filtered out entirely because nothing *literally* passes it to `@Roles()`; after this change, `@Auth([RoleType.USER])` counts.

### What doesn't get resolved (explicit non-goals)

- **Multi-level composite chains** (a composite calling another composite that calls `applyDecorators`). One level only.
- **Conditional or branching decorator construction** (`if (...) return X(); return applyDecorators(...)`). Falls back to unresolved.
- **Non-trivial dataflow**: transformed, computed, or spread (`...roles`) arguments passed into the inner `Roles`/`UseGuards` calls. Falls back to unresolved for that specific inner call.
- **Local variables closed over from the composite's own body** (e.g. `const isPublicRoute = options?.public` used inside `AuthGuard({ public: isPublicRoute })`) are not resolved — but this doesn't need to be, since guard *name* extraction (`guardArgName`) only reads the called function's name (`AuthGuard`), never its argument values.

A composite (or a specific call within one) that falls outside this bounded shape is not silently mishandled — it's simply not recognized, which means the endpoint using it keeps today's behavior: invisible guard, Low-confidence flag. That's the honest, safe default this whole mechanism is built on top of, not a regression.

### The one exclusion: composite-resolved guards never trigger `empty-role`

Composite decorators commonly give their roles parameter a default of `[]` (`awesome-nest-boilerplate`'s `Auth(roles: RoleType[] = [], ...)`), meaning `@Auth([])` — or `@Auth()` — is a deliberate, documented way to say "authenticated, no specific role required," not a developer mistake. `empty-role`'s existing rationale ("a bare `@Roles()` with zero arguments is a likely-forgotten role list") does not transfer to this case: distinguishing "deliberate default" from "developer error" would require understanding the composite's *own* parameter-default semantics, which is exactly the kind of dataflow this ADR just scoped out.

Rather than guess, `GuardApplication` gains one new field: `FromComposite bool`, set `true` when a `GuardApplication` was produced via this resolution mechanism rather than a literal decorator. `empty-role`'s `Check` is updated to skip any `GuardApplication` with `FromComposite == true` — the rule keeps firing on genuine bare `@Roles()` calls (still High confidence, still a syntactic fact), and simply doesn't have an opinion about composite-resolved empty role lists.

**Orientation is deliberate, not incidental**: `false` is the zero value and means the normal, literal-decorator path — every `GuardApplication` built the way extraction has always built them (directly from `@UseGuards()`/`@Roles()`) gets the correct, safe behavior with no explicit initialization anywhere in the existing code. Only the new composite-resolution path has to actively set `true`. Getting this backwards (`true` as zero-value meaning "from composite") would have made every existing, already-correct call site silently and incorrectly exempt from `empty-role` the moment the field was added — exactly the kind of bug a zero-value orientation choice should prevent, not cause.

This is a real, if small, extension of [ADR 0002](0002-intermediate-model-structure.md)'s model — noted there explicitly (see its Consequences section) as well as here, rather than folded in silently, per the project's own rule against that. `mutating-endpoint-without-access-control` and `permission-declared-but-unreferenced` need no rule-logic changes: they already just consume whatever `GuardApplication`/`RoleReference` rows exist, and get more complete input as a result of this work, without needing to know where those rows came from.

## Alternatives considered

- **Do nothing, leave it as a documented limitation indefinitely.** Rejected: the task that motivated this ADR is specifically "the remaining Low findings should be genuine signal, not artifacts of incomplete parsing" — and this pattern is now confirmed common on real code, not a rare edge case worth permanently writing off.
- **General interprocedural / dataflow analysis** (resolve arbitrary transformations, multi-level chains, conditional construction). Rejected as disproportionate: this is the kind of engineering effort formal verification tools take on, which `vision.md` explicitly positions Sphinxor against. It would also make the extractor's behavior much harder to explain and trust — "why did this resolve and that didn't" needs a simple answer.

## Consequences

- `internal/extract/nestjs` gains a new collection pass (composite decorator definitions, project-wide) and a shared resolution step used by both the enum-usage filter and guard/role-application extraction, replacing their current independent, literal-name-only matching.
- `model.GuardApplication` gains `FromComposite bool`.
- Expected, hand-verifiable effect on the two vendored fixtures once implemented: `POST /posts` in `awesome-nest-boilerplate` stops being flagged by `mutating-endpoint-without-access-control` (the target false positive); `RoleType.USER`/`RoleType.ADMIN` become recognized, referenced role declarations; `GET /posts/:id`'s `@Auth([])` does not newly trigger `empty-role`. `testdata/nestjs-boilerplate` (no composite decorators at all) should be entirely unaffected — its existing test assertions are the regression check.
- A composite decorator that doesn't fit the bounded shape above remains exactly as invisible as it is today — this ADR narrows the blind spot, it doesn't claim to close it. `docs/limitations.md` gets updated to reflect the new, narrower boundary once this ships, not to declare the problem solved.
