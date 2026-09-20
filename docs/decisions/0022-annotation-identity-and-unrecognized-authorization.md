# 0022. A recognized annotation name is not a recognized annotation — and an unrecognized one is not an absent one

## Status

Accepted.

§3 was challenged during review on the grounds that [ADR 0015](0015-inert-method-security-guard.md)
reports-and-caveats where §3 suppresses, and confirmed rather than changed. The
disanalogy runs the other way: ADR 0015 has content to report — extracted roles, plus
a doubt about whether they are enforced — whereas here nothing was extracted at all.
A reworded Low finding would carry exactly what §2's collection records and the §3
warning states, a second time and in the form of an accusation. The reasoning is
folded into §3.

## Context

`internal/extract/spring/guards.go` recognizes a method-security annotation by its
**simple name** and nothing else:

```go
var methodSecurityAnnotations = map[string]bool{
	"PreAuthorize": true,
	"Secured":      true,
	"RolesAllowed": true,
}
```

Extraction reads no imports anywhere — confirmed by grep across the whole `spring`
package, not assumed. So any annotation written `@Secured(...)`, from any package, is
recorded as a Spring Security method-security guard.

### What that costs, measured

`alibaba/nacos` declares its own annotation with the same simple name:

```java
package com.alibaba.nacos.auth.annotation;

public @interface Secured {
    ActionTypes action() default ActionTypes.READ;
    String resource() default StringUtils.EMPTY;
    String signType() default SignType.NAMING;
    Class<? extends ResourceParser> parser() default DefaultResourceParser.class;
    ...
}
```

It is unrelated to `org.springframework.security.access.annotation.Secured` — 108
non-test files import it, 428 uses. Sphinxor records **392 `GuardApplication`s named
`Secured`** from it, and:

- **242 of nacos's 245 mutating endpoints escape `mutating-endpoint-without-access-control`
  on that basis.** None of them carries any other guard, so the annotation is doing
  all of the suppressing.
- [ADR 0015](0015-inert-method-security-guard.md)'s warning fires, advising that
  "those annotations are inert at runtime and the endpoints they appear to protect
  are NOT protected" for want of `@EnableMethodSecurity`. That is **doubly wrong**:
  they are not Spring's annotations, so `@EnableMethodSecurity` is irrelevant to
  them, and they *are* enforced — by nacos's own `AuthFilter`.

The endpoints are genuinely protected, so the *outcome* is safe. It is safe by luck.
Sphinxor asserted Spring method security on the strength of a nine-letter word, and
would have asserted it just as confidently had the annotation been
`@Secured` from a logging library.

### How common, and how tractable — both measured

Scanned across **20 real Java repositories** (the 14-project Spring survey corpus
plus dataease, halo, hertzbeat, inlong, litemall and metersphere), resolving every
`@PreAuthorize`/`@Secured`/`@RolesAllowed` use back to the import that binds it:

| Repository | Name | Resolves to | Uses |
|---|---|---|---:|
| `alibaba/nacos` | `@Secured` | **`com.alibaba.nacos.auth.annotation.Secured`** | 428 |
| `thingsboard/thingsboard` | `@PreAuthorize` | `org.springframework.security.access.prepost.PreAuthorize` | 536 |
| `apolloconfig/apollo` | `@PreAuthorize` | Spring | 140 |
| `yangzongzhuan/RuoYi-Vue` | `@PreAuthorize` | Spring | 116 |
| `elunez/eladmin` | `@PreAuthorize` | Spring | 99 |
| `apache/fineract` | `@PreAuthorize` | Spring | 4 |

The other fourteen use none of the three names at all; metersphere (871), litemall
(216) and inlong (79) authorize through Apache Shiro instead, and `@RolesAllowed`
does not appear anywhere in twenty repositories.

**One collision in twenty projects.** Two further measurements make the fix
tractable, and both are stated because a design that needed cross-file symbol
resolution would be a much harder call:

- **Every use of a recognized name has a visible import in the same file. Zero
  exceptions**, across all 20 repositories and all vendored fixtures — no
  same-package declarations, no wildcard imports standing in for one. Extraction is
  already per-file, so the binding is reachable with no cross-file resolution and no
  classpath.
- **Zero fully-qualified inline uses** (`@org.springframework.security.access.prepost.PreAuthorize(...)`),
  the one shape that would defeat matching on the import.

