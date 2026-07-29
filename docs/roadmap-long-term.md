# Long-term roadmap

This document covers direction beyond the v0.1 / v1 / v2 sequencing described in [`vision.md`](vision.md). It exists so long-term ambitions don't creep into short-term execution planning — nothing here is scheduled, and none of it should be started before the version it follows is actually done.

For the record, v0.1 through v2 as currently scoped:

- **v0.1** — one framework, extraction + intermediate model + minimal linting + allowlist mechanism + `sphinxor lint .` in CI.
- **v1** — drift detection between two versions of the model (the project's headline differentiator).
- **v2** — a second and third framework, richer documentation (Mermaid diagrams), a first export target (Cerbos or OPA).

## Beyond v2 (unscheduled, directional only)

- Additional framework coverage (Django, Symfony, ASP.NET), each added as its own effort — never batched, per the "one framework at a time" principle in `vision.md`.
- Additional export targets, each treated as its own project rather than a generic mapping, since RBAC/ABAC/ReBAC and policy-as-code engines don't translate into one another without semantic loss.
- Possible IDE integration (inline findings) once the CLI and CI workflow are mature and stable.
- Possible historical drift tracking across many commits (not just two points in time), if v1's two-version diff proves the underlying model is stable enough to support it.

None of the above is a commitment. This section will be revised as v0.1 and v1 actually ship and reveal what the model can and can't support.
