package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chebilax/sphynxor/internal/extract/nestjs"
	"github.com/chebilax/sphynxor/internal/lint"
	"github.com/chebilax/sphynxor/internal/model"
	"github.com/chebilax/sphynxor/internal/report"
)

func newLintCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "lint [path]",
		Short: "Analyze a NestJS project's authorization model and report findings",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			return runLint(cmd, dir, report.Format(format))
		},
	}

	cmd.Flags().StringVar(&format, "format", string(report.FormatMarkdown), "output format: markdown or json")

	return cmd
}

// runLint wires extraction, rule evaluation, and reporting together, then
// decides the process exit code.
//
// Extraction and rule evaluation are not yet implemented (see
// internal/extract/nestjs and internal/lint); this wiring exists so the
// command's shape — flags, argument handling, output plumbing — is
// reviewable on its own, ahead of that logic landing.
func runLint(cmd *cobra.Command, dir string, format report.Format) error {
	m, err := nestjs.Extract(dir)
	if err != nil {
		return fmt.Errorf("extracting model: %w", err)
	}

	findings := lint.Run(m, lint.DefaultRules())

	if err := report.Write(cmd.OutOrStdout(), m, findings, format); err != nil {
		return fmt.Errorf("writing report: %w", err)
	}

	if hasBlockingFindings(findings) {
		return errBlockingFindings
	}

	return nil
}

// errBlockingFindings is returned by runLint when the run should fail CI —
// vision.md requires a non-zero exit code "on high-confidence findings"
// from the first functional version onward.
var errBlockingFindings = fmt.Errorf("sphinxor: blocking findings reported")

// hasBlockingFindings reports whether any non-allowlisted, high-confidence
// finding exists — the CI-gating boundary from
// docs/decisions/0004-confidence-level-granularity.md.
func hasBlockingFindings(findings []model.Finding) bool {
	for _, f := range findings {
		if f.Confidence == model.ConfidenceHigh && !f.Allowlisted {
			return true
		}
	}
	return false
}
