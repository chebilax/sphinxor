# 0047. Three states per endpoint: protected, unprotected, or needs manual review

## Status

**Proposed** (2026-09-30). Measured before being written, and not implemented. It
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
- **Scope:** corpus-20 at its pins, the 12 corpus-40 projects drawn so far (ADR 0046),
  and the 8 Spring fixtures, at `6669a6d`. That is 7,932 endpoints.
- **Findings** are those of `lint.Run` with no allowlist.

An endpoint is classified as follows, in this order:
- **Known protected:** a guard was read on either layer (roles, permissions,
  `authenticated()`, `denyAll()`), and it is not confirmed inert (ADR 0015).
- **Unknown:** anything that could protect it was left unread. The reasons measured:

  | Reason | What it means |
  |---|---|
  | `url-layer-unknown` | The project's URL layer exists and could not be read: several chains, reactive, a legacy adapter, Shiro, Kotlin. |
  | `url-rule-unresolved` | ADR 0018 Amendment 1: a URL rule may narrow access and was not read. |
  | `url-rule-may-not-apply` | The URL rules that may apply cannot narrow access, but do not settle whether authentication is required. wvp's `requestMatchers(CorsUtils::isPreFlightRequest).permitAll()` followed by `anyRequest().authenticated()` is the case. |
  | `unrecognized-annotation` | ADR 0022 and ADR 0023. |
  | `opaque-requirement` | A guard whose requirement could not be read, such as a bean call naming no literal (ADR 0020 Amendment 3). |
  | `project-annotation` | An annotation declared in the project, on the handler or its class, and read by an `@Aspect` `@annotation(...)` pointcut or by a `HandlerInterceptor` calling `get…Annotation(X.class)`. Detected by structure, never by name. |

- **Known unprotected:** neither of the above.

**Totals**, 7,932 endpoints:

| State | Endpoints |
|---|---:|
| Known protected | 904, of which 772 also carry an unread element, such as a URL layer that is unknown |
| **Unknown** | **6,406** |
| Known unprotected | 622 |

Unknown, by the first reason that applies: `url-layer-unknown` 5,358,
`url-rule-unresolved` 503, `url-rule-may-not-apply` 447, `project-annotation` 47,
`opaque-requirement` 45, `unrecognized-annotation` 6.

**Current findings:** 2,545 `mutating-endpoint-without-access-control`, all Low, plus 50
`permission-declared-but-unreferenced`, which this ADR does not touch.
- **2,228 move** to the review list: `url-layer-unknown` 1,778, `url-rule-unresolved` 279,
  `url-rule-may-not-apply` 143, `project-annotation` 28.
- **317 stay** as findings.

Per repository (endpoints; protected / unknown / unprotected; mutating findings that
move / stay):

| Repository | Endpoints | P / U / N | Move / stay |
|---|---:|---|---|
| JeecgBoot | 931 | 0 / 931 / 0 | 294 / 0 |
| RuoYi-Vue | 148 | 117 / 31 / 0 | 15 / 0 |
| apollo | 224 | 0 / 224 / 0 | 50 / 0 |
| conductor | 189 | 0 / 0 / 189 | 0 / 99 |
| dataease | 16 | 0 / 1 / 15 | 0 / 5 |
| dolphinscheduler | 239 | 0 / 239 / 0 | 108 / 0 |
| eladmin | 133 | 89 / 44 / 0 | 20 / 0 |
| fineract | 1 | 0 / 1 / 0 | 0 / 0 |
| halo | 0 | — | — |
| hertzbeat | 175 | 0 / 0 / 175 | 0 / 91 |
| inlong | 339 | 0 / 339 / 0 | 189 / 0 |
| litemall | 219 | 0 / 219 / 0 | 45 / 0 |
| metersphere | 1,053 | 0 / 1,053 / 0 | 84 / 0 |
| microcks | 97 | 0 / 97 / 0 | 40 / 0 |
| nacos | 434 | 0 / 434 / 0 | 7 / 0 |
| nakadi | 51 | 0 / 51 / 0 | 26 / 0 |
| shenyu | 397 | 0 / 397 / 0 | 148 / 0 |
| spring-cloud-dataflow | 113 | 0 / 113 / 0 | 56 / 0 |
| streampark | 232 | 0 / 232 / 0 | 126 / 0 |
| thingsboard | 551 | 518 / 33 / 0 | 15 / 0 |
| VBlog | 28 | 0 / 28 / 0 | 21 / 0 |
| vhr | 51 | 0 / 51 / 0 | 27 / 0 |
| Stirling-PDF | 404 | 147 / 77 / 180 | 23 / 88 |
| wvp-GB28181-pro | 395 | 0 / 395 / 0 | 115 / 0 |
| arthas | 22 | 0 / 22 / 0 | 14 / 0 |
| agentscope-java | 214 | 0 / 214 / 0 | 119 / 0 |
| poseidon | 34 | 0 / 34 / 0 | 12 / 0 |
| zeus-iot | 206 | 0 / 206 / 0 | 187 / 0 |
| opsli-boot | 22 | 4 / 18 / 0 | 13 / 0 |
| mateclaw | 559 | 0 / 515 / 44 | 282 / 25 |
| alf.io | 394 | 0 / 394 / 0 | 189 / 0 |
| X-Road | 19 | 13 / 6 / 0 | 2 / 0 |
| fixtures (8) | 42 | 16 / 7 / 19 | 1 / 9 |

