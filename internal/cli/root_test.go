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
