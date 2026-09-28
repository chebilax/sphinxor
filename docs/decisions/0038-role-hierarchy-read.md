# 0038. A role hierarchy written in source is read, and the roles it implies are shown as implied

## Status

**Accepted, staged (§12).** Stage 1 — read the hierarchy and state it — is implemented.
Stage 2 — expand implied roles into the matrix, the diff and the export — is **deferred**
until a real project has matrix rows for it to change. Amends [ADR 0031](0031-role-hierarchy.md)
§1 and reopens the alternative that ADR rejected — *"expand the hierarchy into the
matrix"* — on the condition it gave for reopening it: *"if a real project ever needs it."*

The measurement below finds hierarchies common in real applications and mostly readable,
but on the sample **expanding them changes zero matrix rows**, for reasons that have
nothing to do with the hierarchy. That is the evidence bar this project applies
elsewhere, and it is why Stage 2 waits.

**This decision is version-bound.** §13 records which Spring Security releases the
parser and wiring behaviour were read from, and that the conclusion is re-checked on
every Spring Security major version.

## Context

### ADR 0031's zero still holds on the corpus, and does not generalize

Re-measured on 2026-09-28 against the 20-repository corpus at the commits in
[ADR 0035](0035-permissions-in-the-model.md) §9: no file in any of the 20 mentions
`RoleHierarchy`, `roleHierarchy` or `role-hierarchy`, and the ADR 0031 warning fires on
none of them. The corpus has no hierarchy.

The corpus was not built to find one, though. Code search was:

- **Queries.** `RoleHierarchyImpl language:Java`, `RoleHierarchyImpl language:Kotlin`
  and `role-hierarchy language:YAML`, 1,192 hits across 969 repositories.
- **Kept.** Non-fork repositories with at least 50 stars — 30 of them.
- **Excluded.** Spring Security itself; tutorials, courses, samples, study notes and
  docs repositories (13, by their own description); and one Go project whose hit was a
  Casbin YAML key.
- **Result.** 15 applications, each shallow-cloned and read by hand.

Code search is not exhaustive and this is a sample, not a census.

| Project | ★ | Commit | Lang | License | How the hierarchy is built | Status |
|---|---:|---|---|---|---|---|
| `gameyfin/gameyfin` | 1091 | `1f377f3dd` | Kotlin | AGPL-3.0 | `fromHierarchy("""…""".trimMargin())`, two lines | not parsed — Kotlin |
| `wyt1215819315/autoplan` | 670 | `96f65a379` | Java | Apache-2.0 | `setHierarchy("ROLE_ADMIN > ROLE_USER")` | **literal** |
| `flowerfine/scaleph` | 395 | `67d3ca676` | Java | Apache-2.0 | `setHierarchy(literal)` in a `RoleHierarchyImpl` subclass | **dead** — never instantiated |
| `svt/encore` | 337 | `8dd7c596c` | Kotlin | EUPL-1.2 | `fromHierarchy(ROLE_HIERARCHY)`, a `private const val` | not parsed — Kotlin |
| `codeabovelab/haven-platform` | 299 | `bd450c315` | Java | Apache-2.0 | the project's **own** `RoleHierarchyImpl` class, `builder().childs(Authorities.ADMIN_ROLE, …)` | project-defined |
| `techdev-solutions/trackr-backend` | 203 | `f985c5f16` | Java | MIT | `setHierarchy("A > B B > C C > D")` — three pairs, one line | **literal**, version-dependent (§2) |
| `devondragon/SpringUserFramework` | 191 | `91fa02674` | Java | Apache-2.0 | `fromHierarchy(config.getRoleHierarchyString())` from `user.roles.role-hierarchy[n]` | config property |
| `Gosrock/DuDoong-Backend` | 151 | `1d7d7134f` | Kotlin | AGPL-3.0 | `setHierarchy("A > B > C")` | not parsed — Kotlin |
| `molgenis/molgenis` | 122 | `b2b0016b2` | Java | LGPL-3.0 | `CachedRoleHierarchyImpl` over a `DataService` | database |
| `YukeSeko/YukeSeko-Interface` | 118 | `12f35f482` | Java | Apache-2.0 | local `String hierarchy = "ROLE_admin > ROLE_user"` | **literal via local** |
| `nkonev/videochat` | 102 | `fd28bd4d1` | Java | Apache-2.0 | `UserRole.ROLE_ADMIN.name() + " > " + UserRole.ROLE_USER.name() + "\n"` | **enum concatenation** |
| `tagbangers/wallride` | 97 | `212834111` | Java | Apache-2.0 | `setHierarchy("ROLE_ADMIN > ROLE_VIEWER")` | **literal** |
| `bulktrade/SMSC` | 89 | `33dfffc2f` | Java | Apache-2.0 | `setHierarchy("ROLE_POWER_ADMIN_USER > ROLE_ADMIN_USER")` | **literal** |
| `ngcly/music-story` | 65 | `75bcb4144` | Java | Apache-2.0 | commented out | none |
| `sivaprasadreddy/spring-modular-monolith` | 189 | `2933f5f14` | Java | Apache-2.0 | `fromHierarchy(Role.getRoleHierarchy())`, a static method returning enum concatenation | *sample app — not counted* |

