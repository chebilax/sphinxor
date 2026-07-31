# Provenance

Vendored, unmodified, from https://github.com/NarHakobyan/awesome-nest-boilerplate,
commit `8f1c0a8cded54a198ddbc23922aa755922d2b155` (2026-06-09), MIT License.

Used per docs/testing.md: empirical validation against real, representative
NestJS code, not just synthetic fixtures — and per the explicit goal of this
second repo, to exercise an extraction path the first fixture set
(`testdata/nestjs-boilerplate/`) never touches: a **custom composite
decorator**, `@Auth([...])`, instead of separate `@UseGuards()` +
`@Roles()` decorators.

`@Auth` (`src/decorators/http.decorators.ts`) is implemented as:

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
`@UseGuards()` decorator syntax on any class or method. This fixture set
was originally vendored (see PR #2) to demonstrate exactly that: Sphinxor's
extractor recognized only literal decorator call sites, so `@Auth(...)`
was invisible to it, and `POST /posts` — genuinely guarded — surfaced as
a false `mutating-endpoint-without-access-control` finding.

**Since [ADR 0006](../../../../docs/decisions/0006-composite-decorator-resolution.md)
shipped, that false positive is gone.** Extraction now follows a
composite decorator (one matching ADR 0006's bounded shape: a single
return path calling `applyDecorators(...)`) to its `Roles`/`UseGuards`
inner calls, substituting call-site arguments for the composite's own
parameters. `http.decorators.ts` had to be vendored alongside the two
controllers below for this to work at all — extraction needs the
composite's *definition* present in the project being analyzed, not just
its call sites, which is a real difference from how this fixture set was
originally described. `src/constants/role-type.ts` (the `RoleType` enum)
is vendored for the same reason: once `@Auth([RoleType.USER])` resolves
to a real `Roles(...)` application, `RoleType.USER`/`RoleType.ADMIN`
become real, resolvable role references, and resolving them needs the
enum's declaration site in scope.

This fixture set is now the **regression check** for ADR 0006, not a
demonstration of the gap: `extract_second_repo_test.go` asserts `POST
/posts` shows `AuthGuard`/`RolesGuard` and no longer appears among the
`mutating-endpoint-without-access-control` findings, and that
`GET /posts/:id`'s `@Auth([])` (an empty, but genuine, role list) does
not newly trigger `empty-role` — composite-resolved `Roles` applications
are deliberately excluded from that rule (see
`internal/lint/empty_role.go`).

Files, and why each was picked:

- `src/modules/auth/auth.controller.ts` — `POST /auth/login` and
  `POST /auth/register` have **no** guard of any kind in source (confirmed:
  `app.module.ts` registers no global `APP_GUARD` either) — genuine true
  positives for `mutating-endpoint-without-access-control`, not blind-spot
  artifacts. `getCurrentUser` (`GET /auth/me`) carries
  `@Auth([RoleType.USER, RoleType.ADMIN])`, exercising a composite call
  with two role arguments.
- `src/modules/post/post.controller.ts` — `POST /posts` carries
  `@Auth([RoleType.USER])`, genuinely guarded, now correctly resolved.
  `GET /posts/:id` carries `@Auth([])`, the empty-role-list case.
  `PUT /posts/:id` and `DELETE /posts/:id` carry no `@Auth` at all —
  genuine true positives, same as the auth endpoints above.
- `src/decorators/http.decorators.ts` — `Auth`'s own definition, required
  for composite resolution to have anything to resolve against.
- `src/constants/role-type.ts` — the `RoleType` enum `Auth`'s calls
  reference, required for those references to resolve to a declaration
  rather than staying an unresolved raw literal.