**What the table shows:**
- **The unknown state dominates.** In 23 of the 32 repositories more than half the
  endpoints are unknown, and in 21 all are. The URL layer is unread in 23 repositories,
  and that alone makes every endpoint there not known to be unprotected.
- **Most of today's findings are unknowns.** 88% of the mutating findings sit on
  endpoints whose protection was left unread. That is the owner's concern, measured.
- **317 findings stay, in five repositories and the fixtures:**
  - conductor has no authentication in its open-source build.
  - Stirling-PDF has 180 endpoints with nothing on the method under a fully read chain.
  - mateclaw's 44 are `permitAll()` paths.
  - dataease has 5.
  - hertzbeat's 91 are the next section.

### A signal the three states still miss

hertzbeat protects its API with **Sureness**, a third-party security framework. It
enforces through a servlet filter (`SurenessJakartaServletFilter`), not through Spring
Security. Sphinxor sees no URL layer and no annotation, so all 175 endpoints read as
known unprotected, and 91 findings stay. Those are false positives, with a signal present:
a security framework's filter on the classpath. The same shape covers Sa-Token and
similar frameworks.

A third-party framework's filter can be identified by package, the way
[ADR 0023](0023-third-party-authorization-annotations.md) identifies Shiro's annotations:
by import, never by a name heuristic. The endpoints then become unknown, with reason
`third-party-filter`. **Proposed as part of this ADR, and measured on one repository
only.** The frameworks to list need their own survey.

### Structure detects enforcement, not authorization

Project-defined annotations were detected by structure alone: declared in the project and
read by an aspect or an interceptor. 29 such annotations sit on handlers in 13
repositories. How each was classified:
- **By reading the reader:** mateclaw's three and zeus-iot's `@Permission`, each
  confirmed to stop the request, and Stirling-PDF's `@EnterpriseEndpoint`.
- **From `docs/limitations.md`:** metersphere's and streampark's, already recorded there.
- **By the reader class's name and role:** the rest. They are to be confirmed when this
  is implemented.

| Kind | Annotations (endpoints carrying them) |
|---|---|
| **Authorization** (user, tenant or signature) | metersphere `@CheckOwner` 528, `@CheckOrgOwner` 11, `@CheckProjectOwner` 11; mateclaw `@RequireWorkspaceRole` 345, `@RequireGlobalAdmin` 73, `@RequireKbScope` 13; streampark `@Permission` 78, `@OpenAPI` 2; zeus-iot `@Permission` 67; JeecgBoot `@SignatureCheck` 6; dataease `@DeLinkPermit` 1 |
| **Licence or entitlement gate** | Stirling-PDF `@RequiresFeature` 35, `@EnterpriseEndpoint` 19, `@PremiumEndpoint` 6 |
| **Not access control** | logging: metersphere `@Log` 210, JeecgBoot `@AutoLog` 79, dolphinscheduler `@OperatorLog` 73, eladmin `@Log` 63, zeus-iot `@BussinessLog` 27, shenyu `@Log` 5; audit: X-Road `@AuditEventMethod` 7, Stirling `@Audited` 6; others: metersphere `@SendNotice` 37, opsli `@Limiter` 14, metersphere `@FileLimit` 12, streampark `@AppChangeEvent` 8, apollo `@PreAcquireNamespaceLock` 4, JeecgBoot `@PermissionData` 2 (row filtering), opsli `@SearchHis` 1 |

