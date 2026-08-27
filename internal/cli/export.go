package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/chebilax/sphinxor/internal/export/cerbos"
	"github.com/chebilax/sphinxor/internal/report"
)

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export the authorization model to a real authorization engine's policy format",
	}

	cmd.AddCommand(newExportCerbosCmd())

	return cmd
}

func newExportCerbosCmd() *cobra.Command {
	var out string
	var format string

	cmd := &cobra.Command{
		Use:   "cerbos [path]",
		Short: "Export to Cerbos resource policies",
		Long: "Translate the authorization model at path into a Cerbos resource policy set.\n" +
			"The output is explicitly not deploy-ready — review it, and the companion\n" +
			"export report, before deploying anywhere. See\n" +
			"docs/decisions/0009-cerbos-exporter.md for the safety posture this follows:\n" +
			"omit and flag whenever the model can't establish a grant with certainty,\n" +
			"never guess.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			return runExportCerbos(cmd, dir, out, report.Format(format))
		},
	}

	cmd.Flags().StringVar(&out, "out", "cerbos-policies", "output directory for generated Cerbos policy files")
	cmd.Flags().StringVar(&format, "format", string(report.FormatMarkdown), "companion report format: markdown or json")

	return cmd
}

// runExportCerbos wires extraction (findings are not used — Translate
// reads only GuardApplication/RoleReference, per ADR 0009 §3's decision
// that a Finding documents Sphinxor's own uncertainty, not a fact to
// translate) to the Cerbos translator, then writes both the policy files
// and the companion report to disk.
func runExportCerbos(cmd *cobra.Command, dir, out string, format report.Format) error {
	m, _, err := analyzeDirectory(dir)
	if err != nil {
		return err
	}

	result := cerbos.Translate(m)

	written, err := cerbos.WritePolicies(out, result)
	if err != nil {
		return fmt.Errorf("writing policies: %w", err)
	}

	reportPath := filepath.Join(out, "export-report."+reportExtension(format))
	f, err := os.Create(reportPath)
	if err != nil {
		return fmt.Errorf("creating export report: %w", err)
	}
	defer f.Close()
	if err := report.WriteExport(f, result, format); err != nil {
		return fmt.Errorf("writing export report: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Wrote %d policy file(s) and %s to %s\n", len(written), filepath.Base(reportPath), out)
	fmt.Fprintf(cmd.OutOrStdout(), "%d rule(s) exported, %d omission(s), %d unverified role reference(s) — review before deploying.\n",
		len(result.Rules), len(result.Omissions), len(result.UnverifiedRoles))

	return nil
}

func reportExtension(format report.Format) string {
	if format == report.FormatJSON {
		return "json"
	}
	return "md"
}
