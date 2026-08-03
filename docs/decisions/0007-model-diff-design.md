# 0007. Model diff design (v1)

## Status

Accepted.

## Context

`vision.md` names this v1's headline differentiator, not a line item: "diff between two versions of the model: new permissions, removed permissions, endpoints that became public, security regressions between two commits or two PRs," with CI/CD integration that "fail[s] a PR when a regression is detected in the model relative to the reference branch."

`docs/decisions/0002-intermediate-model-structure.md` chose normalized, ID-referenced collections specifically so this would be cheap when it arrived: "each collection diffs independently against its counterpart in another run by comparing stable keys." Two of those stable keys already exist and are load-bearing here — `Endpoint.ID` is deterministically derived from `HTTPMethod + Path` (`model.NewEndpointID`), and `RoleDeclaration.Name` is the qualified `EnumName.MemberName` string — both stable across two separate extraction runs of the same route or role, by construction, not by coincidence.

Three things this ADR has to settle, none of them "cheap to change later" once a CI workflow depends on them:

1. **How Sphinxor gets two models to compare.** `vision.md` says "between two commits or two PRs" — that implies comparing states that exist at different points in git history, but doesn't dictate how Sphinxor obtains them.
2. **How entities without an inherent cross-run identity are matched.** `GuardApplication` and `RoleReference` IDs are sequential, assigned fresh every run — comparing `guardapp-3` from one run against `guardapp-3` from another is meaningless. They need a derived stable key, the same way `Endpoint` and `RoleDeclaration` already have one.
3. **What "a regression" means precisely**, since that's the exact condition the CI-gating exit code keys on — get this wrong and the feature either misses real regressions or fails PRs on noise.

## Decision

Three sub-decisions, all settled below.

### 1. Two pre-extracted directories, not git-native checkout

`sphinxor diff <old-dir> <new-dir>` — the caller (typically a CI script) is responsible for producing two directories to compare (e.g. `git worktree add` for the base ref, the current checkout for the head ref), and Sphinxor extracts and compares them. Sphinxor does not shell out to `git` itself.

- **Why**: keeps Sphinxor a static analyzer over a filesystem, exactly like `sphinxor lint .` already is — no new dependency on a git binary being present/compatible, no ambiguity about which git ref syntax is supported, no credential/auth surface for fetching refs Sphinxor doesn't already have checked out. A CI script producing two directories from two refs is a two-line, well-understood pattern (`git worktree add ../base <base-ref>`); Sphinxor reimplementing that adds real surface area for a convenience that's already trivial to script around it.
- **Alternative rejected**: `sphinxor diff --base <git-ref>`, with Sphinxor shelling out to `git worktree`/`git show` internally. More convenient at the call site, but it means Sphinxor now has an opinion about git ref resolution, detached-HEAD edge cases, and shallow-clone interactions in CI (a common real failure mode for tools that assume full history) — none of which are its job to get right. Rejected for v1; revisit only if the two-directory interface proves to be real friction in practice, not preemptively.

### 2. Derived stable keys for `GuardApplication` and `RoleReference`

Neither carries a natural stable key the way `Endpoint`/`RoleDeclaration` do, so one is derived at diff time (not stored in the model — this is comparison-time logic, not a new model field):

- `GuardApplication` key: `(EndpointID, GuardName, AppliedAt)`. Two guard applications on the same endpoint, same guard name, same scope (class/method) are "the same" guard across runs — this is exactly the granularity `mutating-endpoint-without-access-control` already treats as meaningful.
- `RoleReference` key: `(the owning GuardApplication's derived key, normalize(RawLiteral))`. A role reference is meaningless without which guard application it belongs to; two references naming the same role literal under "the same" guard application (by the key above) are "the same" reference across runs.

`FromComposite` is deliberately excluded from both keys — whether a guard was resolved via composite expansion or found literally is an extraction-mechanism detail, not a fact about the endpoint's authorization surface; a guard that flips from literal to composite-resolved between two runs (e.g. a refactor that introduces a shared `@Auth()` decorator) should diff as unchanged, not as removed-then-added.

**`RawLiteral` is normalized for keying, decided explicitly rather than left silent.** For the common cases — a qualified member-expression (`RoleEnum.admin`) or an extracted string-literal fragment — `RawLiteral` is already whitespace-free by construction (`resolveRoleArg` builds it from `object.property` text or a quote-stripped fragment). But `resolveRoleArg`'s fallback case (an argument that's neither a member expression nor a string literal — a spread, a computed expression, a call) uses the argument's raw source text verbatim, including whatever internal whitespace and line breaks the author happened to write. A pure reformat (Prettier collapsing a multi-line expression onto one line, or vice versa) would change that text without changing its meaning, and — unfixed — would diff an unchanged role reference as removed-then-added in the informational structural diff. Not a CI-gating false failure (`RoleReference` changes don't gate on their own, only `Finding` changes do, per §3), but exactly the kind of noise that erodes trust in a diff output over time, since it appears on every reformat, not rarely. `normalize()` here means: collapse runs of whitespace (including newlines) to a single space, then trim. It applies only to the *derived diff key* — the model's own `RawLiteral` field stays verbatim, unmodified, for display.

