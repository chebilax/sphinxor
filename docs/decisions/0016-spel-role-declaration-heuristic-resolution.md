# 0016. SpEL role references resolve against same-named Java enum constants, as a heuristic

## Status

Accepted.

## Context

`internal/extract/nestjs`'s `resolveRoleArg` only resolves a *qualified*
reference (`RoleEnum.admin`) to a `RoleDeclaration` — a bare string literal
is never resolved, even when it happens to equal a member's underlying
value, because in TypeScript an enum member's name and its backing value
are two different things (`enum RoleEnum { admin = "admin" }`): a string
that matches the value doesn't establish that the source actually meant to
reference that member.

Spring's method-security role checks are always bare SpEL string literals —
`hasRole('ADMIN')` has no syntax for a qualified enum reference at all
(§1 of ADR 0011). A plain Java `enum Role { ADMIN, PHARMACIST }` (Pharmacy's
real, vendored case) has no separate backing value the way the TypeScript
case does — `ADMIN` the string and `ADMIN` the constant's name are the same
text. That structural difference is real, but it does not make a name match
a *guarantee*: `hasRole('ADMIN')` compares a string against a
`GrantedAuthority` at runtime, resolved from wherever the application's
`UserDetailsService`/authorities come from — the `Role` enum is the
*usual* source of that string in a well-organized project, but Spring does
not require it, and nothing stops a project from having both a `Role` enum
and unrelated SpEL string literals that happen to share text with one of
its constants. Resolving on a name match is a strong, useful signal, not a
proof — the original framing of this decision ("safe because Java has no
backing value") overstated that, and is corrected here rather than left to
stand.

Whether that heuristic is *safe to ship* turns on what
`RoleReference.RoleDeclarationID` actually feeds, checked rather than
assumed:

```
$ grep -rn "RoleDeclarationID" internal/ --include="*.go" | grep -v _test.go
internal/lint/unreferenced_permission.go:30/31   — marks a declaration "referenced"
internal/export/cerbos/translate.go:199          — sets roleGrant.verified
```

Exactly two consumers, both non-gating:

- `unreferenced-permission-declared-but-unreferenced` is fixed
  `ConfidenceLow` (`internal/lint/unreferenced_permission.go`) — per ADR
  0004, Low confidence is a non-blocking warning, never a CI gate. A wrong
  heuristic match would at worst suppress one non-blocking warning, not
  produce or hide a CI-blocking finding.
- `translate.go`'s `verified` flag never withholds a grant — `UnverifiedRole`'s
  own doc comment (`internal/export/cerbos/translate.go`) states this
  explicitly: "this doesn't withhold the grant... it's flagged, not
  omitted." The exported Cerbos role is always the `RawLiteral` string,
  identical whether or not it resolved to a declaration. A wrong match
  changes nothing about what's exported — only whether an informational
  "unverified" note is attached alongside it.

No `ConfidenceHigh` finding, no export-withholding decision, and no CI gate
anywhere reads this field. The heuristic's failure mode is confined to
report readability.

## Decision

**A bare SpEL role-check string literal (from `hasRole`/`hasAnyRole`/
`hasAuthority`/`hasAnyAuthority`) resolves against a same-named Java `enum`
constant, when the project has exactly one enum constant with that exact
name.** Exposed and documented as a heuristic name-match, not a proven
reference — the doc comment on the resolving function states plainly that
Spring does not require the SpEL string to actually originate from the
matched enum, the same honesty this project applies to every other stated
limitation.

If more than one enum in the project declares a constant with that name
(ambiguous — which one did the string mean to reference, if either),
resolution is skipped and `RoleDeclarationID` stays `nil`, the same "don't
guess when a real answer isn't available" default used everywhere else in
this project.

## Alternatives considered

- **Never resolve bare SpEL literals, mirror NestJS's rule exactly** —
  rejected: NestJS's rule exists because a bare string there is genuinely
  ambiguous between "references this enum member" and "coincidentally
  equals its value." Java's plain-constant enums don't have that
  ambiguity in the same way, and given the confirmed non-gating consumers
  above, refusing a real, available, mostly-correct answer costs real
  report usefulness for a risk that's already contained to readability —
  the same shape of choice ADR 0012 already made in intersection's favor
  over the stricter subset check.
- **Resolve, and also let it feed a `ConfidenceHigh` decision or gate
  export** — not proposed: the consumer audit above shows nothing currently
  does or should; if a future consumer wants to treat this resolution as
  load-bearing rather than advisory, that's a new decision when that
  consumer is designed, not implied by this one.

## Consequences

- `internal/extract/spring`: SpEL-recognized role literals are resolved
  against `RoleDeclaration`s the same way NestJS resolves qualified
  references, via exact-name lookup against Java `enum` constants (and,
  per ADR 0011 §1, `public static final String` constants where the
  literal's value — not name — is what a plain string constant has to
  offer) collected in the project-wide role-declaration pass.
- `internal/lint/unreferenced_permission.go`, `internal/export/cerbos`: no
  code change — both already consume `RoleDeclarationID` exactly as
  designed; this ADR only changes how often it's non-nil for
  Spring-extracted references, not what either consumer does with it.
- The doc comment on the resolving function must state the heuristic's
  real limitation (a name match, not a proven reference) as plainly as
  this document does — an honest caveat travels with the code, not just
  the ADR.
