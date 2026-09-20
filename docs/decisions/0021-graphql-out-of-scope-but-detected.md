# 0021. GraphQL is out of scope, and a project that uses it is told so

## Status

Accepted.

## Context

The NestJS blind-spot hunt behind [ADR 0020](0020-unanalyzable-is-unknown-not-absent.md)
Amendment 1 found a third silent failure. Unlike the other two it is not a defect:
every line of code involved behaves exactly as designed. It is a scope boundary
that the output does not admit to having.

`notiz-dev/nestjs-prisma-starter`'s entire API is GraphQL. Four `@Resolver`
classes carry 16 operations — 6 `@Query`, 6 `@Mutation`, 3 `@ResolveField`, 1
`@Subscription` — of which 10 are behind `@UseGuards(GqlAuthGuard)` at class or
method level, and 6 are deliberately public (signup, login, token refresh, and
the hello-world resolver). Alongside them sits a two-route hello-world
`@Controller`. Sphinxor reports:

```
43 source file(s).
2 endpoint(s), 0 finding(s): 0 blocking, 0 warning, 0 allowlisted.
```

Clean, confident, and describing the two REST routes. The 16 GraphQL
operations — including the 10 carrying real authorization, and the 6 that are
public — are not mentioned, counted, or hinted at.

### Why the existing safety net does not catch this

[ADR 0019](0019-cli-framework-selection.md) §2 drew the distinction this project
cares about — "couldn't look" versus "looked, found nothing" — and gave the second
case a notice: *parsed N source files but recognized no endpoints*. That notice
keys on the endpoint count being **zero**. Here it is two. A single unrelated REST
controller is enough to suppress the one signal that would have said something,
and the run reads as a clean bill of health.

That is the same structural mistake as ADR 0020's family, in a milder form: a
condition Sphinxor cannot analyze is invisible in the output, and invisibility
reads as absence. The difference is that here the correct fix is not to analyze it.

### Why this is tempting to just implement, and why that is the trap

The authorization vocabulary is *identical*. On
`CatsMiaow/nestjs-project-structure`'s resolver — a mixed REST + GraphQL project —
the decorators are the same ones the REST controllers use:

```ts
@Resolver(() => Simple)
export class SimpleResolver {
  @Query(() => Payload)
  @UseGuards(JwtAuthGuard, RolesGuard)
  @Roles('test')
  ...
```

`@UseGuards` and `@Roles` are already extracted. It would take very little to start
emitting rows for these, and that is precisely the reason to decide the boundary
deliberately rather than let it be crossed by a small convenient change.

What is *not* the same is everything downstream of the decorators. A GraphQL
operation has no HTTP method and no path, and the model's endpoint identity, the
matrix's columns, the allowlist's anchoring, and the Cerbos exporter's
resource/action mapping are all built on method + path
([ADR 0002](0002-intermediate-model-structure.md),
[ADR 0009](0009-cerbos-exporter.md)). `@ResolveField` authorizes a *field* on a
type, which has no equivalent in the current model at all. Supporting GraphQL
honestly is a model change with its own real-fixture validation — not an
extraction tweak — and half-supporting it would produce a matrix whose rows mean
two different things without saying which.

## Decision

**GraphQL remains out of scope. A project that uses it is told so.**

Detection only, with a project-level warning. No resolver is parsed, no operation
becomes an endpoint, and no finding changes.

### §1 Detection

A project is recorded as using GraphQL when `@Resolver` appears in the analyzed
source. `@Query`, `@Mutation`, `@Subscription` and `@ResolveField` are counted so
the warning can state the size of what was skipped, since "this project uses
GraphQL" and "this project has 16 unanalyzed operations" land differently on a
reader.

Detection is deliberately shallow, exactly as [ADR 0020](0020-unanalyzable-is-unknown-not-absent.md)
§3 treats reactive Spring chains: establishing *that* a surface exists is what
converts silent into loud, and it is cheap. Establishing what it requires is the
coverage feature this ADR declines.

