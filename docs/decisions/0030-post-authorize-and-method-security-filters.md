# 0030. `@PostAuthorize` protects a read, not a write — and `@PreFilter`/`@PostFilter` protect nothing

## Status

Accepted.

## Context

[ADR 0029](0029-spring-security-scope.md) §2 lists these as **silent**: Spring
Security's own mechanisms, present in a project, not interpreted, and nothing says
so. §3 makes clearing that status the definition of done, so they are to be
announced rather than read.

[ADR 0011](0011-spring-second-framework.md) §1 cut all three in one sentence:

> **`@PostAuthorize`, `@PreFilter`, `@PostFilter`** evaluate against a method's
> return value or filter a collection after/before invocation — object-level,
> ABAC-shaped checks with no RBAC equivalent in the current model […] Not modeled,
> not approximated; a documented non-goal.

That is right about the *model* and says nothing about what each one protects.
Announcing all three the same way would put a claim in the output that is false for
two of them, which is worse than the silence it replaces.

### The measurement

**Zero occurrences across all 20 corpus repositories.**

| Mechanism | Corpus uses |
|---|---|
| `@PostAuthorize` | 0 |
| `@PreFilter` | 0 |
| `@PostFilter` | 0 |

Against a positive control of **897 `@PreAuthorize`** and **542 `@Secured`** in the
same scan, so the method is not the reason for the zeros. A whole-tree search
ignoring file type surfaced 14 apparent hits, all false positives: nacos's GraalVM
`reflect-config.json` naming Spring's own `PostAuthorizeAuthorizationManager` and
`PreFilterAuthorizationMethodInterceptor` classes, `RuoYi-Vue`'s fastjson
`SimplePropertyPreFilter`, and shenyu's own `apiPostFilter`/`definitionPostFilter`
methods.

**This is a checklist item, not a response to observed harm.** ADR 0029 §3 already
made that case. What follows from the zero is a validation limit, recorded in Consequences.

### What each one does to a caller who fails it

Established by reading Spring Security's source, not by grouping:

| Annotation | Caller who fails it | Stops the method running? |
|---|---|---|
| `@PostAuthorize` | `AccessDeniedException` instead of the result | **no** — the method has already run |
| `@PreFilter` | call succeeds; input collection is filtered | no |
| `@PostFilter` | call succeeds; returned collection is filtered | no |

`AuthorizationManagerAfterMethodInterceptor.invoke()` is unambiguous:

```java
Object result;
try {
    result = mi.proceed();
}
catch (AuthorizationDeniedException ex) { … }
return attemptAuthorization(mi, result);
```

`mi.proceed()` — the method body — runs first. Authorization is checked against the
value it returned. The reactive interceptor,
`AuthorizationManagerAfterReactiveMethodInterceptor`, does the same:
`ReactiveMethodInvocationUtils.proceed(mi)` first, `postAuthorize` applied to the
resulting publisher.

Spring's own reference documentation states the consequence directly:

> Note that `@PostAuthorize` is not recommended for classes that perform database
> writes since that typically means that a database change was made before the
> security invariants were checked. A common example of doing this is if you have
> `@Transactional` and `@PostAuthorize` on the same method.

**Verified against Spring Security 6.5.11**, and the servlet interceptor's ordering
re-read on `7.1.1`/`main` in the same pass as
[ADR 0015](0015-inert-method-security-guard.md) Amendment 1 §2.1. This ordering is
a *positive* — an interceptor that proceeds before it authorizes — so unlike
Amendment 1's negatives it is not falsified by a release adding something. It would
take Spring reversing `@PostAuthorize`'s defining semantics, which would be a
breaking change to the annotation's purpose. Consequences records the version anyway.

## Decision

### §1 `@PostAuthorize` protects a read and does not protect a write

On a `GET`, `@PostAuthorize("hasRole('ADMIN')")` works: the data is fetched, the
check fails, the caller receives an exception rather than the data. The endpoint's
*effect* — disclosure — is prevented.

On a `POST`, `PUT`, `PATCH` or `DELETE`, the state change has **already happened**
when `AccessDeniedException` is thrown. The endpoint's effect is not prevented. The
annotation withholds the response body of a mutation that already occurred.

