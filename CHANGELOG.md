# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.6.0] - 2026-09-18

### Added

- **Spring is now an analyzable framework.** `sphinxor lint`, `sphinxor diff`, and
  `sphinxor export cerbos` all work on Spring projects, not just NestJS:
  - Endpoints from `@RestController`/`@Controller` classes and
    `@GetMapping`/`@PostMapping`/`@PutMapping`/`@DeleteMapping`/`@PatchMapping`
    handlers.
  - Method-security guards — `@PreAuthorize`, `@Secured`, `@RolesAllowed` —
    including a bounded set of SpEL shapes (`hasRole`, `hasAnyRole`,
    `hasAuthority`, `isAuthenticated`) and roles resolved against Java enum
    constants.
  - `SecurityFilterChain` URL-layer rules (`authorizeHttpRequests`), evaluated in
    real Spring first-match-wins order.
  - **The two layers are combined into the real effective policy**, not reported
    separately: an endpoint allowing `ADMIN` or `PHARMACIST` at the method layer
    but restricted to `ADMIN` by a URL rule is reported as `ADMIN`-only. What each
    layer can and cannot be read from is bounded in
    [ADR 0012](docs/decisions/0012-securityfilterchain-effective-policy.md) and
    [ADR 0018](docs/decisions/0018-unrecognized-rule-stops-evaluation.md), with the
    current blind spots listed in [`docs/limitations.md`](docs/limitations.md).
  - `// sphinxor-allow:` markers work in Java source exactly as in TypeScript,
    including the stale-marker finding when one doesn't sit above a recognized
    endpoint.
- **Framework auto-detection, with `--framework` to override.** Sphinxor picks the
  extractor by looking at your source, and reports which one it chose and why. It
  refuses rather than guesses: if it detects nothing, or detects more than one (a
  monorepo with a Java backend and a Nest BFF), it stops and asks for
  `--framework` instead of silently analyzing half your project
  ([ADR 0019](docs/decisions/0019-cli-framework-selection.md)).

### Changed

These affect existing NestJS users too, including ones with no interest in Spring.

- **A run that couldn't examine anything is now an error instead of an empty,
  clean report.** Pointing `sphinxor lint` at a path with no recognizable source
  previously printed `0 endpoint(s), 0 finding(s)` and exited `0` — indistinguishable
  from "your authorization model is fine." It now exits non-zero with
  `no supported framework detected in <path>`. **This can turn a currently-green CI
  job red if it was pointing somewhere unintended** — which is the point, but here
  is the recourse:
  - *Wrong path* (the usual cause): correct the path.
  - *Right path, but the directory legitimately has no framework import* — a
    DTO-only or service-only sub-path, for instance: pass `--framework nestjs`
    (or `spring`). The run then proceeds and exits `0`, warning that it parsed
    files but recognized no endpoints.
  - *Framework misidentified*: pass `--framework` with the one you want.

  A run that *does* parse source but recognizes no endpoints still exits `0`, with
  a warning — a library package legitimately has no routes.
- **A one-line notice on stderr** reports the framework, how it was chosen, and the
  source-file count. `stdout` is unchanged, so `--format json` output still parses
  exactly as before and existing consumers are unaffected.
- **Errors print once, without a usage dump.** A failed run shows the reason, not
  the command's full flag list.

## [0.5.0] - 2026-08-28

### Added

- **"Authenticated, any role" grants** ([ADR 0010](docs/decisions/0010-authenticated-any-role.md)):
  the model now has a positive `AuthenticationRequirement` fact for an
  endpoint guarded by a recognized authentication guard (`AuthGuard`) with
  no specific role resolved — previously indistinguishable from "extraction
  found nothing." `sphinxor export cerbos` exports these as Cerbos's
  documented `*` role grant (confirmed behaviorally, not just from docs),
  raising real coverage on the two vendored repos from 6 to 9 rules (7 to
  10 of 24 endpoints) without guessing: the recognized-guard-name set is
  small and evidence-based, deliberately excludes the literal-empty-`@Roles()`
  case `empty-role` already flags as a probable mistake, and an endpoint
  sharing a Cerbos action with a differently-guarded sibling still collides
  exactly as before.

