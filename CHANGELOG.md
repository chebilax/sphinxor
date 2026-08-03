# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
- Composite decorator resolution (`@Auth(roles)`-style wrappers around
  `applyDecorators()`), one level deep — [ADR 0006](docs/decisions/0006-composite-decorator-resolution.md).
- `sphinxor lint`: NestJS extraction (controllers, endpoints, guards, role
  declarations/references), the three v0.1 lint rules (mutating endpoint
  without access control, permission declared but unreferenced, empty role),
  the `sphinxor-allow` marker mechanism, and the RBAC matrix report
  (Markdown/JSON).
- Initial documentation structure (`docs/`, `CONTRIBUTING.md`, `CHANGELOG.md`).
