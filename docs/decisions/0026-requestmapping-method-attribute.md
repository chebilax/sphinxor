# 0026. `@RequestMapping(method = …)` declares routes, and three more HTTP methods exist

## Status

Proposed.

## Context

[ADR 0011](0011-spring-second-framework.md) §1 recognizes `@GetMapping` and its four
siblings, and names the older form as a deliberate scope cut:

> `@RequestMapping(method = {RequestMethod.GET, RequestMethod.POST})` … is
> deliberately not handled here — none of the vendored fixtures use it, and per this
> project's standing discipline, extraction logic isn't built ahead of real evidence
> that it's needed.

The evidence now exists. Measured across the 20-repository Java corpus, counting
method-level declarations and expanding each `method = {…}` list into the routes it
really declares:

| Repository | routes recovered |
|---|---:|
| `jeecgboot/JeecgBoot` | 268 |
| `apache/inlong` | 156 |
| `thingsboard/thingsboard` | 93 |
| `zalando/nakadi` | 45 |
| `microcks/microcks` | 17 |
| `alibaba/nacos` | 6 |
| `metersphere/metersphere` | 3 |
| **20-repo total** | **588** |
| *14-repo Spring corpus* | *429* |

It also means **the one repository where the model works is only partly seen**:
thingsboard mixes both shapes, so `POST /api/customer` — carrying
`@PreAuthorize("hasAuthority('TENANT_ADMIN')")`, plainly readable — is absent from
its model entirely.

### This figure has been revised twice, and the reasons are worth recording

`docs/limitations.md` has carried three numbers for this gap. Stating why guards
against treating the next one as settled:

- **381** — wrong twice over: it counted eladmin's five and microcks's four
  *meta-annotation declarations* as route declarations, and it counted annotation
  occurrences rather than routes, so `method = {GET, POST}` counted once.
- **415 / 593** — controller-scoped and verb-expanded, but measured before
  [ADR 0025](0025-qualified-annotation-names.md) made microcks's fully-qualified
  controllers visible, and without requiring the annotation to sit at method level.
- **429 / 588** — the figures above: fully-qualified controllers included,
  class-level `@RequestMapping` excluded.

Per ADR 0024's note on prediction accuracy, this is a source-level scan approximating
what extraction will do, not a count of a quantity both compute identically. It
should be expected to move, and the validation below is the authority.

### Most of it is already absorbed

Of the 339 mutating routes among them, **155 need no finding at all**: 44 carry a
Spring Security guard (all thingsboard) and 111 carry an Apache Shiro annotation that
[ADR 0023](0023-third-party-authorization-annotations.md) already recognizes. That
leaves **184** new `mutating-endpoint-without-access-control` findings across 20
repositories, 74 of them in the 14-repository Spring corpus.

This is the third decision in a row where the sequencing paid: had this landed before
ADR 0023, 111 of those findings would have been raised against Shiro-protected code.

### inlong's 110, checked before designing rather than after

inlong supplies the largest single block of new findings, so its endpoints were
examined directly rather than counted.

**They are authenticated, by a layer this extractor does not read.**
`InlongShiroImpl.getShiroFilter` builds a `ShiroFilterFactoryBean` whose chain ends
in a catch-all:

```java
pathDefinitions.put("/api/anno/**/*", "anon");   // login, register
pathDefinitions.put("/doc.html", "anon");        // swagger
// …
pathDefinitions.put("/**", genFiltersInOrder(FILTER_NAME_WEB, FILTER_NAME_TENANT));
```

`FILTER_NAME_WEB` is an `AuthenticationFilter`; `FILTER_NAME_TENANT` a
`TenantAuthenticatingFilter`. Every path not explicitly `anon` requires an
authenticated, tenant-scoped subject.

Classifying inlong's mutating `@RequestMapping(method = …)` routes against that
chain: **114 fall under the `/**` catch-all, 20 are additionally Shiro-annotated, and
zero are `anon`.** Not one of them is reachable without authenticating.

So the honest statement is the one ADR 0024 made for shenyu, and it can be made more
strongly here because the chain was read rather than inferred: **these are endpoints
with no per-endpoint authorization, in an application that authenticates every
request by default.** `mutating-endpoint-without-access-control` is not wrong that
no *authorization* was found; it is wrong if read as "anyone can call this".

**And this is not specific to inlong.** Every repository contributing new findings
has a URL layer this extractor does not read: JeecgBoot a reactive
`SecurityWebFilterChain`, nakadi a pre-5.7 `WebSecurityConfigurerAdapter` with
`.access(hasScope(…))`, thingsboard and nacos multiple `SecurityFilterChain` beans.
[ADR 0020](0020-unanalyzable-is-unknown-not-absent.md) §2 already warns on the last
two; the Shiro and legacy-adapter cases warn about nothing, which
`docs/limitations.md` records.

## Decision

### §1 A method-level `@RequestMapping` with an explicit `method` attribute declares routes

`@RequestMapping(value = "/x", method = RequestMethod.POST)` is an endpoint, read
exactly as `@PostMapping("/x")` is: same path resolution, same class-level base path,
same version attribute, same guard attachment.

`method = {RequestMethod.GET, RequestMethod.POST}` expands into **one `Endpoint` per
verb**, which ADR 0011 §1 already specified as the required behaviour —
"extraction must expand this into multiple `Endpoint` rows (one per verb, same
handler)" — and which the corpus genuinely exercises rather than merely permitting:
**532 annotations yield 588 routes, because 40 of them declare more than one verb.**
An implementation that read only the first verb would be wrong 40 times and look
right in aggregate.

Scope, stated because each exclusion is a real shape in the corpus:

- **An explicit `method` attribute is required.** A method-level `@RequestMapping`
  without one maps *every* HTTP verb in Spring. There are **164** of those across 17
  repositories, and representing them is a different question — see §4.
- **Class-level `@RequestMapping(method = …)`**, which constrains every handler in
  the controller, is not read as a verb source. It continues to supply the base path
  only.

### §2 `HEAD`, `OPTIONS` and `TRACE` join `HTTPMethod`

`model.HTTPMethod` has five constants. The corpus declares three more: `HEAD` (4),
`OPTIONS` (4), `TRACE` (2). Ten routes — small, and a model decision rather than an
extraction one, so it is argued rather than assumed.

**They are added.** Three reasons, in order of weight:

1. **A declared endpoint is a fact.** The application routes those verbs to a
   handler. Dropping them would make the matrix silently incomplete for a project
   using them — the failure class this project has spent five decisions removing —
   and skip-and-say would be a warning about ten routes that could simply be shown.
2. **Nothing downstream needs teaching.** [ADR 0009](0009-cerbos-exporter.md)'s
   action is the lowercased HTTP method, so `head`, `options` and `trace` are valid
   Cerbos actions the moment the constants exist. Endpoint identity is
   `(method, path)` and stays unique. The markdown matrix prints the string.
3. **`isMutating` is unaffected, deliberately.** None of the three is added to
   `internal/lint/mutating_endpoint.go`'s mutating set: `HEAD` and `OPTIONS` are
   safe by specification, and `TRACE` — though it has its own history as a
   cross-site-tracing vector — does not mutate state, which is what that rule is
   about. Recording them changes what the matrix shows, not what the rules claim.

The alternative, skipping them with a warning, was rejected on the arithmetic: it
costs a model constant to represent them and a project-level warning to refuse to,
and the refusal is the one that has to be explained to a reader.

### §3 What this does not change

Nothing about authorization. A route recovered here carries whatever its handler
already declares — a Spring guard, a Shiro annotation recorded as unrecognized
(ADR 0023), or nothing — under exactly the rules that already apply. The 184 new
findings are the ordinary consequence of endpoints becoming visible, not a new claim
about them.

In particular, per the Context: **every repository gaining findings here has a URL
layer this extractor cannot read**, and inlong's was read by hand and authenticates
every non-`anon` path. `docs/limitations.md` carries that caveat in its body, as it
does for shenyu.

### §4 A verb-less method-level `@RequestMapping` stays out, and is recorded

164 method-level `@RequestMapping` annotations across 17 repositories declare no
`method` attribute. In Spring each maps **all** HTTP verbs.

Representing one faithfully means either eight `Endpoint` rows per handler — of which
the developer probably intended one or two, and which would put a `DELETE` in the
matrix for every such handler — or a new "any verb" concept in `HTTPMethod`, which is
a model change with consequences for identity, the exporter and `isMutating`.

Both are decisions this ADR deliberately does not make, because bundling a second
model question with §2 is how a focused decision becomes a two-headed one. The shape
is newly measured and is recorded in `docs/limitations.md` with both options and no
verdict.

## Alternatives considered

- **Expand a verb-less `@RequestMapping` to all verbs now.** Rejected per §4, and
  not because it is wrong — Spring really does route every verb there — but because
  it is a model decision that deserves its own measurement of what those handlers
  actually accept.
- **Skip `HEAD`/`OPTIONS`/`TRACE` with a warning.** Rejected per §2.
- **Add the three verbs to `isMutating`.** Rejected: none mutates state, and the
  rule is about state change, not about risk in general.
- **Read class-level `@RequestMapping(method = …)` as constraining every handler.**
  Out of scope here; unmeasured, and it interacts with §1's expansion in ways that
  deserve their own evidence.

## Consequences

- `model.HTTPMethod` gains three constants. No entity, relationship or export format
  changes.
- Endpoint counts rise substantially in six repositories; `docs/limitations.md`'s
  route-shapes entry loses this bullet and keeps the two that remain
  (interface-inherited routes, method-level mapping meta-annotations).
- The two meta-annotation families remain unresolved. This decision is **necessary
  but not sufficient** for them: zero handlers carrying an eladmin
  `@Anonymous*Mapping` also carry a literal `@RequestMapping`, so each still needs
  method-level meta-annotation resolution, and shenyu's additionally needs depth 2.
- **Validation before this is Accepted**, against the real corpus per `docs/testing.md`.
  The per-repository prediction, to be checked and any gap explained rather than
  absorbed:

  | Repository | endpoints | mutating findings |
  |---|---|---|
  | JeecgBoot | 587 → ~855 | 170 → ~213 |
  | inlong | 159 → ~315 | 74 → ~184 |
  | thingsboard | 454 → ~547 | 4 → ~10 |
  | nakadi | 1 → ~46 | 1 → ~23 |
  | microcks | 80 → ~97 | 31 → ~34 |
  | nacos | 422 → ~428 | 3 → ~6 |
  | metersphere | 1050 → ~1053 | 84 → 84 |

  - **No other repository changes in any respect**, and all four vendored fixtures
    stay byte-identical — none uses the `method =` form.
  - thingsboard's `POST /api/customer` appears, carrying `TENANT_ADMIN` from its
    existing `@PreAuthorize`: the single clearest check that a recovered route keeps
    its authorization.
  - A test that `method = {GET, POST}` yields two endpoints sharing one handler.
  - A test that `HEAD`, `OPTIONS` and `TRACE` are extracted and that **none** of them
    produces a `mutating-endpoint-without-access-control` finding (§2.3).
  - A test that a verb-less method-level `@RequestMapping` still yields no endpoint
    (§4), so the exclusion is pinned rather than incidental.