An enclosing transaction can roll it back, but only when
`@EnableTransactionManagement` is ordered ahead of `@EnableMethodSecurity` — Spring
documents this as something the developer must arrange deliberately, and **this
extractor cannot establish it**. Bean ordering, proxy advisor precedence and
`@Transactional` placement are exactly the runtime configuration ADR 0011 §1 keeps
out of scope. So the tool cannot claim the rollback, and must not assume it.

**Therefore protection is verb-dependent**, which no mechanism in the model has
been before.

### §2 The treatment, split by what the endpoint does

A `@PostAuthorize` bound to `org.springframework.security.access.prepost` (identity
per [ADR 0022](0022-annotation-identity-and-unrecognized-authorization.md) §1,
qualified spellings per [ADR 0025](0025-qualified-annotation-names.md)) is recorded
as an `UnrecognizedAuthAnnotation` on **every** endpoint carrying it. That gives it
[ADR 0023](0023-third-party-authorization-annotations.md)'s treatment throughout:
the Guards column shows `?`, the run warns naming the count, and the Cerbos export
omits the endpoint — all existing behaviours with existing consumers.

**One thing is withheld: it does not suppress
`mutating-endpoint-without-access-control` on a mutating endpoint.**

| Endpoint | Recorded | Guards column | `mutating-endpoint-without-access-control` |
|---|---|---|---|
| `GET`, `HEAD`, `OPTIONS`, `TRACE` | yes | `?` | rule does not apply to these verbs at all |
| `POST`, `PUT`, `PATCH`, `DELETE`, `ANY` | yes | `?` | **fires** |

Suppressing it would assert that nothing needs looking at, on an endpoint whose
mutation Spring does not stop. That is the same manufactured confidence §4 refuses
for the filters, and [ADR 0011](0011-spring-second-framework.md) §1 calls it worse
than a missed finding for an auditing tool.

**The carve-out is keyed on the binding, never the simple name.** Only
`org.springframework.security.access.prepost.PostAuthorize` loses the suppression.
A project-local or third-party `@PostAuthorize` keeps ADR 0022 §3's treatment
unchanged, because nothing is known about when *it* runs — assuming Spring's
evaluation order for someone else's annotation is the nacos fault
([ADR 0022](0022-annotation-identity-and-unrecognized-authorization.md) §1) in the
one change most obliged to avoid it. The first implementation of this section did
key on the name, and a test written for this ADR caught it.

Note what the first column does *not* buy on a read: the rule only examines mutating
verbs, so on a `GET` there is no finding to suppress and never was. The `?` marker,
the warning and the export omission are the whole of the treatment there. The split
is therefore narrower in code than it reads — it is one carve-out in one rule's
suppression set — but it is the difference between the report saying "protected"
and "protected against disclosure, not against the write".

### §3 The finding says why it still fires

A reader who sees `mutating-endpoint-without-access-control` on a handler with an
authorization annotation directly above it will assume the tool missed it. The
message must pre-empt that, so when the endpoint carries a `@PostAuthorize` it
reads:

> `POST /orders/{id}` has a `@PostAuthorize` but no guard that runs before the
> method. `@PostAuthorize` is evaluated after the handler executes, so the state
> change has already happened when access is denied — unless an enclosing
> transaction rolls it back, which is not visible here.

rather than the default `has no detected guard or role decorator`, whose premise is
false on its face here. This is the same reasoning ADR 0022 §3 used to introduce
the suppression in the first place: a message whose premise the reader can see is
wrong discredits the rule.

**When the annotation is confirmed inert** — `@EnableGlobalMethodSecurity` leaving
`prePostEnabled` at its `false` default, per
[ADR 0015](0015-inert-method-security-guard.md) — the endpoint gets the **ordinary**
message, not this one. Nothing evaluates the annotation at all, so describing when
it evaluates would be the false statement Amendment 1 was written to remove.

### §4 `@PreFilter`/`@PostFilter` are announced project-wide and change nothing

Counted and reported in a project-level warning naming the count and the classes.
**Not** recorded per endpoint, **not** added to `UnrecognizedAuthAnnotations`, and
they suppress nothing.

