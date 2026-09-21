# 0023. A third-party authorization annotation is found-and-not-understood, not absent

## Status

Accepted.

§1's on-demand-import clause was reversed during implementation: a test showed that
honoring a wildcard bound every unimported annotation in the file to the Shiro
package, including `@RestController`. The reasoning and the measurement that make
excluding it the cheaper error are in §1.

§3 was added during review. The draft suppressed 907 findings on the unstated
assumption that Shiro's annotations are wired up, having just demonstrated with
`@DePermit` that an authorization-looking annotation can do nothing. ADR 0015 already
answers that shape for Spring, and §3 applies the same treatment rather than
inventing a new one.

## Context

[ADR 0022](0022-annotation-identity-and-unrecognized-authorization.md) §2 created a
state between "guarded" and "bare": an access-control annotation that Sphinxor finds
and cannot interpret. It reaches that state only through one door — an annotation
whose **simple name collides with a Spring one** (`@Secured`, `@PreAuthorize`,
`@RolesAllowed`) but whose import does not.

Apache Shiro's `@RequiresPermissions` collides with nothing. It never enters the
recognized set, so it is not "recognized under the wrong binding" — it is never
looked at. It falls straight through to *no access control found*, which ADR 0022 §3
established is the wrong thing to report about an endpoint that has some.

nacos was caught because its annotation happened to be spelled like Spring's. That is
not a principle; it is a coincidence in the defect's shape.

### What that costs, measured

Scanned across the 20-repository Java corpus (the 14-project Spring survey plus
dataease, halo, hertzbeat, inlong, litemall, metersphere), counting **mutating**
routes whose annotation block carries an annotation bound by import to
`org.apache.shiro.authz.annotation`:

| Repository | mutating routes | …carrying a Shiro annotation |
|---|---:|---:|
| `metersphere/metersphere` | 639 | **555** |
| `apache/streampark` | 227 | **101** |
| `jeecgboot/JeecgBoot` | 274 | **80** |
| `apache/shenyu` | 206 | **66** |
| `linlinjava/litemall` | 104 | **65** |
| `apache/inlong` | 117 | **40** |
| **Total** | | **907** |

Every one of those is currently reported as having no access control.

Within the 14-repository Spring corpus specifically, **247** mutating routes carry a
Shiro annotation. Of those, 181 (JeecgBoot 80, streampark 101) sit on endpoints the
extractor can currently see, so they map to reported
`mutating-endpoint-without-access-control` findings out of the corpus's 918 — **around
a fifth of all current findings in the corpus are accusations against code carrying a
readable authorization annotation.** The remaining 66 are shenyu's, currently hidden
inside `@RestApi` controllers extraction does not recognize (`docs/limitations.md`),
and would surface the moment that gap closes.

That last point is the sequencing argument, and it is why this decision comes before
the route shapes: fixing shenyu's `@RestApi` first would surface 179 routes whose
only protection is Shiro, generating ~101 new findings against protected code —
re-creating on shenyu exactly the defect ADR 0022 removed from nacos, through a
different door.

### This does not widen the Spring Security scope

The objection to anticipate, because it is the first thing a reader will think:
*Sphinxor supports Spring Security; Shiro is a different framework; recognizing it is
scope creep.*

It is not, because nothing here interprets Shiro. `@RequiresPermissions("system:user:edit")`
carries a permission string, and this decision does **not** read it, does not model
it, and does not report it — for the same reason nacos's `resource`/`action` pairs go
unread: the model has no field for a permission (*Permissions as metadata*,
`docs/limitations.md`). What is established is narrower and purely local: **an
authorization annotation is present on this endpoint, and Sphinxor does not
understand it.**

"Shiro is unsupported" remains true after this decision. "There is no access control
here" was never true, and that is the only claim being withdrawn.

## Decision

### §1 Recognition is by import package, not by annotation name

An annotation on a controller class or handler method whose import binds it to a
package in the **known authorization-annotation packages** list is recorded as an
`UnrecognizedAuthAnnotation` (ADR 0022 §2) against that endpoint.

