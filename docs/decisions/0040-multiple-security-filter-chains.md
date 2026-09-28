# 0040. More than one `SecurityFilterChain`: what the corpus shows, and the narrow case that can be read

## Status

**Proposed — a measurement ADR.** It records how the six corpus repositories with several
`SecurityFilterChain` beans actually use them, the Spring facts that decide which chain
serves a request, and a proposed scope. It amends nothing yet. If accepted, it narrows
[ADR 0020](0020-unanalyzable-is-unknown-not-absent.md) §2, which records any project with
more than one chain as having an unknown URL layer.

It follows [ADR 0039](0039-route-path-constants.md), because a chain is selected by URL
path. The measurement below uses ADR 0039's readable paths.

**Read the yield before accepting (§5).** Selection itself turns out to be the smaller
problem. The dominant blocker is conditional activation, whether by annotation or by a
branch in code, and **the proposed scope recovers none of the six repositories**. The
recommendation (§6) follows from that.

## Context

### What is at stake

Measured on 2026-09-28, at the commits in [ADR 0035](0035-permissions-in-the-model.md) §9.
Six of the 20 corpus repositories have more than one chain:

| Repo | Chains | Endpoints | Rows with roles |
|---|---:|---:|---:|
| thingsboard | 4 | 551 | 519 |
| nacos | 5 | 434 | 0 |
| apollo | 6 | 224 | 0 |
| spring-cloud-dataflow | 2 | 113 | 0 |
| microcks | 2 | 97 | 0 |
| fineract | 5 | 1 | 0 |

That is 1,420 endpoints whose URL layer is unknown, so the Cerbos export omits all of them.
thingsboard is the only corpus repository with role-bearing rows at all.

### How the 24 chains are declared

Each chain bean was read with the extractor's own helpers, and the conditions by hand:

| Property | Count |
|---|---:|
| Chain beans | 24 |
| carrying `@Profile` or `@Conditional…` (on the method or its class) | **16** |
| with **no** `securityMatcher`, so they match every request | 16 |
| with a method-level `@Order` | 11 |
| `securityMatcher` of string literals | 4 |
| `securityMatcher` of a constant, a builder, a lambda, or a runtime value | 4 |

Per repository:

- **apollo.** Six chains across three applications in one tree: `apollo-biz` (a library),
  `apollo-configservice` and `apollo-portal`. Four portal chains are mutually exclusive:
  `@Profile("auth")`, `@Profile("ldap")`, `@Profile("oidc")`, and
  `@ConditionalOnMissingProfile({"auth", "ldap", "oidc"})`. Three chains share
  `@Order(99)`.
- **fineract.** Five chains in one application, selected by properties:
  `fineract.security.basicauth.enabled`, `fineract.security.oauth2.enabled` (three
  chains, `@Order(1)` to `@Order(3)`) and `...oidc-federation...`. One matcher is
  `authorizationServerConfigurer.getEndpointsMatcher()`, a runtime value, and one is a
  builder, `API_MATCHER.matcher("/api/**")`.
- **nacos.** Five chains across five modules composed into one server. Four are gated by
  `@Conditional(...)` classes or `@ConditionalOnMissingBean`, whose outcome depends on
  the configured auth plugin.
- **spring-cloud-dataflow.** Two chains in **two different servers** in one tree, the
  Data Flow server and the Skipper server. Both carry
  `@Conditional(OnOAuth2SecurityEnabled.class)`.
- **thingsboard.** Four chains:
  - `resources`, with method-level `@Order(0)` and a lambda
    `securityMatchers(m -> m.requestMatchers("/*.js", ...))`;
  - a transport chain with `@Order(1)` and a constant matcher;
  - the main chain, with no method-level `@Order` and no matcher, whose rules name their
    paths through constants (`requestMatchers(TOKEN_BASED_AUTH_ENTRY_POINT)`);
  - a rule-engine chain gated by
    `@ConditionalOnExpression("'${service.type:null}'=='tb-rule-engine'")`, active only in
    one deployment mode.

  `@Order(SecurityProperties.BASIC_AUTH_ORDER)` appears on three **classes**, where it
  orders nothing (§2).
