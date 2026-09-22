# 0036. An endpoint that lost its protection fails the build

## Status

**Accepted.** Amends [ADR 0007](0007-model-diff-design.md) §3, which is the CI-gating
contract and which that ADR says must be revisited rather than worked around.

## Context

`README.md` claims `sphinxor diff` "fails the build on a *regression*: an endpoint that
newly lost its protection, or one whose explicit exemption was quietly removed."

**The first half is false and the second is narrower than stated.** Constructed and
run against the shipped binary at `ae56b19`, not reasoned about:

| case | `becamePublic` reported | exit code |
|---|---|---:|
| Spring: `@PreAuthorize` removed from a mutating endpoint | `DELETE /api/users/{id}` | **0** |
| NestJS: `@UseGuards` + `@Roles` removed | `DELETE /users/:id` | **0** |
| Shiro `@RequiresPermissions` removed | **not even detected** | **0** |
| `sphinxor-allow` removed from a Low finding | — | **0** |
| `sphinxor-allow` removed from a High finding | — | 1 |
| New High finding (`empty-role`) appears | — | 1 |

The gate works; it is simply narrower than the sentence describing it. `HasRegressions()`
reads only `Regressions`, `diffRegressions` skips anything that is not
`ConfidenceHigh`, and `empty-role` is the only High-confidence rule
(`mutating_endpoint.go` and `unreferenced_permission.go` are both Low, by decision).
So **the gate fires on exactly one rule**, and an endpoint's protection vanishing is
computed, printed under *Became Public*, and then not gated.

ADR 0007 §3 put "endpoints that became public" under **structural diff
(informational)** deliberately. This reverses that specific placement and nothing else
about §3. The reversal has a warrant in §3's own text, which describes the failure
mode it was guarding against as an implementation that would "silently fail to gate a
real loss of protection" — the phrase names exactly the state the tool is in.

### Why not simply gate on the Low rule

`mutating-endpoint-without-access-control` is Low **by decision**, and
`docs/limitations.md` records at length why: a global guard, an unresolved composite,
a Shiro filter chain, an unreadable URL layer. On the 20-repository corpus it fires
**1,313 times across 16 repositories**, essentially all false positives in the safe
direction. Promoting it would make `sphinxor diff` unusable on exactly the projects
that need it.

The *transition* is a different fact from the *state*. "This endpoint has no guard" is
a claim about what Sphinxor can see. "This endpoint had a guard and now has none" is a
claim about a change Sphinxor saw both sides of, and it is only reachable when the
tool already recognized the protection that disappeared.

### The measurement: what "protection" has to mean

Extracted with the shipped extractor across all 20 repositories, counting endpoints
protected under the definition `becamePublic` uses today (a `GuardApplication` refers
to it) against the broadened one below:

| Repository | endpoints | narrow | broad | of which unrecognized-annotation only |
|---|---:|---:|---:|---:|
| `metersphere` | 1053 | **0** | 869 | 869 |
| `nacos` | 429 | **0** | 393 | 393 |
| `JeecgBoot` | 931 | **0** | 244 | 244 |
| `litemall` | 219 | **0** | 115 | 115 |
| `streampark` | 232 | **0** | 102 | 102 |
| `shenyu` | 394 | **0** | 100 | 100 |
| `inlong` | 339 | **0** | 79 | 79 |
| `thingsboard` | 551 | 519 | 519 | 0 |
| `RuoYi-Vue` | 147 | 116 | 116 | 0 |
| `eladmin` | 133 | 99 | 99 | 0 |
| `apollo` | 224 | 68 | 68 | 0 |
| the other 9 | 878 | 0 | 0 | 0 |
| **Total** | **5530** | **802** | **2704** | **1902** |

**Seven repositories have zero protection under the narrow definition and real
protection under the broad one.** On those, removing an authorization annotation from
any of 1,902 endpoints is invisible to a gate built on `GuardApplication` alone —
which the Shiro row of the first table shows concretely: that case produces no
*Became Public* entry at all today, not merely an ungated one.

That is the whole argument for §2. It is not symmetry for its own sake.

## Decision

### §1 A third regression case: protection lost

Adding to [ADR 0007](0007-model-diff-design.md) §3's two cases, a regression is also:

