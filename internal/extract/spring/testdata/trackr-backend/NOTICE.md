# Provenance

Vendored, unmodified, from https://github.com/techdev-solutions/trackr-backend,
commit `f985c5f16ee6e7edcec124f33d9f93ef01e4f99c` (2019-02-08), MIT License.
The upstream `LICENSE` is copied alongside, as that license's notice condition
requires.

Used per docs/testing.md and docs/decisions/0038-role-hierarchy-read.md: the
**role hierarchy** fixture. It is the only application in that ADR's sample of
fifteen whose matrix shows roles next to a hierarchy written in source, which is
what makes it the case the decision turns on rather than a parse test.

Files, and why each was picked:

- `.../core/security/MethodSecurityConfiguration.java` — the hierarchy. A
  `@Bean` returning `RoleHierarchy`, built by `setHierarchy(...)` on a
  `RoleHierarchyImpl` from one string literal, and wired explicitly into the
  method-security expression handler with `setRoleHierarchy(...)` under the
  legacy `@EnableGlobalMethodSecurity(prePostEnabled = true)`. The class carries
  `@Profile("granular-security")`.

  The literal is
  `"ROLE_ADMIN > ROLE_SUPERVISOR ROLE_SUPERVISOR > ROLE_EMPLOYEE ROLE_EMPLOYEE > ROLE_ANONYMOUS"`:
  three pairs on **one line**. Spring Security up to 5.0 reads that as three
  edges; from 5.1 it reads each line as one chain, so this becomes one
  malformed chain whose middle "roles" contain spaces and can never match an
  authority, and `ROLE_ADMIN` reaches only `ROLE_ANONYMOUS` at its end. The project
  builds on Spring Boot 1.5.19, so Spring Security 4.2, so the first reading is
  the one it runs — and nothing in these files says so. That is the point of
  the fixture.
- `.../domain/employee/vacation/VacationRequestController.java` — three
  endpoints the hierarchy would touch: `PUT /vacationRequests/{id}/approve` and
  `PUT /vacationRequests/{id}/reject` carry `hasRole('ROLE_SUPERVISOR')`, which
  a `ROLE_ADMIN` holder also reaches under the 4.2 reading;
  `GET /vacationRequests/daysPerEmployeeBetween` carries `hasRole('ROLE_ADMIN')`,
  the top of the hierarchy, which nothing further implies. One row that the
  hierarchy widens and one it cannot, in one file.

The other trackr controllers with roles (`AddressBookController`,
`BillableTimeController`, `WorkTimeController`, `InvoiceController`) add no shape
this pair lacks, and `AddressBookController`'s path does not resolve, which would
test ADR 0020 Amendment 1 rather than this decision.
