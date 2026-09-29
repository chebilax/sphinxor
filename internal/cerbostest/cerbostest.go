// Package cerbostest locates the Cerbos CLI for the real-engine tests of
// docs/decisions/0009-cerbos-exporter.md §5.
//
// Without the CLI those tests skip, so a contributor without it still gets
// everything else `go test ./...` covers. Where the CLI is installed on
// purpose — CI sets SPHINXOR_REQUIRE_CERBOS=1 — a skip would let the
// real-engine validation disappear from a green run, so it fails instead.
package cerbostest

import (
	"os"
	"os/exec"
	"testing"
)

// RequireEnv names the variable that turns a missing CLI from a skip into a
// failure.
const RequireEnv = "SPHINXOR_REQUIRE_CERBOS"

// Binary returns the cerbos CLI's path, or skips t — or fails it when
// RequireEnv is "1". It logs the path, so a verbose run shows each
// real-engine test actually executing.
func Binary(t testing.TB) string {
	t.Helper()
	path, err := exec.LookPath("cerbos")
	if err != nil {
		if os.Getenv(RequireEnv) == "1" {
			t.Fatalf("cerbos CLI not found on PATH, and %s=1: real-engine validation (ADR 0009 §5) must run here", RequireEnv)
		}
		t.Skip("cerbos CLI not found on PATH — skipping real-engine validation (see ADR 0009 §5)")
	}
	t.Logf("real-engine validation with %s", path)
	return path
}
