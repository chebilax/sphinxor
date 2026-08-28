# 0012. Simple-pattern SecurityFilterChain parsing and the method×URL effective policy (amends ADR 0011)

## Status

Proposed.

## Context

ADR 0011 deferred `SecurityFilterChain`/`authorizeHttpRequests` entirely, scoping the
first Spring cut to method-level annotations only. Before writing any extraction
code, two real repositories were examined by hand to check that scoping against
real evidence rather than the ADR's own reasoning alone:

- **`categolj/blog-api`** (Apache-2.0, real, actively maintained): 0 of 40 REST
  endpoints use `@PreAuthorize`/`@Secured`/`@RolesAllowed`. `@EnableMethodSecurity(prePostEnabled
  = false)` explicitly disables them; every endpoint's real access control is a
  fully custom, programmatic `AuthorizationManager` bean — a category ADR 0011
  already excluded, and no static analyzer handles well.
- **`Kitty-Hivens/Pharmacy`** (MIT, real, standard practice, visible `ADMIN`/
  `PHARMACIST` RBAC): 19 of 21 endpoints (90%) use `@PreAuthorize`; the other 2 are
  explicit `permitAll()` `SecurityFilterChain` rules. Every rule in this app's
  `SecurityFilterChain` is a plain `.requestMatchers(pattern[, method]).hasRole(...)`/
  `.hasAnyRole(...)`/`.permitAll()` call — no custom `AuthorizationManager`
  anywhere.

**The decision driver is not the coverage percentage — it's a correctness bug found
on the conventional case.** `Pharmacy`'s `SupplierController.GET` carries
`@PreAuthorize("hasAnyRole('ADMIN','PHARMACIST')")`, but the matching
`SecurityFilterChain` rule is `.requestMatchers("/api/suppliers/**").hasRole("ADMIN")`.
Spring AND-combines both interceptors — a `PHARMACIST` passes the method check and
fails the URL check, so the real effective policy is `ADMIN`-only. An extractor that
reads only `@PreAuthorize` (ADR 0011's original scope) would report `ADMIN` **or**
`PHARMACIST` — a confidently wrong, *permissive* answer, on an app that has the
correct information sitting right there in parseable form. That's not a coverage
gap; it's Sphinxor asserting broader access than reality, on the target (annotation-
using, conventional) case ADR 0011 was written for. That disqualifies
annotations-only as correct, not just as incomplete — the reason this needed a
revision rather than a coverage footnote.

`blog-api` staying out of scope is not weakened by this — it confirms the opposite
edge would have been the wrong lesson too: a custom `AuthorizationManager` remains
genuinely unparseable without executing arbitrary Java, and no repo examined asked
Sphinxor to do that.

## Decision

### 1. Simple-pattern `SecurityFilterChain` parsing moves into the first Spring cut

