# 0009. First authorization-engine exporter: Cerbos

## Status

Accepted.

## Context

`vision.md` names exporting to a real authorization engine as part of v2 ("Export
to a first target engine (Cerbos or OPA, whichever model is closest to yours)"),
and is explicit that this isn't a generic mapping problem: "RBAC, ABAC, ReBAC, and
policy-as-code (Rego/Cedar) don't translate cleanly into one another without
semantic loss." This ADR is where that gets made concrete for the first exporter,
rather than discovered mid-implementation.

**This is a downstream feature, by construction.** It reads the normalized model
(`internal/model`, ADR 0002) and `Finding`s already produced by extraction and
linting, and produces a new artifact from them. It must not reach back into
`internal/extract/nestjs` or add framework-specific logic anywhere in the export
path — the entire point of ADR 0002's framework-independent model is that anything
built downstream of it works for every future framework's extractor automatically.
If a translation decision here ends up needing NestJS-specific knowledge, that's a
sign the model itself is missing something, not a reason to reach past it.

**This is also the first directional feature, and that matters more than the
engine choice.** Every Sphinxor feature so far — extraction, the three lint rules,
the RBAC matrix, drift detection — only observes: worst case, a finding is wrong
and someone ignores it. An exporter is different in kind: its output is meant to be
fed into a real authorization engine that makes real access decisions. A generated
policy that is wrong in the *permissive* direction doesn't just mislabel something
that was already safe — it creates a new hole where the analysis, not the original
code, is the thing that got it wrong. The safety posture below is the part of this
ADR that matters most; the engine choice matters less than getting that posture
right before any code exists.

The current model has no notion of "resource" or "action" the way a policy engine
needs them — confirmed by reading `internal/model/model.go`, not assumed. It has
`Controller`, `Endpoint` (HTTP method + path), `GuardApplication`, `RoleDeclaration`,
`RoleReference`. Any exporter has to decide what a "resource" and an "action" are
before it can decide anything else, and that decision has nowhere to hide once a
real target schema is involved.

## Decision

### 1. Cerbos first, not OPA/Rego — and why this order, specifically

**Cerbos.** Cerbos's policy model is resource-oriented: a resource policy names one
resource kind, and its rules grant named roles specific actions on it — structurally
close to Sphinxor's own resources/actions/roles/permissions framing (`vision.md`).
That closeness is the reason to go first, not just a convenience:

- **The translation is direct**, because the shapes already line up — low
  incidental complexity to build through before the real question (does the
  *content* survive translation?) can even be asked.
- **It's a sharper test of the model's completeness than Rego would be.** Cerbos's
  schema is fixed and specific (`resource`, `actions`, `roles`, `derivedRoles`,
  `condition`, ...): if the normalized model is missing a concept the schema
  expects, or holds a concept the schema has nowhere to put, that mismatch is
  immediately visible — there's no field to awkwardly stuff it into. Rego is
  general-purpose: any shape of Go value can be marshaled into a policy that merely
  *looks* plausible, which would let a real gap in the model go unnoticed simply
  because Rego's flexibility absorbed it instead of exposing it. Confirmed against
  real Cerbos policy examples (`resource_policy_01.yaml`,
  `derived_roles_01.yaml` in `cerbos/cerbos`), not assumed from documentation
  summaries alone — see §4 below for what that check already found.

OPA/Rego becomes the natural v0.5.0 target once this ships: the pipeline this ADR
sets up (model → intermediate rule set → omission/report generation → engine-specific
serialization) is meant to be reusable, with only the last step swapped for a Rego
serializer. This ADR does not design that reuse boundary in detail — that's an
implementation decision for whichever code review actually builds it — but it's the
expectation this ordering is chosen against.

### 2. Resource/action mapping (what "translation" concretely means here)

Since the model has no `Resource`/`Action` concept, this exporter derives them from
what does exist, using the most direct reading available rather than inventing a new
grouping:

- **Resource kind** = the owning `Controller.Name` (e.g. `UsersController` →
  `users`, normalized). One Cerbos resource policy file per controller.
- **Action** = the `Endpoint.HTTPMethod`, lowercased (`get`, `post`, `patch`, ...).

This is a coarser action vocabulary than Cerbos idiomatically uses (real-world
Cerbos policies often name actions semantically — `approve`, `view:public` — rather
than by HTTP verb), and that's stated here as a known, accepted limitation, not
silently smoothed over: Sphinxor's model doesn't know an endpoint's business
meaning, only its method and path. A future model enhancement to capture a
richer action vocabulary is out of scope for this ADR.

### 3. Safety posture: omit and flag, never guess

The exporter's default output must be **safe to be wrong about, by construction**,
because "wrong" here can mean "too permissive," and that specific failure mode is
unacceptable regardless of how rare it would be:

- **A generated resource policy grants a role access to an action if and only if
  extraction found a real `GuardApplication`/`RoleReference` establishing it** —
  composite-resolved guards (ADR 0006) count as real for this purpose, the same way
  ADR 0007's diff treats them as real; a bare Low-confidence
  `mutating-endpoint-without-access-control` finding does not, since it documents
  *absence* of evidence, not presence of a role.
- **An endpoint with no confirmed guard produces no rule for that resource/action —
  it is omitted, not granted to a wildcard role and not granted to nobody via an
  explicit deny rule either.** This is deliberately not just "the cautious choice":
  Cerbos denies by default for any action with no matching rule, so omission is the
  structurally correct safe state, not a workaround. The alternative — emitting an
  explicit rule for the uncertain case — is rejected outright (§ Alternatives).
- **An `sphinxor-allow` marker (ADR 0003) is a statement that Sphinxor's own
  analysis doesn't apply here — it has no bearing on what the exporter emits.** An
  allowlisted endpoint with no real guard is exported exactly like any other
  unguarded endpoint: omitted, flagged. Allowlisting a finding must never be
  readable, downstream, as "safe to expose."
- **A `RoleReference` that didn't resolve to a `RoleDeclaration`** (bare string
  literal, no enum/const backing it) is still exported using its raw literal — Cerbos
  only needs a role name string, not a canonical declaration — but is flagged in the
  companion report (§4) as unverified, since Sphinxor itself couldn't confirm it's a
  real, canonical role name.