### Fixed

- `sphinxor export cerbos --format json`'s companion report no longer
  collides with the generated policy directory — it's written as
  `<out>.report.json`, a sibling of `--out`, not nested inside it (a real
  `cerbos compile` against `--out` used to fail on the report file itself).

## [0.4.0] - 2026-08-27

### Added

- **`sphinxor export cerbos <dir> --out <policy-dir>`** — the first
  authorization-engine exporter ([ADR 0009](docs/decisions/0009-cerbos-exporter.md)):
  translates the normalized model into a Cerbos resource policy set.
  Downstream of extraction by construction (works for any future
  framework automatically). Output is explicitly not deploy-ready:
  a rule is only generated when a real `GuardApplication`/`RoleReference`
  establishes it; anything the model can't confirm (no guard, guarded
  with no specific role, or two endpoints whose confirmed roles disagree
  under the controller+method action mapping) is omitted, never guessed,
  and flagged both inline in the generated YAML and in a companion
  `export-report.md`/`.json`. Validated against the two vendored NestJS
  repos with the real `cerbos compile` CLI, not just structurally.
- `sphinxor version` (and `--version`), reporting the release tag on a
  release build, `dev` on a local build.
- CI: `make check` (build, vet, `gofmt -l`, tests) on every push and pull
  request; a release workflow building the `sphinxor` binary on tag push.
- `SECURITY.md`: private vulnerability disclosure process.

## [0.3.0] - 2026-08-03

### Added

- **`sphinxor diff <base-dir> <head-dir>`** — v1's headline differentiator
  (`docs/vision.md`, [ADR 0007](docs/decisions/0007-model-diff-design.md)):
  drift detection between two versions of a project's authorization model,
  not just a point-in-time audit.
  - Structural diff, always reported: added/removed endpoints, added/removed
    role declarations, added/removed guard applications and role references,
    and endpoints that became public.
  - Regression detection that gates CI: a new `High`-confidence finding, or a
    `High`-confidence finding that lost its `sphinxor-allow` exemption between
    the two sides. `Low`-confidence findings and unchanged pre-existing
    `High` findings never gate, on either side of the comparison.
  - Takes two pre-extracted directories rather than git refs — Sphinxor never
    shells out to `git`; see the README for the `git worktree add` CI pattern.
  - Markdown and JSON output, same convention as `sphinxor lint`.

## [0.2.0] - 2026-07-31

### Added

- Composite decorator resolution (`@Auth(roles)`-style wrappers around
  `applyDecorators()`), one level deep — [ADR 0006](docs/decisions/0006-composite-decorator-resolution.md).
  Closes the false-positive gap on endpoints protected only through such a
  wrapper, within the bounded shape the ADR describes; anything outside
  that shape remains a documented blind spot (`docs/limitations.md`).

## [0.1.0] - 2026-07-31

### Added

- NestJS extraction: controllers, endpoints, `@UseGuards`/role decorators,
  declared roles and permissions, normalized into the intermediate model
  ([ADR 0002](docs/decisions/0002-intermediate-model-structure.md)).
- The three v0.1 lint rules: mutating endpoint (`POST`/`PUT`/`PATCH`/`DELETE`)
  with no detected access control; permission declared but never referenced;
  empty role.
- The `sphinxor-allow` comment-marker allowlist mechanism, plus the
  stale-allow-marker finding for a marker that doesn't sit above a
  recognized endpoint — [ADR 0003](docs/decisions/0003-allowlist-format.md).
- Two-tier `High`/`Low` finding confidence, with `High` gating CI —
  [ADR 0004](docs/decisions/0004-confidence-level-granularity.md).
- `sphinxor lint`: extraction + the rule set + the RBAC matrix report
  (Markdown/JSON).
- Test corpus expanded to a second real NestJS repository with a different
  guard style, validating extraction against more than one project's
  conventions ([ADR 0005](docs/decisions/0005-test-fixture-provenance.md)).
- Initial documentation structure (`docs/`, `CONTRIBUTING.md`, `CHANGELOG.md`).
