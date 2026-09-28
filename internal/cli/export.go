package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	var framework string
	var permissionCallees, roleCallees, superuserPermissions, superuserRoles []string
	var noSuperuserEscape bool

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
			decl, err := buildDeclarations(permissionCallees, roleCallees, superuserPermissions, superuserRoles, noSuperuserEscape)
			if err != nil {
				return err
			}
			return runExportCerbos(cmd, dir, out, framework, report.Format(format), decl)
		},
	}

	cmd.Flags().StringVar(&out, "out", "cerbos-policies", "output directory for generated Cerbos policy files")
	cmd.Flags().StringVar(&format, "format", string(report.FormatMarkdown), "companion report format: markdown or json")
	cmd.Flags().StringVar(&framework, "framework", "", "force a framework (nestjs, spring) instead of detecting it")

	// ADR 0041: permissions are exported only through semantics the owner
	// declares. None of these is needed for an export without permissions.
	cmd.Flags().StringArrayVar(&permissionCallees, "permission-callee", nil,
		"declare that this @PreAuthorize bean call names a permission the principal must hold, e.g. @ss.hasPermi "+
			"(append :any-of or :all-of for a call naming several); repeatable")
	cmd.Flags().StringArrayVar(&roleCallees, "role-callee", nil,
		"declare that this bean call names a role the principal must hold, e.g. @ss.hasRole; repeatable")
	cmd.Flags().StringArrayVar(&superuserPermissions, "superuser-permission", nil,
		"a permission that passes every declared check, e.g. *:*:*; repeatable")
	cmd.Flags().StringArrayVar(&superuserRoles, "superuser-role", nil,
		"a role that passes every declared check, e.g. admin; repeatable")
	cmd.Flags().BoolVar(&noSuperuserEscape, "no-superuser-escape", false,
		"state that the declared beans admit no superuser beyond what each call names")

	return cmd
}

// buildDeclarations turns the ADR 0041 flags into Declarations and applies
// its refusal. With none of them it returns the zero value, which never
// fails validation — an export without permissions is never refused.
func buildDeclarations(permissionCallees, roleCallees, superuserPermissions, superuserRoles []string, noEscape bool) (cerbos.Declarations, error) {
	d := cerbos.Declarations{
		SuperuserPermissions: superuserPermissions,
		SuperuserRoles:       superuserRoles,
		NoSuperuserEscape:    noEscape,
	}
	for _, v := range permissionCallees {
		callee, combine, err := cerbos.ParseCallee(v)
		if err != nil {
			return cerbos.Declarations{}, err
		}
		if d.PermissionCallees == nil {
			d.PermissionCallees = map[string]cerbos.Combine{}
		}
		d.PermissionCallees[callee] = combine
	}
	for _, v := range roleCallees {
		callee, combine, err := cerbos.ParseCallee(v)
		if err != nil {
			return cerbos.Declarations{}, err
		}
		if d.RoleCallees == nil {
			d.RoleCallees = map[string]cerbos.Combine{}
		}
		d.RoleCallees[callee] = combine
	}
	return d, d.Validate()
}

// runExportCerbos wires extraction (findings are not used — Translate
// reads only GuardApplication/RoleReference, per ADR 0009 §3's decision
// that a Finding documents Sphinxor's own uncertainty, not a fact to
// translate) to the Cerbos translator, then writes both the policy files
// and the companion report to disk.
func runExportCerbos(cmd *cobra.Command, dir, out, framework string, format report.Format, decl cerbos.Declarations) error {
	m, _, err := analyzeDirectory(cmd.ErrOrStderr(), dir, framework)
	if err != nil {
		return err
	}

	result := cerbos.TranslateWith(m, decl)

	written, err := cerbos.WritePolicies(out, result)
	if err != nil {
		return fmt.Errorf("writing policies: %w", err)
	}

	// A sibling of --out, not nested inside it: cerbos compile scans an
	// entire directory for .yaml/.yml/.json policy candidates and errors
	// on anything that doesn't parse as one -- confirmed empirically
	// (export-report.json inside the policy directory made a real
	// `cerbos compile` fail with "unknown field \"Rules\""). Nesting the
	// report only "looked right" until it was actually compiled.
	reportPath := strings.TrimSuffix(out, string(filepath.Separator)) + ".report." + reportExtension(format)
	f, err := os.Create(reportPath)
	if err != nil {
		return fmt.Errorf("creating export report: %w", err)
	}
	defer f.Close()
	if err := report.WriteExport(f, result, format); err != nil {
		return fmt.Errorf("writing export report: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Wrote %d policy file(s) to %s, and the export report to %s\n", len(written), out, reportPath)
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
