# 0047. Three states per endpoint: protected, unprotected, or needs manual review

## Status

**Proposed** (2026-09-30), figures finalized 2026-10-02 at `5b64842`. Measured before being written, and not implemented. It
replaces one row of [ADR 0018](0018-unrecognized-rule-stops-evaluation.md) Amendment 1's
interim table: `mutating-endpoint-without-access-control` on an endpoint whose URL rule
is unread.

## Context

### The owner's goal

When Sphinxor cannot reconstruct an endpoint's authorization, it says so and names the
endpoint, so the user can check it. It reports no false positive where it has a signal
that something protects the endpoint. And it never produces a result that reads as "all
clear" while unknowns remain.

Today an endpoint is in one of two states for lint: guarded, or not. "Not guarded" mixes
two different facts:
- nothing protects the endpoint, and every layer that could was read;
- something that could protect it was left unread.

The second produces a Low `mutating-endpoint-without-access-control` finding whose
premise, "no detected guard", is true only of what was read. Where the tool cannot read a
layer, a run with no finding can still read as clean.

### Measured: the states each endpoint would be in today

Measured with a throwaway harness on the extractor's own model:
- **Scope:** corpus-20 at its pins and the 12 corpus-40 projects drawn so far (ADR
  0046), 7,890 endpoints, plus the 8 Spring fixtures, 42 endpoints.
- **Binary:** `main` at `5b64842`, after #77 (several chain beans), #80 (project
  enforcement) and #82 (constant matcher arrays). This finalizes the figures of the
  first draft, which were taken at `6669a6d`.
- **Findings** are those of `lint.Run` with no allowlist.

An endpoint is classified as follows, in this order:
- **Known protected:** a guard or authentication requirement was read on either layer,
  and it is not confirmed inert (ADR 0015).
- **Unknown:** anything that could protect it was left unread. The reasons:

  | Reason | What it means |
  |---|---|
  | `url-layer-unknown` | The project's URL layer exists and could not be read: several chains, reactive, a legacy adapter, Shiro, Kotlin. |
  | `url-rule-unresolved` | ADR 0018 Amendment 1: a URL rule may narrow access and was not read. |
  | `url-rule-may-not-apply` | The URL rules that may apply cannot narrow access, but do not settle whether authentication is required. wvp's `requestMatchers(CorsUtils::isPreFlightRequest).permitAll()` followed by `anyRequest().authenticated()` is the case. |
  | `unrecognized-annotation` | ADR 0022 and ADR 0023. |
  | `opaque-requirement` | A guard whose requirement could not be read (ADR 0020 Amendment 3). |
  | `project-annotation` | A `ProjectEnforcement` (ADR 0023 Amendment 1): an annotation declared in the project whose aspect or interceptor can stop the request. |

- **Known unprotected:** neither of the above.

**Totals**, 32 repositories:

| State | All 32 repositories | Spring Security projects (21) | Not Spring Security (11) |
|---|---:|---:|---:|
| Known protected | 1,386 | 1,386 (38%) | 0 |
| — of which protected by authentication alone | 532 | 532 | 0 |
| **Unknown** | **6,078** | **2,233 (61%)** | **3,845** |
| Known unprotected | 426 | 47 (1%) | 379 |
| Endpoints | 7,890 | 3,666 | 4,224 |

Which projects use Spring Security for their endpoints is recorded in
[`coverage.md`](../coverage.md).

Unknown, by the first reason that applies: `url-layer-unknown` 5,615,
`url-rule-may-not-apply` 447, `project-annotation` 15, `url-rule-unresolved` 1.

**The 2,535 current `mutating-endpoint-without-access-control` findings, all Low:**

| Where they would go | Findings |
|---|---:|
| **Review list:** the endpoint is unknown (`url-layer-unknown` 1,889, `url-rule-may-not-apply` 143, `project-annotation` 5, `url-rule-unresolved` 1) | 2,038 |
| **On endpoints protected by authentication alone:** today's rule counts only guards, not authentication (§1) | 277 |
| **Stay as findings:** known unprotected | 220 |

