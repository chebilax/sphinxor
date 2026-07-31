# Provenance

Vendored, unmodified, from https://github.com/NarHakobyan/awesome-nest-boilerplate,
commit `8f1c0a8cded54a198ddbc23922aa755922d2b155` (2026-06-09), MIT License.

Used per docs/testing.md: empirical validation against real, representative
NestJS code, not just synthetic fixtures — and per the explicit goal of this
second repo, to exercise an extraction path the first fixture set
(`testdata/nestjs-boilerplate/`) never touches: a **custom composite
decorator**, `@Auth([...])`, instead of separate `@UseGuards()` +
`@Roles()` decorators.

`@Auth` (`src/decorators/http.decorators.ts`, not vendored — the
decorator's *definition* isn't relevant to extraction, only its call
sites are) is implemented as:

```ts
export function Auth(roles: RoleType[] = [], options?: ...): MethodDecorator {
  return applyDecorators(
    Roles(roles),
    UseGuards(AuthGuard({ public: isPublicRoute }), RolesGuard),
    ...
  );
}
```

`Roles(roles)` and `UseGuards(...)` are called here as plain JavaScript
function calls inside `applyDecorators(...)` — not as `@Roles()` /
`@UseGuards()` decorator syntax on any class or method in the vendored
files. Sphinxor's extractor only recognizes decorator *call sites*
(`@Name(...)` immediately preceding a class or method declaration), so
`@Auth(...)` is invisible to it: neither a guard nor a role reference is
extracted from it, even though the endpoint is genuinely protected.

This is a real, intentional demonstration of the blind spot documented in
`internal/lint/mutating_endpoint.go` and the PR that added this fixture —
not a bug to fix in v0.1. It's also why `permission-declared-but-unreferenced`
and `empty-role` cannot fire anywhere in this fixture set: neither file
contains a literal `@Roles()` decorator, so `RoleType` (the app's actual
role enum, `src/constants/role-type.ts`, not vendored — nothing here
references it via a recognized decorator) is filtered out entirely by the
same enum-usage heuristic documented in `roles.go`.

Files, and why each was picked:

- `src/modules/auth/auth.controller.ts` — `POST /auth/login` and
  `POST /auth/register` have **no** guard of any kind in source (confirmed:
  `app.module.ts` registers no global `APP_GUARD` either) — genuine true
  positives for `mutating-endpoint-without-access-control`, not blind-spot
  artifacts.
- `src/modules/post/post.controller.ts` — `POST /posts` carries
  `@Auth([RoleType.USER])`, genuinely guarded but invisible to this
  extractor (the blind-spot case). `PUT /posts/:id` and `DELETE /posts/:id`
  carry no `@Auth` at all — genuine true positives, same as the auth
  endpoints above.
