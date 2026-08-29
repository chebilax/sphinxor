# 0013. AuthenticationRequirement gains an AppliedAt field (amends ADR 0012)

## Status

Accepted.

## Context

ADR 0012's Consequences section states plainly: "`internal/model`: one additive
`GuardScope` value, `ScopeRequestMatcher`. Nothing else changes shape." That
claim was wrong, discovered while implementing `internal/export/cerbos`'s
per-layer reduction, and the fix shipped in the same PR as the reduction logic
itself — initially as a field named `Scope`, renamed to `AppliedAt` during this
ADR's own review for consistency with `GuardApplication.AppliedAt` (same
concept, same name) — rather than being reported as its own finding first.
This ADR is that report, written after the fact: a process miss the
implementation should have avoided, not a decision this project is asking to
reopen after the fact.

ADR 0010 introduced `AuthenticationRequirement` on the assumption that an
endpoint has at most one: the one recognized authentication guard, wherever it
was found. `File`/`Line` were enough to say *where* the evidence lives, because
nothing downstream ever needed to distinguish one `AuthenticationRequirement`
from another on the same endpoint — there was only ever one.

ADR 0012 broke that assumption without updating the field it depends on: once
`SecurityFilterChain`'s `.anyRequest().authenticated()` can independently
produce an `AuthenticationRequirement` for the same endpoint that a method-level
`isAuthenticated()` SpEL check already did, a single endpoint can have one
`AuthenticationRequirement` per layer. `internal/export/cerbos`'s reduction
(§2) needs to group each `AuthenticationRequirement` into `layerMethod` or
`layerURL` before it can decide whether to intersect it against anything —
exactly the decision `GuardApplication.AppliedAt` already exists to answer for
guards. `AuthenticationRequirement` had no equivalent field.

`File`/`Line` cannot substitute for that decision. The only way to recover a
layer from a file path is to pattern-match the path itself (e.g. "a file named
`*SecurityConfig*` is the URL layer") — a framework-specific, unreliable
heuristic, and a direct regression of the precedent ADR 0011 already set with
`GuardApplication.DeclaresRoles`: replacing exactly this kind of string-based
inference with an explicit, extractor-set field, because a second framework's
naming conventions can't be relied on to encode a fact the model should state
directly.

## Decision

Add `AppliedAt GuardScope` to `AuthenticationRequirement` — same field name,
same `GuardScope` vocabulary `GuardApplication.AppliedAt` already uses
(`ScopeClass`, `ScopeMethod`, `ScopeRequestMatcher`), not a differently-named
field for the same concept. NestJS's extractor sets it from the recognized
guard's own `AppliedAt` (`internal/extract/nestjs/authentication.go`) — a
mechanical propagation, not a new judgment call; NestJS has only ever had one
layer, so this changes no NestJS behavior (`layerOf` maps every non-
`ScopeRequestMatcher` value to `layerMethod`).

## Alternatives considered

- **Infer layer from `File`/`Line` via a path heuristic** — rejected: relies on
  a framework's file-naming convention rather than a fact the extractor states
  directly, the same kind of fragility `DeclaresRoles` was added to remove.
- **Leave `AuthenticationRequirement` unchanged and have `internal/export/cerbos`
  track layer out-of-band** (e.g. a side map keyed by the requirement's ID,
  populated by the extractor separately) — rejected: pushes a fact about the
  requirement itself into a parallel structure the extractor has to keep in
  sync by convention, purely to avoid touching the model type; strictly worse
  than a field on the struct for no real benefit.

## Consequences

- `internal/model`: `AuthenticationRequirement` gains one field, `AppliedAt
  GuardScope`. This is the second model-shape change ADR 0012 did not
  anticipate (the first being the already-planned `ScopeRequestMatcher` value
  itself).
- `internal/extract/nestjs/authentication.go`: one line, propagating
  `recognized.AppliedAt` into the new field. No behavior change for existing
  NestJS output — confirmed by the full existing test suite passing unchanged.
- `internal/export/cerbos/translate.go`: `layerOf` now classifies
  `AuthenticationRequirement.AppliedAt` the same way it classifies
  `GuardApplication.AppliedAt`.
- No further model change is anticipated for the remaining Spring extractor
  work, but ADR 0012 said the same thing about this one — the honest status is
  "not anticipated," not "ruled out."
