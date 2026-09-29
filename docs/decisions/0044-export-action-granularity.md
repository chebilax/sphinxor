# 0044. Export granularity: a route condition only where endpoints collide

## Status

**Accepted with option B** (2026-09-29), on the owner's condition that the second calling
contract be stated where it is read (§ *The contract*). Implemented. It takes the decision [ADR 0041](0041-permission-export.md)
§7 recorded as the export's next: rules are keyed by (controller, HTTP verb), so two
endpoints of one controller and verb with different requirements collide, and both are
omitted. It amends [ADR 0009](0009-cerbos-exporter.md) §2's action mapping, if accepted.

## Context

Every figure below comes from the extractor and the exporter, per `docs/testing.md`,
measured at `7c64037`.

### What collisions cost

**Today, in shipped output** (default flags):

| Target | Endpoints in rules | Lost to action collision |
|---|---:|---:|
| `nestjs-boilerplate` fixture | 8 | 8 |
| `awesome-nest-boilerplate` fixture | 2 | 2 |
| `ghostfolio-shape` fixture | 2 | 2 |
| `blog-api` fixture | 0 | 2 |
| videochat (ADR 0038 sample) | 5 | 29 |

**With ADR 0041's declared permissions:**
- RuoYi-Vue: 34 rules cover 51 endpoints, and 77 collide;
- the `ruoyi-vue-pro` fixture: 1 rule, 2 collide.

**Latent, once their URL layers become readable:**
- thingsboard: 317 of 551 endpoints share a (controller, verb) with a different
  requirement;
- eladmin: 41;
- molgenis: 17.

Collisions cap every later URL-layer improvement.

### What a request can supply

The model keeps each framework's own route template: `/users/:id` for NestJS and
`/api/customers/{id}` for Spring. That is what each framework reports at runtime for the
matched route:
- Spring: `HandlerMapping.BEST_MATCHING_PATTERN_ATTRIBUTE`;
- NestJS: the route's `path`.

A policy enforcement point can therefore pass the template without Sphinxor inventing a
format.

## Options (ADR 0041 §7), measured

The recovery of options A and B was measured with a throwaway prototype of B. It was not
committed; A recovers the same endpoints by construction.

| Target | Today: rules / endpoints in rules | A or B: endpoints in rules | Where the other collided endpoints go |
|---|---|---:|---|
| `nestjs-boilerplate` | 7 / 8 | 10 | 6 were unguarded siblings, now omitted under `no-guard` |
| `awesome-nest-boilerplate` | 2 / 2 | 4 | — |
| `ghostfolio-shape` | 2 / 2 | 3 | — |
| `blog-api` | 0 / 0 | 1 | 1 unguarded sibling → `no-guard` |
| videochat | 3 / 5 | 13 | 21 unguarded siblings → `no-guard` |
| RuoYi-Vue, declared | 34 / 51 | **117** | 11 unguarded siblings → `no-guard` |
| `ruoyi-vue-pro`, declared | 1 / 1 | 3 | — |

A collision has always mixed two kinds of endpoint: guarded endpoints that differ from each
other, and unguarded siblings that were hidden behind the collision reason. Both options
export the first kind and give the second kind its true reason. On RuoYi-Vue, every
permission endpoint the export can reach becomes a rule: 117 of 117.

- **A — one action per endpoint.** The action becomes verb plus route
  (`"delete /users/:id"`). Every rule is finer.
  - **Cost: every existing action is renamed.** All 22 rules the default export produces
    today, across the fixtures and the sample, change their action, and so does any
    deployed policy or enforcement point built on them. The rule count grows to about one
    per endpoint.
- **B — a route condition only where endpoints collide.** Actions stay (controller,
  verb). A disagreeing group is split by route, and each part becomes a rule conditioned
  on `"<route>" == R.attr.route`, all-of with any permission condition.
  - **Cost:** a colliding action now needs the enforcement point to pass `R.attr.route`.
  - **Existing rules: 0 of 22 change.** A group that agrees is never touched.
  - Checked in the real engine, with Cerbos 0.55.0 and a test suite on RuoYi-Vue's real
    `gen get` pair (`/tool/gen/genCode/{tableName}` needs `tool:gen:code`,
    `/tool/gen/column/{tableId}` needs `tool:gen:list`). 6 of 6 pass: each holder is
    allowed on its own route and denied on the sibling's route and on a request with no
    route. A missing attribute denies, which is the safe direction.
