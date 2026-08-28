package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestExportCerbos_JSONReportDoesNotCollideWithPolicyDir is a regression
// test for a real bug: the export report was originally written inside
// --out, alongside the generated policy files. cerbos compile scans an
// entire directory for .yaml/.yml/.json policy candidates and errors on
// anything that doesn't parse as one — with --format json, the report
// itself became exactly such a file, and a real `cerbos compile` against
// --out failed with "unknown field \"Rules\"" on export-report.json. This
// runs the actual `sphinxor export cerbos` command (not just the
// internal/export/cerbos library) end to end, since the bug lived in the
// CLI's file layout, not the translation logic.
func TestExportCerbos_JSONReportDoesNotCollideWithPolicyDir(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "policies")

	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{
		"export", "cerbos",
		"../extract/nestjs/testdata/nestjs-boilerplate/src",
		"--out", outDir,
		"--format", "json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("reading %s: %v", outDir, err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".yaml" {
			t.Errorf("--out contains a non-policy file %q — the report must be written as a sibling, not nested inside the policy directory", e.Name())
		}
	}

	cerbosPath, err := exec.LookPath("cerbos")
	if err != nil {
		t.Skip("cerbos CLI not found on PATH — skipping real-engine validation")
	}
	compileOut, err := exec.Command(cerbosPath, "compile", "--skip-tests", outDir).CombinedOutput()
	if err != nil {
		t.Fatalf("cerbos compile %s failed:\n%s", outDir, compileOut)
	}
}
