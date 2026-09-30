# 0018. An unrecognized SecurityFilterChain rule that matches stops evaluation — correcting ADR 0012 §1

## Status

Accepted.

**Amendment 1: Accepted** (2026-09-30, the owner's decision). An endpoint whose first
matching rule is unresolved no longer gets "nothing from the URL layer". Its effective
policy is unknown whenever some possible outcome of the unresolved rule could narrow
access. That closes the latent over-grant recorded in *Consequences* below. See
*Amendment 1*.

## Context

ADR 0012 §1 states: "A rule using an unrecognized shape is simply not
matched by extraction — the endpoint falls through to the next recognized
rule (or nothing)." Taken literally, and checked against the real,
already-vendored `blog-api` fixture before writing any rule-matching code,
this is a safety inversion, not merely an incomplete description.

`blog-api`'s real `SecurityConfig.java` (`internal/extract/spring/testdata/blog-api/`)
contains, in source order: `.requestMatchers(GET, "/tenants/{tenantId}/entries.zip").access(exportForTenant)`
(unrecognized — a custom `AuthorizationManager`), followed later by the
trailing `.anyRequest().permitAll()`. In real Spring evaluation,
first-match-wins means a request to `GET /tenants/{tenantId}/entries.zip`
is governed by the `.access(exportForTenant)` rule — the real,
unknown-to-Sphinxor check — and Spring never reaches `.anyRequest()` for
that request at all. ADR 0012 §1's literal rule — treat the unrecognized
rule "as not matched," fall through to the next one — would have extraction
skip past it and land on `.anyRequest().permitAll()`, concluding the
endpoint is public. That's a confidently wrong, dangerously *permissive*
answer for an endpoint actually gated by custom logic: exactly the failure
mode this project's "omit and flag, never guess in the permissive
direction" posture exists to prevent (docs/vision.md;
docs/decisions/0009-cerbos-exporter.md §3).

"Not matched by extraction" conflates two different facts: the rule is
*unrecognized* (Sphinxor cannot read what it grants) and the rule is
*present and governing* (Spring itself evaluates and stops at it, whether
or not Sphinxor can read it). ADR 0012 §1 treated unrecognized as if it
meant absent. It means opaque, not absent — and an opaque, first-matching
rule is not the same situation as no rule matching at all.

The specific endpoint that would trigger this
(`GET /tenants/{tenantId}/entries.zip`, owned by `EntryRestController`) is
not itself vendored — only `blog-api`'s `SecurityConfig.java` and two other
controllers are (`internal/extract/spring/testdata/blog-api/NOTICE.md`).
The danger doesn't depend on that: the rule chain producing it is real,
vendored, and unmodified: the failure is a property of the chain's
structure, verifiable by reading it, independent of which specific real
endpoint happens to strike the unrecognized rule first.

## Decision

**Rules are evaluated in source order. The first rule whose pattern (and,
if scoped, HTTP method) matches the endpoint wins — recognized or not. If
that first match is unrecognized, the endpoint's URL layer result is
unresolved (nothing is contributed, the same as an unparseable case
everywhere else in this project) — evaluation never continues past a
matching rule to a later one, regardless of recognition.**

A rule is only skipped when its pattern (or method scope) does not match
the endpoint at all — an ordinary non-match, no different in kind from any
other inapplicable rule, recognized or not. The boundary is exact:

- Unrecognized **and** matches → stop; URL layer unresolved for this
  endpoint.
- Unrecognized **and** does not match → continue to the next rule, same as
  any non-matching rule.
- Recognized **and** matches → this rule's grant is the URL layer's
  contribution; stop (first-match-wins).
- Recognized **and** does not match → continue.

This is exactly real Spring `authorizeHttpRequests` evaluation order,
translated faithfully rather than approximated for extraction's
convenience.

## Alternatives considered