> **(c)** an endpoint present in **both** base and head, **protected** in base (§2),
> **unprotected** in head, and **not allowlisted** in head (§4).

`sphinxor diff` exits non-zero. Cases (a) and (b) are unchanged, and no lint rule's
confidence changes.

### §2 What counts as protection — any evidence of authorization, not a guard

An endpoint is **protected** in a snapshot when any of these refers to it:

1. a `GuardApplication` — method-level annotations, NestJS `@UseGuards`/`@Roles`,
   composite-resolved guards ([ADR 0006](0006-composite-decorator-resolution.md)), and
   **URL-layer grants**, which are `GuardApplication`s with
   `AppliedAt: ScopeRequestMatcher` ([ADR 0012](0012-securityfilterchain-effective-policy.md));
2. an `UnrecognizedAuthAnnotation` — Shiro
   ([ADR 0023](0023-third-party-authorization-annotations.md)), a same-named annotation
   from another package such as nacos's `@Secured`
   ([ADR 0022](0022-annotation-identity-and-unrecognized-authorization.md)), and
   `@PostAuthorize` ([ADR 0030](0030-post-authorize-and-method-security-filters.md));
3. a `PermissionReference` whose guard refers to it
   ([ADR 0035](0035-permissions-in-the-model.md));
4. an `AuthenticationRequirement` ([ADR 0010](0010-authenticated-any-role.md)).

**Losing a Shiro annotation is losing protection.** ADR 0029 §1's boundary —
*detecting* another system is not *supporting* it — is about what Sphinxor claims to
understand, not about whether its disappearance is a change worth failing a build
over. Sphinxor never claimed to know what `@RequiresPermissions("yarnQueue:create")`
requires; it does know the annotation was there and is not any more, and that is the
whole of what this gate asserts.

**Terms 3 and 4 are redundant today, and are named anyway.** Measured: every
`PermissionReference` in the corpus belongs to a guard that already satisfies term 1
(RuoYi-Vue 116 of 116, eladmin 89 of 99), and the Spring corpus has zero
`AuthenticationRequirement`s. Naming them costs two lines and buys the thing ADR 0011
§1 paid for the hard way, when two consumers each derived one fact by an implicit
route and a second framework would have made both silently wrong. ADR 0035 §7 already
defers NestJS permissions to a step where a `PermissionReference` need not travel with
a `GuardApplication`; on that day a definition written as "term 1, and the rest follow"
is wrong with no test failing.

**Protection is deliberately coarse: presence, never content.** It asks *is there
evidence of authorization*, not *what does it require*. Two consequences, both
accepted:

- Changing `hasRole('ADMIN')` to `hasRole('USER')` is **not** a transition and does
  not gate. A privilege *widening* is invisible to this gate. Catching it means
  ordering roles by strength, and Sphinxor holds no such order — ADR 0031 announces a
  `RoleHierarchy` precisely because it **cannot read one**. The structural diff shows
  the change; the gate does not fire on it.
- Adding protection is never a regression, in any direction.

### §3 Readable protection becoming unreadable is not a loss

An endpoint whose `@PreAuthorize("hasRole('ADMIN')")` becomes
`@PreAuthorize("@ss.hasPermi('user:delete')")` — or `@PreAuthorize("@validator.check(#id)")`,
which Sphinxor cannot read at all — is protected on both sides and **must not gate**.

This follows from §2 rather than needing a special case, and both shapes were verified
against the shipped binary to produce no *Became Public* entry today. It is written
down because the failure mode is specific and bad: `RolesUnresolved` is a fact about
**extraction**, not about the application
([ADR 0020](0020-unanalyzable-is-unknown-not-absent.md) Amendment 3). A gate that
fired on it would fail a PR for a refactor that changed no protection whatsoever, and
would fire *more* the less Sphinxor understands — the incentive exactly inverted. The
mirror case, unreadable becoming readable, is likewise not a transition.

### §4 An endpoint deliberately made public, and marked, does not gate

An endpoint carrying a `sphinxor-allow` marker
([ADR 0003](0003-allowlist-format.md)) in **head** does not produce a regression under
(c), whatever its base state. The marker is the author's statement about the code as
it now stands, which is exactly this case: *yes, I removed that guard, here is why.*

