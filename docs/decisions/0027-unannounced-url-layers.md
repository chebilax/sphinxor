# 0027. Two more URL layers are detected and announced

## Status

Accepted.

## Context

[ADR 0020](0020-unanalyzable-is-unknown-not-absent.md) §2 established that a URL
layer which exists and cannot be read is recorded as *unknown*, not absent, and
announced. It detects two forms: more than one `SecurityFilterChain` bean, and a
reactive `SecurityWebFilterChain`.

Two more exist in real code and are announced by nothing.

| Form | Repositories | Announced today |
|---|---:|---|
| `extends WebSecurityConfigurerAdapter` | **1** — nakadi | no |
| `@Bean ShiroFilterFactoryBean` | **6** — JeecgBoot, shenyu, streampark, inlong, litemall, metersphere | no |

Both were recorded as gaps by the Spring survey in `docs/limitations.md`; neither has
had a decision written.

### Why now, rather than with the decision that needs it

[ADR 0026](0026-requestmapping-method-attribute.md) is Accepted and would add 184
`mutating-endpoint-without-access-control` findings across the corpus. **110 of them
land on inlong and 22 on nakadi — the two projects whose URL layer nothing
announces.**

Those findings are not wrong about what they measured: no per-endpoint authorization
was found. But with no caveat attached, a reader takes "no access control" to mean
"anyone can call this", and for inlong that is false. Its chain was read by hand for
ADR 0026:

```java
pathDefinitions.put("/api/anno/**/*", "anon");   // login, register
pathDefinitions.put("/doc.html", "anon");        // swagger
// …
pathDefinitions.put("/**", genFiltersInOrder(FILTER_NAME_WEB, FILTER_NAME_TENANT));
```

Of its mutating `@RequestMapping(method = …)` routes, **114 fall under that `/**`
catch-all and zero are `anon`**. Every one requires an authenticated, tenant-scoped
subject.

So this decision lands first. It is the same sequencing as
[ADR 0023](0023-third-party-authorization-annotations.md) before
[ADR 0024](0024-controller-meta-annotations.md): qualify the findings before
surfacing them, rather than surfacing them and qualifying afterwards.

### Shiro is out of scope, and announcing it is not a scope change

The objection to anticipate: Sphinxor supports Spring Security, and
`ShiroFilterFactoryBean` is not Spring Security.

ADR 0023 already answered the same objection for Shiro's *annotations*, and the
answer here is the narrower half of it: **nothing is parsed**. This decision does not
read a filter chain definition, does not resolve `anon` against a path, and does not
learn what any endpoint requires. It records that an authorization layer exists and
was not analyzed — which is a fact about Sphinxor, not a claim about Shiro.

"Shiro's URL layer is unsupported" stays true. What changes is that a project with
one stops being reported as though it had none.

### One thing the measurement corrected

A first count by grep said shenyu had a reactive `SecurityWebFilterChain` that ADR
0020 §3 was failing to announce. It does not. The type appears in shenyu only in an
`import` and a `@ConditionalOnClass({… SecurityWebFilterChain.class …})`; no `@Bean`
returns one, and existing detection is correctly silent.

Recorded because the near-miss is the argument for §1's shape: detection matches a
**declaration**, never a mention. A token-presence check would have invented a
reactive URL layer for shenyu out of an import.

## Decision

### §1 Both forms are detected by declaration, not by mention

- **`WebSecurityConfigurerAdapter`**: a class whose `extends` clause names it. This
  is Spring Security's own pre-5.7 configuration base class — squarely in
  [ADR 0011](0011-spring-second-framework.md)'s scope, simply an older API than the
  `SecurityFilterChain` bean that replaced it.
- **`ShiroFilterFactoryBean`**: a `@Bean`-annotated method whose return type is that
  class — the same shape `isChainBeanOfType` already matches for
  `SecurityFilterChain`, reused rather than reimplemented.

Neither is parsed. Detection costs the token and nothing else, which is ADR 0020
§2's own argument for separating detection from parsing.

