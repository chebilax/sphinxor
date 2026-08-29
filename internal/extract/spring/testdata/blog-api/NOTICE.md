# Provenance

Vendored, unmodified, from https://github.com/categolj/blog-api,
commit `23ae16bab3f5a50fac932cb51efad5d9d9c41959` (2025-06-24), Apache-2.0
License.

Used per docs/testing.md and docs/decisions/0011-spring-second-framework.md:
the out-of-scope negative fixture — a real, actively maintained Spring app
whose access control is entirely programmatic. `@EnableMethodSecurity(prePostEnabled
= false)` explicitly disables method-level `@PreAuthorize`/`@Secured`/`@RolesAllowed`
project-wide (confirmed: zero occurrences of any of the three across every
`web` controller in the project, not just the vendored subset), and most
`SecurityFilterChain` rules call `.access(AuthorizationManager)` with a
custom, tenant-scoped authorization manager — the category ADR 0011 and
ADR 0012 both explicitly exclude, since resolving it requires executing
arbitrary Java, not parsing a declarative rule.

This fixture is used for a test asserting the out-of-scope case is honestly
reported as unresolved, never silently shown as unprotected — not for
positive extraction coverage.

Files, and why each was picked:

- `config/SecurityConfig.java` — the whole `authorizeHttpRequests` chain.
  Not uniformly opaque, and vendored specifically because it isn't: most
  rules use `.access(...)` (unrecognized, per ADR 0012 §1), but
  `/admin/import` and `/entries.zip` use `.hasAuthority(X)` — a
  *recognized* shape — sitting in the same rule chain as the unrecognized
  ones. First-match-wins evaluation and per-rule (not per-file) recognition
  both need to hold up against this real mix, not just against a file
  that's entirely one or the other.
- `admin/web/EntryImportController.java` — `POST /admin/import` matches
  `SecurityConfig`'s recognized `.hasAuthority("entry:import")` rule (should
  extract as a concrete, non-role `hasAuthority` grant — outside ADR 0012's
  `hasRole`/`hasAnyRole` role vocabulary, a distinct, correctly-unresolved
  case in its own right). `POST /tenants/{tenantId}/admin/import` matches
  `.access(importForTenant)` instead — same controller, same method-security
  posture (none), different URL-layer recognizability, in one small file.
- `category/web/CategoryRestController.java` — `GET /categories` and
  `GET /tenants/{tenantId}/categories` match none of `SecurityConfig`'s
  explicit rules at all (categories isn't `entries`, `admin/import`, or
  `entries.zip`) and fall to the trailing `.anyRequest().permitAll()` —
  genuinely, correctly unguarded, a true negative rather than a blind-spot
  artifact, with zero method-level annotations either (confirming
  `@EnableMethodSecurity(prePostEnabled = false)` isn't leaving anything
  dead behind for extraction to trip over).
