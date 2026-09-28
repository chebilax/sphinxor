# Provenance

Vendored, unmodified, from https://github.com/conductor-oss/conductor,
commit `9ba8af910651f4dcbc4409f3ea2700a5fdb01037`, Apache License 2.0.
The upstream `LICENSE` is copied alongside, as section 4(a) of that license
requires; the repository has no `NOTICE` file for section 4(d) to carry over.

Used per docs/testing.md and docs/decisions/0039-route-path-constants.md §8:
the **static-import** and **placeholder** fixture for route paths.

Files, and why each was picked:

- `rest/.../controllers/AdminResource.java` — `@RequestMapping(ADMIN)`, where
  `ADMIN` arrives through `import static ...RequestMappingConstants.ADMIN`. It is
  the shape behind 106 of conductor's 150 constant-path routes, and the one ADR
  0038's simple-name index could not resolve.
- `rest/.../config/RequestMappingConstants.java` — the constants: interface
  fields, implicitly `static final`, built by concatenation
  (`ADMIN = API_PREFIX + "admin"`), so the path is `/api/admin`.
- `ai/.../a2a/server/A2AServerResource.java` — a mapping path that is a property
  placeholder, `${conductor.a2a.server.basePath:/api/a2a/workflow}`, resolved from
  configuration at runtime. Before ADR 0039 it was reported verbatim as the route
  `/${conductor.a2a.server.basePath:/api/a2a/workflow}`, which is a silently false
  path; now it is unresolved and says why (§3).

Three files from one repository, against ADR 0005 Amendment 1's trigger of about
twenty.
