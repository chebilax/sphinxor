# Provenance

Vendored, unmodified, from https://github.com/novuhq/novu,
commit `36c5c0cefa400e8edc6ec8b18ac4574288eabd03` (2026-09-18), MIT License.

Novu is split-licensed: `enterprise/packages` is proprietary, everything else
is MIT (`LICENSE-ENTERPRISE`, first bullet). Both files here are under
`apps/api`, so MIT applies.

Used per docs/testing.md and docs/decisions/0020-unanalyzable-is-unknown-not-absent.md
Amendment 2 §7: the **readable-version** fixture. novu bootstraps
`VersioningType.URI` with `defaultVersion: '1'`, so these two controllers are
two distinct applications-facing routes — `/v1/topics` and `/v2/topics` — that
collapse onto one `(method, path)` identity when the `version` key is not read.

Files, and why each was picked:

- `apps/api/src/app/topics-v1/topics-v1.controller.ts` — `@Controller('/topics')`
  with **no version declared**. This is the half of the pair that must keep its
  existing path-derived `NewEndpointID`: "no version" is *absent*, not
  *unknown*, and the whole bounding argument in §7 rests on endpoints like this
  one being byte-for-byte unaffected.
- `apps/api/src/app/topics-v2/topics.controller.ts` — `@Controller({ path:
  '/topics', version: '2' })`, a plain string literal. The readable-version
  half. Picked over the `subscribers` pair named in the ADR because it is the
  same mechanism in the same repository at roughly half the vendored size (see
  docs/decisions/0005-test-fixture-provenance.md on corpus growth).

The pair shares five `(method, path)` identities — `GET`/`POST /topics`,
`GET`/`PATCH`/`DELETE /topics/:topicKey` — so it exercises identity separation,
not merely version reading.