### §2 They produce the existing unknown-URL-layer state, with their own reason

Both set `URLLayer{Present: true, Analyzed: false}` — the state ADR 0020 §2 already
defined — so every consumer behaves as it does for a multi-chain project today, with
no new code path: `sphinxor lint` warns that the roles shown come from the method
layer alone and may be broader than the application enforces, and
`sphinxor export cerbos` omits every endpoint rather than exporting a
method-layer-only policy.

Each gets its own `Reason`, because the sentence a reader needs differs:

- `WebSecurityConfigurerAdapter` — a pre-Spring-Security-5.7 URL layer whose
  `authorizeRequests` rules are not parsed. nakadi's additionally use
  `.access(hasScope(…))`, which [ADR 0012](0012-securityfilterchain-effective-policy.md)
  puts out of scope even in the modern API.
- `ShiroFilterFactoryBean` — an Apache Shiro URL layer, not Spring Security, whose
  filter chain definitions are not parsed. The reason says Shiro by name, because
  "your URL layer could not be analyzed" sends a reader looking for a
  `SecurityFilterChain` they do not have.

### §3 A project with more than one unreadable layer says so once, naming all of them

`URLLayerStatus` holds a single `Reason`, and JeecgBoot has **both** a reactive
`SecurityWebFilterChain` (announced today) and a `ShiroFilterFactoryBean` (not). The
existing code assigns one reason by a `switch`, so a naive addition would either
overwrite JeecgBoot's current reason or be shadowed by it.

Neither is acceptable: both layers are real and unread. The reason becomes a list of
what was found, joined into one warning. One project, one warning, every unreadable
layer named.

### §4 Detection does not fire where a layer was successfully read

`Present && Analyzed` is unchanged: RuoYi-Vue has exactly one `SecurityFilterChain`
whose rules parse, and it must stay quiet. So must the vendored `Pharmacy` fixture,
whose chain is the positive control for the whole URL-layer feature.

A warning that fires on healthy projects is the failure ADR 0020 Amendment 2 §8
named when it conditioned its own warning on guards actually differing. The
noise-floor test (`TestAnalyzeDirectory_RealProjectStaysQuiet`) is the existing pin
on this and must pass unchanged.

## Alternatives considered

- **Parse the Shiro filter chain.** Rejected, and it is worth being explicit since
  inlong's chain was read by hand for ADR 0026 and is not complex. Reading one
  project's chain is not the same as parsing the form: `getShiroFilter` is delegated
  through an interface to `InlongShiroImpl`, the definitions are built imperatively
  into a `LinkedHashMap` across several statements, and the filters themselves are
  arbitrary classes. That is the array-push composite shape ADR 0006 declined, in
  another language.
- **Announce Shiro only when Shiro annotations were also found.** Rejected: the URL
  layer is the thing that protects endpoints carrying no annotation at all, which is
  exactly the population at risk.
- **Treat `WebSecurityConfigurerAdapter` as parseable**, since `authorizeRequests`
  resembles `authorizeHttpRequests`. Rejected as out of scope here: it is a real
  option, but it is a parsing decision needing its own evidence about how many
  chains use readable matchers, and nakadi's use `.access(…)` which ADR 0012 excludes
  regardless.

## Consequences

- No model change. `URLLayerStatus` keeps its three fields; `Reason` carries a joined
  list.
- Six repositories begin warning, and their Cerbos exports begin omitting endpoints —
  a real behaviour change for projects that were silently exporting method-layer-only
  policies. That is the point: ADR 0020 §2's own reproduction showed a two-chain
  project exporting a grant the application denied.
- ADR 0026 can then land with its findings correctly qualified on inlong and nakadi.
### Validation — measured after implementation

**The warning now fires on exactly six more repositories**, and on nothing else:

