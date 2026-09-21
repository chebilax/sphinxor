# 0024. A controller declared by a meta-annotation is a controller

## Status

Proposed.

## Context

[ADR 0011](0011-spring-second-framework.md) §1 recognizes a controller by the literal
presence of `@RestController` or `@Controller` on the class. Spring does not require
that: an annotation *annotated with* `@RestController` composes it, and a class
carrying the composed annotation is a controller as far as Spring is concerned.

`apache/shenyu` declares one:

```java
@Target(ElementType.TYPE)
@Retention(RetentionPolicy.RUNTIME)
@Validated
@RestController
@RequestMapping
public @interface RestApi {
    @AliasFor(attribute = "path", annotation = RequestMapping.class)
    String[] value() default {};
}
```

`@RestApi("/plugin-handle")` is `@RestController` + `@RequestMapping(path = "/plugin-handle")`.
It sits on **35 of shenyu-admin's 41 controller classes**, hiding **179 route
declarations**. What a run reports for shenyu instead is 192 endpoints, of which 155
come from `shenyu-examples/` demo applications, 26 from `shenyu-web`, and **11 from
the real admin API** — and nothing in the output suggests the admin API is missing.
That silence is the failure class [ADR 0020](0020-unanalyzable-is-unknown-not-absent.md)
exists to eliminate, reached through endpoint discovery: ADR 0019 §2's "recognized no
endpoints" notice cannot fire, because 192 is not zero.

### The sequencing paid out, and it is worth recording as a measurement

This fix was deliberately ordered **after** [ADR 0023](0023-third-party-authorization-annotations.md),
on the argument that surfacing 179 routes whose only protection is Apache Shiro would
generate ~101 findings against protected code — re-creating on shenyu the defect
ADR 0022 had just removed from nacos.

Measured now that ADR 0023 is in place:

| | |
|---|---:|
| Route declarations hidden by `@RestApi` | 179 |
| …of which mutating | 101 |
| …carrying a Shiro annotation, suppressed by ADR 0023 | **66** |
| …carrying nothing, becoming new findings | **35** |

**101 → 35.** Two-thirds of the cost was absorbed by doing the other decision first.
This is the second time in this sequence that ordering by measurement beat ordering
by intuition — the first being ADR 0023 itself, whose shenyu result turned its own
sequencing prediction into a measurement.

### What the 35 are, stated plainly here rather than in a footnote

They are **not** 35 endpoints established to be unguarded. shenyu-admin configures a
Shiro URL layer — `ShiroConfiguration`, with a `ShiroFilterFactoryBean` and a filter
chain definition map — and this extractor does not parse it (`docs/limitations.md`).
Some of those 35 are plausibly covered by a path rule there.

The honest statement is: **35 endpoints surfaced with no method-level access control,
in a project whose URL layer is unread.** That places them in exactly the same
category as the `SecurityFilterChain` blind spot — Low confidence, safe direction,
and the tool cannot conclude. Recording it now matters because in six months
"35 unguarded endpoints in shenyu" would otherwise read as an established fact.

### How common this is — the whole corpus, not one project

Every `@interface` declaration across the 20-repository Java corpus was classified by
what it composes. **Exactly one composes `@RestController`: shenyu's `@RestApi`.**

Meta-annotations composing a *mapping* annotation exist, and both families are
instructive because neither would gain a single endpoint:

- **`eladmin`'s `@AnonymousGetMapping` and siblings** (5 declarations, 11 uses,
  `@Target(METHOD)`) compose `@RequestMapping(method = RequestMethod.GET)`. Resolving
  them yields a shape ADR 0011 §1 **already declines to read** — the
  `@RequestMapping(method = …)` scope cut — so they resolve to nothing regardless.
