# Known limitations

`vision.md` commits Sphinxor to heuristic, confidence-graded analysis, not formal verification — and to owning that openly rather than promising completeness general-purpose SAST tools already failed to deliver. This file is where that commitment gets kept concretely: real gaps in what the current static analysis can see, found empirically against real code, not hypothesized in advance.

This is not the roadmap. `roadmap-long-term.md` is about what's planned; this is about what v0.1 honestly cannot see today, whether or not fixing it is ever planned. An entry here can outlive several roadmap cycles without becoming wrong.

## Global guards (`APP_GUARD` providers, `app.useGlobalGuards()`)

Sphinxor does not parse NestJS module provider wiring. A guard registered globally — via an `APP_GUARD`-token provider in a module, or via `app.useGlobalGuards()` in `main.ts` — protects every endpoint in the application without any decorator appearing at the endpoint or its controller. This extractor only sees decorators, so it cannot see this form of protection at all.

**Consequence**: an endpoint protected exclusively by a global guard will be flagged by `mutating-endpoint-without-access-control`, at Low confidence — the rule's Low grade exists specifically because of this gap (see `internal/lint/mutating_endpoint.go`).

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
