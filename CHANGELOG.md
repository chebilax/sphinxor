# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- **`empty-role` no longer fails the build on a role requirement Sphinxor
  could not read.** On Spring, presence and role-check are fused into one
  annotation (ADR 0011 §1), so an annotation whose content was outside the
  recognized SpEL subset — a bean call like
  `@PreAuthorize("@ss.hasPermi('system:user:edit')")`, or a same-named
  annotation from another framework such as alibaba's
  `@Secured(resource = …, action = …)` — was recorded as declaring *zero*
  roles rather than as declaring a requirement that went unread. That is
  `empty-role`'s trigger, at High confidence, which gates CI.

  A survey of 14 production Spring repositories measured **675 such
  findings across four of them** (nacos 392, RuoYi-Vue 116, eladmin 99,
  apollo 68), every one naming a permission the source states plainly. All
  four failed CI; all four now pass, with no other change to any
  repository in the corpus. The same fix cleared 5 instances from this
  project's own vendored `ruoyi-vue-pro` fixture.

  A role list that was *read and found empty* — `@Secured({})`, NestJS's
  `@Roles()` — still fires, unchanged: that is the case the rule exists
  for. `permitAll()`/`denyAll()` also still fire, deliberately, per
  ADR 0017. Affected endpoints now show `?` in the Roles column instead of
  `-`, and the run warns how many there are, because `-` claims no role is
  required while `?` says the requirement was not recovered. See
  [ADR 0020](docs/decisions/0020-unanalyzable-is-unknown-not-absent.md)
  Amendment 3.

### Security

Six blind spots, found by an audit of Spring and NestJS extraction against
real projects and by the NestJS hunt that followed it across six more
repositories (including immich and ToolJet). All of the same kind: a
construct Sphinxor could not analyze was recorded as *absent* rather than
*unknown*, and the report presented the result with confidence it hadn't
earned. See [ADR 0020](docs/decisions/0020-unanalyzable-is-unknown-not-absent.md)
and its Amendment 1.

- **A route's declared API version is now part of its identity.** NestJS's
  `@Controller({ path, version })` and `@Version()`, and Spring's `version`
  attribute on a mapping annotation, are route discriminators: two handlers
  sharing a path and differing only in version are two endpoints the running
  application routes separately. Extraction read the `path` key and stepped
  over `version`, so they collapsed onto one identity — NestJS merging them,
  so each was reported carrying the other's guards, and Spring dropping one
  outright. Found by measuring `docs/limitations.md`'s duplicate-route gap
  across 17 real repositories; hand-verified on cal.com, where one
  `GET /v2/event-types` requires authentication (`ApiAuthGuard`) and the other
  makes it optional (`OptionalApiAuthGuard`), and the merge misreported both.
  A version that cannot be read — a constant reference, or an array of them,
  which is the majority shape in real code — is treated as *unknown* rather
  than assumed equal to another unknown. An endpoint declaring no version
  keeps exactly the identity it had before, so existing allowlist anchors and
  diff baselines are untouched. See ADR 0020 Amendment 2 §7.

- **One route declared by two controllers no longer merges or disappears.** Endpoint
  identity was `(method, path)`, so two controllers declaring the same absolute route
  shared one: NestJS merged them, and an unguarded endpoint was reported carrying the
  other's guards with `mutating-endpoint-without-access-control` suppressed; Spring
  dropped one of the pair outright. Both endpoints are now kept, each with its own
  guards, and `sphinxor export cerbos` omits them, since a policy written for one
  might govern the other's traffic. Verified on `YunaiV/ruoyi-vue-pro`, where a
  runtime path prefix keyed on the Java package separates an admin API from an app
  API: `PUT /member/user/update` exists twice, one side carrying `@PreAuthorize` and
  the other no access control at all, and the unguarded side was absent from the
  report entirely. It is now reported and flagged.

  The run warns about such a collision only when the two sides' guards actually
  differ. Warning on every collision would have fired 324 times across the 17-repo
  survey behind this work, almost entirely on cases it proved harmless — a monorepo's
  per-service health check, a worker deliberately re-declaring a route. Measured
  against the implementation, the criterion fires 47 times in `ruoyi-vue-pro`, 13 in
  `eugenp/tutorials`, and not at all in immich, `shenyu`, cal.com, novu or
  `amplication`. See ADR 0020 Amendment 2 §8.

- **A project exposing a GraphQL API is now told that it was not analyzed.**
  GraphQL stays out of scope
  ([ADR 0021](docs/decisions/0021-graphql-out-of-scope-but-detected.md)), but a
  scope boundary the output never mentioned was indistinguishable from having
  nothing to report. `notiz-dev/nestjs-prisma-starter`'s entire API is 16
  resolver operations, 10 of them behind `GqlAuthGuard`; Sphinxor reported its
  two hello-world REST routes with 0 findings and no caveat, because ADR 0019
  §2's "recognized no endpoints" notice keys on zero endpoints and two is not
  zero. Resolvers are now detected and the run warns, naming how many
  operations it did not analyze. No resolver is parsed and no finding changes.