**In scope: 9 Java applications with a live hierarchy.** The three Kotlin ones cannot be
read at all, since Sphinxor parses no Kotlin ([ADR 0011](0011-spring-second-framework.md)
§1). That applies to their endpoints as much as to their hierarchies. Two are not live.

Of the 9:

| Form | Count | Readable statically |
|---|---:|---|
| whole string literal at the call site | 4 | yes |
| literal through a local variable | 1 | yes |
| enum `.name()` concatenation | 1 | yes |
| project-defined hierarchy class | 1 | no — its semantics are the project's code |
| configuration property | 1 | no |
| database | 1 | no |

**6 of 9 readable by form.** The brief this ADR was drafted against gave 9 of 13 from an
earlier hand count with different buckets (a `static final` concatenation, a constant
chain across classes, four enum concatenations). That list is not available on this
machine, so the two cannot be reconciled row by row. This table is the one that can be
re-run.

### Finding 1: the same literal means different things on different Spring Security versions

`RoleHierarchyImpl` has parsed its string three ways. Read at the tags in
`spring-projects/spring-security`:

| Version | Parser | `"A > B\nB > C"` | `"A > B > C"` | `"A > B B > C"` |
|---|---|---|---|---|
| ≤ 5.0 | regex `\s*([^\s>]+)\s*>\s*([^\s>]+)`, anywhere | A>B, B>C | **A>B only** | A>B, B>C |
| 5.1 | per line, `split(" > ")` | A>B, B>C | A>B, B>C | A>"B B">C — **malformed** |
| ≥ 5.2 | per line, `split("\\s+>\\s+")` (commit `be0ad673c2`) | A>B, B>C | A>B, B>C | A>"B B">C — **malformed** |

The three agree on exactly one form: **one `X > Y` pair per line.**

trackr is the case this matters for. It builds on Spring Boot 1.5.19, so Spring Security
4.2, and its literal is three pairs on one line. On 4.2 that literal is
`ADMIN > SUPERVISOR > EMPLOYEE > ANONYMOUS`. On anything since 5.1 it is one malformed
chain: `ROLE_ADMIN` implies a role literally named `ROLE_SUPERVISOR ROLE_SUPERVISOR`,
which no authority can ever match, and the chain ends at `ROLE_ANONYMOUS`. So an
administrator reaches no supervisor or employee endpoint through it. Nothing in the source files says which
version applies.

Some APIs do establish a floor, and all of them are past the split. Checked at the tags:

- `authorizeHttpRequests(…)` exists from 5.5.
- `@EnableMethodSecurity` exists from 5.6.
- `RoleHierarchyImpl.fromHierarchy(…)` and `withDefaultRolePrefix()` exist from 6.3.

A project using any of them is on the ≥ 5.2 grammar.

### Finding 2: which layer a hierarchy reaches depends on wiring and version

A `@Bean RoleHierarchy` is picked up automatically by some configurers and not others.
Read at the tags listed in §13 (`getBeanNamesForType` / `ObjectProvider<RoleHierarchy>` /
an `@Autowired(required = false) setRoleHierarchy`):

| Layer | Applies a `RoleHierarchy` bean |
|---|---|
| legacy `@EnableGlobalMethodSecurity`, `@PreAuthorize` | yes, every version read — set on the default expression handler |
| legacy `@EnableGlobalMethodSecurity`, `@Secured` / `@RolesAllowed` | **no**, every version read — they go through a plain `RoleVoter` |
| legacy `authorizeRequests()` / `WebSecurityConfigurerAdapter` | yes, every version read |
| `authorizeHttpRequests()` | from **6.1** |
| `@EnableMethodSecurity` (`@PreAuthorize`, `@Secured`, `@RolesAllowed`) | from **6.3** — not on 5.6–6.2, and not backported to 6.2.8 |

