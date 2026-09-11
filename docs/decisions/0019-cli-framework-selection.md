# 0019. CLI framework selection, and "couldn't look" vs "looked and found nothing"

## Status

Accepted.

## Context

`internal/extract/spring` is complete, verified against two real vendored
repositories, and proven to feed every downstream consumer (`internal/lint`,
`internal/diff`, `internal/export/cerbos`) unchanged — ADR 0011's stated test of
the model's framework-independence, met. None of it is reachable from the command
line. `internal/cli/analyzeDirectory` hardcodes `nestjs.Extract`; nothing in
`internal/cli` references the Spring package at all.

The consequence is not "a feature is missing." It is a false negative in the
reassuring direction, confirmed by running it rather than reasoning about it:

```
$ sphinxor lint internal/extract/spring/testdata/Pharmacy
# RBAC Matrix

0 endpoint(s), 0 finding(s): 0 blocking, 0 warning, 0 allowlisted.
...
$ echo $?
0
```

That is a real Spring application with 21 `@PreAuthorize`-guarded endpoints
reporting a clean bill of health, with a zero exit code that would pass CI. A user
cannot tell this from "your authorization model is fine." It is the product-level
form of the exact inversion ADR 0018 corrected inside the Spring extractor, and
the one `docs/vision.md` builds the whole confidence-grading posture around:
**"don't know" must never render as "safe."**

Two decisions are entangled here and are settled together because they share one
crux. Choosing an extractor is how the tool decides *what* to look at; the
zero-result semantics are how it reports *whether it looked at all*. Splitting
them would mean deciding detection while deferring its most important failure
mode.

Three facts constrain the options, each checked against the repository rather than
assumed:

- **The vendored fixtures carry no build files.** `testdata/Pharmacy/` and
  `testdata/blog-api/` contain `NOTICE.md` plus source trees; the NestJS fixtures
  likewise. Per ADR 0005's curated-subset provenance discipline, they are source
  only — no `pom.xml`, no `build.gradle`, no `package.json`. Any detection scheme
  keyed on build files therefore **cannot be validated against the existing real
  corpus** without re-vendoring, while one keyed on source signatures can be
  validated today.
- **The extractors do not share a signature.** `nestjs.Extract` returns
  `(*model.Model, AllowlistOutcome, error)`; `spring.Extract` returns
  `(*model.Model, error)`.
- **The "did we look?" signal already exists and is discarded.** Both packages'
  `parseProject` walks the tree and returns the files it parsed; the count is
  simply dropped. Distinguishing "parsed zero source files" from "parsed many,
  recognized no endpoints" needs no new analysis — only that this number survive
  to the CLI.

## Decision

### 1. Auto-detect by source signature, with `--framework` as an explicit override — and **ambiguity or no match is a hard error, never a guess**

`sphinxor lint|diff|export` selects an extractor by inspecting the target
directory. A `--framework` flag (`nestjs`, `spring`) overrides detection entirely
and skips it.

The safety of this rests on one rule, which is the actual decision here:

- **Exactly one framework detected** → use it, reporting which one in the output
  header so the choice is never invisible.
- **No framework detected** → **hard error, non-zero exit**, naming the directory
  and telling the user to pass `--framework` if they believe it is wrong.
- **More than one detected** (a real monorepo with a Java backend and a Nest BFF
  is not exotic) → **hard error, non-zero exit**, listing what was found and
  requiring `--framework` to disambiguate.

Auto-detection is not dangerous in itself; auto-detection that *silently resolves
its own uncertainty* is. Requiring detection to produce one confident answer, and
refusing rather than picking in every other case, is the same rule the project
already applies to policy export ("omit and flag, never guess," ADR 0009 §3) and
to filter-chain evaluation ("unresolved, not public," ADR 0018), applied one level
up at the CLI.

Detection keys on **source signatures, not build files**: a file whose extension
the extractor accepts, carrying an import or annotation characteristic of the
framework (e.g. a `.java` file importing `org.springframework`; a `.ts` file
importing from `@nestjs/common`). This is chosen over build-file detection
specifically so it can be verified against the fixtures the project already has —
a detection rule validated only against hand-made directories would be exactly the
"synthetic fixtures encode the author's assumptions" failure `docs/testing.md`
rejects.

### 2. Zero extracted endpoints must distinguish "couldn't look" from "looked, nothing to flag"

Extraction reports how many source files it actually parsed. The CLI then treats
the two cases differently, because they mean opposite things:

- **Zero source files parsed** → the tool never looked. This is an **error, with a
  non-zero exit code** — never a rendered matrix, never a clean run. In practice
  this is a wrong path, a wrong framework, or an empty directory, and all three
  deserve to stop the user rather than reassure them.
