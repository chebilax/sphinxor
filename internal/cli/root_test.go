package cli

import (
	"bytes"
	"testing"
)

func TestVersionCmd_PrintsVersion(t *testing.T) {
	old := version
	version = "v9.9.9-test"
	defer func() { version = old }()

	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := out.String(), "v9.9.9-test\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestVersionFlag_MatchesVersionCommand(t *testing.T) {
	old := version
	version = "v9.9.9-test"
	defer func() { version = old }()

	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := out.String(); got != "sphinxor version v9.9.9-test\n" {
		t.Errorf("--version output = %q, want it to report v9.9.9-test", got)
	}
}

// TestResolveVersion_FallsBackWhenUnstamped covers the `go install` case:
// with no -ldflags stamp, the version must come from the build info
// rather than a placeholder — reporting "dev" for a tagged release a
// user just installed is wrong on their first command.
//
// Under `go test` the build info has no resolved module version, so this
// asserts the end of the ladder (never empty, never "(devel)") rather
// than a specific tag; the tag case is verified by actually running
// `go install <pkg>@<version>`, which no unit test can reproduce.
func TestResolveVersion_FallsBackWhenUnstamped(t *testing.T) {
	old := version
	version = ""
	defer func() { version = old }()

	got := resolveVersion()
	if got == "" || got == "(devel)" {
		t.Fatalf("resolveVersion() = %q, want a usable version string", got)
	}
}

// TestResolveVersion_LdflagsStampWins keeps the release path primary:
// when a stamp exists it is used verbatim, never overridden by build
// info.
func TestResolveVersion_LdflagsStampWins(t *testing.T) {
	old := version
	version = "v1.2.3"
	defer func() { version = old }()

	if got := resolveVersion(); got != "v1.2.3" {
		t.Errorf("resolveVersion() = %q, want the ldflags stamp v1.2.3", got)
	}
}
