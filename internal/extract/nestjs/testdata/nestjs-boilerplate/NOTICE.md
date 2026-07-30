# Provenance

Vendored, unmodified, from https://github.com/brocoders/nestjs-boilerplate,
commit `549cc37a3925ab87a4e61b45efb3b86d2d8e234e` (2026-06-13), MIT License.

Used per docs/testing.md: empirical validation against real, representative
NestJS code, not just synthetic fixtures. This is a small subset of that
project's `src/`, chosen because it exercises real-world patterns the
synthetic unit tests don't: an object-literal `@Controller({ path, version })`
argument, `AuthGuard('jwt')` as a call-expression guard argument, a
string-keyed numeric enum (`'admin' = 1`), and — in `app.config.ts` — an
enum (`Environment`) with nothing to do with authorization, used as the
regression case for not treating every enum in a project as a role
declaration.

Files:

- `src/users/users.controller.ts` — class-level `@UseGuards` + `@Roles`
  expansion, object-literal controller path.
- `src/auth/auth.controller.ts` — real unguarded-by-design endpoints
  (login/register), verified by hand.
- `src/roles/roles.enum.ts` — the actual role registry, string-keyed.
- `src/config/app.config.ts` — an unrelated enum, for the role-enum noise
  filter regression test.
