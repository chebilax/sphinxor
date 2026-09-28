# 0043. A method-level permit-all is a public declaration — neither protection nor an empty role check

## Status

**Proposed.** It settles the question [ADR 0017](0017-declaresroles-excludes-isauthenticated.md)
and [ADR 0020](0020-unanalyzable-is-unknown-not-absent.md) Amendment 3 §10 left open for
`permitAll()`, and reads JSR-250 `@PermitAll`, which
[ADR 0012](0012-securityfilterchain-effective-policy.md) Amendment 1 only announces. It
tests the owner's starting position:

> permit-all is a positive "public" declaration — neither an empty role check nor
> protection. So it shouldn't raise empty-role; it shouldn't suppress
> mutating-endpoint-without-access-control (a public mutating endpoint gets the Low
> finding, allowlistable per ADR 0003); and a transition from any protection to
> permit-all must trip the became-public gate (ADR 0036). The pinned 0037 case must still
> fail the build afterwards.

## Context

### The two forms, and what each does today

| Form | Today | Why that is wrong |
|---|---|---|
| `@PreAuthorize("permitAll()")` | a guard with `DeclaresRoles: true` and no role, so **`empty-role`** fires (High, build-failing), and the endpoint counts as protected | it is neither: not a role check left empty, and not protection |
| JSR-250 `@PermitAll` | counted and announced, attached to no endpoint (ADR 0012 Amendment 1), so the endpoint is analyzed as unannotated | the declaration is real, and the report does not show it |

A chain's `.permitAll()` is **not** in scope here, and stays as ADR 0012 §1 decided: it
contributes no requirement. At the URL layer it means "this layer adds nothing", and a
method guard still applies behind it, so it is not a statement that the endpoint is public.
There are 54 such rules across the corpus, and none changes.

### Measured

At `43eb2e0`, on the 20 corpus repositories at [ADR 0035](0035-permissions-in-the-model.md)
§9's commits, the fixtures, and upstream yudao at the vendored fixture's commit (`8e80602`):

| | `@PreAuthorize("permitAll()")` | `@PermitAll` |
|---|---:|---:|
| 20-repository corpus | 0 | 0 |
| vendored `ruoyi-vue-pro` fixture | 0 | 1 (a `PUT`) |
| upstream yudao | 0 | **109 uses in 47 files, 46 of them controllers** |

In upstream yudao:
- **97 are on methods, none at class level.** None shares a method with another guard,
  so JSR-250's method-over-class precedence never arises there.
- **38 are on mutating mappings:** 34 `POST`, 3 `PUT`, 1 `DELETE`, plus 4
  `@RequestMapping` whose verb is unread.
- **yudao enables `@EnableMethodSecurity(securedEnabled = true)`**, so Spring's own
  JSR-250 support is off and `@PermitAll` is inert to Spring (ADR 0015). It is
  `YudaoWebSecurityConfigurerAdapter` that reads it, on the handler or its class, to add
  runtime `permitAll` URL rules. Either way, what `@PermitAll` never does is *protect*
  anything. That is why the starting position holds whether or not the annotation is
  enabled.

**The pinned case.** `TestPermissionWidenedToPermitAllGates`: `@ss.hasPermi('x')` →
`permitAll()` fails the build today only through `empty-role`, because `permitAll()`
leaves a guard on both sides and the became-public gate sees nothing.

## Decision (proposed)

### §1 A new fact: `PublicDeclaration`

A method-level `permitAll()` and a `@PermitAll` (bound by its import, ADR 0022) each
record a `PublicDeclaration` on the endpoint:
- the endpoint;
- the scope (method or class);
- the form (`permitAll()` or `@PermitAll`);
- file and line.

**It is not a `GuardApplication`.** That is the whole point: every consumer that asks "is
this endpoint protected?" reads guards, and a permit-all must answer "no". A class-level
`@PermitAll` applies to methods with no method-level method-security annotation of their
own, which is JSR-250's precedence. It never occurs in yudao, so it is covered by a
synthetic test.

### §2 Every consumer, enumerated from the code

