// Package cli wires Sphinxor's Cobra command tree.
package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// version is set at release build time via -ldflags (see
// .github/workflows/release.yml), which sets it to the triggering tag
// (e.g. "v0.6.0").
//
// It is deliberately empty by default: empty means "not stamped," which
// resolveVersion answers from the build info rather than reporting a
// placeholder. Release binaries still take the ldflags path.
var version = ""

// resolveVersion reports the version this binary should claim, in
// descending order of authority:
//
//  1. The -ldflags stamp, when there is one — release artifacts.
//  2. The module version Go records in the build info. `go install
//     <pkg>@<version>` records the exact tag — without this, installing a
//     tagged release that way reported "dev", since `go install` never
//     applies this project's ldflags: a wrong answer on a user's very
//     first command. A local `go build` inside the repo lands here too,
//     reporting the VCS pseudo-version (…+dirty on an uncommitted tree),
//     which identifies the exact commit rather than flattening every
//     local build to one label.
//  3. "dev" — when there is no build info to read from either, such as a
//     build from an exported tarball or with -buildvcs=false. Inventing
//     a version there would be worse than saying so.
func resolveVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		// "(devel)" is what Go records for a build that isn't from a
		// resolved module version — no more informative than "dev".
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}

// Execute runs the sphinxor CLI's root command.
func Execute() error {
	return newRootCmd().Execute()
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sphinxor",
		Short:   "Static analysis for your authorization model (RBAC/ABAC/IAM)",
		Version: resolveVersion(),
		Long: "Sphinxor reconstructs, audits, and documents the authorization model\n" +
			"that actually exists in your code, rather than the one declared\n" +
			"elsewhere. See docs/vision.md.",

		// Runtime failures (framework not detected, ambiguous, nothing to
		// analyze — docs/decisions/0019-cli-framework-selection.md) are a
		// designed part of this CLI's behavior, not misuse, so they print
		// the error alone: no usage dump, which buries the actual message,
		// and no duplicate print (cmd/sphinxor writes it to stderr once).
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newLintCmd())
	cmd.AddCommand(newDiffCmd())
	cmd.AddCommand(newExportCmd())
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
			_, err := fmt.Fprintln(cmd.OutOrStdout(), resolveVersion())
			return err
		},
	}
}
