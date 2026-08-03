# Security Policy

Sphinxor reconstructs the authorization model of other people's repositories, so a
vulnerability in Sphinxor itself deserves at least the same seriousness it applies
to the code it analyzes.

## Supported versions

Sphinxor is pre-1.0 and solo-maintained. Only the latest tagged release and `main`
are supported — there is no long-term-support branch yet.

| Version | Supported |
|---|---|
| latest tag | ✅ |
| `main` | ✅ |
| older tags | ❌ |

## Reporting a vulnerability

**Please use [GitHub's private vulnerability reporting](https://github.com/chebilax/sphinxor/security/advisories/new)**
for this repository, rather than a public issue. This keeps the report private
until a fix is available.

If that isn't available to you for some reason, open a regular issue asking to be
contacted privately — don't post exploit details or proof-of-concept code in a
public issue.

Please include:
- What you found and where (file/line if applicable).
- A minimal way to reproduce it.
- What you think the impact is (what an attacker could actually do with it).

## What counts as a security issue here

In scope:
- Anything that lets a scanned repository's content (a crafted `.ts` file, a
  pathological decorator/comment shape) cause Sphinxor to execute code, read/write
  outside the scanned path, crash in an exploitable way, or otherwise do something
  the person running it didn't ask for. Sphinxor's extraction depends on
  `github.com/smacker/go-tree-sitter`, a cgo binding around a native parser —
  a memory-safety bug reachable by feeding it adversarial source is in scope even
  though the bug would technically live in a dependency, not Sphinxor's own Go code.
- Supply-chain issues in Sphinxor's own dependencies or release/distribution path
  (`go.mod`/`go.sum`, the GitHub Actions release workflow, `go install`).

Out of scope (please file as a regular bug instead):
- False positives or false negatives in the lint rules or extraction (a guard
  Sphinxor misses, a finding it gets wrong, a role it fails to resolve) — these are
  accuracy bugs, not vulnerabilities in Sphinxor itself, per `docs/vision.md`'s own
  framing of this as heuristic, confidence-graded analysis rather than formal
  verification. The exception: if the false negative is *caused by* an exploitable
  flaw in the extraction/parsing logic rather than a rule simply not covering that
  case yet, it's in scope above.
- Findings about Sphinxor's own third-party dependencies (`go.mod`/`go.sum`) —
  please open a regular issue or PR; these get the same triage as any other
  dependency update.

## Response

This is a solo-maintained project. There's no SLA, but reports will be
acknowledged and triaged as soon as reasonably possible, and credited in the fix's
commit/release notes unless you ask not to be.
