# 0037. The diff compares permissions — step 2a

## Status

**Proposed.**

Completes one of the three items [ADR 0035](0035-permissions-in-the-model.md)
deferred to step 2 by name. The Cerbos export of permissions and NestJS permissions
are the other two and are **not** in this ADR.

## Context

[ADR 0035](0035-permissions-in-the-model.md) put `PermissionReference` in the model
and `Permissions` in the matrix. Its §7 consumer table ends the `internal/diff` row
with a sentence that is the whole reason this ADR exists:

> **Unchanged — deferred by name.** `guardAppKey` is (endpoint, guard name, scope)
> and none of those moves, so no existing diff result changes. A permission added or
> removed between two runs is invisible to `sphinxor diff` until step 2, and that is
> a stated gap, not an oversight.

That gap is real and it is measurable. Confirmed against the shipped binary at
`98c638d`, not reasoned about: on RuoYi-Vue at `13db1fc`, editing
`SysUserController.java:149` from `@ss.hasPermi('system:user:edit')` to
`@ss.hasPermi('system:user:audit')` and diffing base against head produces a report
whose every section reads *No change*. The endpoint is the same,
the guard application is the same (`@PreAuthorize`, method scope), no role reference
moved, and nothing in `Result` has a slot for the one thing that did change.

This matters more on the two repositories that have permissions than the ratio
suggests. RuoYi-Vue and eladmin report **zero roles** and rely entirely on the
permission column; for those projects, "the diff compares roles" means the diff
compares nothing about what their endpoints require.

### What this ADR must settle, and what it must not reopen

Three things, mapping onto ADR 0007's own three:

1. **A derived stable key for `PermissionReference`.** Its `ID` is a per-run
   sequential value, exactly as `GuardApplication`'s and `RoleReference`'s are, so
   [ADR 0007](0007-model-diff-design.md) §2's problem statement applies verbatim and
   its answer — derive a key at comparison time, never store one — is the approach to
   follow rather than to re-decide.
2. **How a permission appears in the structural diff.**
3. **Whether any permission change gates CI.**

Point 3 is where most of the risk sits, and most of it is already settled elsewhere.
[ADR 0036](0036-became-public-gates-ci.md) §2 defines protection as *presence, never
content*, and lists a `PermissionReference` as its term 3. That decision is not
reopened here. What this ADR does is state what follows from it for the changes only
a permission diff can see, and check each one against the shipped code rather than
against the reading of it.

### The measurement

Run on 2026-09-22 against the same 20-repository Java corpus at the same commits
[ADR 0035](0035-permissions-in-the-model.md) §9 lists, with a probe placed inside
`internal/diff` so it reuses the real `extract.Run` rather than a scan.

| Repository | permission refs | distinct keys under §1 | key collisions | orphaned refs | callees |
|---|---:|---:|---:|---:|---|
| `yangzongzhuan/RuoYi-Vue` | 116 | **116** | **0** | 0 | `@ss.hasPermi` 115, `@ss.hasRole` 1 |
| `elunez/eladmin` | 96 | **96** | **0** | 0 | `@el.check` 96 |
| the other 18 | 0 | 0 | 0 | 0 | — |

Three facts this establishes, each used below:

- **The proposed key is injective on the corpus.** 212 references, 212 distinct keys,
  no two references collapsing onto one. §1 still says what happens when two do,
  because a key that is injective on today's corpus is not a key that cannot collide.
- **Every reference resolves to its guard.** Zero orphans, so the index degrades
  gracefully for a case that does not occur, exactly as `indexRoleReferences` already
  does.
- **Two different callees exist in one repository.** RuoYi-Vue carries
  `@ss.hasPermi` and `@ss.hasRole`, which is the pair §1's key is shaped to keep
  apart.

## Decision

### §1 The key is `(guard application key, Via, RawLiteral)` — verbatim, not normalized

```go
type permissionRefKey struct {
	guardApp guardAppKey
	via      string
	literal  string
}
```

`guardAppKey` is ADR 0007 §2's existing `(EndpointID, GuardName, AppliedAt)`,
unchanged and reused. A permission reference is meaningless without which guard
application required it, the same way a role reference is.

