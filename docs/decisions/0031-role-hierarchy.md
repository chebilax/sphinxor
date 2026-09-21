# 0031. A `RoleHierarchy` is announced as under-reporting, and its content is not resolved

## Status

Accepted.

## Context

[ADR 0029](0029-spring-security-scope.md) §2 lists `RoleHierarchy` as **silent**,
and §3 makes clearing that status part of the definition of done.

[ADR 0011](0011-spring-second-framework.md) §1 scoped it out:

> **`RoleHierarchy`** (e.g. `ROLE_ADMIN > permission:read`, letting one role imply
> another) is never resolved — every role name is treated as exactly what's
> written, consistent with never inferring a relationship the source doesn't state
> syntactically at the point being read.

That reasoning is sound and this ADR does not reopen it. What ADR 0029 identified
is narrower: not resolving the hierarchy is a defensible scope cut, and *saying
nothing about its existence* is not.

Split out of [ADR 0030](0030-post-authorize-and-method-security-filters.md), which
originally carried it. It shares that ADR's list but nothing else: it is a
project-wide configuration bean rather than a handler annotation, it affects the
roles column rather than a finding, and its error runs in the opposite direction.

### The measurement

**Zero occurrences across all 20 corpus repositories** — no `RoleHierarchy` bean,
no `RoleHierarchyImpl`, no `role-hierarchy` configuration key, in any file type.
Against a positive control of 897 `@PreAuthorize` in the same scan.

So this is a checklist item rather than a response to observed harm, with the
validation limit that implies (see Consequences).

## Decision

### §1 The hierarchy is detected and announced; its content is not parsed

A project declaring a `RoleHierarchy` is recorded in a project-level status
alongside `MethodSecurity`, and the run warns. The rules inside it —
`ROLE_ADMIN > ROLE_USER` and friends — are **not** read, and no grant is expanded.

### §2 The announcement states the direction of the error

The warning says that the roles shown are **narrower** than what the application
actually grants: where the matrix lists `ROLE_USER` for an endpoint, a
`ROLE_ADMIN` holder reaches it too, and the matrix does not show that.

Stating the direction is the substance of the announcement, not decoration. A bare
"this project has a role hierarchy" leaves the reader to work out which way the
numbers are wrong, and the two directions call for opposite responses.

This is why it is a **warning and not a finding**. The error under-reports access:
a reader acting on the matrix over-restricts rather than under-restricts, which is
the tolerable failure mode [ADR 0015](0015-inert-method-security-guard.md) already
identified. Nothing here asserts an endpoint is unprotected.

### §3 What detection looks for, and what it will miss

Three shapes, all Java, all project-wide:

- a `@Bean` method whose return type is `RoleHierarchy`;
- a `RoleHierarchyImpl.fromHierarchy(...)` or `.withDefaultRolePrefix()` call;
- a `setRoleHierarchy(...)` call on an expression handler.

It will miss a hierarchy configured in YAML or properties, or in Kotlin — both
already out of scope ([ADR 0011](0011-spring-second-framework.md) §1) and neither
newly hidden by this decision. Recorded so the announcement is not read as a
guarantee that a project without the warning has no hierarchy. This is the
`Found == false` boundary [ADR 0015](0015-inert-method-security-guard.md) draws,
applied unchanged: **not located** is never *confirmed absent*.

## Alternatives considered

- **Expand the hierarchy into the matrix**, so `ROLE_ADMIN > ROLE_USER` adds
  `ROLE_ADMIN` to every `ROLE_USER` grant. Rejected here as a different kind of
  decision: it changes reported grants rather than annotating them, it reopens ADR
  0011 §1's scope cut, and it needs an answer for transitive chains, cycles and the
  Cerbos export's role set. Doing it inside an "announce what is silent" ADR would
  smuggle a model change past the checklist. It is the obvious follow-up if a real
  project ever needs it — and with zero corpus occurrences, none does.
- **Treat a hierarchy as making the URL layer unknown**, as ADR 0020 §2 treats
  multiple filter chains. Rejected: that treatment omits endpoints from the export
  and marks roles unresolved, which is right when the tool cannot tell what the
  policy *is*. Here it can — the roles shown are real and correct, just not
  exhaustive. Downgrading them would discard accurate information.
- **Say nothing, per ADR 0011 §1.** Rejected per ADR 0029 §3: it is Spring
  Security's own mechanism, so not interpreting it is a gap, and not saying so is
  the defect this project has spent eight decisions removing.

## Consequences

- `internal/model`: one project-level status. No per-endpoint state, no change to
  `RoleReference`.
- `internal/extract/spring`: a project-wide bean scan, in the same pass as
  `scanMethodSecurityStatus`.
- `internal/cli/analyze.go`: one new project warning.
- `internal/lint`, `internal/export/cerbos`, `internal/diff`: **unchanged.** No
  finding fires, no grant changes, no endpoint is omitted.
- [ADR 0029](0029-spring-security-scope.md) §2 moves `RoleHierarchy` from
  **silent** to **detected and announced**.
- **Validation limit**: zero corpus occurrences, so the tests are constructed and
  the regression bar is a corpus no-op — `sphinxor lint` byte-identical across all
  20 repositories. Same position as
  [ADR 0030](0030-post-authorize-and-method-security-filters.md) and
  [ADR 0024](0024-controller-meta-annotations.md) §4, and honest about being weaker
  than a measured decision.
