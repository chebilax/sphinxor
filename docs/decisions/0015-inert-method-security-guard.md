# 0015. A confirmed-inert method-security guard counts as unguarded

## Status

Accepted.

## Context

`@EnableMethodSecurity` independently gates three annotation families via
three attributes, each with its own real default, verified against Spring's
own documentation rather than assumed:

| Annotation(s) | Attribute | Default on `@EnableMethodSecurity` | Default on `@EnableGlobalMethodSecurity` (deprecated) |
|---|---|---|---|
| `@PreAuthorize`/`@PostAuthorize` | `prePostEnabled` | `true` | `false` |
| `@Secured` | `securedEnabled` | `false` | `false` |
| `@RolesAllowed`/`@PermitAll`/`@DenyAll` | `jsr250Enabled` | `false` | `false` |

A `@Secured`/`@RolesAllowed` annotation is real source, and extraction
correctly records a `GuardApplication` for it (the annotation is genuinely
present — ADR 0011 §1). But if the project's `@EnableMethodSecurity` never
sets `securedEnabled`/`jsr250Enabled` to `true`, that annotation is inert at
runtime: Spring never evaluates it, and the endpoint is unprotected by it.
`internal/lint/mutating_endpoint.go` (`MutatingEndpointWithoutAccessControl`)
only checks `GuardApplication` *presence* — it has no way to see "present but
inert," so it stays silent on an endpoint that is, in the running
application, wide open. This is exactly the false-confidence risk ADR 0011
§1 named: Sphinxor asserting protection that isn't actually enforced, worse
for an auditing tool than a missed finding.

ADR 0011 flagged that a confidence caveat was needed here but left "exact
confidence-level plumbing" undecided. Working through it surfaced that it
isn't a confidence question at all — no existing rule's confidence varies
per-finding, and the gap is a finding that currently doesn't fire, not one
that fires at insufficient confidence. It's a behavior change to
`mutating-endpoint-without-access-control`, decided here rather than folded
into implementation.

## Decision

**Add a project-wide `MethodSecurityStatus` fact to `model.Model`.
`mutating-endpoint-without-access-control` treats a `GuardApplication` whose
annotation family is confirmed not enabled as equivalent to no guard at
all — but only when the evidence is a confirmed negative, never when it's
merely absent.**

```go
type MethodSecurityStatus struct {
	// Found is true if @EnableMethodSecurity or the deprecated
	// @EnableGlobalMethodSecurity was found anywhere in the parsed
	// project. False means "no evidence either way" — Sphinxor's static
	// view is necessarily partial (a base class, a parent module, a
	// starter-based setup, or Kotlin config (ADR 0011's own stated blind
	// spot) could enable it outside what was scanned) — never "confirmed
	// disabled project-wide."
	Found bool
	// PrePostEnabled/SecuredEnabled/Jsr250Enabled are true only when
	// Found is true and at least one located annotation enables that
	// family (respecting *that annotation's own* real default for any
	// attribute it doesn't set explicitly — the two annotations' defaults
	// differ, per the table above). Meaningless when Found is false.
	PrePostEnabled bool
	SecuredEnabled bool
	Jsr250Enabled  bool
}
```

The liveness check per `GuardApplication.GuardName`:

```go
func isConfirmedInert(g model.GuardApplication, s model.MethodSecurityStatus) bool {
	if !s.Found {
		return false // unknown, not confirmed inert — never downgrade on absence of evidence
	}
	switch g.GuardName {
	case "PreAuthorize", "PostAuthorize":
		return !s.PrePostEnabled
	case "Secured":
		return !s.SecuredEnabled
	case "RolesAllowed":
		return !s.Jsr250Enabled
	default:
		return false // NestJS guards, or any future non-method-security guard: unaffected
	}
}
```

`mutating-endpoint-without-access-control` treats an endpoint as guarded
only if at least one of its `GuardApplication`s is *not* confirmed-inert —
an endpoint whose only guards are all confirmed-inert is flagged exactly as
if it had no `GuardApplication` at all.

