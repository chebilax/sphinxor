# Provenance

Vendored, unmodified, from https://github.com/Kitty-Hivens/Pharmacy,
commit `f5b760f4a47b6bc48b72757c3a43744d57a4cae8` (2026-07-28), MIT License.

Used per docs/testing.md and docs/decisions/0011-spring-second-framework.md:
the conventional, in-scope Spring fixture — a real, standard-practice
Spring Security app with visible `ADMIN`/`PHARMACIST` RBAC, `@PreAuthorize`
on 19 of 21 endpoints, and a plain, simple-pattern
`SecurityFilterChain` (no custom `AuthorizationManager` anywhere). This
is the repo that drove [ADR 0012](../../../../../docs/decisions/0012-securityfilterchain-effective-policy.md):
`SupplierController.GET`'s method annotation (`hasAnyRole('ADMIN',
'PHARMACIST')`) and its matching `SecurityFilterChain` rule
(`hasRole('ADMIN')`) genuinely disagree, and Spring AND-combines them,
so the real effective policy is `ADMIN`-only — an extractor reading only
the method annotation would report a confidently wrong, permissive
answer.

Files, and why each was picked:

- `config/SecurityConfig.java` — the whole app's `authorizeHttpRequests`
  chain: `permitAll()` (`/auth/**`, `/v3/api-docs/**`,
  `/api/v1/secret/**`), `hasRole()`/`hasAnyRole()` rules scoped by path and
  by `HttpMethod`, and the trailing `.anyRequest().authenticated()`
  catch-all — the full recognized-shape vocabulary ADR 0012 §1 describes,
  in one real file. Also carries `@EnableMethodSecurity` (present, not
  disabled — the opposite of `blog-api`'s `SecurityConfig.java`) and a
  real developer comment documenting a since-fixed URL-prefix bug,
  independently verified against the controllers' actual `@RequestMapping`
  paths before being trusted.
- `controller/SupplierController.java` — the endpoint that drove ADR 0012:
  `GET /api/suppliers` is method-`{ADMIN,PHARMACIST}` /
  URL-`{ADMIN}`, a subset-reducible but genuinely non-identity
  intersection, real effective policy `{ADMIN}`.
- `controller/CustomerController.java` — every endpoint carries
  `@PreAuthorize`, and `/api/customers/**` matches none of
  `SecurityConfig`'s explicit `requestMatchers` rules, so the URL layer
  falls through to `.anyRequest().authenticated()` — a real occurrence of
  the method layer holding the concrete role and the URL layer being the
  universal, unconstraining one (`internal/export/cerbos`'s intersection
  identity-element case, `TestTranslate_CustomerControllerShape`). An
  earlier version of that test wrongly claimed the reverse shape (no
  method annotation at all) — corrected after rereading this file
  directly, not assumed from memory.
- `controller/AuthController.java` — `POST /auth/login` has no guard of
  any kind (method or URL: `/auth/**` is `permitAll()`) — a genuine,
  by-design unguarded endpoint, not a blind-spot artifact.
- `model/Role.java` — the actual role registry: exactly two values,
  `ADMIN` and `PHARMACIST`. Confirms (not assumes) that no real occurrence
  of the effective-policy intersection's empty-set or genuine
  non-subset-partial-overlap branches exists anywhere in this app — every
  rule in the whole codebase requires `ADMIN` alone or
  `ADMIN`-or-`PHARMACIST`, so `ADMIN` is always common to both layers
  whenever both are concrete. Those two branches
  (`internal/export/cerbos/effective_policy_test.go`'s
  `TestTranslate_PartialOverlapIntersectsToSharedRole` and
  `TestTranslate_DisjointLayersProduceNoCommonRole`) stay synthetic-only
  per docs/testing.md's real-fixture-vs-pure-logic principle — this file
  is the evidence for that decision, not an oversight in fixture
  selection.