- **microcks.** Two chains in one class, neither annotated as conditional and neither
  with `@Order`. The first has
  `securityMatcher("/rest/**", "/rest-valid/**", "/graphql/**", "/soap/**")` and permits
  everything. The second has no matcher and configures its rules **inside a code branch**:
  `if (Boolean.TRUE.equals(keycloakEnabled))` sets about a dozen role rules, and the
  `else` permits everything. `keycloakEnabled` is `private final Boolean keycloakEnabled
  = true;`. The roles are constants through `import static ...AuthorizationChecker.*`,
  which ADR 0039's index resolves.

### §2 The Spring facts that decide which chain serves a request

Verified at source, not taken from documentation:

- **Order is the order of an injected `List<SecurityFilterChain>`**
  (`WebSecurityConfiguration.setFilterChains`, Spring Security 6.5.11). Spring
  Framework sorts that list with its order-source provider, which honours the
  **`@Order` on the `@Bean` factory method** and the bean definition's order attribute
  (`DefaultListableBeanFactory.FactoryAwareOrderSourceProvider`, Spring Framework
  6.2.11). **A class-level `@Order` on the `@Configuration` class is not an order source
  for the chains it produces.** Beans without one sort as lowest precedence, then in
  registration order.
- **The first chain whose matcher matches serves the request**, and only its rules apply.
  A chain without a `securityMatcher` matches every request.
- **"A match-everything chain must be last" is enforced only on recent versions.** Spring
  Security rejects a chain configured after an any-request chain from **6.4** (in
  `WebSecurity`), and from **6.5** through `WebSecurityFilterChainValidator`, which also
  rejects two chains with an equal matcher. 6.3 and earlier start without complaint, and
  the later chain is never invoked.

These are version-bound in the way ADR 0038 §13 describes, and they are re-checked on
every major version.

### §3 A defect this measurement found in the single-chain reader

A chain rule whose role argument is not a string literal —
`.requestMatchers("/admin/**").hasAnyRole(ROLE_ADMIN)` — is **dropped**
(`chainRuleFromTerminal` returns no rule when it reads no role). It should be kept as
unrecognized, which per [ADR 0018](0018-unrecognized-rule-stops-evaluation.md) stops
evaluation. Reproduced: with `.anyRequest().permitAll()` after it, `DELETE /admin/wipe`
is reported with **no guard**, and the URL layer as analyzed.

The error runs in the safe direction: an ADMIN-only endpoint is reported as unprotected,
the lint finding fires, and the export omits the endpoint. But it is a false statement,
and it breaks ADR 0018.

**No analyzed single-chain corpus repository has such a rule today**, so there is zero
current effect. microcks's second chain is exactly this shape, though, so it becomes live
the moment several chains are read. The fix is a prerequisite for anything below: read
the constant through ADR 0039's index, and where that fails, keep the rule as
unrecognized. It is also worth doing on its own, because a false "unprotected" is still
false.

## Decision (proposed)

### §4 Scope: several chains are read only when source settles which one serves each endpoint

Several chains are read together only when **all** of these hold:

1. **No chain is conditional.** No `@Profile` or `@Conditional…` on any chain method or
   its class. Which chains exist then depends on configuration, and a policy built from
   the union would be one no deployment runs.
2. **Every chain's matcher is readable**: absent (matches everything), string literals,
   or constants resolvable under ADR 0039 §2, including `securityMatchers(m ->
   m.requestMatchers(...))` with readable arguments. A builder or a runtime value is
   unreadable.
