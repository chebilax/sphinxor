// Package spring extracts Sphinxor's intermediate model (internal/model)
// from a Spring project, per docs/decisions/0011-spring-second-framework.md,
// docs/decisions/0012-securityfilterchain-effective-policy.md,
// docs/decisions/0015-inert-method-security-guard.md,
// docs/decisions/0016-spel-role-declaration-heuristic-resolution.md, and
// docs/decisions/0017-declaresroles-excludes-isauthenticated.md.
//
// This cut covers structural discovery (controllers, endpoints) plus
// method-security annotation extraction: @PreAuthorize/@Secured/@RolesAllowed,
// bounded SpEL recognition, Java role declarations, and the
// @EnableMethodSecurity project-wide check. SecurityFilterChain parsing —
// the method×URL effective-policy's URL layer — is a separate, later pass,
// verified independently against the same vendored fixtures (testdata/)
// before building on top of this one, not assumed correct by association.
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
// intermediate model: controllers, endpoints, method-security guard
// applications, role declarations/references, and authentication
// requirements. SecurityFilterChain-derived facts (the URL layer) are not
// part of this cut's output yet.
func Extract(dir string) (*model.Model, error) {
	files, err := parseProject(dir)
	if err != nil {
		return nil, err
	}

	b := newBuilder()

	// Pass 0: role declarations, project-wide, before resolving any
	// reference to them — mirrors internal/extract/nestjs/extract.go's own
	// staging (a declaration can live in a different file than every
	// reference to it), restricted to literals actually referenced
	// somewhere (roles.go's own doc comment) so an unrelated enum/constant
	// in the project isn't mistaken for a role registry.
	usedLiterals := make(map[string]bool)
	for _, f := range files {
		for lit := range collectUsedRoleLiterals(f.tree.RootNode(), f.src) {
			usedLiterals[lit] = true
		}
	}
	for _, f := range files {
		decls := extractRoleDeclarations(f.tree.RootNode(), f.src, f.relPath, b.nextID("role"), usedLiterals)
		b.model.RoleDeclarations = append(b.model.RoleDeclarations, decls...)
	}
	roleByName := uniqueRoleDeclarationsByName(b.model.RoleDeclarations)

	// Pass 1: @EnableMethodSecurity/@EnableGlobalMethodSecurity,
	// project-wide — docs/decisions/0015-inert-method-security-guard.md.
	// Independent of every other pass; order relative to them doesn't
	// matter, but it must complete before any consumer reads
	// b.model.MethodSecurity (none does yet in this package — recorded for
	// internal/lint's use, per that ADR's Consequences).
	for _, f := range files {
		scanMethodSecurityStatus(f.tree.RootNode(), f.src, &b.model.MethodSecurity)
	}

	// Pass 2: controllers, endpoints, and method-security guards.
	for _, f := range files {
		extractControllers(f.tree.RootNode(), f.src, f.relPath, b, roleByName)
	}

	// Pass 3: authentication requirements (ADR 0010) — derived from the
	// fully-assembled RoleReference collection above (an endpoint's
	// authCandidate can only be resolved once every guard contributing to
	// that endpoint, class- or method-level, is known), same ordering
	// reason as internal/extract/nestjs's own final pass.
	b.model.AuthenticationRequirements = computeAuthenticationRequirements(&b.model, b.authCandidates, b.nextID("authreq"))

	return &b.model, nil
}

// uniqueRoleDeclarationsByName maps a role literal to its RoleDeclaration
// ID, but only when exactly one declaration in the project carries that
// name — docs/decisions/0016-spel-role-declaration-heuristic-resolution.md's
// stated boundary: an ambiguous name (more than one declaration) resolves
// to nothing, the same "don't guess when a real answer isn't available"
// default used everywhere else in this project.
func uniqueRoleDeclarationsByName(decls []model.RoleDeclaration) map[string]model.ID {
	byName := make(map[string][]model.ID, len(decls))
	for _, d := range decls {
		byName[d.Name] = append(byName[d.Name], d.ID)
	}
	out := make(map[string]model.ID, len(byName))
	for name, ids := range byName {
		if len(ids) == 1 {
			out[name] = ids[0]
		}
	}
	return out
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
	// seenEndpoints tracks which Endpoint IDs have already been created, so
	// two real handlers that share HTTPMethod+Path (differing only in
	// `produces`, per docs/decisions/0014-endpoint-identity-and-content-negotiation.md)
	// are merged into one Endpoint rather than appended as separate entries
	// sharing one ID. The first handler encountered, in file-then-source
	// order (parseProject walks files in deterministic lexical order),
	// wins as the Endpoint's own HandlerName/File/Line.
	seenEndpoints map[model.ID]bool
	// authCandidates accumulates every @PreAuthorize("isAuthenticated()")
	// occurrence found while applying guards, consumed by the final
	// computeAuthenticationRequirements pass (authentication.go).
	authCandidates []authCandidate
}

func newBuilder() *builder {
	return &builder{counters: make(map[string]int), seenEndpoints: make(map[model.ID]bool)}
}

func (b *builder) nextIDFor(prefix string) model.ID {
	b.counters[prefix]++
	return model.ID(fmt.Sprintf("%s-%d", prefix, b.counters[prefix]))
}

// nextID is bound to a fixed prefix so it can be passed around as a plain
// func() model.ID where a specific entity kind's IDs are being generated —
// mirrors internal/extract/nestjs/extract.go's identical helper.
func (b *builder) nextID(prefix string) func() model.ID {
	return func() model.ID { return b.nextIDFor(prefix) }
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