- **Files parsed, zero endpoints recognized** → the tool looked and found no
  routes. This is **reported prominently as a warning but exits zero**: a library
  package or a monorepo subpath legitimately has no controllers, and failing those
  runs would make the tool noisy enough to be disabled — the adoption failure
  `vision.md` and ADR 0003 both keep in view. The warning states plainly that if
  the project does define routes, their shape was not recognized, and points at
  `docs/limitations.md`.

This adds a non-zero exit condition that is **not** a `Finding`, which
`docs/decisions/0004-confidence-level-granularity.md` did not anticipate — that
ADR's contract ("non-zero on high-confidence findings") is extended, not
contradicted: a run that could not look has no findings to grade, and reporting
success for it was never what that contract meant.

## Alternatives considered

- **Explicit `--framework` only, no detection** — rejected, though it is the
  safest option on its own terms: it cannot misdetect, because it never detects.
  Rejected because the hard-error rule in §1 already removes the misdetection
  danger (a wrong guess is impossible when every ambiguous or empty outcome
  refuses), leaving only the ergonomic cost — a required flag on every invocation,
  in every CI config — with no safety gain to pay for it. Kept as the override.
- **Auto-detect with a silent fallback** (pick the "most likely" framework, or
  default to NestJS when unsure) — rejected outright. This is the option that
  reintroduces the exact bug this ADR exists to fix: a Spring monorepo whose root
  contains a stray `.ts` file would be analyzed as NestJS and report a confident
  clean run. A default is a guess wearing a convention's clothing.
- **Run every extractor and merge the results** — rejected for now. Superficially
  attractive for monorepos, but it makes "which framework governs this endpoint"
  ambiguous in the model, and a single `*model.Model` mixing two frameworks'
  endpoints has no defined meaning for `diff` (ADR 0007's stable keys) or for the
  Cerbos exporter's resource mapping (ADR 0009 §2). A real monorepo story is worth
  having; it is a design problem of its own, not a free side effect of selection.
- **Detect by build file (`pom.xml`, `build.gradle`, `package.json`)** — rejected
  as the primary signal: it is the more conventional choice, but it cannot be
  validated against this project's existing real fixtures, which are source-only
  curated subsets (ADR 0005). Adopting a rule whose tests could only ever be
  synthetic contradicts `docs/testing.md` for precisely the component whose job is
  to decide whether the tool looks at anything at all. Build files may later be
  added as a *corroborating* signal if real repositories show source signatures
  are insufficient.

## Consequences

- `internal/cli` gains a framework-selection step ahead of `analyzeDirectory`, and
  `analyzeDirectory` stops hardcoding `nestjs.Extract`. The two extractors'
  differing signatures (`AllowlistOutcome` on one side only) are reconciled at
  that seam.
- **`sphinxor-allow` is NestJS-only, and wiring Spring into the CLI exposes that
  to users for the first time.** ADR 0003's mechanism is implemented in
  `internal/extract/nestjs`; `internal/extract/spring` has no allowlist support at
  all. A Spring user meeting a false positive would have no suppression mechanism
  — which is the precise scenario ADR 0003 was written to prevent ("the first
  false positive on a login endpoint gets the tool disabled in CI within a week").
  **That judgment has been made: it blocks.** Porting the allowlist to
  `internal/extract/spring` is a prerequisite of any Spring-carrying release, not
  a follow-up — on the same reasoning that disqualified releasing Spring without
  CLI wiring. The scenario is not hypothetical: `TestLint_Pharmacy` shows
  `mutating-endpoint-without-access-control` already firing on `POST /auth/login`,
  an endpoint Pharmacy makes public deliberately via `permitAll()`, and ADR 0011
  §2 / ADR 0012 §1 decided *on purpose* that a framework-level `permitAll()` is
  not treated as Sphinxor's own allowlist (a developer's `permitAll()` may itself
  be the mistake). So Spring's first real finding on a real app is on an
  intentionally-public endpoint whose only sanctioned suppression does not exist
  for Spring. This is a port of ADR 0003's settled design, not a new decision: the
  marker grammar (`internal/allowlist.ParseMarker`) is already
  framework-independent — `//` line comments are identical in Java and TypeScript
  — and `lint.Run` already takes framework-independent endpoint IDs; only the
  marker-to-endpoint anchor matching needs a Spring counterpart.
- The output header states which framework was selected and how (detected or
  `--framework`), so a silently wrong analysis becomes a visibly wrong one.
- `docs/limitations.md` gains the zero-endpoints warning's pointer target: what
  "parsed files but recognized no routes" can mean per framework.
- Validation, per `docs/testing.md`: detection is verified against all four real
  vendored fixtures (both NestJS, both Spring), and the end-to-end bar for this
  work is `sphinxor lint` on `testdata/Pharmacy` producing the real Spring
  findings — the same run that currently reports `0 endpoint(s)` and exits 0.
- `v0.6.0` is tagged only after that run is real. The Spring arc stays out of the
  CHANGELOG until then (`[Unreleased]` deliberately empty), because a release note
  saying "Spring support" is what would invite the false negative this ADR closes.