Neither ever denies a call. A caller with no matching authority invokes the method
successfully and receives a shorter collection. They are a data concern. Recording
them per endpoint would mark the Guards column `?` on an endpoint nothing guards,
and would suppress a finding on a mutation nothing stops.

### §5 `UnrecognizedAuthAnnotation` keeps its name and gains an honest comment

The type is reused rather than replaced. Its name reads as "we could not identify
this", which was true of the nacos collision that produced it, but ADR 0023 already
broadened its use to Shiro's annotations and this ADR extends it to Spring's own.
The doc comment is corrected to say what the type actually means — **authorization
present but not interpreted**, whatever its provenance — with the three cases named.

A new per-endpoint type was rejected: ADR 0011 §1 records what it costs when one
consumer is not taught a new state, and that hazard is worth more than a better
name. Renaming would touch ADR 0022's and 0023's vocabulary and is deliberately not
bundled here.

## Alternatives considered

- **Treat `@PostAuthorize` as protection everywhere**, as ADR 0023 treats a Shiro
  annotation. Rejected per §1: on a mutating endpoint Spring does not stop the
  mutation, so the suppression would claim protection that does not exist. This was
  the first draft of this ADR, and it was wrong.
- **Treat `@PostAuthorize` as protection nowhere.** Rejected: on a read it genuinely
  prevents disclosure, and flagging it would be a false positive in the other
  direction. The rule does not examine reads anyway, so this would only cost the
  `?` marker and the export omission — accuracy given up for uniformity.
- **Announce all three annotations identically**, as ADR 0011 §1 groups them.
  Rejected: the grouping is about evaluation time, and the output needs to say what
  is protected.
- **Assume a rollback when `@Transactional` is on the same method.** Rejected per
  §1: it depends on advisor ordering the extractor cannot see, and guessing it
  wrong reintroduces the false claim this ADR exists to avoid. Spring documents the
  ordering as something to arrange deliberately, which is itself evidence it is not
  the default.
- **Read `@PostAuthorize` as a guard, extracting roles from its SpEL.** Rejected:
  its expressions reference `returnObject`, which the model has no term for, so the
  roles recovered would be the subset resembling `@PreAuthorize` and the rest would
  vanish — an unknown presented as an absence, the defect
  [ADR 0020](0020-unanalyzable-is-unknown-not-absent.md) removes. Also out of scope:
  ADR 0029 §3 defines done as *not silent*, explicitly not *read*.

## Consequences

- `internal/extract/spring`: `@PostAuthorize` recorded as an
  `UnrecognizedAuthAnnotation`; a project-wide count for `@PreFilter`/`@PostFilter`.
- `internal/model`: one project-level status for the filter counts, and the
  corrected doc comment on `UnrecognizedAuthAnnotation` (§5). **No new per-endpoint
  state.**
- `internal/lint/mutating_endpoint.go`: **this rule does change**, contrary to the
  first draft. Its suppression set excludes `@PostAuthorize` on mutating endpoints,
  and it gains the §3 message. The rule's doc comment currently states that an
  endpoint carrying an `UnrecognizedAuthAnnotation` "is skipped entirely" — that
  sentence becomes false and is corrected with the reason.
- `internal/cli/analyze.go`: one new project warning for the filters; the existing
  unrecognized-annotation warning covers `@PostAuthorize`.
- [ADR 0029](0029-spring-security-scope.md) §2 moves one row from **silent** to
  **detected and announced**. `RoleHierarchy` is split into its own decision and
  stays on the list until that lands.
- **A verb-dependent notion of protection enters the model for the first time.**
  Every mechanism so far has protected an endpoint or not. Worth naming, because
  the next mechanism with this shape should find the precedent rather than
  re-derive it.
- **Validation limit, stated plainly.** With zero corpus occurrences there is no
  real input to validate against, so the tests are constructed and the regression
  bar is a corpus no-op: `sphinxor lint` must stay byte-identical across all 20
  repositories. That is weaker than any Spring decision since ADR 0022 has met, and
  it is the honest claim — the position [ADR 0024](0024-controller-meta-annotations.md)
  §4 took for its unexercised branch, for the same reason.