| Repository | before | after | form |
|---|---|---|---|
| nakadi | silent | **warns** | `WebSecurityConfigurerAdapter` |
| shenyu, streampark, inlong, litemall, metersphere | silent | **warns** | `ShiroFilterFactoryBean` |
| JeecgBoot | warns (reactive) | warns (**both**) | reactive + Shiro |

JeecgBoot's reason reads *"a reactive SecurityWebFilterChain was found …; and an
Apache Shiro ShiroFilterFactoryBean was found …"* — joined, not replaced, which is
§3 working.

**RuoYi-Vue stays silent**, correctly: its single `SecurityFilterChain` parses, so it
is `Present && Analyzed`. Endpoint and finding counts are **unchanged across all 14**
repositories, all four vendored fixtures are byte-identical, and
`TestAnalyzeDirectory_RealProjectStaysQuiet` passes unchanged.

### The Cerbos export effect, measured per repository

Extending ADR 0020 §2's omit-everything behaviour to seven more projects is a visible
change in principle. Measured, **it is a no-op on rule counts**:

| Repository | rules before | rules after | omission reason before → after |
|---|---:|---:|---|
| nakadi | 0 | 0 | `no-guard` → `url-layer-unknown` (1) |
| JeecgBoot | 0 | 0 | `url-layer-unknown` (587), unchanged |
| shenyu | 0 | 0 | `no-guard` (253) + `route-collision` (118) → `url-layer-unknown` (371) |
| streampark | 0 | 0 | `no-guard` → `url-layer-unknown` (232) |
| inlong | 0 | 0 | `no-guard` → `url-layer-unknown` (159) |
| litemall | 0 | 0 | `no-guard` → `url-layer-unknown` (213) |
| metersphere | 0 | 0 | `no-guard` → `url-layer-unknown` (1050) |

**Every one of the seven already exported zero rules**, because none has a Spring
Security guard that resolves to a role — their authorization is Shiro, or OAuth2
scopes, or a filter chain. So nothing that was being exported stops being exported.

What changes is the *reason* recorded against each omitted endpoint, and it changes
to the more accurate one: these endpoints are not omitted because nothing guards
them, they are omitted because the layer that does could not be read. A reader
auditing the export report was previously told `no-guard` about 1,655 endpoints in
six projects that all have a URL layer.

**The stated reason was false before, and that is the substantive result.** Nothing
was exported wrongly — 1,655 endpoints were correctly omitted — but each was labelled
`no-guard`, in six projects that every one of them has an authentication layer. The
export report was telling a reader "no access control was detected for this endpoint"
about endpoints sitting behind a Shiro filter chain or an OAuth2 scope check. The
omission was right; the explanation for it was not.

### Does anything become invisible? Checked, because one reason per endpoint masks another

shenyu's 118 endpoints moved from `route-collision` to `url-layer-unknown`, and an
omitted endpoint carries one reason, so the collision no longer appears in the export
report. Whether that loses information was verified rather than assumed:

- **The model keeps it.** shenyu still records **43 `RouteCollision`s** after this
  change, unchanged.
- **ADR 0020 Amendment 2 §8's warning is independent of the URL layer.** Confirmed on
  a synthetic project declaring both a `ShiroFilterFactoryBean` and a guard-differing
  collision: both warnings fire, neither suppressing the other.
- **shenyu's own collisions do not warn, and did not before.** All 43 have
  non-differing guards — they are `shenyu-examples/` demo applications — which is
  exactly the case §8 deliberately stays quiet about.
- **The endpoints are omitted either way.** A collision omits unconditionally and so
  does an unknown URL layer, so the export's safety behaviour is byte-identical.

What changed is which of two applicable labels the report prints, and it now prints
the project-wide one that would have omitted the endpoint regardless. No collision
information is lost from the model, the warnings, or the export's behaviour.

Tests, in `internal/extract/spring/urllayer_detection_test.go`: each form detected
and named; a *mention* of either type in an `import` and a `@ConditionalOnClass`
detected as nothing, pinning the shenyu near-miss; two unreadable layers naming both;
and a parseable single chain staying `Analyzed` and quiet.