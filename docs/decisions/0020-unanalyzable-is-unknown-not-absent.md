# 0020. A rule, matcher, or layer that could not be analyzed is unknown, never absent — generalizing ADR 0018

## Status

Proposed.

## Context

A blind-spot audit across the documented limitations and several new real
repositories found three silent failures. They are not three bugs. They are one
conceptual error at three scopes, and it is an error this project already
identified and corrected once.

[ADR 0018](0018-unrecognized-rule-stops-evaluation.md) found that ADR 0012 §1 had
conflated *unrecognized* with *absent*: a `SecurityFilterChain` rule whose **shape**
Sphinxor couldn't read was treated as if it weren't there, so evaluation fell
through to a later, more permissive rule. ADR 0018 fixed that — for a rule's shape.

The principle behind it was never applied to the two neighbouring cases:

- a **matcher** whose pattern can't be read, and
- an entire **layer** that can't be analyzed.

Both still resolve to "absent," and both produce silently wrong answers.

### A. An unreadable matcher becomes match-all

`requestMatchersArgs` (`internal/extract/spring/securityfilterchain.go`) collects
only string-literal arguments. When none are found it returns `nil` patterns — and
upstream, `nil` patterns is *also* the sentinel meaning `anyRequest()`, "matches
every path." So a matcher Sphinxor cannot read silently becomes a rule that matches
**everything**.

Verified against a real run:

```
.requestMatchers(antMatcher("/admin/**")).hasRole("ADMIN")
.anyRequest().permitAll()

| DELETE | /admin/wipe  | ... | ADMIN | - |
| DELETE | /public/wipe | ... | ADMIN | - |   <- really permitAll(): anyone can call it
0 finding(s), 0 blocking, 0 warning
```

**State this plainly, because it is the worst defect found in this project to
date**: Sphinxor asserts protection that does not exist, on a destructive endpoint,
*and* suppresses the `mutating-endpoint-without-access-control` finding that exists
precisely to catch an unguarded `DELETE`. Both the claim and the safety net fail
together, in the same direction, with a clean report. That is not incomplete
coverage — it is a false security assurance, the single failure mode
`docs/vision.md` and [ADR 0009](0009-cerbos-exporter.md) §3 are written to prevent.

The trigger is not exotic. `requestMatchers(antMatcher("/admin/**"))` is idiomatic
Spring Security 6 — the officially recommended form when mixing matcher types —
and `RegexRequestMatcher`, `mvc.pattern(...)`, and custom `RequestMatcher` beans
all take the same path.

### B. An unanalyzable URL layer is treated as no URL layer

`findSecurityFilterChainRules` requires exactly one `SecurityFilterChain` bean
(`if len(lambdas) != 1 { return nil, "", false }`); `Extract` then skips the URL
layer entirely. Nothing records that a layer existed and went unread.

The consequence reaches the **deployable artifact**. The same application, differing
only in bean count:

| | exported Cerbos policy | reality |
|---|---|---|
| one chain | `roles: [ADMIN]` | ADMIN |
| two chains | `roles: [ADMIN, ANALYST]` | ADMIN |

The two-chain export grants `ANALYST` an access the running application denies, and
the export report says nothing. Multi-chain configurations are common (a separate
chain for actuator, or API versus form login).

### C. The reactive URL layer is not looked for at all

`internal/extract/spring` contains no reference to `SecurityWebFilterChain`,
`ServerHttpSecurity`, or `pathMatchers`. Confirmed by A/B against the *same* official
sample in both flavours: the servlet version reports `SCOPE_message:read` /
`SCOPE_message:write` correctly; the reactive twin reports nothing for either.

In a `pathMatchers`-only application this fails loudly and in the safe direction (the
mutating endpoint gets flagged). Combined with `@PreAuthorize` — the ordinary case in
reactive applications — it becomes case B: the method layer is reported as the whole
story, and over-reports.

### What the three share

In all three, something that could not be analyzed was recorded as something that
was not there. An unreadable matcher became "matches everything." An unread layer
became "no such layer." Neither is a guess the code knows it is making, which is
why nothing warns.

The distinction that matters, and that the current code cannot express:

- **absent** — genuinely no such rule/matcher/layer. A method-security-only Spring
  app has no URL layer, and the method layer really is the complete picture.
- **unknown** — it exists and could not be read. The complete picture is not
  available, and any answer derived as though it were is unearned.

## Decision

### The principle

**A rule, matcher, or layer that Sphinxor could not analyze is recorded as
*unknown*, never as *absent*, and never as a permissive default. Any result derived
from an incomplete picture must say so.**

This is ADR 0018's correction stated at the level it should have been stated
originally. It governs the three cases below and is intended to pre-decide the next
member of this family rather than waiting for it to be found in an audit.

### §1 An unreadable matcher is an unrecognized rule, not a universal one

`nil` patterns stops doubling as the `anyRequest()` sentinel. A `requestMatchers(...)`
call from which no pattern could be extracted becomes the existing
`chainUnrecognized` kind, which ADR 0018 already gives the correct behavior: it is
opaque, it may match, so evaluation stops and the endpoint's URL layer is
unresolved. `anyRequest()` keeps its own explicit representation.

Conservative, because an unreadable matcher *might* match any path — so every
endpoint reaching that rule has an unknown URL-layer outcome rather than a
confidently wrong one.

**Paired with widening what can be read**, so "unknown" stays rare rather than
becoming the common answer: the wrapper forms that triggered this — `antMatcher("…")`,
`AntPathRequestMatcher.antMatcher("…")` — carry a perfectly readable string literal
one call deeper, and are recognized. Genuinely unreadable matchers (regex, custom
beans) resolve to unknown. Fixing the dangerous default without this would trade a
silent wrong answer for a large, honest, and needless loss of coverage on idiomatic
configurations.