| Package | Framework |
|---|---|
| `org.apache.shiro.authz.annotation` | Apache Shiro |

Binding is resolved by ADR 0022 §1's existing per-file `importTable`, and requires a
**single-type import**. The on-demand form is deliberately not honored — a change
from this section as first accepted, made during implementation because a test
proved the original text wrong. It is recorded here rather than quietly applied:

> A wildcard says a package is in scope; it does not say which simple names come
> from it. ADR 0022 can honor one because its caller has already narrowed the
> question to three known names — *"is this `@Secured` Spring's?"* is answerable
> from a wildcard. Here the rule is package membership itself, asked of **every**
> annotation in the file, so honoring a wildcard binds every otherwise-unimported
> name to that package. The first implementation did exactly that and recorded
> `@RestController` and `@PostMapping` as Shiro authorization annotations — the
> over-capture this ADR rejects the name heuristic for, reached from the other side.

Resolving it soundly would require a list of Shiro's annotation names, which is
precisely what §1 exists to avoid. The measured trade is lopsided: **274 of the
corpus's 275 Shiro imports are single-type**, and the one on-demand file (litemall's
`AdminIndexController`) contains a single mutating route. One missed route in 20
repositories, in the safe direction — it keeps the finding it has today — against a
name list and an unsound rule.

**By package rather than by name, deliberately.** The package *is* Shiro's
authorization-annotation package; every annotation in it is one. Enumerating names
would mean asserting a list — `RequiresPermissions`, `RequiresRoles`,
`RequiresAuthentication`, `RequiresUser`, `RequiresGuest` — of which the corpus
exercises only the first three, and an incomplete list would silently miss the rest.
The package is one verified fact instead of five partly-unverifiable ones. It also
follows ADR 0022 §1's discipline exactly: identity comes from the import, never from
the spelling.

Requiring the binding, rather than accepting the name alone, matters for the same
reason it did in ADR 0022: a project-local annotation that happens to be called
`@RequiresPermissions` would otherwise be assumed to be Shiro's. Those fall into the
residue below.

### §2 Behaviour is ADR 0022's, unchanged

A Shiro annotation produces exactly what nacos's `@Secured` produces today: an
`UnrecognizedAuthAnnotation`, no `GuardApplication`, no role, `?` in the Guards
column, `mutating-endpoint-without-access-control` suppressed for that endpoint
(§3), and a project-level warning naming the bound package and the endpoint count
(§3a). No new model concept, no new rule, no new output shape — this decision widens
what reaches an existing state.

The accepted risk is ADR 0022 §3's, unchanged and now larger: a Shiro annotation that
is declared but not enforced (no `ShiroFilterFactoryBean`, no realm wired) would go
unflagged. It is the same lopsided trade — a false negative on a misconfigured
project against a false positive on 907 correctly configured routes — and the warning
names the package so a reader can check.

### §3 Whether Shiro's annotations are switched on is located and reported

Shiro annotations do nothing at runtime unless the framework's annotation support is
wired in — `AuthorizationAttributeSourceAdvisor`, the exact counterpart of Spring's
`@EnableMethodSecurity`. §2 suppresses a finding on 907 routes; this section bounds
what that suppression rests on.

This is [ADR 0015](0015-inert-method-security-guard.md)'s treatment, applied
unchanged rather than reinvented. The annotations are **not** downgraded when the
wiring is not found — absence is not evidence, for the same reasons ADR 0015 gives
and one more that is specific here: Shiro's `shiro-spring-boot-web-starter` enables
annotation support by auto-configuration, with no Java bean to find at all, and a
build file is not something this extractor parses. Guessing "inert" would invent
findings; the run says what it found instead.