- **`shenyu`'s `@ShenyuGetMapping` and siblings** are a genuine **two-level** chain:
  `@ShenyuGetMapping` → `@ShenyuRequestMapping(method = {RequestMethod.GET})` →
  `@RequestMapping`. They live in `shenyu-client` and `shenyu-examples` — the client
  SDK and demo applications, not the admin API — and bottom out at `@RequestMapping`,
  which is again unread.

So the corpus contains a multi-level chain, and following it would surface zero
endpoints. That is the evidence §1's depth limit rests on, rather than a preference
for simplicity.

## Decision

### §1 One level, class-level, and only for a meta-annotation that composes a controller

A class annotated with a project-declared annotation that is itself annotated
`@RestController` or `@Controller` is a controller, and is extracted exactly as if
the composed annotations were written on the class directly.

Three bounds, each measured rather than assumed:

- **One level of indirection.** A meta-annotation composing *another* meta-annotation
  that composes `@RestController` is not resolved. This matches
  [ADR 0006](0006-composite-decorator-resolution.md)'s one-level choice for NestJS
  composites, and here the evidence is stronger than symmetry: the corpus's only
  multi-level chains are shenyu's `@Shenyu*Mapping`, which bottom out at
  `@RequestMapping` and would surface nothing at any depth.
- **`@Target(TYPE)` only — the class level.** Method-level mapping meta-annotations
  are out of scope because every one measured resolves to
  `@RequestMapping(method = …)`, which ADR 0011 §1 does not read. Adding them would
  be machinery with a measured yield of zero endpoints. When that scope cut is
  revisited, this one should be revisited with it; they are the same blocker.
- **The meta-annotation's declaration must be in the analyzed source.** shenyu's is.
  One imported from a shared library cannot be resolved at all — there is nothing to
  read — and such a class stays unrecognized, which is the same shape as apollo's
  interface-declared routes and belongs to that decision, not this one.

### §2 What is read from the composition, and what is ignored

Only two things are taken from a resolved meta-annotation:

1. **That the class is a controller** — from `@RestController`/`@Controller` on the
   declaration. No attribute of theirs is read; neither carries one that matters
   here.
2. **The controller's base path** — from `@RequestMapping`, resolved in this order:
   - the `path`/`value` argument **on the declaration itself**, when it is a string
     literal (`@RequestMapping("/base")` written on the `@interface`);
   - otherwise, the argument **at the use site**, routed through `@AliasFor` per §3.

Everything else the meta-annotation composes is **ignored**, deliberately and by
name: `@Validated`, `@Inherited`, `@Documented`, and any other annotation it carries.
shenyu's `@RestApi` composes `@Validated`, which has no bearing on routing or
authorization. Resolving a meta-annotation is not an invitation to interpret
everything inside it.

**Security annotations composed by a meta-annotation are explicitly not resolved
here.** No repository in the corpus wraps `@PreAuthorize` in a meta-annotation — a
negative result specifically looked for across 20 repositories — so there is nothing
to validate such a rule against, and ADR 0011 §1 already lists composed
method-security annotations as out of scope. If one is ever measured, it is a
separate decision.

### §3 `@AliasFor`, bounded to the one question being asked

`@AliasFor` is read only to answer "which attribute of this meta-annotation carries
`@RequestMapping`'s path?". Two forms are recognized, both of which appear in the
corpus:

- **Explicit**: `@AliasFor(attribute = "path", annotation = RequestMapping.class)` —
  shenyu's form.
- **Implicit same-name**: `@AliasFor(annotation = RequestMapping.class)` on an
  attribute named `path` or `value`, which aliases the attribute of the same name.

