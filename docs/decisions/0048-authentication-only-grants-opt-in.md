# 0048. Authentication-only grants are exported only on request

## Status

**Proposed** (2026-10-02). Measured before being written, and not implemented. It changes
behaviour the export has had since [ADR 0010](0010-authenticated-any-role.md): a
role-less `AuthGuard` (NestJS), or an `authenticated()` URL rule with nothing else read
(Spring), exports `roles: ["*"]`.

## Context

### A `"*"` grant is a positive statement made from knowing almost nothing

The export writes `roles: ["*"]` when the only requirement Sphinxor read on an endpoint is
authentication. A deployer reads such a rule as "any authenticated user may do this".

Since #81, every policy file with such a rule says what it really means: no restriction
detected beyond authentication. **But the rule still grants.** In a policy, a grant is the
statement that matters, and the caveat travels in a comment.

[ADR 0039](0039-route-path-constants.md) Amendment 1 produced the corpus's first such
rules, and its justification showed the cost. Of mateclaw's 81 newly exported `"*"`
endpoints:
- **9 check a role in handler code.** The policy would allow `DELETE
  /api/v1/workspaces/{id}` to any authenticated user. The handler first calls
  `requirePermission(id, userId, "owner")`.
- **26 check ownership in handler code.**

Sphinxor cannot see either kind of check ([ADR 0047](0047-three-states-per-endpoint.md)
§5). That cuts against the owner's principle that the tool never presents what it does
not know as fine.

[ADR 0041](0041-permission-export.md) settled a similar question for permissions by making
their export opt-in: rules resting on statements Sphinxor did not infer are written only
when the user asks. Authentication-only grants rest on an absence. They deserve at least
the same.

### Measured: every existing export

The current binary at `5b64842` was run on corpus-20, the 12 corpus-40 projects and every
fixture, Spring and NestJS. **Every `"*"` rule is authentication-only**: none mixes `"*"`
with a role.

| Target | Exported rules | `"*"` rules | Endpoints behind them | Where the authentication comes from |
|---|---:|---:|---:|---|
| mateclaw | 49 | 49 | 81 | a URL-layer blanket, `/api/** authenticated()` |
| `nestjs-boilerplate` (fixture) | 9 | 5 | 5 | a role-less `AuthGuard` on the controller or handler |
| `ghostfolio-shape` (fixture) | 3 | 3 | 3 | the same |
| `awesome-nest-boilerplate` (fixture) | 4 | 1 | 1 | the same |
| `Pharmacy`, `blog-api`, `trackr-backend` (Spring fixtures) | 9 | 0 | 0 | — |
| **All** | **74** | **58 (78%)** | **90** | |

None of the NestJS fixtures registers a global guard: each `"*"` there comes from a guard
written on the endpoint or its controller. No other corpus project exports anything
today. Their URL layers are unknown, or their endpoints carry roles or nothing.

## Decision (proposed)

### §1 Not exported by default

An action whose only possible grant is `"*"` (authentication read, and no role or
permission) is **omitted by default** under its own reason:
- **`authenticated-only`** in the export, with the detail "no restriction detected beyond
  authentication; exported only with `--export-authenticated-only`";
- an **`authentication only`** entry on ADR 0047's review list, once that ADR is accepted.
  Until then, the export report lists these endpoints.

Cerbos denies an action with no rule. An omitted endpoint is therefore denied to everyone
by a deployed default export. That is stricter than the application, and it is the safe
direction (ADR 0009 §3).

### §2 Opt-in restores today's output

`sphinxor export cerbos --export-authenticated-only` writes exactly what the export writes
today: the `"*"` rules, with the statement that #81 added to each policy file and to the
report. The report records that the flag was given, as it records ADR 0041's declarations.

### §3 What users of today's exports must add

| Export | Without the flag, after this change | To keep today's output |
|---|---|---|
| A NestJS project whose endpoints carry a role-less `AuthGuard` | those actions omitted, so denied by default in Cerbos | add `--export-authenticated-only` |
| A Spring project whose URL layer requires `authenticated()` for endpoints with nothing else read (mateclaw) | the same | add `--export-authenticated-only` |
| Any export with role or permission rules only | unchanged | nothing |

On the measured targets that is 58 of 74 rules. mateclaw's export becomes empty, and so
does `ghostfolio-shape`'s. `nestjs-boilerplate` keeps 4 of 9 rules, and
`awesome-nest-boilerplate` keeps 3 of 4.

### §4 Consumers

| Consumer | Change |
|---|---|
| Cerbos export | §1 and §2 |
| Export report | lists `authenticated-only` omissions, and states the flag and what it restores |
| Matrix, lint, `sphinxor diff` | none. Authentication stays a requirement that was read: the endpoint is known protected for findings (ADR 0047), and only the export stops granting from it |
| ADR 0047's review list | gains `authentication only` as a reason, for endpoints that are protected but whose authorization beyond authentication is unknown. This touches ADR 0047's first open question |
| Tests | the tests asserting a `"*"` rule set the option: `effective_policy_test`, `permissions_test`, `real_world_test`, `wildcard_test` and `project_enforcement_test` in the export and Spring packages, NestJS `authentication_test`, and the report's `export_wildcard_test` |

## Open question for the owner

**Should an authentication requirement written on the endpoint itself stay exported by
default?** That is NestJS's role-less `AuthGuard`, or Spring's
`@PreAuthorize("isAuthenticated()")`. It is the developer's own statement about that
endpoint, unlike mateclaw's URL-layer blanket. But it leaves handler checks just as
invisible.
- Measured: 9 endpoints, all NestJS fixtures, against 81 from a URL blanket.
- **Recommendation:** no distinction. The principle is about what Sphinxor knows, not
  about who wrote the rule, and two defaults would be harder to explain than one flag.

## Alternatives considered

- **Keep exporting `"*"` with #81's statement.** This is the status quo. The caveat is a
  comment; the grant is the rule.
- **Export `"*"` only where no handler check is found.** That would need a call-graph
  analysis Sphinxor does not have. The mateclaw justification needed reading to separate
  9 role checks from 46 handlers where no check was found, and even those 46 are a
  candidate list.
- **Drop `"*"` entirely, with no flag.** That would remove a legitimate use: a project
  that really has authentication-only endpoints, deployed by a user who has reviewed
  them.

## Consequences (if accepted)

- One new flag, one new omission reason, and the report section.
- `CHANGELOG.md` under *Changed*, with §3's migration table: existing users must add the
  flag to keep their output.
- ADR 0010's export consequence is amended, not its extraction: authentication is still
  read and modelled.