An explicit `setRoleHierarchy(…)` on a handler reaches that handler's layer on every
version. trackr sets it on the method expression handler and SMSC on the web one; both
also carry `@EnableGlobalMethodSecurity`.

videochat is the case where the version can matter. It carries `@EnableMethodSecurity`
and `authorizeHttpRequests`, and builds its hierarchy with `new RoleHierarchyImpl()` +
`setHierarchy`, not `fromHierarchy`, so nothing in its source pins 6.3. It also sets the
hierarchy on its own `MethodSecurityExpressionHandler` bean. `@EnableMethodSecurity`
uses such a bean on every version read, so its `@PreAuthorize` receives the hierarchy
whatever the version. Only its `@Secured` (`securedEnabled = true`) and its URL layer
depend on the version.

The first draft of this finding said all of videochat's method-layer reach was
version-dependent. Reading the expression-handler wiring falsified that. It is recorded
here rather than quietly corrected, for the reason ADR 0015 Amendment 1 §2 gives.

Its build file says Spring Boot 3.5.6, so Spring Security 6.5, where every layer applies
the hierarchy. Sphinxor does not read build files (see Alternatives), so the run cannot
know that, and the warning states what it assumed (§12).

### Finding 3: in the sample, expansion changes no matrix row

The shipped binary was run on all 9 in-scope applications, plus scaleph and the sample
app:

| Project | Endpoints | Rows with a role | Why the hierarchy has nothing to widen |
|---|---:|---:|---|
| autoplan | 78 | 0 | its `hasRole` rules are in a `WebSecurityConfigurerAdapter` URL layer, reported as unanalyzed ([ADR 0020](0020-unanalyzable-is-unknown-not-absent.md) §2) |
| YukeSeko-Interface | 69 | 0 | same |
| wallride | 161 | 0 | same |
| haven-platform | 139 | 0 | `@Secured(Authorities.ADMIN_ROLE)` — a constant, so 28 rows are `?` — and the hierarchy is project-defined anyway |
| SMSC | 5 | 0 | its 174 `@PreAuthorize` sit on Spring Data REST repositories, not controllers |
| videochat | 49 | 0 | `isAuthenticated()` and bean calls; no endpoint requires a role |
| SpringUserFramework | 35 | 0 | `hasRole('ADMIN')` is on a service method; and the hierarchy is a config property |
| molgenis | 287 | 19 | roles present — but the hierarchy is in the database |
| **trackr-backend** | 18 | **7** | roles present **and** the hierarchy is a literal — but the literal is the version-dependent form (Finding 1) |

**One project** has role-bearing rows next to a hierarchy in source, and its hierarchy
is the one form that cannot be read without knowing the version. Of the five whose
hierarchy *is* unambiguous (autoplan, YukeSeko, videochat, wallride, SMSC), none has a
role-bearing row for it to widen. The gaps that stand in the way are all recorded
elsewhere: the legacy URL layer, constant-valued role arguments, Spring Data REST.
The hierarchy is not what is missing.

## Decision

### §1 What is read

The argument of `RoleHierarchyImpl.fromHierarchy(…)`, or of `setHierarchy(…)` on a
`RoleHierarchyImpl`, inside a hierarchy ADR 0031 §3 already detects, when that argument
evaluates statically to one string from:

- string literals;
- `+` concatenation of the terms in this list;
- a local variable, or a `static final String` field of a class in the analyzed tree,
  initialized by such an expression;
- `E.C.name()` where `E` is an enum in the analyzed tree declaring constant `C`, which
  evaluates to `"C"`. This is Java's defined meaning of `name()`, not a guess.

Resolution uses the same project-wide index [ADR 0032](0032-controllers-that-yield-no-routes.md)
§1 builds, and stops at the first term it cannot evaluate. A partial string is never
parsed.

**Everything else stays announced as today**, and the ADR 0031 warning gains the reason
it was not read:

- a configuration property;
- a database or service call;
- a project-defined `RoleHierarchy` implementation;
- an expression that is not constant;
- a static method call — `spring-modular-monolith`'s `Role.getRoleHierarchy()`, seen
  once, in a sample app;
