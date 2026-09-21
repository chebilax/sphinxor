# 0028. A handler that maps every HTTP verb

## Status

Proposed.

## Context

[ADR 0026](0026-requestmapping-method-attribute.md) §4 deliberately left one shape
out: a **method-level `@RequestMapping` with no `method` attribute**. In Spring that
maps *every* HTTP verb to the handler — `GET`, `HEAD`, `POST`, `PUT`, `PATCH`,
`DELETE`, `OPTIONS` and `TRACE`.

```java
@RequestMapping("/anything")
public Response anything(...) { … }
```

It was left out because representing it is a model question, and bundling a second
model decision into ADR 0026's `HEAD`/`OPTIONS`/`TRACE` change is how a focused
decision becomes a two-headed one. This is that decision.

### How many, measured with the right instrument

ADR 0026's postmortem established that a windowed regex miscounts what is
method-level, and this is the same question, so it was measured with a tree-sitter
walk that distinguishes a `class_declaration`'s annotations from a
`method_declaration`'s.

| | |
|---|---:|
| **Class-level** verb-less `@RequestMapping` | **735** |
| **Method-level** verb-less `@RequestMapping` | **141** |

The class-level 735 are path prefixes and are already handled correctly; they are not
endpoints and must not be counted. **The population is 141, across 12 repositories** —
JeecgBoot 79, shenyu 23, spring-cloud-dataflow 12, apollo 7, litemall 6, inlong 5,
thingsboard 4, and one each in RuoYi-Vue, dolphinscheduler, eladmin, nacos, nakadi.

*A previous count said 164 across 17 repositories.* That was the windowed regex
again, counting some class-level annotations as method-level. 141 is the parse.

### Almost nothing absorbs this one

Each of the 141 was classified by what its handler or class already carries:

| | |
|---|---:|
| Carries a Spring Security guard | **1** |
| Carries an Apache Shiro annotation (ADR 0023) | **14** |
| **Carries nothing** | **126** |

Only 15 of 141 are absorbed — about 11%, against roughly 46% for ADR 0026. This
decision therefore lands nearly its full cost as new findings, whichever
representation is chosen, and the two representations differ by a factor of four in
how many that is.

## The two representations, and what each costs

### Option A — one `Endpoint` per verb, eight rows per handler

Faithful to Spring: the route really does answer all eight verbs.

- **Consumers that must change: zero.** Every existing consumer already handles a
  set of concrete verbs.
- **Matrix rows: 141 → 1,128.** A net **+987 rows**, the large majority of them
  `HEAD`, `OPTIONS` and `TRACE` entries nobody reads.
- **Findings: ×4.** `mutating-endpoint-without-access-control` fires per *endpoint*,
  and four of the eight verbs are mutating, so each of the 126 unprotected handlers
  produces **four findings naming the same handler** — **504 findings**, against 126
  handlers. On JeecgBoot alone that is 280 findings on top of its current 227.

Each of those four findings is individually true — `POST /x`, `PUT /x` and
`DELETE /x` really are routable and unguarded. They are also four restatements of
one fact about one line of code, and a rule that says the same thing four times is
how a CI gate gets switched off.

### Option B — one `Endpoint` whose verb is "any"

Compact and equally faithful: one handler, one row, marked as answering every verb.

- **Matrix rows: 141 → 141.** No inflation.
- **Findings: 126**, one per unprotected handler.
- **Consumers that must change: six**, enumerated below, and this is the whole cost.

## Decision

### §1 One endpoint per handler, with an explicit any-verb term

`model.HTTPMethod` gains `MethodAny` ("ANY"), and a verb-less method-level
`@RequestMapping` produces exactly one `Endpoint` carrying it.

Option A is rejected on the measurement, not on taste: **504 findings restating 126
facts**, and +987 matrix rows of `TRACE` and `OPTIONS`, is a worse report than 126
findings and 141 rows. Faithfulness to Spring's routing table is not the goal; an
authorization model a human will read is.

`MethodAny` is a **positive fact, not an unknown**. Spring really does route every
verb there; nothing was unanalyzable. It is therefore not an
[ADR 0020](0020-unanalyzable-is-unknown-not-absent.md) case and does not get that
family's warning-and-omit treatment by default — each consumer below decides on its
own merits.

### §2 The six consumers, each with its required behaviour

This section is the decision. ADR 0011 §1 found two consumers silently depending on a
convention neither checked, and the lesson taken from it is not "never add model
state" — it is **"when you add one, enumerate every consumer before shipping."**
Every reader of `HTTPMethod` in the codebase was listed and triaged; six need to know
about `MethodAny`, and the rest only print it.

