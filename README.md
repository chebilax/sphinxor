# sphinxor

Static analysis for your authorization model (RBAC/ABAC/IAM).

Sphinxor reconstructs the authorization model that actually exists in your code —
endpoints, guards, roles, permissions — instead of the one declared somewhere else
that may have drifted. It analyzes, detects, and explains, at an honestly stated
confidence level; it never makes an authorization decision itself. See
[`docs/vision.md`](docs/vision.md) for the full positioning.

Currently targets **NestJS** ([`docs/decisions/0001-target-framework-choice.md`](docs/decisions/0001-target-framework-choice.md)).

## Install

```sh
go install github.com/chebilax/sphinxor/cmd/sphinxor@latest
```

`@latest` tracks the newest tagged release once one exists; pin to a specific
version instead (e.g. `@v0.3.0`) for reproducible builds. `sphinxor version`
reports what you have installed.

## Usage

### `sphinxor lint` — audit one checkout

```sh
sphinxor lint .
```

Extracts the authorization model at the given path (defaults to `.`) and reports
findings from the v0.1 rule set:

- a mutating endpoint (`POST`/`PUT`/`PATCH`/`DELETE`) with no detected access control;
- a role/permission declared but never referenced by any guard;
- an empty role.

Every finding carries a confidence grade — `High` or `Low`
([`docs/decisions/0004-confidence-level-granularity.md`](docs/decisions/0004-confidence-level-granularity.md)) —
never a bare "vulnerable/not vulnerable" verdict. `High`-confidence, non-allowlisted
findings exit non-zero, so `lint` is usable as a CI gate as-is. `Low` findings are
reported but never fail the build — they mark cases the static analysis can't
resolve with certainty (see **Known blind spots** below), not cases you should ignore.

If a route is intentionally unprotected (a health check, a login endpoint), mark it
in place rather than accepting a false positive on every run:

```ts
// sphinxor-allow: public health check, no auth by design
@Get('health')
check() { ... }
```

See [`docs/decisions/0003-allowlist-format.md`](docs/decisions/0003-allowlist-format.md)
for the marker grammar and why it's a comment rather than a decorator or a
hand-maintained config file.

### `sphinxor diff` — catch regressions between two versions

```sh
sphinxor diff <base-dir> <head-dir>
```

This is v1's headline feature: not just a point-in-time audit, but drift detection
between two versions of the same project's authorization model — a PR against its
target branch, or any two commits.

**The two-directory contract.** `diff` takes two paths on disk and never shells out
to `git` itself — no assumption about which `git` version or ref syntax is
available, no credential surface for fetching refs, no shallow-clone surprises in
CI. Producing the two directories is the caller's job, and it's a two-line pattern
in any CI script:

```sh
git worktree add ../base <base-ref>
sphinxor diff ../base .
```

`<base-ref>` is typically the PR's target branch (e.g. `origin/main`). Both sides
are extracted and linted independently, in-process — see
[`docs/decisions/0007-model-diff-design.md`](docs/decisions/0007-model-diff-design.md)
§1 for why this beats Sphinxor having its own opinion about git.

**What gates CI, and what doesn't.** `diff` exits non-zero only on a *regression*:

- a `High`-confidence finding in `head` with no matching finding in `base`'s
  `High`-confidence set (something newly wrong), **or**
- a `High`-confidence finding in `head` that matched a `base` finding which *was*
  allowlisted and no longer is (someone removed a `sphinxor-allow` marker —
  structurally the same code, but the human acknowledgment protecting it is gone).

It does **not** gate on:

- any `Low`-confidence finding, new or otherwise;
- a `High`-confidence finding that already existed, unchanged, in `base` (it already
  gated whichever PR introduced it — re-failing every subsequent PR on the same
  pre-existing finding would be pure noise).

Everything else `diff` reports — added/removed endpoints, added/removed role
declarations, added/removed/changed guard applications and role references,
endpoints that became public — is structural, informational output: always shown,
never itself a reason the run fails. Full rule in
[`docs/decisions/0007-model-diff-design.md`](docs/decisions/0007-model-diff-design.md) §3.

**Known blind spots carry over.** `diff` compares whatever the extractor could see
in each snapshot, so anything the extractor can't see, it can't diff either — a
regression hidden behind a global guard (`APP_GUARD`, `app.useGlobalGuards()`) or an
unresolved composite decorator (see
[`docs/limitations.md`](docs/limitations.md)) is invisible to `diff` the same way it
would be to a single `lint` run on either side, not something `diff` closes by
comparing two points in time. If you're gating CI on `diff`, read
[`docs/limitations.md`](docs/limitations.md) first.

Both commands accept `--format markdown` (default) or `--format json`.

## Documentation

Start at [`docs/README.md`](docs/README.md). In particular:
[`docs/vision.md`](docs/vision.md) for scope and philosophy,
[`docs/decisions/`](docs/decisions/) for why each design choice was made,
[`docs/limitations.md`](docs/limitations.md) for what the current analysis honestly
cannot see, [`docs/testing.md`](docs/testing.md) for how correctness is validated.

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md).
