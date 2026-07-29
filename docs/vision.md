# Sphinxor – Project Vision

Sphinxor is part of the same ecosystem as **[Lynxor](https://github.com/chebilax/lynxor)**, sharing the same philosophy: analyze, detect, explain — never decide on the developer's behalf.

[Lynxor](https://github.com/chebilax/lynxor) deals with the security of a Git repository.
Sphinxor deals with the quality of the **authorization model** (IAM/RBAC/ABAC) living inside that repository.

## Vision

> **The Sphinx doesn't build the gate. It decides who gets to pass.**

Sphinxor **is not** an authorization engine like Keycloak, Cerbos, OPA, Casbin, or OpenFGA. It makes no runtime access decisions. It solves a problem that sits **upstream** of those tools: reconstructing, auditing, and documenting an application's actual authorization model — the one that exists *in the code*, not the one declared in a spec that's drifted for the last six months.

**Positioning:**

> **The drift detector for your authorization model.**
> *Design, audit and document what your code actually enforces.*

Sphinxor doesn't replace Keycloak or Cerbos. It integrates with them and becomes the source of truth for the authorization model — built from the code itself, not from an intent declared elsewhere.

---

## Why this project, and why now

### What already exists — and why it's not enough

**Authorization-as-code platforms (Permit.io and similar)** work top-down: you declare a policy (UI, CLI, or OpenAPI spec), then wire it into your code via an SDK. That's excellent for *starting* a clean authorization system. But it says nothing about what's **already in production** in an existing codebase, and a declared policy can silently drift from the real code as PRs pile up. Sphinxor works the other way around: the code is the truth, and the declared policy (if any) is checked against it — not the reverse.

**General-purpose SAST tools (Semgrep, CodeQL, Snyk, SonarQube)** structurally miss business-logic authorization issues (IDOR, missing access control), because they do pattern-matching without ever building an explicit model of what should be protected. Recent comparisons show very low detection rates on this exact category, with high false-positive rates as soon as you step outside generic patterns. This is exactly why Sphinxor doesn't try to "detect authorization vulnerabilities" in general — it first builds a structured model (resources, actions, roles), then checks that model for consistency. The model is the product; vulnerability detection is a consequence of it, not the starting point.

**The pattern already exists — elsewhere.** On Kubernetes, tools like Krane or KubiScan perform static analysis of RBAC configurations to surface risks. In smart contracts, recent research automatically extracts role-permission pairs from Solidity source code. Nobody has built the equivalent, in depth, for mainstream web frameworks (Spring, NestJS, Django, Symfony, ASP.NET). That's Sphinxor's open lane — and it should be framed this way in every piece of communication: *what Krane does for Kubernetes, Sphinxor does for your application backend.*

### What makes this problem hard (and why it should be said out loud)

A "real" authorization policy for an endpoint can depend on composed annotations, global filters, conditional configuration, AOP — not just a decorator visible on the function. Purely syntactic analysis (what Tree-sitter alone gives you) will have blind spots, the same way rule-based SAST tools did before it. Sphinxor must openly own that it does **heuristic, confidence-graded analysis** (high-confidence findings / needs manual review), not formal verification — and never promise the completeness that general-purpose SAST already promised and failed to deliver. That's an honest positioning choice that builds trust instead of destroying it at the first false positive.

---

## Features — sequenced roadmap

The original document covered six frameworks, five exporters, and eight rule families all at once. That's a two-year plan presented as a launch. The version below sequences by proof of value.

### v0.1 — proof of value (one framework, done properly)

* Pick **one** starting framework (whichever you have the most real expertise in — Spring or NestJS are good candidates given their heavy use of explicit decorators/annotations, which makes them easier to model faithfully).
* Extraction: endpoints, authorization guards/decorators/annotations, declared roles and permissions.
* Build the intermediate model (resources / actions / roles / permissions).
* Minimal but useful linting:
  * mutating endpoint (POST/PUT/DELETE) with no detected access control;
  * permission declared but never referenced;
  * empty role.
* **Explicit allowlist mechanism from v0.1 onward**: a developer must be able to mark a route as "public, on purpose" (annotation, comment, or `.sphinxor-allow` config file) — without this, the first false positive on a login endpoint or a healthcheck gets the tool disabled in CI within a week, exactly the trap that limited adoption of general-purpose SAST in this space.
* Output: RBAC matrix in Markdown + JSON.
* `sphinxor lint .` as a CLI, non-zero exit code on high-confidence findings — usable in CI from this version on.

### v1 — the real differentiator: drift

* **Diff between two versions of the model**: new permissions, removed permissions, endpoints that became public, security regressions between two commits or two PRs.
* CI/CD integration: fail a PR when a regression is detected in the model relative to the reference branch.
* This feature — not the initial model generation — is the hardest to replicate and the hardest to find elsewhere today. It deserves to be the project's headline message, not a line in a feature list.

### v2 — expansion

* Second and third frameworks.
* Mermaid diagrams, richer documentation.
* Export to a first target engine (Cerbos or OPA, whichever model is closest to yours) — treat each subsequent export as its own project: RBAC, ABAC, ReBAC, and policy-as-code (Rego/Cedar) don't translate cleanly into one another without semantic loss; it's not a simple field mapping.

---

## Technical architecture

Core in **Go**, consistent with [Lynxor](https://github.com/chebilax/lynxor) (possible sharing of internal modules, CLI conventions, and developer experience — same team, same stack, a real asset for the credibility of the ecosystem).

* CLI (Cobra), same usage conventions as [Lynxor](https://github.com/chebilax/lynxor).
* Analysis engine based on Tree-sitter for multi-language parsing.
* Framework-independent intermediate model.
* Rule engine with confidence levels per finding (no binary "vulnerable/not vulnerable" findings).
* Allowlist/suppression mechanism versioned with the code (not an external database).
* Exporters and plugins added one framework at a time, never all at once.

---

## Documentation — same standard as Lynxor, in English from day one

Sphinxor fully adopts the documentation convention already in place on [Lynxor](https://github.com/chebilax/lynxor/tree/main/docs). This isn't a layer bolted on afterward to look "professional" — it's the same real process, documented as it happens, starting from the very first decision.

**All documentation must be written in English** — `docs/`, ADRs, `README.md`, `CONTRIBUTING.md`, `CHANGELOG.md`, code comments, commit messages. No exceptions and no mixing languages within a file. This is a deliberate departure from [Lynxor](https://github.com/chebilax/lynxor/tree/main/docs), where documentation started in French and later needed a costly, partial retrofit to English; Sphinxor starts clean instead of paying that debt later.

* **`docs/README.md`** — entry point explaining the structure of `docs/` and why each decision was made; the first file a contributor should read.
* **`docs/decisions/`** — one ADR (Architecture Decision Record) per non-trivial decision, numbered (`0001-...`, `0002-...`), written at the moment the decision is made, not reconstructed afterward. Short format: context, decision, alternatives considered and why they were rejected, consequences. On Sphinxor this typically covers: choice of the first target framework, structure of the intermediate model, the allowlist mechanism, the export format.
* **`docs/benchmarks.md`** — honest, numbers-based comparisons, not marketing claims: "why Sphinxor and not just a custom Semgrep rule," performance on real repositories, false positives measured against a real corpus of Spring/NestJS projects.
* **`docs/testing.md`** — explicit testing philosophy: empirical validation against real code (real, representative open source repositories), not just synthetic mocks — same standard as [Lynxor](https://github.com/chebilax/lynxor/tree/main/docs), tested against real repos/APIs rather than fabricated cases.
* **`docs/roadmap-long-term.md`** — vision beyond the v0.1/v1/v2 sequencing above, kept separate so it doesn't pollute the short-term execution roadmap.
* **`docs/vision.md`** — this document, once the repo is initialized.
* **`CONTRIBUTING.md`** (root) — describes the real process, not an aspirational one: branch per feature, ADR required for any non-trivial decision (points to `docs/decisions/`), empirical validation before merge, `make check` before any PR.
* **`CHANGELOG.md`** (root) — Keep a Changelog format, one entry per tagged version.
* **`SECURITY.md`** (root) — private vulnerability disclosure process (GitHub private reporting or a dedicated contact), supported versions. Particularly relevant here: Sphinxor inspects other people's authorization code, so a false negative in its own findings, or a flaw in the tool itself, needs a clear, non-public reporting path from day one — not something to bolt on once the project has users.

The principle that proved itself on [Lynxor](https://github.com/chebilax/lynxor/tree/main/docs) and must be repeated here as-is: documentation describes a process that already genuinely exists in the project — it doesn't invent one ahead of time, and it doesn't claim a rigor the project hasn't earned yet. An empty `docs/benchmarks.md` is better than one that states numbers nobody measured.

---

## Philosophy

[Lynxor](https://github.com/chebilax/lynxor) improves the security of a repository.
Sphinxor improves the quality and reliability of the authorization model living inside that repository — and detects when it drifts.

The project makes no authorization decisions. It:

* analyzes;
* detects;
* explains, with an honestly stated confidence level;
* documents;
* alerts on drift between two versions;
* offers exports to existing engines, without ever claiming to replace them.

The goal isn't to be exhaustive from day one, but to become the progressive reference for auditing and continuously documenting authorization models, independent of whichever IAM engine is used downstream.