### Frequency is not the argument

One sighting in twenty would be thin grounds for a mechanism if frequency were the
case for it. It is not.

[ADR 0011](0011-spring-second-framework.md) scopes this extractor to **Spring
Security**. Recognizing an annotation as Spring method security without establishing
that it comes from Spring Security does not merely risk an error — it means the
scope boundary is not actually being enforced, only approximated by spelling.
Frequency sets the urgency; the boundary sets the correctness. nacos is the evidence
that the approximation fails on real code, not the reason the approximation is
wrong.

**`@Secured` is the name at risk, and it is worth saying why.** It is an ordinary
English word, and a plausible name for any project's own authorization annotation.
nacos is one instance. The Grails Spring Security plugin ships another,
`grails.plugin.springsecurity.annotation.Secured` — distinct from Spring's, with its
own `httpMethod` attribute and SpEL support — which is evidence that the *name* gets
reused in the ecosystem rather than a second affected project, since Grails sources
are Groovy and this extractor does not parse them. `@PreAuthorize` is distinctive
enough that a collision is unlikely. That asymmetry justifies particular care on
`@Secured`; it does not justify a rule that applies only to it, since a name-based
exception list is the same approximation one layer down.

### The trap in the obvious fix

Rejecting the foreign annotation and stopping there makes the report **less useful
and no more true**. Those 242 endpoints are protected. Reject the annotation, record
nothing in its place, and nacos goes from 3 findings to **245
`mutating-endpoint-without-access-control` findings**, each asserting that an
endpoint "has no detected guard or role decorator" — of code that has one, in plain
sight, one line above the handler.

That would trade a false claim of protection for 245 false accusations of its
absence. Both are unearned confidence; only the direction differs.

The distinction the model cannot currently express:

- **no access control found** — nothing on the endpoint. `mutating-endpoint-without-access-control`
  is exactly right.
- **access control found and not understood** — an annotation is there, it is
  evidently about authorization, and Sphinxor cannot say what it requires or whether
  Spring enforces it.

This project already has the second concept and already handles it well elsewhere:
the **global-guard** case ([ADR 0020](0020-unanalyzable-is-unknown-not-absent.md) §4)
is precisely "protection exists, its requirement is unknown, and the run says so."

## Decision

### §1 A recognized name counts as a Spring annotation only when an import binds it

`@PreAuthorize`, `@Secured` and `@RolesAllowed` are treated as Spring method-security
annotations only when the same file's imports bind that simple name to an accepted
package:

| Simple name | Accepted binding |
|---|---|
| `PreAuthorize` | `org.springframework.security.access.prepost.PreAuthorize` |
| `Secured` | `org.springframework.security.access.annotation.Secured` |
| `RolesAllowed` | `javax.annotation.security.RolesAllowed`, `jakarta.annotation.security.RolesAllowed` |

**Every string in that table was checked, because a typo in any of them would
silently reject the genuine annotation** — the inverse of the defect this ADR exists
to fix, and one the corpus would not catch:

- `PreAuthorize` is confirmed by the corpus itself: all 122 imports across the 20
  repositories are `org.springframework.security.access.prepost.PreAuthorize`, with
  no second spelling anywhere.
- `Secured` could not be confirmed that way — **no project in twenty imports Spring's
  `@Secured` at all**, so the only `@Secured` in the measured corpus is nacos's. Its
  name was verified against Spring Security's current API documentation.
- `RolesAllowed` likewise has zero corpus coverage; `jakarta.annotation.security.RolesAllowed`
  was verified against the Jakarta Annotations specification, and the `javax` form is
  its pre-rename twin.

The two with no corpus coverage therefore get positive tests of their own in the
validation bar below, rather than being trusted.

A wildcard import of an accepted package (`import org.springframework.security.access.prepost.*;`)
binds the name and is accepted. Anything else — a different package, or **no binding
import at all** — does not.

Treating an unbound name as *not established* rather than as Spring's is the
conservative reading, and it is also the only one the language supports: Spring's
annotations live under `org.springframework.*`, so a file using `@Secured` with no
import cannot be picking up Spring's by same-package resolution. It is either a
wildcard (accepted above) or a project-local annotation — which is the foreign case.
**Measured at zero occurrences** across 20 repositories and every vendored fixture,
so this costs nothing today and is chosen on principle rather than on evidence of
need.

