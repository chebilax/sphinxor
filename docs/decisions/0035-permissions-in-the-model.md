# 0035. The model carries a permission, not only a role — step 1: literal permissions in Spring Security annotations

## Status

**Accepted.** Amends [ADR 0002](0002-intermediate-model-structure.md).

## Context

`docs/limitations.md`'s *Permissions as metadata* entry is the largest gap that file
records, and it has deliberately sat open: *"Closing it means amending ADR 0002 to
carry a requirement that is neither a guard nor a role. That amendment has not been
written."* [ADR 0029](0029-spring-security-scope.md) §3 depends on it staying open —
its definition of done excludes reading the *content* of an announced expression
precisely so that "done" does not hang on a decision nobody has made.

This is that amendment, scoped to one step.

### What this step covers, and what it does not

**In scope.** Spring Security only, per [ADR 0029](0029-spring-security-scope.md) §1.
A permission named by a **literal** inside a Spring Security method-security
annotation: represented in the model, shown in the matrix, and with each lint rule's
treatment of it stated.

**Deferred to step 2, by name**: the Cerbos export of permissions, diff semantics for
permissions, and NestJS. Two further items are deferred *with an obligation attached*
rather than merely postponed — §3's `@ss.hasRole('admin')` literal, which the export
must classify as a role or a permission when it learns permissions at all, and §8's
list of what stays announced-not-read.

### The measurement

Run on 2026-09-21 against the 20-repository Java corpus — the 14-project Spring
survey plus dataease, halo, hertzbeat, inlong, litemall, metersphere, the same corpus
[ADR 0023](0023-third-party-authorization-annotations.md) defines. Every repository at
its then-current `main`; the commits are listed in §9 so the numbers below are
reproducible rather than approximate.

**Method.** The annotation is identified by the shipped extractor — `parseImports` +
`resolveAnnotation` + `resolveQualifiedAuth`, so [ADR 0022](0022-annotation-identity-and-unrecognized-authorization.md)'s
and [ADR 0025](0025-qualified-annotation-names.md)'s binding rules hold exactly as
they do in production, and alibaba's same-named `@Secured` is excluded for the same
reason it is excluded today. "Sits on a recognized endpoint" is resolved by running
`spring.Extract` itself and matching guard sites. Only the classification of an
expression's *content* is new, and it runs over one small expression string at a
time — not a windowed scan over a file, which is the method that produced three
successive wrong counts in `docs/limitations.md`.

#### Spring Security method-security annotations exist in 5 of the 20 repositories

**895 annotation sites.** An independent census of files importing
`org.springframework.security.access.*` / `jakarta.annotation.security.RolesAllowed`
names the same five repositories and no others.

| Repository | Sites | Form |
|---|---:|---|
| `thingsboard/thingsboard` | 536 | role expression (`hasAuthority`, `hasAnyAuthority`) |
| `apolloconfig/apollo` | 140 | bean call |
| `yangzongzhuan/RuoYi-Vue` | 116 | bean call |
| `elunez/eladmin` | 99 | bean call |
| `apache/fineract` | 4 | role expression (`hasAnyAuthority`) |
| the other 15 | **0** | — |

Every site is `@PreAuthorize`. **Spring's own `@Secured`, `@RolesAllowed` and
`@PostAuthorize` have zero uses in the corpus** — consistent with
[ADR 0030](0030-post-authorize-and-method-security-filters.md)'s own zero, and with
`imports.go`'s note that no corpus project imports Spring's `@Secured` at all. 535 of
thingsboard's 536 are method-level; so is every site in every other repository, and
the one class-level annotation is thingsboard's.

#### Of those 895, 540 are already read and 355 are not

The 540 are the role expressions (thingsboard 536, fineract 4) — `hasAuthority('SYS_ADMIN')`,
`hasAnyAuthority('ALL_FUNCTIONS', 'REGISTER_DATATABLE')`. They are ADR 0011 §1's
subset working as designed, and this ADR does not touch them.

The 355 are bean-call SpEL, in three repositories, and they divide cleanly:

| Form | Sites | Where | Example |
|---|---:|---|---|
| **A** — bean call, every argument a quoted literal | **205** | RuoYi-Vue 116, eladmin 89 | `@ss.hasPermi('system:user:edit')` |
| **B** — bean call, no arguments | **73** | apollo 63, eladmin 10 | `@unifiedPermissionValidator.isSuperAdmin()` |
| **C** — bean call, non-literal arguments | **77** | apollo 77 | `@unifiedPermissionValidator.hasModifyNamespacePermission(#appId, #env, #clusterName, #namespaceName)` |

**205 literal, 150 not.** Form A's 205 sites carry **212 literal occurrences** —
RuoYi-Vue 116, eladmin 96, the difference being six `@el.check('a','b')` calls naming
two or three permissions at once. **125 are distinct**: 72 in RuoYi-Vue, 53 in eladmin.

Three negatives, each looked for deliberately:

