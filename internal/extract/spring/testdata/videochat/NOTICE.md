# Provenance

Vendored, unmodified, from https://github.com/nkonev/videochat,
commit `fd28bd4d1ac9d69a0ae1e7ae95671376ad259043` (2026-09-20), Apache License 2.0.
The upstream `LICENSE` is copied alongside, as section 4(a) of that license
requires; the repository has no `NOTICE` file for section 4(d) to carry over.

Used per docs/testing.md and docs/decisions/0038-role-hierarchy-read.md §11:
the parser's real input for a hierarchy built by **enum concatenation**, the
one readable form in that ADR's sample the trackr-backend fixture does not have.

Files, and why each was picked:

- `aaa/src/main/java/name/nkonev/aaa/security/SecurityRoleConfig.java` — the
  hierarchy. It is built as
  `UserRole.ROLE_ADMIN.name() + " > " + UserRole.ROLE_USER.name() + "\n"`,
  handed to `setHierarchy(...)` on a `new RoleHierarchyImpl()`, and set on the
  project's own `MethodSecurityExpressionHandler` bean under
  `@EnableMethodSecurity(prePostEnabled = true, securedEnabled = true)`. Two
  things only real code supplied: the trailing `"\n"`, which a decoder reading
  only a literal's first fragment would drop, and the wiring. That wiring is
  why `@PreAuthorize` receives the hierarchy on every version, while `@Secured`
  receives it only from Spring Security 6.3 (ADR 0038 Finding 2). Nothing in
  these files pins 6.3, so the run states that it assumed it.
- `aaa/src/main/java/name/nkonev/aaa/dto/UserRole.java` — the enum, in a
  different package from its use, which is what makes `.name()` resolve
  through the project-wide index rather than within one file.

The project's build file says Spring Boot 3.5.6, so Spring Security 6.5, where
the assumption holds. Sphinxor does not read build files, and the fixture
exists to show the run saying what it assumed rather than knowing.