**The allowlist state must be carried in the `Snapshot`, not derived from findings**,
and that is measured rather than assumed. A `GET` endpoint that loses its
`@PreAuthorize` and gains a marker reports, today:

```
1 endpoint(s), 0 finding(s): 0 blocking, 0 warning, 0 allowlisted.
```

The marker is matched and the endpoint **is** allowlisted — it produces no
`stale-allow-marker`, so `MatchFile` resolved it — but
`mutating-endpoint-without-access-control` does not fire on a `GET`, so **no finding
exists to carry `Allowlisted: true`**. Deriving the set from findings would gate a
deliberately-public `GET`, with the author's marker sitting one line above the
handler. `Snapshot` therefore gains `AllowlistedEndpoints`, populated from
`allowlist.Outcome` — the same set `lint.Run` already receives, crossing no new
boundary. ADR 0007 §1 chose in-process re-extraction specifically so allowlist status
could not be dropped between the two sides, and this keeps that property.

### §5 An identity change is removed-plus-added, and does not gate

`Endpoint.ID` is derived from `HTTPMethod + Path` (`model.NewEndpointID`), so a route
that gains a version, or whose path becomes unreadable, is a **different endpoint**:
it appears in `RemovedEndpoints` and `AddedEndpoints`, and (c)'s "present in both"
test fails. It does not gate. Verified: moving a protected `DELETE /api/users/{id}` to
`/api/v2/users/{id}` produces `+ DELETE /api/v2/users/{id}` and
`- DELETE /api/users/{id}`, and no *Became Public* entry.

**This is accepted, and the cost is named rather than implied.** A single PR that both
renames an endpoint and removes its guard will not gate.

Three reasons it is the right trade:

- **It is not silent.** The removal and the addition are both in the structural diff,
  and the added endpoint carries whatever findings it earns.
- **The alternative is the matching this project has twice refused.** Pairing a removed
  endpoint with an added one means fuzzy or positional matching across an identity
  change — rejected outright by [ADR 0002](0002-intermediate-model-structure.md) and by
  ADR 0007 §2, which chose derived stable keys precisely to avoid it. A wrong pairing
  produces a **false gate on a rename**, which is the noise-fatigue failure
  `vision.md` is built to avoid, and renames are common where guard removals are rare.
- **It is consistent with §6, not an arbitrary hole.** A renamed endpoint arriving
  without protection is, to the model, a *new* unprotected endpoint — and §6
  deliberately does not gate those.

### §6 A new unprotected endpoint does not gate — a decision, not an omission

An endpoint present in head and absent from base never produces a regression under
(c), however unprotected and however mutating.

The reason is measured. `mutating-endpoint-without-access-control` fires **1,313 times
across 16 of the 20 corpus repositories**, and `docs/limitations.md` records why
almost all of them are false positives in the safe direction — a global `APP_GUARD`, a
Shiro `ShiroFilterFactoryBean` with a `/**` catch-all, a `SecurityFilterChain` that
will not parse. On `inlong`, read by hand, **114 mutating routes** fall under an
authenticating catch-all with zero `anon`. Gating new endpoints would fail essentially
every PR that adds a route to any of those projects, on evidence the tool itself
grades Low.

(c) requires protection in base, which is evidence Sphinxor positively recognized. That
is the difference, and it is what keeps this gate quiet on a corpus where the Low rule
is loud.

### §7 How it is reported

A regression under (c) carries a synthesized `model.Finding`:

- `RuleID: "endpoint-became-public"`, `Confidence: ConfidenceHigh`,
  `SubjectKind: SubjectEndpoint`, `SubjectID` the endpoint's ID;
- a new `RegressionReason`, `ReasonBecamePublic`.

It renders through the existing row — `- [BECAME-PUBLIC] \`endpoint-became-public\`: …` —
so no output shape changes.

**`endpoint-became-public` is not a lint rule and `sphinxor lint` never emits it.**
It is a diff-only fact with no meaning in a single snapshot, it is absent from
`lint.DefaultRules()`, and it is not in `docs/vision.md`'s three-rule set. Synthesizing
a `Finding` is how it reaches the existing report and JSON shape without a parallel
structure; the name is chosen to read as what it is rather than as a fourth rule.

