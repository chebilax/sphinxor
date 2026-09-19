# Known limitations

`vision.md` commits Sphinxor to heuristic, confidence-graded analysis, not formal verification — and to owning that openly rather than promising completeness general-purpose SAST tools already failed to deliver. This file is where that commitment gets kept concretely: real gaps in what the current static analysis can see, found empirically against real code, not hypothesized in advance.

This is not the roadmap. `roadmap-long-term.md` is about what's planned; this is about what the current release honestly cannot see, whether or not fixing it is ever planned. An entry here can outlive several roadmap cycles without becoming wrong.

## Global guards (`APP_GUARD` providers, `app.useGlobalGuards()`)

Sphinxor does not parse NestJS module provider wiring. A guard registered globally — via an `APP_GUARD`-token provider in a module, or via `app.useGlobalGuards()` in `main.ts` — protects every endpoint in the application without any decorator appearing at the endpoint or its controller. This extractor only sees decorators, so it cannot see *what* that guard requires.

Since [ADR 0020](decisions/0020-unanalyzable-is-unknown-not-absent.md) §4 it does see *that* one is registered, and says so. Both forms are detected, and a run on such a project opens with a warning that endpoint-level results understate protection across the board. Verified against `nestjs/nest`'s own `19-auth-jwt` sample, where the recommended pattern — a global guard with `@Public()` opting out — inverts the default this extractor assumes and made every endpoint report as unguarded.

**Consequence**: an endpoint protected exclusively by a global guard is still flagged by `mutating-endpoint-without-access-control`, at Low confidence — the rule's Low grade exists specifically because of this gap (see `internal/lint/mutating_endpoint.go`) — but the report no longer presents that flag without saying the whole project's results lean that way. The error direction was always the safe one; it being unsignalled was not.

**What to do about it today**: mark the affected endpoint(s) with a `// sphinxor-allow: <reason>` comment (`docs/decisions/0003-allowlist-format.md`), same as any other endpoint the tool gets wrong for a reason a human can verify.

## Composite decorators built with `applyDecorators()` — narrowed, not closed

Confirmed common in real code, not a hypothetical edge case: a project can define its own decorator (e.g. `@Auth(roles)`) that internally calls NestJS's `applyDecorators()` to bundle `UseGuards(...)`, `Roles(...)`, and other decorators together into one. Found and hand-verified on [`NarHakobyan/awesome-nest-boilerplate`](https://github.com/NarHakobyan/awesome-nest-boilerplate): `POST /posts` is genuinely guarded via `@Auth([RoleType.USER])`, confirmed no global guard was doing the protection instead.

As of [ADR 0006](decisions/0006-composite-decorator-resolution.md), extraction follows **one level** of this indirection: a composite matching a specific, bounded shape (a single, unconditional return path calling `applyDecorators(...)`, plain identifier parameters, direct pass-through argument substitution) is resolved, and `POST /posts` above no longer produces a false positive. What's still invisible, deliberately, per that ADR's stated non-goals:

- **Multi-level composite chains** — a composite calling another composite that calls `applyDecorators(...)`.
- **Conditional or branching decorator construction** — a composite with more than one `return` path (e.g. `if (...) return SkipAuth(); return applyDecorators(...)`).
- **Destructured parameters** — `function Auth({ roles }: { roles: RoleType[] })` rather than a plain `roles` parameter.
- **Non-trivial dataflow** — a parameter transformed before being passed to the inner `Roles`/`UseGuards` call (e.g. `Roles(roles.map(...))`), or passed via spread (`Roles(...roles)`).

A composite outside this bounded shape isn't guessed at — it falls back to exactly the behavior described above (invisible, Low-confidence flag on the endpoint using it), the same honest default as before ADR 0006, not a new failure mode.

