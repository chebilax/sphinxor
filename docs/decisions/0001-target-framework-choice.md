# 0001. Target framework for v0.1

## Status

Accepted.

## Context

Sphinxor's v0.1 scope (see [`../vision.md`](../vision.md)) is deliberately narrow: one framework, done properly, rather than six frameworks done shallowly. The framework choice determines the shape of everything downstream — the Tree-sitter grammar to parse, what an "authorization guard" looks like syntactically, and how faithfully the intermediate model (resources / actions / roles / permissions) can be built without guessing.

`vision.md` narrows the field to frameworks with **explicit, syntactically visible authorization constructs** (decorators/annotations), because that's what makes extraction tractable with Tree-sitter alone in v0.1 — Sphinxor openly does heuristic, confidence-graded analysis, not formal verification, and a framework where authorization is scattered across implicit conventions or deeply dynamic configuration would blow that budget immediately.

Two candidates were named in `vision.md`:

- **Spring** (Java/Kotlin) — `@PreAuthorize`, `@Secured`, `@RolesAllowed`, method- and class-level security annotations, Spring Security configuration.
- **NestJS** (TypeScript) — Guards (`@UseGuards`), custom decorators (`@Roles`, `@Public`), metadata reflection via `Reflector`.

## Decision

**NestJS.**

`vision.md` frames the choice as "whichever you have the most real expertise in," but the deciding factor here was testability rather than personal familiarity: in NestJS, guards and decorators (`@UseGuards`, `@Roles`, `@Public`) are attached locally, at the route or handler where they apply. Ground truth for a given endpoint can be established by reading that one location, which means test fixtures can be verified by hand, one route at a time, without cross-referencing project-wide configuration.

Spring's equivalent (`SecurityFilterChain`, global `HttpSecurity` rules) can override or complement method-level annotations from a separate configuration class, so establishing ground truth for a single endpoint can require reading both the annotation *and* the global config to know which one actually wins. That's a harder, more error-prone thing to verify by hand when building and validating a test corpus in v0.1 — exactly the empirical validation `testing.md` commits to. NestJS's locally-scoped model keeps that verification tractable.

## Alternatives considered

- **Spring** — mature, heavily annotation-driven, larger and more heterogeneous corpus of real-world open source repositories for empirical testing (per [`../testing.md`](../testing.md)). Rejected for v0.1: method-level security annotations can be layered (class + method) and overridden or complemented by global `SecurityFilterChain` / `HttpSecurity` configuration in a separate file, so establishing ground truth for a single endpoint by hand — which the v0.1 test corpus depends on — requires reconciling both sources rather than reading one local declaration.
- **Other frameworks named in the original (pre-sequencing) scope** — Django, Symfony, ASP.NET — rejected for v0.1 specifically because their authorization patterns lean more on implicit convention or runtime configuration than on syntactically visible decorators/annotations, making them a poor fit for a Tree-sitter-first approach at this stage. They remain candidates for v2 (see [`../roadmap-long-term.md`](../roadmap-long-term.md)).

## Consequences

NestJS becomes the reference implementation for the intermediate model (subject of a future ADR): the Tree-sitter grammar to integrate is TypeScript, and the extraction logic will target `@UseGuards`, `@Roles`/custom decorators, and `Reflector`-based metadata as the primary authorization surface.

Mistakes made in modeling this first framework's authorization surface will likely need to be revisited when a second framework is added in v2, since the intermediate model is meant to be framework-independent — so this choice is worth getting right rather than fast. In particular, Spring's global-config override pattern (rejected here for testability reasons, not because it's rare) will need to be accounted for when the model is generalized in v2 — it should not be designed away just because NestJS doesn't exhibit it.