- **`hasPermission(...)` — Spring Security's own permission expression — has zero
  uses.** The mechanism Spring ships for exactly this is not the mechanism production
  code uses. Extending `parseSpEL` to read `hasPermission` would be a change with no
  corpus behind it, which is the mistake ADR 0011 §1 made when it cut
  `@RequestMapping(method = …)` for want of a fixture and hid 611 routes.
- **Zero string concatenation inside any Spring Security annotation in the corpus.**
  Not one `+` in any of the 895 expressions.
- **Zero permission-shaped literals in a role expression**, in the corpus's
  method-security annotations. All 982 literals inside
  `hasRole`/`hasAnyRole`/`hasAuthority`/`hasAnyAuthority` (thingsboard 974, fineract 8)
  are role-shaped; not one contains a `:`. The role slot is not silently carrying
  permissions there.

  It is carrying one somewhere else, and the exception is recorded rather than left to
  be discovered: the vendored `blog-api` fixture writes
  `.hasAuthority("entry:import")` in a `SecurityFilterChain`. That is the **URL
  layer**, which this measurement did not cover and which §2 does not touch — a
  `hasAuthority` literal stays a role wherever it appears, because nothing
  distinguishes a permission-shaped string from a role name except a convention
  Sphinxor does not read. Whether the URL layer should have a permission term at all
  is a step-2 question, and it has exactly one known instance to be decided against.

#### `#p0.id+':manage'` is not in this measurement, and the reason matters

dataease's `@DePermit({"#p0.id+':manage'"})` — 97 uses — was named as the dynamic case
to keep announced-not-read. It is not the dynamic case *inside a Spring Security
annotation*, because **dataease contains zero Spring Security annotations of any
kind**: `org.springframework.security.access` does not appear in its source. `@DePermit`
is `io.dataease.auth.DePermit`, a project-local annotation [ADR 0029](0029-spring-security-scope.md) §1
puts out of scope, and [ADR 0032](0032-controllers-that-yield-no-routes.md)'s
Consequences already record that it has no reader anywhere in dataease's own source.

The dynamic case that *is* in scope is apollo's form C, and it is dynamic in a
stronger sense than concatenation: those 77 expressions **carry no string literal at
all**. The requirement is encoded in the bean method's name and its `#param`
arguments. There is nothing to read even in principle, which is what
`docs/limitations.md` already says about apollo and which this measurement confirms
across all 140 of its sites, form B included.

So dataease stays announced-not-read for the reason it always did — a project-local
annotation is out of scope — not because of anything this ADR decides.

#### Only 68 of apollo's 140 sites sit on a recognized endpoint

The other 72 are on the inherited-interface controllers
[ADR 0032](0032-controllers-that-yield-no-routes.md) announces. Every one of
RuoYi-Vue's 116 and eladmin's 99 sits on a recognized endpoint, one site per endpoint.
That matches the models exactly: RuoYi-Vue reports 116 endpoints with an unrecovered
role list, eladmin 99, apollo 68.

#### One of the 212 literals is a role, not a permission

RuoYi-Vue's `ruoyi-generator/.../GenController.java:128` carries `@ss.hasRole('admin')` — same bean, same
shape, and the literal is a role name. It is the corpus's only such case
(115 `@ss.hasPermi`, 1 `@ss.hasRole`, 89 `@el.check`), and §3 says what happens to it.

#### There is no permission declaration site in Java source

Looked for specifically, in both repositories that carry literal permissions.
`system:user:edit` occurs in RuoYi-Vue in exactly four places: three `@PreAuthorize`
annotations on `SysUserController`, and one `INSERT` into `sys_menu` in
**`sql/ry_20260417.sql`**, which seeds the `perms` column. That `INSERT` is the
declaration — and it is a database seed, not code. eladmin is the same shape. Neither
repository declares a permission as a Java `enum` or `static final String`; a scan for
one returns only unrelated constants (`"jdbc:"`, `"https://"`, `"rmi:"`).

This is decisive for one lint rule, and §6 uses it.

#### Both beans resolve, and one attribute of them does not

`@ss` is `@Service("ss") class PermissionService`, declaring
`boolean hasPermi(String permission)` and `boolean hasRole(String role)`. `@el` is
`@Service(value = "el") class AuthorityConfig`, declaring `Boolean check(String... permissions)`.
Both are in-repo and both are resolvable by the same kind of project-wide index
ADR 0032 §1 already builds for interfaces.

What resolving them buys is smaller than it looks, and eladmin shows why:

```java
public Boolean check(String ...permissions){
    List<String> elPermissions = /* current user's authorities */;
    return elPermissions.contains("admin") || Arrays.stream(permissions).anyMatch(elPermissions::contains);
}
```

`@el.check()` with no arguments does not mean *requires nothing*. It means **requires
`admin`**, because an empty varargs array makes the second disjunct vacuously false.
Ten of eladmin's sites are exactly that. An empty argument list read as an empty
requirement would be a new instance of the defect
[ADR 0020](0020-unanalyzable-is-unknown-not-absent.md) Amendment 3 removed — and this
time on code whose semantics sit in a method body, not in the annotation.