**Consequence for what's still invisible**: same as the global-guard case — a Low-confidence, hedged false positive rather than a confident wrong claim. A connected, secondary consequence: a role enum referenced *only* through a wrapped decorator outside this bounded shape is invisible to the role-declaration usage filter (`internal/extract/nestjs/roles.go`), so `permission-declared-but-unreferenced` and `empty-role` cannot fire on those roles either.

**What to do about it today**: for anything outside the resolved shape, same as above — `sphinxor-allow` on endpoints known to be protected this way. There is no plan to extend beyond one level of indirection or direct pass-through substitution in v0.1; whether it's worth the extraction complexity in a later version is an open question, not a commitment made here.

## Spring: `SecurityFilterChain` beyond simple, single-chain patterns

Not "`SecurityFilterChain` is unsupported" — as of [ADR 0012](decisions/0012-securityfilterchain-effective-policy.md), recognized simple-pattern `authorizeHttpRequests` rules are supported and correctly AND-combined with method-level `@PreAuthorize`/`@Secured`/`@RolesAllowed` (the set intersection described there), verified against the real mismatch that drove that ADR: `Kitty-Hivens/Pharmacy`'s `SupplierController.GET` allows `ADMIN` or `PHARMACIST` at the method layer but only `ADMIN` at the URL layer, and Sphinxor now reports the real, `ADMIN`-only effective policy rather than the method layer's broader claim.

What's still invisible, per that ADR's stated scope and [ADR 0018](decisions/0018-unrecognized-rule-stops-evaluation.md)'s correction to it:

- **A custom `AuthorizationManager`** (`.access(...)`) — executing arbitrary Java to know the real answer is categorically out of scope. Confirmed common in real code, not hypothetical: `categolj/blog-api`'s entire tenant-scoped rule set uses this, alongside a handful of ordinary `.hasAuthority(...)` rules in the *same* chain — extraction recognizes the `.hasAuthority(...)` ones individually and correctly stops evaluating at the first `.access(...)` rule that matches a given endpoint (per ADR 0018), rather than skipping past it to a later, more permissive rule.
- **More than one `SecurityFilterChain` bean in the same project** (chain selection by `@Order`/`securityMatcher` scoping) — extraction requires finding exactly one `@Bean`-annotated method returning `SecurityFilterChain` project-wide, rather than guessing which chain (or ordering) actually applies to a given request. Per ADR 0020 §2 this is now *announced*, not silently skipped: the project's URL layer is recorded as present-but-unknown, `sphinxor lint` warns that the roles it shows may be broader than what the application enforces, and `sphinxor export cerbos` omits every endpoint rather than exporting a method-layer-only policy. Previously the layer just vanished, and the audit's two-chain reproduction exported `roles: [ADMIN, ANALYST]` for an endpoint the running application restricted to `ADMIN` — a grant the application itself denies.
- **Reactive `SecurityWebFilterChain` / `ServerHttpSecurity`** (Spring WebFlux) — not parsed. Its rules use a different builder API (`authorizeExchange`, `pathMatchers`) that extraction does not read at all. Like the multi-chain case, and for the same reason, it is detected so it cannot be mistaken for "this project has no URL layer", and produces the same warning and the same export omission.
- **Non-Ant-pattern matchers**: regex or character-class syntax, a custom `RequestMatcher` bean, `mvc.matcher(...)`, `dispatcherTypeMatchers`. Only literal segments, `*`, `**`, and `{var}`-as-wildcard are matched. A matcher whose pattern cannot be read makes that *rule* unknown (ADR 0020 §1) — it stops evaluation for endpoints it might cover and grants its roles to none of them. The wrapper forms that carry an ordinary readable pattern one call deeper, `requestMatchers(antMatcher("/admin/**"))`, are read normally.
- **`@PostAuthorize`/`@PreFilter`/`@PostFilter`**, composed/meta-annotations, `RoleHierarchy` resolution, and Kotlin source remain out of scope, per ADR 0011.

