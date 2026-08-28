# 0010. Model an "authenticated, any role" requirement (amends ADR 0002)

## Status

Proposed.

## Context

Exporting the two vendored NestJS repos to Cerbos (ADR 0009, PR #7) produced 6
rules covering 7 of 24 endpoints (`nestjs-boilerplate` has 16 — 5 on
`UsersController`, 11 on `AuthController` — not 15; `UsersController`'s `findAll`
and `findOne` are two endpoints merged into one exported `get` rule, easy to
undercount as "4" if the rule count and the endpoint count get conflated. Verified
by recounting directly against a fresh `export-report.json`, not carried forward
from an earlier statement in this conversation, which had the same off-by-one.)
The exporter's own companion report is the
evidence, not a guess: 17 omissions, split `no-guard` (10), `guarded-no-role` (5),
`action-collision` (2) — recomputed directly from the real `export-report.json`
output, not from memory. (An earlier verbal summary in this conversation said "8"
for the guarded-no-role count; that was wrong, corrected here before it became the
basis for a decision. The 2 action-collision cases include one endpoint,
`awesome-nest-boilerplate`'s `GET /posts/:id` via `@Auth([])`, that is *also*
guarded-with-no-role underneath — it shows up under `action-collision` instead
because it happens to share a Cerbos action with a role-bearing sibling, per ADR
0009 §2's controller+method mapping.)

All 5 pure `guarded-no-role` cases are the same real shape:
`@UseGuards(AuthGuard('jwt'))` with no `@Roles()`/composite-role-decorator
anywhere on the endpoint. This is not a gap in what extraction found — it's a
confirmed fact about the source: NestJS guards run as a list, in order; if no
role-checking guard (`RolesGuard`, or a project's own composite wrapping it) is in
that list, no role check happens for that route, full stop. The model already knows
this precisely — `GuardApplication` records exist, `RoleReference` records don't —
but nothing in the model *says* "this is confirmed authenticated-only," so every
consumer (today: three lint rules and one exporter) that cares has to re-derive
"has a GuardApplication, has zero resolved RoleReferences" independently, and none
of them can distinguish it from "extraction didn't find a role for some unrelated
reason." That's exactly the kind of implicit, re-derived fact ADR 0002 already
solved once for `RoleReference.RoleDeclarationID` (nil is an explicit, honest "no
declaration found," never inferred into existence) — this is the same problem one
level up, at the endpoint's overall access-control status rather than a single
reference's resolution.

This matters beyond Cerbos. The gap is in the model, not in
`internal/export/cerbos`'s serializer — so today it silently caps every future
exporter (OPA/Rego, v0.5.0, and whatever comes after) at the same coverage ceiling,
independent of how good each serializer is. Fixing it once in extraction fixes it
for all of them; fixing it once in the Cerbos exporter would need re-fixing per
exporter, and OPA/Rego's general-purpose flexibility (ADR 0009 §1's own reasoning
for why Cerbos went first) would make a Rego-only version of this fix easy to get
subtly wrong without Cerbos's fixed schema forcing the question.

**Whether Cerbos can even express "authenticated, any role" was checked, not
assumed.** Cerbos's own docs (`resource_policies.adoc`, note 12): "Rules can also
refer directly to static roles. The special value `*` can be used to disregard
roles when evaluating the rule." Confirmed behaviorally, not just read: a real
`cerbos compile` run against a policy using `roles: ["*"]`, with a test asserting a
principal holding an unrelated role (`totally_unrelated_role`) against it, passed —
`EFFECT_ALLOW`, exactly as documented.

## Decision

**Add a new normalized entity, `AuthenticationRequirement`, to `internal/model`,**
populated by extraction:

```go
// AuthenticationRequirement is a positive, confirmed fact: this endpoint has
// at least one real GuardApplication, and none of them resolve to a specific
// role -- "authenticated, any role" in the source. Never inferred from
// silence; only created when extraction can point at the guard(s) that
// establish it.
type AuthenticationRequirement struct {
    ID         ID
    EndpointID ID
    File       string
    Line       int
}
```

Added to `Model.AuthenticationRequirements`. This is additive to ADR 0002's
collection set — no existing field or type changes shape.

**Extraction creates one `AuthenticationRequirement` for an endpoint when it has
at least one `GuardApplication` and zero resolved `RoleReference`s, with one
deliberate exclusion**: a literal `@Roles()` call with zero arguments (the exact
shape `EmptyRole` already flags at High confidence as a probable mistake) does
**not** produce an `AuthenticationRequirement`. `EmptyRole`'s own reasoning already
draws this line for a related purpose — a composite's empty-by-default role list is
"that composite's documented... behavior," while a literal empty call is "the
forgotten-argument smell this rule targets" — and this decision leans on the same
distinction for a different consumer: exporting `roles: ["*"]` for code already
flagged as *probably a bug* would mean confidently granting broad access based on
what might be a mistake, not a deliberate design choice. That's precisely the
guess-in-the-permissive-direction failure ADR 0009 exists to prevent, now one layer
upstream in the model instead of in the exporter. Composite-resolved empty role
lists, and endpoints with no role-checking guard applied at all (the 5 real cases
above), both still qualify — neither is flagged as a mistake anywhere else in the
system.

**Interaction with export's action-collision detection (ADR 0009 §2) is
compatible without new logic, not incidentally.** "Authenticated, any role" is a
broader grant than a specific named role — an endpoint requiring `RoleType.USER`
and a sibling requiring only authentication are still genuinely different access
requirements sharing one Cerbos action, and must still collide, not merge. Treating
`AuthenticationRequirement`'s `*` the same way `Translate` already treats any other
role name — one more entry in the grant-name set a sibling's set is compared
against — gets this right by construction: two `AuthenticationRequirement`
endpoints sharing an action still merge (identical grants), one `AuthenticationRequirement`
and one specific-role endpoint sharing an action still collide (different grants).
No change to the collision algorithm itself, only to what one endpoint's grant set
can contain — confirmed by re-checking the awesome-nest-boilerplate case: it would
remain a collision under this model change, correctly, since `RoleType.USER` and
`*` are still two different things. That endpoint stays omitted even after this
ADR — a real, stated limit of this fix, not swept under it.

## Alternatives considered

- **Derive "authenticated, any role" independently inside each exporter**
  (status quo) — rejected: duplicates the same "has guard, zero roles" computation
  across every exporter (already duplicated once, between three lint rules and the
  Cerbos exporter, before this ADR), and buries a fact about what the *source code*
  requires inside export-specific heuristics, contradicting ADR 0002's premise that
  the model is the framework- and consumer-independent source of truth.
- **Include the literal-empty-`@Roles()` case** (don't carve out `EmptyRole`'s
  territory) — rejected: would export a real grant for code already flagged
  elsewhere as a probable mistake. Rejected outright, not just deprioritized, for
  the same reason ADR 0009 rejected permissive placeholders for uncertain cases.
- **Represent this as a sentinel role name** (e.g. a synthetic
  `RoleReference{RawLiteral: "__authenticated__"}`) instead of a separate entity —
  rejected: `RoleReference.RawLiteral` is documented as literal source text (ADR
  0002), and `PermissionDeclaredButUnreferenced`/the RBAC matrix both already treat
  every `RoleReference` as naming something a human wrote in the source. A
  fabricated literal that never appeared in any file would corrupt both, and would
  need its own exclusion logic wherever `RoleReference` is read — worse than one
  new, honestly-named entity.

## Consequences

- `internal/model` gains `AuthenticationRequirement` and
  `Model.AuthenticationRequirements`, additive per ADR 0002's normalized shape.
- `internal/extract/nestjs` gains the logic above — the only extraction change;
  no NestJS-specific concept is introduced, since "guard present, zero resolved
  roles, not the flagged-empty-literal case" is already exactly what
  `mutating_endpoint.go` and `empty_role.go` each compute a piece of today.
- `internal/export/cerbos`'s `Translate` gains one new source of `roleGrant`
  (value `"*"`) alongside `RoleReference`-derived ones, replacing today's blanket
  `ReasonNoRole` omission for endpoints this now covers. The Rego exporter (v0.5.0)
  inherits the same fix for free, per the whole point of this living in the model.
- Concretely, applied to the two vendored repos as they stand today: **3**,
  not 5, of `nestjs-boilerplate` `AuthController`'s omitted endpoints actually
  become newly exported — `get`/`patch`/`delete` on `auth`, each a single,
  sibling-free endpoint (`GET`/`PATCH`/`DELETE /auth/me`). The `post` case does
  **not** newly export, and an earlier draft of this ADR claimed it did without
  carrying the interaction analysis above through the arithmetic for this specific
  group. `POST /auth/logout` and `POST /auth/refresh` (the two endpoints that would
  gain an `AuthenticationRequirement`) share the `auth` resource's `post` action
  with **six** other endpoints that have no guard at all
  (`/auth/email/{login,register,confirm,confirm/new}`, `/auth/forgot/password`,
  `/auth/reset/password`) — confirmed directly against `export-report.json`, not
  assumed. A grant of `*` and a grant of nothing are different grant sets, so this
  group collides under the existing algorithm exactly as designed (no special case
  needed — this is what "no change to the collision algorithm" in the interaction
  analysis above actually means in practice, including where it costs coverage, not
  only where it's neutral). Those 8 endpoints don't disappear from the report; they
  move from six `no-guard` + two `guarded-no-role` omissions to eight
  `action-collision` omissions instead — same safe outcome (nothing exported),
  different, more precise stated reason, since two of the eight now demonstrably
  disagree with the rest rather than all eight uniformly having nothing to say.
  Net for `nestjs-boilerplate`: 4 → 7 rules, 5 → 8 of its 16 endpoints covered.
  `awesome-nest-boilerplate` is unchanged (its one relevant case stays inside an
  action-collision, per the interaction analysis above). Combined across both
  repos: **6 → 9 rules, 7 → 10 of 24 endpoints covered.** A real, meaningful
  improvement — not a full close of the gap, and smaller than this ADR first
  claimed once checked against its own stated mechanism rather than eyeballed.
- Existing lint rules (`mutating-endpoint-without-access-control`,
  `permission-declared-but-unreferenced`, `empty-role`) are unaffected — none of
  them read `AuthenticationRequirement`, and this ADR doesn't ask them to.
  Whether the RBAC matrix report should also surface this new fact is a small,
  separate follow-up, not decided here.
