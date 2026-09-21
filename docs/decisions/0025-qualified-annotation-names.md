# 0025. An annotation's identity is its simple name, however it is written

## Status

Accepted.

## Context

Java lets an annotation be written fully qualified at the use site, and
`microcks/microcks` must:

```java
package io.github.microcks.web;

@org.springframework.web.bind.annotation.RestController
public class RestController {
```

Its own class is named `RestController`, so Spring's cannot be imported into that
file. The whole `io.github.microcks.web` package writes the annotation out in full —
**13 files**.

Extraction matches `annotationCall.Name`, which for that use is the entire dotted
path, against `controllerAnnotations`, which holds `"RestController"`. It does not
match. **None of those 13 classes is a controller as far as Sphinxor is concerned**,
verified end to end: a minimal project whose only class carries a fully-qualified
`@RestController` reports zero endpoints.

### The mirror image of the nacos collision

This is worth naming, because it is the same fault with the sign reversed.

[ADR 0022](0022-annotation-identity-and-unrecognized-authorization.md) fixed a case
where **a matching simple name made a foreign annotation count**: nacos's
`com.alibaba.nacos.auth.annotation.Secured` was read as Spring method security
because the last nine letters matched. Here, **a qualified name makes Spring's own
annotation not count**: `@org.springframework.web.bind.annotation.RestController` is
Spring's, unambiguously and more provably than any import, and it is ignored.

Both come from the same mistake — treating the *string as written* as the
annotation's identity. ADR 0022 answered it for one direction by resolving imports.
This answers the other: the identity is the simple name, and the qualification, when
present, is evidence about which package it came from rather than part of the name.

### What it costs, measured

`microcks` reports **50 endpoints**. The 13 fully-qualified files hold:

| | |
|---|---:|
| Route declarations in a shape extraction already reads (`@GetMapping` &c.) | **30** |
| …of which mutating | 11 |
| Additionally behind the separate `@RequestMapping(method = …)` gap | 4 |
| Spring Security annotations in them | **0** |

So roughly **40% of microcks's API surface is missing for a reason unrelated to any
other known gap**, and 30 of those routes are in a shape the extractor already
handles — blocked solely by the class annotation above them. Nothing about the
authorization picture changes: those files carry no Spring Security annotations, so
the 11 new mutating endpoints arrive unguarded.

This one is also silent in the way that matters: ADR 0019 §2's "recognized no
endpoints" notice cannot fire, because microcks recognizes 50.

### Do mapping annotations appear this way too?

Asked explicitly, because a rule that fixed the controller and left its routes
invisible would produce a *worse* state than the status quo — a recognized
controller with zero endpoints, which no warning covers.

Measured across all 20 repositories, counting every fully-qualified annotation use of
any package:

| Annotation | fully-qualified uses |
|---|---:|
| `RestController` | **13** |
| `Controller`, `RequestMapping`, `GetMapping`, `PostMapping`, `PutMapping`, `DeleteMapping`, `PatchMapping` | **0** |

Zero, in every spelling. The corpus's other fully-qualified annotations are
`@org.springframework.stereotype.Service` (21), `@io.swagger…RequestBody` (30) and
`@org.mapstruct.Mapper` (8) — none of which extraction reads.

So the mapping case is unexercised. §2 covers it anyway, and the reason is in §2.

## Decision

### §1 The simple name is the identity, normalized once at the parse site

`parseAnnotation` (`internal/extract/spring/syntax.go`) records the annotation's
**simple name** — the last segment of the dotted path — rather than the text as
written. Every existing matcher then works unchanged, because every one of them
already compares against a simple name.

The full path is not discarded: it is kept alongside, because §3 needs it.

Normalizing once at the parse site rather than at each comparison is the whole point.
Counted rather than estimated, an annotation's name is matched at **eight call sites
across three files** — `controllerAnnotations`, `httpMappingAnnotations`,
`methodSecurityAnnotations`, ADR 0023's third-party package lookup, ADR 0024's
controller meta-annotation map, ADR 0022's `resolveAnnotation`, a literal
`"PreAuthorize"` comparison, and `findAnnotation`'s by-name lookups for
`RequestMapping` and `AliasFor`. A fix applied at seven of them would be a latent
inconsistency of exactly the kind ADR 0011 §1 found when two consumers depended on a
naming convention neither checked.

### §2 It applies to every annotation extraction reads, including the unexercised ones

Controller annotations, mapping annotations, method-security annotations and
meta-annotation names are all matched on the simple name. The mapping annotations
have **zero** fully-qualified uses in the corpus and are covered regardless.

This is not building ahead of evidence, and the distinction matters because this
project has declined to do that twice recently — Sa-Token in
[ADR 0023](0023-third-party-authorization-annotations.md) §4, and the array-push
composite extension. Those were **new capabilities** proposed on no evidence. This
is **one normalization applied uniformly**; covering only the measured annotation
would mean adding an *exception* for the others, and that exception is what would
need justifying.

The concrete cost of the exception is the half-fixed state: microcks's
`RestController.java` would become a recognized controller whose routes are still
invisible, and no warning in the tool covers "controller recognized, zero endpoints
extracted from it". Fixing the controller and leaving its routes unreadable is worse
than fixing neither, because it looks resolved.

### §3 A fully-qualified use is its own binding

This section exists because §1 would otherwise break
[ADR 0022](0022-annotation-identity-and-unrecognized-authorization.md), and the
failure would be a confident wrong claim rather than a gap.

