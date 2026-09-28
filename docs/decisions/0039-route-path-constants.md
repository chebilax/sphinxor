# 0039. A route path built from constants is read, and a path with no path given is not unreadable

## Status

**Accepted** (2026-09-28), with four checks settled before implementation. They are
recorded in §9, and the measured results in §10. Narrows [ADR 0020](0020-unanalyzable-is-unknown-not-absent.md)
Amendment 1 §5 to the paths that genuinely cannot be read. It reuses and extends the
constant evaluation [ADR 0038](0038-role-hierarchy-read.md) §1 introduced.

It is sequenced before the multiple-`SecurityFilterChain` work because chain selection
happens by URL path, and that should not be designed against 637 endpoints whose path
is unknown.

## Context

### What "the route path could not be read" covers today

Measured on 2026-09-28 against the 20-repository corpus at the commits in
[ADR 0035](0035-permissions-in-the-model.md) §9, on `main` at `530b8d0`. The warning
fires on 10 repositories for **637 endpoints**. The measurement walked every
controller with the extractor's own helpers, and accounts for all of them: 639
handler routes, which ADR 0014 content negotiation merges into 637 endpoints (two
merges in conductor). They fall into three groups.

**A. No path was given at all — 66 routes, misreported.** `@GetMapping()`,
`@PostMapping()`, `@RequestMapping(method = RequestMethod.OPTIONS)`,
`@GetMapping(produces = "application/json")`. Spring maps these to the class's base
path. The extractor marks them unresolved because it treats "the annotation has
arguments, and none of them is a readable path" as "the path cannot be read"
(`subPathResolved := subPathRead || mapping.Args == nil`, and the class-level
equivalent). An empty argument list, or arguments that are only `method`, `produces`
or `consumes`, is not an unreadable path; there is no path.

| Repo | Routes |
|---|---:|
| dolphinscheduler | 33 |
| hertzbeat | 11 |
| nakadi | 7 |
| nacos | 6 |
| metersphere | 3 |
| thingsboard | 3 |
| RuoYi-Vue | 2 |
| spring-cloud-dataflow | 1 |

dolphinscheduler and metersphere are affected **only** by this group.

**B. The path is built from constants — 573 routes, in 8 repositories.**

| Form | Routes |
|---|---:|
| `Class.CONST` | 351 |
| bare identifier — same class, static import, or inherited | 113 |
| array of one literal, `{"/x"}` | 43 |
| `+` concatenation | 38 |
| array of several paths | 18 |
| `Class.CONST` on both class and method | 8 |
| other array or mixed shapes | 2 |

**C. The path is a runtime property — 10 routes, all conductor.**
`${conductor.a2a.server.basePath:/api/a2a/workflow}`. Spring resolves placeholders in
mapping paths from configuration. Two are string literals, which the extractor
**reports verbatim as the path today** — a silently wrong route. Eight more are
constants whose value contains a placeholder, which group B's resolution would otherwise
turn into the same wrong answer.

### How far ADR 0038's index gets as-is

ADR 0038's `constIndex` resolves **212 of the 573** group B routes unchanged. It fails
on the other 361 for two reasons, both measured:

- **A static import (137 routes: conductor 106, nacos 27, nakadi 4).**
  `import static com.netflix.conductor.rest.config.RequestMappingConstants.ADMIN;` then
  `@RequestMapping(ADMIN)`. The index resolves a bare identifier only within its own
  class.
- **A class simple name declared more than once (209 routes, all nacos).** nacos declares
  **nine** classes named `Constants`, and the index refuses an ambiguous simple name.
  That refusal is right, since picking one would be a guess. What resolves it is the
  file's own imports.

A throwaway prototype with Java's own name resolution resolved **all 573**: 555 to one
path and 18 to several. That resolution means:

- nested classes;
- single-type and on-demand imports;
- the same package;
- static single and on-demand imports;
- constants inherited from a supertype.

It left nothing unresolved for a reason other than the placeholders in group C. Values
were checked by hand against source:

- conductor `ADMIN` = `API_PREFIX + "admin"` = `/api/admin`, an interface constant
  reached through a static import;