The *Became Public* section stays exactly as it is. It lists the transition whether or
not it gated, so an allowlisted one remains visible instead of vanishing from the
report because it was excused.

## Alternatives considered

- **Promote `mutating-endpoint-without-access-control` to High.** Rejected on the
  measurement above: 1,313 findings across 16 repositories, near-universally false
  positives in the safe direction. It would also gate the *state*, not the transition,
  failing every PR on such a project forever rather than on the commit that changed
  something.
- **Correct the README instead and change no behaviour.** The honest minimum, and
  rejected because `vision.md` names this capability as the differentiator in the same
  words the README uses — "endpoints that became public" — and the fact is already
  computed. The gap is one condition wide.
- **Define protection as `GuardApplication` only**, the current `becamePublic`.
  Rejected on measurement: blind on seven repositories and 1,902 endpoints, including
  every Shiro project in the corpus.
- **Gate on any change to an endpoint's requirement**, including `ADMIN` → `USER`.
  Rejected per §2: Sphinxor holds no ordering over roles, so "weaker" is not a fact it
  can assert. A gate that fired on every role edit would be a gate on churn.
- **Match removed endpoints against added ones** to catch a rename-plus-unguard in one
  PR. Rejected per §5, on the same grounds ADR 0002 and ADR 0007 §2 already settled.
- **Derive the allowlist from findings** rather than carrying the set. Rejected on the
  measured `GET` case in §4: the flag has nothing to ride on when no rule fires.
- **A separate exit code for a became-public regression.** Rejected: `sphinxor diff`
  has one gating contract, and a second failing code would make every CI integration
  choose which failures it cared about, which is the allowlist's job.

## Consequences

- `internal/diff`: `becamePublic` broadened per §2; `Snapshot.AllowlistedEndpoints`;
  `ReasonBecamePublic`; `diffRegressions` gains case (c).
- `internal/cli`: `analyzeDirectory` returns the allowlisted set for the diff path;
  `lint` and `export cerbos` are unaffected in behaviour.
- `internal/lint`, `internal/report`, `internal/export/cerbos`, `internal/model`:
  **no change**. No rule, no confidence, no model field moves.
- **`README.md`'s diff sentence is corrected in the same change** to describe what
  gates, rather than being left ahead of the code a second time.
- **This is an exit-code change**, and `CHANGELOG.md` says so under its own heading. A
  pipeline that was green can go red — which is the point, but it is not something to
  discover from a diff.

### The regression bar

- **The gate is a no-op on a single-snapshot corpus.** Every one of the 20
  repositories diffed against itself must report zero regressions and exit 0. There
  are no transitions in a snapshot compared with itself, and the point of running it
  is that the broadened definition cannot invent one.
- The eight constructed cases, with the exit code each must produce:

  | case | after |
  |---|---:|
  | Spring `@PreAuthorize` removed | **1** |
  | NestJS `@UseGuards` + `@Roles` removed | **1** |
  | Shiro `@RequiresPermissions` removed | **1** |
  | protection removed **and** `sphinxor-allow` added | **0** |
  | `GET` protection removed **and** `sphinxor-allow` added | **0** |
  | `hasRole('ADMIN')` → `@ss.hasPermi('user:delete')` | **0** |
  | `hasRole('ADMIN')` → `@validator.check(#id)` (unreadable) | **0** |
  | protected endpoint gains a version (identity change) | **0** |
  | new unprotected mutating endpoint added | **0** |

- `sphinxor lint` output stays byte-identical everywhere: this decision touches no
  rule and no report.

### Measured after implementation

All nine constructed cases produce the predicted exit code. Every one of the 20
corpus repositories diffed against itself reports zero regressions and exits 0, and
`sphinxor lint --format json` is byte-identical on all 20 against the 0.7.0 binary.

**One addition to §7, made during implementation rather than designed in.** The
report listed an excused transition with no indication of why it had not failed the
build, leaving a reader to reconcile `0 regression(s)` with an endpoint named under
*Became Public*. An allowlisted transition now renders as
`- DELETE /api/users/{id} (allowlisted — does not fail the build)`, derived in the
renderer from the absence of a matching regression rather than from a new field. It
is §7's "excused is not the same as invisible" carried one step further: excused is
not the same as *unexplained* either.