| Consumer | Today | Proposed |
|---|---|---|
| extraction, `permitAll()` | a guard, `DeclaresRoles: true` | a `PublicDeclaration`, and no guard |
| extraction, `@PermitAll` | a project-wide count (ADR 0012 Am1) | a `PublicDeclaration`; the count and its warning retire |
| `empty-role` | fires on `permitAll()` | cannot fire: no guard is recorded |
| `mutating-endpoint-without-access-control` | suppressed by the `permitAll()` guard; fires on `@PermitAll` endpoints (unannotated) | **fires on both**, Low, with a message naming the declaration: *"declared public by @PermitAll; mark it with sphinxor-allow if that is intended"*. Allowlistable (ADR 0003) |
| became-public gate (ADR 0036) | protection → `permitAll()` does not fire (guard on both sides). The transition fails the build only because the head gains an `empty-role` | **fires**: the head has no guard. The transition keeps failing the build, now for the reason that describes it, a loss of protection |
| pinned ADR 0037 case | fails the build through `empty-role` | still fails the build, through became-public. The test asserts `HasRegressions()`, not which rule, and keeps passing |
| diff, structural | `permitAll()` shows as a guard | public declarations added or removed get their own section |
| Cerbos export | `permitAll()`: a guard with no role → `guarded-no-role` omission | omitted with a new reason, `declared-public`. A public endpoint does not belong behind a policy decision point that authorizes principals. Granting `roles: ["*"]` would assert "any authenticated principal", which is not what public means |
| matrix / JSON | `permitAll()` hides under Roles as nothing | the Guards column shows `public (permitAll())` or `public (@PermitAll)` |
| route-collision check | compares guards | a public declaration counts as a difference between the two sides |
| CLI warnings | the `@PermitAll` count | retired. The method-security "may be inert" caveat is unaffected, since permit-all protects nothing either way |
| URL-layer `.permitAll()` | no requirement (ADR 0012 §1) | **unchanged** (Context) |

### §3 Precedence where a permit-all meets a guard on the same endpoint

JSR-250 and `@PreAuthorize` both give a method-level annotation precedence over a
class-level one. If a method carries a guard and its class a permit-all, the guard applies
and no declaration is recorded. If the class carries a guard and the method a permit-all,
the declaration applies and the class guard is not attached to that method. Neither
arrangement occurs in the measured code. Both get synthetic tests. The existing handling
of class and method guards together is unchanged.

## Effect, predicted

- **The 20-repository corpus:** zero occurrences, so byte-identical.
- **The `ruoyi-vue-pro` fixture:**
  - its one `PUT` gains a `PublicDeclaration` and a matrix label;
  - it keeps the Low finding it already has (its message names the declaration);
  - the project warning retires.
- **Upstream yudao** (not vendored):
  - 97 endpoints labelled public;
  - the 38 mutating ones keep the Low finding they already get today, now with a message
    saying why and inviting `sphinxor-allow`;
  - no `empty-role` anywhere.

## Alternatives considered

- **Keep `permitAll()` a guard and drop `empty-role` on it** (option 1 when the owner was
  asked). Rejected: a guard replaced by a permit-all would pass the became-public gate,
  and the pinned ADR 0037 case would stop failing the build.
- **Map `@PermitAll` to today's `permitAll()`** (option 2). Rejected: a build-failing
  `empty-role` on each of yudao's 109 deliberate declarations, the class of false positive
  ADR 0020 Amendment 3 removed 675 of.
- **Let a permit-all exempt the mutating finding automatically**, as a framework-level
  `sphinxor-allow`. Rejected for this ADR: the declaration says what the application
  does, and the allowlist says the reviewer accepts it. Folding one into the other would
  exempt 38 of yudao's public writes without anyone looking at them.
- **Export a permit-all as `roles: ["*"]`.** Rejected in §2: `*` means any authenticated
  principal in Cerbos, not anyone.

## Consequences (if accepted)

- `model.PublicDeclaration`.
- `internal/extract/spring`:
  - `spelNoRole` becomes a declaration rather than a guard;
  - `@PermitAll` is read into the declaration, and `scanPermitAll` retires;
  - §3's precedence.
- `internal/lint`: the mutating rule's message; `empty-role` needs no code change.
- `internal/diff`: a public-declarations section. The gate needs no code change, since
  it already reads guards only.
- `internal/export/cerbos`: `ReasonDeclaredPublic`.
- `internal/report`: the Guards label.
- Tests:
  - each row of §2;
  - both precedence arrangements;
  - `@Secured` → `permitAll()` and `@RolesAllowed` → `@PermitAll` gating through
    became-public;
  - the pinned ADR 0037 case still failing the build.
- The fixture's output changes as predicted, and the corpus is byte-identical. Both are
  measured before merge.
