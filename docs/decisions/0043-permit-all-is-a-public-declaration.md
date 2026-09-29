# 0043. A method-level permit-all is a public declaration — neither protection nor an empty role check

## Status

**Accepted** (2026-09-29), including the 4 new route-collision warnings on yudao, and
implemented. It settles the question [ADR 0017](0017-declaresroles-excludes-isauthenticated.md)
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
| upstream yudao | 0 | **106 annotations, on 106 endpoints** (see *Validation*) |

In upstream yudao (counts from the extractor, see *Validation*):
- **All 106 are on handler methods, none at class level, and none shares a method or a
  class with another method-security guard.** So JSR-250's method-over-class precedence
  never arises there.
- **53 are on mutating endpoints:** 45 `POST`, 3 `PUT`, 1 `DELETE`, 4 verb-less
  `@RequestMapping` (`ANY`).
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
| route-collision check | compares guards | a public declaration counts as a difference between the two sides. On yudao this adds 4 warnings (*Validation*): the source does declare those sides differently, and at runtime yudao's filter turns the declaration into a URL rule for the shared path, so which handler's intent governs it is exactly what the warning asks a reader to check |
| CLI warnings | the `@PermitAll` count | retired. The method-security "may be inert" caveat is unaffected, since permit-all protects nothing either way |
| URL-layer `.permitAll()` | no requirement (ADR 0012 §1) | **unchanged** (Context) |

### §3 Precedence where a permit-all meets a guard on the same endpoint

Spring Security runs **one interceptor per annotation family**:
- `@PreAuthorize` (with `permitAll()`);
- JSR-250 (`@RolesAllowed`, `@DenyAll`, `@PermitAll`);
- `@Secured`.

Each resolves its own annotations method first, then class. Read at source:
`Jsr250AuthorizationManager` scans `@DenyAll`/`@PermitAll`/`@RolesAllowed` on the method
and then its target class, and `PrePostMethodSecurityConfiguration` registers a separate
`preAuthorize()` interceptor (Spring Security 6.5.11). So precedence holds **within a
family**, and across families both apply:

| Class | Method | Result |
|---|---|---|
| `@RolesAllowed` | `@PermitAll` | public: same family, the method's declaration wins |
| `@PermitAll` | `@RolesAllowed` | guarded: same family, the method's guard wins |
| `@PreAuthorize("hasRole(...)")` | `@PreAuthorize("permitAll()")` | public: same family |
| `@PreAuthorize("hasRole(...)")` | `@PermitAll` | **guarded and declared public**: two interceptors run, and the role is still required |
| `@PermitAll` | none | public |

**Correction.** The first draft said a method-level permit-all overrides a class-level
guard in general. Reading the interceptors showed that holds only within a family, so a
class `@PreAuthorize` still applies under a method `@PermitAll`. It is recorded rather
than quietly corrected. Guard-and-guard combinations are unchanged from before this ADR.
None of these arrangements occurs in yudao or the corpus, and each has a synthetic test.

## Effect, predicted

- **The 20-repository corpus:** zero occurrences, so byte-identical.
- **The `ruoyi-vue-pro` fixture:**
  - its one `PUT` gains a `PublicDeclaration` and a matrix label;
  - it keeps the Low finding it already has (its message names the declaration);
  - the project warning retires.
- **Upstream yudao** (not vendored), measured in *Validation*:
  - 106 endpoints labelled public;
  - the 53 mutating ones keep the Low finding they already get today, now with a message
    saying why and inviting `sphinxor-allow`;
  - no `empty-role` anywhere;
  - 4 more route-collision warnings, where a public side meets an unannotated one.

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
  exempt 53 of yudao's public writes without anyone looking at them.
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

## Validation against upstream yudao (2026-09-29)

Measured with the extractor at `bc46395` (v0.10.0), against yudao at `8e80602`, the vendored
fixture's commit. The current binary was run on the whole tree, and each `@PermitAll`,
bound by its import, was mapped to its extracted endpoint by file and line.