- **A comment between a decorator and what it decorates no longer deletes
  endpoints.** In tree-sitter-typescript a comment is a named sibling, so for
  shapes as ordinary as `@Post('x') // note` the decorators were attached to
  the comment instead of the handler. The handler then had no route decorator
  and stopped being an endpoint at all — no row in the matrix, and so no lint
  rule to fire on it. On a `@Controller` line it cost every route in the class
  at once. On `CatsMiaow/nestjs-project-structure` this hid an entire
  controller whose `POST`, `PUT` and `DELETE` are unguarded, reported as zero
  findings because there was nothing left to report on; that project goes from
  9 endpoints and 0 findings to 23 and 3. A comment was the whole
  difference between a false "all clear" and three real findings on a fully
  unguarded controller, out of a two-line defect — the clearest example in
  this project so far that the dangerous bugs are not the complicated ones.
- **An endpoint whose route path cannot be read no longer collides with
  another endpoint's identity.** Endpoint identity is derived from
  `(method, path)`, and extraction reads only string literals — so
  `@Controller(RouteKey.Asset)`, a route constant, an array form, a template
  literal, and Spring's `@RequestMapping(Routes.ADMIN)` all silently produced
  an empty prefix. Unrelated endpoints then shared one identity. NestJS merged
  them, reporting one endpoint's guards and roles against another: a wide-open
  `DELETE` shown as `ADMIN`-protected with the
  `mutating-endpoint-without-access-control` finding suppressed — the same
  false assurance as the `antMatcher` defect below, reached a different way.
  Spring instead dropped one of the pair, so the unguarded endpoint vanished
  from the report entirely. Such endpoints now keep an identity synthesized
  from their controller and handler, are marked in the matrix with a leading
  `…`, are named in a project-level warning, and are omitted by
  `sphinxor export cerbos`, which cannot name a policy after a fragment of a
  route. They remain fully analyzed and linted.

- **A `SecurityFilterChain` matcher whose pattern couldn't be read no longer
  matches every request.** `requestMatchers(antMatcher("/admin/**"))` — the
  idiomatic Spring Security 6 form — collapsed into the same internal state as
  `.anyRequest()`, so its `hasRole("ADMIN")` was applied to the whole
  application. On the audit's reproduction, a `DELETE /public/wipe` endpoint
  that really falls through to `permitAll()` was reported as ADMIN-protected
  with zero findings: a false assurance on a destructive endpoint, which also
  suppressed the `mutating-endpoint-without-access-control` safety net that
  would otherwise have caught it. Such a matcher now makes its rule *unknown* —
  it grants its roles to no endpoint and stops evaluation for the ones it might
  cover. `antMatcher(...)` wrappers carrying a readable pattern are now read
  properly rather than being lost this way.
- **A URL layer that exists but couldn't be analyzed is no longer treated as
  no URL layer.** A project with more than one `SecurityFilterChain` bean had
  the layer silently skipped, leaving the method layer to stand as the complete
  picture. `sphinxor export cerbos` turned that into a deployable policy
  granting `[ADMIN, ANALYST]` on an endpoint the running application restricted
  to `ADMIN`. Such a project is now reported as having an unanalyzed URL layer:
  `lint` warns that its roles may be broader than reality, and `export` omits
  every endpoint and says why rather than exporting a grant the application
  denies.
- **Reactive Spring Security (`SecurityWebFilterChain` / WebFlux) is now
  detected.** Its rules are still not parsed — that remains out of scope — but
  a reactive project no longer looks like one with no URL-layer authorization
  at all. It gets the same warning and the same export omission as the
  multi-chain case.
- **Two systematic distortions are now stated instead of implied.** A Spring
  project whose method-security annotations were found with no
  `@EnableMethodSecurity` anywhere in the analyzed source now warns that those
  annotations may be inert at runtime, in which case the roles shown for them
  are imaginary. A NestJS project registering a global guard (`APP_GUARD`
  provider or `app.useGlobalGuards()`) now warns that endpoint-level results
  understate protection, since every route is protected by default. Neither
  changes a finding; both change what the output means.

### Fixed

- `sphinxor version` no longer reports `dev` for a binary installed with
  `go install <pkg>@<version>`. `go install` doesn't apply the release
  workflow's `-ldflags`, so a freshly installed tagged release misreported
  itself on the user's first command. The version now falls back to the module
  version Go records in the build info when no `-ldflags` stamp is present —
  release binaries are unaffected, since the stamp still takes precedence. A
  local `go build` inside the repo now reports the VCS pseudo-version (with
  `+dirty` on an uncommitted tree) instead of `dev`, identifying the exact
  commit; `dev` remains for builds with no VCS information at all.

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
