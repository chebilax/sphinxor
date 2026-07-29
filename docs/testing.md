# Testing philosophy

## Principle

Sphinxor's core value proposition is extracting an authorization model from real code. A test suite built only from synthetic fixtures cannot validate that claim — synthetic fixtures encode the author's assumptions about how the target framework is used, not how it's actually used in the wild.

Sphinxor is therefore validated empirically, against real, representative open source repositories in the target framework, not just hand-written fixtures. This is the same standard applied on [Lynxor](https://github.com/chebilax/lynxor/tree/main/docs).

## What this means in practice

- **Fixtures still exist** for fast, deterministic unit tests of individual rules and parsing logic (e.g. "this exact decorator shape produces this exact model node"). They're necessary but not sufficient.
- **Real-repo tests** run the full extraction + linting pipeline against a curated set of open source repositories in the target framework, and assert on the resulting model / findings. These catch the cases fixtures don't: unusual annotation composition, conditional configuration, real-world project structure.
- The set of real repositories used for this purpose is NestJS-specific, per [`decisions/0001-target-framework-choice.md`](decisions/0001-target-framework-choice.md), and will be documented once the first extraction pipeline exists. Listing candidate repositories before there's a pipeline to run them against would be premature.
- **False positive / false negative rates are measured, not assumed.** Any claim about detection quality in `benchmarks.md` must trace back to a specific run against a specific corpus, not intuition.

## What's out of scope for now

Formal verification, fuzzing of the parser, and performance benchmarking at scale are not part of the v0.1 testing effort. They may become relevant as the tool matures, but adding them now would be testing infrastructure ahead of the product it's meant to validate.