Per repository (endpoints; protected / unknown / unprotected; protected by authentication
alone; mutating findings that move to review / stay):

| Repository | Endpoints | P / U / N | Authentication alone | Move / stay |
|---|---:|---|---:|---|
| metersphere | 1,053 | 0 / 1,053 / 0 | 0 | 84 / 0 |
| JeecgBoot | 931 | 0 / 931 / 0 | 0 | 294 / 0 |
| mateclaw | 559 | 498 / 14 / 47 | 498 | 5 / 25 |
| thingsboard | 551 | 518 / 33 / 0 | 0 | 15 / 0 |
| nacos | 434 | 0 / 434 / 0 | 0 | 7 / 0 |
| Stirling-PDF | 404 | 147 / 257 / 0 | 33 | 111 / 0 |
| shenyu | 397 | 0 / 397 / 0 | 0 | 148 / 0 |
| wvp-GB28181-pro | 395 | 0 / 395 / 0 | 0 | 115 / 0 |
| alf.io | 394 | 0 / 394 / 0 | 0 | 189 / 0 |
| inlong | 339 | 0 / 339 / 0 | 0 | 189 / 0 |
| dolphinscheduler | 239 | 0 / 239 / 0 | 0 | 108 / 0 |
| streampark | 232 | 0 / 232 / 0 | 0 | 126 / 0 |
| apollo | 224 | 0 / 224 / 0 | 0 | 50 / 0 |
| litemall | 219 | 0 / 219 / 0 | 0 | 45 / 0 |
| agentscope-java | 214 | 0 / 214 / 0 | 0 | 119 / 0 |
| zeus-iot | 206 | 0 / 206 / 0 | 0 | 187 / 0 |
| conductor | 189 | 0 / 0 / 189 | 0 | 0 / 99 |
| hertzbeat | 175 | 0 / 0 / 175 | 0 | 0 / 91 |
| RuoYi-Vue | 148 | 117 / 31 / 0 | 0 | 15 / 0 |
| eladmin | 133 | 89 / 44 / 0 | 0 | 20 / 0 |
| spring-cloud-dataflow | 113 | 0 / 113 / 0 | 0 | 56 / 0 |
| microcks | 97 | 0 / 97 / 0 | 0 | 40 / 0 |
| nakadi | 51 | 0 / 51 / 0 | 0 | 26 / 0 |
| vhr | 51 | 0 / 51 / 0 | 0 | 27 / 0 |
| poseidon | 34 | 0 / 34 / 0 | 0 | 12 / 0 |
| VBlog | 28 | 0 / 28 / 0 | 0 | 21 / 0 |
| arthas | 22 | 0 / 22 / 0 | 0 | 14 / 0 |
| opsli-boot | 22 | 4 / 18 / 0 | 0 | 13 / 0 |
| X-Road | 19 | 13 / 6 / 0 | 1 | 2 / 0 |
| dataease | 16 | 0 / 1 / 15 | 0 | 0 / 5 |
| fineract | 1 | 0 / 1 / 0 | 0 | 0 / 0 |
| halo | 0 | — | — | — |
| fixtures (8) | 42 | 16 / 7 / 19 | 0 | 1 / 9 |

**What the table shows:**
- **Most unknowns are outside Spring Security.** 63% of them (3,845 of 6,078) are in
  projects whose authorization is not Spring Security at all, and Sphinxor is not built to
  read those (`coverage.md`).
- **On Spring Security projects, 61% is unknown.** Almost all of it is a URL layer or a
  URL rule that is not read.
- **The findings that stay** are conductor (99, no authentication in its open-source
  build), hertzbeat (91, the next section), mateclaw's `permitAll()` paths (25),
  dataease (5) and the fixtures (9).

### A signal the three states still miss

hertzbeat protects its API with **Sureness**, a third-party security framework. It
enforces through a servlet filter (`SurenessJakartaServletFilter`), not through Spring
Security. Sphinxor sees no URL layer and no annotation, so all 175 endpoints read as
known unprotected, and 91 findings stay. Those are false positives, with a signal present:
a security framework's filter on the classpath. Sa-Token and similar frameworks have the
same shape.