- **C — keep (controller, verb) and name the collision.** Status quo; the report already
  names each collision and its siblings. It recovers nothing.

## Decision (proposed)

**B.** It recovers every collided endpoint that has a requirement, as A does. It changes
none of the 22 rules exported today, and it asks the enforcement point for the route only on
the actions that need it. A buys the same recovery by renaming every action.

- **Route value:** `Endpoint.Path` verbatim, the framework's own template.
- **Unguarded siblings** get their own omission reason (`no-guard`, or whatever their
  requirement dictates), as if they had never collided. **39 endpoints** on the measured
  targets move from "collision" to their true reason: 6 in `nestjs-boilerplate`, 1 in
  `blog-api`, 21 in videochat, and 11 in RuoYi-Vue declared.
- **Renaming a route changes the policy.** The condition names the template as written,
  so a renamed route no longer matches its rule and access is denied until the policy is
  regenerated. That is the safe direction, and it is also what happens to any policy
  derived from source: it describes the source it was generated from.
- **Endpoints whose route cannot be named** stay omitted exactly as today: unresolved
  paths, route collisions across controllers, unresolved versions, `ANY` verbs. B keys on
  a readable route and does not change those rules.
- **A group whose members share a route and still disagree** cannot occur outside ADR
  0014's merge, which already unions them. If it ever did, it stays an action collision.

### The contract, stated where it is read

B creates two calling contracts: most actions need only resource and verb, and a
route-split action also needs the route template. A deployer who does not know gets
unexplained denials. So the contract appears in three places:
- **each generated policy file** that uses a route condition carries a `ROUTE CONDITION`
  header comment. It names the attribute (`R.attr.route`) and the format (the template,
  `/users/:id` or `/api/customers/{id}`, never `/users/42`), and says that renaming a route
  denies until regeneration;
- **the export report** has a *Route-conditioned actions* section with the same contract,
  listing each split action;
- **the JSON report** carries `RouteContract` and `RoutedActions`, and each split rule
  carries its `Route`.

**The fail-closed behaviour is tested in the real engine**
(`TestRealWorldExport_AwesomeNestBoilerplate`, run by CI's Cerbos step). On the generated
`post` policy, a `RoleType.USER` principal is:
- allowed with `route: "/posts"`;
- denied with a concrete path (`/posts?page=1`, `/posts/42`);
- denied with no route.

Dropping the route condition from split rules fails the three deny cases, so the test
catches the regression it exists for.

### Measured, as implemented

`main` at `7c64037` against this implementation:
- **lint JSON:** byte-identical on all 42 targets;
- **exit codes:** unchanged;
- **exports:** changed on exactly the five predicted targets:

| Target | Rules before → after | Omissions before → after |
|---|---|---|
| `nestjs-boilerplate` | 7 → 9 | 8 → 6 |
| `awesome-nest-boilerplate` | 2 → 4 | 6 → 4 |
| `ghostfolio-shape` | 2 → 3 | 2 → 1 |
| `blog-api` | 0 → 1 | 4 → 3 |
| videochat | 3 → 11 | 47 → 39 |

**No existing rule line is removed** from any policy file. The only non-comment line
removed anywhere is `blog-api`'s `rules: []`, which now holds a rule. The 22 rules the
export produced before are unchanged.

## Alternatives considered

- **A**, rejected above on cost: the same recovery, and every action renamed.
- **C**, rejected: it recovers nothing, and the measured losses are in shipped output.
- **A route condition on every rule**, not only colliding ones. Rejected: it changes all
  22 existing rules and makes every enforcement point pass the route, for no recovery
  beyond B.
- **A semantic action vocabulary** (`approve`, `view`). Out of scope, as ADR 0009 §2 says:
  the model knows method and path, not business meaning.

## Consequences (if accepted)

- `internal/export/cerbos`: the split, the route condition, the contract and the list of
  conditioned actions in the report; `policy.go` already renders conditions (ADR 0041).
- Nothing else changes: the model, extraction, lint and the diff are untouched.
- Regression bar:
  - every target whose export has no collision is byte-identical, which is all 20 corpus
    repositories by default;
  - the four fixtures and videochat change as in the table above;
  - `cerbos compile` passes, with the route test suite in CI's real-engine tests.