Recognized shape, inside `authorizeHttpRequests(auth -> auth. ...)`:
`.requestMatchers("pattern")` or `.requestMatchers(HttpMethod.X, "pattern")` (also
multiple pattern arguments), followed by exactly one of `.hasRole(X)`,
`.hasAnyRole(X, Y, ...)`, `.hasAuthority(X)`, `.hasAnyAuthority(X, Y, ...)`,
`.permitAll()`, `.denyAll()`, `.authenticated()` — plus the trailing
`.anyRequest().<terminal>()` catch-all Spring's own docs recommend always having.
Path patterns support the bounded, Ant-style set actually seen in real code: literal
segments, `*` (one segment), `**` (any number of segments), `{var}` (one segment,
Spring's path-variable placeholder used as a wildcard here) — not full
`AntPathMatcher`/`PathPatternParser` parity (no regex matchers, no character
classes). Rules are matched against extracted endpoints **first-match-wins over
declaration order**, mirroring Spring's actual runtime evaluation — confirmed
behavior (§1 above), not assumed.

**Not recognized — stays a documented blind spot, per ADR 0011's existing
discipline, extended rather than reopened:** `.access(AuthorizationManager)`
(`blog-api`'s case), any pattern using regex/character-class syntax beyond the
bounded set above, a custom `RequestMatcher` or `mvc.matcher(...)` bean reference,
`dispatcherTypeMatchers`, and multiple `securityMatcher`-scoped
`SecurityFilterChain` beans (real and documented by Spring, not seen in either
repo examined — chain selection by `@Order` and `securityMatcher` is its own
problem, deferred rather than half-solved). A rule using an unrecognized shape is
simply not matched by extraction — the endpoint falls through to the next
recognized rule (or nothing), the same "can't parse it, don't guess" default used
everywhere else in this project, not a new kind of gap.

`permitAll()` contributes no requirement, consistent with ADR 0011 §2's existing
decision that an explicit "public" signal is not Sphinxor's own allowlist by
inference — it simply means this layer has nothing to add to whatever the other
layer (if any) requires.

**New `GuardScope` value: `ScopeRequestMatcher`.** A `GuardApplication` derived from
a recognized `authorizeHttpRequests` rule, additive to the existing `ScopeClass`/
`ScopeMethod` values — no new type. `File`/`Line` point at the rule's location in
the `SecurityConfig` class, not the endpoint's own controller — an honest pointer
to where the evidence actually lives, letting a reviewer jump straight to it.

### 2. The method×URL effective policy: where the combination logic lives, and why there

**Not at extraction time.** Extraction records both layers' `GuardApplication`s and
`RoleReference`s completely and honestly, exactly as it already does for every
other fact — it does not try to pre-resolve one combined answer. Checked against a
real failure mode before deciding this, not assumed safe: if extraction instead
tried to resolve `SupplierController`'s conflict itself and only kept the
`ADMIN`-only result, the method layer's own `GuardApplication` (`DeclaresRoles:
true`, genuinely present in source) would end up with zero attached
`RoleReference`s — and `internal/lint/empty_role.go` would read that as "a role
check was declared with nothing in it," a false positive on code that was never
actually a mistake, just superseded by a stricter layer. Extraction staying purely
descriptive avoids inventing this bug.

**Not by teaching the model general boolean (AND-of-ORs) structure.** The fully
general form of "requires (A or B) and (C or D)" cannot always be losslessly
flattened into one role list — modeling it properly would mean `RoleReference`
gaining real boolean composition, a materially bigger change than this ADR's
"simple pattern" scope, and the same shape of complexity ADR 0009 already declined
to take on for Cerbos's `derivedRoles`/`condition`. Not needed here either: real
disagreement between the two layers, when it isn't cleanly reducible, is exactly
the kind of case this project omits and flags rather than represents awkwardly.

**Lives in `internal/export/cerbos`'s `Translate`, extending logic it already
has.** ADR 0009 already made this function the one place responsible for turning
raw model facts into a safe, non-guessing grant — and it already does
structurally identical reasoning for a related problem: comparing multiple sources
of grant information and either reducing them to one answer when they agree, or
omitting and flagging when they don't (`ReasonActionCollision`, comparing
*sibling endpoints* sharing a Cerbos action). This is that same mechanism, applied
one level deeper — *within* one endpoint, across its guard layers, not just
across endpoints.

Concretely, in `Translate`'s current `rolesByEndpoint` construction (today: union
every `RoleReference` plus every `AuthenticationRequirement` for an endpoint into
one flat grant list, no layer awareness at all): grants are first collected **per
layer** — method-derived (`AppliedAt` is `ScopeClass`/`ScopeMethod`, including
composite-resolved) versus URL-derived (`AppliedAt == ScopeRequestMatcher`) — before
being combined into the endpoint's final grant set:

- If an endpoint has role-bearing (`DeclaresRoles`) `GuardApplication`s in only one
  layer (every NestJS endpoint, and most Spring endpoints today — confirmed: zero
  endpoints in either real repo examined have role-bearing applications in *both*
  layers except `SupplierController`), nothing changes — today's flat union *is*
  already correct, because there's only one set to union.
- If both layers contribute a role-bearing set, **the operation is set
  intersection, not a subset check.** Spring's two interceptors AND-combine: a
  principal must satisfy the method layer *and* the URL layer independently, and
  any role held by a principal that appears in *both* layers' allowed-role sets is
  sufficient to satisfy both, whether or not either set contains the other. `*`
  (`AuthenticationRequirement`, ADR 0010) is the identity element for this
  operation — `*` ∩ *anything* = that thing, so a concrete set always wins over
  "authenticated, any role" on the other side, and `*` ∩ `*` = `*` — reproducing
  exactly the earlier subset-based results for `SupplierController`
  (`{ADMIN, PHARMACIST} ∩ {ADMIN} = {ADMIN}`) and `Pharmacy`'s `CustomerController`
  shape (`not-established` treated as imposing no constraint when the *other*
  layer has one; if *both* are not-established, that's the existing "no guard at
  all" case, unchanged, not treated as `*`).
  - **The intersection is *sound*, not *complete*, and that asymmetry is exactly
    the direction this project already accepts everywhere else: it never claims a
    role is grantable unless a principal holding it is provably allowed by both
    layers, but a principal who separately holds one qualifying role for each
    layer independently (e.g. `ADMIN` and `MANAGER`, for method-set
    `{ADMIN, PHARMACIST}` and url-set `{PHARMACIST, MANAGER}`) can be authorized by
    the real app without holding any role in the intersection at all. The exported
    policy under-grants relative to that edge case rather than over-grants — the
    acceptable kind of incompleteness, not the dangerous kind, and no different in
    kind from composite-decorator resolution stopping at one level (ADR 0006) or
    complex SpEL simply not being parsed (§1) — narrower than perfect, never wrong
    in the permissive direction.
  - **Non-empty intersection**: exported as the effective `Rule`, same as any
    other resolved role set — this is what recovers real, safe answers the
    original subset-only draft of this ADR discarded. `{ADMIN, PHARMACIST} ∩
    {PHARMACIST, MANAGER} = {PHARMACIST}` is an exact, zero-guessing effective
    policy, not an ambiguous case; treating it as unresolved (the original
    subset-check draft's behavior) would have been avoidable coverage loss, the
    inverse of this project's own "omit only when necessary" principle.
  - **Empty intersection** — no role satisfies both layers — is a distinct,
    honestly-scoped signal, not the same bucket as a genuinely unparseable rule: a
    **new `OmissionReason`, `ReasonNoCommonRole`**, `Detail` stating plainly what
    each layer required and that they share no role. Deliberately *not* worded as
    "this endpoint is unreachable" or "denies everyone" — that would overclaim:
    per the soundness note above, a principal holding one qualifying role from
    *each* layer independently could still pass in the real app even when the two
    layers share no single common role. The honest claim is narrower and still
    useful: no single role is safely exportable as sufficient here, which is
    itself worth a reviewer's attention.

**A conscious, stated asymmetry, not an oversight**: `internal/report`'s RBAC
matrix is not changed to perform this reduction. It is a human-facing audit view
that already shows every `GuardApplication`'s roles as raw fact for a person to
read; showing both layers' literal content there is truthful display, not a claim
about what's allowed, unlike a deployed Cerbos policy. The safety-critical
combination belongs specifically where a wrong answer becomes a real, enforced
grant — the exporter — not in a report whose job is showing facts.

### 3. `AuthenticationRequirement`'s carryover — confirmed for the model, adapted for the algorithm

**The model concept carries over with zero change**, confirming what ADR 0010's
design already aimed for: `AuthenticationRequirement.File`/`Line` naturally
discriminate origin (a controller file versus a `SecurityConfig` file) without
needing an explicit source field.

**The *algorithm* computing it does not carry over unchanged, and this ADR states
that plainly rather than silently porting something wrong.**
`internal/extract/nestjs/authentication.go` decides *per endpoint, globally*:
across every `GuardApplication` an endpoint has, is there zero resolved role
anywhere at all? That's correct for NestJS, where only one layer can ever apply.
Spring's Java equivalent must decide **per layer**: whether the method layer alone
establishes "authenticated, any role" (its own guards, its own resolved roles,
ignoring what the URL layer separately found) and, independently, whether the URL
layer alone does — because a naive global check (`total role refs across every
layer > 0` suppresses the requirement) would wrongly suppress a real, independently-
true method-layer `isAuthenticated()` fact just because the URL layer happened to
name a concrete role elsewhere. Letting §2's reduction step reconcile the two
already-independent, already-correct per-layer facts is what correctly implements
"authenticated-any-role only wins when the other layer has nothing stricter" —
suppressing one layer *before* reconciliation, based on the other layer's
unrelated facts, would not.

### 4. Scope, superseding ADR 0011 §3's table

**Added to the first cut**: simple-pattern `SecurityFilterChain` parsing (§1), the
method×URL effective-policy reduction (§2), per-layer `AuthenticationRequirement`
computation (§3).

**Unchanged from ADR 0011, still out of scope**: custom `AuthorizationManager`
beans and any other programmatic authorization (`blog-api`'s case — still honestly
reported as "authorization not statically analyzable here," never silently shown
as unprotected), `@PostAuthorize`/`@PreFilter`/`@PostFilter`, composed/meta-
annotations, `RoleHierarchy` resolution, Kotlin source, multiple `securityMatcher`-
scoped `SecurityFilterChain` beans, regex/custom request matchers.

## Alternatives considered

- **Reconcile at extraction time, keep only the effective `RoleReference`s** —
  rejected: tested against `empty_role.go`'s existing logic before deciding, not
  assumed safe; produces a real false positive on the superseded layer's genuinely-
  present, genuinely-non-empty role declaration.
- **Give the model real boolean (AND-of-OR) structure** — rejected: bigger than
  this ADR's simple-pattern scope, same class of complexity ADR 0009 already
  declined for Cerbos's `derivedRoles`/`condition`; the cases this project can't
  losslessly flatten are exactly the cases it should omit and flag, not represent
  awkwardly.
- **Ignore the `SupplierController` finding and ship ADR 0011's original
  annotations-only scope** — rejected: confirmed wrong, not just incomplete, on
  the conventional target case; shipping it would mean Sphinxor asserting broader
  access than a real app actually grants.
- **Extend scope further to also handle `blog-api`-shaped custom
  `AuthorizationManager` beans**, using the coverage numbers as justification —
  rejected: `blog-api` is the outlier this decision explicitly declined to let
  dictate scope in *either* direction; a custom `AuthorizationManager` requires
  executing arbitrary Java to know its answer, categorically different from
  parsing a declarative rule.
- **Also change `internal/report`'s role listing to perform the same reduction** —
  rejected for now: no safety requirement forces it (see §2's stated asymmetry);
  revisit only if real use shows the raw per-layer display is actually confusing
  in practice, not preemptively.
- **Reduce via subset check ("effective set = whichever layer's set contains the
  other's, else give up") instead of set intersection** — this was this ADR's own
  first draft, corrected before Accepted rather than after: subset reduction gives
  the right answer whenever the two layers happen to be comparable (both real
  repos' cases are, including `SupplierController`), but discards a real, exact,
  zero-guessing answer whenever they're not — `{ADMIN, PHARMACIST}` and
  `{PHARMACIST, MANAGER}` share no subset relationship but do share a role,
  `PHARMACIST`, which is exactly the effective policy. Treating that as
  "conflicting, can't determine" would have been avoidable coverage loss, not
  caution — the inverse of omitting only when a safe answer genuinely isn't
  available. Intersection subsumes subset reduction as its comparable-sets special
  case, so nothing already correct regresses.

## Consequences

- `internal/model`: one additive `GuardScope` value, `ScopeRequestMatcher`.
  Nothing else changes shape.
- `internal/extract/spring` gains its own real parsing surface for
  `SecurityFilterChain` rules: recognized terminal calls, the bounded Ant-pattern
  matcher, first-match-wins rule evaluation against extracted endpoints — sized
  honestly as new engineering, not folded silently into "parsing annotations."
- `internal/export/cerbos`'s `Translate` gains the per-layer grant grouping and the
  set-intersection reduction step, plus one new `OmissionReason`
  (`ReasonNoCommonRole`, for the empty-intersection case). Verified before writing
  this, not assumed:
  `internal/diff` and `internal/report` already treat `GuardScope`/`AppliedAt`
  fully generically (`diff/keys.go` uses it only as part of an identity key;
  `report/diff.go` only prints it) — a new value needs no changes there. All three
  existing `internal/lint` rules were re-checked and none reference
  `GuardScope`/`AppliedAt` at all — zero changes required, confirmed by grep, not
  assumed from the package boundary.
- `docs/limitations.md` gains the refined blind-spot statement: not
  "`SecurityFilterChain` is unsupported," but "recognized simple-pattern rules are
  supported and AND-combined correctly with method annotations; a custom
  `AuthorizationManager`, a `securityMatcher`-scoped second chain, or a non-Ant-
  pattern matcher is not, and is reported as unresolved rather than guessed."
- Real Spring repository provenance: `categolj/blog-api` and `Kitty-Hivens/Pharmacy`
  are already hand-examined; formal vendoring (license, `NOTICE.md`, pinned commit
  — ADR 0005's discipline) happens at implementation time, not decided further
  here. Given what each repo actually demonstrates, `Pharmacy` (or a repo like it)
  is the natural fixture for the conventional, in-scope case; `blog-api` remains
  useful specifically as a real, licensed example of the out-of-scope case,
  worth keeping for a test that confirms it's honestly reported as unresolved
  rather than silently mishandled.
