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
