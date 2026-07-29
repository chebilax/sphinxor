# Documentation index

This is the entry point for Sphinxor's documentation. Read this first if you're a new contributor.

## Structure

- [`vision.md`](vision.md) — project vision, positioning, and philosophy. The single source of truth for scope. Read this before anything else.
- [`decisions/`](decisions/) — Architecture Decision Records (ADRs). One file per non-trivial decision, numbered, written at the moment the decision is made. This is where "why" lives — read it before assuming a design choice was arbitrary.
- [`testing.md`](testing.md) — testing philosophy: what gets validated, and against what.
- [`benchmarks.md`](benchmarks.md) — honest, numbers-based comparisons. Empty until real measurements exist.
- [`roadmap-long-term.md`](roadmap-long-term.md) — vision beyond the v0.1/v1/v2 sequencing in `vision.md`, kept separate so it doesn't pollute short-term execution planning.

## Why this structure exists

Sphinxor follows the same documentation convention as [Lynxor](https://github.com/chebilax/lynxor/tree/main/docs): documentation describes a process that already genuinely exists in the project. It doesn't invent one ahead of time, and it doesn't claim a rigor the project hasn't earned yet. An empty `benchmarks.md` is better than one with invented numbers.

All documentation is written in English from day one — no exceptions, no mixing languages within a file.

## For contributors

See [`CONTRIBUTING.md`](../CONTRIBUTING.md) at the repository root for the process: branching, when an ADR is required, and what's expected before a PR.
