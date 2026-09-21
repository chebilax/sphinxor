# 0028. A handler that maps every HTTP verb

## Status

Accepted.

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

### §4 Overlap with a verb-specific mapping on the same path

Spring matches the most specific mapping first, so a path can carry both. The corpus
contains **exactly one instance**, and it is a clean one —
`spring-cloud-dataflow`'s `TaskSchedulerController`:

```java
@RequestMapping("/tasks/schedules")                     // class-level base path
…
@RequestMapping("/instances/{taskDefinitionName}")      // verb-less
public PagedModel<ScheduleInfoResource> filteredList(…)

@DeleteMapping("/instances/{taskDefinitionName}")       // DELETE only
public void deleteSchedulesforDefinition(…)
```

At runtime `DELETE` reaches `deleteSchedulesforDefinition` and **every other verb**
reaches `filteredList`. They are two endpoints, not a duplicate.

**The model records them as two endpoints and does not compute the complement.** The
any-verb endpoint means "all verbs", not "all verbs except those a sibling claims".

Computing the complement was considered and rejected: it makes one endpoint's meaning
depend on its siblings, and identity, `sphinxor diff` and the exporter all assume an
endpoint means what its own annotations say. Adding a sibling elsewhere in the class
would silently change an existing endpoint's identity, which is the order-dependence
ADR 0020 Amendment 2 spent an amendment removing from route collisions.

**They do not collide, and do not need to.** `ANY /x` and `DELETE /x` are different
`HTTPMethod`s, so `model.NewEndpointID` already gives them separate identities and
ADR 0020 Amendment 2 §8's machinery never fires. It does not need to, because §8's
property is achieved here for free: **each handler's guards attach to its own row**,
so neither endpoint can report the other's protection — which is the whole thing §8
exists to prevent.

**What is inaccurate, and in which direction.** The `ANY` row claims to cover
`DELETE`, which at this path it does not. That over-states *which verbs the row
covers*, never *what protects them*: the real `DELETE` endpoint is present with its
own guards and gets its own finding if it has none. So a reader can be misled about
which handler serves `DELETE`, but not into believing an unguarded route is guarded.
The error is one-directional and lands on the safe side, which is the same standard
ADR 0012's intersection and ADR 0026's recovered routes are held to.

Specified although one instance exists, on the same grounds as ADR 0024 §4 and
ADR 0026 §4: a shape the corpus barely exercises is exactly the one whose behaviour
should be written down rather than discovered later.

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
### Validation — measured after implementation

**138 endpoints, 128 findings.** The number that mattered was **not 504**: that is
what Option A's behaviour would have produced, and its absence is the check that the
rejected representation was not implemented by accident.

| Repository | endpoints | mutating findings |
|---|---|---|
| JeecgBoot | 855 → 931 | 227 → 294 |
| shenyu | 371 → 394 | 122 → 145 |
| spring-cloud-dataflow | 103 → 115 | 45 → 57 |
| apollo | 217 → 224 | 43 → 50 |
| litemall | 213 → 219 | 39 → 45 |
| inlong | 334 → 339 | 184 → 189 |
| thingsboard | 547 → 551 | 10 → 14 |
| RuoYi-Vue, dolphinscheduler, eladmin, nacos, nakadi | +1 each | +1 each, except nacos |

No other repository changed, and all four vendored fixtures are byte-identical.

**The three-endpoint gap, traced rather than absorbed.** The measurement predicted
141 and the extractor produced 138, the whole difference being JeecgBoot's 79 against
76. The three are in `ISysBaseAPI.java` — a Java **interface**, not a controller
class. The probe walked every `method_declaration` carrying the annotation; the
extractor requires the enclosing type to be a `@RestController`/`@Controller`, and
correctly produces nothing for an interface. The probe over-counted; the extractor is
right. (A nested controller class was considered first and ruled out: the corpus
contains none, though it is worth knowing that a nested one would also produce
nothing — see `docs/limitations.md`.)

The finding count came in at 128 against 126 for the same reason one level down: the
absorption classification credited class-level annotations that the real guard
attachment resolves differently. Both gaps are the ADR 0024 "approximate prediction"
case — a scan guessing what extraction will do — and both are small and in the
direction of the scan over-crediting itself.

Tests: `isMutating` treats `ANY` as mutating and fires **once** per handler, not four
times; a class-level verb-less `@RequestMapping` still yields only a base path; §4's
overlap yields two endpoints whose guards do not leak across; a verb-scoped URL rule
neither grants its roles nor falls through to a later `permitAll()`; and the Cerbos
exporter omits an any-verb endpoint with `ReasonAnyVerb` rather than writing a rule
on an action no request carries.

**One test was deliberately inverted, not deleted.** ADR 0026 §4's
`TestRequestMappingMethod_VerblessStaysOut` asserted this shape produced no endpoint.
That exclusion was explicitly temporary — ADR 0026 declined the model decision rather
than judging the shape unimportant — so the test is renamed and its assertion
reversed in place, with the supersession recorded in its comment, to keep the shape's
history where it is tested.