- more than one hierarchy bean;
- the 6.3 builder, `withDefaultRolePrefix().role(…).implies(…)` — zero uses in the
  sample, and not read for the reason [ADR 0035](0035-permissions-in-the-model.md) §2
  gives for `hasPermission`;
- a `GrantedAuthorityDefaults` bean, which changes the `ROLE_` prefix §4 relies on —
  zero in the sample;
- a declaring class carrying `@Profile` or `@Conditional…` — trackr's does. Sphinxor
  evaluates no profile anywhere, and a hierarchy that exists only under one must not
  turn into unconditional grants in the matrix.

### §2 The string is parsed only in the form every version agrees on, unless the source fixes the version

Each non-blank line must be exactly one `X > Y` pair. The lines come from splitting the
evaluated string on `\n`, which is what videochat's trailing `"\n"` exists for.

A line holding a chain (`A > B > C`) or several pairs (`A > B B > C`) is read only when
the source establishes Spring Security ≥ 5.2 by one of the APIs in Finding 1. Otherwise
**the whole hierarchy is not read** — never the agreeing subset, because the other
reading may be the one that runs. The warning quotes both readings in that case:

```
warning: this project declares a Spring Security role hierarchy in: MethodSecurityConfiguration.
         Its rules are written in a form Spring Security reads differently by version: up to 5.0 as
         ROLE_ADMIN > ROLE_SUPERVISOR > ROLE_EMPLOYEE > ROLE_ANONYMOUS; from 5.2 as ROLE_ADMIN >
         "ROLE_SUPERVISOR ROLE_SUPERVISOR" > "ROLE_EMPLOYEE ROLE_EMPLOYEE" > ROLE_ANONYMOUS. A
         quoted role containing spaces matches no authority. Nothing in the analyzed source fixes
         the version, so the roles below are shown as declared and may be NARROWER than what the
         application grants. It is declared under @Profile("granular-security"), which Sphinxor does
         not evaluate: it applies only when that condition holds.
```

That is the output on the trackr fixture, verbatim. It is strictly more than a reader got
before, it changes no fact in the matrix, and it is correct under both readings. It says
"from 5.2" rather than "from 5.1" because 5.1's exact-separator parser agrees with 5.2's
on this string, and in general the run compares only the ≤5.0 and ≥5.2 readings. Where
those two agree, the string is read. 5.1 was one minor line, and §13 records the choice.

A cycle is not read either. Spring rejects one at startup, so the source is not the
running configuration.

### §3 A layer's roles are widened only where the hierarchy is shown to reach that layer

| Layer | Reach established by |
|---|---|
| method, `@PreAuthorize` | `setRoleHierarchy(…)` on a method-security expression handler, including one exposed as a bean; **or** a hierarchy bean with `@EnableGlobalMethodSecurity`; **or** a hierarchy bean with `@EnableMethodSecurity` **and** a 6.3 floor (`fromHierarchy` / `withDefaultRolePrefix` in the tree) |
| method, `@Secured` / `@RolesAllowed` | a hierarchy bean with `@EnableMethodSecurity` **and** a 6.3 floor. **Never** under `@EnableGlobalMethodSecurity` alone, which does not apply it to them (Finding 2) |
| URL (`SecurityFilterChain`, `authorizeHttpRequests`) | `setRoleHierarchy(…)` on a web expression handler; **or** a hierarchy bean **and** a 6.1 floor — in practice the same 6.3 evidence, since nothing observable in source pins 6.1 alone |

The legacy URL layer reaches on every version but is not analyzed, so there is no role
there to widen.

Where reach is not established, that layer's roles are shown as declared, and the
warning says the hierarchy was read but not shown to apply to it.
`MethodSecurityStatus` gains which enabler was found, since today it records only that
one was.

### §4 Matching is by the authority each check compares, not by the literal

A hierarchy names authorities. A role reference names what its call was given. The
Spring extractor records the compared authority on each `RoleReference` as a new field,
`Authority`:

| Written as | Compared authority |
|---|---|
| `hasRole('X')`, `hasAnyRole`, URL-DSL `.hasRole`, `@RolesAllowed` | `ROLE_X`, unless `X` already starts with `ROLE_` |
| `hasAuthority('X')`, `hasAnyAuthority`, `@Secured` | `X` |

