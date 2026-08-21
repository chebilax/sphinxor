# 0008. Release binary platform coverage

## Status

Accepted.

## Context

`.github/workflows/release.yml`, added in the release-readiness pass this ADR
follows up on, builds the `sphinxor` binary natively per OS/arch on a tag push,
rather than cross-compiling from a single runner. That's not an arbitrary
implementation shortcut — it's a direct consequence of a choice this project already
made and stands by: `vision.md`'s "Technical architecture" section commits Sphinxor
to Tree-sitter as its parsing engine, "for multi-language parsing," specifically
because it's the standard, error-tolerant toolkit capable of handling real
decorator/annotation syntax (nested calls, multi-line expressions, string
escaping — exactly what ADR 0006's composite-decorator work had to reason about) in
a way a hand-rolled regex parser cannot, and because it's the same toolkit `vision.md`
already commits to reusing for every framework named in v2 (Spring/Java,
Django/Python, Symfony/PHP) — a decision worth restating here, not revisiting.

Tree-sitter's Go binding (`github.com/smacker/go-tree-sitter`) is a cgo wrapper
around the native C parser. This has a real, confirmed cost, not an assumed one:
`CGO_ENABLED=0 go build ./cmd/sphinxor` fails outright (undefined symbols in the
generated bindings), so a release binary can only be built on — and, absent a
cross-compilation C toolchain, effectively for — the OS/arch it's compiled on. This
is the correct, deliberate price of the parser choice above, not a flaw introduced
by the release workflow; the release workflow just has to plan around it honestly,
which is what this ADR does.

The release workflow shipped (PR #5) with two matrix legs — `linux/amd64`,
`darwin/arm64` — chosen as "the two platforms this can build for today" without
further justification of *why those two and not others*. That's the gap this ADR
closes: which platforms get a prebuilt binary is a real choice with a real
downside (a runner not in the matrix means no prebuilt binary, `go install` only),
worth stating deliberately rather than leaving as whatever happened to be convenient
to type first.

## Decision

**Final matrix: `linux/amd64`, `darwin/arm64`, `windows/amd64`.**

- **`linux/amd64`** — the default CI/server target; unchanged from the original matrix.
- **`darwin/arm64`** — Apple Silicon has been Apple's default since 2020; kept.
- **`darwin/amd64` (Intel Mac) is deliberately excluded** — a shrinking population at
  this point, not worth a fourth native runner (and its ongoing upkeep) for a
  pre-1.0, solo-maintained project to carry indefinitely.
- **`windows/amd64` is added** — NestJS/Node backend development has a real, non-trivial
  Windows population, and for many of those developers a prebuilt binary is the
  difference between trying Sphinxor and not: `go install` assumes a Go toolchain
  (and, per the note below, a C toolchain) already on the machine, a taller ask on
  Windows than on Linux/macOS for a non-Go-native backend developer.

**`go install` remains the fallback for every platform not in this matrix** — but
that fallback is not a zero-dependency escape hatch, and this ADR states that
honestly rather than leaving it implied: since the underlying build is cgo, `go
install` still requires a real C toolchain on the installing machine (MSVC or
mingw-w64 on Windows, Xcode Command Line Tools on macOS, `gcc`/`build-essential` on
Linux), not just a Go toolchain. `linux/arm64` and `darwin/amd64` users, among others,
depend on this fallback and its C-toolchain requirement today.

## Alternatives considered

- **Cross-compile from one Linux runner with `CGO_ENABLED=0`** — rejected outright,
  not as a style preference: confirmed by direct test that the build fails
  entirely without cgo, since the tree-sitter binding's generated Go code depends
  on its compiled C sources.
- **Cross-compile with a cross toolchain (osxcross for Linux→Darwin, mingw-w64 for
  Linux→Windows) from a single runner** — rejected for now: real, ongoing
  maintenance burden (pinned SDK versions, and specifically for osxcross, Apple SDK
  redistribution terms that are awkward for a public CI config) for a project at
  this stage. Native per-OS runners are simpler, and GitHub Actions provides
  `ubuntu-latest`/`macos-latest`/`windows-latest` natively at no extra setup cost —
  the more maintainable choice today, revisitable if runner cost or count ever
  becomes the actual bottleneck.
- **Adopt GoReleaser** (matching Lynxor) — still not adopted, consistent with the
  reasoning already recorded in `release.yml`'s own comments: GoReleaser doesn't
  itself dissolve the cgo cross-compilation constraint (it would still need the
  same per-OS toolchains, or a `zig cc`-based cgo cross-compile setup, under the
  hood); adopting it now would add a new tool dependency without removing the
  actual constraint this ADR is about.
- **Full matrix** (add `linux/arm64`, `darwin/amd64`, `windows/arm64`) — rejected:
  no evidence of real demand at this project's current stage strong enough to
  justify three more native runners and their upkeep. Additive later, from actual
  requests, without needing to reopen this ADR's reasoning — only its matrix.

## Consequences

- `release.yml`'s build matrix gains a `windows-latest` / `windows/amd64` leg once
  this ADR is Accepted — implemented as a follow-up change after acceptance, not
  bundled into this document.
- The Windows leg's archive format and binary name will differ from the Unix legs
  (`sphinxor.exe`, likely a `.zip` rather than `.tar.gz`, matching Windows
  convention) — an implementation detail of that follow-up change, not a further
  design decision this ADR needs to settle.
- If Tree-sitter's Go ecosystem ever produces a viable pure-Go binding (no cgo),
  the constraint this ADR works around would dissolve, and the "one native runner
  per platform" approach could be revisited in favor of trivial cross-compilation.
  Not expected soon; noted as the actual escape hatch rather than left unstated.
- Adding a platform later (e.g. `linux/arm64`, or reconsidering `darwin/amd64`) is a
  matrix-only change against this ADR's already-settled reasoning, not a reason to
  write a new one — unless the reasoning itself (not just the matrix) needs to
  change.
