# 0029. Sphinxor's Spring support targets Spring Security, and here is the checklist

## Status

Accepted.

## Context

[ADR 0011](0011-spring-second-framework.md) added "Spring" as the second framework
and scoped §1 to Spring Security's constructs, but it never said so as a decision.
Everything since has assumed it — [ADR 0022](0022-annotation-identity-and-unrecognized-authorization.md)'s
import-based identity is only meaningful if "is this Spring Security's annotation?"
is the question being asked, and [ADR 0023](0023-third-party-authorization-annotations.md)
and [ADR 0027](0027-unannounced-url-layers.md) both had to argue, separately, that
detecting Apache Shiro is not the same as supporting it.

Seven decisions have now been made against a boundary that exists only by
implication. This states it, and turns the current state into a list a reader can
check rather than a claim they have to take on trust.

It also answers a question that has no answer today: **what would finishing look
like?**

## Decision

### §1 The target is Spring Security, not authorization in Spring applications

Sphinxor's Spring extractor analyzes **Spring Security**. It does not analyze
authorization in Spring applications generally.

The difference is not pedantic — it is the difference between a gap and a
non-goal. Apache Shiro, imperative in-handler checks
(`context.authenticatedUser().validateHasPermissionTo("READ_LOAN")`), JAX-RS routing
and runtime path prefixes applied by a `WebMvcRegistrations` bean are **other
systems**. Sphinxor recording nothing about what they require is correct behaviour,
not a missing feature, and `docs/limitations.md` records each because a reader needs
to know the report is silent about them — not because they are owed an
implementation.

**Detecting that another system is present is not interpreting it.** ADR 0023 records
a Shiro annotation as found-and-not-understood; ADR 0027 announces a
`ShiroFilterFactoryBean`. Neither reads what Shiro requires, and neither is a step
toward doing so. The claim withdrawn in both cases was never "we understand this" —
it was "there is nothing here".

### §2 The mechanism list

Every Spring Security mechanism, with exactly one status:

- **read** — extracted into the model and reflected in the matrix.
- **detected and announced** — present, not interpreted, and the run says so.
- **silent** — present, not interpreted, and nothing says so. This is the only
  status that is a defect.
- **out of scope** — not Spring Security; see §1.

#### Method security

| Mechanism | Status | Decision |
|---|---|---|
| `@PreAuthorize` / `@Secured` / `@RolesAllowed`, identified by import | read | [0011](0011-spring-second-framework.md) §1, [0022](0022-annotation-identity-and-unrecognized-authorization.md) §1 |
| …written fully qualified | read | [0025](0025-qualified-annotation-names.md) §1/§3 |
| SpEL `hasRole` / `hasAnyRole` / `hasAuthority` / `hasAnyAuthority` | read | [0011](0011-spring-second-framework.md) §1 |
| SpEL `isAuthenticated()` | read | [0010](0010-authenticated-any-role.md), [0017](0017-declaresroles-excludes-isauthenticated.md) |
| SpEL `permitAll()` / `denyAll()` | read (as no role list) | [0017](0017-declaresroles-excludes-isauthenticated.md), [0020](0020-unanalyzable-is-unknown-not-absent.md) Am3 §10 |
| SpEL outside that subset — bean calls, boolean combinations, `#param` comparisons | **detected and announced** (`?` in Roles, warning names the count) | [0020](0020-unanalyzable-is-unknown-not-absent.md) Am3 §9/§11 |
| A same-named annotation from another package | detected and announced | [0022](0022-annotation-identity-and-unrecognized-authorization.md) §2/§3a |
| `@EnableMethodSecurity` / `@EnableGlobalMethodSecurity`, and its absence | read; absence announced | [0015](0015-inert-method-security-guard.md), [0020](0020-unanalyzable-is-unknown-not-absent.md) §4 |
| `@EnableReactiveMethodSecurity` | read | [0015](0015-inert-method-security-guard.md) Am1 |
| `@PostAuthorize` | detected and announced (`?`, and the finding still fires on a write) | [0030](0030-post-authorize-and-method-security-filters.md) §1/§2 |
| `@PreFilter` / `@PostFilter` | detected and announced (project-wide; they authorize nothing) | [0030](0030-post-authorize-and-method-security-filters.md) §4 |

#### The URL layer