## Decision

### §1 One new collection, mirroring `RoleReference` — its declaration counterpart deferred

`Model` gains one collection:

```
permissionReferences: [{ id, guardApplicationId, rawLiteral, via, file, line }]
```

`PermissionReference` is the exact analogue of `RoleReference`: one place in the code
where a **named requirement that is not a role** is required, attached to a
`GuardApplication`. `RawLiteral` is the literal as written. `Via` is the callee
verbatim — `@ss.hasPermi`, `@el.check` — and it is the field that keeps this
representation honest, because §3 needs it.

**`PermissionDeclaration` is deliberately not added in this step.** ADR 0002 chose
normalized collections partly so a declaration/reference split would exist for the
unreferenced-permission rule, and the symmetry argument for adding one now is real.
It is outweighed by the measurement: there are **zero** permission declaration sites
in Java source in the corpus, so the collection would be empty in every repository,
`PermissionReference.PermissionDeclarationID` would be `nil` in all 212 cases, and no
rule could consult either. The consumers a later addition would touch — `internal/diff`
(two new lists), `internal/report/diff.go`, `internal/diff/regressions.go`'s
`subjectKey`, and `internal/lint/unreferenced_permission.go` — are precisely the ones
that would have to change anyway to make the rule mean anything, so deferring costs no
extra retrofit. Diffing keys a `PermissionReference` on its literal, exactly as
`roleRefKey` keys a `RoleReference` on `normalizeRawLiteral(RawLiteral)` rather than on
a declaration ID, so nothing in step 2's diff work depends on this either.

If a corpus repository is ever found declaring permissions in code, that is the
trigger to add it, and the empty state to add it *from* is honest today.

### §2 What is read: a whole-expression bean call whose arguments are all literals

`parseSpEL` gains one recognized shape. A `@PreAuthorize` string is read as a
permission requirement when **all** of these hold:

- the whole expression is one call, `@<bean>.<method>(args)`, anchored at both ends —
  the same whole-string discipline `splitCall` already enforces, so
  `hasRole('X') or @ss.hasPermi('y')` does not match its bean-call suffix;
- the callee begins with `@`, Spring's bean-reference sigil;
- `args` is non-empty and every argument is a clean single-quoted literal, by the same
  rule `parseQuotedArgList` already applies to role arguments.

One `PermissionReference` is produced per literal, in written order.

Everything else stays exactly as it is today — **detected and announced, not read**:

- a bean call with no arguments (form B, 73 sites);
- a bean call with any non-literal argument (form C, 77 sites);
- a **negated** call, `!@bean.method(...)`. apollo has one, and it carries no literal
  so nothing turns on it in the corpus — but a negated requirement is not a
  requirement, and reading `@x.check('a')` out of `!@x.check('a')` would invert the
  fact. Stated because it is cheap to state and expensive to discover later;
- a boolean combination containing a bean call. Zero in the corpus;
- `hasPermission(...)` — Spring Security's **built-in** expression, with no `@` and no
  bean. Zero in the corpus, and not read for that reason. The leading `@` is what
  separates it from a bean whose method happens to carry the same name, and that
  collision is not hypothetical: the vendored `ruoyi-vue-pro` fixture writes
  `@ss.hasPermission('member:user:update')`, which **is** read, while
  `spel_test.go`'s existing `hasPermission('ADMIN')` case **is not**. Those two lines
  are the boundary's regression pair and both must stay;
- `@PostAuthorize`, which ADR 0030 §2 does not read at all. Zero in the corpus.

**An argument list that could not be read is never an empty requirement.** This is
ADR 0020 Amendment 3 §9's rule applied to the new term, and eladmin's `@el.check()`
is the case that proves it is not theoretical.

### §3 The bean is not interpreted, and `Via` is why that is honest

Sphinxor records that the annotation names `'system:user:edit'` via `@ss.hasPermi`. It
does **not** decide what `@ss.hasPermi` means.

The consequence is stated rather than hidden: RuoYi-Vue's one `@ss.hasRole('admin')`
is recorded as a `PermissionReference` whose literal is a role name. One of 212.

Two ways to avoid that were considered and both are rejected here:

- **Discriminating on the callee's name** (`hasPermi` → permission, `hasRole` → role)
  is the name heuristic [ADR 0023](0023-third-party-authorization-annotations.md) §1
  rejected for third-party annotations, for the reason that applies unchanged here: it
  covers the members this corpus happens to contain and silently mis-handles the next
  one.
- **Resolving the bean** and reading its method signature is the ADR 0032 §1
  discipline and it *is* feasible — both beans resolve, as the measurement shows. It
  is rejected for step 1 on cost against benefit: it answers one site in 212, it
  bottoms out in the parameter's *name* (`permission` versus `role`), which is a name
  heuristic one level deeper rather than a resolution, and eladmin shows the real
  semantics can live in a method body no signature exposes. Recorded in §8 as a step-2
  question with its own measurement, not closed.