### §2 An existing-but-unanalyzable URL layer is recorded as unknown

Presence of a URL layer is detected separately from parsing it. Detection alone is
what converts silent into loud, and it is cheap: the tokens are `SecurityFilterChain`
and `SecurityWebFilterChain`.

- **No URL layer found at all** → absent. The method layer is complete. Current
  behavior is correct and must not regress: a method-security-only application must
  not start reporting uncertainty it doesn't have.
- **A URL layer found but not analyzable** — more than one `SecurityFilterChain`
  bean, zero parseable ones, or a reactive `SecurityWebFilterChain` (§3) — →
  **unknown**, recorded on the model, not silently skipped.

Consumers then differ, matching what each output already means:

- `sphinxor lint`'s matrix is an inventory, so it still shows the method-layer roles
  it found, plus a prominent warning that the URL layer exists and was not analyzed,
  so the effective policy may be narrower than shown.
- `sphinxor export cerbos` emits **grants**, so per ADR 0009 §3 it must not export a
  method-layer-only rule as though it were the effective policy. Those endpoints are
  omitted and flagged in the companion report. Exporting the broader set is guessing
  in the permissive direction, which that ADR forbids.

### §3 Reactive chains are detected, and parsing them is explicitly out of scope here

`SecurityWebFilterChain` / `ServerHttpSecurity` presence is detected and feeds §2's
unknown state. Actually parsing `pathMatchers(...)` is **not** part of this ADR —
that is new framework coverage, and it belongs in its own decision with its own
real-fixture validation. This ADR's obligation is only that a reactive application
stops being told a method-layer-only answer is complete.

### §4 Project-level uncertainty is stated, not resolved

Two further audit findings are *not* wrong answers; they are correct answers
presented with more confidence than earned. Both keep their current analysis and
gain a project-level warning, using the notice mechanism
[ADR 0019](0019-cli-framework-selection.md) §2 already established (stderr, so
`--format json` on stdout stays clean).

- **Method-security annotations found, no `@EnableMethodSecurity` located.**
  `isConfirmedInert` deliberately treats `Found == false` as "no evidence either
  way," because the enabling configuration can live in a parent module or unparsed
  Kotlin. That reasoning stands and does not change. But the consequence today is
  that `@PreAuthorize("hasRole('ADMIN')")` on a wide-open endpoint is reported as
  `Roles: ADMIN` with zero findings. The warning says the annotations were found,
  the enabling configuration was not seen, and if it is not enabled elsewhere these
  annotations are inert and the endpoints are unprotected.
- **NestJS `APP_GUARD` provider detected.** On the inverted-default pattern NestJS's
  own documentation recommends — a global guard protecting everything, `@Public()`
  opting out — endpoint-level results systematically understate protection. Verified
  on `nestjs/nest`'s official `19-auth-jwt` sample. The direction is safe, but the
  distortion is silent; the warning says a global guard was registered and
  endpoint-level results may understate protection.

Neither warning changes a finding or a grant. They change what the user believes the
output means, which is the whole point.

## Alternatives considered

- **Three separate ADRs, one per defect** — rejected, and this is the substantive
  choice in this document. The audit's actual finding is that ADR 0018's lesson was
  applied to the case that revealed it rather than to the principle behind it.
  Writing three patches would repeat exactly that mistake and leave the fourth
  member of the family — whatever it turns out to be — undecided.
- **Treat an unreadable matcher as matching nothing** (skip, continue) — rejected:
  that is ADR 0012 §1's original error, re-entered through a different door. A
  skipped rule lets evaluation reach a later, more permissive one.
- **Keep exporting method-layer-only rules when the URL layer is unknown, with a
  warning in the report** — rejected. A warning in a companion document does not
  make a deployed policy less permissive, and ADR 0009 §3 already settled that
  omission is the safe state because Cerbos denies by default.
- **Treat "no `@EnableMethodSecurity` found" as confirmed-inert** (downgrade the
  guards) — rejected, unchanged from ADR 0015: it would produce false positives on
  correctly configured applications whose enabling config is outside the scanned
  tree. The defect is the missing signal, not the analysis.
- **Parse reactive `pathMatchers` now** — rejected for this ADR as scope creep;
  detection is what closes the silent failure, parsing is a coverage feature.

## Consequences

- `internal/extract/spring` gains a distinct representation for an unreadable
  matcher and for an unknown URL layer; `internal/export/cerbos` gains the
  corresponding omission reason. Both are additive to the model.
- **Coverage will visibly drop in places, and that is the intended outcome**: some
  endpoints that today carry confident roles will become unresolved-and-flagged.
  Every such case is one where the previous answer was not actually earned. The
  `antMatcher` recognition in §1 keeps this from being gratuitous.
- Real-fixture validation: the audit's reproductions become regression tests —
  the `antMatcher`/regex match-all case, the two-chain export divergence, and the
  reactive/servlet A/B on the same upstream sample. Each must be confirmed to fail
  against today's behavior before being kept, the standard set by ADR 0014's
  regression test and reaffirmed for ADR 0018's.
- `docs/limitations.md` is updated: the multi-chain and non-Ant-matcher entries stop
  describing silent gaps and start describing loud ones; a reactive entry is added.
- A dedicated NestJS blind-spot hunt across several real repositories remains
  outstanding. The audit behind this ADR probed Spring considerably harder, and
  given the hit rate there, parity should not be assumed.