This reproduces the defaulting `SecurityExpressionRoot` applies. `Authority` is empty on
NestJS, which has no hierarchy concept, and nothing there reads it.

### §5 An implied role is its own collection, never a `RoleReference`

```
impliedRoles: [{ id, roleReferenceId, authority, path }]
```

One entry per authority that reaches a declared role through the transitive closure.
`path` is the chain as written, for example `["ROLE_ADMIN", "ROLE_SUPERVISOR"]`, so a
reader can see why the role is there.

It is not a `RoleReference` because it is not a place in the code where a role is
required. Folding it in would make every consumer that counts references — `empty-role`,
the diff, the collision check, the unreferenced-declaration rule — silently count
something the source does not state. That is the `DeclaresRoles` lesson ADR 0035 §7
cites.

`RoleHierarchyStatus` gains `Read bool`, the edges, and a not-read reason.

### §6 In the matrix, implied roles share the cell and are marked

```
| PUT | /vacationRequests/{id}/approve | … | ROLE_SUPERVISOR · implied: ROLE_ADMIN | … |
```

**In the Roles cell, not a new column**, because the error being corrected is a reader
scanning Roles and concluding "only supervisors". A separate column is exactly where
that reader would not look. ADR 0035 §5 made the same argument for keeping its
Permissions column adjacent. It applies more strongly here, because implied roles
answer the same question as declared ones.

Declared roles come first, unchanged. Implied roles follow the marker `· implied:`,
sorted. JSON gets `impliedRoles`, omitted when empty, so every project without a read
hierarchy serializes byte-identically.

A `?` cell stays `?`, and a `-` cell stays `-`: nothing implies from an unread or empty
requirement.

### §7 Warnings

- **Read and applied:** the ADR 0031 warning is replaced by a notice that quotes the
  edges and says implied roles are marked in the matrix. It is still stated, because the
  matrix now contains something the annotation does not.
- **Read, not applied to a layer (§3):** the notice says so and names the layer.
- **Not read (§1, §2):** the ADR 0031 warning as today, plus its reason, plus both
  readings where §2 applies.

### §8 Lint rules: none changes, and why each is safe

- **`empty-role`** reads `RoleReference`s. Implied roles are not references, and an
  empty declared list implies nothing. **Zero delta by construction.**
- **`mutating-endpoint-without-access-control`** reads guards. It is unchanged.
- **`permission-declared-but-unreferenced`** is unchanged. A role that appears only in
  the hierarchy is still never *required* by code, which is that rule's subject. It is
  Low confidence and does not gate.

### §9 `sphinxor diff`: the hierarchy is compared, implied roles are not keyed

Implied roles are derived. One edge added to the hierarchy would otherwise appear as N
per-endpoint additions, burying the one change that caused them. So the diff:

- **does not** key `impliedRoles`;
- **does** gain a *Role hierarchy* section: edges added and removed, plus a transition
  between read and not read.

**Nothing new gates.** An added edge `ROLE_USER > ROLE_ADMIN` is a privilege escalation
across the whole application, and this ADR only *reports* it. A readable hierarchy is
the ordering over roles that [ADR 0036](0036-became-public-gates-ci.md) §2 and
`internal/diff/keys.go` say Sphinxor lacks. Using it to gate — on that edge, or on
`hasRole('ADMIN')` becoming `hasRole('USER')` — is a separate decision, **deferred by
name**, and it would rest on §2's grammar caveat.

### §10 Cerbos export: implied roles are exported, with their provenance inline

A read and applied implied role goes into the rule's `roles:` list with an inline
comment. `policy.go` already writes inline comments on role entries:

```yaml
roles:
  - "ROLE_SUPERVISOR"
  - "ROLE_ADMIN" # implied by role hierarchy: ROLE_ADMIN > ROLE_SUPERVISOR
```

Without it, the policy denies an administrator an endpoint Spring lets them call, and a
migration built on the export breaks for exactly the most privileged users.
[ADR 0009](0009-cerbos-exporter.md)'s rule is *never guess*: §1–§3 export only edges
that were read, in a grammar every version agrees on, on a layer they are shown to
reach. That is not a guess.

Where the hierarchy is detected and not applied, the export stays as today and the
**export report** says the policy may under-grant because of it. Today it says nothing.
That line is added in either stage of §12.

Cerbos derived roles (`parentRoles`) could model the relation natively. That was
rejected for this step: it is a second policy kind for a reviewer to cross-reference,
and the inline form keeps the grant and its reason on one line.

