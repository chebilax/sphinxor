# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

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