So a project carrying Shiro annotations is scanned for
`AuthorizationAttributeSourceAdvisor`, and §3a's warning states whether it was
located. Found: the suppression rests on wiring that is present in the analyzed
source. Not found: the annotations may be inert, in which case those endpoints are
not protected and the suppressed findings were real — and the reader is told exactly
that, with the same "absence of evidence is not evidence" caveat ADR 0015 carries.

**Measured**: all six repositories in the corpus that use Shiro annotations declare
`AuthorizationAttributeSourceAdvisor` in Java source (8 files across the corpus,
`org.apache.shiro.spring.security.interceptor`). So in every case measured, the
check confirms the suppression rather than qualifying it — which is the outcome that
makes the suppression defensible, and a fact that could only be established by
looking.

### §4 The list grows on measured evidence, one framework at a time

A package joins the table when it has been seen in a real repository, not because it
is popular or plausible. Sa-Token (`cn.dev33.satoken.annotation`) is the obvious next
candidate and is **deliberately excluded**: it has zero occurrences across the 20
repositories scanned. Adding it now would be building ahead of evidence, which this
project declines elsewhere for the same reason.

## Alternatives considered

- **Match on annotation name with a pattern** — anything matching
  `Requires|Permission|Permit|Role|Auth|Secur|Access|Grant`. **Rejected, and the
  measurement is decisive.** Against the corpus it captures 1,615 mutating routes to
  the package rule's 907, but the excess is not gain:
  - **`@IgnoreAuth`** (JeecgBoot, `org.jeecg.config.shiro`, 7 mutating routes) is a
    marker that *skips* authentication. A rule matching "Auth" would suppress
    `mutating-endpoint-without-access-control` on endpoints explicitly declared
    public — inverting the finding on exactly the routes where it is correct.
  - **`@RequiresPermissionsDesc`** (litemall) declares `menu()` and `button()`: UI
    metadata for the admin console, not enforcement. It matches the pattern
    perfectly.
  - 576 of the excess is `@PreAuthorize` and `@Secured`, already handled correctly as
    Spring guards and by ADR 0022 respectively — double-counting, not coverage.
  - And it still **under**-matches: metersphere's `@CheckOwner` (515 uses) contains no
    matching substring, so the pattern misses the single largest project-local
    authorization annotation in the corpus.

  A heuristic that suppresses correct findings on public endpoints, fires on
  documentation, and misses its biggest target is worse than the narrower rule on
  every axis.
- **Record any unrecognized annotation on a handler as possible access control.**
  Rejected outright. The corpus census shows the annotations actually sitting on
  handlers are overwhelmingly OpenAPI (`@Operation` 2,019, `@ApiOperation` 484),
  logging (`@Log`, `@OperationLog`, `@AutoLog`, `@ApolloAuditLog`), Lombok, and rate
  limiting (`@TpsControl`). This would suppress essentially every finding in the
  corpus.
- **Read what Shiro requires** — extract `"system:user:edit"` into the model.
  Rejected as out of scope here for the reason given in *Permissions as metadata*:
  there is nowhere in ADR 0002's model to put a permission. That is the open model
  question, and this decision deliberately does not touch it.

## Consequences

- One package list and one call site. No model change: `UnrecognizedAuthAnnotation`
  already exists and already has the right shape and semantics.
- **Project-local authorization annotations remain a stated residue**, and it is
  substantial. What each one is was checked rather than assumed, and they did not all
  come back the same:
  - metersphere's **`@CheckOwner`** (515 uses, `io.metersphere.system.security`) —
    enforcement, confirmed: read by `CheckOwnerAspect`, `CheckProjectOwnerAspect` and
    `CheckOrgOwnerAspect`.
  - streampark's **`@Permission`** (78, `…core.annotation`) — enforcement, confirmed:
    read by `PermissionAspect`.
  - **`@AuthAction`** (28) — from the Sentinel dashboard vendored inside JeecgBoot;
    its reader lives in the dependency, so enforcement could not be confirmed from
    source either way.
  - dataease's **`@DePermit`** (97, `io.dataease.auth`) — **no reader anywhere in the
    analyzed source**, only its own declaration. It may be enforced outside the tree,
    or it may be dead. Nothing in the source says.

  No package list can enumerate any of these, so their endpoints keep their current
  findings — the status quo, not a regression — and the residue is recorded in
  `docs/limitations.md` rather than guessed at.

  `@DePermit` is worth keeping in view: it is an authorization-*looking* annotation
  with no demonstrable enforcement, which is a third way the rejected name heuristic
  would have been wrong. Matching on spelling would have suppressed 97 findings on
  the strength of a word, with no more evidence of protection than the word itself —
  the same mistake as reading nacos's `@Secured` as Spring's, arrived at from the
  opposite direction.
