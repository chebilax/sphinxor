# 0027. Two more URL layers are detected and announced

## Status

Proposed.

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
- **Validation before this is Accepted**, against the real corpus per `docs/testing.md`:
  - The warning fires on exactly **nakadi** (`WebSecurityConfigurerAdapter`) and
    **inlong, streampark, shenyu, litemall, metersphere** (`ShiroFilterFactoryBean`),
    and **JeecgBoot's existing reactive warning is joined by, not replaced with**,
    its Shiro one.
  - **No other repository changes in any respect** — in particular RuoYi-Vue, whose
    chain parses, stays quiet.
  - All four vendored fixtures byte-identical, and
    `TestAnalyzeDirectory_RealProjectStaysQuiet` passes unchanged.
  - A test that a *mention* of either type — an import, a `@ConditionalOnClass` —
    does **not** trigger detection, pinning the shenyu near-miss so a token check
    cannot creep back in.
  - A test that a project with two unreadable layers names both (§3).
