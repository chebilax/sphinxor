# 0042. Kotlin source is detected and announced, not passed over

## Status

**Proposed.** It applies [ADR 0020](0020-unanalyzable-is-unknown-not-absent.md) §2's
principle ("unanalyzable is unknown, not absent") to a boundary
[ADR 0011](0011-spring-second-framework.md) §1 and [ADR 0029](0029-spring-security-scope.md)
§2 draw: Kotlin source is out of scope. It does not bring Kotlin into scope. Parsing
Kotlin stays out, and nothing here reads a Kotlin declaration's meaning.

## Context

ADR 0029's definition of done excludes Kotlin. That exclusion is safe only if a project
written partly or wholly in Kotlin is **told** it was not analyzed. Measured on
2026-09-28 at `eec508e`.

### A mixed project is passed over silently

A constructed project with:
- `src/main/java/app/JavaController.java`: `DELETE /java/items/{id}`, `@PreAuthorize("hasRole('ADMIN')")`;
- `src/main/kotlin/app/KotlinController.kt`: `DELETE /kotlin/items/{id}`, no annotation;
- `src/main/kotlin/app/SecurityConfig.kt`: `@EnableMethodSecurity` and a `SecurityFilterChain`.

`sphinxor lint` output:
- `Analyzing … as spring (detected), 1 source file(s).` The two `.kt` files are neither
  counted nor mentioned.
- The matrix has **one** endpoint. `DELETE /kotlin/items/{id}` is absent, and nothing
  says a controller was skipped.
- The Kotlin `SecurityFilterChain` is not seen, so the URL layer is recorded as
  **absent**. That is the exact error ADR 0020 §2 exists to prevent: an unreadable
  layer taken for no layer. The export then grants from the method layer alone. With
  the Kotlin chain requiring `hasRole("SUPERADMIN")` on `/java/**`, `export cerbos`
  exported `DELETE` on `java` to `ADMIN`: a grant the application denies. The same rule
  written in Java, laid out the same way, is read, and the endpoint is correctly omitted
  (disjoint roles). So the over-grant is Kotlin's alone.

  (The first draft demonstrated this with `denyAll()`. That over-grants even in Java,
  because of a separate defect reported alongside this ADR, so it did not isolate Kotlin.
  Recorded rather than quietly replaced.)
- The only mention of Kotlin is incidental and **wrong in effect**: the method-security
  caveat reports that no `@EnableMethodSecurity` was located, so the annotations "are
  NOT protected". The enabler is present, in Kotlin.

### Wholly Kotlin projects get loud but misattributed output

In ADR 0038's sample:

| Project | Kotlin files | Kotlin controllers | Java files | Output |
|---|---:|---:|---:|---|
| gameyfin | 277 | 3 | 0 | "no supported framework detected" |
| encore | 75 | 2 | 0 | "no supported framework detected" |
| DuDoong-Backend | 845 | 27 | 11 | "recognized no endpoints … their shape may not be recognized" |

None says the cause is Kotlin. **The 20-repository corpus contains no Kotlin file**,
which is why this never surfaced in a corpus measurement.

## Decision (proposed)

### §1 Kotlin source files are counted, never parsed

The Spring extractor counts `.kt` files under the analyzed tree, with the same exclusions
as `.java` (test directories and test file names). Their content is searched as **text**
for a fixed set of annotation and type names. Nothing is parsed, and nothing found in a
Kotlin file becomes an endpoint, a guard or a role. ADR 0029 §1 draws the line this way:
"detecting that another system is present is not interpreting it".

### §2 One project warning, naming what the Kotlin files hold

When any Kotlin file is present, the run warns once:

- **always:** `N Kotlin source file(s) were found and not analyzed (Kotlin is not parsed, ADR 0011 §1)`;
- **plus, when any contains `@RestController` or `@Controller`:** `K of them declare
  controllers; their routes are absent from the matrix, along with any access control on
  them`;
- **plus, when any contains Spring Security configuration** (a `SecurityFilterChain` or
  `SecurityWebFilterChain` bean, `@EnableWebSecurity`, `@EnableMethodSecurity`,
  `@EnableGlobalMethodSecurity`, `@EnableReactiveMethodSecurity`, or a
  `WebSecurityConfigurerAdapter`): that it is named, and §3 and §4 apply.

### §3 Kotlin security configuration makes the URL layer unknown

A `SecurityFilterChain`, `SecurityWebFilterChain` or `WebSecurityConfigurerAdapter` found
in Kotlin makes `URLLayerStatus` present and unanalyzed, with the reason *"URL
authorization is configured in Kotlin, which is not parsed"*. From there ADR 0020 §2
applies unchanged: the lint warning says roles may be broader, and `export cerbos` omits
every endpoint instead of granting from the method layer alone. This closes the
over-grant in Context.

### §4 A Kotlin method-security enabler changes the caveat, not the flags

A Kotlin `@EnableMethodSecurity` (or one of its siblings) is not read, so
`MethodSecurityStatus.Found` stays `false`, and ADR 0015's inert-guard logic is
unchanged. The ADR 0020 §4 caveat stops saying the annotations "are NOT protected". It
says instead that an enabling annotation was found in Kotlin and not read, so whether
they are enabled is unknown. That is the truthful version of the same uncertainty.

### §5 A wholly Kotlin Spring project says so

When framework detection finds no supported framework but the tree holds Kotlin files
that mention Spring's web or security annotations, the error names Kotlin as the reason
instead of suggesting `--framework`. When Spring is detected with zero Java endpoints and
Kotlin controllers are present (DuDoong), §2's warning makes the cause explicit. The
existing "recognized no endpoints" line stays.

## Alternatives considered

- **Parse Kotlin.** Out of scope by ADR 0011 §1 and ADR 0029 §2, and a new-language
  effort, not an announcement. This ADR is what makes deferring it safe.
- **A warning with the count only.** Rejected: it would leave the URL layer recorded as
  absent, and the export's over-grant with it. §3 is the part that changes a result, not
  only a message.
- **Treat any Kotlin file as making the whole run unknown.** Rejected: a project with
  Kotlin tests or build scripts, and its security in Java, would lose a correct analysis.
  Only Kotlin *security configuration* makes the URL layer unknown. Kotlin controllers
  are announced, since their routes are simply absent.

## Consequences

- `internal/extract/spring`: a Kotlin scan beside `parseProject`, reusing its file
  exclusions; text search only.
- `internal/model`: a `KotlinStatus` (file count, controller-file count, security-file
  count). `URLLayerStatus` gains the Kotlin reason.
- `internal/cli/analyze.go`: §2's warning, §4's caveat wording, §5's message.
- `internal/export/cerbos`: unchanged code; it sees an unknown URL layer.
- ADR 0029 §2: a row for Kotlin source, *detected and announced*.
- Regression bar:
  - the 20 corpus repositories are byte-identical, since none contains Kotlin;
  - the constructed mixed project warns, and its export omits the Java endpoint;
  - the three Kotlin sample applications name Kotlin as the cause;
  - fixture: the constructed project, synthetic by necessity. The corpus has no Kotlin,
    and the sample's Kotlin projects are AGPL-3.0 (gameyfin, DuDoong) or EUPL-1.2
    (encore), which ADR 0005's note on copyleft shapes puts outside vendoring.