### §2 The warning

Emitted through the mechanism ADR 0019 §2 established — stderr, so `--format json`
on stdout stays clean — alongside ADR 0020 §4's project-level caveats. It states
that resolvers were found, how many operations, that they are not analyzed, and
that any authorization on them is absent from the report.

It fires **whenever resolvers are present**, not only when they outnumber the REST
endpoints. A mixed project is the more dangerous case, not the less: its REST
matrix is correct and complete, which is exactly what makes the missing half easy
to overlook.

It does not change any finding, the exit code, or what gates CI. This is the same
posture as ADR 0020 §4: it changes what the reader should believe the output
covers, which is the whole point.

### §3 What this does not do

No resolver is parsed. Guards and roles that *are* readable on a resolver are still
not extracted, because extracting them would put rows in the matrix that have no
method and no path. When GraphQL support is genuinely wanted it gets its own ADR,
its own model decision for operation identity and field-level authorization, and
its own real fixtures.

## Alternatives considered

- **Parse resolvers now** — rejected, and this is the substantive choice here. It
  is tempting because the decorators are already understood, but an operation with
  no method and no path does not fit an identity, a matrix row, an allowlist
  anchor, or a Cerbos action. Every one of those would need deciding, and deciding
  them inside a fix for a missing warning is how a scope boundary gets crossed
  without anyone choosing to cross it.
- **Emit resolvers as endpoints with a synthetic path** (e.g. `POST /graphql` per
  operation, or `QUERY /me`) — rejected as the worst option available. It would
  fabricate routes that do not exist, and this project's entire posture is that
  inventing a confident answer is worse than admitting to an unknown one. It would
  also collide every operation onto one identity, which ADR 0020 Amendment 1 §5
  has just finished removing.
- **Treat a resolver-bearing project as zero-endpoint and error out** — rejected.
  It is wrong for mixed projects, where the REST analysis is genuinely useful and
  correct, and an error that fires on a legitimate configuration is how a CI tool
  gets switched off.
- **Widen ADR 0019 §2's notice instead** — e.g. fire it when endpoints are found
  but most source files contain none — rejected as a proxy for the real signal.
  A heuristic on file ratios would misfire on ordinary projects with many
  non-controller files, and would still say nothing about *what* was skipped.
  Detecting the construct by name is both simpler and more specific.
- **Document it in `docs/limitations.md` and stop there** — rejected: that is the
  current state, and it is what produced a clean bill of health on a project whose
  whole authorization surface was never examined. A limitation the user has to
  already suspect in order to look up is not a signal.

## Consequences

- `internal/extract/nestjs` gains resolver detection and `internal/model` a small
  status value for it, in the shape ADR 0020 §2/§4 already established for URL
  layers and global guards. `internal/cli` gains one more project warning.
- Regression tests are minimal fixtures **reduced from** the two real repositories
  the hunt found and labelled as such, following the practice in
  `internal/extract/spring`: `notiz-dev/nestjs-prisma-starter` for the
  GraphQL-first case that produced the false clean run, and
  `CatsMiaow/nestjs-project-structure` for the mixed case where the warning must
  fire alongside a correct REST matrix. Both real repositories are also run end to
  end by hand before the change is called done, since a reduction can preserve a
  shape while losing the thing that made it occur. Confirmed failing before being
  kept, per the bar set in ADR 0014 and reaffirmed since.
- The noise floor is guarded the same way ADR 0020 §4's caveats are: a project with
  no resolvers must produce no warning, pinned by a test against a real
  GraphQL-free project.
- `docs/limitations.md` gains a GraphQL entry describing a loud gap rather than a
  silent one.
- Spring's equivalent — `@QueryMapping`/`@MutationMapping`/`@SchemaMapping` — is
  **not** covered here. It was not surveyed during the hunt, and adding it on the
  strength of symmetry rather than evidence would be the kind of assumption this
  project keeps finding it was wrong to make. It is named here so the gap is
  recorded rather than implied.