**`Via` is in the key, and that is the substantive part of this section.**
[ADR 0035](0035-permissions-in-the-model.md) §3 decided that Sphinxor records
*"the annotation names `'system:user:edit'` via `@ss.hasPermi`"* and explicitly does
**not** decide what `@ss.hasPermi` means. The pair `(Via, RawLiteral)` is therefore
the whole of the recorded fact; the literal alone is half of it. Keying on the
literal alone would make the diff assert that `@ss.hasPermi('admin')` and
`@ss.hasRole('admin')` are the same requirement — which is precisely the
interpretation ADR 0035 §3 refused to make, arrived at through the back door of a
comparison key instead of through a decision.

That is not a hypothetical pair. RuoYi-Vue is the corpus's one repository with two
callees on the same bean, and the odd one out —
`ruoyi-generator/.../GenController.java:128`'s `@ss.hasRole('admin')` — is the site
ADR 0035 §3 names as the case where `Via` is load-bearing. A refactor that turned
that line into `@ss.hasPermi('admin')` would change what the endpoint requires in
any reading of those two beans, and a key without `Via` would report no change at
all.

**The literal is keyed verbatim — `normalizeRawLiteral` is deliberately not
applied.** This is a departure from `roleRefKey` and it is the right one, because
ADR 0007 §2's reason for normalizing does not exist on this path and the cost does:

- *The reason does not exist.* Normalization was introduced for NestJS's
  `resolveRoleArg` fallback, which puts an argument's **raw source text** — internal
  whitespace, line breaks and all — into `RawLiteral`, so a Prettier reformat
  changes the key without changing the meaning. There is no such fallback here.
  ADR 0035 §2 admits a permission only when every argument is a clean single-quoted
  literal, and `parseQuotedArgList` returns the text strictly between the quotes;
  `Via` comes from `splitCall`, which hands it to `isBeanReference`, which accepts
  only a chain of `isIdentifier` parts and so can never contain whitespace. Neither
  field has a shape a reformat can perturb.
- *The cost does exist.* `parseQuotedArgList` preserves whatever is inside the
  quotes, spaces included. Collapsing runs of whitespace would key `'report:view
  all'` and `'report:view  all'` as one permission. Those are two different strings
  to whatever enforces them, and Sphinxor does not read the enforcer.

A key that cannot remove noise and can create a false equality is a worse key than
the verbatim one. Stated here rather than left as an omission, because the obvious
move when adding a second reference type is to copy the first one's key function.

**Two references with the same key collapse to one, and position is not part of the
key.** `@el.check('a','a')` produces two `PermissionReference`s with one key, and the
index keeps one — the behaviour `indexRoleReferences` already has, reached the same
way. Measured at **zero occurrences** across the corpus's 212 references, so nothing
in the corpus rides on it. Adding the argument's ordinal to the key would fix that
collapse and break something worse: `@el.check('a','b')` becoming
`@el.check('b','a')` would report two removals and two additions for a reorder that
changes nothing a reader cares about. A permission's position in an argument list is
not part of the requirement, and the key does not pretend otherwise.

### §2 Two lists, no "changed" list

`diff.Result` gains:

```go
AddedPermissionReferences   []model.PermissionReference
RemovedPermissionReferences []model.PermissionReference
```

and the Markdown report gains one section after *Role References*:

```
## Permission References

+ @ss.hasPermi('system:user:audit') (ruoyi-admin/.../SysUserController.java:149)
- @ss.hasPermi('system:user:edit') (ruoyi-admin/.../SysUserController.java:149)
```

**A changed permission is a removal plus an addition, and there is no third list.**
This is not a rendering shortcut; it is the same refusal ADR 0002 and ADR 0007 §2
already made twice and [ADR 0036](0036-became-public-gates-ci.md) §5 made a third
time. Reporting `system:user:edit → system:user:audit` as one *changed* entry means
pairing a removal with an addition, which is matching across an identity change —
and the moment two permissions change on one guard in the same commit, the pairing
is a guess. Two lists say exactly what the model knows.

**Each entry renders `Via('literal')` and its source location, and both halves are
deliberate.**

