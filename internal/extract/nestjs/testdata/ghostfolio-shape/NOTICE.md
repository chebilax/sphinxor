# Provenance

**This fixture is a reduction, not a copy. No upstream bytes are vendored here.**

Shape derived from https://github.com/ghostfolio/ghostfolio, commit
`dcf622aa590dde42c1f6b35f5d121c9f49a20cdc` (2026-09-20), **AGPL-3.0**, whose
`apps/api/src/decorators/requires-scope.decorator.ts` declares:

```ts
export function RequiresScope(...requiredScopes: Scope[]) {
  return applyDecorators(
    SetMetadata(REQUIRES_SCOPE_KEY, requiredScopes),
    UseGuards(AuthGuard('jwt'), HasPermissionGuard, ImpersonationGuard, ScopeGuard),
  );
}
```

## Why the bytes are not vendored

Every other fixture in this repository is MIT or Apache-2.0, matching Sphinxor's
own MIT license. ghostfolio is AGPL-3.0. Vendoring it would make the tree no
longer uniformly permissive — a durable licensing change for a regression anchor
on a one-character parser bug. The files beside this notice were therefore
written for this repository and reproduce the *shape* under test: a rest
parameter in the composite's own signature, a single unconditional return
calling `applyDecorators(...)`, a `SetMetadata` that consumes the rest parameter,
and a `UseGuards(...)` mixing a call-form guard (`AuthGuard('jwt')`) with plain
identifiers.

This is a **deliberate departure from `docs/testing.md`'s real-fixture bar**,
recorded rather than taken silently, and the test that uses it says so in its own
comment as that document requires. The departure is narrow, and the empirical
half of the bar was met before it was taken: the fix was validated against the
real repository, not against these files.

## The real-repository measurement this fixture stands in for

Run against real ghostfolio (`apps/api/src`, 118 endpoints), recorded in
[ADR 0006](../../../../docs/decisions/0006-composite-decorator-resolution.md)
Amendment 1:

| | before | after |
|---|---:|---:|
| endpoints carrying recognized guards | 78 | 105 |
| `mutating-endpoint-without-access-control` findings | 13 | 0 |

All 13 were false positives on endpoints genuinely guarded by `@RequiresScope`.
Re-running the other ten surveyed repositories changed not one guard and not one
finding. The cause was isolated to the rest parameter itself: with
`...requiredScopes` the endpoints report no guards, and with `requiredScopes` the
same file resolves `AuthGuard` and `ScopeGuard`.

## Files, and why each exists

- `src/requires-scope.decorator.ts` — the composite. Its rest parameter is what
  used to disqualify it; its `SetMetadata(REQUIRES_SCOPE_KEY, requiredScopes)`
  is the reason the rest parameter is never referenced by an inner
  `UseGuards`/`Roles` call, which is why recognizing the composite is sufficient
  and substituting the parameter is not required.
- `src/watchlist.controller.ts` — three endpoints carrying `@RequiresScope(...)`,
  two of them mutating, plus one deliberately undecorated `POST /watchlist/import`
  so the fixture also proves `mutating-endpoint-without-access-control` still
  fires where it should, rather than only that it stopped firing where it
  shouldn't.
