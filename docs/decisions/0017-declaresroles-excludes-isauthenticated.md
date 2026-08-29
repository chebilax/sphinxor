# 0017. `DeclaresRoles` is false for `isAuthenticated()` — correcting ADR 0011 §1

## Status

Accepted.

## Context

ADR 0011 §1 states: "For Spring, every recognized guard (`PreAuthorize`/
`Secured`/`RolesAllowed`) sets `DeclaresRoles: true` — there's no NestJS-style
split between a role-bearing guard and a separate protection-only guard,
since presence and role-check are fused." Taken literally, this is wrong,
not merely incomplete: `@PreAuthorize("isAuthenticated()")` (ADR 0011 §2 /
ADR 0010) resolves zero `RoleReference`s, and `internal/lint/empty_role.go`
fires on any `GuardApplication` with `DeclaresRoles: true` and zero
`RoleReference`s. Setting `DeclaresRoles: true` for `isAuthenticated()`
would make `empty-role` report "declares no roles" on an annotation that is
*intentionally* "authenticated, any role" — the exact case ADR 0010 built
`AuthenticationRequirement` as a positive representation of, specifically
so it would not read as a probable mistake.

`DeclaresRoles`'s actual meaning, established when ADR 0011 introduced it:
*this guard carries the endpoint's role requirement as a `RoleReference`
list.* Checked against NestJS's own extraction (`internal/extract/nestjs/controllers.go`),
that's never true for a bare `AuthGuard` — presence/authentication and
role-declaration are two separate `GuardApplication` kinds there, and only
the `Roles`-derived one sets `DeclaresRoles: true`. `isAuthenticated()` is
Spring's structural equivalent of a bare `AuthGuard`: presence and
authentication, not a role list given nothing. §1's "every recognized
guard" was stated too broadly — it didn't anticipate that one SpEL shape
inside `PreAuthorize` (`isAuthenticated()`) asserts something categorically
different from the rest, the same way a bare `AuthGuard` differs from a
`Roles`-carrying one.

## Decision

**`DeclaresRoles` is `true` for `PreAuthorize`/`Secured`/`RolesAllowed`
*except* when the annotation is `@PreAuthorize` and its SpEL content parses
to exactly `isAuthenticated()`; there, it is `false`.**

`permitAll()`, `denyAll()`, and any unrecognized SpEL content keep
`DeclaresRoles: true`, per ADR 0011 §2's explicit intent: "An endpoint
marked `permitAll()` still surfaces through the normal unguarded/no-role
path if a lint rule would otherwise flag it." Only `isAuthenticated()`
moves, because only it has a positive, non-role representation
(`AuthenticationRequirement`) to move to — `permitAll()`/unrecognized SpEL
have no such representation and are meant to keep surfacing as "guarded,
no role."

The fix lives in extraction, where `DeclaresRoles` is set, not in
`internal/lint/empty_role.go`. That rule read the field correctly; the
field was populated wrong for this one case. `internal/lint/empty_role.go`
requires no change.

## Alternatives considered

- **Modify `empty_role.go` to also check `AuthenticationRequirements`
  before flagging** — rejected: repairs an upstream-wrong field
  (`DeclaresRoles: true` on something that declares no roles) in
  downstream rule logic, adding framework-shaped cross-checking to a rule
  meant to stay framework-agnostic — the identical seam ADR 0011's own
  grep already caught once (`empty_role.go`/`report.go` branching on the
  literal string `"Roles"`, fixed by generalizing the *signal*, not by
  patching the two consumers around it). Would also make NestJS and Spring
  diverge for the same underlying case (NestJS needs no such cross-check
  at all, since a bare `AuthGuard` never sets `DeclaresRoles: true` in the
  first place) — this decision keeps both frameworks' extraction populating
  the field with the same meaning, so no consumer needs to know which
  framework produced the model.
- **Leave ADR 0011 §1 as written, treat this as an accepted false
  positive** — rejected outright: reintroduces exactly the false-confidence
  failure mode ADR 0010/0011 were written to avoid, on a case those same
  ADRs positively solved for.

## Consequences

- `internal/extract/spring`: the `GuardApplication` built from a
  `@PreAuthorize` annotation sets `DeclaresRoles` conditionally on its
  parsed SpEL result (`spelAuthenticated` → `false`; everything else,
  including `spelRoles` and `spelUnrecognized` → `true`), not
  unconditionally `true` for the annotation name alone.
  `@Secured`/`@RolesAllowed` are unaffected — they have no SpEL, no
  `isAuthenticated()`-equivalent shape, and keep `DeclaresRoles: true`
  unconditionally, matching ADR 0011 §1's original (correct, for these two)
  intent.
- `internal/lint/empty_role.go`: no change. Confirmed correct as written;
  the bug was in what fed it, not in the rule.
- Amends ADR 0011 §1's stated rule for `DeclaresRoles`; ADR 0011's other
  content (endpoint extraction, SpEL recognition scope, the
  `@EnableMethodSecurity` gate) is unaffected.