What keeps the mislabel from misleading a reader is `Via`: the matrix renders
`@ss.hasRole('admin')`, not a bare `admin` in a column headed *Permissions*. The
collection name is inexact for one row; nothing a reader sees is.

**This is deferred, not settled, and step 2 inherits it as an obligation.** The matrix
can show `Via` and leave the question open; a Cerbos policy cannot. When the export
learns permissions it has to decide whether `@ss.hasRole('admin')`'s literal becomes a
`roles:` entry or is omitted as a permission — and the two answers produce different
policy for the same endpoint. Recorded here so that decision is made deliberately, by
the ADR that adds the export, rather than falling out of whichever branch the
implementation happens to take.

### §4 `GuardApplication` gains exactly one field, and the existing two are untouched

```
DeclaresPermissions bool
```

True when this guard's associated `PermissionReference`s constitute its requirement.
Explicit, not inferred from `len(permissionRefs) > 0`, for the reason
[ADR 0011](0011-spring-second-framework.md) §1 introduced `DeclaresRoles`: two
consumers were inferring that fact by comparing a guard name against a string, and a
second framework would have made both silently wrong rather than visibly broken.

`DeclaresRoles` and `RolesUnresolved` keep their current meaning and their current
values on every existing path. For a guard that this step now reads:

| | today | after |
|---|---|---|
| `DeclaresRoles` | true | true (unchanged) |
| `RolesUnresolved` | true | **false** — the expression was read |
| `DeclaresPermissions` | — | **true** |

**`RolesUnresolved` going false is the one hazard in this ADR, and it is nameable.**
`internal/lint/empty_role.go` fires on `DeclaresRoles && !RolesUnresolved && !FromComposite &&`
zero role references — which is precisely the state above. Left alone, this step would
produce **205 High-confidence, CI-gating findings** across RuoYi-Vue and eladmin,
reproducing the 675-finding defect ADR 0020 Amendment 3 exists to have fixed, in the
change meant to improve the same code. §6 is where that is closed, and the regression
bar in §9 is the check that it was.

**No second unresolved flag is added.** An expression that could not be read leaves
the requirement unknown without saying which *kind* of requirement it is, which is one
fact and is already spelled `RolesUnresolved`. §5 renders both columns `?` from it. A
second flag becomes necessary only when a boolean combination is read — roles
recovered, permissions not, on one annotation — which §2 does not do, and which is a
step-2 concern.

### §5 The matrix gains a column, and `?` versus `-` is per column

`report.Row` gains `Permissions []string`, rendered between `Roles` and `Findings`:

```
| Method | Path | Handler | Controller | Guards | Roles | Permissions | Findings |
```

Each cell is rendered from `Via` and the literals — `@ss.hasPermi('system:user:edit')` —
so §3's honesty survives the projection rather than depending on it.

The `?`/`-` discipline [ADR 0020](0020-unanalyzable-is-unknown-not-absent.md)
Amendment 3 §11 established applies to the new column unchanged, driven by the same
flag:

- `RolesUnresolved` renders **both** the Roles and the Permissions cell `?`. Neither
  kind of requirement was recovered, and a `-` in either would be the claim that
  decision exists to stop.
- An endpoint whose requirement *was* read as a permission gets `Roles: -` and its
  permissions in the new cell. The `-` is a real claim and it is the right one: the
  expression was read end to end and names no role. It is safe only because the
  Permissions column is adjacent and populated — a reader scanning the Roles column
  alone must not come away with "unprotected", which is why the column is added in the
  same change rather than after it.

Markdown gets the column; JSON consumers get `permissions`, omitted when empty.

### §6 Each lint rule, stated

**`mutating-endpoint-without-access-control` — unchanged, and measured at zero delta.**
Its suppression set is built from `GuardApplications`, and a permission-bearing
`@PreAuthorize` already produces one. Every endpoint this step touches is already
suppressed today. RuoYi-Vue's 15, eladmin's 20 and apollo's 50 findings are on
endpoints carrying no annotation at all, and none of them moves.

**`empty-role` — must be taught, or §4's hazard fires 205 times.** Its trigger gains
one term:

```go
if !g.DeclaresRoles || g.DeclaresPermissions || g.FromComposite || g.RolesUnresolved || refCount[g.ID] > 0 {
    continue
}
```

The rationale, not just the code: this rule's subject is *a role list the source
states as empty* — `@Secured({})`, `@Roles()`. An annotation that declares a
permission and no role has not left a role list empty; it has not declared one. That
is the same distinction [ADR 0017](0017-declaresroles-excludes-isauthenticated.md)
drew when it excluded `isAuthenticated()`, reached
from the other side. The rule's own doc comment already cites
`@PreAuthorize("@ss.hasPermi('system:user:edit')")` as the case that disproved its
original confidence rationale; this step makes that citation load-bearing rather than
historical, so the comment is updated in the same change.

The rule keeps High confidence, because §2 guarantees the state it fires on is still
"read, and empty" — a permission list that could not be read never reaches it.