1. **`isMutating` (`internal/lint/mutating_endpoint.go`)** — `MethodAny` **is
   mutating**. The handler accepts `POST`, `PUT`, `PATCH` and `DELETE`; a rule about
   state change must say so. This is what produces the 126 findings.
2. **The Cerbos action (`internal/export/cerbos/policy.go`, `translate.go`)** — the
   action is the lowercased method, and `any` is **not an HTTP action**. Exporting it
   would write a Cerbos rule granting an action no request ever carries: a policy
   that looks like a grant and governs nothing. Such an endpoint is **omitted**, with
   its own omission reason, per [ADR 0009](0009-cerbos-exporter.md) §3's
   never-guess-permissively posture.
3. **URL-layer rule matching (`internal/extract/spring/securityfilterchain.go`)** —
   a rule scoped to one verb (`requestMatchers(POST, "/x")`) neither clearly matches
   nor clearly misses an any-verb endpoint: it governs one of the eight. Resolving
   that to "matches" would over-report the rule's roles across seven verbs it does
   not cover, and to "misses" would drop a rule that really does apply. It is
   therefore **unresolved**, which [ADR 0018](0018-unrecognized-rule-stops-evaluation.md)
   already defines: evaluation stops and the endpoint's URL layer is unknown. An
   unscoped rule (`anyRequest()`, or a matcher with no method) applies normally.
4. **Endpoint identity (`model.NewEndpointID`)** — `ANY /x` is its own identity.
   Where a project declares both `@GetMapping("/x")` and a verb-less
   `@RequestMapping("/x")`, those are two endpoints that overlap at runtime, which is
   [ADR 0020](0020-unanalyzable-is-unknown-not-absent.md) Amendment 2 §8's
   collision case and is already handled: kept apart, never merged.
5. **Collision detection (`internal/extract/collide`)** — compares and sorts by
   `HTTPMethod`, so it sees `ANY` as a distinct verb and needs no special case. Named
   here because it reads the field and was checked, not because it changes.
6. **`sphinxor diff` (`internal/diff`)** — identity-derived, so a route that later
   gains an explicit verb is reported as one endpoint removed and one added. That is
   the same consequence ADR 0020 Amendment 1 §5 accepted for a path becoming
   readable, and is accepted here for the same reason.

Everything else that touches `HTTPMethod` — the matrix, the export report tables,
the diff report — prints the string and needs nothing.

### §3 The findings say what they mean

126 endpoints become `mutating-endpoint-without-access-control` findings at Low
confidence. Their message must name the shape, because "`ANY /x` has no detected
guard" invites a reader to look for a verb that is not in the source.

And the caveat that applies to every recent endpoint-discovery decision applies here:
of the 12 repositories affected, the ones gaining most of the findings — JeecgBoot,
shenyu, inlong, litemall — all have a URL layer this extractor cannot read, and since
[ADR 0027](0027-unannounced-url-layers.md) every one of them says so.

## Alternatives considered

- **Option A, eight rows per handler.** Rejected above on 504-versus-126.
- **Detect and warn without creating endpoints.** Zero model change, zero consumers,
  zero findings — and it leaves 141 real endpoints out of the matrix, which is the
  gap the last four decisions have been closing. It would be choosing the quiet
  answer over the true one.
- **Expand to the five "commonly used" verbs**, dropping `HEAD`/`OPTIONS`/`TRACE`.
  Rejected: it still multiplies findings threefold and the cut is arbitrary, since
  ADR 0026 §2 just added those three on the grounds that a declared endpoint is a
  fact.
- **A `Methods []HTTPMethod` field on `Endpoint`.** More faithful still, and a much
  larger model change — identity, diff, the exporter and the matrix all assume one
  verb per row. Not justified by 141 handlers.

## Consequences

- `model.HTTPMethod` gains one term. Six consumers change, each specified in §2.
- 141 endpoints appear; 126 new Low-confidence findings, in projects that mostly
  carry an announced unreadable URL layer.
- One new Cerbos omission reason.
- **Validation before this is Accepted**, per `docs/testing.md`:
  - The 12 repositories gain exactly 141 endpoints between them, distributed as
    measured above, and **126** new mutating findings — not 504, which is the number
    that would indicate Option A's behaviour had been implemented by accident.
  - **No other repository changes**, and all four vendored fixtures stay
    byte-identical — none declares a verb-less method-level `@RequestMapping`.
  - A test per §2 consumer: `MethodAny` is mutating; a Cerbos export omits it with
    the new reason rather than emitting an `any` action; a verb-scoped URL rule
    leaves it unresolved while an unscoped one applies; and `ANY /x` alongside
    `GET /x` is a collision rather than a merge.
  - A test that a **class-level** verb-less `@RequestMapping` still contributes only
    a base path and creates no endpoint — the 735, which must not move.
