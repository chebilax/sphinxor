package cli

import (
	"fmt"
	"io"

	"github.com/chebilax/sphinxor/internal/extract"
	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
)

// resolveFramework decides which extractor to run against dir, per
// docs/decisions/0019-cli-framework-selection.md §1.
//
// The rule that makes auto-detection safe is that it never resolves its
// own uncertainty: exactly one detected framework is used, and both "none"
// and "more than one" are hard errors asking for --framework.
// Auto-detection isn't the danger; auto-detection that quietly picks is.
//
// The returned string says how the choice was made, so the run can state
// it in its output — a silently wrong analysis becomes a visibly wrong one.
func resolveFramework(dir, override string) (extract.Framework, string, error) {
	if override != "" {
		f, err := extract.ParseFramework(override)
		if err != nil {
			return "", "", err
		}
		return f, "--framework", nil
	}

	detected, err := extract.Detect(dir)
	if err != nil {
		return "", "", fmt.Errorf("detecting framework in %s: %w", dir, err)
	}

	switch len(detected) {
	case 1:
		return detected[0], "detected", nil
	case 0:
		return "", "", fmt.Errorf(
			"no supported framework detected in %s (looked for: %s)\n"+
				"If this is a supported project, pass --framework explicitly",
			dir, extract.Join(extract.All))
	default:
		return "", "", fmt.Errorf(
			"more than one framework detected in %s: %s\n"+
				"Pass --framework to say which one to analyze",
			dir, extract.Join(detected))
	}
}

// analyzeDirectory resolves the framework for dir, extracts the model, and
// runs the full rule set against it, including allowlist matching and
// stale-marker findings — the shared pipeline behind `sphinxor lint`,
// `sphinxor diff`, and `sphinxor export`.
//
// It also enforces ADR 0019 §2's distinction between "couldn't look" and
// "looked, found nothing": extracting zero source files is an error, not
// an empty report, because an empty report reads as a clean bill of health
// for a project the tool never actually examined.
//
// Notices (which framework was chosen; parsed files but recognized no
// endpoints) go to w, which callers point at stderr so they never
// contaminate a --format json report on stdout.
func analyzeDirectory(w io.Writer, dir, override string) (*model.Model, []model.Finding, error) {
	framework, how, err := resolveFramework(dir, override)
	if err != nil {
		return nil, nil, err
	}

	sourceFiles, err := extract.SourceFileCount(dir, framework)
	if err != nil {
		return nil, nil, fmt.Errorf("scanning %s: %w", dir, err)
	}
	if sourceFiles == 0 {
		return nil, nil, fmt.Errorf(
			"no %s source files found under %s — nothing was analyzed.\n"+
				"Check the path, or pass --framework if the framework was misidentified",
			framework, dir)
	}

	fmt.Fprintf(w, "Analyzing %s as %s (%s), %d source file(s).\n", dir, framework, how, sourceFiles)

	m, allow, err := extract.Run(dir, framework)
	if err != nil {
		return nil, nil, fmt.Errorf("extracting model at %s: %w", dir, err)
	}

	// Parsed real files but recognized no routes. Unlike the zero-files
	// case this is not an error: a library package or a monorepo subpath
	// legitimately has no controllers, and failing those runs is the
	// noise that gets a CI tool switched off. It is still said out loud,
	// because the other cause is extraction not recognizing this
	// project's route shapes — which an empty matrix alone would present
	// as a clean run.
	if len(m.Endpoints) == 0 {
		fmt.Fprintf(w,
			"warning: parsed %d %s source file(s) but recognized no endpoints.\n"+
				"         If this project does define routes, their shape may not be recognized —\n"+
				"         see docs/limitations.md. This is not treated as a failure.\n",
			sourceFiles, framework)
	}

	// Project-level caveats (ADR 0020 §2/§4). These don't change a finding
	// or a grant; they change what the reader should believe the output
	// means. They go to w (stderr) for the same reason the framework
	// notice does: stdout stays clean for --format json.
	for _, warning := range projectWarnings(m) {
		fmt.Fprintf(w, "warning: %s\n", warning)
	}

	findings := lint.Run(m, lint.DefaultRules(), allow.AllowlistedEndpoints)
	findings = append(findings, allow.StaleMarkers...)

	return m, findings, nil
}

// projectWarnings returns the project-level caveats that change how the
// whole report should be read, per docs/decisions/0020-unanalyzable-is-unknown-not-absent.md
// §2 and §4.
//
// None of these changes a finding or a grant. They exist because a result
// can be correct as far as it goes and still be presented with more
// confidence than the analysis earned — and an audit tool that does that
// is the failure this project is built to avoid.
func projectWarnings(m *model.Model) []string {
	var out []string

	// §2: a URL layer exists and could not be read, so the method layer
	// is not the whole story for any endpoint in this project.
	if m.URLLayer.Unknown() {
		out = append(out, "this project's URL-layer authorization could not be analyzed ("+m.URLLayer.Reason+").\n"+
			"         Roles shown below come from the method layer alone and may be BROADER than what the\n"+
			"         application actually enforces. `sphinxor export cerbos` omits these endpoints entirely.")
	}

	// §4: method-security annotations that may never be switched on. The
	// analysis deliberately does not downgrade them (ADR 0015: absence of
	// the enabling annotation is no evidence either way, since it can live
	// in a parent module or unparsed Kotlin) — but silence here reads as
	// confirmation, and the consequence if they really are inert is that
	// every role shown for them is imaginary.
	if !m.MethodSecurity.Found && hasMethodSecurityAnnotations(m) {
		out = append(out, "method-security annotations were found, but no @EnableMethodSecurity /\n"+
			"         @EnableGlobalMethodSecurity was located in the analyzed source. If it isn't enabled\n"+
			"         elsewhere (a parent module, Kotlin config), those annotations are inert at runtime and\n"+
			"         the endpoints they appear to protect are NOT protected.")
	}

	// §4: the inverted default. Verified against nestjs/nest's own
	// 19-auth-jwt sample, where every endpoint shows no guard because the
	// guard is registered globally and @Public() opts out.
	if m.GlobalGuards.Registered {
		out = append(out, "a global guard is registered via "+m.GlobalGuards.Mechanism+", which protects every route\n"+
			"         by default. Endpoint-level results below UNDERSTATE protection: an endpoint showing no\n"+
			"         guard may still be protected globally.")
	}

	return out
}

// springMethodSecurityAnnotations are exactly the annotations that
// @EnableMethodSecurity switches on — and so exactly the ones the §4
// caveat is about.
var springMethodSecurityAnnotations = map[string]bool{
	"PreAuthorize": true,
	"Secured":      true,
	"RolesAllowed": true,
}

// hasMethodSecurityAnnotations reports whether the project declares roles
// through one of those annotations.
//
// The check is by annotation name rather than by layer on purpose.
// MethodSecurity.Found is false for any framework without the concept
// (model.Model documents that zero value as "unknown", never "disabled"),
// so a layer-shaped check fires this Spring-specific caveat — naming an
// annotation that does not exist there — on every NestJS project that
// uses @Roles(). A caveat that is both wrong and unmissable on half the
// supported frameworks trains people to ignore the ones that are right.
func hasMethodSecurityAnnotations(m *model.Model) bool {
	for _, g := range m.GuardApplications {
		if g.DeclaresRoles && springMethodSecurityAnnotations[g.GuardName] {
			return true
		}
	}
	return false
}