**`permission-declared-but-unreferenced` — unchanged, and the measurement says why.**
The rule is named for permissions and has always operated on `RoleDeclarations`; that
mismatch predates this ADR. Extending it to permissions would require a
`PermissionDeclaration`, and §1 does not add one because there is nothing in Java
source to populate it with: RuoYi-Vue's registry is `sys_menu.perms` in
`sql/ry_20260417.sql` and eladmin's is `sys_menu.permission` in `sql/eladmin.sql`.
A rule that can never fire is worse than a rule that is honestly out of scope, so this
step leaves it alone and records the real reason in its doc comment — "no declaration
found" here is a fact about where the registry lives, not a gap in extraction.

### §7 Every consumer of the affected types, and what happens to each

The affected types are `Model`, `GuardApplication`, `RoleReference`, `report.Row`.
Enumerated from the source, not from memory — this is the `DeclaresRoles` lesson, where
two consumers inferred a fact independently and a second framework would have made both
silently wrong.

| Consumer | Reads | Step 1 |
|---|---|---|
| `extract/spring/spel.go` | — | **Changes.** One new recognized shape (§2). |
| `extract/spring/guards.go` | builds guards + role refs | **Changes.** Builds `PermissionReference`s, sets `DeclaresPermissions`, leaves `RolesUnresolved` false when read. |
| `extract/spring/roles.go` | `collectUsedRoleLiterals` | **Changes, negatively.** Permission literals must **not** enter `usedLiterals`, or a Java constant whose value is `'system:user:edit'` becomes a `RoleDeclaration`. Measured at zero occurrences today; the guard is there so it stays zero. |
| `extract/spring/authentication.go` | resolved role refs per layer suppress an `isAuthenticated()` candidate | **Changes.** A permission reference is stricter evidence on the same layer and must suppress it too. Corpus effect **zero** — no endpoint in the three repositories carries both — which is exactly why it would have gone unnoticed. |
| `extract/spring/securityfilterchain.go` | builds URL-layer role refs | **Unchanged.** The URL layer has no permission concept; §2 reads annotations only. |
| `extract/collide/collide.go` | role refs per guard, to compare two colliding routes | **Changes.** `guardSignatures` must include permissions, or two routes differing only in their permission compare equal and the collision warning is silently suppressed. Corpus effect zero, **and no fixture reaches it either** — `ruoyi-vue-pro`'s colliding sides differ by one having no annotation at all, so its tests pass with or without the change. This one needs a synthetic two-controller case, and per `docs/testing.md` the test says in its own comment that it is synthetic and why. |
| `lint/mutating_endpoint.go` | guards | **Unchanged** (§6). |
| `lint/empty_role.go` | `DeclaresRoles`, `RolesUnresolved`, role refs | **Changes** (§6). The one hazard. |
| `lint/unreferenced_permission.go` | role declarations + refs | **Unchanged** (§6). |
| `report/report.go` | guards, role refs, `RolesUnresolved` | **Changes.** New `Row.Permissions` (§5). |
| `report/markdown.go` | `renderRoles`, `renderGuards` | **Changes.** New column and `renderPermissions` (§5). |
| `report/export.go` | Cerbos result only | **Unchanged.** |
| `cli/analyze.go` | `DeclaresRoles` + guard name; `RolesUnresolved` count | **Changes in output, not in logic.** `unresolvedRoleEndpoints` falls RuoYi-Vue 116 → 0, eladmin 99 → 10, and the `ruoyi-vue-pro` fixture 5 → 0. The warning's own text must be rewritten in the same change: it currently offers *"a `@PreAuthorize` bean call such as `@ss.hasPermi('...')`"* as its example of the unreadable case, which is precisely the case this step reads. `@el.check()` or `@bean.check(#id)` replaces it. `hasMethodSecurityAnnotations` is unaffected — `DeclaresRoles` stays true. |
| `export/cerbos/translate.go` | role refs → grants; omission reasons | **Changes in what it says, not what it exports.** A permission is not a Cerbos role and nothing new is exported. But RuoYi-Vue's **113 `guarded-no-role` omissions** carry the detail *"the `@Roles()` call is empty (flagged separately as a likely mistake)"*, which becomes false the moment the permission is read. A new reason, `permission-not-exportable`, replaces it for those rows and is the hook step 2 attaches to. eladmin is unaffected — all 133 of its endpoints are already omitted as `url-layer-unknown`. |
| `export/cerbos/policy.go` | rules, omissions | **Unchanged** beyond rendering the new reason. |
| `diff/keys.go`, `diff/diff.go`, `diff/regressions.go` | role refs, guard keys, finding subjects | **Unchanged — deferred by name.** `guardAppKey` is (endpoint, guard name, scope) and none of those moves, so no existing diff result changes. A permission added or removed between two runs is invisible to `sphinxor diff` until step 2, and that is a stated gap, not an oversight. |
| `report/diff.go` | diff result | **Unchanged**, for the same reason. |
| `allowlist/*` | endpoints | **Unchanged.** Marker matching is on endpoints. |
| `extract/nestjs/*` | its own role refs | **Unchanged — deferred by name.** NestJS is step 2. |