### 3. A regression is a new High-confidence finding, or a High-confidence finding that stops being allowlisted

Diffing produces two kinds of output, and only one of them gates CI:

- **Structural diff** (informational, always shown): added/removed endpoints, added/removed role declarations, added/removed/changed guard applications and role references — per `vision.md`'s "new permissions, removed permissions, endpoints that became public."
- **Regressions** (CI-gating): a `Finding` present in the new model's `High`-confidence, non-allowlisted set that either (a) has no matching finding in the **old model's `High`-confidence set** at the same `(RuleID, SubjectKey)`, or (b) matches an old finding that *was* allowlisted and no longer is. Case (b) matters specifically because it's the "someone removed a `sphinxor-allow` marker" scenario — structurally identical code, but the human acknowledgment protecting it is gone, which is exactly the kind of regression a point-in-time `sphinxor lint` run can't see and a diff is uniquely positioned to catch.

**Case (a)'s match set is explicitly the old model's High-confidence findings only — not "any finding regardless of confidence" — stated here precisely because getting this wrong fails silently, in the dangerous direction.** No rule in the current v0.1 set varies its own confidence per instance (`mutating-endpoint-without-access-control` and `permission-declared-but-unreferenced` are always Low, `empty-role` is always High), so this exact scenario isn't reachable through today's three rules — but the matching logic has to be correct on its own terms, not correct-by-accident because nothing currently exercises the failure mode. If a future rule (or a refined confidence model, per ADR 0004's own note that a middle tier may be revisited in v1) ever produces a Low finding for a subject that later becomes High for the same subject and rule, an implementation that matched "does this subject key have any old finding at all" — ignoring confidence — would find the old Low finding, conclude "not new," and silently fail to gate a real loss of protection. The comparison must filter the old side to `High` before matching, not just the new side. Covered by a dedicated test at implementation time (constructed directly via `model.Finding` literals, the same way existing rule tests are — not something that needs to be reachable through today's extractor to be worth testing).

A `Low`-confidence finding appearing or disappearing is never a regression by itself — that's unchanged from `lint`'s existing exit-code boundary (ADR 0004): Low findings are warnings, not gates, whether compared across two runs or seen in one. A pre-existing High-confidence finding that persists unchanged between old and new is *not* a new regression (it already gated the PR that introduced it, if `lint` was run then) — surfacing it again on every subsequent PR would be exactly the noise-fatigue failure mode `vision.md` is built to avoid.

**No serialization boundary for allowlist status to fall through, by construction.** `sphinxor diff <old-dir> <new-dir>` re-extracts and re-runs `lint.Run` independently on each directory in-process (§1) — it never reads a previously-generated report file from either side. Confirmed directly against the current implementation, not assumed: `lint.Run` (`internal/lint/rules.go`) never filters findings by `Allowlisted` — it tags the flag on matching findings but always includes them in its returned slice — and nothing in `internal/report` drops an allowlisted finding either; `markdown.go` and the JSON path only ever *read* `.Allowlisted` for display and counting. Case (b) above depends on the old side's allowlisted findings being present to match against, and they are, end to end.

## Alternatives considered

- **Git-native checkout** (see above) — rejected for v1, revisit only on demonstrated friction.
- **Sequential/positional matching** (diff entities by their position in each collection rather than a derived key) — rejected outright: this is precisely what ADR 0002 chose normalized collections to avoid, and would make the diff wrong the moment an endpoint is added or removed anywhere before another endpoint in file order.
- **Treat every finding-count change as a regression** (any new finding, any confidence) — rejected: would gate CI on Low-confidence noise, reintroducing the false-positive-fatigue trap `vision.md` explicitly built the allowlist and confidence grading to avoid. Confirmed and stated above.

## Consequences

- A new `internal/diff` package (or similar) implementing per-collection comparison by the keys above, consumed by a new `sphinxor diff <old-dir> <new-dir>` CLI command.
- `internal/report` gains a diff-shaped output (structural changes + regressions), likely Markdown + JSON per the same v0.1 output convention, though the exact rendering is implementation, not a further design decision this ADR needs to settle.
- The regression definition in §3 is the CI-gating contract from here on — changing it later (e.g. adding a middle confidence tier, per ADR 0004's deferred v1 revisit) means revisiting this ADR too, not just the confidence one.