The `RolesAllowed` row deliberately accepts both the `javax` and `jakarta` namespaces:
both are the real JSR-250 annotation and Spring reads both. This is a binding
question, not a preference.

### §2 An unrecognized authorization annotation is recorded as unknown, not discarded

An annotation that carries a recognized name but is **not** bound to an accepted
package is recorded on the model as an **unrecognized authorization annotation**
against its endpoint — its name, the import that actually bound it, and its
location. It does **not** become a `GuardApplication`.

Keeping it out of `GuardApplication` is deliberate and is the lesson of ADR 0011 §1's
`DeclaresRoles` finding: a flag on a shared entity gets read by consumers that do not
check it. A `GuardApplication{Recognized: false}` would be counted as protection by
every rule, exporter and report that does not know to look — which is the defect this
ADR is fixing, reintroduced one field deeper. A separate collection cannot be
mistaken for a guard by code that has never heard of it.

### §3 `mutating-endpoint-without-access-control` does not fire on an endpoint that has one

An endpoint carrying an unrecognized authorization annotation is not in the state that
rule describes. Its message — "has no detected guard or role decorator" — would be
false on its face, disprovable by reading the line above the handler.

So the rule skips those endpoints, and the run instead warns at project level. The
matrix marks the row rather than leaving it looking bare.

**Why suppression and not a reworded Low finding.** The obvious middle path — keep a
finding, change its wording to "carries an authorization annotation Sphinxor does not
understand" — was considered and rejected. That sentence is precisely what §2's
collection records and what the warning below says, so the finding would state the
same fact a third time, and state it as an accusation. There is nothing else for it
to carry: unlike ADR 0015, where the report holds extracted roles and the caveat adds
a doubt about whether they are enforced, here **nothing was extracted** — no roles, no
requirement, no guard. A finding with no content beyond "I did not understand this"
is noise wearing a finding's clothes.

The stronger reason is that any finding retained on such an endpoint keeps an
accusation whose premise is untrue. `mutating-endpoint-without-access-control` says
the endpoint "has no detected guard or role decorator"; an authorization annotation
is sitting one line above the handler. On nacos that premise would be false 242
times, against code that is genuinely protected.

**The residual risk, stated rather than buried**: if such an annotation is in fact
decorative — declared, never enforced — Sphinxor stays quiet where it previously
accused. That is a real loss of a finding. It is accepted because the trade is
structurally lopsided, not merely convenient: a false negative on a rare case (a
home-grown authorization annotation that enforces nothing) against a false positive
on a common one (a home-grown authorization annotation that works). And it is not
silence — §2 records the annotation, the matrix marks the row, and the warning names
the package, which is the one thing a reader needs to check what Sphinxor cannot.

### §3a The warning names the package and the count, because a bare one is unusable

The warning must be actionable, not merely present. "Unrecognized annotations were
found" tells a reader nothing they can act on; **"392 endpoints carry
`com.alibaba.nacos.auth.annotation.Secured`, which Sphinxor does not recognize"**
tells them exactly what to go and read, and how much of the report depends on it.

So it names, for each distinct unrecognized annotation: the package it was actually
bound to, and how many endpoints carry it. Naming the *bound package* rather than the
simple name is the load-bearing part — `@Secured` alone would read as Spring's, which
is the confusion this whole ADR exists to remove.

### §4 ADR 0015's inert-method-security warning stops seeing foreign annotations

`hasMethodSecurityAnnotations` (`internal/cli/analyze.go`) drives ADR 0015's "these
annotations may be inert" caveat off `GuardApplication.GuardName`. Under §1 a foreign
annotation never becomes a `GuardApplication`, so the warning stops firing on it —
and the doubly-wrong advice measured on nacos stops with it. This is a consequence of
§1 rather than a separate change, and it is named here so it is verified rather than
assumed.

## Alternatives considered

- **Keep simple-name matching, add `com.alibaba.nacos…Secured` to a denylist.**
  Rejected. It fixes the one project that was measured and leaves the boundary
  approximated by spelling for every project that was not. A denylist is also
  unbounded in the wrong direction — the set of non-Spring annotations named
  `Secured` is not enumerable.
