# 0041. Permissions reach the Cerbos export only through semantics the owner declares

## Status

**Proposed.** It completes the last item [ADR 0035](0035-permissions-in-the-model.md)
deferred to step 2 by name, and settles two obligations that ADR left open:

- **§3's obligation.** RuoYi-Vue's `@ss.hasRole('admin')` is recorded as a permission.
  Is it exported as a role, omitted, or exported with a caveat?
- **Amendment 1's superuser escape.** Every corpus bean behind a read permission also
  admits a principal the call does not name. A policy built from the literal alone
  states a requirement stricter than the bean enforces, and a deployable artifact
  carries that more heavily than a warning does.

It amends [ADR 0009](0009-cerbos-exporter.md) by adding a way to export an endpoint whose
requirement is a permission. ADR 0009's safety posture is unchanged.

## Context

### What the model holds, and what the export does with it today

Measured on 2026-09-28 at `b996b04`, across the 20 corpus repositories at
[ADR 0035](0035-permissions-in-the-model.md) §9's commits and the vendored fixtures.
Three projects carry permission references:

| Project | References | Endpoints | Callees | Export today |
|---|---:|---:|---|---|
| RuoYi-Vue | 117 | 117 | `@ss.hasPermi` 116, `@ss.hasRole` 1 | all 117 omitted, `permission-not-exportable` |
| eladmin | 96 | 89 | `@el.check` 96 | all 89 omitted, `url-layer-unknown` (ADR 0040's unreadable single chain) |
| `ruoyi-vue-pro` fixture | 5 | 5 | `@ss.hasPermission` 5 | 3 `permission-not-exportable`, 2 `route-collision` |

Every literal is a single token: no comma-joined set, and no empty string among the read
references. No endpoint carries both a role and a permission, and none has
permission-bearing guards at both the class and the method level. eladmin has **6** calls
naming several literals, `@el.check('job:list','user:list')`, whose bean admits a
principal holding **any** of them.

**RuoYi-Vue is the only corpus project whose permission endpoints the export could reach**,
since its URL layer is analyzed (`anyRequest().authenticated()`). It is also the most
common authorization shape in the corpus, and today it exports nothing.

### The callee's name does not say what the bean does

RuoYi's bean, `PermissionService`, declares six methods:

- `hasPermi` and `hasAnyPermi(String)`, where `hasAnyPermi` takes one **comma-joined**
  string;
- `lacksPermi`, a **negation**;
- `hasRole`, `lacksRole` and `hasAnyRoles`.

Only two are used in the corpus. An export that interpreted callees by name would turn
the first `@ss.lacksPermi('x')` it met into a grant for holders of `x`, the exact
inversion. This is why ADR 0035 §3 refused the name heuristic ([ADR
0023](0023-third-party-authorization-annotations.md) §1), and exporting does not make it
safer.

### The superuser escape, in a real engine

ADR 0035 Amendment 1 read the escape at source:

| Callee | Also passes when | Who holds that |
|---|---|---|
| `@ss.hasPermi` | the permission set contains `*:*:*` | RuoYi user id 1 |
| `@ss.hasRole` | a role key equals `admin` | RuoYi role 1 |
| `@el.check` | the authorities contain `admin` | every eladmin `is_admin` user |

Checked with the pinned Cerbos CLI (0.55.0, the version CI uses), by `cerbos compile` with
a policy test suite, as [ADR 0010](0010-authenticated-any-role.md) checked `roles: ["*"]`.
25 test cases, all passing, established:

- A permission is expressible as a rule with `roles: ["*"]` and the condition
  `"system:user:edit" in P.attr.permissions`. A principal without the attribute is denied.
- **A policy built from the literal alone denies RuoYi's superuser**, a principal whose
  permissions are `["*:*:*"]`. It is stricter than the bean, in a real engine.
- An escape written into the condition —
  `any.of: ["*:*:*" in P.attr.permissions, "admin" in P.roles, <literal>]` — admits
  exactly what the bean admits.
- Several literals combine with `any.of`, eladmin's semantics.
- `@ss.hasRole('admin')` exported as `roles: ["admin"]` admits exactly role `admin`.

So the escape can be represented faithfully, but only by someone who knows what it is.

### Resource/action granularity limits the yield

The exporter keys a rule by (controller, HTTP verb) ([ADR 0009](0009-cerbos-exporter.md)
§2). When two endpoints of one controller and verb need different permissions, that is an
action collision, and both are omitted. For RuoYi-Vue's 117 permission endpoints:

| | Resource/action groups | Endpoints |
|---|---:|---:|
| one requirement per group — exportable | 36 | **54** |
| differing requirements — collide | 22 | 63 |

The `@ss.hasRole('admin')` endpoint is one of the 63: `GenController POST` also holds
`tool:gen:import`. **§3's obligation therefore has no effect on the corpus today**, but
it still has to be settled, because the next project will not be so obliging.

## Decision (proposed)

### §1 By default nothing changes

Without the declarations below, no permission is exported, exactly as today. The omission
detail gains one sentence naming the flags that would export it. On all 20 corpus
repositories the default output is byte-identical.

### §2 The owner declares what a callee means; Sphinxor never infers it

Three repeatable flags on `sphinxor export cerbos` declare a callee's semantics. The
callee is matched exactly against `PermissionReference.Via`:

| Flag | Declares |
|---|---|
| `--permission-callee '@ss.hasPermi'` | the literal is a permission the principal must hold |
| `--role-callee '@ss.hasRole'` | the literal is a role the principal must hold |
| a `:any-of` or `:all-of` suffix on either | how a call naming several literals combines |

Undeclared callees stay omitted, under a new reason, `callee-not-declared`, which replaces
`permission-not-exportable` for them.

A call naming several literals is exported only when its callee carries a suffix. Neither
reading is the natural default: `@el.check` is any-of, and another bean could be all-of.

The declaration is a statement **about the application**, made by the person who knows
it. It is exactly as reliable as that person's knowledge. The export report prints the
declarations verbatim, so a reviewer of the policy sees the assumptions it rests on.

### §3 The superuser escape must be declared or explicitly waived

Declaring any callee also requires one of:

- `--superuser-permission '*:*:*'` or `--superuser-role admin` (each repeatable): the
  escape the bean honours. It is added to every exported permission and role rule, as the
  `any.of` terms verified above;
- `--no-superuser-escape`: an explicit statement that the bean has none.

With neither, **the command fails** before writing anything, and the error cites ADR 0035
Amendment 1: every bean behind a permission in the corpus has an escape, so a silent
default would be wrong three times out of three.

**Why not export the literal with a caveat**, the treatment ADR 0038 §10 gave the role
hierarchy? The error runs in the direction ADR 0009 calls safe: a stricter policy denies.
But it denies precisely the most privileged users, in a file meant to be deployed. A
report line is read once; a policy is enforced on every request. A warning suffices for a
matrix that informs a human. A deployable artifact should not rest on an assumption nobody
made, so the owner has to make it.

### §4 The Cerbos shape

- **A permission** becomes `roles: ["*"]` with the condition
  `"<literal>" in P.attr.permissions`. With escapes, or several literals, that becomes an
  `any.of` (or `all.of` for `:all-of`) of those terms.
- **A role** becomes `roles: ["<literal>"]`, plus any declared superuser roles.
- **The integration contract**, stated in the export report: the calling service passes
  the principal's permission strings, as granted and before any bean logic, as
  `attr.permissions`. The escape is modelled in the policy, so it must not also be
  expanded in the attribute. The attribute name is fixed at `permissions`; a flag to
  rename it waits until a user needs one.
- **Combination with the URL layer is ADR 0012's, unchanged.** RuoYi-Vue's
  `authenticated()` is implied by `roles: ["*"]`. A URL-layer role list, where one exists,
  becomes the rule's `roles:` with the permission as its condition.

### §5 ADR 0035 §3's obligation: settled by declaration, not by name

- **`@ss.hasRole('admin')` becomes a role if and only if the owner declares `@ss.hasRole`
  a role callee.**
- If the owner declares it a *permission* callee, that is their statement, and it is
  exported as one. The report shows it.
- **Undeclared, it is omitted** (`callee-not-declared`), never exported as a permission
  by default.

This meets ADR 0035 §3's requirement that the decision be made deliberately, and it keeps
§3's refusal of the name heuristic intact. Sphinxor still decides nothing about what
`hasRole` means. In the corpus the question is moot today, because that endpoint collides
on `GenController POST` (see Context).

### §6 Everything else is unchanged

The model, extraction, the matrix, `sphinxor lint`, `sphinxor diff` and the default
export are unchanged. The permission warning of ADR 0035 Amendment 1 still fires: a
declaration informs the export, not the lint report.

## Alternatives considered

- **Interpret callees by name** (`hasPermi` → permission, `hasRole` → role). Rejected: the
  same bean defines `lacksPermi` and a comma-joined `hasAnyPermi`, so a name reading would
  invert one and mis-split the other. ADR 0023 §1 and ADR 0035 §3 refused this for reading,
  and exporting raises the stakes.
- **Read each bean's body** to find the semantics and the escape. Rejected, as ADR 0035 §3
  rejected it for extraction. RuoYi's and eladmin's escapes are in the bean. yudao's is
  deeper, in a permission service the bean calls. A pattern-matcher over method bodies
  would be right until the first bean it half-understands.
- **Export the literal with a Caveats line.** Rejected in §3: a deployable policy that
  denies the superuser is a heavier claim than a report line can offset.
- **Export permissions as Cerbos role names** (`roles: ["system:user:edit"]`). Rejected:
  it asserts roles the application does not have, and pushes a permission-to-role mapping
  onto every integrator.
- **A configuration file instead of flags.** Deferred: three settings do not justify a new
  file format, which is harder to reverse. If a fourth setting arrives, that decision gets
  its own ADR.
- **Derived roles** (a `has_system_user_edit` derived role per permission). Rejected for
  now, for the reason ADR 0038 §10 gave: a second policy kind to cross-reference, where an
  inline condition keeps grant and reason on one line.
- **Finer actions, to lift the 63 collisions.** Out of scope: ADR 0009 §2's coarse action
  vocabulary is a known limit, and changing it changes every export, not only permissions.

## Consequences

- `internal/cli/export.go`: the flags of §2 and §3, and the refusal when neither escape
  flag is given.
- `internal/export/cerbos`:
  - `Translate` takes the declarations;
  - permission and role rules per §4;
  - `callee-not-declared` replaces `permission-not-exportable` where no declaration
    applies;
  - `Result` gains the declarations and the integration contract, printed by the report.
- `internal/model`, `internal/extract`, `internal/lint`, `internal/diff`: unchanged.
- Regression bar:
  - **default:** byte-identical on all 20 corpus repositories and every fixture;
  - **RuoYi-Vue, declared** (`--permission-callee '@ss.hasPermi' --role-callee
    '@ss.hasRole' --superuser-permission '*:*:*' --superuser-role admin`): 36 rules for
    54 endpoints predicted, 63 action-collision omissions. It must pass `cerbos compile`
    with a test suite asserting the superuser is admitted and a principal without the
    permission is not;
  - **the `ruoyi-vue-pro` fixture, declared:** its 3 `permission-not-exportable` endpoints, measured for rules versus action collisions;
  - **a missing escape flag:** the command fails with the ADR 0035 Amendment 1 message.
- The first corpus project to export rules for its main authorization shape. The policy
  says, in the report, whose statement its semantics are.