3. **Each chain configures `authorizeHttpRequests` once, outside any code branch.** A
   chain whose rules sit in an `if`/`else` is decided at runtime, however constant the
   condition looks (microcks's `keycloakEnabled`). Evaluating control flow is a different
   kind of analysis from anything Sphinxor does.
4. **The order is determined**:
   - by distinct method-level `@Order` values;
   - or, where some are unordered, by at most one match-everything chain, which is last.
     Where the order would otherwise rest on registration order, the chains must be
     declared in one class, and the match-everything chain must also be declared last
     there. That is the order Spring would run on 6.3 and earlier, and the only one it
     permits on 6.4 and later.

Each endpoint is then assigned to the first chain whose matcher matches its path and
method. That chain's rules are applied to it exactly as a single chain's are today (ADR
0012, ADR 0018). An endpoint that could match an earlier chain's unreadable rule is
unknown, as a single chain's would be.

When any condition fails, the URL layer stays unknown as today, and the warning names the
**first failing condition**, not just "N chains were found".

### §5 Yield on the corpus

| Repo | Blocked by |
|---|---|
| apollo | conditional chains (1); three applications in one tree |
| fineract | conditional chains (1); a runtime and a builder matcher (2) |
| nacos | conditional chains (1) |
| spring-cloud-dataflow | conditional chains (1); two servers in one tree |
| thingsboard | one conditional chain, rule-engine mode (1) |
| microcks | its rules in an `if`/`else` (3) |

**None of the six repositories is recovered.** Activation decided at runtime is the
blocker in every case: by annotation in five, by a code branch in the sixth. Ordering
(condition 4) blocks none of them on its own, and neither do readable matchers alone.
thingsboard, the repository whose role rows motivated this item, is blocked by one chain
active only in a deployment mode the rest of its configuration does not use.

What every repository gains regardless is the warning naming *why*. For example, *5
SecurityFilterChain beans, 4 of them conditional on `@Profile`/`@Conditional`* replaces
*5 SecurityFilterChain beans were found*.

### §6 Options for the owner

- **A. Accept §4 as scoped.** Fix §3 first, and improve the warning everywhere. Sound,
  but on this corpus the selection logic would run on nothing: it would be machinery
  without a measured beneficiary, the situation ADR 0038 §12 staged around.
- **B. A. plus deployment-mode exclusion.** Treat a chain whose condition can be shown
  *false in the default configuration* as absent, and say so. thingsboard's
  `service.type` defaults to `null` in the expression itself. Rejected as the default:
  "default configuration" is a deployment fact, the ADR 0020 Amendment 2 finding H
  reasoning. It is listed because it is the one extension that would reach thingsboard,
  and only if the owner accepts stating the assumption in the output, as ADR 0038 §12
  does for versions.
- **C. Partition by application.** Assign chains and endpoints to the Maven/Gradle module
  that deploys them. This would separate spring-cloud-dataflow's two servers and apollo's
  three applications. It means reading build files, which ADR 0038 rejected for the same
  cost. It is the natural follow-up if multi-application trees keep appearing.
- **D. Stop at §3 and the better warning; defer §4.** Fix the dropped-rule defect, and
  make the warning name the blocker (*4 of 5 chains are conditional on `@Profile` /
  `@Conditional`*, *the rules are configured inside a code branch*). Keep §4's design
  recorded, and implement it when a project meets its conditions.
- **E. D. plus constant-branch evaluation.** Treat `if (CONSTANT)` in a chain method as
  decided when the condition is a compile-time constant under ADR 0039's index. That
  would reach microcks, but only if `Boolean.TRUE.equals(finalInstanceField)` counts, and
  it does not: an instance field and a method call are outside ADR 0039 §2. Listed so it
  is not rediscovered.

**Recommendation: D**, staged the way ADR 0038 was:
- §3 is a real defect, cheap to fix, and a prerequisite for any multi-chain reading.
- The better warning reaches all six repositories and all 1,420 endpoints today.
- §4 is recorded, so it does not have to be re-derived.

It becomes worth implementing when a project meets its four conditions, or sooner if the
owner accepts B (thingsboard) or C (spring-cloud-dataflow, apollo). Those are each a
decision about stating an assumption in the output, not about parsing.

## Alternatives considered

- **Union of all chains' rules.** Rejected: a request is served by one chain, never
  several, and a union claims restrictions or grants from chains that never run for that
  path.
- **The most restrictive chain for every endpoint.** Rejected: the "safe" direction for
  lint is the unsafe one for the export, which would then grant less than the application
  does. That is not the problem, but it presents a guess as a policy.
- **Evaluate `@Profile` / `@ConditionalOnProperty` from `application.yml`.** Rejected for
  the reason B is: profiles and properties are deployment facts.
- **Trust class-level `@Order`.** Rejected by §2: Spring does not use it for these beans.

## Consequences (if accepted as D)

- `internal/extract/spring/securityfilterchain.go`:
  - §3's fix: a role argument is read through ADR 0039's index, and a rule whose role
    cannot be read is kept as unrecognized, never dropped;
  - chain discovery records, per chain, whether it is conditional, whether its matcher
    is readable, and whether its rules sit in a branch, for the warning.
- `internal/model`: `URLLayerStatus.Reason` names the blocker. No per-endpoint change.
- No new fixture: the §3 fix is pinned by a synthetic test (the corpus has no instance,
  §3), and the warning by the six corpus repositories.
- Regression bar:
  - lint JSON and exports byte-identical on all 20 repositories, since §3 touches none;
  - only the multi-chain warning's text changes, on the six;
  - every fixture unchanged.