- **Require an import only for `@Secured`, the name actually at risk.** Tempting,
  since `@PreAuthorize` collisions are implausible and this would be a smaller change.
  Rejected: it is the same approximation with a shorter list, and it would leave the
  code asserting a Spring binding it never checked for two of three names. The
  measurements show the general rule is free.
- **Reject the foreign annotation and record nothing.** Rejected — this is the
  245-false-accusation outcome analysed above. ADR 0020's principle is *record as
  unknown*, not *record nothing*.
- **Record it as a `GuardApplication` with a `Recognized: false` flag.** Rejected,
  per §2: a flag on a shared entity is read only by consumers that know to read it,
  and every one that does not would treat it as protection.
- **Resolve annotations through the classpath, properly.** Out of scope and
  unnecessary: it would require build-system integration and dependency resolution,
  and the measurements show every real use is bound by an import in the same file.

## Consequences

- One new model collection and one new project warning. `GuardApplication`,
  `RoleReference` and the export format are untouched.
- Extraction reads Java import declarations for the first time. That is new machinery,
  but per-file and small — the binding of three names, not a symbol table.
- **nacos**: 392 asserted Spring guards become 392 recorded unknowns; the ADR 0015
  warning stops firing; `mutating-endpoint-without-access-control` must **not** jump
  to 245. The §11 unresolved-roles warning from
  [ADR 0020](0020-unanalyzable-is-unknown-not-absent.md) Amendment 3 stops firing for
  these too, since they are no longer role-declaring guards — replaced by this ADR's
  own warning, which is the more accurate statement about them.
- A project using a *correctly imported* Spring annotation sees no change whatsoever.
- **One cosmetic gap, unexercised and deliberately left.** `sphinxor export cerbos`
  omits an endpoint whose only access control is an unrecognized annotation under
  `ReasonNoGuard` ("no GuardApplication at all was found"), which understates what is
  known. Adding a reason code for it would be building ahead of evidence: no project
  in the corpus reaches that path, since nacos's export is already omitted wholesale
  for its unreadable URL layer, and its export is byte-identical before and after.
  Recorded in `docs/limitations.md` rather than fixed.

### Validation — measured, not predicted
Run against the real corpus per `docs/testing.md`. Every item below was measured
after implementation:

- **nacos**: guards **392 → 0**, unrecognized-annotation records **0 → 392**, all
  bound to `com.alibaba.nacos.auth.annotation.Secured`.
  `mutating-endpoint-without-access-control` stays at **3** — it did not jump to 245.
  ADR 0015's inert-method-security warning is gone (§4), ADR 0020 Amendment 3 §11's
  unresolved-roles warning is gone with it, and this ADR's warning is present naming
  the package and the count.
- **Exactly one repository in the 14-project Spring corpus changed**, and it is
  nacos: a file-by-file diff of both stdout and stderr across all 14 shows no other
  difference of any kind. thingsboard still records 467 guards and 439 role-carrying
  endpoints; apollo (68), RuoYi-Vue (116) and eladmin (99) keep every guard, all
  bound by correct Spring imports.
- `Pharmacy`, `blog-api`, `ruoyi-vue-pro` and `tutorials` produce **byte-identical**
  output, and nacos's Cerbos export — policies and report — is byte-identical too.
- Tests, in `internal/extract/spring/imports_test.go`: the same `@Secured` name
  imported from Spring and from nacos, guard in the first case and unrecognized
  annotation in the second; a recognized name with no binding import; a wildcard
  import that does bind; a static import that must not; and §3 end to end, where an
  endpoint with an unrecognized annotation is skipped while a genuinely bare sibling
  in the same file is still flagged.
- Positive tests for the two accepted package strings with **no corpus coverage** —
  Spring's `@Secured`, and `@RolesAllowed` in both namespaces. Nothing else in the
  suite would catch a typo there, and the failure it would cause is this ADR's own
  defect inverted.

**One consequence not anticipated in the draft**: six existing unit tests had Java
snippets using `@PreAuthorize`/`@Secured` with no import at all — the shorthand a
hand-written snippet naturally falls into, and a shape §1 now treats as unbound. They
were corrected by adding the real import rather than by relaxing the rule: an
annotation with no import would not compile, so the snippets were not valid Java for
what they claimed to test. Every test that already wrote its imports out in full —
including both that use the vendored fixtures — passed untouched.