- **No `derivedRoles` or `condition` (Cerbos's ABAC-shaped, ReBAC-adjacent features)
  are ever generated in this version.** The model has no attribute-condition data to
  translate honestly — synthesizing one to approximate what a decorator "probably"
  meant is exactly the guessing this posture rules out.
- **Every generated policy file carries a mandatory header comment**
  (`# Generated by sphinxor — review before deploying. Not deploy-ready.` plus a
  pointer to the companion report) — in the artifact itself, not only in
  surrounding docs or a README that might not travel with the file when it's copied
  into a deploy pipeline.

### 4. Semantic-loss handling: both a companion report and inline comments

Every omission is surfaced in two places, not one, because they serve different
readers:

- **Inline YAML comments**, at the exact point of omission, for someone reading the
  generated policy file directly — immediate context with no need to cross-reference
  a separate document.
- **A companion report** (`export-report.md`/`.json`, same Markdown+JSON convention
  as the existing RBAC matrix and diff reports), listing every omitted
  endpoint/unresolved role with its reason, for someone auditing "what does this
  export not cover" at a glance, without reading every generated policy file. This
  mirrors `docs/limitations.md`'s existing role: a gap that isn't written down
  doesn't stay honestly represented for long.

### 5. Real-engine validation is part of "done," not a follow-up

`cerbos compile <policy-dir>` (confirmed to exist in `cerbos/cerbos`'s `cmd/cerbos`)
compiles and validates a policy set without a running server. Every exporter test
against the two real vendored NestJS repos must end with the generated output
actually passing `cerbos compile` — a policy that doesn't compile in the real engine
is this feature's equivalent of a diff regression that silently doesn't fire: the
kind of bug that "it looks right" doesn't catch, per `docs/testing.md`.

## Alternatives considered

- **OPA/Rego first** — rejected for v0.4.0 for the completeness-test reason in §1;
  planned as v0.5.0, reusing this ADR's pipeline and safety posture.
- **An engine-neutral IR-only output** (e.g. plain JSON of "role → allowed
  actions," no real target schema) — rejected: doesn't exercise the model against a
  real engine's actual constraints, and doesn't produce the "distributed,
  review-before-deploying artifact" this feature and `vision.md`'s exporter
  positioning are both about.
- **Emitting a permissive placeholder for uncertain cases** (e.g. an explicit
  allow-all rule marked `TODO`, on the theory that an explicit, visible TODO is
  "found" faster than a silent omission) — rejected outright: a rule that grants
  access is live the moment the policy is deployed, TODO comment or not. Omission
  (§3) achieves the same visibility (via the companion report) without ever being
  live-and-wrong in the meantime.
- **Presenting exported output as deploy-ready** — rejected outright, not just
  discouraged: `vision.md`'s own positioning is "analyze, detect, explain — never
  decide on the developer's behalf," and a policy a user deploys unreviewed inverts
  that into Sphinxor making the decision by omission.
- **Silent skip of unmappable constructs, no report** — rejected: contradicts both
  `vision.md`'s "explains, with an honestly stated confidence level" and the
  allowlist mechanism's own precedent (ADR 0003) of never letting a gap in coverage
  go undocumented.

## Consequences

- A new `internal/export/cerbos` package (or similar), consuming `*model.Model` and
  `[]model.Finding` only — no dependency on `internal/extract/nestjs`, enforceable
  by import graph alone.
- A new CLI surface, likely `sphinxor export cerbos <dir> --out <policy-dir>`,
  matching the existing `lint`/`diff` command conventions.
- Test infrastructure gains a dependency on the real `cerbos` CLI (or its Docker
  image) for the compile-validation step in §5 — a test-time dependency, not a
  runtime one; Sphinxor itself never depends on Cerbos to run.
- The resource/action mapping in §2 is a real, stated limitation (HTTP-verb-grained
  actions, controller-grained resources) that a future model enhancement could
  improve — not committed to being permanent, but not solved by this ADR either.
- v0.5.0's Rego exporter is expected to reuse this ADR's model-to-intermediate-rules
  logic and its omission/report mechanism, changing only the serialization target —
  the actual reuse boundary is an implementation decision for that work, not
  designed here.