### §8 What this step does not do, listed so it cannot be read as more

- It does not read forms B and C: **150 of the 355** bean-call sites, all of apollo's
  140 among them. Those stay `?`, and the ADR 0020 Amendment 3 warning keeps naming
  their count.
- It does not read `hasPermission(...)`, boolean combinations, or `@PostAuthorize`.
- It does not interpret the bean (§3), and does not resolve it. Whether it should is a
  step-2 question with its own measurement: the corpus has two beans, both resolvable,
  and the answer would change one site in 212. A second corpus repository using a
  differently-named bean method is what would make it worth asking.
- It does not export permissions to Cerbos, diff them, or read them on NestJS.
- It does not touch anything ADR 0029 §1 puts out of scope: Shiro's
  `@RequiresPermissions`, nacos's `@Secured`, dataease's `@DePermit`, SCDF's YAML,
  in-handler checks. Their counts in `docs/limitations.md` — 416, 419, 97, 66 — are
  unchanged by this ADR, and the entry there must not be rewritten to suggest
  otherwise.

Against `docs/limitations.md`'s figure of **1,133 permission declarations with nowhere
in the model to go**, this step gives a home to **212 literal occurrences at 205
sites**. That is the accurate fraction, and it is small because most of the 1,133 are
declared by mechanisms ADR 0029 §1 puts out of scope — not because the in-scope part
was left half-done.

### §9 The prediction, the fixtures, and what the PR must show

The check on this ADR. `sphinxor lint` and `sphinxor export cerbos` are run against
all 20 repositories at the commits below, before and after. **Eighteen repositories
must be byte-identical.**

That is measured on the **JSON** matrix, and the distinction is not a loophole: §5
adds a Markdown column, so every Markdown matrix in every project gains one header
cell and one `-` per row, by construction and regardless of whether the project has a
single permission. The JSON is the fact-level comparison — `permissions` is
`omitempty`, so an unaffected row serializes exactly as before — and it is the
stricter check of the two. A Markdown diff showing anything beyond the new column on
an unaffected project is a failure.

The baseline was taken with the shipped binary in the same session as the
measurement, and it corroborates the prediction's premises before any code is
written: **`rolesUnresolved` is non-zero in exactly three repositories** — RuoYi-Vue
116, eladmin 99, apollo 68, which are the three the measurement found bean calls in —
and **`empty-role` currently fires zero times in all twenty**, so any non-zero count
after the change is this ADR's doing and nothing else's.

| | endpoints | roles on | roles `?` | findings | `empty-role` |
|---|---:|---:|---:|---:|---:|
| RuoYi-Vue | 147 | 0 | **116** | 15 | 0 |
| eladmin | 133 | 0 | **99** | 20 | 0 |
| apollo | 224 | 0 | **68** | 50 | 0 |
| thingsboard | 551 | **519** | 0 | 14 | 0 |
| the other 16 | 4,475 | 0 | 0 | 1,313 | 0 |

| Repository | Commit | Predicted effect |
|---|---|---|
| `yangzongzhuan/RuoYi-Vue` | `13db1fc` | **116 permission references on 116 endpoints**, 116 literal occurrences, 72 distinct (71 permissions + `admin` via `@ss.hasRole`). Roles cell `?` → `-` on 116 rows. Unrecovered-requirement warning **116 → 0**. Findings **15 → 15**. Cerbos: 113 omissions change reason, 0 rules exported before and after. |
| `elunez/eladmin` | `55fbf70` | **96 permission references on 89 endpoints**, 53 distinct. Roles cell `?` → `-` on 89 rows; **10 stay `?`** (`@el.check()`, form B). Warning **99 → 10**. Findings **20 → 20**. Cerbos byte-identical (all 133 endpoints already `url-layer-unknown`). |
| `apolloconfig/apollo` | `0edf58e` | **Byte-identical.** 140 sites, 0 readable literals. Warning stays 68. Findings stay 50. |
| `thingsboard/thingsboard` | `7b7f16c` | **Byte-identical.** 536 role expressions, unaffected by §2. |
| `apache/fineract` | `c516013` | **Byte-identical.** 4 role expressions, none on a recognized endpoint. |
| `JeecgBoot`, `conductor`, `dataease`, `dolphinscheduler`, `halo`, `hertzbeat`, `inlong`, `litemall`, `metersphere`, `microcks`, `nacos`, `nakadi`, `shenyu`, `spring-cloud-dataflow`, `streampark` | see below | **Byte-identical.** Zero Spring Security method-security annotations in each. |

<details>
<summary>Remaining commits</summary>