A third-party framework's filter can be identified by package, the way
[ADR 0023](0023-third-party-authorization-annotations.md) identifies Shiro's annotations:
by import, never by a name heuristic. The endpoints then become unknown, with reason
`third-party-filter`. **This is proposed, and measured on one repository only.** The
frameworks to list need their own survey.

### Which project annotations count, for findings

ADR 0023 Amendment 1 already detects project-enforced annotations for the export, with the
criterion "the reader can stop the request". Across the corpus and fixtures it classifies
36 annotation–reader pairs:
- **34 are enforcing.** They include every reader that enforces access, and also loggers,
  audit, rate limits and locks that can throw or skip `proceed()`, such as `@AutoLog`,
  eladmin's `@Log` and `@OperationLog`.
- **2 are non-enforcing:** RuoYi-Vue's and shenyu's `@Log`, both confirmed by reading.

**For findings, the same criterion would hide real findings wherever a logger can throw.**
An endpoint carrying only such a logger would move to the review list. **On today's corpus
the effect is nil.** Every logger classified enforcing sits in a project whose URL layer
is already unknown, so its endpoints are unknown for that reason first. The only endpoints
whose state the `project-annotation` reason decides are:
- mateclaw's 14, all `@RequireKbScope` or `@RequireWorkspaceRole`;
- dataease's 1, `@DeLinkPermit`.

All of them are authorization.

**Proposed:** findings use the same criterion for now. This is to be revisited when a
corpus project shows a logger deciding an endpoint's state, with the measurement as the
trigger, as ADR 0038 Stage 2 works.

## Decision (proposed)

### §1 Three states

Every endpoint is **known protected**, **known unprotected**, or **unknown (needs manual
review)**, with the definitions above. Known protected wins: a requirement that was read
protects the endpoint whatever else is unread.

**Protected by authentication alone** (an `authenticated()` rule, a role-less
`AuthGuard`, `isAuthenticated()`) is known protected for findings, so
`mutating-endpoint-without-access-control` stops firing on it. Today's rule counts only
guards, so it fires on 277 such endpoints, almost all mateclaw's. These endpoints are
protected, but what authorization they have beyond authentication is unknown, so they go
on the review list with reason `authentication only`. That is the same line
[ADR 0048](0048-authentication-only-grants-opt-in.md) draws for the export, where they
stop being granted by default.

A protected endpoint whose policy is otherwise incomplete, such as a read method guard
under an unread URL layer, is not listed. The export already omits it (ADR 0020 §2, ADR
0018 Amendment 1). See *Open questions*, 1.

### §2 The review list

Unknown endpoints go into a per-endpoint **needs manual review** list, each with every
reason that applies. The list is in the text report and in the JSON. Unknown raises no
finding.

**This replaces ADR 0018 Amendment 1's interim row.** For an endpoint whose URL rule is
unread, `mutating-endpoint-without-access-control` no longer fires; the endpoint is in
the review list instead.

**The trade-off.** The unread rule may be a `permitAll()`, so the endpoint may truly be
public. Keeping the finding would say so, but it would also say "no detected guard" of
5,615 endpoints in projects whose URL layer Sphinxor never read. It is answered by making
the review list as visible as findings: in the summary line, in the JSON, and never
collapsed away. An endpoint that needs review is not an endpoint that passed.

### §3 The summary never reads clean while unknowns remain

- **Text:** `0 findings, 148 endpoints need manual review`.
- **JSON:** a `review` count beside the finding counts.
- **Exit codes are unchanged:** unknown does not fail CI, and High findings still do. A
  run is clean only when both counts are zero.

### §4 Consumers