**The sharp boundary this decision depends on, stated explicitly**:
"confirmed not enabled" (`Found == true` and the specific flag is `false`,
by explicit setting or by that annotation's own real default) downgrades to
unguarded. "Not found at all" (`Found == false`) does not — it leaves
today's behavior exactly as it is, guard presence still counts as guarded.
Collapsing these two into one "not confirmed enabled" bucket would create
the symmetric failure this decision exists to fix: a real, live guard in a
part of the project Sphinxor's static view doesn't reach would be
misreported as unguarded, a false positive on the "vulnerability" side —
annoying, but importantly not the dangerous direction (a false claim of
protection). Only a confirmed negative — Spring's own documented behavior,
not Sphinxor's inference — earns the downgrade.

**Why this is not deferred the way `SecurityFilterChain` parsing was staged
across ADR 0011/0012.** That staging was a genuine "I don't see this yet"
omission — a real absence of information, reported honestly as
unresolved. This gap is different in kind: Sphinxor *does* have the
information (`@EnableMethodSecurity`'s attributes are Spring's own
documented, deterministic behavior, not something requiring new parsing
capability to discover) and currently discards it, actively reporting
"guarded" on an endpoint confirmed unprotected. An auditing tool doesn't
defer fixing a known false negative to "measure real-world impact" — the
fact is settled by Spring's own documentation, not hypothetical.

## Alternatives considered

- **Record the fact in the model for report display only, leave
  `mutating-endpoint-without-access-control`'s logic unchanged** —
  rejected: leaves the actual false-confidence gap open (the whole reason
  this needed a decision) while adding a fact nothing consults for safety.
  Correctly identified as *not* analogous to `SecurityFilterChain`'s staged
  rollout — that gap is an honest "don't know," this one is Sphinxor
  actively asserting something Spring's own docs already contradict.
- **Downgrade confidence instead of changing whether the finding fires** —
  rejected: no existing rule varies confidence per-finding (`empty-role` is
  fixed High; the other two are fixed Low), and the real gap is a finding
  that doesn't exist yet, not one that exists at the wrong confidence.
- **Treat `Found == false` the same as a confirmed-inert family** —
  rejected: collapses "no evidence" into "confirmed absent," producing
  false positives against real, live guards Sphinxor's static extraction
  simply didn't observe (a base module, a parent config, Kotlin source).
  The whole point of this ADR is to react only to a *confirmed* negative.

## Consequences

- `internal/model`: new `MethodSecurityStatus` type, new
  `Model.MethodSecurity` field. NestJS-produced models leave this at its
  zero value (`Found: false`) — no behavior change, since
  `isConfirmedInert` returns `false` immediately when `Found` is false.
- `internal/extract/spring`: a new project-wide scan for
  `@EnableMethodSecurity`/`@EnableGlobalMethodSecurity`, computing
  `MethodSecurityStatus` before (or independent of) per-file guard
  extraction — the same "project-wide fact needed before resolving
  per-file references" shape `internal/extract/nestjs`'s role-declaration
  pass already has.
- `internal/lint/mutating_endpoint.go`: gains the confirmed-inert check.
  This is the "further required change" ADR 0011's success criterion asked
  to be reported, not silently patched — reported here. `internal/diff`,
  `internal/export/cerbos`, and the other two lint rules are unaffected:
  none of them make a "is this endpoint guarded at all" decision the way
  `mutating-endpoint-without-access-control` does.

---

## Amendment 1 — `@EnableReactiveMethodSecurity` (2026-09-21)

### Status

Accepted.

### §1 Context: the one place the tool says something false

[ADR 0029](0029-spring-security-scope.md) as accepted listed
`@EnableReactiveMethodSecurity` as **silent**, and its §3 singled the mechanism out
as the one where silence was not the whole problem. (That row now reads **read**,
changed by this amendment.)

The original decision above scans for two enabling annotations. A reactive project
enables method security with a third, so `MethodSecurity.Found` stayed `false`, and
the ADR 0020 §4 caveat fired — quoted here as it stood before this change:

> method-security annotations were found, but no `@EnableMethodSecurity` /
> `@EnableGlobalMethodSecurity` was located in the analyzed source. If it isn't
> enabled elsewhere (a parent module, Kotlin config), those annotations are inert
> at runtime and the endpoints they appear to protect are **NOT protected**.

On a correctly configured WebFlux application that is a false statement about a
real, working security configuration — not a gap reported honestly, which is what
`Found == false` is designed to be everywhere else. The reason it is wrong here is
narrow and fixable: the enabling annotation *was* in the analyzed source, and the
scan did not know to look for it.

### §2 What the annotation actually does

`@EnableReactiveMethodSecurity` does **not** resemble its servlet counterpart, and
the amendment is only safe because that was checked rather than assumed. It
declares `proxyTargetClass`, `mode`, `order` and `useAuthorizationManager` — and
none of `prePostEnabled`, `securedEnabled`, `jsr250Enabled`.

Established by reading the two configuration classes it imports, not the reference
documentation:

| Family | Effective | Evidence |
|---|---|---|
| `@PreAuthorize` / `@PostAuthorize` | **enabled**, unconditionally | on the `useAuthorizationManager = true` path, `ReactiveAuthorizationManagerMethodSecurityConfiguration` registers `AuthorizationManagerBefore`/`AfterReactiveMethodInterceptor`; on the `false` path, `ReactiveMethodSecurityConfiguration` registers `PrePostAdviceReactiveMethodInterceptor` |
| `@Secured` | **not enabled** | neither configuration registers a `SecuredAuthorizationManager` or any `@Secured` metadata source |
| `@RolesAllowed` (JSR-250) | **not enabled** | neither configuration registers a `Jsr250AuthorizationManager` |

`useAuthorizationManager` (default `true`) selects *which* pre/post implementation is
registered, not *whether* one is, so it changes none of the three and is
deliberately not read.

**The first draft of this amendment had the JSR-250 row wrong**, following
`useAuthorizationManager` on the strength of the servlet annotation's documentation
mentioning JSR-250 compliance. Reading the reactive configuration classes falsified
it. Recorded rather than quietly corrected because the difference is exactly the
standard the original decision set for itself — Spring's own behaviour, not a
plausible inference — and because the `@Secured` row rests on the same evidence and
would have been believed on the same weak grounds.

#### §2.1 Version provenance, and when this must be re-checked

**Verified against Spring Security 6.5.11, 7.1.1 and `main` (7.2.0-SNAPSHOT), on
2026-09-21.** All three agree: `EnableReactiveMethodSecurity` declares the same
four attributes, and `ReactiveAuthorizationManagerMethodSecurityConfiguration`
registers exactly `preFilter`, `preAuthorize`, `postFilter`, `postAuthorize` and
an expression handler — no `SecuredAuthorizationManager`, no
`Jsr250AuthorizationManager`. 6.5.x and 7.x are the two lines currently
maintained.

**This conclusion is version-bound, and the `@Secured`/`@RolesAllowed` rows are the
version-bound part.** They are a *negative* established by reading source: "no
interceptor is registered for this family." A negative of that shape is only true
of the releases it was read from. If a later Spring Security adds reactive
`@Secured` or JSR-250 support, `securedEnabled: false` stops describing Spring and
starts producing **false positives** — `mutating-endpoint-without-access-control`
firing on endpoints that are genuinely protected, which is the failure direction
[ADR 0011](0011-spring-second-framework.md) §1 tolerates but does not want.

The pre/post row is not exposed the same way. It is a positive — an interceptor
that is registered — and a future release removing it would be a breaking change
to the annotation's entire purpose.

**Re-check on every Spring Security major version.** The check is two files and
takes minutes: read `EnableReactiveMethodSecurity`'s attributes, and read the bean
methods of the configuration class its selector imports. If either row in the §2
table changes, `enableReactiveMethodSecurityDefaults` changes with it and this
section records the new version. The defaults live in one struct literal in
`internal/extract/spring/method_security.go` carrying a pointer back here, so
there is exactly one place to change.

### §3 Decision

**`scanMethodSecurityStatus` recognizes `@EnableReactiveMethodSecurity`, with the
effective flags `prePostEnabled: true`, `securedEnabled: false`,
`jsr250Enabled: false`.** Everything else in this ADR applies unchanged: the flags
OR-combine across enablers, and `isConfirmedInert` reads them as before.

Two consequences follow from that reuse, both intended:

- **A correctly configured reactive project stops being told its `@PreAuthorize`
  annotations may be inert.** This is the defect §1 names.
- **A `@Secured` or `@RolesAllowed` under a reactive-only configuration is
  downgraded to unguarded**, and `mutating-endpoint-without-access-control` fires
  on its endpoint. This is a new finding that did not exist before, on an
  annotation Spring genuinely never evaluates.

The second is the part that needed the evidence in §2 to be real. It is a
*confirmed negative* in this ADR's sense — Spring registers no interceptor for
those families on either reactive path — and not the "absent, therefore assumed
off" reasoning the original decision rejects. Had the answer only been "the
documentation does not mention `@Secured`", the correct treatment would have been
to leave `Found` false for it, because silence in a document is not a confirmed
negative and this ADR's whole boundary is that distinction.

The direction is also the safe one: the downgrade *understates* protection, which
the original decision already identified as the tolerable failure mode. That is a
reason it is acceptable, not a reason it would have been acceptable without the
evidence.

**A hybrid application keeps `@Secured`.** A project carrying both enablers has it
genuinely switched on by the servlet one, and the existing OR across located
annotations already produces that — pinned by a test, since the reactive enabler
contributing a `false` makes the OR load-bearing in a way it was not before.

### §4 Measured effect

One corpus repository out of twenty uses the annotation: **halo**, in
`WebServerSecurityConfig`, alongside `@EnableWebFluxSecurity` and a
`SecurityWebFilterChain`.

**`sphinxor lint` produces byte-identical output on all twenty repositories,
before and against this change** — 765 KB of report compared, zero differing
lines. That is the validation, and it is a negative one by design.

halo's report is unchanged for two independent reasons, both worth stating because
either alone would have been enough:

1. It declares **zero** `@PreAuthorize`, `@Secured` and `@RolesAllowed`
   annotations, so `hasMethodSecurityAnnotations` is false and the §1 caveat never
   fired for it. Its authorization lives entirely in the reactive URL layer, which
   [ADR 0020](0020-unanalyzable-is-unknown-not-absent.md) §3 already detects and
   announces.
2. It yields **zero recognized endpoints** from 1,001 parsed files, and the run
   says so. Its production routes are `RouterFunction`s in 151 files — reactive
   functional routing, which is not annotation-based and not Spring Security. Its
   only four `@RestController`s are nested classes inside test files — the nested
   controller on [ADR 0029](0029-spring-security-scope.md)'s remaining silent list.

This corrects ADR 0029 §3, which as accepted stated that all of its remaining items
"measure zero occurrences across the 20-repository corpus". That is true of the
*defect* here and false of the *mechanism*: halo uses the annotation, and halo also
exercises the nested-controller item. The difference matters because the count was
offered as the reason the urgency is low, and one of the two readings supports that
and the other does not. The claim is corrected in place rather than left standing.

So the amendment is validated by construction rather than by corpus delta: the
false statement is reproducible only on a project that both enables method security
reactively *and* annotates handlers, which no corpus repository does. Five tests
pin the behaviour, including both directions §3 calls out, the hybrid case, and the
unchanged absence boundary.

### §5 Consequences

- `internal/extract/spring/method_security.go`: one entry in
  `methodSecurityDefaultsFor`, one `methodSecurityDefaults` value carrying the §2
  table, and a note on the OR that the hybrid case now depends on.
- `internal/cli/analyze.go`: the ADR 0020 §4 caveat names
  `@EnableReactiveMethodSecurity` alongside the other two. A caveat that reports
  what was not found must name what was looked for, or a reactive reader
  reasonably concludes the reactive enabler was never checked.
- No model change, no new field, no new consumer — the [ADR 0011](0011-spring-second-framework.md)
  §1 `DeclaresRoles` hazard does not apply, because this adds a way of setting
  existing fields rather than a state something must learn to read.
- [ADR 0029](0029-spring-security-scope.md) §2 moves this mechanism from **silent**
  to **read**, and its §3 count is corrected as described in §4. Four items remain.
- **A standing obligation**: §2.1's negative is re-checked on every Spring Security
  major version. This is the first decision in the log that depends on an external
  project *not* doing something, so it is the first that can be falsified by someone
  else's release rather than by a change here.
