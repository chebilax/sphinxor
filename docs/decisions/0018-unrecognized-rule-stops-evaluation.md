# 0018. An unrecognized SecurityFilterChain rule that matches stops evaluation — correcting ADR 0012 §1

## Status

Accepted.

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
