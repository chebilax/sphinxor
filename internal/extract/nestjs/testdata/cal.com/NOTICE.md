# Provenance

Vendored, unmodified, from https://github.com/calcom/cal.com,
commit `54343aa685ae8f33159d2f485ec4a57bad5c574a` (2026-09-20), MIT License.

Used per docs/testing.md and docs/decisions/0020-unanalyzable-is-unknown-not-absent.md
Amendment 2 §7: the **unreadable-version** fixture, and the only vendored
evidence of real guard bleed caused by a collapsed version.

cal.com bootstraps `VersioningType.CUSTOM` with a header extractor
(`cal-api-version`), so the version never appears in the URL. Both controllers
declare `path: "/v2/event-types"` literally and are distinguished at runtime by
that header alone.

Files, and why each was picked:

- `.../event-types_2024_04_15/controllers/event-types.controller.ts` —
  `version: [VERSION_2024_04_15, VERSION_2024_06_11]`, an **array of constant
  references**. Unreadable as a literal, which is the majority shape in real
  versioned code and the case §7 routes to *unknown* rather than guessing at.
  Its `GET /` carries `@UseGuards(ApiAuthGuard)` — authentication required.
- `.../event-types_2024_06_14/controllers/event-types.controller.ts` —
  `version: VERSION_2024_06_14_VALUE`, a single constant reference. Its `GET /`
  carries `@UseGuards(OptionalApiAuthGuard)` — authentication optional.

The guard difference is the point. Merged onto one identity, the endpoint whose
authentication is *optional* is reported carrying the mandatory `ApiAuthGuard`
of the other, and the endpoint requiring authentication is reported carrying an
optional guard it does not have. Both halves of the pair are misreported, in
opposite directions, from one collapsed identity.

Picked over the `bookings` pair named in the ADR: same repository, same defect,
same guard-difference shape, at roughly a third of the vendored lines (see
docs/decisions/0005-test-fixture-provenance.md on corpus growth).