ADR 0022 §1 requires a method-security annotation's simple name to be bound by an
import to an accepted package, and treats an **unbound** name as not-Spring's —
recording it as an unrecognized authorization annotation. That rule is correct for a
bare `@Secured` with no import. Applied naively after §1, a fully-qualified
`@org.springframework.security.access.prepost.PreAuthorize` would normalize to
`PreAuthorize`, find no import binding it (there is none — it is written out in
full), and be recorded as **an unrecognized authorization annotation for Spring's own
annotation, spelled out in the source**.

So: **when an annotation is written fully qualified, that path is its binding**, and
it is checked against the accepted packages directly. A qualified use is a stronger
binding than an import, not a weaker one — it names the package at the use site with
no file-level indirection at all.

Measured occurrences today: **zero**. This is specified and tested regardless,
because it is not an extension — it is the correctness condition for the change §1
makes. Leaving it out would not leave a gap; it would introduce a defect.

### §4 A dotted name must match a known package in full, never by its last segment

Taking the last segment of any dotted name would mean treating
`@com.example.RestController` as Spring's — the nacos fault reintroduced by the very
change meant to fix its mirror image. So a dotted annotation name is accepted only
when the **whole path** matches a known Spring package plus the annotation, and for
method-security annotations §3's check likewise runs against the whole path.

The corpus makes the reason concrete, and not in the way first expected. The only
short dotted annotation names in 20 repositories are **`@lombok.Data` (7 uses)** and
**`@feign.Headers` (1)** — and those are not truncations. `lombok` and `feign` really
are those libraries' whole package paths, so a one-segment prefix is a complete,
valid qualification.

Which means a truncated `@annotation.RestController` and a complete `@lombok.Data`
are **indistinguishable to a name matcher**: both are one lowercase segment followed
by a type name, and telling them apart needs the imports and the package structure.
A last-segment rule cannot make that distinction and would get `@lombok.Data` right
only by accident of it not matching anything. Requiring the full path gets both right
for the same reason — `@lombok.Data` matches no Spring package, and neither would a
truncation.

## Alternatives considered

- **Match the annotation name by suffix** — accept anything ending in
  `.RestController`. Rejected: it is the nacos collision again, one dot to the left.
  `@com.example.RestController` is not Spring's, and the suffix cannot tell.
- **Special-case microcks's package.** Rejected as absurd on its face, recorded only
  because the measured population is one project and the temptation to scope the fix
  to it should be named and dismissed rather than left implicit.
- **Fix only `@RestController`, the one measured.** Rejected per §2: it produces a
  recognized controller with invisible routes, a state worse than the current one
  because it looks resolved, and it requires writing an exception rather than
  omitting one.
- **Resolve the package properly, via imports and the compilation unit's own
  package.** Unnecessary: a fully-qualified use carries its package inline, which is
  the only case that exists. Reserved for if a partially-qualified form is ever
  measured (§4).

## Consequences

- microcks rises from 50 endpoints to roughly 80, with **11 new mutating findings**
  and no change to any role or guard, since those files carry no Spring Security
  annotations.
- No model change. One field added to the internal `annotationCall` struct, which
  does not leave the extractor.
- The `@RequestMapping(method = …)` gap still hides 4 more microcks routes; this
  decision is independent of it and does not touch it.
### Validation — measured after implementation

| | before | after |
|---|---:|---:|
| microcks endpoints | 50 | **80** |
| microcks `mutating-endpoint-without-access-control` | 20 | **31** (+11) |
| microcks roles / guards | 0 | **0** |

The prediction — 50 → ~80 and 11 new findings — landed exactly, for the reason
ADR 0024 records about exact predictions: this count came from route declarations
inside files that either are or are not recognized, the same question the scan and
the extractor answer.

**No other repository in the 20-project corpus changed in any respect**, and all four
vendored fixtures are byte-identical. `microcks`'s own `RestController.java` and
`GraphQLController.java` are still absent, correctly — their routes are
`@RequestMapping(method = …)`, the separate gap this decision does not touch.

Tests, in `internal/extract/spring/qualified_names_test.go`: a fully-qualified
controller with fully-qualified mappings; `@com.example.RestController` rejected;
fully-qualified `@PreAuthorize`, `@Secured` and `@RolesAllowed` in **both**
namespaces each recognized as Spring's; a fully-qualified nacos `@Secured` and a
fully-qualified Shiro `@RequiresPermissions` each still landing as unrecognized; the
name/qualifier split including the `@lombok.Data` shape; and ADR 0024's
meta-annotation resolution surviving a qualified composed `@RestController`.

### What the call-site audit found

§1 claimed every existing matcher would work unchanged. That was **half right, and
the half that was wrong is the interesting one.** The matchers do still compare
simple names — but §4's whole-path rule has to be applied *alongside* each
comparison, so six of the eight sites gained an `isSpringWeb()` check. §1 and §4
were consistent as written; §1's "unchanged" was simply loose.

Auditing the sites afterwards for any that still matched a bare name turned up a real
hole the ADR had not anticipated: **ADR 0024's controller meta-annotation lookup**
matched `metas[a.Name]` on the simple name alone, so `@other.pkg.RestApi` would have
resolved to shenyu's `@RestApi`. That is §4's hazard applied to project-declared
annotations rather than Spring's, and this change would have introduced it. Fixed by
recording each meta-annotation's declaring package and requiring a qualified use to
match it, and pinned by a test. No corpus project writes a project meta-annotation
qualified, so nothing would have caught it.