- Shiro's URL layer (`ShiroFilterFactoryBean`, seen in shenyu and streampark) stays
  entirely unread, as does everything Shiro requires. This decision changes one
  claim: that these endpoints have no access control.
### Validation — measured after implementation

**845 `mutating-endpoint-without-access-control` findings removed across the 20
repositories, every one on an endpoint carrying a readable authorization
annotation**:

| Repository | findings before | after | removed |
|---|---:|---:|---:|
| `metersphere/metersphere` | 639 | 84 | **555** |
| `apache/streampark` | 227 | 126 | **101** |
| `jeecgboot/JeecgBoot` | 251 | 170 | **81** |
| `linlinjava/litemall` | 104 | 39 | **65** |
| `apache/inlong` | 117 | 74 | **43** |
| `apache/shenyu` | 87 | 87 | **0** |

**shenyu's zero turns this ADR's sequencing argument from a prediction into a
measurement.** The Context above argued that fixing shenyu's `@RestApi` route shape
first would surface 179 routes whose only protection is Shiro and generate ~101
findings against protected code. The run confirms the premise directly: all 100 of
shenyu's Shiro annotations sit inside `@RestApi` controllers extraction does not
recognize, so **no visible endpoint carries one** and the project gains exactly
nothing here. Those 101 findings were avoided by doing this decision first, and they
are what the other order would have produced.

Every other repository is unchanged in every respect: endpoint counts, findings of
every rule, and the vendored `Pharmacy`, `blog-api`, `ruoyi-vue-pro` and `tutorials`
fixtures are byte-identical. dataease, halo and hertzbeat — no Shiro — do not move.

The §3a warnings name the bound package and the count (JeecgBoot: 104 endpoints on
`@RequiresPermissions`, 8 on `@RequiresRoles`; streampark: 102), and §3 reports
Shiro's wiring as **located** in every case, which is what makes the suppression
defensible rather than assumed.

The 845 is against the 907 source-level routes this ADR predicted. The difference is
shenyu's 66 invisible ones and a handful already suppressed by another guard; the
figures above are what the tool actually does, and supersede the prediction.

- **Original validation bar**, against the real corpus per `docs/testing.md`:
  - The three 14-corpus repositories drop the predicted findings and nothing else.
  - **No repository without a Shiro import changes in any respect** — in particular
    thingsboard, nacos, apollo, RuoYi-Vue and eladmin, and both vendored fixtures.
  - A test that an annotation bound to the Shiro package becomes an
    `UnrecognizedAuthAnnotation` while a project-local annotation of the *same simple
    name* does not, in one file — the ADR 0022 §1 discipline, re-verified here
    because this decision is the first to apply it to a name Spring does not use.
  - A test that the on-demand import form binds **nothing**, and specifically that
    no ordinary Spring annotation is swept up by one — the bug that reversed §1's
    original clause, pinned so it cannot return.
  - A test that `@IgnoreAuth` — an annotation in a package named `...config.shiro`
    that is *not* `org.apache.shiro.authz.annotation` — does **not** suppress, which
    is the rejected alternative's failure mode pinned as a regression.
  - §3 both ways: a project with Shiro annotations and an
    `AuthorizationAttributeSourceAdvisor` bean reports the wiring as located; the
    same project without one reports that it was not, without changing which
    findings are suppressed.
