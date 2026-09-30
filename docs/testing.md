# Testing philosophy

## Principle

Sphinxor's core value proposition is extracting an authorization model from real code. A test suite built only from synthetic fixtures cannot validate that claim — synthetic fixtures encode the author's assumptions about how the target framework is used, not how it's actually used in the wild.

Sphinxor is therefore validated empirically, against real, representative open source repositories in the target framework, not just hand-written fixtures. This is the same standard applied on [Lynxor](https://github.com/chebilax/lynxor/tree/main/docs).

## What this means in practice

- **Fixtures still exist** for fast, deterministic unit tests of individual rules and parsing logic (e.g. "this exact decorator shape produces this exact model node"). They're necessary but not sufficient.
- **Real-repo tests** run the full extraction + linting pipeline against a curated set of open source repositories in the target framework, and assert on the resulting model / findings. These catch the cases fixtures don't: unusual annotation composition, conditional configuration, real-world project structure.
- The set of real repositories used for this purpose is NestJS-specific, per [`decisions/0001-target-framework-choice.md`](decisions/0001-target-framework-choice.md), and will be documented once the first extraction pipeline exists. Listing candidate repositories before there's a pipeline to run them against would be premature.
- **False positive / false negative rates are measured, not assumed.** Any claim about detection quality in `benchmarks.md` must trace back to a specific run against a specific corpus, not intuition.
- **The real-fixture bar applies where real code can surprise the tool, not everywhere code-adjacent.** Its value is proportional to what real input exposes that a hand-written one can't: unusual annotation composition, a framework quirk nobody would think to fabricate, a genuine mismatch between two independent authorization layers (`decisions/0012-securityfilterchain-effective-policy.md`'s `SupplierController` finding). That's the bar extraction code is held to, non-negotiably. It does not extend to pure downstream logic whose input is already a framework-independent value (a set of role-name strings, an ID, a bool) — a real repository can reveal nothing about how `internal/export/cerbos`'s set-intersection reduction handles two disjoint role sets that a hand-built pair of sets doesn't reveal equally, because by the time that logic runs, "where the strings came from" is no longer part of the computation. Reaching for a third real repository specifically to exercise such a branch, when no repository already vendored for a legitimate extraction reason happens to exhibit it, is chasing the rule's letter past its purpose — accept synthetic coverage there instead, and say so in the test's own comment, not silently.

- **Published figures come from the extractor, never from grep or a regular expression.** A count that appears in an ADR, a PR, `docs/limitations.md` or a release note is produced by running Sphinxor's own extraction (or a measurement built on its parser and helpers) over the pinned inputs. Text search sees mentions as well as declarations and misses shapes it was not written for. Every count made another way on 2026-09-28 was wrong: yudao's `@PermitAll` usage was first given as 109 uses on 97 methods, 38 of them mutating, and the extractor found 106 endpoints, 53 of them mutating, because three grep hits were comments and a regular expression skipped annotations with nested parentheses. Grep remains fine for *finding where to look*; it is not a source for a number anyone will read.

- **The measurement corpus changes only by an ADR** ([ADR 0046](decisions/0046-corpus-extension.md)). Candidates are found by scope (Spring Security as a dependency), never by searching for a shape under measurement. A project is excluded for what it is (no web dependency, a tutorial, a fork), never for what the tool makes of it: a web project where Sphinxor finds no endpoints stays in, as a finding. A figure from the corpus is an occurrence, "this shape exists in real applications", never a rate.

- **A coverage change reports the rules it newly exports, and justifies each one.** Making an unknown known makes the export more willing to grant, so it can uncover an over-grant that was hidden behind the unknown: [ADR 0023](decisions/0023-third-party-authorization-annotations.md) Amendment 1 was found this way. "Existing exported rules are unchanged" is not enough for such a change. The PR lists every new exported rule, grouped as it helps, and says why each grant is correct.

## What's out of scope for now

Formal verification, fuzzing of the parser, and performance benchmarking at scale are not part of the v0.1 testing effort. They may become relevant as the tool matures, but adding them now would be testing infrastructure ahead of the product it's meant to validate.
