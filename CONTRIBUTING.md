# Contributing

This describes the real process used on this project, not an aspirational one.

## Branching

One branch per feature, opened against `main`. No direct commits to `main`.

## Architecture Decision Records

Any non-trivial decision requires an ADR before merge, not after. This includes (non-exhaustively): the intermediate model's structure, the allowlist format, confidence-level granularity, and the target framework itself.

- ADRs live in [`docs/decisions/`](docs/decisions/), numbered sequentially (`0001-...`, `0002-...`).
- Format: context, decision, alternatives considered and why they were rejected, consequences.
- Written at the moment the decision is made — not reconstructed afterward to justify code that already exists.
- If you're unsure whether a decision is "non-trivial" enough to need one, open the PR with a draft ADR and ask; it's cheaper to skip an unnecessary one than to reconstruct a missing one later.

## Before opening a PR

- Empirical validation against real code, per [`docs/testing.md`](docs/testing.md) — synthetic fixtures alone are not sufficient for anything touching extraction or linting logic.
- `make check` passes locally.
- Any new non-trivial decision has a corresponding ADR.

## Documentation language

All documentation, code comments, and commit messages are written in English. No exceptions, no mixing languages within a file.
