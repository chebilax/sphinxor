package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chebilax/sphinxor/internal/diff"
	"github.com/chebilax/sphinxor/internal/report"
)

func newDiffCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "diff <base-dir> <head-dir>",
		Short: "Compare two versions of a NestJS project's authorization model",
		Long: "Compare the authorization model at base-dir against head-dir — typically\n" +
			"the same project checked out at two points in git history (e.g. a\n" +
			"reference branch and the current PR). See docs/decisions/0007-model-diff-design.md.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiff(cmd, args[0], args[1], report.Format(format))
		},
	}

	cmd.Flags().StringVar(&format, "format", string(report.FormatMarkdown), "output format: markdown or json")

	return cmd
}

// runDiff analyzes both directories independently — in-process, no
// intermediate serialized report — and compares the results, per ADR
// 0007 §1 and its note that this avoids any boundary where allowlist
// status could be dropped before the de-allowlisting regression case
// can match against it.
func runDiff(cmd *cobra.Command, baseDir, headDir string, format report.Format) error {
	baseModel, baseFindings, err := analyzeDirectory(baseDir)
	if err != nil {
		return err
	}
	headModel, headFindings, err := analyzeDirectory(headDir)
	if err != nil {
		return err
	}

	result := diff.Compare(
		diff.Snapshot{Model: baseModel, Findings: baseFindings},
		diff.Snapshot{Model: headModel, Findings: headFindings},
	)

	if err := report.WriteDiff(cmd.OutOrStdout(), result, format); err != nil {
		return fmt.Errorf("writing diff report: %w", err)
	}

	if result.HasRegressions() {
		return errRegressionsFound
	}

	return nil
}

// errRegressionsFound is returned by runDiff when the run should fail
// CI — vision.md requires failing "a PR when a regression is detected in
// the model relative to the reference branch."
var errRegressionsFound = fmt.Errorf("sphinxor: regressions found relative to base")