### §11 The fixture, the prediction, and the regression bar

**Fixture — `internal/extract/spring/testdata/trackr-backend`**, vendored in this PR.
Two files, MIT, commit `f985c5f`, license checked and copied per ADR 0005 (see its
`NOTICE.md`). It is the only application in the sample with roles next to a hierarchy
in source, and it carries all three hazards this ADR has to get right:

- the version-dependent grammar (§2);
- explicit method-layer wiring under the legacy enabler (§3);
- a `@Profile` on the declaring class (§1).

`TestRoleHierarchy_TrackrFixture` pinned the state before Stage 1: detected, three rows,
roles exactly as declared.

**Predicted before implementation, on that fixture, and confirmed by Stage 1:**

- still **no** expansion — §1 on the profile and §2 on the grammar, either of which
  suffices;
- the warning changes to quote both readings;
- the JSON is byte-identical.

The test's assertion on roles stands unchanged. What changes is the warning text.

**Positive expansion has no real fixture, and that is measured, not overlooked:**
Finding 3 found no project with role rows next to an unambiguous, reached hierarchy.
Its tests are synthetic, and per `docs/testing.md` each says so in its own comment. The
**parser** is extraction code and does get real input. videochat's `SecurityRoleConfig`
and its `UserRole` enum (Apache-2.0, `fd28bd4`) are the enum-concatenation case, trailing
`"\n"` included, and are vendored with the implementation.

**Regression bar:**

- the 20-repository corpus byte-identical in JSON and export, and silent, since none has
  a hierarchy;
- the existing fixtures byte-identical;
- `empty-role` at zero everywhere, as ADR 0035 §9 left it;
- on the in-scope sample: edges read for autoplan, YukeSeko, videochat, wallride and
  SMSC; **zero matrix rows changed** in all nine.

**Measured for Stage 1, 2026-09-28**, `main` at `14560dc` against this branch. The runs
covered all 20 corpus repositories at ADR 0035 §9's commits, all eleven vendored fixtures
(six Spring, five NestJS) and the nine in-scope sample applications:

- **Lint JSON:** byte-identical on all 40.
- **Cerbos policy files:** byte-identical on all 40.
- **Exit codes:** unchanged on all 40.
- **stderr:** unchanged on every corpus repository and every fixture except the two
  hierarchy fixtures.
- **Edges read:** autoplan, YukeSeko, videochat, wallride and SMSC, as predicted.
- **Both readings stated:** trackr.
- **Not read, each with its reason:** haven-platform (a project-defined class), molgenis
  (not a string), SpringUserFramework (a configuration getter).
- **Export report:** gains its *Caveats* section on exactly the three targets whose URL
  layer is analyzed — the two new fixtures and videochat. Every other sample project
  has an unanalyzed legacy URL layer, so every endpoint is already omitted and no
  caveat is added.

### §12 Staging: read now, expand when a project needs it

**Accepted as staged.** The owner accepted it on 2026-09-28, on the grounds this section
gives: an expansion that changes zero matrix rows in the sample does not meet the
project's evidence bar yet.

The design above is settled so that it does not have to be re-derived. The measurement
says its two halves differ in value today:

