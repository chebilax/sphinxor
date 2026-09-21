# 0032. Routes a recognized controller did not yield are announced, on two conditions

## Status

Accepted.

## Context

[ADR 0029](0029-spring-security-scope.md) §3 has three items left, all endpoint
discovery. Two are separate entries on that list:

- **Routes declared on an inherited interface** — a `@RestController` whose
  handlers are `@Override` methods, with the mappings on an interface.
- **A method-level mapping meta-annotation** — `@AnonymousGetMapping`,
  `@ShenyuPostMapping`: a project-declared annotation composing
  `@RequestMapping`, which [ADR 0024](0024-controller-meta-annotations.md) §1
  resolves at the class level only.

Both hide routes from a class already recognized as a controller. This ADR
announces them without resolving them.

### Two conditions, because one does not cover it

The first condition drafted was **a recognized controller that yielded zero
endpoints**. It is clean and it covers both causes — but only when a controller is
*entirely* hidden. It misses the partially-inherited case, which is worse than a
controller vanishing: the controller appears in the matrix, some of its routes are
listed, and nothing signals that the rest are missing.

`apollo`'s `PortalManagementController` is that case. **47 `@Override` methods, 9
carrying an inline mapping, 38 relying on the external interface, and 29
`@PreAuthorize`.** It yields 9 endpoints, so it is not zero and the first
condition never fires on it.

So a second condition was measured: **a public `@Override` method with no mapping
annotation, in a controller class that implements an interface.**

### The measurement

Both conditions, over all 20 repositories, with the real extractor for the first
and a dedicated probe for the second.

**Condition A — recognized controller, zero endpoints recovered:**

| Repository | Controllers | Endpoints | Yielding zero | Cause |
|---|---|---|---|---|
| dataease | 38 | 16 | **31** | inherited interfaces |
| apollo | 64 | 224 | **13** | inherited interfaces |
| shenyu | 83 | 394 | **2** | `@ShenyuRequestMapping` / `@ShenyuPostMapping` |
| eladmin | 26 | 133 | **2** | `@AnonymousGetMapping` |
| the other 16 | — | — | **0** | — |

48 controllers, 4 repositories, **zero false positives** — all 48 checked
individually, none in test code. Two apparent false positives are not: eladmin's
`AppRun` is a `@SpringBootApplication` that is also a `@RestController` and looks
routeless, but carries `@AnonymousGetMapping("/")`; nacos's `ExtractorManager`
appeared only in an early regex pass, because the string `@Controller` occurs in
one of its javadoc comments.

**Condition B — `@Override`, no mapping, in a controller implementing an
interface.** Stated plainly it is far too broad: **9 repositories, and 5 of them
pure noise** — `InitializingBean` (JeecgBoot), `Callback<Map>` (nacos),
`SubscriptionOutput` (nakadi), `ClientRegisterConfig` (shenyu), `Function<…>`
(spring-cloud-dataflow), `Predicate<Method>` (halo). Those are real controllers
implementing a non-routing interface, so the file is right and the method is
wrong.

Narrowing it the way this project narrows everything else — **by what the
interface actually is, resolved rather than guessed** — removes all of it:

| Interface | Treatment | Effect |
|---|---|---|
| in-repo, carries mapping annotations | **route-bearing** | dataease: 222 methods / 32 files |
| not in the repository, not a framework package | **unknown, so announced** | apollo: 146 methods / 14 files |
| in-repo, no mapping annotations | not a routing interface | nacos, nakadi, shenyu, thingsboard: excluded |
| `java.*`, `javax.*`, `jakarta.*`, `org.springframework.*`, … | framework type | JeecgBoot, dataflow, halo: excluded |

**2 repositories, 368 methods, 46 files, zero false positives** at probe stage.
(The implemented rule, with the `default`-method arm added, also reaches shenyu's
eight `PagedController` implementors — 16 further hidden routes the probe missed.)
apollo's 14 files
are its 13 fully-hidden controllers plus `PortalManagementController` — which is
exactly the "14 OpenAPI v1 controllers" `docs/limitations.md` has always
described.

### Neither condition subsumes the other

This was the question to settle, and the answer is no, in both directions:

- **B misses the meta-annotation cause entirely.** It scores **zero** on eladmin,
  and shenyu's `OrderController` has **zero** `@Override` methods. Those handlers
  are ordinary methods carrying an unresolved annotation; no interface is
  involved.
- **A misses the partially-inherited case**, which is why B was measured at all.

So both are kept. Union: **50 controllers across 4 repositories, zero false
positives.**

## Decision

