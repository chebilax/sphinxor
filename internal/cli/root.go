// Package cli wires Sphinxor's Cobra command tree.
package cli

import "github.com/spf13/cobra"

// Execute runs the sphinxor CLI's root command.
func Execute() error {
	return newRootCmd().Execute()
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sphinxor",
		Short: "Static analysis for your authorization model (RBAC/ABAC/IAM)",
		Long: "Sphinxor reconstructs, audits, and documents the authorization model\n" +
			"that actually exists in your code, rather than the one declared\n" +
			"elsewhere. See docs/vision.md.",
	}

	cmd.AddCommand(newLintCmd())

	return cmd
}