`Via('literal')` reproduces the source form, and it is produced by the *same
function* the matrix column uses (`report.renderPermissionReference`), not by a
second string-builder that happens to agree today — so a reader comparing the diff
against the matrix sees one spelling by construction. That shared spelling carries
§1's honesty into the report: a bare `admin` under a heading reading *Permission
References* is exactly the false claim ADR 0035 §3 designed `Via` to prevent, and
the diff must not be the one place that claim goes unqualified.

`file:line` locates the change, and it is what a `PermissionReference` actually
carries. Something must locate it — the corpus's 116 RuoYi-Vue references span 116
endpoints over ~72 distinct literals, so a bare list of strings would not tell a
reviewer what moved. The route would have been the natural choice, since the rest of
this report speaks in routes and `## Guard Applications` already renders
`on <endpointID>`. It is not used, because `PermissionReference` is attached to a
`GuardApplication` rather than to an `Endpoint` (ADR 0035 §1), so putting the route
in the report means either a new pair type on `Result` — breaking the property that
every field there is a plain model collection — or a model field duplicating a fact
`GuardApplicationID` already determines. `file:line` needs neither, and points at
the line the reviewer has to open anyway.

**The `Role References` section is left exactly as it is**, rendering a bare
`RawLiteral` with no endpoint. It has the same readability weakness and fixing it
would change output on every project that has roles, with no decision behind it.
Named here as a known asymmetry so it reads as noticed rather than overlooked.

JSON gets the two fields through the existing `json.Encoder` on `diff.Result`, with
no tags and no shape change, exactly as every other field on that struct.

### §3 Nothing new gates — and each case was run, not reasoned

No new regression case. `HasRegressions()`, `diffRegressions`, `Snapshot`,
`RegressionReason` and the `endpoint-became-public` synthesis are untouched.

**A permission changing to a different string is reported and not gated.** This
follows from [ADR 0036](0036-became-public-gates-ci.md) §2's *presence, never
content*, and the argument is strictly stronger for permissions than for the roles
that decision was written about. ADR 0036 §2 rejected gating on `ADMIN` → `USER`
because Sphinxor holds no ordering over roles — [ADR 0031](0031-role-hierarchy.md)
announces a `RoleHierarchy` precisely because it cannot read one. Over permissions it
holds strictly less than that: under ADR 0035 §3 it does not even know that
`@ss.hasPermi` denotes a permission check rather than a role check, so a gate would
have to rank two opaque strings produced by a call whose meaning is deliberately
unread. There is no honest direction to rank them in.

**An endpoint that loses all its permissions along with its annotation already
gates**, under ADR 0036 §1 case (c), and needs nothing from this ADR. Its §2 term 3
names `PermissionReference` for exactly this, and its own note that terms 3 and 4 are
"redundant today" is why: in the corpus every permission reference belongs to a
`@PreAuthorize` that is itself a `GuardApplication`, so removing the annotation
removes both. Verified as a constructed case below rather than assumed from the
table.

**Three transitions that look like losses and deliberately do not gate**, each
checked against the shipped binary:

- `@ss.hasPermi('x')` → `@ss.hasPermi()`. Readable becoming unreadable, which
  ADR 0036 §3 settles: `RolesUnresolved` is a fact about extraction, and a gate that
  fired on it would fail a PR that changed no protection and would fire *more* the
  less Sphinxor understands.
- One of two literals removed from a call that keeps the other. The endpoint is
  protected on both sides; which of two opaque strings is the weaker requirement is
  the same unanswerable question as above.
- A permission removed while a role remains on the same guard, or the reverse. Both
  sides are protected under ADR 0036 §2 term 1.

**One transition in this family does gate, through an existing rule, and it is worth
recording because nothing was designed to make it happen.**
`@PreAuthorize("@ss.hasPermi('x')")` becoming `@PreAuthorize("permitAll()")` is a
real widening to fully public that keeps the annotation in place — so ADR 0036's
protection test, which asks only whether evidence of authorization is present, sees
a `GuardApplication` on both sides and does not fire. It gates anyway: `permitAll()`
parses to `spelNoRole`, which yields `DeclaresRoles: true`, `RolesUnresolved: false`
and no references, and that is exactly `empty-role`'s trigger. A new High-confidence
finding appears, and ADR 0007 §3 case (a) fails the build. Confirmed by running it,
not by reading `guards.go`. It is included in §4's table so that a future change to
`empty-role` or to `spelNoRole` cannot remove the gate silently.