### §1 Two conditions, one announcement

A controller is reported as having unrecovered routes when **either**:

- **A** — it is a recognized controller and no `Endpoint` refers to it; or
- **B** — it implements an interface that is **route-bearing in this repository**
  (declared here, carrying mapping annotations); or it implements an interface
  **not in this repository and not a framework type**, *and* has a public
  `@Override` method with no mapping annotation.

**Condition B has two arms because implementing the first draft found a third
shape.** The draft was "unmapped `@Override` method plus an interface", and it
flagged eight shenyu-admin controllers. They turned out to be real, for a reason
the condition was not expressing: `PagedController` declares
`@PostMapping("list/search")` and `@PostMapping("list/search/adaptor")` as
**`default` methods**, and the eight classes override only `pageService()`, which
is not a route. The rule found the right controllers by accident and would have
missed any implementor that overrode nothing at all. A route-bearing in-repo
interface therefore suffices on its own — its mapped methods are routes the class
serves, and this extractor reads no interfaces.

The `@Override` signal is still needed for the *unknown* arm, where nothing can
distinguish a routing interface from an ordinary one and flagging every controller
implementing any external type would be noise.

**No count of the missing routes is recorded.** shenyu is why: two routes hide
behind a class that overrides one unrelated method, so any number derived from the
class alone is wrong. Establishing one means resolving the interface, which this
decision does not do.

**The cause is deliberately not diagnosed.** Condition A in particular reports the
one thing that is certain — *this is a controller and no routes came out of it* —
which holds for every cause including causes not yet encountered. Two
cause-specific detectors would cover exactly the shapes the corpus contains and
stay silent on the next one, which is the failure that cost 611 routes when ADR
0011 §1 cut `@RequestMapping(method = …)` for want of a fixture.

Condition B is narrower by necessity: it cannot avoid asking *what the interface
is*, because without that it is 5/9 noise. That question is answered by resolving
the interface, not by a name heuristic — the same discipline
[ADR 0023](0023-third-party-authorization-annotations.md) §1 applied when it
rejected a name list for third-party annotations.

**The interface index is keyed by fully-qualified name, and a name declared twice
with different answers is treated as ambiguous rather than routing.** ADR 0022's
"spelling is not identity" applies to interfaces as well, and JeecgBoot proved it
during implementation: it declares `org.jeecg.common.airag.api.IAiragBaseApi`
twice — in `jeecg-system-local-api` with no mappings, and in
`jeecg-system-cloud-api` as a Feign client with seven. A simple-name index
conflated them and produced this condition's only corpus false positive. Same FQN
in two modules, so even FQN keying cannot choose between them: which is on the
classpath is a build profile, and a warning resting on a guess about that is not
worth issuing. Excluded, and recorded in `docs/limitations.md`.

### §2 What the announcement claims, and what it must not

It says routes were **not recovered**, never that routes **exist**.

A `@Controller` whose methods are all `@ExceptionHandler`s, or a `@RestController`
kept as a marker on an application class, is a genuinely routeless controller and
a correct zero. The corpus contains no such case, but the wording must not turn a
future one into a false alarm:

> 31 recognized controller(s) produced no routes, and 1 produced fewer than it
> declares: `ChartDataServer`, `ChartViewServer`, … Their handlers' mappings were
> not recognized — declared on an inherited interface, behind a method-level
> meta-annotation, or in a shape `docs/limitations.md` does not yet list. Any
> authorization on those handlers is missing from this report along with the
> routes.

The last sentence is what matters for an authorization tool. apollo's 14
controllers take **75 `@PreAuthorize`** with them — 46 on the 13 that vanish
entirely, 29 on `PortalManagementController`, which does not.

That also resolves a discrepancy this ADR opened. `docs/limitations.md` said "72",
and condition A alone measured 46, which looked like the older figure being wrong.
It was not: 46 + 29 = **75**, the whole `openapi/v1/controller` directory. The
partially-inherited controller is where the difference lived, which is a second
reason condition B belongs here.

### §3 It changes no finding and no export

Condition A's controllers have no endpoints, so no rule can fire and the exporter
has nothing to omit. Condition B's controllers keep exactly the endpoints they
already had; the unrecovered ones are still unrecovered, and nothing is invented
for them.

This is a warning and only a warning. Nothing downstream consults the new status
to decide whether an endpoint is protected, which keeps the
[ADR 0011](0011-spring-second-framework.md) §1 `DeclaresRoles` hazard out of
scope: one new project-level fact, one consumer.

### §4 The count is the endpoint count's missing companion

