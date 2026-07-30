# 0005. Test fixture provenance: vendored copies vs. clone-on-demand

## Status

Accepted.

## Context

`docs/testing.md` requires empirical validation against real, representative open source repositories, not just synthetic fixtures — and requires that standard to be routine, not occasional: `make check` (fmt, vet, build, test) is the gate `CONTRIBUTING.md` requires before every PR, and CI runs the same. If the tests that satisfy `testing.md`'s "real code" requirement aren't part of that routine gate, the requirement degrades from an enforced standard to an occasional manual spot-check.

`internal/extract/nestjs/testdata/nestjs-boilerplate/` currently holds four files copied from `brocoders/nestjs-boilerplate` (MIT, commit pinned and documented in `NOTICE.md`), exercised by `extract_test.go` as a reproducible regression test. That's a real architectural choice with a real alternative, made while implementing extraction rather than surfaced beforehand — this ADR corrects that, per the project's own rule against picking silently among reasonable options.

## Decision

**Vendor a small, curated, attributed subset of files directly into the repository as `testdata/`.**

The deciding factor is reliability of the check gate itself: a test suite that needs network access at test time either makes `make check` non-deterministic and environment-dependent (fails offline, fails in a network-restricted CI runner, fails if the upstream repo is renamed, made private, or deleted), or has to be split into a separate, easy-to-forget, easy-for-CI-to-silently-skip optional target — which undermines exactly the rigor `testing.md` is trying to establish by making "real code" validation routine. Vendored files make the empirical-validation tests exactly as reliable as every other test in the suite: `go test ./...`, no exceptions, no network, no flakiness from an upstream repo changing shape under us.

Licensing is handled per-file in `NOTICE.md` (source URL, pinned commit, license, and why each file was chosen) — already correct, not a consequence of this ADR.

### Alternative — integration test that clones a pinned SHA on demand

Keeps foreign source code out of the repository entirely; expanding the corpus is adding a URL and a commit SHA rather than copying files.

- **Pro**: no third-party source living in the tree, even attributed. Scales better than vendoring as the corpus grows to many repositories (see Consequences).
- **Con, and reason for rejection now**: introduces a network dependency into the check gate, with all the reliability costs above. A pinned SHA also doesn't fully guarantee reproducibility the way a vendored copy does — the hosting service can be unreachable, or (rarer, but not impossible) a repository can be deleted or force-pushed over even at a specific commit if a maintainer rewrites history. Rejected for v0.1's small corpus (one repo, four files); worth revisiting if the corpus grows large enough that vendoring's tree-bloat cost starts to outweigh the reliability argument (see Consequences).

## Consequences

Test fixture provenance for real-repo validation follows this pattern going forward: a small, curated subset of files (not a full repo clone) vendored under `internal/extract/nestjs/testdata/<repo-name>/`, with a `NOTICE.md` documenting source URL, pinned commit, license, and why each specific file was chosen — not copied wholesale.

This scales linearly with corpus size: each additional real repository added for validation (the next planned step is a second one) adds a few more files and a few more KB, not a full checkout. If the corpus eventually grows large enough that this becomes real tree bloat, that's a reason to revisit this ADR with the clone-on-demand alternative back on the table — not a reason to abandon the reliability argument above without one.