`JeecgBoot` `8149187` · `conductor` `9ba8af9` · `dataease` `289d56b` · `dolphinscheduler` `e683256` ·
`halo` `40fcfd7` · `hertzbeat` `7177bb4` · `inlong` `13c4379` · `litemall` `a1ef964` ·
`metersphere` `53df526` · `microcks` `4a9ab4c` · `nacos` `b4fec02` · `nakadi` `09f70f8` ·
`shenyu` `65b9ef5` · `spring-cloud-dataflow` `a1216d3` · `streampark` `829466b`

</details>

**The regression bar that matters is not any of those numbers.** It is that
`empty-role` produces **zero** findings everywhere — all 20 repositories and all four
vendored fixtures, exactly as it does today. §4's hazard is a 205-instance
High-confidence regression sitting one forgotten condition away, and the prediction is
only met if that count is zero. apollo byte-identical and 18 of 20 unchanged are the
other two bars.

#### The vendored fixtures

`internal/extract/spring/testdata/ruoyi-vue-pro` is the same lineage as RuoYi-Vue and
is the one fixture this step changes. It carries **5 `@PreAuthorize` sites, all form
A**, all `@ss.hasPermission('…')` — 5 literal occurrences, **4 distinct**
(`member:user:query` appears twice). It has already once carried an instance of
exactly the false positive §4 warns about, unnoticed, until ADR 0020 Amendment 3
removed it.

| | before | after |
|---|---:|---:|
| endpoints | 11 | 11 |
| rows with `Roles: ?` | **5** | **0** |
| rows with a permission | 0 | **5** |
| findings | 5 (all Low) | **5** |
| the ADR 0020 Am3 warning | "…for 5 endpoint(s)" | **gone** |
| the route-collision warning | fires on 2 routes | **fires on 2 routes** |
| the inert-method-security warning | fires | **fires** |

`testdata/Pharmacy` — the role-model positive control — `testdata/blog-api` and
`testdata/tutorials` must be byte-identical in JSON, and differ in Markdown by the
new column alone.

**Measured after implementation, and every line above held.** The two changed
repositories and the fixture match this table exactly; the other 18 repositories are
byte-identical in JSON; `empty-role` fires zero times in all 20 and all 4 fixtures;
`RuoYi-Vue` records 116 permission references over 72 distinct literals and `eladmin`
96 over 53, which are §9's predicted distinct counts unchanged. `export cerbos` on
`RuoYi-Vue` turns 113 `guarded-no-role` omissions into 113 `permission-not-exportable`
and exports the same zero rules.

#### The tests on that fixture, updated deliberately

Checked rather than assumed. Most tests touching the fixture assert route-collision
facts (`collision_test.go`, `analyze_test.go`'s
`TestAnalyzeDirectory_GuardDifferingCollisionWarns`) and **survive unchanged** — the
two colliding sides still differ, because one carries an annotation and the other
carries none.

**One test asserts the old state directly and must be changed, not left to fail into
a fix:**

`internal/extract/spring/roles_unresolved_test.go`,
`TestRolesUnresolved_DeclaredEmptyVersusUnread` — its `unreadBeanCall` case is
`@PreAuthorize("@ss.hasPermi('system:user:edit')")` asserted as
`RolesUnresolved: true`, with the reason *"a bean-call SpEL expression is outside the
recognized subset"*. Step 1 makes that false.

The change is **not** to flip the expectation and stop there. That test's own doc
comment says the failure mode it guards is *"the two cases get conflated again"*. Its
sibling case, `@Secured(Roles.ADMIN)`, would still hold the unread side up — but the
unread side would no longer contain a **bean call**, which is the shape that supplied
the hazard in the first place. So:

- the existing `unreadBeanCall` case becomes `RolesUnresolved: false`,
  `DeclaresPermissions: true`, with its reason rewritten to say the bean call was read;
- a **new** case is added alongside it — `@el.check()`, the argument-less form B — that
  keeps asserting `RolesUnresolved: true`, so the unread half of the distinction is
  still pinned by a live assertion rather than by a comment;
- the end-to-end assertion at the bottom, `empty-role fired on exactly [declaredEmpty]`,
  **stays exactly as written**. It is the same check as §9's regression bar at unit
  scale, and it is the assertion that would have caught the 205-finding hazard.

The commit carries that reason. A test expectation changed in the same commit as the
behaviour that changed it is the one place a silent flip is indistinguishable from a
deliberate one, which is why it is written down here first.

#### One endpoint, before and after — to be shown in the PR

Counts do not show whether the output is better. `PUT /system/user` in RuoYi-Vue,
`SysUserController.java:149`:

```java
@PreAuthorize("@ss.hasPermi('system:user:edit')")
@PutMapping
public AjaxResult edit(@Validated @RequestBody SysUser user)
```

Before:

```
| Method | Path          | Handler | Controller        | Guards | Roles | Findings |
| PUT    | /system/user  | edit    | SysUserController | -      | ?     | -        |
```

After:

```
| Method | Path          | Handler | Controller        | Guards | Roles | Permissions                        | Findings |
| PUT    | /system/user  | edit    | SysUserController | -      | -     | @ss.hasPermi('system:user:edit')   | -        |
```

