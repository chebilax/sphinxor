# 0003. Allowlist format

## Status

Accepted.

## Context

`vision.md` requires this mechanism to exist from the first functional version, not be added later: a developer must be able to mark a route as "public, on purpose" — a login endpoint, a healthcheck — without Sphinxor flagging it every run. `vision.md` is explicit about why this can't slip to a later version: an unsuppressible false positive on a login endpoint or healthcheck is exactly what gets a CI-gating tool disabled within a week, "the same trap that limited adoption of general-purpose SAST in this space." It's also explicit that whatever mechanism is chosen must be **versioned with the code, not an external database** — so a hosted dashboard or database-backed suppression list is out of scope regardless of which option below is picked.

`vision.md` names three candidate mechanisms without picking one: an annotation, a comment, or a `.sphinxor-allow` config file. These aren't fully interchangeable — they trade off differently on two axes that matter here:

- **Colocation**: is the exemption visible right next to the code it exempts, or centralized in one place?
- **Auditability**: can a reviewer see *all* exemptions in a project at once, to judge whether the mechanism is being overused?

There's also a naming collision to be careful about: NestJS apps commonly already have their *own* `@Public()`-style decorator, used at runtime to make Nest's own auth guard skip a route. That is an application-level authorization decision. Sphinxor's allowlist is a different thing — a statement that "Sphinxor's analysis is wrong or not applicable here," which may or may not coincide with the app being genuinely public. Conflating the two (e.g. Sphinxor silently treating any `@Public()`-shaped decorator as its own allowlist) would mean Sphinxor inherits false negatives from whatever the app's own convention happens to be, instead of requiring an explicit, Sphinxor-specific opt-out — which undermines the entire "explicit allowlist so false positives don't get the tool disabled" rationale by making it too easy to accidentally satisfy.

## Decision

**Hybrid: in-code comment marker as the required mechanism, plus a generated report — not a hand-maintained central file.**

The in-code marker must be a **comment**, not a decorator: a decorator (Option A) requires every scanned project to take on a compile-time dependency on a Sphinxor package or stub, which contradicts Sphinxor's positioning in `vision.md` as an external, non-invasive analyzer — the same principle that keeps Lynxor from ever requiring changes to the repository it scans. A comment marker (Option B) gets the same colocation and drift-resistance benefits without that coupling.

The central-file auditability benefit (Option C) is kept, but without Option C's drift risk: rather than a hand-maintained `.sphinxor-allow` file that can silently go stale or re-match a renamed route, Sphinxor **generates** the equivalent report from the comment markers it finds during each scan. There is exactly one source of truth — the comment in the code — so the report can't drift from it by construction.

**Addition to close the comment's structural weakness** (no compiler/parser tie to the declaration below it, unlike a decorator): if a marker is found during a scan but isn't immediately above a recognized endpoint, Sphinxor emits it as a **"stale allow marker" finding**. This reuses whatever matching/identity logic the model ([`0002-intermediate-model-structure.md`](0002-intermediate-model-structure.md)) already has for detecting renamed or moved routes, rather than introducing separate logic for it.

### Option B — In-code comment marker (chosen, as the required in-code mechanism)

A fixed-grammar comment directly above the route handler, e.g. `// sphinxor-allow: public — health check endpoint, no auth by design`.

- **Colocation**: highest — same position as Option A, no import required.
- **Auditability**: needs a generated report to see all exemptions at once — solved by generating that report from the markers on every scan, rather than requiring a hand-maintained file (see Decision above).
- **Drift-resistance**: strong — but slightly weaker in one respect than a decorator: a comment has no compiler/parser tie to the specific declaration below it, so a bad refactor could leave the comment orphaned above the wrong line without erroring the way a misplaced decorator more often would. Mitigated by the "stale allow marker" finding described in the Decision above.
- **Cost**: none beyond parsing a comment grammar. No dependency on Sphinxor at runtime or at compile time — the deciding factor over Option A.

### Option A — In-code decorator (rejected)

A dedicated decorator, e.g. `@SphinxorAllow('public-healthcheck')`, distinct from any app-level `@Public()`-style decorator, applied directly to the controller or handler.

- **Colocation**: highest — sits on the exact line it exempts.
- **Auditability**: requires a codebase-wide search/report to see all exemptions at once; not free, but mechanical (Sphinxor can generate this list itself as part of its output).
- **Drift-resistance**: strong — moves and renames with the code by construction, can't silently stop applying without the route itself changing.
- **Cost**: requires the target project to import a Sphinxor package (or declare an ambient/stub decorator) just to use the analysis tool. Rejected for this reason: it makes Sphinxor invasive, contradicting its positioning as an external, non-invasive analyzer.

### Option C — Central `.sphinxor-allow` config file (rejected as hand-maintained; kept as a generated report)

A single file (YAML/JSON) listing exemptions by a stable endpoint identity (per [`0002-intermediate-model-structure.md`](0002-intermediate-model-structure.md), `httpMethod + path`), each with a required reason field.

- **Colocation**: none — physically separate from the code it exempts.
- **Auditability**: highest — every exemption in the project is visible in one file, in one PR diff, without hunting through the codebase. This is the strongest argument for it, and it's the part the Decision above keeps.
- **Drift-resistance**: weakest, and ironically so for a tool whose headline feature is drift detection. If a route is renamed or removed, a hand-maintained entry doesn't move with it — it either goes silently stale, or, worse, could start matching a *different*, unrelated route that happens to reuse the same path/method later. Rejected as a hand-edited, primary mechanism for this reason; kept only as a generated artifact, which sidesteps the problem since it's regenerated from the comment markers on every scan rather than edited independently of them.

## Consequences

The comment-marker grammar defined here becomes the parser surface for the allowlist matcher, and — per [`0002-intermediate-model-structure.md`](0002-intermediate-model-structure.md) — how allowlist entries bind to an endpoint's stable identity. The "stale allow marker" finding needs to be scoped as part of the initial implementation, not deferred, since without it a moved or renamed route silently loses its exemption's error-signaling — the one drift-resistance gap comments have relative to a decorator.
