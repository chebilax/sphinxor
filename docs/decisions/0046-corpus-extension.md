# 0046. Extending the measurement corpus without biasing it toward what is being looked for

## Status

**Accepted** (2026-09-29), with the owner's correction to §1's exclusions and two
additions (§2's pool record, §6). It defines how the Spring measurement corpus grows. It amends no code
decision, and changes no figure already published: every existing figure was measured on
the 20-repository corpus and stays attributed to it.

## Context

### Three decisions wait on evidence the corpus does not supply

| Deferred item | Its trigger | Hits in the corpus today |
|---|---|---:|
| [ADR 0045](0045-legacy-url-layer.md) — the pre-6.0 `authorizeRequests()` reader | a project whose legacy URL layer meets §1: one unconditional site, one fluent expression, no branch, with role rules | 0 |
| [ADR 0038](0038-role-hierarchy-read.md) Stage 2 — expanding implied roles | rows with roles next to a readable hierarchy that reaches their layer | 0 (no hierarchy at all) |
| [ADR 0040](0040-multiple-security-filter-chains.md) §4 — chain selection | several chains, none conditional, readable matchers, determined order, rules outside branches | 0 |

The nine-application hierarchy sample hit two of these triggers repeatedly. Seven of the
nine use a legacy adapter, and autoplan alone would meet both ADR 0045's and Stage 2's
triggers. But that sample was **found by searching for `RoleHierarchyImpl`**: it contains
hierarchies by construction, and says nothing about how common they are.

### How the current corpus was chosen, and what it is

**Its selection criteria were never recorded.** [ADR 0035](0035-permissions-in-the-model.md)
§9 pins the commits, and nothing states why these 20. Measured now, with the extractor and
build files (a Maven POM read with an XML parser, and the four Gradle builds read by hand):

| Property | The 20 repositories |
|---|---|
| **Spring Boot major** | 2.x: 6, from 2.1.5 to 2.7.18; 3.x: 6, from 3.3.1 to 3.5.15; 4.x: 8, from 4.0.3 to 4.1.1 |
| **Stars** | median **13,553**; minimum 966 (nakadi); maximum 48,027 (JeecgBoot) |
| **Application type** (by reading each README) | 13 platforms or infrastructure (apollo, nacos, shenyu, dolphinscheduler, inlong, streampark, hertzbeat, conductor, spring-cloud-dataflow, fineract, thingsboard, metersphere, halo); 5 admin scaffolds (RuoYi-Vue, eladmin, JeecgBoot, litemall, dataease); 2 others (microcks, nakadi) |
| **URL layer, from the extractor** | 5 Shiro, 6 several chains, 2 reactive, 2 unparseable single chains, 1 legacy adapter, 4 analyzed or absent |
| **Archived since pinning** | 2 (nakadi, spring-cloud-dataflow). Both pins still resolve |

**The version spread is not the bias; size and application type are.** The corpus has six
Boot 2.x projects, where `WebSecurityConfigurerAdapter` was still current, yet only one
uses it. Three of those six authorize through Shiro, and two build a
`SecurityFilterChain`. The shapes the deferred items need turned up in the hierarchy
sample, which spans 97 to 1,091 stars and is mostly single-application business software.
The corpus is two orders of magnitude more popular, and mostly multi-module platforms.
Popular platforms get modernized and split into modules; small applications keep the
configuration they were written with.

So the corpus under-represents small, single-application Spring Security code, and not
by accident: selecting the best-known repositories selects exactly the projects that have
moved on.

## Decision (proposed)

### §1 Discovery by scope, never by shape

Candidates are found by what defines Sphinxor's scope, **Spring Security as a
dependency**. That means `spring-boot-starter-security` or `spring-security-config` in a
Maven or Gradle build file of a non-fork Java repository. They are **never** found by a
shape under measurement: not `RoleHierarchyImpl`, not `WebSecurityConfigurerAdapter`, not
`SecurityFilterChain`, not a permission bean. The ADR 0038 sample showed what happens
otherwise: a sample built from the thing being counted contains it, every time.

**Excluded**, with the reason recorded per candidate:
- tutorials, courses, samples, demos, study notes and documentation, by their own README
  or description;
- forks and mirrors;
- Kotlin-only projects: their absence of analysis is announced (ADR 0042), and they
  measure nothing;
- **projects with no web dependency**, meaning neither `spring-boot-starter-web` nor
  `spring-boot-starter-webflux` (nor `spring-webmvc`/`spring-webflux`) in any build
  file: libraries, batch jobs, CLI tools. They have no endpoint to authorize.

**The exclusion is decided on the project, never on the tool's output.** A web project
where the extractor finds zero endpoints is **kept**, and that is a finding. Excluding it
would remove exactly the projects Sphinxor cannot read (`RouterFunction` routing, JAX-RS,
routing shapes not yet known) and bring the bias back through the back door.

*(Correction: the first draft excluded "repositories whose pinned commit yields zero
recognized endpoints". That is circular, and the owner caught it. It is recorded here
rather than quietly corrected.)*

### §2 Stratified, and drawn without a human choosing

Candidates are stratified on the two axes the Context shows the bias lives on, plus the
one the deferred items depend on:

| Axis | Strata | Informs |
|---|---|---|
| **Spring Boot major**, from build files | 2.x, 3.x, 4.x | **ADR 0045**: the legacy adapter is current only on 2.x (removed in Spring Security 6.0, i.e. Boot 3.0). **ADR 0038 Stage 2**: the reach of a hierarchy is version-dependent (§13 there) |
| **Stars** at selection | 50–500, 500–5,000, over 5,000 | **ADR 0038 Stage 2 and ADR 0045**: small applications are where the sample found both shapes. **ADR 0040**: multi-module platforms, where several chains live, sit in the upper bands |
| **Application type**, from the README | business application, platform or infrastructure, admin scaffold | balance only: **at most one project per lineage** (the RuoYi, yudao and JeecgBoot families each count once, in the existing corpus included), and admin scaffolds capped at a third of the extension |

Within each Boot × stars cell (9 cells), candidates are ordered by the SHA-256 of their
`owner/name` and taken from the top. The draw is reproducible from the candidate list, and
nobody chooses a repository because of what it contains. The candidate list, and the
position each selected repository took, are recorded.

**The pool is recorded per cell:**
- how many candidates the search returned;
- how many survived the exclusions;
- **whether GitHub's search result cap was hit**. Code search returns at most 1,000
  results per query.

Where the cap was hit, the hash draw ran inside a pool GitHub's own ranking had already
filtered, and `docs/corpus.md` says so for that cell. The draw is then unbiased only
relative to that ranked pool. Splitting a capped query (by star range or push date) until
each part is under the cap is attempted first. Whatever remains capped is stated, not
hidden.

### §3 Twenty more repositories: two per cell, plus two spares

That makes **18 drawn, 2 per cell, and 2 spares** held for replacements, for a corpus of
40.

- **Why not fewer.** The deferred triggers are rare in the existing corpus (0 of 20
  each). An extension that cannot plausibly contain a hit cannot tell "rare" from
  "absent". With 6 cells covering Boot 2.x and the lower two star bands, the extension
  weights exactly where the sample found the shapes, without searching for them.
- **Why not more.** A measurement run already means cloning and analyzing every
  repository. Measured on 2026-09-29, a full before-and-after comparison of the 20
  repositories (lint JSON and Cerbos export, with two binaries) takes **43 seconds** of
  wall time on a 10-core machine, with already-cloned trees. 40 keeps that at around a
  minute and a half, a routine step of every decision. That is ADR 0005 Amendment 1's
  concern about cost, applied to the corpus rather than the fixtures.

### §4 Pinned in one place, cloned on demand

- **One file, `docs/corpus.md`**, becomes the corpus's single record. It lists all 40
  repositories with:
  - the full commit SHA and selection date;
  - the stratum, the stars at selection and the Boot version with its source (POM parsed,
    or Gradle read by hand);
  - the application type and its lineage.

  The existing 20 move there from ADR 0035 §9 with their pins unchanged. ADRs cite it.
- **The pin** is the default branch's HEAD at the selection date. A pin that stops
  resolving (a deleted repository, a force-push) is recorded as lost and replaced from the
  spares, never silently dropped. Archived repositories stay, while their pins resolve.
- **Clone on demand, never vendored** (ADR 0005 Amendment 1's split): depth 1 at the
  pinned SHA, with a `scripts/corpus-clone.sh` reading `docs/corpus.md`.

### §5 The triggers are written down before the draw

What counts as a hit for each deferred item is fixed by this ADR, before any repository is
cloned:
- **ADR 0045:** a project whose legacy URL layer meets its §1 and contains at least one
  role rule;
- **ADR 0038 Stage 2:** at least one endpoint row with a role, in a project whose
  hierarchy is read (not only detected), on a layer the hierarchy is shown to reach;
- **ADR 0040:** a project meeting §4's four conditions.

After the draw, each is counted with the extractor, per `docs/testing.md`, and reported
per cell. **A hit does not by itself reopen a deferred decision**: it is evidence for the
owner to weigh, the way ADR 0038's trigger was always meant to work.

### §6 What the extended corpus can and cannot show

**Two projects per cell support occurrence, not frequency.** The extended corpus can show
that a shape exists in real applications of a given kind. It cannot show what share of
projects have it. **No figure computed on it is a rate**, and none is to be published as
one: "3 of the 40 use a legacy adapter" describes the corpus, not the Spring ecosystem.
The strata and their fixed quotas make that explicit. A cell's two projects stand for
nothing but themselves.

What it can support is exactly what the deferred items need. Each trigger in §5 asks
whether a qualifying project exists, not how many do.

## Alternatives considered

- **Search for each shape and add what is found.** Rejected: it measures presence, not
  frequency, and every later rate computed on the corpus would inherit the bias. The
  ADR 0038 sample is kept as what it is, a sample of hierarchies.
- **Add the nine sample applications to the corpus.** Rejected for the same reason: they
  were chosen because they contain a hierarchy.
- **Select by popularity again**, the most-starred Spring Security projects. Rejected by
  the Context: it re-selects the projects that have moved past the shapes in question.
- **Uniform random over all candidates, unstratified.** Rejected: GitHub's long tail is
  dominated by tiny and abandoned projects, and a uniform draw would swing the bias the
  other way. Stratifying fixes the mix and still leaves the choice within a cell to the
  hash.
- **A larger extension (50+).** Deferred. If 20 more finds no hit for any trigger, that
  is itself a measured answer, and the next extension can be sized from it.

## Consequences (if accepted)

- The candidate search, the exclusions and the draw are run and recorded in
  `docs/corpus.md` in one PR, with the 20 new pins and each one's stratum.
- A baseline run of the current binary over the 20 new repositories, reported in the same
  PR: endpoints, URL-layer forms, and the §5 trigger counts.
- Existing figures stay attributed to the 20-repository corpus. New figures state which
  corpus they come from ("corpus-20" or "corpus-40") until corpus-40 has been the
  baseline for a release.
- `docs/testing.md` gains one line: corpus membership changes only by an ADR; selection
  never searches for a measured shape; exclusions are decided on the project, never on
  the tool's output; and a figure from the corpus is an occurrence, never a rate.
