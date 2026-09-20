# Provenance

Vendored, unmodified, from https://github.com/eugenp/tutorials,
commit `f180d9273f5ed1eef5ec8f07a0da9acfdb27f6a4` (2026-09-18), MIT License.

Used per docs/testing.md and docs/decisions/0020-unanalyzable-is-unknown-not-absent.md
Amendment 2 §7: the **Spring version** fixture, establishing that §7 is not a
NestJS-only concern.

Files, and why each was picked:

- `spring-boot-modules/spring-boot-5/src/main/java/com/baeldung/apiversions/header/ProductController.java`
  — Spring's native API versioning: `@GetMapping(value = "/{id}", version = "1.0")`
  and `@GetMapping(value = "/{id}", version = "2.0")`, two handlers on one path,
  **in a single controller in a single file**. Extraction reads `path`/`value`
  and steps over `version`, so the two collapse into one endpoint and — Spring
  dropping rather than merging — the second disappears from the report entirely.

Chosen deliberately as the smallest possible honest case: 36 lines, one file, no
surrounding application needed, and the version is a plain string literal, so it
exercises the *readable* branch of §7 on the Spring side while cal.com exercises
the unreadable branch on the NestJS side.

This repository is a tutorial corpus, not a production application. It is
vendored only for this mechanism, which it demonstrates in its clearest form;
nothing about protection modelling is claimed from it.
