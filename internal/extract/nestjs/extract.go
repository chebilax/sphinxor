// Package nestjs extracts Sphinxor's intermediate model
// (internal/model) from a NestJS project, per
// docs/decisions/0001-target-framework-choice.md.
package nestjs

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/chebilax/sphinxor/internal/model"
)

// AllowlistOutcome is what extraction learned about sphinxor-allow
// markers, per docs/decisions/0003-allowlist-format.md: which endpoints
// they successfully exempt, and which ones didn't match anything.
type AllowlistOutcome struct {
	AllowlistedEndpoints map[model.ID]bool
	StaleMarkers         []model.Finding
}

// Extract walks the NestJS project rooted at dir and builds the
// intermediate model: controllers, endpoints, guard applications, role
// declarations, and role references, plus the allowlist marker outcome.
func Extract(dir string) (*model.Model, AllowlistOutcome, error) {
	files, err := parseProject(dir)
	if err != nil {
		return nil, AllowlistOutcome{}, err
	}

	b := newBuilder()

	// Pass 1: role declarations, project-wide, before resolving any
	// reference to them — a role can be declared in a different file than
	// the one that references it (e.g. roles.enum.ts vs. users.controller.ts).
	// Restricted to enums actually named by a @Roles() call somewhere (see
	// roles.go) so unrelated enums (env config, status flags, ...) aren't
	// mistaken for role registries.
	usedEnumNames := make(map[string]bool)
	for _, f := range files {
		for name := range collectRoleEnumNames(f.tree.RootNode(), f.src) {
			usedEnumNames[name] = true
		}
	}
	for _, f := range files {
		decls := extractRoleDeclarations(f.tree.RootNode(), f.src, f.relPath, b.nextID("role"), usedEnumNames)
		b.model.RoleDeclarations = append(b.model.RoleDeclarations, decls...)
	}
	roleByName := make(map[string]model.ID, len(b.model.RoleDeclarations))
	for _, d := range b.model.RoleDeclarations {
		roleByName[d.Name] = d.ID
	}

	// Pass 2: controllers, endpoints, guards, role references, and
	// allowlist marker matching — matching is file-scoped (a marker only
	// ever exempts an endpoint in the same file), so it happens per file
	// alongside extraction rather than as a separate project-wide pass.
	outcome := AllowlistOutcome{AllowlistedEndpoints: make(map[model.ID]bool)}
	for _, f := range files {
		fileAnchors := extractControllers(f.tree.RootNode(), f.src, f.relPath, b, roleByName)

		allowlisted, stale := matchFileAllowlist(f.src, f.relPath, fileAnchors, b.nextID("finding"))
		for _, id := range allowlisted {
			outcome.AllowlistedEndpoints[id] = true
		}
		outcome.StaleMarkers = append(outcome.StaleMarkers, stale...)
	}

	return &b.model, outcome, nil
}

// builder accumulates model entities and assigns them unique,
// human-readable IDs as extraction proceeds.
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

// nextID is bound to a fixed prefix so it can be passed around as a plain
// func() model.ID where a specific entity kind's IDs are being generated.
func (b *builder) nextID(prefix string) func() model.ID {
	return func() model.ID { return b.nextIDFor(prefix) }
}

type parsedFile struct {
	relPath string
	src     []byte
	tree    *sitter.Tree
}

// parseProject parses every relevant .ts file under dir once, so both
// extraction passes can reuse the same trees without re-parsing.
func parseProject(dir string) ([]parsedFile, error) {
	var files []parsedFile

	parser := sitter.NewParser()
	parser.SetLanguage(typescript.GetLanguage())

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", "dist", ".git", "coverage":
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
	if !strings.HasSuffix(path, ".ts") {
		return false
	}
	base := filepath.Base(path)
	for _, suffix := range []string{".d.ts", ".spec.ts", ".test.ts", ".e2e-spec.ts"} {
		if strings.HasSuffix(base, suffix) {
			return false
		}
	}
	return true
}
