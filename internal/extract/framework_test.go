package extract

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDetect_RealFixtures is the validation ADR 0019 §1 names: detection
// verified against all four real vendored fixtures, not hand-made
// directories. A detection rule tested only against synthetic trees would
// be exactly the failure docs/testing.md rejects, and least acceptable in
// the component that decides whether the tool looks at anything at all.
func TestDetect_RealFixtures(t *testing.T) {
	cases := []struct {
		dir  string
		want Framework
	}{
		{"nestjs/testdata/nestjs-boilerplate/src", NestJS},
		{"nestjs/testdata/awesome-nest-boilerplate/src", NestJS},
		{"spring/testdata/Pharmacy/backend/src/main/java", Spring},
		{"spring/testdata/blog-api/src/main/java", Spring},
	}

	for _, tc := range cases {
		t.Run(tc.dir, func(t *testing.T) {
			got, err := Detect(tc.dir)
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("Detect(%s) = %v, want exactly [%s]", tc.dir, got, tc.want)
			}
		})
	}
}

// TestDetect_Monorepo covers the ambiguity case ADR 0019 §1 refuses to
// guess at: a directory containing both a Java backend and a Nest BFF.
// Built by placing two real fixtures side by side, so the detected
// signatures are real source, not invented snippets.
func TestDetect_Monorepo(t *testing.T) {
	root := t.TempDir()
	copyTree(t, "spring/testdata/Pharmacy/backend/src/main/java", filepath.Join(root, "backend"))
	copyTree(t, "nestjs/testdata/nestjs-boilerplate/src", filepath.Join(root, "bff"))

	got, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Detect on a mixed monorepo = %v, want both frameworks (the caller must refuse, not pick)", got)
	}
}

// TestDetect_NoFramework is the other refusal case: a directory with
// source files that belong to no supported framework detects nothing, so
// the caller errors instead of running an extractor that will find
// nothing and report a clean run.
func TestDetect_NoFramework(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.py"), []byte("import django\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Detect = %v, want none", got)
	}
}

// TestDetect_SourceFilesWithoutSignature guards the distinction detection
// actually rests on: TypeScript that isn't NestJS must not be detected as
// NestJS merely for being TypeScript. Without this, "has .ts files" would
// silently become the rule, and any TS project would be analyzed as Nest
// and reported clean.
func TestDetect_SourceFilesWithoutSignature(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "util.ts"), []byte("export const add = (a: number, b: number) => a + b;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Detect = %v, want none (TypeScript alone is not NestJS)", got)
	}

	// ...but the files are still counted as parseable source, which is
	// what makes "detected nothing" and "nothing to look at" distinct
	// diagnoses in the CLI (ADR 0019 §2).
	n, err := SourceFileCount(dir, NestJS)
	if err != nil {
		t.Fatalf("SourceFileCount: %v", err)
	}
	if n != 1 {
		t.Errorf("SourceFileCount = %d, want 1", n)
	}
}

func TestSourceFileCount_RealFixtureAndEmpty(t *testing.T) {
	n, err := SourceFileCount("spring/testdata/Pharmacy/backend/src/main/java", Spring)
	if err != nil {
		t.Fatalf("SourceFileCount: %v", err)
	}
	if n == 0 {
		t.Fatal("expected Pharmacy to have Java source files")
	}

	// A real Spring project counted as NestJS has nothing to parse — the
	// case that must become an error rather than an empty clean report.
	n, err = SourceFileCount("spring/testdata/Pharmacy/backend/src/main/java", NestJS)
	if err != nil {
		t.Fatalf("SourceFileCount: %v", err)
	}
	if n != 0 {
		t.Errorf("SourceFileCount(Pharmacy, nestjs) = %d, want 0", n)
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