An `@AliasFor` with no `annotation =` element declares an alias *within* the same
annotation (shenyu's `@ShenyuGetMapping` pairs `value` and `path` this way). It says
nothing about `@RequestMapping` and is not followed.

The use-site argument is then bound the ordinary Java way: a single positional
argument binds to `value`, and `name = …` binds to `name`.

If no attribute aliases `@RequestMapping`'s path and the declaration's
`@RequestMapping` carries no literal path, the controller has **no base path** —
which is a fact about the source, not a failure, and is recorded as the empty prefix
exactly as a `@RestController` with no `@RequestMapping` already is.

### §4 A base path that cannot be read is unknown, not empty

If the use-site argument exists but is not a string literal — a constant reference,
a concatenation, an array of them — the base path is **unresolved**, and the endpoint
takes [ADR 0020](0020-unanalyzable-is-unknown-not-absent.md) Amendment 1 §5's existing
treatment: identity synthesized from controller and handler, the path marked `…`, the
run warning and naming the controller, and `sphinxor export cerbos` omitting it.

This is stated now rather than discovered later. All 35 of shenyu's uses carry a
string literal, so nothing in the corpus exercises it — which is exactly why the
behaviour is specified here instead of being left to whatever the code happens to do
on the first project that does it differently. It is also free: §5's machinery
already exists and already does the right thing for a literal `@RequestMapping` whose
argument cannot be read.

## Alternatives considered

- **Follow meta-annotations to any depth.** Rejected on measurement, not principle.
  The corpus's only multi-level chains resolve to `@RequestMapping`, which is unread,
  so unbounded depth surfaces zero additional endpoints while adding cycle detection
  and a resolution order to get wrong.
- **Treat any class-level annotation whose name ends in `Api`/`Controller`/`Resource`
  as a controller.** Rejected for the reason [ADR 0023](0023-third-party-authorization-annotations.md)
  rejected its name heuristic: spelling is not identity. The corpus census found
  **115** project-declared `@Target(TYPE)` annotations — `@TbCoreComponent`,
  `@ArdApi`, `@Finder`, `@ConditionalOnProfile` and the like — of which exactly one
  is a controller. A name rule would invent controllers out of the rest.
- **Resolve the meta-annotation's composed security annotations too.** Rejected as
  unmeasurable: zero instances in 20 repositories, so nothing to validate against.
- **Skip `@AliasFor` and take the meta-annotation's sole string argument as the base
  path.** Tempting, and it would work on shenyu. Rejected because it guesses: an
  annotation's first string argument is not necessarily a path, and the information
  needed to know is written in the source one line above. Reading `@AliasFor` is
  cheap and correct; assuming is cheap and sometimes wrong.

## Consequences

- shenyu's reported endpoint count rises from 192 to roughly 371, and the admin API
  stops being invisible — the substantive point, independent of the finding count.
- **35 new `mutating-endpoint-without-access-control` findings on shenyu**, Low
  confidence, in a project whose Shiro URL layer is unread. See the framing above;
  these are surfaced-and-uncertain, not established.
- `docs/limitations.md`'s route-shapes entry loses its first bullet and gains a note
  that the remaining shapes (interface-inherited routes, `@RequestMapping(method=)`,
  JAX-RS, method-level mapping meta-annotations) are unchanged.
- One new resolution step in controller extraction, reading annotation declarations
  that are already parsed. No model change, no new entity, no new output shape.
- **Validation before this is Accepted**, against the real corpus per `docs/testing.md`:
  - shenyu: 192 → ~371 endpoints, the 35 `@RestApi` controllers' routes present with
    their real paths, their 100 Shiro annotations recorded as unrecognized
    authorization annotations (ADR 0023) rather than as nothing, and exactly 35 new
    mutating findings.
  - **No other repository in the 20-project corpus changes in any respect**, since
    none declares a controller-composing meta-annotation.
  - All four vendored fixtures byte-identical.
  - A test that a one-level controller meta-annotation resolves, including its
    `@AliasFor`-routed base path, and that a **two-level** one does not — the depth
    bound pinned, since the corpus cannot exercise it on a recognized mapping.
  - A test that an unreadable use-site path takes §4's unresolved treatment, since
    no corpus project exercises that branch either.
  - A test that an ordinary `@Target(TYPE)` project annotation composing no
    controller — `@ConditionalOnProperty`-style — does not make its class a
    controller.
