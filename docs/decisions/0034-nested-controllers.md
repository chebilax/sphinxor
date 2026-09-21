# 0034. A nested `@RestController` is resolved, not announced

## Status

Accepted.

## Context

`extractControllers` walked top-level `class_declaration` nodes only, so a
`@RestController` declared inside another class contributed nothing. It is the
last item on [ADR 0029](0029-spring-security-scope.md) §3's list.

It is worse than an unread route shape. The class yields no endpoints **and no
controller**, so [ADR 0032](0032-controllers-that-yield-no-routes.md)'s detection
cannot see it either — that condition needs a recognized controller to report on.
The class is invisible rather than incomplete, which is the failure
[ADR 0020](0020-unanalyzable-is-unknown-not-absent.md) exists to remove.

### Why resolving rather than announcing

The other two items on this list were announced because reading them is a
substantial piece of work — an interface lookup, a builder-chain walk. This one is
a missing recursion. Announcing it would mean writing a detector for a shape that
costs one function to read properly, and leaving the routes missing.

An earlier argument for resolving was that top-level test controllers are already
extracted, so nested ones should be too. **That argument does not hold** and is
recorded here because it was made: `parseProject` already skips directories named
`test` and files ending `Test`/`Tests`/`IT`, so top-level controllers in those
paths are *not* extracted. The consistency it appealed to does not exist.

The argument that does hold is the measurement below.

### The measurement

All **33** nested controller declarations in the 20-repository corpus fall in
paths `parseProject` already skips:

| Skipped by | Count |
|---|---|
| a directory named `test` **and** a `*Test.java` filename | 32 |
| the directory rule alone | 1 |
| **reaching the parser** | **0** |

So this is a **zero-delta change against the corpus**, and that is the argument
for it rather than against: it cannot regress anything measured, and it exists for
production code, where Java permits the shape and nothing warned about it.

It also settles a sequencing question. The open "should test sources be analyzed
at all" question (`docs/limitations.md`) does not block this and is not blocked by
it — the corpus's nested controllers are invisible either way.

## Decision

**`extractControllers` walks class declarations at any depth.**

One recursion replaces one loop over `root`'s direct children. Everything
downstream is unchanged: a nested class carrying `@RestController` becomes a
controller, its mapping methods become endpoints, and its guards attach exactly as
a top-level class's would.

**A local class inside a method body is reached too, and deliberately not
excluded.** Spring cannot component-scan one, so it is not a controller in
practice — but no real project writes a `@RestController` in a method body, and a
special case for a hypothetical costs more than it saves. If one ever appears it
produces an endpoint that does not exist, which is the over-report direction; it
is written down here rather than discovered.

## Alternatives considered

- **Announce it, like the other two items.** Rejected: the detection is harder
  than the fix. There is no recognized controller to hang a warning on, so a
  detector would need its own tree walk — the same walk that resolves it — and
  would then report a gap instead of closing it.
- **Resolve only `static` nested classes**, which is what Spring can actually
  instantiate as a bean. Rejected as unmeasurable precision: the corpus has no
  production instance of either kind, so the distinction would be guesswork
  dressed as rigour, and an inner-class controller that somehow is registered
  would go missing again.
- **Settle the test-sources question first.** Rejected per the measurement: the
  two are independent, and blocking a zero-risk change on a larger open question
  would leave the last silent item open for no gain.

## Consequences

- `internal/extract/spring/controllers.go`: one helper,
  `controllerCandidateClasses`, replacing the top-level loop.
- **Regression bar: zero delta.** `sphinxor lint` is byte-identical across all 20
  repositories, which is the predicted result of the measurement above and the
  whole reason this is safe.
- No model change, no new warning, no new project-level fact.
- [ADR 0029](0029-spring-security-scope.md) §2 moves the nested `@RestController`
  from `silent` to **read** — the last item on the list.
- The open question **"should test sources be analyzed at all"** stays open in
  `docs/limitations.md`, unchanged by this. `parseProject`'s skip rules remain a
  partial, undeclared policy rather than a decision, and they are the reason this
  change measures zero.