**Consequence**: an endpoint whose real access control depends on any of the above is reported using whatever the method layer alone establishes (or as unguarded, if the method layer has nothing either) — under-reporting relative to a rule Sphinxor can't read, never over-reporting a grant that isn't real, consistent with the intersection's own soundness-not-completeness property. Where the gap is project-wide rather than rule-local — the multi-chain and reactive cases — the run says so explicitly instead of leaving the reader to infer it from this file.

**What to do about it today**: `sphinxor-allow` on endpoints known to be protected by one of the above, same as any other endpoint the tool cannot see into.

## Spring: method-security annotations that may never be switched on

`@PreAuthorize`, `@Secured` and `@RolesAllowed` do nothing at runtime unless `@EnableMethodSecurity` (or the older `@EnableGlobalMethodSecurity`) is present. An application can carry a full set of annotations and enforce none of them.

Sphinxor deliberately does not downgrade those annotations when it can't find the enabling one, per [ADR 0015](decisions/0015-inert-method-security-guard.md): absence is not evidence, since the configuration can live in a parent module, an imported starter, or Kotlin source this extractor doesn't parse. Guessing "disabled" would invent findings; guessing "enabled" is what it does, and that is the safer of the two guesses to make out loud.

What changed with ADR 0020 §4 is that it is now made out loud. When method-security annotations are found and no enabling annotation is located anywhere in the analyzed source, the run warns that those annotations may be inert — in which case every role shown for the endpoints they appear to protect is imaginary, and the endpoints are not protected at all.

**Consequence**: unchanged in the model and the findings; the roles are still reported. The difference is that a report which is confidently wrong in this specific way now carries the one sentence that lets a reader check.

**What to do about it today**: confirm where method security is enabled for the application. If it's outside the analyzed source, nothing is wrong with the report; if it's nowhere, the finding is that the annotations are decorative.

## A `sphinxor-allow` marker separated from its endpoint by a block comment

The allowlist matcher (`internal/allowlist`, shared by both extractors since the Spring port) skips blank lines and `//` line comments between a marker and the endpoint it exempts, but not `/* */` blocks. So a marker placed above a Javadoc/JSDoc block, rather than directly above the endpoint's first decorator/annotation, exempts nothing.

This is recorded as benign, deferred debt rather than a latent false negative, on two grounds — both checked rather than assumed:

- **The dominant Spring idiom is immune by construction, not by luck.** Java annotations are part of tree-sitter's `method_declaration` node, so the endpoint's anchor line is its *first annotation*. A handler documented with OpenAPI annotations (`@Operation(summary = ...)`, the pattern in `Kitty-Hivens/Pharmacy`) therefore has its documentation *inside* the anchor, never between the marker and it. Across the whole vendored Java corpus — both `SecurityConfig`s, all five controllers, the role enum — there is not one block comment; handlers are either bare (`categolj/blog-api`) or annotation-documented (Pharmacy). Javadoc above handlers is common in Java generally, but not in the Spring REST controllers this targets.
- **When it does occur, it fails loudly, not silently.** A marker that matches nothing produces the `stale-allow-marker` finding ([ADR 0003](decisions/0003-allowlist-format.md)) at `High` confidence, naming the marker's file and line and stating that it does not sit directly above a recognized endpoint — and `High` findings gate CI. Verified directly against the real Pharmacy fixture with a Javadoc inserted between marker and handler. A developer never silently believes an exemption applied; their build fails telling them it didn't. That is the self-announcing refusal the stale-marker finding exists to produce, the inverse of this project's reassuring-false-negative failure class.

**What to do about it today**: put the marker directly above the endpoint's first decorator or annotation, above any Javadoc/JSDoc block. The stale-marker finding will tell you if you haven't.

**Why it isn't fixed yet**: the matcher is now shared by both extractors, so widening it to skip block comments is a two-framework behavior change affecting existing NestJS results, and it deserves its own tests rather than being folded into an unrelated change. Deferred deliberately, not overlooked.