**This row is also the argument for §5's column being added in the same change, not
after it.** `Guards` is already `-` here — a role-declaring guard is surfaced under
Roles, not Guards, so the cell is empty by design. Read the "after" row without the
Permissions column and it is `- | -`: a stronger and entirely false claim than the `?`
it replaced. The column is what makes `Roles: -` safe, and the PR shows this row so
that is visible rather than argued.

## Alternatives considered

- **Do nothing; keep the gap open.** Rejected. ADR 0029 §3 deliberately excluded this
  question from "done" so that done was reachable, not so that it would never be
  asked. The measurement now exists, and 205 sites in two production repositories is
  the smallest, most literal slice of it.

- **Reuse `RoleReference` and call a permission a role.** Rejected: it makes the RBAC
  matrix state that `POST /system/user` requires the role `system:user:edit`, which is
  false, and it would put permission literals into `usedLiterals`, turning any Java
  constant with that value into a `RoleDeclaration`. Two wrong claims to avoid one new
  collection.

- **One generic `RequirementReference` replacing both roles and permissions.**
  Genuinely attractive — one term, one diff key, one column. Rejected because it
  re-types `RoleReference`, which 12 consumers read, to gain nothing this step needs:
  the two things really do differ downstream, most sharply at the Cerbos exporter,
  where a role is a `roles:` entry and a permission is not exportable at all. That is
  a step-2 decision made early and irreversibly.

- **Set `DeclaresRoles: false` on a guard whose expression is a pure permission call**,
  by analogy with ADR 0017's `isAuthenticated()`. It sidesteps §4's hazard entirely —
  `empty-role` needs `DeclaresRoles` and would never reach these guards. Rejected on
  its knock-on: `report.go` routes a `!DeclaresRoles` guard into the **Guards** column,
  so `PreAuthorize` would start appearing there on 205 rows where it does not today,
  and `cli/analyze.go`'s `hasMethodSecurityAnnotations` would go false for RuoYi-Vue,
  silently retiring a caveat that is correct. Three consumers changed implicitly to
  avoid changing one explicitly, which is the shape of the `DeclaresRoles` defect
  itself.

- **Discriminate permission from role by the callee's name**, or **by resolving the
  bean**. Both rejected in §3, the first on ADR 0023 §1's principle, the second on cost
  against a benefit measured at one site in 212.

- **Read forms B and C as well**, recording the bean method's name as the requirement
  (`isSuperAdmin`, `hasModifyNamespacePermission`). Rejected: a method name is not a
  permission identifier, nothing in apollo maps one to the other, and the result would
  be 150 rows of invented vocabulary in a matrix whose whole value is that its contents
  came from the source. They stay announced, which is what `docs/limitations.md`
  already says about apollo.

- **Also read `hasPermission(...)` and `@PostAuthorize`, for completeness.** Rejected
  at zero corpus occurrences. ADR 0011 §1 cut a shape for want of a fixture and hid 611
  routes; the mirror-image mistake — adding a shape for want of one — costs untested
  code on a path nothing exercises. Both stay listed in ADR 0029 §2 as they are.

- **Add `PermissionDeclaration` now, for symmetry.** Rejected in §1: zero declaration
  sites in Java source, and the consumers a later addition touches are the same ones
  the rule itself would require.

## Consequences

- `internal/model`: one collection, one field on `GuardApplication`. Both documented
  against this ADR, and `RolesUnresolved`'s doc comment gains the sentence that it now
  drives two matrix columns.
- `internal/extract/spring`: `spel.go`, `guards.go`, `roles.go`, `authentication.go`.
- `internal/extract/collide`, `internal/lint/empty_role.go`, `internal/report`,
  `internal/cli/analyze.go`, `internal/export/cerbos/translate.go`: per §7.
- `internal/diff`, `internal/extract/nestjs`: unchanged, deferred by name.
- **[ADR 0029](0029-spring-security-scope.md) §2 splits one row.** *"SpEL outside that
  subset — bean calls, boolean combinations, `#param` comparisons"* becomes two: a
  bean call whose arguments are all literals is **read**; everything else stays
  *detected and announced*. The §3 definition of done is unaffected — it never required
  reading content — and the checklist stays empty of `silent`.
- **`docs/limitations.md`'s *Permissions as metadata* entry is amended, not closed.**
  Its Spring table's RuoYi-Vue and eladmin rows change from "zero roles" to the
  permissions now recorded; its **Status** line stops saying no ADR closes the question
  and starts saying which slice this one closes and which 150 sites and four
  out-of-scope mechanisms it does not. The headline claim — that a production Spring
  application's RBAC matrix has no role data in it — survives unchanged, because
  permissions are not roles and thingsboard is still the only repository where the role
  model works.
- **A new asymmetry worth stating before it is discovered.** After this step a Spring
  project can carry a requirement the model holds, the matrix shows, and the Cerbos
  exporter cannot export. That is a new shape of incompleteness — previously, anything
  the model held was exportable — and §7 gives it a named omission reason rather than
  letting it look like a bug.
