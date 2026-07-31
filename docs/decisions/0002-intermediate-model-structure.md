# 0002. Intermediate model structure

## Status

Accepted.

## Context

Per [`0001-target-framework-choice.md`](0001-target-framework-choice.md), extraction targets NestJS: controllers (`@Controller()`), routes (`@Get()`/`@Post()`/etc.), guards (`@UseGuards()`), and role-style decorators (`@Roles()`, `@Public()`, or whatever convention a given project uses — Nest itself ships no canonical role decorator, only the `Reflector`/metadata mechanism apps build their own on top of).

The model this extraction produces has to serve four consumers, not just one:

1. The three v0.1 lint rules (`vision.md`): mutating endpoint with no detected access control; permission declared but never referenced; empty role.
2. The v0.1 output: an RBAC matrix in Markdown + JSON.
3. The allowlist mechanism ([`0003-allowlist-format.md`](0003-allowlist-format.md)), which needs a stable way to identify *which* endpoint an allowlist entry exempts.
4. v1's drift detection — diffing two versions of the model to surface new/removed permissions and endpoints that became public. This is explicitly on the roadmap (`vision.md`, v1), not a hypothetical: the model's shape should not make that diffing gratuitously harder, even though diffing itself is not built in v0.1.

Two of the three lint rules (`permission declared but never referenced`, `empty role`) require an aggregate view across the whole codebase, not just a per-endpoint one — this pushes the design toward *something* queryable globally, not purely a per-file tree.

A separate wrinkle: NestJS has no built-in notion of a "permission" or a canonical role registry. A project might declare its roles as a TypeScript `enum`, as string constants, or nowhere explicit at all (bare string literals passed straight to `@Roles('admin')`). "Permission declared but never referenced" only means something if there's a declaration site to compare usage against — when a project has no such site (bare string literals only), that rule can't fire, and the model has to represent "no declaration found" honestly rather than inventing one.

## Decision

**Option B — normalized, ID-referenced collections.**

Both `vision.md`'s stated priorities beyond v0.1 depend on this shape directly: v1's diff engine needs to compare two versions of the model cheaply enough to run in CI on every PR, and the v0.1 lint rules for unreferenced permissions and empty roles need direct, aggregate queries rather than a tree walk. A nested tree's one real advantage — mapping close to 1:1 onto the Markdown/JSON RBAC matrix output — is cheap to recover the other way: projecting normalized collections into a display tree at serialization time is a small, one-directional transform. The tree's cost, by contrast, is recurring: positional/synthetic-key matching for every diff, and flattening for every aggregate query, hits exactly the two things `vision.md` calls out as this project's core value (drift detection, and lint rules that need a global view), not incidental overhead.

### Option B — Normalized, ID-referenced collections (chosen)

Flat collections, each entity with a stable ID, referencing each other by ID rather than by nesting — closer to a small in-memory relational model:

```
endpoints:        [{ id, httpMethod, path, handlerName, controllerId, file, line }]
guardApplications: [{ id, endpointId, guardName, appliedAt: class|method }]
roleDeclarations:  [{ id, name, declaredIn: file/line, kind: enum|const|none-found }]
roleReferences:    [{ id, guardApplicationId, roleDeclarationId|null, rawLiteral }]
findings:          [{ id, ruleId, confidence, subjectId, subjectKind, allowlisted: bool }]
```

- **Pro**: the aggregate lint rules are a direct query ("which `roleDeclarations` have zero `roleReferences` pointing at them") instead of a tree walk. Each collection diffs independently against its counterpart in another run by comparing stable keys (e.g. `httpMethod+path` for endpoints, `name` for role declarations) — this is close to what v1's diff engine will need to do anyway, so the normalized shape does that work once instead of deferring it. `roleReferences.roleDeclarationId: null` gives an honest, explicit representation of "reference with no matching declaration found," instead of silently having no way to express it.
- **Con**: more upfront ceremony (five collections instead of one tree) for a v0.1 that only needs to run three lint rules and print one matrix. Serializing to the Markdown/JSON matrix output requires a join step that a tree would get for free — accepted as the cheaper cost, per the Decision above.

### Option A — Nested tree (rejected)

One JSON document per analysis run, shaped around what the Markdown/JSON RBAC matrix output looks like: a list of controllers, each containing its endpoints, each carrying its resolved guards and role references inline.

```
Controller
├─ path, filePath
└─ Endpoint[]
   ├─ httpMethod, path, handlerName, line
   ├─ confidence-graded AccessControlStatus (protected / unprotected / allowlisted)
   └─ Guard[]
      ├─ name, source (class-level | method-level)
      └─ RoleReference[] (role name as written in code)
```

- **Pro**: maps almost 1:1 onto the output format — serializing this to the RBAC matrix is close to a direct walk. Easiest to reason about for a single endpoint.
- **Con**: the two aggregate lint rules (unreferenced permission, empty role) require flattening the whole tree at query time anyway, since role declarations and role references live in different, non-adjacent parts of the tree (declaration is usually a top-level enum, references are nested three levels down under specific endpoints). Diffing two trees in v1 means matching nodes positionally or by a synthetic key (path + method) at every level — doable, but the matching logic isn't handed to you by the shape. Rejected for exactly this reason.

### Shared points, independent of the option chosen

- Every `Endpoint` needs a **stable identity** usable by the allowlist mechanism and (later) by drift diffing. `httpMethod + full path` is the natural candidate for NestJS since Nest routes are declared, not dynamically generated in the common case — but this identity breaks if a route's path is renamed between two versions, which is an honest limitation to state (not silently paper over) once v1 diffing exists.
- `Finding.confidence` is a field on the model, not a decision made here — its exact levels (e.g. how many grades, what each means) are deliberately left to a separate future ADR, per your scoping. This ADR only commits to *a* confidence field existing on findings.
- Role declarations extracted from something other than a literal `enum` (e.g. a `const` object, or no declaration at all) are represented as best-effort, with an explicit "no declaration found" state — never inferred into existence. This keeps the model honest about what it actually found versus what it's guessing.

## Consequences

The normalized collections above become the shape every NestJS extractor, all three v0.1 lint rules, the RBAC matrix serializer, and the allowlist matcher are built against. The RBAC matrix output (Markdown + JSON) is produced by a join/projection step over these collections rather than being the model's native shape — that projection is implementation, not a further design decision. Retrofitting a tree shape later would mean touching the extractor, all three lint rules, the serializer, and the allowlist matcher at once — this is exactly the kind of decision `vision.md` and `CONTRIBUTING.md` call out as needing an ADR *before* code, not after.

**Amended by** [`0006-composite-decorator-resolution.md`](0006-composite-decorator-resolution.md), which adds a `FromComposite` field to `guardApplications`. Recorded here so the model's current shape is traceable from this ADR, not just discoverable by reading a later one.
