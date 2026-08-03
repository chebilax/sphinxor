## What changed and why

<!-- The diff already shows what changed -- explain why. -->

## ADR

<!-- If this is a non-trivial decision (a real choice between alternatives -- a library, a data format, a trade-off with a real downside), link the ADR in docs/decisions/. Delete this section if not applicable. -->

## Validation

- [ ] `make check` passes (`go build`, `go vet`, `gofmt -l`, `go test`)
- [ ] Validated empirically, not just "it compiles" -- against real NestJS repos for anything touching extraction/linting, per `docs/testing.md`
- [ ] Docs updated alongside the code (`docs/testing.md`, `docs/limitations.md`, `docs/roadmap-long-term.md`, or an ADR), if applicable

<!-- See CONTRIBUTING.md for the full process this project follows. -->
