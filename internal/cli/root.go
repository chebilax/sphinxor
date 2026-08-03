// Package cli wires Sphinxor's Cobra command tree.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is overridden at release build time via -ldflags (see
// .github/workflows/release.yml), which sets it to the triggering tag
// (e.g. "v0.3.0"). A plain `go build`/`go install` with no ldflags — the
// case for every local dev build — reports "dev": an honest default
// rather than a stale or fabricated version number.
var version = "dev"

// Execute runs the sphinxor CLI's root command.
func Execute() error {
	return newRootCmd().Execute()
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sphinxor",
		Short:   "Static analysis for your authorization model (RBAC/ABAC/IAM)",
		Version: version,
		Long: "Sphinxor reconstructs, audits, and documents the authorization model\n" +
			"that actually exists in your code, rather than the one declared\n" +
			"elsewhere. See docs/vision.md.",
	}

	cmd.AddCommand(newLintCmd())
	cmd.AddCommand(newDiffCmd())
	cmd.AddCommand(newVersionCmd())

	return cmd
}

// newVersionCmd exists alongside the `--version` flag Cobra's Version
// field already provides above: some users reach for `sphinxor version`
// as a subcommand rather than a flag, and both should report the same
// value rather than only one of the two working.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the sphinxor version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), version)
			return err
		},
	}
}