- nacos `Constants.A2A.ADMIN_PATH` = `/v3/admin/ai/a2a`, a nested class whose outer
  simple name exists nine times;
- thingsboard `TbUrlConstants.RPC_V1_URL_PREFIX` = `/api/plugins/rpc`;
- spring-cloud-dataflow `UiController.WEB_UI_INDEX_PAGE_ROUTE` = `/dashboard`.

### An array is several routes

`@RequestMapping({"/a", "/b"})` maps both paths. Spring combines every class-level
pattern with every method-level pattern. The 18 array routes are **37 endpoints**. Today
they are one unresolved endpoint each, so two thirds of those routes do not appear at
all.

## Decision

### §1 A mapping with no path given has the base path, and is resolved

When a mapping annotation's arguments contain no path — no positional value, no `value=`,
no `path=` — its sub-path is empty and resolved. This applies to the class-level
`@RequestMapping` too. Only an argument that **is** a path and cannot be read leaves the
endpoint unresolved. This fixes group A: 66 routes.

### §2 Constants are evaluated with Java's name resolution

ADR 0038's constant index becomes package- and import-aware. A term is evaluated, per
ADR 0038 §1's forms, with names resolved in the order Java resolves them:

1. a local variable in the enclosing method;
2. a field of the enclosing class, then of its enclosing classes, then of its supertypes
   (resolved in the declaring file's own context);
3. a static single import, then static on-demand imports;
4. for `X.CONST`, the class `X` resolved as: a nested class of the enclosing chain, a
   single-type import, the same package, an on-demand import, or a fully qualified name
   as written.

**A name that is not found, or found more than once, is unknown — never a pick.**
Evaluation still stops at the first term it cannot evaluate, and a partial path is never
produced. Concretely, each of these is unknown, with a test apiece in
`route_constants_test.go`, the treatment ADR 0022 gave annotation identity:

| Case | Why it cannot be settled from source |
|---|---|
| a single-type import of a class outside the tree | that class is the answer, and it is not visible. A same-named class elsewhere in the tree — even in the file's own package — is not taken instead |
| two on-demand imports that both supply the class | Java itself rejects the name as ambiguous |
| an on-demand import of a package with no class in the tree | that package could declare the name. This includes `import org.springframework.web.bind.annotation.*;` |
| two static on-demand imports that both declare the constant | ambiguous, as above |
| a static on-demand import of a class outside the tree | it could declare the constant |
| a class with a supertype outside the tree, and the name not found in the known part of its hierarchy | an inherited field would shadow any static import, and it cannot be seen |

Each rule was checked by disabling it and watching exactly its own test fail.

Java compile-time constants also admit a `char` literal inside a String concatenation
(`API_PATH + '/' + API_VERSION`), which is read. `char + char` is numeric addition in
Java and stays unknown.

ADR 0038's hierarchy evaluation uses the same index. That only adds resolutions: every
hierarchy it reads today it still reads, to the same string, because a unique simple
name is the case both rules agree on. The ADR 0038 fixtures and sample are the check.

### §3 A path containing a property placeholder is unresolved

A path containing `${`, whether written literally or produced by a constant, is
unresolved, with the reason stated. It is runtime configuration, in the same class as the
configured path prefix in ADR 0020 Amendment 2 (finding H), which no static read settles.
Resolving it from `application.yml` is rejected (see Alternatives).

This moves conductor's two literal-placeholder endpoints **from resolved to unresolved**,
the one place this ADR takes something the run currently claims to know and withdraws
it. The claim was wrong.

### §4 An array of paths is one endpoint per path

As Spring combines them, one endpoint per (class path × method path × verb), each with
its own identity, as [ADR 0026](0026-requestmapping-method-attribute.md) §1 already does
for one handler with several verbs. They share the handler's source line, so a
`sphinxor-allow` marker above it exempts all of them, as it does for multi-verb routes.
18 routes become 37 endpoints.

### §5 Identity, and what `sphinxor diff` reports against an older baseline

A newly readable endpoint takes the ordinary identity, `NewEndpointID(method, path)`, or
the versioned one where ADR 0020 Amendment 2 §7 applies. ADR 0020 Amendment 1 §5
anticipated this and accepted it: *"if extraction later learns to resolve the constant,
the endpoint's ID changes and `sphinxor diff` will report it as one endpoint removed and
one added."*

So a diff between a baseline taken before this change and a head taken after it reports
every affected endpoint as removed and re-added. Predicted from the prototype, and
measured in §10:

| Repo | Removed (old synthesized ID) | Added (real path) | Why the counts differ |
|---|---:|---:|---|
| nacos | 378 | 383 | 5 array routes become 10 endpoints |
| conductor | ~142 | ~151 | 7 array routes become 14. The 8 constant placeholders stay unresolved, so their IDs do not change. The 2 literal placeholders move the other way (§3) |
| thingsboard | 39 | 41 | 1 route of 3 paths |
| dolphinscheduler | 33 | 33 | all group A |
| hertzbeat | 13 | 13 | |
| nakadi | 11 | 11 | |
| spring-cloud-dataflow | 6 | 6 | 1 array adds one endpoint, and 1 newly readable path merges with an existing same-controller endpoint (ADR 0014) |
| shenyu | 3 | 6 | 3 routes of 2 paths |
| RuoYi-Vue | 3 | 4 | 1 route of 2 paths |
| metersphere | 3 | 3 | all group A |

The implementation measures the exact numbers, which go into the release notes with
this table.

**This is not expected to fail a build.** Measured, not assumed:

- every finding on the affected endpoints today is low-confidence
  `mutating-endpoint-without-access-control`, and the diff gates only on high-confidence
  findings;
- ADR 0036's became-public gate compares endpoints present on **both** sides, which a
  re-identified endpoint is not.

The cost is noise in the first diff after upgrading. The release notes say so, and say
the remedy: re-baseline on the new version.

**One new collision:** conductor declares `GET /api/secrets` in both `SecretResource` and
`SecretController`, one of them through a constant. It was invisible while that path was
unreadable. ADR 0020 Amendment 2 §8 handles it: two endpoints, and a warning if their
guards differ.

### §6 What changes for each consumer

- **`sphinxor lint`**
  - The matrix shows the real paths.
  - The *route path could not be read* warning falls from 637 endpoints in 10
    repositories to the conductor placeholders, and says why they are unreadable.
  - Findings keep firing on the same handlers, under new IDs.
  - Exit codes are unchanged: the affected findings are low-confidence.
- **JSON:** `path` and `pathUnresolved` change on those endpoints, and array routes add
  endpoints.
- **`sphinxor export cerbos`**
  - Those endpoints stop being omitted for their path and go through normal translation.
  - Of the 10 repositories, the export runs only where the URL layer is analyzed or
    absent: conductor, hertzbeat and RuoYi-Vue. None of their affected endpoints has a
    role (RuoYi-Vue's carry permissions, conductor's and hertzbeat's nothing), so the
    **predicted** change is in omission *reasons* in the report, not in policy files.
    The implementation measures it.
- **`sphinxor diff`:** as §5.

### §7 What stays unresolved, and says why

A path remains unresolved when it contains a placeholder (§3), or when evaluation reaches:

- a constant declared outside the analyzed tree — a dependency's class;
- a method call;
- a non-`final` field;
- an ambiguous name;
- a text block;
- Kotlin.

The warning names the reason category, not just "not a string literal". None of these
occurs in the corpus except the placeholders.

### §8 Fixtures

Real input for what the corpus showed and no current fixture has. Both repositories are
Apache-2.0; each fixture carries the license, and nacos also carries its upstream
`NOTICE` file (§4(d)). Per ADR 0005 Amendment 1, the size is stated.

| Fixture | Files | Lines | Covers |
|---|---|---:|---|
| conductor @ `9ba8af9` | `AdminResource.java`, `RequestMappingConstants.java`, `A2AServerResource.java` | 376 | static import of an interface constant built by concatenation, and a placeholder path (§3) |
| nacos @ `b4fec02` | `A2aAdminController.java`, `ai/constant/Constants.java`, `lock/constant/Constants.java` | 650 | a nested-class constant whose outer simple name is declared twice in the fixture — the second `Constants` is the smallest of nacos's nine, included as the decoy that makes the simple-name index refuse |

`testdata/` grows from 2,747 to about 3,770 lines of vendored source, and from 161 KB to
about 230 KB, or roughly 12% of the tracked tree. Every ADR 0005 Amendment 1 trigger
stays unmet: under 1 MB and 25%, and no fixture edited. Group A and arrays of literals
need no new fixture; the existing fixtures and synthetic tests cover them, and each test
says so.

### §9 The checks settled before implementation

- **The nacos decoy is a real, unmodified file.** `lock/src/main/java/com/alibaba/nacos/lock/constant/Constants.java`
  was fetched from GitHub at `b4fec02` and diffed against the vendored copy: identical.
  It is one of the nine classes named `Constants` that nacos really declares, chosen as
  the smallest. Nothing was written or edited for the test. The duplicate-name *rule* is
  also pinned synthetically (§2's table), and those tests say they are synthetic.
- **Ambiguity stays unknown** — §2's table, one test per case.
- **The Cerbos export delta is measured per repository** — §10.
- **"The hierarchy reader can only resolve more" is verified, not assumed.** All nine
  in-scope hierarchy-sample applications, the two sample-app extras and both hierarchy
  fixtures were re-run. **13 of 13 hierarchy warnings are byte-identical**: none
  different, and none newly resolved. The old index never resolved a hierarchy to a
  wrong class. Where it could not settle a name, the new one could not either, on this
  sample.
- **Fixture file counts against ADR 0005 Amendment 1's trigger of about 20 files from
  one repository:** conductor 3, nacos 3.

### §10 Measured, 2026-09-28

`main` at `530b8d0` against this implementation. The runs covered all 20 corpus
repositories at ADR 0035 §9's commits, every fixture, and the ADR 0038 sample.

**Unchanged:**
- the ten corpus repositories with no unreadable path: lint JSON, policy files, export
  report and stderr all byte-identical;
- every pre-existing fixture: byte-identical;
- **exit codes**, everywhere;
- in the corpus, only the unresolved-path warning's lines moved.

**Identity churn — what `sphinxor diff` reports against a baseline taken before this
change:**

| Repo | Removed | Added | Endpoints | Unresolved paths |
|---|---:|---:|---|---|
| nacos | 378 | 383 | 429 → 434 | 378 → 0 |
| conductor | 146 | 151 | 184 → 189 | 148 → 6 (placeholders) |
| thingsboard | 39 | 39 | 551 → 551 | 39 → 0 |
| dolphinscheduler | 33 | 33 | 239 → 239 | 33 → 0 |
| hertzbeat | 13 | 13 | 175 → 175 | 13 → 0 |
| nakadi | 11 | 11 | 51 → 51 | 11 → 0 |
| spring-cloud-dataflow | 6 | 4 | 115 → 113 | 6 → 0 |
| shenyu | 3 | 6 | 394 → 397 | 3 → 0 |
| RuoYi-Vue | 3 | 4 | 147 → 148 | 3 → 0 |
| metersphere | 3 | 3 | 1053 → 1053 | 3 → 0 |

Across the corpus the unreadable-path count falls from **637 to 6**, all six conductor
placeholders.

**Cerbos export delta — the check the ADR had missed.**
- **Exported rules: 0 before and 0 after, in every corpus repository**, so no policy
  grants anything new.
- Policy files still differ where the endpoint set changed, because omitted endpoints
  appear as comments in them.
- What moves is the omission *reason*:

| Repo | Omissions | Reasons, before → after |
|---|---|---|
| conductor | 184 → 189 | `path-unresolved` 148 → 6, `no-guard` 36 → 181, `route-collision` 0 → 2 (`GET /api/secrets`, two controllers) |
| hertzbeat | 175 → 175 | `path-unresolved` 13 → 0, `no-guard` 162 → 175 |
| RuoYi-Vue | 147 → 148 | `path-unresolved` 3 → 0, `permission-not-exportable` 113 → 117 |
| nacos, thingsboard, shenyu, nakadi, spring-cloud-dataflow, dolphinscheduler, metersphere | counts follow the endpoint count | `url-layer-unknown` throughout: these exports omit every endpoint for their URL layer, before and after |

In the ADR 0038 sample, videochat is the one place rules appear: **0 → 3 exported
rules**, and 29 of its endpoints now show an `action-collision` omission instead of
`path-unresolved`.

**Two corrections of silently false paths:**
- conductor's 2 literal-placeholder endpoints, reported until now as
  `/${conductor.a2a.server.basePath:/api/a2a/workflow}` and similar;
- in the sample, SpringUserFramework's 13.

Each is now unresolved with its reason. This is the one direction in which the change
withdraws something the run used to assert.

**Same-path merges (ADR 0014).** A path that becomes readable can equal a sibling's in
the same controller, and ADR 0014 then merges them into one endpoint:
- spring-cloud-dataflow 1, thingsboard 1;
- in the sample, wallride 41 and molgenis 15, where Spring distinguishes the handlers by
  `params=` (`params="draft"`, `params="publish"`).

A merged endpoint carries every merged handler's guards, so a merge could hide an
unguarded variant. Measured: **in all 58 merges, the merged handler's guards equal every
same-verb sibling's**, so nothing is hidden here. ADR 0014's rule is applied unchanged.
Whether handlers distinguished only by `params` or `headers` should be separate endpoints
is ADR 0014's question, not this one, and it is recorded as open in `docs/limitations.md`.

**Fixtures.**
- `testdata/` grows to **3,773 lines** of vendored source and **233 KB**, which is
  **11.9%** of the tree.
- All fixtures analyze in well under a second.
- No ADR 0005 Amendment 1 trigger is met.

## Alternatives considered

- **Keep ADR 0038's simple-name index as-is.** Resolves 212 of 573. Rejected: the other
  361 fail on static imports and on duplicated simple names, both of which Java's own
  rules settle without guessing.
- **Resolve placeholders from `application.yml` / `application.properties`.** Rejected:
  profiles, environment overrides, and config outside the repository make the value a
  deployment fact rather than a source fact. It is the ADR 0020 Amendment 2 finding H
  reasoning, unchanged. The placeholder's default value (`:/api/a2a/workflow`) is also
  not used, because the default applies only when the property is unset, which the source
  cannot tell.
- **Keep only the first path of an array.** Rejected: it hides real routes, the failure
  ADR 0026 §1 fixed for verbs.
- **A separate ADR for group A.** Rejected: it is a two-line fix with the same identity
  consequence and the same release note, and splitting it would mean two rounds of diff
  noise for the same users.
- **Keep the old synthesized identity for newly readable endpoints**, to spare the diff.
  Rejected by ADR 0020 Amendment 1 §5 itself: a path-derived identity is what makes two
  endpoints on the same route collide correctly, and a synthesized one on a readable path
  would hide exactly the `GET /api/secrets` collision §5 found.

## Consequences

- `internal/extract/spring`:
  - `controllers.go` stops treating "no path argument" as unreadable (§1), expands
    arrays (§4), and evaluates constants (§2);
  - `role_hierarchy.go`'s index becomes package- and import-aware and moves to a shared
    file;
  - placeholders are recognized (§3).
- `internal/model`: `Endpoint` gains the reason a path is unresolved, for the warning. No
  other field changes.
- `internal/cli/analyze.go`: the unresolved-path warning names reasons (§7).
- `internal/export/cerbos`, `internal/lint`, `internal/diff`: no code change; they see
  more resolved endpoints.
- `docs/limitations.md`: the *route path declared with a constant* entry shrinks to §7's
  list.
- The CHANGELOG and release notes carry §5's per-repository table as measured, and the
  instruction to re-baseline.
- The regression bar:
  - the unresolved-path count on the corpus falls to the conductor placeholders;
  - lint exit codes unchanged on all 20;
  - policy files unchanged, or each change explained;
  - the ADR 0038 hierarchy sample reads the same edges;
  - every other fixture byte-identical.
