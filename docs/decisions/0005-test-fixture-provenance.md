# 0005. Test fixture provenance: vendored copies vs. clone-on-demand

## Status

Accepted.

**Amendment 1 (re-evaluation at 2,747 lines): Proposed.** See *Amendment 1* below. It keeps
vendoring and replaces the line-count threshold with triggers tied to what vendoring
actually costs.

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

Where the license requires its text to travel with the code — MIT's notice condition,
Apache-2.0 §4(a) — the upstream license file is copied next to the `NOTICE.md`, unmodified.
Naming the license is not the same as including it. As of 2026-09-28, eight fixtures (seven
MIT, one Apache-2.0) had only the name, and their license texts were added then.

This scales linearly with corpus size: each additional real repository added for validation (the next planned step is a second one) adds a few more files and a few more KB, not a full checkout. If the corpus eventually grows large enough that this becomes real tree bloat, that's a reason to revisit this ADR with the clone-on-demand alternative back on the table — not a reason to abandon the reliability argument above without one.

### Growth so far

Recorded so the threshold above is reached deliberately rather than discovered late. At the time this ADR was written the corpus was one repository and four files. It now stands at **seven repositories and roughly 2,500 lines** of vendored source, having roughly tripled during [ADR 0020](0020-unanalyzable-is-unknown-not-absent.md) Amendment 2 alone — which added novu, cal.com, `eugenp/tutorials` and `ruoyi-vue-pro` for one decision.

That is still comfortably inside what vendoring is worth paying, and **no action is taken here**.

**Recounted 2026-09-28**, adding `trackr-backend` and `videochat` for
[ADR 0038](0038-role-hierarchy-read.md): **ten repositories and 2,747 lines**. There were
2,566 before them, and the "seven" above was already eight by then. That is within about
250 lines of the re-evaluation point below. The re-evaluation is due **before** the next
fixture is added, not with it.

The point of the number is the trajectory: if it continues at this rate, this ADR should be re-evaluated against the clone-on-demand alternative at around 3,000 lines, while the decision is still cheap to change — not at 8,000, when it isn't.

**A note on what counts.** `internal/extract/nestjs/testdata/ghostfolio-shape/` is *not* vendored source and does not count toward the figure above. Its shape comes from `ghostfolio/ghostfolio`, which is AGPL-3.0, where this repository and every fixture in it are permissive; the files were written for this repository instead of copied, and its `NOTICE.md` says so. That is a licensing decision, not a change to this ADR: vendoring remains the pattern for permissively licensed fixtures. A second copyleft-only shape would be worth surfacing as its own decision rather than settling by precedent here.

## Amendment 1 — the re-evaluation at 2,747 lines (2026-09-28)

### Status

Proposed.

### Context

*Growth so far* set a re-evaluation "at around 3,000 lines, while the decision is still
cheap to change", and the recount there made it due before the next fixture. This is
that re-evaluation. Everything below was measured on `main` at `85144eb`.

**What the vendored corpus costs today.**

| Cost | Measured |
|---|---|
| Vendored source | 10 repositories, 27 files, **2,747 lines, 101 KB** (ghostfolio-shape excluded, as above) |
| With licenses and NOTICEs | 161 KB for all of `testdata/`. **8.8%** of the tracked tree's 1.83 MB. The ADRs and the Go code are most of the rest. The packed repository is 1.26 MiB |
| Largest fixture | novu, 733 lines. The license text outweighs the vendored source for videochat (Apache-2.0, 11 KB against 2 KB) |
| Test time | the full suite, uncached: **2.0 s** wall. Analyzing all eleven fixtures end to end with the binary: **0.47 s** |
| Use | 20 test files reference a fixture. Pharmacy and nestjs-boilerplate are referenced 10 times each; five fixtures are referenced once |
| Maintenance | none recurring. Every file is unmodified upstream code at a pinned commit, and none has been edited since it was vendored |
| Licensing | one license file per fixture, added for all ten on 2026-09-28 |

**How it grew.** Measured from the date each `NOTICE.md` was added:

