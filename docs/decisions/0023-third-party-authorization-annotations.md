# 0023. A third-party authorization annotation is found-and-not-understood, not absent

## Status

Proposed.

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

Binding is resolved by ADR 0022 §1's existing per-file `importTable`, including the
on-demand form — `import org.apache.shiro.authz.annotation.*;` occurs once in the
corpus (litemall's `AdminIndexController`) and must bind, or that file's `@RequiresPermissions`
would be missed.

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

### §3 The list grows on measured evidence, one framework at a time

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
- **Validation before this is Accepted**, against the real corpus per `docs/testing.md`:
  - The three 14-corpus repositories drop the predicted findings and nothing else:
    JeecgBoot and streampark lose their Shiro-covered mutating findings, each gains
    the §3a warning naming `org.apache.shiro.authz.annotation`, and shenyu is
    unchanged in count (its Shiro annotations sit in controllers not yet recognized)
    while gaining the warning for the ones that are.
  - **No repository without a Shiro import changes in any respect** — in particular
    thingsboard, nacos, apollo, RuoYi-Vue and eladmin, and both vendored fixtures.
  - A test that an annotation bound to the Shiro package becomes an
    `UnrecognizedAuthAnnotation` while a project-local annotation of the *same simple
    name* does not, in one file — the ADR 0022 §1 discipline, re-verified here
    because this decision is the first to apply it to a name Spring does not use.
  - A test that the on-demand import form binds, since exactly one file in 20
    repositories relies on it and nothing else in the suite would catch its loss.
  - A test that `@IgnoreAuth` — an annotation in a package named `...config.shiro`
    that is *not* `org.apache.shiro.authz.annotation` — does **not** suppress, which
    is the rejected alternative's failure mode pinned as a regression.
