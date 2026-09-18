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

	findings := lint.Run(m, lint.DefaultRules(), allow.AllowlistedEndpoints)
	findings = append(findings, allow.StaleMarkers...)

	return m, findings, nil
}
