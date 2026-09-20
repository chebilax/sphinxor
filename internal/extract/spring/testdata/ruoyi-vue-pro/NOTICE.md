# Provenance

Vendored, unmodified, from https://github.com/YunaiV/ruoyi-vue-pro,
commit `8e80602b875f69158faf4c92445e67691821bf0f` (2026-09-19), MIT License.

Used per docs/testing.md and docs/decisions/0020-unanalyzable-is-unknown-not-absent.md
Amendment 2 §8: the **configured-path-prefix** fixture, and the project's
evidence that a same-path collision can hide a genuinely unguarded mutating
endpoint in production code.

Both controllers declare `@RequestMapping("/member/user")` literally, in one
application and one Maven module. They are separated at runtime only by
`YudaoWebAutoConfiguration`, which registers a `WebMvcRegistrations` bean
calling `RequestMappingHandlerMapping.setPathPrefixes(...)` to prefix
`/admin-api` and `/app-api` according to the controller's Java **package
name** — an arbitrary runtime predicate, categorically unreadable by static
analysis, in the same class as ADR 0012's custom `AuthorizationManager`.

Files, and why each was picked:

- `.../controller/admin/user/MemberUserController.java` — the admin side.
  `PUT /member/user/update` carries
  `@PreAuthorize("@ss.hasPermission('member:user:update')")`.
- `.../controller/app/user/AppMemberUserController.java` — the app side. Its
  own `PUT /member/user/update` carries **no access control at all**.

Measured before §8: extraction reported one `PUT /member/user/update`,
attributed to the admin controller and carrying its `@PreAuthorize`, while the
app-side handler was absent from the report entirely and
`mutating-endpoint-without-access-control` did not fire for it. Linting the app
package on its own flagged it correctly — the finding was lost only because a
same-path sibling shadowed it.

The `/member/user` pair was picked over this repository's `/member/address`
pair because it carries the mutating endpoint, which is what makes the lost
finding demonstrable rather than merely a missing row.

**Note on language**: this project's source comments and annotation values are
in Chinese. The files are vendored unmodified, as ADR 0005 requires and as
every other fixture here is. `CLAUDE.md`'s English-only rule governs content
this project authors; it is not applied to third-party source under
`testdata/`, which would have to be altered to comply and would stop being the
real code the fixture exists to test against.