| Consumer | Behaviour |
|---|---|
| **Matrix, Markdown** | A state per row, rendered in the Guards cell: `?` or `URL ?` as today, extended to every unknown reason. The review list follows the findings section. |
| **Matrix, JSON** | Each row gains `state` (`protected`, `unprotected`, `unknown`) and `reviewReasons`. A top-level `review` array lists the unknown endpoints. Both are new fields; nothing existing changes shape. |
| **Lint summary and exit code** | As §3. |
| **`mutating-endpoint-without-access-control`** | Fires on known-unprotected endpoints only. Its message keeps "no access control detected" and never says "unprotected" (§5). |
| **`sphinxor diff` and the became-public gate** | Protected or unknown in base, known unprotected in head, gates CI. That is ADR 0036 as it stands, since ADR 0036 already treats unrecognized protection as not confirmed public, now covering every unknown reason. Protected to unknown is reported as `became-unknown`, informational, and does not gate. |
| **Cerbos export report** | Unchanged in what it grants: unknown endpoints are already omitted, under `url-layer-unknown` or `url-rule-unresolved`. The report gains a pointer to the review list, so the two lists agree. |
| **Project-level warnings** | Those that give only a count gain the per-endpoint list, or point to it: `N endpoint(s) carry <unrecognized annotation>`, `the requirement could not be read for N endpoint(s)`, `the URL rule that may govern N endpoint(s)…`, and `URL-layer authorization could not be analyzed`, which gives no count today. Warnings about routes that were never extracted (unrecovered controllers, Kotlin, functional routing, GraphQL) have no endpoint to list. They stay project-level, and the summary counts them, so it does not read clean either. |

### §5 A limit, stated

Zero false positives is not fully reachable. Some checks leave no signal at all:
- a check written directly in handler code (`if (!user.isAdmin()) throw …`) under a
  fully read URL layer;
- a servlet filter the tool cannot identify.

The tool cannot classify those endpoints as unknown. For them, the finding says "no
access control detected", never "unprotected", and `sphinxor-allow` remains the remedy.
This goes into `docs/limitations.md` with this ADR.

## Open questions for the owner

1. **Should protected endpoints with an incomplete policy be listed too?** That is a read
   requirement under an unread URL layer: 1,305 endpoints, mostly thingsboard, mateclaw's
   annotated endpoints, Stirling-PDF and eladmin. They are protected, and the export
   already omits them. Listing them would say "the requirement shown may not be all of
   it", which the matrix and the warnings already say per project.
2. **The size of the list.** In metersphere every one of 1,053 endpoints needs review,
   for one reason. Proposed: the text report groups by reason and prints the first
   entries of large groups, while the JSON lists everything.
3. **Can `sphinxor-allow` acknowledge a review?** It would take an endpoint off the list
   once a person has checked it, which keeps the list meaningful across runs. It would
   also let a list be silenced. Proposed: yes, and acknowledged entries stay in the JSON
   with `reviewed: true`.
4. **Third-party filters (hertzbeat).** Accept `third-party-filter` as an unknown reason,
   identified by package, with the framework list surveyed separately.
5. **Authentication only.** §1 lists these endpoints with reason `authentication only`,
   consistent with ADR 0048. If ADR 0048 is not accepted, they could stay off the list as
   plain known protected.

## Alternatives considered

- **Keep the finding and add "the URL layer was not read" to its message.** This is the
  interim behaviour. It still counts 2,038 endpoints as findings whose premise is
  unknown, and a CI dashboard reads them as problems found, not as work to do.
- **Detect project annotations by name** (`Permission`, `Auth`, `Check…`). Rejected by
  ADR 0023: dataease's `@DePermit` has no reader at all.
- **Detect them by structure alone.** Rejected for the export by ADR 0023 Amendment 1, measured: it removed 59 of RuoYi-Vue's 100 declared rules, because its logging aspect reads `@Log`. For findings it would be worse, since it would hide real findings behind logging annotations.

## Consequences (if accepted)

- The measurement harness becomes a test on fixtures, one per reason.
- `docs/limitations.md` gains §5's limit.
- ADR 0018 Amendment 1's `mutating-endpoint-without-access-control` row is superseded.
- A changelog entry under *Changed*: findings move to the review list, and exit codes
  are unchanged.