By endpoint count, almost a third of the structural signal is not access control: 548 of
1,743 annotation placements (31%). 1,135 are authorization and 60 licence gates. Structure alone would move an endpoint carrying only
`@Log` into the review list. That hides a real finding, which breaks the goal from the
other side.

Checking whether the reader file can deny (`throw`, `return false`, `sendError`) does
not separate them either: `@AutoLog`'s and `@BussinessLog`'s readers contain a `throw`
too. **The criterion proposed:** the reader can stop the request on the path that reads
the annotation.
- **An interceptor:** its `preHandle` returns `false`, or it sends an error, after
  reading it.
- **An aspect:** its advice throws, or does not call `proceed()`, on a branch that
  depends on it.

That is still structure, not names. Its precision is to be measured when implemented; it
is not measured here.

## Decision (proposed)

### §1 Three states

Every endpoint is **known protected**, **known unprotected**, or **unknown (needs manual
review)**, with the definitions above. Known protected wins: a guard that was read
protects the endpoint whatever else is unread.

The effective policy may still be incomplete, as with a readable method guard under an
unread URL layer. The export already omits those endpoints (ADR 0020 §2, ADR 0018
Amendment 1). They are not added to the review list. See *Open questions*, 1.

### §2 The review list

Unknown endpoints go into a per-endpoint **needs manual review** list, each with every
reason that applies. The list is in the text report and in the JSON. Unknown raises no
finding.

**This replaces ADR 0018 Amendment 1's interim row.** For an endpoint whose URL rule is
unread, `mutating-endpoint-without-access-control` no longer fires; the endpoint is in
the review list instead.

**The trade-off.** The unread rule may be a `permitAll()`, so the endpoint may truly be
public. Keeping the finding would say so, but it would also say "no detected guard" of
5,358 endpoints in projects whose URL layer Sphinxor never read. It is answered by making
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

1. **Should protected endpoints with an incomplete policy be listed too?** That is a
   read method guard under an unread URL layer: 772 endpoints, almost all RuoYi-Vue,
   thingsboard and eladmin. They are protected, and the export already omits them. Listing
   them would say "the roles shown may not be the whole requirement", which the matrix
   and the warnings already say per project.
2. **The size of the list.** In metersphere every one of 1,053 endpoints needs review,
   for one reason. Proposed: the text report groups by reason and prints the first
   entries of large groups, while the JSON lists everything.
3. **Can `sphinxor-allow` acknowledge a review?** It would take an endpoint off the list
   once a person has checked it, which keeps the list meaningful across runs. It would
   also let a list be silenced. Proposed: yes, and acknowledged entries stay in the JSON
   with `reviewed: true`.
4. **Third-party filters (hertzbeat).** Accept `third-party-filter` as an unknown reason,
   identified by package, with the framework list surveyed separately.

## Alternatives considered

- **Keep the finding and add "the URL layer was not read" to its message.** This is the
  interim behaviour. It still counts 2,228 endpoints as findings whose premise is
  unknown, and a CI dashboard reads them as problems found, not as work to do.
- **Detect project annotations by name** (`Permission`, `Auth`, `Check…`). Rejected by
  ADR 0023: dataease's `@DePermit` has no reader at all.
- **Detect them by structure alone.** Measured above: about a third of placements are
  logging, audit or rate limits, and they would hide real findings.

## Consequences (if accepted)

- The measurement harness becomes a test on fixtures, one per reason.
- `docs/limitations.md` gains §5's limit.
- ADR 0018 Amendment 1's `mutating-endpoint-without-access-control` row is superseded.
- A changelog entry under *Changed*: findings move to the review list, and exit codes
  are unchanged.