- **Stage 1 — read and quote.** §1, §2, and the warnings below:
  - the edges are parsed and quoted in the warning;
  - trackr's two readings are stated;
  - each not-read reason is named;
  - a `@Profile` / `@Conditional…` on the declaring class is named. The rules are still
    quoted, because quoting them asserts nothing about any endpoint;
  - where the hierarchy's reach depends on a version the source does not fix, the
    warning states the assumption it made (below);
  - the export report gains its line.

  **The assumption, stated.** Stage 1 assumes the hierarchy *does* reach every
  version-dependent layer. That is the direction in which the warning's claim — *roles
  shown may be NARROWER* — is the safe one to make. The run names each such layer and the
  version it assumed:
  - `@PreAuthorize` under `@EnableMethodSecurity`, with no 6.3 floor and no explicit
    expression-handler wiring: assumed ≥ 6.3;
  - `@Secured` / `@RolesAllowed` under `@EnableMethodSecurity`, where enabled, with no 6.3
    floor: assumed ≥ 6.3;
  - an analyzed `authorizeHttpRequests` URL layer with no 6.3 floor: assumed ≥ 6.1.

  It also says what follows on an earlier version: those roles are exact. For videochat,
  verbatim, where `@PreAuthorize` is absent from the list because its hierarchy is wired
  explicitly into its own expression handler:

  ```
  warning: this project declares a Spring Security role hierarchy in: SecurityRoleConfig: ROLE_ADMIN > ROLE_USER.
           Roles are shown as declared, without it, so they are NARROWER than what the application
           grants wherever it applies: a role placed above another also reaches every endpoint the
           lower one does. Whether it applies to @Secured and to authorizeHttpRequests rules depends
           on the Spring Security version, which nothing in the analyzed source fixes. This assumes it
           does (6.3 or later; 6.1 for authorizeHttpRequests). On an earlier version the roles shown
           there are exact.
  ```

  §4's `RoleReference.Authority` moves to Stage 2. Stage 1 has no consumer for it, and a
  field nothing reads is the `DeclaresRoles` hazard ADR 0035 §7 describes.

  This improves the output of **every** in-scope project with a hierarchy — five get
  their rules quoted, four get the reason they could not be — and changes no fact in the
  matrix, JSON, diff or policy.
- **Stage 2 — expand.** §3, §5, §6, §9's section and §10's roles. On the sample it
  changes zero rows, because the gaps in Finding 3 sit upstream of it. It becomes worth
  its model change when the first real project has role rows next to a readable, reached
  hierarchy. That is most plausibly one where [ADR 0020](0020-unanalyzable-is-unknown-not-absent.md)
  stops leaving the legacy URL layer unanalyzed, since three of the five readable
  projects keep their roles there.

Accepting this ADR accepts both stages' design and implements Stage 1. Stage 2 is
implemented when its trigger is met, or sooner by explicit decision. It is not dropped.

### §13 Version provenance, and when this must be re-checked

**Verified against Spring Security 4.2.0, 5.0.0, 5.1.0, 5.1.13, 5.2.0, 5.4.0, 5.5.0,
5.6.0, 5.7.0, 6.0.0, 6.1.0, 6.2.0, 6.2.8, 6.3.0, 6.5.11, 7.1.1 and `main`, on
2026-09-28**, by reading `RoleHierarchyImpl`, the three method-security configuration
classes, `GlobalMethodSecurityConfiguration` and the two URL authorization configurers at
each tag in `spring-projects/spring-security`. 6.5.x and 7.x are the lines currently
maintained.

What was established, and where it changes:

- **The parser changed twice.**
  - ≤ 5.0 matched `X > Y` pairs anywhere in the string.
  - 5.1 split on newlines, then on the exact separator `" > "`.
  - From 5.2 it splits on newlines, then on `\s+>\s+` (commit `be0ad673c2`). The same
    parser is still in 6.5.11, 7.1.1 and `main`.

  So **one literal means different things across 5.1 and 5.2**, and across 5.0 and 5.1
  (Finding 1). The one form all three agree on is one `X > Y` pair per line, with single
  spaces around `>`.

  The run compares only the ≤ 5.0 and ≥ 5.2 readings. A string the two agree on is read,
  even where 5.1 would differ, which happens only with irregular spacing around `>`.
  5.1 was a single minor line, and naming it in every warning would cost more clarity
  than it buys.
- **`@EnableMethodSecurity` ignores a `RoleHierarchy` bean before 6.3.** 5.6–6.2.8 never
  read one. 6.3–6.4 inject it with `ObjectProvider<RoleHierarchy>`; 6.5.11 and 7.1.1 do
  so with an `@Autowired(required = false) setRoleHierarchy`. A user-defined
  `MethodSecurityExpressionHandler` bean is used on every version from 5.6.
- **`authorizeHttpRequests` ignores a `RoleHierarchy` bean before 6.1.**
- **Legacy `@EnableGlobalMethodSecurity`** applies it to the expression handler on every
  version read, and never to `@Secured` / `@RolesAllowed`.
- **Floors and a ceiling the source can establish:**
  - `authorizeHttpRequests(…)` ⇒ ≥ 5.5;
  - `@EnableMethodSecurity` ⇒ ≥ 5.6;
  - `RoleHierarchyImpl.fromHierarchy(…)` / `withDefaultRolePrefix()` ⇒ ≥ 6.3;
  - `new RoleHierarchyImpl()` / `setHierarchy(…)` ⇒ **< 7.0**, since 7.x removed the
    public constructor and `setHierarchy`. That is why 7.x needs no special case: a 7.x
    project can only build its hierarchy through `fromHierarchy` or the builder, and
    either proves ≥ 6.3.