| Mechanism | Status | Decision |
|---|---|---|
| One `SecurityFilterChain` with readable Ant patterns, AND-combined with the method layer | read | [0012](0012-securityfilterchain-effective-policy.md) |
| More than one `SecurityFilterChain` bean | detected and announced | [0020](0020-unanalyzable-is-unknown-not-absent.md) §2 |
| Reactive `SecurityWebFilterChain` | detected and announced | [0020](0020-unanalyzable-is-unknown-not-absent.md) §3 |
| `WebSecurityConfigurerAdapter` (pre-5.7) | detected and announced | [0027](0027-unannounced-url-layers.md) §1 |
| `.access(...)` custom `AuthorizationManager` | detected (rule-local: stops evaluation, grants nothing) | [0018](0018-unrecognized-rule-stops-evaluation.md) |
| A matcher whose pattern cannot be read (regex, custom bean, `mvc.matcher`) | detected (rule-local, as above) | [0020](0020-unanalyzable-is-unknown-not-absent.md) §1 |
| `RoleHierarchy` | detected and announced (roles shown are narrower) | [0031](0031-role-hierarchy.md) §1/§2 |

#### Endpoint discovery

| Mechanism | Status | Decision |
|---|---|---|
| `@RestController` / `@Controller`, literal or fully qualified | read | [0011](0011-spring-second-framework.md) §1, [0025](0025-qualified-annotation-names.md) |
| A meta-annotation composing a controller, one level | read | [0024](0024-controller-meta-annotations.md) §1 |
| `@GetMapping` and siblings | read | [0011](0011-spring-second-framework.md) §1 |
| `@RequestMapping(method = …)`, including multi-verb | read | [0026](0026-requestmapping-method-attribute.md) §1 |
| A verb-less method-level `@RequestMapping` | read (as `ANY`) | [0028](0028-verbless-request-mapping.md) §1 |
| A route path or version that cannot be read | detected and announced | [0020](0020-unanalyzable-is-unknown-not-absent.md) Am1 §5, Am2 §7 |
| One route declared by two controllers | detected and announced | [0020](0020-unanalyzable-is-unknown-not-absent.md) Am2 §8 |
| A **method-level** mapping meta-annotation (`@AnonymousGetMapping`) | **silent** | [0024](0024-controller-meta-annotations.md) §1 excludes it |
| Routes declared on an **inherited interface** | **silent** | recorded in `docs/limitations.md`, no decision |
| A **nested** `@RestController` | **silent** | recorded in `docs/limitations.md`, no decision |
| Functional routing (`RouterFunction`) | **silent** | recorded in `docs/limitations.md`, no decision |

#### Out of scope — other systems (§1)

Apache Shiro annotations ([0023](0023-third-party-authorization-annotations.md)) and
its `ShiroFilterFactoryBean` ([0027](0027-unannounced-url-layers.md)) are *detected
and announced*, which §1 distinguishes from supported. Imperative in-handler checks,
JAX-RS routing, runtime path prefixes, authorization declared in YAML, and
project-local authorization annotations are neither read nor detected, and a zero on
them is correct.

Kotlin source is also out of scope, per [ADR 0011](0011-spring-second-framework.md) §1 —
a language boundary rather than a framework one, and the only item here that would
be reached by parsing more rather than by deciding more.

### §3 What "Spring Security at 100%" means

**No mechanism in §2 has status `silent`.**

It deliberately does **not** mean every mechanism is *read*. An unreadable SpEL
expression whose permission the model cannot hold is `detected and announced`, and
that is a finished state under this definition: the run says what it could not
interpret, and the reader is not misled. Reading the permission *content* is the open
ADR 0002 model question recorded in `docs/limitations.md`, and it is deliberately not
part of this definition — otherwise "done" would depend on a decision nobody has
made.

By that definition, **three** items remain, and all three are endpoint discovery:

1. A method-level mapping meta-annotation.
2. Routes on an inherited interface, and a nested `@RestController`.
3. Functional routing — `RouterFunction`, the reactive counterpart of an annotated
   controller. It is Spring's own routing, not another system, so §1 does not put
   it out of scope the way it does JAX-RS.

   **Added by a check that expected the opposite answer.** ADR 0019 §2 warns when a
   run parses files and recognizes no endpoints, and on halo — which yields zero
   endpoints from 1,001 files — that warning does fire, which looks like
   `RouterFunction` being detected and announced. It is not. The condition is
   `len(m.Endpoints) == 0` across the whole project, so it announces *emptiness*,
   not a route shape it failed to read. Adding a single annotated controller to a
   `RouterFunction` project removes the warning and the functional routes vanish in
   silence.

   That is the real corpus case, not a hypothetical: of the three repositories using
   `RouterFunction`, **shenyu** (6 files, 394 recognized endpoints) and
   **JeecgBoot** (1 file, 931) get no warning at all. shenyu's are real routes —
   `POST /helloWorld2`, `GET /rewrite`, `GET /pdm`, `GET /oms`, `GET /timeout`.
   Only halo is announced, and only by accident of having nothing else.