| Date | Added | Cumulative lines |
|---|---|---:|
| 2026-07-30/31 | nestjs-boilerplate, awesome-nest-boilerplate | 608 |
| 2026-08-29 | Pharmacy, blog-api | 1,155 |
| 2026-09-20 | cal.com, novu, ruoyi-vue-pro, tutorials — one decision, ADR 0020 Amendment 2 | 2,566 |
| 2026-09-28 | trackr-backend, videochat — ADR 0038 | 2,747 |

Growth is lumpy, not steady: one decision added 51% of today's total. The "rate" the ADR
worried about is a count of decisions that need real input, not a slope.

**What clone-on-demand would cost instead.** Measured for the same ten pins:

| Cost | Measured |
|---|---|
| Whole upstream repositories | about **2 GB** together; cal.com alone is 1.1 GB, novu 491 MB |
| A realistic fetch | depth 1, `--filter=blob:none`, sparse checkout of the vendored paths only: **~2 s and 1.6–11 MB per repository**, mostly tree objects — cal.com 2.2 s / 1.6 MB, tutorials 1.8 s / 11 MB. For all ten, on the order of 20 s and tens of MB per uncached run |
| Availability | all ten pinned commits still resolve, and none of the ten repositories is archived. The risk the ADR named — a repository deleted or force-pushed — has not happened. It is also not zero, and it grows with the number of pins |
| New machinery | a fetch step in `make check` and CI, a cache for it, a way to run offline, and a decision about what the test does when the fetch fails |

### Decision

**Keep vendoring. Clone-on-demand is not worth revisiting at this size.**

The 3,000-line figure was a proxy for "tree bloat", and on every axis that bloat would
show up on, the corpus is small:

- a tenth of the tree;
- half a second of analysis;
- no maintenance.

Clone-on-demand would add roughly 20 s and a network dependency to a 2 s check. That is
the exact reliability cost the original decision rejected, paid to remove 161 KB.

**The threshold changes from a line count to the costs themselves.** This ADR is
re-evaluated when any one of these is first true:

1. `testdata/` exceeds **1 MB**, or **25%** of the tracked tree's bytes;
2. analyzing the vendored fixtures takes more than **5 s** of the check;
3. a single decision needs more than **~20 files** from one repository. That is a
   subset too large to curate by hand, which is where vendoring stops being "a small,
   curated subset" in this ADR's own words;
4. a fixture has to be **edited** to stay useful — for example because the extractor now
   needs a file the original subset did not include, repeatedly. Unmodified is the
   property that makes a vendored file trustworthy.

At today's lumpy rate, trigger 1 is roughly six times the current size away. The
triggers are recorded next to the numbers above, so each re-check is a measurement
rather than a judgment.

**The split that already exists is kept, and named.** The 20-repository measurement
corpus ([ADR 0035](0035-permissions-in-the-model.md) §9) is already clone-on-demand:
it is re-cloned at pinned commits whenever a decision is measured, and it is not part of
`make check`. That is the right place for clone-on-demand. Measurement runs by hand, can
tolerate a network, and needs whole repositories. Regression tests run on every PR,
cannot tolerate a network, and need a few files. The two alternatives this ADR weighed
are both in use, each where it fits.

### Alternatives considered

- **Switch to clone-on-demand now.** Rejected on the numbers above: it buys 161 KB and
  costs a network dependency and roughly tenfold check time.
- **Keep the line-count threshold and raise it.** Rejected. Lines are not what costs
  anything here, and a raised number would be re-evaluated on the same wrong axis.
- **A hybrid: vendor small fixtures, fetch large ones.** Rejected for now. No fixture is
  large (novu's 733 lines is the maximum), and the hybrid carries both mechanisms'
  costs. Trigger 3 is the condition under which it would become worth designing.

### Consequences

- *Growth so far* stops being the trigger. Its numbers remain as history.
- Each new fixture's PR states the current `testdata/` size and share of the tree, so the
  triggers are checked as a matter of course rather than rediscovered.
- No code or test changes.