- **`GrantedAuthorityDefaults`** changes the role prefix the method-security handler
  applies (6.5.11, 7.1.1). §1 already stops reading when one is present.

**Re-check on every Spring Security major version.** The check means reading
`RoleHierarchyImpl`'s parse method, the `RoleHierarchy` wiring in
`PrePostMethodSecurityConfiguration`, `SecuredMethodSecurityConfiguration`,
`Jsr250MethodSecurityConfiguration` and `AuthorizeHttpRequestsConfigurer`, and whether
the public constructor is still absent. If any line above changes, this section records
the new version and the grammar and assumption rules change with it. The constants live
in one place, `internal/extract/spring/role_hierarchy.go`, which points back here.

Like ADR 0015 Amendment 1, this can be falsified by someone else's release rather than by
a change here. The difference is that the statements here are mostly positives: a parser
that exists, a setter that is wired. What can go stale is the version numbers the
warning names, not a finding.

## Alternatives considered

- **Keep ADR 0031 as is.** Rejected: 9 of the 15 applications carry a live hierarchy in
  Java and 5 are unambiguous. ADR 0031's premise of zero occurrences was a fact about
  the corpus, not about Spring applications.
- **Expand immediately, everything in §1, on every layer.** Rejected by Findings 1 and 2.
  On trackr it would assert `ROLE_ADMIN` reaches supervisor endpoints under a grammar
  that stopped meaning that in 5.1. On videochat it would apply the hierarchy to `@Secured`,
  which Spring Security 5.6–6.2 would not apply it to.
- **Read the Spring Security version from `pom.xml` / `build.gradle`.** It would settle
  Findings 1 and 2 outright, and trackr's `1.5.19.RELEASE` is right there. Rejected for
  this step: it means reading Maven and Gradle files, resolving BOMs and version
  catalogs, and handling parent POMs. That is a new extraction surface with its own
  failure modes, for one project in the sample. It is the right follow-up if Stage 2's
  trigger arrives on a project that the API-based floors in Finding 1 do not cover.
- **Expand into the agreeing subset** of an ambiguous string — `A>B` from `A > B > C`.
  Rejected: the other reading may be the one that runs, and presenting a subset as the
  hierarchy is a narrower claim that looks like a complete one.
- **A separate "Implied roles" column.** Rejected in §6.
- **Store implied roles as `RoleReference`s with a flag.** Rejected in §5.
- **Gate on hierarchy edges added.** Deferred by name in §9.

## Consequences

As implemented for Stage 1:

- `internal/extract/spring`:
  - `role_hierarchy.go` gathers hierarchy sites per file, then evaluates and parses them
    project-wide (§1–§2), and computes the §12 assumption;
  - `method_security.go` records which servlet enabler was found.
- `internal/model`:
  - `RoleHierarchyStatus` gains `Read`, `Edges`, the two readings, `NotRead`,
    `Condition` and `AssumedReach`, and there is a new `RoleHierarchyEdge`;
  - `MethodSecurityStatus` gains `Modern` / `Legacy`;
  - nothing reads either to decide a grant, a finding or a diff result.
- `internal/cli/analyze.go`: the hierarchy warning is rendered from those fields.
- `internal/export/cerbos` and `internal/report`: `Result.Caveats`, omitted from JSON when
  empty, and a *Caveats* section in the markdown report when not.
- `internal/lint`, `internal/diff`: unchanged (§8, §9).
- Fixtures: `trackr-backend` (MIT) and `videochat` (Apache-2.0), each with its license
  copied and a `NOTICE.md`.
- [ADR 0029](0029-spring-security-scope.md) §2's `RoleHierarchy` row: a literal or
  constant hierarchy is **read and stated**, and the rest stays **detected and
  announced**, with its reason.
- `docs/limitations.md` records Finding 1 and the Stage 2 deferral.
- [ADR 0005](0005-test-fixture-provenance.md)'s growth note is recounted, to ten
  repositories.

Deferred to Stage 2, and unchanged here: `RoleReference.Authority` (§4), `ImpliedRoles`
(§5), the matrix marker (§6), the diff's *Role hierarchy* section (§9) and implied roles
in the policy (§10).