`@EnableReactiveMethodSecurity` was on this list and was the one item where the tool
did not merely stay quiet but stated something false. [ADR 0015](0015-inert-method-security-guard.md)
Amendment 1 closed it; the row above now reads **read**. Item 5 replaced it, so the
count is unchanged — which is the checklist working, not failing.

The three method-security items that were on this list — the reactive enabler,
the `@PostAuthorize` group and `RoleHierarchy` — were named `silent` rather than
`out of scope`, which extended the list beyond the endpoint-discovery gaps.
All three are now closed, so what remains is exactly the endpoint-discovery set. The reasoning: ADR 0011 §1 called them
out of scope, and §1 above reserves that status for *other systems*. These are Spring
Security's own mechanisms, so not interpreting them is a gap, and not saying so is
the defect this project has spent seven decisions removing.

**A correction to this paragraph, made by the first amendment written against it.**
As accepted, it claimed all of these "measure zero occurrences across the
20-repository corpus", and offered that as the reason the urgency is low. Working
the reactive enabler falsified it. **halo** carries `@EnableReactiveMethodSecurity` in
`WebServerSecurityConfig`, and its only four `@RestController`s are nested classes,
which is item 2 above. The zero was a count of the *defect* — a project that both
enables method security reactively and annotates handlers — not a count of the
*mechanism*, and the paragraph read it as the second. The corpus figures per item
are now recorded by the amendment that measures them
([ADR 0015](0015-inert-method-security-guard.md) Am1 §4) rather than asserted here
in aggregate, because an aggregate claim over four unmeasured items is exactly the
kind of thing a checklist exists to stop.

The status of each item is unchanged by any of this: a checklist that omits what
nobody happened to use is not a checklist.

## Alternatives considered

- **Leave the boundary implicit.** Rejected: it has already been re-argued from
  scratch twice, in ADR 0023 and ADR 0027, and the next third-party mechanism would
  have made it three.
- **Define done as "every mechanism read".** Rejected per §3: it makes the
  definition depend on the unanswered ADR 0002 question, so it could never be
  reached and would therefore say nothing.
- **Treat `@PostAuthorize`, `RoleHierarchy` and the reactive enabler as out of
  scope**, as ADR 0011 §1 did. Rejected per §3 — they are Spring Security, and
  "out of scope" here means another system.

## Consequences

- No code change. This ADR is a statement and a checklist.
- `README.md` says "Spring Security" where it stated the **scope** as "Spring", in
  the same change: the architecture diagram's extractor node, the "what it does"
  capability line (which now also links here), and the layer-combining line. Its
  remaining bare "Spring" mentions describe a specific demo project and the vendored
  test corpus, which really are Spring projects, and are left alone.
- **`docs/vision.md` gains a scope section and keeps its existing wording.** All
  three of its "Spring" mentions were checked and none is a scope claim: one names
  the market category ("mainstream web frameworks (Spring, NestJS, Django, Symfony,
  ASP.NET)"), one is the v0.1 deliberation over which framework to start with
  ("Spring or NestJS are good candidates"), and one describes the benchmark corpus.
  Rewriting the second would falsify the record of a decision as it was actually
  made, which is the one thing `vision.md` and the ADR log exist to preserve.

  What it lacked was not corrected wording but a stated boundary, so it gains one —
  the promise, citing this ADR for the checklist. It also says plainly that **NestJS
  has no equivalent enumeration**, because a scope section covering one of two
  supported frameworks would otherwise imply both are documented to this standard.
- Five items get a status they did not have, and this changes the list before it
  changes any behaviour — which is the point of writing it down. (The corpus counts
  originally given alongside them were wrong; see §3.)
- Future decisions inherit a place to record their effect. An ADR that moves a
  mechanism from `silent` to `detected and announced` closes a line here, and one
  that adds a mechanism adds a line.
