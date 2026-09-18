package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	pharmacyFixture = "../extract/spring/testdata/Pharmacy"
	nestjsFixture   = "../extract/nestjs/testdata/nestjs-boilerplate"
)

// TestAnalyzeDirectory_DetectsSpringAndFindsEndpoints is the end-to-end
// bar docs/decisions/0019-cli-framework-selection.md names: before this
// wiring, `sphinxor lint` on this exact real Spring project reported
// "0 endpoint(s), 0 finding(s)" and exited 0, because analyzeDirectory
// hardcoded the NestJS extractor. A confident clean bill of health on a
// 21-guard application is the product-level form of the reassuring false
// negative this project exists to refuse.
func TestAnalyzeDirectory_DetectsSpringAndFindsEndpoints(t *testing.T) {
	var notices bytes.Buffer
	m, findings, err := analyzeDirectory(&notices, pharmacyFixture, "")
	if err != nil {
		t.Fatalf("analyzeDirectory: %v", err)
	}

	if len(m.Endpoints) == 0 {
		t.Fatal("expected real Spring endpoints, got none — the regression this wiring closes")
	}
	if !strings.Contains(notices.String(), "as spring (detected)") {
		t.Errorf("expected the notice to state the framework and how it was chosen, got %q", notices.String())
	}

	// The real finding on the deliberately-public login endpoint, which
	// TestLint_Pharmacy pins at the extractor level — confirmed to survive
	// all the way through the CLI pipeline.
	var sawLogin bool
	for _, f := range findings {
		if f.RuleID == "mutating-endpoint-without-access-control" && strings.Contains(string(f.SubjectID), "/auth/login") {
			sawLogin = true
		}
	}
	if !sawLogin {
		t.Errorf("expected the POST /auth/login finding to reach the CLI, got %+v", findings)
	}
}

// TestAnalyzeDirectory_HonorsSpringAllowlistMarker is the second half of
// 0019's end-to-end bar: a marker in Java source must actually suppress,
// through the CLI, not just in the extractor's own tests.
func TestAnalyzeDirectory_HonorsSpringAllowlistMarker(t *testing.T) {
	dir := t.TempDir()
	copyTree(t, pharmacyFixture, dir)

	authController := filepath.Join(dir, "backend/src/main/java/haru/pharmacy/controller/AuthController.java")
	data, err := os.ReadFile(authController)
	if err != nil {
		t.Fatal(err)
	}
	const target = "    @PostMapping(\"/login\")"
	if !strings.Contains(string(data), target) {
		t.Fatalf("fixture shape changed: %q not found", target)
	}
	patched := strings.Replace(string(data), target,
		"    // sphinxor-allow: login is public by design\n"+target, 1)
	if err := os.WriteFile(authController, []byte(patched), 0o644); err != nil {
		t.Fatal(err)
	}

	_, findings, err := analyzeDirectory(&bytes.Buffer{}, dir, "")
	if err != nil {
		t.Fatalf("analyzeDirectory: %v", err)
	}

	var sawLogin bool
	for _, f := range findings {
		if f.RuleID != "mutating-endpoint-without-access-control" || !strings.Contains(string(f.SubjectID), "/auth/login") {
			continue
		}
		sawLogin = true
		if !f.Allowlisted {
			t.Errorf("marker did not suppress the finding: %+v", f)
		}
	}
	if !sawLogin {
		t.Error("expected the finding to still be produced, marked Allowlisted (a marker suppresses, it does not erase)")
	}
}

func TestResolveFramework(t *testing.T) {
	t.Run("detects nestjs", func(t *testing.T) {
		f, how, err := resolveFramework(nestjsFixture, "")
		if err != nil || f != "nestjs" || how != "detected" {
			t.Fatalf("got (%q, %q, %v), want (nestjs, detected, nil)", f, how, err)
		}
	})

	t.Run("override wins and is reported as such", func(t *testing.T) {
		f, how, err := resolveFramework(pharmacyFixture, "nestjs")
		if err != nil || f != "nestjs" || how != "--framework" {
			t.Fatalf("got (%q, %q, %v), want (nestjs, --framework, nil)", f, how, err)
		}
	})

	t.Run("no framework is a hard error, not a default", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, _, err := resolveFramework(dir, "")
		if err == nil {
			t.Fatal("expected an error rather than a guessed framework")
		}
		if !strings.Contains(err.Error(), "--framework") {
			t.Errorf("error should tell the user how to proceed, got %q", err)
		}
	})

	t.Run("ambiguity is a hard error, not a pick", func(t *testing.T) {
		root := t.TempDir()
		copyTree(t, pharmacyFixture, filepath.Join(root, "backend"))
		copyTree(t, nestjsFixture, filepath.Join(root, "bff"))

		_, _, err := resolveFramework(root, "")
		if err == nil {
			t.Fatal("expected an error on a mixed monorepo rather than silently analyzing one half")
		}
		if !strings.Contains(err.Error(), "nestjs") || !strings.Contains(err.Error(), "spring") {
			t.Errorf("error should name what it found, got %q", err)
		}
	})

	t.Run("unknown framework value", func(t *testing.T) {
		if _, _, err := resolveFramework(".", "django"); err == nil {
			t.Fatal("expected an error for an unsupported framework")
		}
	})
}

// TestAnalyzeDirectory_NoSourceFilesIsAnError is ADR 0019 §2's core
// distinction: the tool never looked, so it must say so rather than
// render an empty matrix that reads as a clean run.
func TestAnalyzeDirectory_NoSourceFilesIsAnError(t *testing.T) {
	_, _, err := analyzeDirectory(&bytes.Buffer{}, pharmacyFixture, "nestjs")
	if err == nil {
		t.Fatal("expected an error when the chosen framework has no source files to parse")
	}
	if !strings.Contains(err.Error(), "nothing was analyzed") {
		t.Errorf("error should say plainly that nothing was analyzed, got %q", err)
	}
}

// TestAnalyzeDirectory_ParsedFilesButNoEndpointsWarns is the other side of
// §2: a library subpath legitimately has no routes, so this warns and
// succeeds rather than failing a build — but it does not stay silent,
// because the other cause is extraction not recognizing this project's
// route shapes.
func TestAnalyzeDirectory_ParsedFilesButNoEndpointsWarns(t *testing.T) {
	dir := t.TempDir()
	src := "import { Injectable } from '@nestjs/common';\n\n@Injectable()\nexport class Helper {}\n"
	if err := os.WriteFile(filepath.Join(dir, "helper.service.ts"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var notices bytes.Buffer
	m, _, err := analyzeDirectory(&notices, dir, "")
	if err != nil {
		t.Fatalf("a parseable project with no routes must not be an error: %v", err)
	}
	if len(m.Endpoints) != 0 {
		t.Fatalf("expected no endpoints, got %+v", m.Endpoints)
	}
	if !strings.Contains(notices.String(), "recognized no endpoints") {
		t.Errorf("expected a warning naming the situation, got %q", notices.String())
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copying %s: %v", src, err)
	}
}