### §4 The constructed cases and the exit code each must produce

Built as minimal Spring projects under the pattern ADR 0036's bar already uses — a
base directory and a head directory differing in one edit — and run through the real
`sphinxor diff`.

| case | structural diff | exit |
|---|---|---:|
| literal changed, `@ss.hasPermi('user:edit')` → `@ss.hasPermi('user:delete')` | 1 removed, 1 added | **0** |
| callee changed, literal identical, `@ss.hasPermi('x')` → `@ss.hasRole('x')` | 1 removed, 1 added | **0** |
| permission added to an endpoint that already had one | 1 added | **0** |
| one of two literals removed, the other kept | 1 removed | **0** |
| arguments reordered, `@el.check('a','b')` → `@el.check('b','a')` | **no change** | **0** |
| whole `@PreAuthorize("@ss.hasPermi('x')")` removed | 1 removed, *Became Public* | **1** |
| the same removal, with a `sphinxor-allow` marker added | 1 removed, *Became Public* marked allowlisted | **0** |
| `@ss.hasPermi('x')` → `@ss.hasPermi()` (readable → unreadable) | 1 removed | **0** |
| `@ss.hasPermi('x')` → `permitAll()` (§3's widening) | 1 removed | **1** |
| permission gained on a previously unguarded endpoint | 1 added | **0** |

### §5 The regression bar

- **The corpus self-diff is a no-op.** Each of the 20 repositories diffed against
  itself must report zero regressions, exit 0, **and** produce empty
  `AddedPermissionReferences` and `RemovedPermissionReferences`. The second half is
  the one this ADR adds: a key that is not stable across two runs of the same source
  shows up here as phantom additions and removals, and on RuoYi-Vue and eladmin it
  would show up 212 times.
- **A real base/head pair, not only constructed ones.** RuoYi-Vue at `13db1fc`
  diffed against a copy of itself with one `@PreAuthorize` literal edited, to show
  the new section working on production source rather than on a fixture built to
  suit it.
- **`sphinxor lint` stays byte-identical on all 20 repositories and all four
  vendored fixtures.** This decision touches no rule, no confidence, no model field
  and no matrix column — only `internal/diff` and `internal/report/diff.go`.
- **Every existing test in `internal/diff` passes unchanged.** Two new lists on a
  struct and one new index should not perturb endpoint, role-declaration, guard or
  regression comparison, and if they do, that is the finding.

#### Measured after implementation, and every line above held

- **All ten constructed cases produce the predicted exit code**, and each was run
  against the shipped `98c638d` binary *first*: the exit codes are identical on both
  sides, which is what makes the "not an exit-code change" claim in Consequences a
  measurement rather than a reading of the code. The structural column matched too —
  two lines for each of the two *changed* cases, one for each single-sided one, and
  **zero for the reorder**.
- **The corpus self-diff is a no-op on all 20 repositories**: exit 0, zero
  regressions, zero added or removed permission references, and zero `+`/`-` lines in
  any section — RuoYi-Vue's 116 and eladmin's 96 included.
- **`sphinxor lint --format json` is byte-identical on all 20 repositories** against
  the `98c638d` binary, and on all nine vendored fixtures, which each also self-diff
  to exit 0 with no output lines.
- **The real base/head pair works**: RuoYi-Vue at `13db1fc` against a copy with
  `SysUserController.java:149` edited from `system:user:edit` to `system:user:audit`
  reports that one pair under *Permission References* and exits 0. Before this
  change, that same pair produced a report whose **every** section read *No change* —
  run, not assumed, and the reason the Context section states it as a fact.
- **The measurement's key was injective**: 212 references, 212 distinct keys, zero
  collisions, zero orphans.

### §6 What this does not do

- **The Cerbos export of permissions.** ADR 0035 §3 left it an obligation with a
  named open question — whether `@ss.hasRole('admin')`'s literal becomes a `roles:`
  entry or is omitted — and that question is untouched here.
- **NestJS permissions.** There are none in the model to diff; ADR 0035 deferred the
  extraction, and this ADR compares whatever the model carries, so it will cover
  NestJS on the day the extractor produces a `PermissionReference` and not before.
- **A `PermissionDeclaration` diff.** ADR 0035 §1 did not add the collection, for the
  measured reason that no corpus repository declares a permission in Java source.
  There is nothing to compare.
- **Forms B and C.** A bean call with no arguments or with `#param` arguments names
  no literal, produces no `PermissionReference`, and so is invisible to this diff for
  the same reason it is invisible to the matrix — all 140 of apollo's sites included.
  This ADR narrows nothing and widens nothing about what is read.
- **Any change to what gates.** §3 is a statement of what already follows from
  ADR 0036, plus one observation about `empty-role`. If a permission *widening*
  should gate, that is a decision about ordering opaque strings, and it needs a
  measurement and an ADR of its own.

## Alternatives considered

- **Key on the literal alone, leaving `Via` out**, by analogy with `roleRefKey`'s
  `(guardApp, literal)`. Rejected per §1: it silently decides that two different bean
  calls naming the same string are the same requirement, which is ADR 0035 §3's
  question answered by accident. The analogy also fails on its own terms — a role
  reference has no `Via`, because the guard name *is* how the role was named.
- **Reuse `normalizeRawLiteral` for symmetry.** Rejected per §1: its motivating case
  cannot occur on this path, and it introduces a false equality between two
  permission strings differing only in internal whitespace.
- **A `ChangedPermissionReferences` list pairing a removal with an addition on the
  same guard.** Rejected per §2, on the grounds ADR 0002, ADR 0007 §2 and ADR 0036 §5
  have already settled three times: pairing across an identity change is a guess, and
  it is ambiguous as soon as two literals change on one guard.
- **Gate on any permission change.** Rejected per §3. It would fail a build on every
  permission rename and every split of one permission into two, on a pair of strings
  Sphinxor cannot order — the gate-on-churn failure ADR 0036 §2 rejected for roles,
  with less information to go on.
- **Gate when an endpoint's permission count falls but does not reach zero.**
  Rejected: `@el.check('a','b')` → `@el.check('a')` is narrower or wider depending
  entirely on whether the bean's body ORs or ANDs its arguments, which is a method
  body ADR 0035 §3 does not read. eladmin's `check` ORs; nothing guarantees the next
  one does.
- **Add `PermissionReference` as a new `SubjectKind` so findings could key on it.**
  Rejected as premature: no rule produces a finding whose subject is a permission
  reference, so the `subjectKey` branch would be unreachable. ADR 0035 §1 already
  lists `diff/regressions.go`'s `subjectKey` among the consumers a later
  `PermissionDeclaration` would touch; that is the change that would make it mean
  something.

## Consequences

- `internal/diff/keys.go`: `permissionRefKey`, `keyOfPermissionReference`,
  `indexPermissionReferences`, `diffPermissionReferences`, `sortPermissionReferences`.
- `internal/diff/diff.go`: two fields on `Result`, two lines in `Compare`.
- `internal/report/diff.go`: one new section.
- `internal/model`, `internal/lint`, `internal/extract`, `internal/export/cerbos`,
  `internal/cli`, `internal/allowlist`: **no change.**
- `README.md`'s drift bullet names what `sphinxor diff` compares; permissions join
  that list in the same change, and the sentence about what it deliberately does not
  catch gains the permission case alongside the `ADMIN` → `USER` one it already
  names.
- `docs/limitations.md`'s *Permissions as metadata* entry lists the diff among what
  ADR 0035 does not cover. That line comes off the list in the same change, and
  nothing else in the entry moves — the open part of that gap is still the larger
  one.
- ADR 0035 §7's `internal/diff` row is superseded by this ADR, and its `report/diff.go`
  row with it. The other rows stand.
- **Not an exit-code change.** §4's table contains two `1`s and both are gates that
  already existed at `98c638d` — ADR 0036 case (c), and `empty-role` through
  ADR 0007 §3 case (a). A pipeline that was green stays green.