The matrix reports `N endpoint(s)` and a reader cannot tell whether N covers the
application. For these four repositories it does not, and the gap is large —
dataease reports **16 endpoints from 38 controllers**.

So the warning leads with counts and follows with names: the number tells a reader
the matrix is incomplete and how incomplete, the names tell them where to look.

## Alternatives considered

- **Condition A alone.** Rejected: misses `PortalManagementController`, the case
  where the report is most misleading because the controller is present and looks
  complete.
- **Condition B alone**, on the theory that it subsumes A. Rejected on
  measurement: it scores zero on eladmin and on shenyu's meta-annotation
  controllers, which involve no interface at all.
- **Condition B without resolving the interface.** Rejected: 5 of 9 repositories
  are false positives, all of them controllers implementing an ordinary
  non-routing interface.
- **A denylist of known non-routing interfaces** (`InitializingBean`, `Function`,
  `Predicate`, …). Rejected for ADR 0023 §1's reason: a name list covers the
  members the corpus happens to contain and silently mis-handles the rest.
  Resolving the interface answers the actual question.
- **Diagnose the cause in the message** ("declared on an interface"). Rejected: a
  wrong diagnosis sends a reader looking in the wrong place, and §1 avoids needing
  one.
- **Report it as a finding.** Rejected per §3: a finding attaches to a subject,
  and condition A's subject is an endpoint that does not exist.

## Consequences

- `internal/model`: one project-level status listing the controllers and which
  condition matched. No per-endpoint state.
- `internal/extract/spring`: a post-pass comparing `Controllers` against the
  controller IDs referenced by `Endpoints`, plus a project-wide interface index
  (name → declared in this repo, carries mappings) for condition B. That index is
  the same infrastructure a future interface-resolution decision would need.
- `internal/cli/analyze.go`: one new project warning.
- `internal/lint`, `internal/export/cerbos`, `internal/diff`: unchanged (§3).
- [ADR 0029](0029-spring-security-scope.md) §2 moves **two** rows from `silent` to
  `detected and announced`.
- **First Spring decision since ADR 0028 with a non-zero corpus effect.** The
  regression bar is therefore not a no-op, and is met exactly: **apollo, dataease,
  eladmin and shenyu gain a warning; the other sixteen are byte-identical; every
  endpoint and finding count is unchanged in all twenty.**

  | Repository | Announced |
  |---|---|
  | dataease | 31 produced no routes, 1 produced fewer |
  | apollo | 13 produced no routes, 1 produced fewer (`PortalManagementController`) |
  | shenyu | 2 produced no routes, 8 produced fewer (`PagedController` implementors) |
  | eladmin | 2 produced no routes (`AppRun`, `LimitController`) |
- `docs/limitations.md` gains dataease's 31, which nothing currently records, and
  its apollo figures are reconciled per §2.
- **Recorded for later, and it does not point where the route count suggested.**

  | | Controllers | Routes recoverable? | Authorization recoverable? |
  |---|---|---|---|
  | apollo | 14 | **No** — interfaces come from an external artifact | would be **75 `@PreAuthorize`**, which the model reads |
  | dataease | 32 | **Yes** — 32 of 33 interfaces are in the repository | **No** — see below |

  dataease is the larger case on route count, and on route count only. Its
  authorization is **97 `@DePermit`** annotations, declared at
  `io.dataease.auth.DePermit` and written entirely on the interfaces, carrying
  permission expressions such as `{"#p0.id+':manage'"}`. That is a project-local
  annotation, which [ADR 0029](0029-spring-security-scope.md) §1 puts out of
  scope, expressing a permission rather than a role, which is the open ADR 0002
  question in `docs/limitations.md`.

  It goes further, on the measurement
  [ADR 0023](0023-third-party-authorization-annotations.md) established for
  third-party annotations: **`@DePermit` has no reader anywhere in dataease's
  source.** Only the declaration, the imports and the 97 uses — no aspect, no
  interceptor, no reflective lookup. Its interfaces sit under
  `io.dataease.api.xpack`, so enforcement is presumably in a module not in the
  repository. Resolving dataease's interfaces would therefore surface 31
  controllers' routes carrying an annotation that is neither read by this tool nor
  visibly enforced in the code, and any finding on those endpoints could not be
  judged true or false from the source available.

  So the choice for a future resolution decision is between routes whose
  authorization the model cannot hold and cannot even see enforced (dataease), and
  authorization the model *can* hold whose routes are unreachable (apollo).
  Written down because it is exactly the conclusion that gets re-derived from the
  route count and comes out backwards.
