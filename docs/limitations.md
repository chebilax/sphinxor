# Known limitations

`vision.md` commits Sphinxor to heuristic, confidence-graded analysis, not formal verification — and to owning that openly rather than promising completeness general-purpose SAST tools already failed to deliver. This file is where that commitment gets kept concretely: real gaps in what the current static analysis can see, found empirically against real code, not hypothesized in advance.

This is not the roadmap. `roadmap-long-term.md` is about what's planned; this is about what v0.1 honestly cannot see today, whether or not fixing it is ever planned. An entry here can outlive several roadmap cycles without becoming wrong.

## Global guards (`APP_GUARD` providers, `app.useGlobalGuards()`)

Sphinxor does not parse NestJS module provider wiring. A guard registered globally — via an `APP_GUARD`-token provider in a module, or via `app.useGlobalGuards()` in `main.ts` — protects every endpoint in the application without any decorator appearing at the endpoint or its controller. This extractor only sees decorators, so it cannot see this form of protection at all.

**Consequence**: an endpoint protected exclusively by a global guard will be flagged by `mutating-endpoint-without-access-control`, at Low confidence — the rule's Low grade exists specifically because of this gap (see `internal/lint/mutating_endpoint.go`).

**What to do about it today**: mark the affected endpoint(s) with a `// sphinxor-allow: <reason>` comment (`docs/decisions/0003-allowlist-format.md`), same as any other endpoint the tool gets wrong for a reason a human can verify.

## Composite decorators built with `applyDecorators()`

Confirmed common in real code, not a hypothetical edge case: a project can define its own decorator (e.g. `@Auth(roles)`) that internally calls NestJS's `applyDecorators()` to bundle `UseGuards(...)`, `Roles(...)`, and other decorators together into one. Inside that composite decorator's own implementation, `Roles(...)` and `UseGuards(...)` are plain JavaScript function calls, not `@Roles()` / `@UseGuards()` decorator syntax applied to a class or method. Sphinxor's extractor only recognizes literal decorator call sites (`@Name(...)` immediately preceding a declaration) — so an endpoint decorated with `@Auth(['admin'])` looks, syntactically, exactly like an endpoint with no guard at all.

Found and hand-verified on [`NarHakobyan/awesome-nest-boilerplate`](https://github.com/NarHakobyan/awesome-nest-boilerplate) (see `internal/extract/nestjs/testdata/awesome-nest-boilerplate/NOTICE.md`): `POST /posts` is genuinely guarded via `@Auth([RoleType.USER])`, confirmed no global guard is doing the protection instead (its `app.module.ts` registers no `APP_GUARD`), and is still flagged by `mutating-endpoint-without-access-control`.

**Consequence**: same as the global-guard case — a Low-confidence, hedged false positive rather than a confident wrong claim, which is what that confidence grade is for. A connected, secondary consequence: a role enum referenced *only* through a wrapped decorator like this is invisible to the role-declaration usage filter (`internal/extract/nestjs/roles.go`), so `permission-declared-but-unreferenced` and `empty-role` cannot fire on those roles either — not because the roles are fine, but because nothing recognizable references them.

**What to do about it today**: same as above — `sphinxor-allow` on endpoints known to be protected this way. There is no plan to special-case `applyDecorators()` unwrapping in v0.1; whether it's worth the extraction complexity in a later version is an open question, not a commitment made here.
