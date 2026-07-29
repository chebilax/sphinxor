# 0004. Confidence level granularity

## Status

Proposed — pending confirmation before any implementation starts.

## Context

`vision.md` requires every finding to carry an honestly stated confidence grade — "no binary 'vulnerable/not vulnerable' findings" — because Sphinxor does heuristic, confidence-graded analysis, not formal verification, and overstating certainty is exactly what erodes trust on the first false positive. The CLI is also required to gate on this from v0.1: `sphinxor lint .` must produce "non-zero exit code on high-confidence findings — usable in CI from this version on."

`vision.md`'s own wording, in the section explaining why heuristic analysis must be owned openly, names a specific split: *"high-confidence findings / needs manual review."* That phrase is doing two jobs at once: it's the honesty framing for output, and it's the literal boundary the CI exit code needs (fail on the first bucket, don't fail on the second). Any granularity chosen here has to keep that boundary meaningful — it can't average it away into something CI can no longer act on unambiguously.

This was deferred out of [`0002-intermediate-model-structure.md`](0002-intermediate-model-structure.md), which only commits to `Finding` having *a* `Confidence` field, not its concrete values — this ADR settles those values. The current skeleton (`internal/model/model.go`) defines `Confidence` as a bare `string` type with no constants, specifically so this decision isn't made by omission.

## Decision

**Not yet made.** Three options, all consistent with "no binary vulnerable/not-vulnerable," differing in how much they ask v0.1 to calibrate.

### Option A — Two-tier: `HighConfidence` / `NeedsReview`

Takes `vision.md`'s own wording as the literal spec: exactly two grades. `HighConfidence` gates CI (non-zero exit); `NeedsReview` is reported but never gates. No further subdivision.

- **Pro**: directly grounded in `vision.md`'s text — nothing invented. Minimal calibration burden: extraction and lint rules only ever have to answer one yes/no question ("is this certain enough to fail a build over"), not rank degrees of uncertainty. The CI-gating rule vision.md specifies is a one-line filter.
- **Con**: no room to distinguish "worth a comment on the PR" from "worth ignoring by default" within the non-gating bucket — everything that isn't high-confidence lands in one undifferentiated pile. If that pile turns out noisy in practice, there's no grade to de-prioritize by without a later migration.

### Option B — Three-tier ordinal: `High` / `Medium` / `Low`

CI gates on `High` by default (matching vision.md's rule), with the threshold configurable later. `Medium` and `Low` exist to let output and future tooling (dashboards, PR comments) prioritize without a hard gate.

- **Pro**: more room to be honest about degrees of uncertainty instead of collapsing them into one bucket — a guard resolved through straightforward decorator composition and one resolved through a heuristic that had to guess at an ambiguous case are both "not gating," but they aren't equally certain, and this lets that show.
- **Con**: `Medium` needs its own honest, articulable definition — what specifically demotes a finding from `High` to `Medium` rather than `Low`? Inventing that boundary now, before real NestJS fixtures show what the actual gradations of uncertainty look like in practice, risks encoding a distinction that doesn't match reality and has to be redrawn later anyway.

### Option C — Numeric score (0.0–1.0) with named threshold bands

Findings carry a continuous score; CI gates above a configurable threshold (default matching vision.md's "high-confidence" bar).

- **Pro**: most flexible for future tuning — thresholds can move without a data model change, and a future scoring heuristic could produce a real number instead of being forced into a bucket.
- **Con**: a bare number reads as more precise than a heuristic, confidence-graded tool can honestly justify — `vision.md`'s entire positioning is about not overstating what static analysis can promise, and a score like `0.73` implies a calibration exercise (what does 0.73 mean, precisely?) that hasn't been done and that v0.1's rule set (three simple rules) doesn't produce enough data to justify. This risks reintroducing, in a different form, exactly the false precision `vision.md` warns general-purpose SAST tools already got wrong.

## Consequences

Whichever option is chosen fills in `Confidence`'s concrete values in `internal/model/model.go`, and unblocks `hasBlockingFindings` in `internal/cli/lint.go`, which is currently a stub pending this decision. Each of the three v0.1 lint rules will need to assign a grade to every finding it produces, so this also shapes how much justification each rule has to carry for the grade it picks.