**Correction.** The Context first gave 109 uses on 97 methods, 38 of them mutating. Those
came from a text grep and a regular expression over annotation blocks, and both were
wrong:
- the grep's 3 extra hits are comments in `YudaoWebSecurityConfigurerAdapter`, mentions
  rather than annotations;
- the regular expression missed handler methods whose annotation arguments contain
  nested parentheses.

The extractor's counts replace them above. As ADR 0015 Amendment 1 §2 asks, this is
recorded rather than quietly corrected.

**Today (v0.10.0), for the 106 `@PermitAll` endpoints:**

| | Count |
|---|---:|
| endpoints | 106: 53 `GET`, 45 `POST`, 3 `PUT`, 1 `DELETE`, 4 `ANY` |
| carrying a guard, or an unrecognized authorization annotation | 0 |
| mutating, with the Low `mutating-endpoint-without-access-control` | **53 of 53** |
| `empty-role` | 0 |
| under a class-level guard, or beside another guard on the method | 0 |
| in a route collision | 27 |
| path unresolved | 0 |
| Cerbos export | all omitted: yudao has 2 `SecurityFilterChain` beans, so its URL layer is unknown (ADR 0040) |

**What ADR 0043 changes on yudao, predicted from those numbers:**
- **Findings: none added, none removed.** The 53 Low findings stay, and their message
  names `@PermitAll` and suggests `sphinxor-allow`. No `empty-role` appears.
- **The matrix:** 106 rows gain `public (@PermitAll)` in Guards; the project warning
  counting them retires.
- **The became-public gate:** nothing moves on yudao itself. No `@PermitAll` endpoint
  carries a guard today, so no existing diff result changes. The gate matters for
  transitions: replacing a guard with `@PermitAll` already trips it (tested in ADR 0012
  Amendment 1), and replacing one with `permitAll()` newly will.
- **Route collisions:** of the 26 colliding routes with a `@PermitAll` side,
  - 21 already warn, since the other side carries a guard;
  - 1 has `@PermitAll` on every side and stays quiet;
  - **4 newly warn**, where the other side is unannotated. This is the one visible
    change beyond labels, and the reason §2's collision row says why it is correct.
- **The export:** unchanged on yudao. `url-layer-unknown` omits every endpoint before
  any permit-all reason is reached.

**The starting position holds on yudao as it does on the corpus.** The declaration never
protects, never needs `empty-role`, and never suppresses the mutating finding. The only
behaviour it adds beyond labels is the 4 collision warnings. The pinned ADR 0037 case is
not exercised by yudao, which has no `permitAll()`; the synthetic test covers it.

## Measured, as implemented (2026-09-29)

`main` at `dffac0a` against this implementation.

- **Corpus, fixtures and sample:** all 42 targets byte-identical except the
  `ruoyi-vue-pro` fixture. There, as predicted:
  - its one `PUT /member/user/reset-password` gains `public (@PermitAll)`;
  - its Low finding's message names the declaration and `sphinxor-allow`;
  - the retired count warning is gone;
  - its export omission reason moves from `no-guard` to `declared-public`.
- **Upstream yudao, v0.10.0 against this implementation:**
  - **findings identical:** 217 before and after, the same (rule, subject, confidence)
    set, 0 `empty-role`;
  - 106 rows labelled public;
  - route-collision warnings 47 → **51**. The 4 new ones are exactly the predicted app and
    admin twins: `GET /promotion/combination-activity/list-by-ids`,
    `GET /promotion/point-activity/list-by-ids`,
    `GET /promotion/seckill-activity/list-by-ids` and `GET /system/area/tree`;
  - export report identical, since the URL layer is unknown;
  - exit code unchanged.
- **The pinned ADR 0037 case** still fails the build, now through became-public
  (asserted). `@Secured` → `permitAll()` and `@RolesAllowed` → `@PermitAll` fail as
  became-public too. The first of these also failed before, through `empty-role`, which was
  checked by running the test against the previous code.

