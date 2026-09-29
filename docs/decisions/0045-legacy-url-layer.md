# 0045. The pre-6.0 `authorizeRequests()` URL layer: measured, designed, and deferred

## Status

**Proposed — a measurement ADR.** It measures what reading the legacy URL layer would
recover:
- `WebSecurityConfigurerAdapter.configure(HttpSecurity)`;
- the fluent pre-6.0 `authorizeRequests().antMatchers(...)` DSL.

[ADR 0027](0027-unannounced-url-layers.md) §1 announces this layer as unknown, and
[ADR 0038](0038-role-hierarchy-read.md) §12 named reading it as the likeliest way to meet
Stage 2's trigger. It amends nothing yet.

## Context

Every figure comes from the extractor, its chain-rule helpers, or a throwaway prototype of
the reader, per `docs/testing.md`, measured at `c809dc4`.

### Where the layer occurs

| Project | Set | `authorizeRequests()` sites | Rules (role rules) | Unreadable matchers |
|---|---|---:|---|---:|
| nakadi | corpus | 1, in a `ResourceServerConfigurerAdapter` | every rule `.access(hasScope(...))` | 0 |
| eladmin | corpus | 1, fluent, in a `SecurityFilterChain` bean | 16 (0) | 6, runtime `@AnonymousAccess` URLs |
| autoplan | sample | 1, fluent, in an adapter | 17 (3) | 0 |
| YukeSeko-Interface | sample | 1 | 4 (1) | 2 |
| wallride | sample | **2** | 3 (3) | 0 |
| SMSC | sample | 1 | 6 (0) | 0 |
| trackr-backend | sample | 1 (and a second adapter) | 1 (0) | 0 |
| haven-platform | sample | 1 | 3 (0) | 0 |
| molgenis | sample | 1, **through a registry variable** passed to another method | not collectable from one expression | — |

### What a reader would recover (prototype: the fluent chain read with the existing rule machinery)

| Project | URL layer | Rows with roles | Exported rules | Findings |
|---|---|---|---|---|
| **corpus: nakadi** | unknown → unknown | 0 → 0 | 0 → 0 | 26 → 26 |
| **corpus: eladmin** | unknown → unknown | 0 → 0 | 0 → 0 | 20 → 20 |
| autoplan | unknown → analyzed | **0 → 48** | **0 → 29** | 51 → 12 |
| SMSC | unknown → analyzed | 0 → 0 | 0 → 0 | 8 → 8 |
| haven-platform | unknown → analyzed | 0 → 0 | 0 → 0 | 53 → 53 |
| YukeSeko-Interface | unknown → analyzed | 0 → 0 | 0 → 0 | 38 → 38 |
| wallride | unknown → unknown (two sites) | 0 | 0 | 90 → 90 |
| trackr-backend | unknown → unknown (two adapters) | 7 → 7 | 0 | 6 → 6 |
| molgenis | unknown → analyzed — **wrongly** | 19 → 19 | **0 → 12** | 135 → 135 |

- **Corpus gain: zero.**
  - nakadi's rules sit inside `if (authMode == FULL)`, and every one is `.access(...)`,
    which ADR 0018 already reads as unrecognized.
  - eladmin's six runtime matchers precede `anyRequest()`, so every endpoint stays
    unknown (ADR 0020 §1).
  - dolphinscheduler's chain is not this DSL.
- **Sample gain: one application.** autoplan gains 48 role rows and 29 rules. Its
  hierarchy (`ROLE_ADMIN > ROLE_USER`) is readable, and the legacy `authorizeRequests()`
  applies a `RoleHierarchy` bean on every version (ADR 0038 Finding 2). **So autoplan
  alone would meet ADR 0038 Stage 2's trigger.**
- **The prototype over-granted on molgenis.**
  `expressionInterceptUrlRegistry = http.authorizeRequests();` was read as a chain with no
  rules. The URL layer was marked analyzed, and the export granted 12 rules from the method
  layer alone, while the real rules are applied in later statements and in
  `configureUrlAuthorization(...)`. This is ADR 0020 §2's error, recreated by a reader that
  assumes one expression holds the whole chain.

## Decision (proposed)

### §1 The design, if implemented

The legacy layer is read only when **all** of these hold. Otherwise it stays unknown, with
the failing condition named in the warning (ADR 0040 §5's pattern):
1. **exactly one** `authorizeRequests()` site, in an unconditional adapter or chain bean;
2. **the whole rule chain is one fluent expression.** The `authorizeRequests()` result
   is never assigned, passed to a method or returned; molgenis's registry fails here;
3. **no rule sits in a code branch** (nakadi fails here);
4. matchers are `antMatchers`/`mvcMatchers`/`requestMatchers` over literals or
   constants (ADR 0039's index). `regexMatchers` or a runtime value is an unreadable
   matcher, handled as ADR 0020 §1 already does.

Rules then go through the existing machinery unchanged (ADR 0012, ADR 0018, ADR 0040 §3,
ADR 0012 Amendment 1). The legacy-only terminals (`anonymous()`, `fullyAuthenticated()`,
`rememberMe()`, `access(...)`) are unrecognized and stop evaluation.

### §2 Recommendation: defer, staged like ADR 0038 and ADR 0040

Record §1 and implement it when a **corpus** project needs it. The measured corpus gain is
zero. The sample gain is one application, and the one thing autoplan would unlock, ADR 0038
Stage 2, has so far been held to the same bar. The corpus's single adapter project is not
reachable for reasons this reader cannot fix (a branch and `.access(...)`).

What the measurement changes regardless:
- **§1's condition 2 is recorded as a requirement** for any future reader of chained
  configuration, legacy or not: the prototype showed how easily "the chain is one
  expression" turns an unknown layer into an absent one;
- **the corpus skews modern.** One adapter project in 20 against seven in the nine-app
  sample. If the owner wants this measured on more than one application, the lever is a
  corpus extension, not a reader.

## Alternatives considered

- **Implement §1 now.** Rejected on the evidence: no corpus project changes. It would
  move autoplan alone, and with it Stage 2's trigger, on a sample the corpus has not
  confirmed.
- **Read the layer without condition 2**, as the prototype did. Rejected: it over-grants
  on molgenis.
- **Read branched configuration by taking the union of branches.** Rejected for ADR
  0040's reason: a request is served by one branch, never several.

## Consequences

- None in code. `docs/limitations.md`'s legacy-layer entry points here and states the
  corpus gain.
- If §1 is implemented later: autoplan, SMSC, haven and YukeSeko become analyzable, and
  molgenis, wallride, trackr, nakadi and eladmin stay unknown with named reasons. The
  regression bar is those outcomes, and zero corpus change.
