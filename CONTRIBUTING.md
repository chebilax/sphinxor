# Contributing

This describes the real process used on this project, not an aspirational one.

## Setup

After cloning, point git at the repository's hooks:

```sh
git config core.hooksPath .githooks
```

Install the Cerbos CLI, which the real-engine export tests run against. Without it they skip, and `make check` ends with a notice saying so. Use the version CI pins as `CERBOS_VERSION` in `.github/workflows/ci.yml`. For example, on macOS (Apple silicon):

```sh
V=0.55.0   # keep in step with CERBOS_VERSION in .github/workflows/ci.yml
curl -fsSLO "https://github.com/cerbos/cerbos/releases/download/v$V/cerbos_${V}_Darwin_arm64.tar.gz"
curl -fsSLO "https://github.com/cerbos/cerbos/releases/download/v$V/checksums.txt"
grep " cerbos_${V}_Darwin_arm64.tar.gz\$" checksums.txt | shasum -a 256 -c -
tar -xzf "cerbos_${V}_Darwin_arm64.tar.gz" cerbos && mv cerbos ~/.local/bin/   # any directory on PATH
```

Other platforms use the matching archive from the same release, such as `cerbos_${V}_Linux_x86_64.tar.gz`.

`.githooks/commit-msg` rejects a commit message carrying an authorship or tool-credit trailer. Commits are authored by the repository owner only. The hook enforces that for commits; PR descriptions, comments and release notes follow the same rule by hand.

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