- **Keep ADR 0012 §1 as written** — rejected: the blog-api demonstration
  above is a real, reproducible false-permissive result on real, vendored
  source, not a hypothetical edge case. Shipping it would mean Sphinxor
  asserting public access on an endpoint a real reviewer would need to
  check by hand — worse than the coverage gap this project already accepts
  everywhere else, because it's confidently wrong rather than honestly
  silent.
- **Skip the unrecognized rule but stop scanning at that endpoint's own
  next rule regardless of match** — considered and rejected: adds
  complexity (tracking "we already gave up on this endpoint") for no
  benefit over the simpler, exact rule stated above; the exact rule
  already produces "unresolved" correctly without needing a separate
  bookkeeping concept.

## Consequences

- `internal/extract/spring`'s SecurityFilterChain rule-evaluation pass
  (PR 3) implements the corrected rule directly — this ADR is written
  before that code, not as a retrofit.
- `docs/decisions/0012-securityfilterchain-effective-policy.md` §1's
  "falls through to the next recognized rule" sentence is superseded by
  this ADR's stated boundary; ADR 0012's other content (recognized shapes,
  the method×URL intersection in `internal/export/cerbos`, `ScopeRequestMatcher`)
  is unaffected.
- Validation: the exact endpoint that would trigger the false-permissive
  result (`GET /tenants/{tenantId}/entries.zip`) isn't vendored
  (`EntryRestController` isn't part of the curated blog-api subset). The
  rule chain producing the danger is real and vendored; the endpoint
  striking it is constructed for the test, labeled explicitly as such —
  the same split already applied to `internal/export/cerbos`'s
  partial-overlap/empty-intersection tests (docs/testing.md). The test
  must assert the constructed endpoint resolves to *unresolved*, not
  public, and must be confirmed (the same way the ADR 0014 merge-bug
  regression test was) to actually fail under the ADR 0012 §1
  fall-through behavior this ADR corrects.
- **This correction's safety value is currently latent, not active —
  recorded here so a future reader doesn't mistake it for redundant.** The
  model as it stands has no positive "intentionally public" fact:
  `permitAll()` and an unrecognized-rule match are represented identically
  (nothing contributed — `applySecurityFilterChain` handles
  `chainNoRequirement` and `chainUnrecognized` in the same branch). So the
  false-permissive *outcome* this ADR describes isn't observable in today's
  model; what the corrected rule actually fixes today is which rule is
  identified as governing, and it prevents a genuinely wrong grant in the
  case where a later matching rule is a recognized roles rule (the old
  behavior would have attached that rule's roles to an endpoint the opaque
  rule really governs). The permissive-direction danger becomes real the
  day the model distinguishes "intentionally public" from "could not
  determine" — which any exporter or rule needing that distinction would
  introduce. Getting evaluation order right by anticipation is cheaper
  than retrofitting it under a model that has started to trust it.

## Amendment 1 — An unresolved rule makes the effective policy unknown when it could narrow access (2026-09-30)

### The latent issue, now reachable

*Consequences* recorded that "unresolved" was handled exactly like `permitAll()`: the
URL layer contributes nothing. The false-permissive outcome was latent until "the model
distinguishes 'intentionally public' from 'could not determine'".

The Cerbos export already crosses that line. [ADR 0012](0012-securityfilterchain-effective-policy.md)
makes the effective policy the intersection of both layers, and an intersection can only
narrow. An endpoint with a method guard whose URL rule is opaque was exported with the
method roles alone, as if the URL layer added nothing. The opaque rule may restrict
further than the method guard, or deny outright, so the export can grant more than the
application does. The shapes:
- `.access(...)`;
- a rule inside a branch ([ADR 0040](0040-multiple-security-filter-chains.md) §4, as read
  since #74);
- a helper method handed the registry.

**Measured before this amendment** on corpus-20, the 12 corpus-40 projects drawn so
far, and every fixture:
- **No endpoint is exported** with a method guard behind an unresolved URL rule.
- **Five projects have endpoints behind an unresolved rule:** RuoYi-Vue 148, mateclaw
  501, wvp 395, arthas 22, and the `blog-api` fixture 1. Only RuoYi-Vue's are
  method-guarded, 117 of them, and they are not exportable without declarations.
- **With [ADR 0041](0041-permission-export.md)'s declarations for RuoYi-Vue,** the
  export writes 100 rules on endpoints governed first by an opaque rule:

  ```java
  permitAllUrl.getUrls().forEach(url -> requests.requestMatchers(url).permitAll());
  ```

  Those 100 rules are correct, and that is what shapes this amendment.

### Decision: the outcome set

A rule that matches an endpoint but **may not apply** keeps its terminal. That is a rule
with an unreadable matcher, or one that is not straight-line code (ADR 0040 §4). It then
contributes two possible outcomes: its terminal, or evaluation continuing to the later
rules. Evaluation collects every outcome an endpoint can reach:
- **A rule that surely matches, is readable and recognized** ends the walk, as before.
  With no uncertain rule before it, the result is exactly ADR 0012's.
- **A rule that may not apply** adds its terminal and evaluation continues.
- **An opaque rule adds "anything".** That is an unrecognized terminal such as
  `.access(...)`, a helper call handed the registry, or a verb-scoped rule met by an ANY
  endpoint (ADR 0028 §2).
- **Reaching the end** without a sure match adds "no rule".

**The effective policy is unknown only when some possible outcome could narrow access:**
a role rule, `denyAll()`, or "anything". When every outcome is `permitAll()`,
`authenticated()` or "no rule", the URL layer cannot narrow the method layer. The method
layer alone is then exact, and it is exported.

RuoYi-Vue is the second case. Its opaque rule ends in `permitAll()` or reaches
`authenticated()`, so its 100 declared rules stay. A first draft of this amendment made
every unresolved rule unknown. It would have dropped them, and was narrowed after this
measurement.

### Consumers (interim)

| Consumer | An endpoint whose effective policy is unknown |
|---|---|
| **Cerbos export** | Omitted, under its own reason `url-rule-unresolved`, naming the rule's line. |
| **Matrix** | The method layer's guards and roles still shown, as inventory. The Guards cell gains `URL ?`, and JSON rows gain `urlRuleUnresolved: true` (omitted when false, so every other row is byte-identical). A run warning names the endpoints. |
| **`mutating-endpoint-without-access-control`** | Still fires at Low, with a message saying the URL rule that applies could not be read. The unread rule may be a `permitAll()`, so the endpoint may truly be public, and suppressing the finding would hide that. |
| **`sphinxor diff` and the became-public gate** | Counted as not confirmed public, as [ADR 0036](0036-became-public-gates-ci.md) already does for an unrecognized annotation. The gate fires when an endpoint goes from protected, or unknown, to known unprotected. |

### Measured after

Main against this change, on corpus-20, the 12 corpus-40 projects, every fixture, and
the #74 reproduction. Everything else is byte-identical in lint JSON, policies, export
report and warnings.

| Target | Rows marked `URL ?` | Export | Findings |
|---|---:|---|---|
| mateclaw | 501 of 559, all behind its unreadable `OPENAPI_PATHS` branch, whose `else` arm is `hasRole("ADMIN")` | 0 rules before and after; 501 omissions change reason from `no-guard` to `url-rule-unresolved` | 307, the same count, and the messages name the rule |
| RuoYi-Vue | 1, `ANY /`: a verb-scoped `requestMatchers(GET, "/", …)` met by an ANY endpoint | policies byte-identical | unchanged |
| `blog-api` fixture | 1 of 4, the `.access(exportForTenant)` endpoint | 1 rule before and after | unchanged |
| #74 reproduction | 2 of 2 | 0 rules before and after | unchanged |

RuoYi-Vue's export with ADR 0041 declarations is byte-identical: 100 rules, 31
omissions.

This table is the interim behaviour. The per-endpoint review list of the next ADR is
meant to replace the `mutating-endpoint-without-access-control` row.
