// Package spring extracts Sphinxor's intermediate model (internal/model)
// from a Spring project, per docs/decisions/0011-spring-second-framework.md
// and docs/decisions/0012-securityfilterchain-effective-policy.md.
//
// This first cut covers structural discovery only: controllers and
// endpoints, mirroring how internal/extract/nestjs itself was built up in
// stages. Guard/role extraction (@PreAuthorize/@Secured/@RolesAllowed,
// bounded SpEL recognition, the @EnableMethodSecurity check) and
// SecurityFilterChain parsing are separate, later passes — each verified
// independently against the same vendored fixtures (testdata/) before
// building on top of this one, not assumed correct by association.
package spring

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/java"

	"github.com/chebilax/sphinxor/internal/model"
)

// Extract walks the Spring project rooted at dir and builds the
// intermediate model's structural facts: controllers and endpoints. Guard
// applications, role declarations/references, and authentication
// requirements are all empty in this cut's output — populated by later
// passes, not yet wired in.
func Extract(dir string) (*model.Model, error) {
	files, err := parseProject(dir)
	if err != nil {
		return nil, err
	}

	b := newBuilder()
	for _, f := range files {
		extractControllers(f.tree.RootNode(), f.src, f.relPath, b)
	}

	return &b.model, nil
}

// builder accumulates model entities and assigns them unique,
// human-readable IDs as extraction proceeds — identical in shape to
// internal/extract/nestjs's builder, kept separate rather than shared
// because the two extractors have no other dependency on each other
// (docs/decisions/0009-cerbos-exporter.md's boundary discipline, applied
// one layer up: extraction packages don't depend on each other either).
type builder struct {
	model    model.Model
	counters map[string]int
}

func newBuilder() *builder {
	return &builder{counters: make(map[string]int)}
}

func (b *builder) nextIDFor(prefix string) model.ID {
	b.counters[prefix]++
	return model.ID(fmt.Sprintf("%s-%d", prefix, b.counters[prefix]))
}

type parsedFile struct {
	relPath string
	src     []byte
	tree    *sitter.Tree
}

// parseProject parses every relevant .java file under dir once.
//
// Skips Maven/Gradle build output directories (target, build, out) and
// version control metadata, the same category of exclusion
// internal/extract/nestjs applies for node_modules/dist/coverage. Also
// skips any directory literally named "test" — the standard Maven/Gradle
// source-root convention (src/test/java, as opposed to src/main/java) — and
// files ending in the common JUnit naming suffixes, mirroring
// internal/extract/nestjs's .spec.ts/.test.ts exclusion. Neither vendored
// fixture currently has a test source root, so this isn't exercised by the
// real-fixture tests yet, but it's the same standard convention, not a
// guess.
func parseProject(dir string) ([]parsedFile, error) {
	var files []parsedFile

	parser := sitter.NewParser()
	parser.SetLanguage(java.GetLanguage())

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "target", "build", "out", ".git", "test":
				return filepath.SkipDir
			}
			return nil
		}
		if !isExtractableSourceFile(path) {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		tree, err := parser.ParseCtx(context.Background(), nil, src)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			rel = path
		}

		files = append(files, parsedFile{relPath: rel, src: src, tree: tree})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func isExtractableSourceFile(path string) bool {
	if !strings.HasSuffix(path, ".java") {
		return false
	}
	base := strings.TrimSuffix(filepath.Base(path), ".java")
	for _, suffix := range []string{"Test", "Tests", "IT"} {
		if strings.HasSuffix(base, suffix) {
			return false
		}
	}
	return true
}
