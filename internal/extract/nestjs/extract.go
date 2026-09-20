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

	"github.com/chebilax/sphinxor/internal/allowlist"
	"github.com/chebilax/sphinxor/internal/model"
)

// Extract walks the NestJS project rooted at dir and builds the
// intermediate model: controllers, endpoints, guard applications, role
// declarations, and role references, plus the allowlist marker outcome.
func Extract(dir string) (*model.Model, allowlist.Outcome, error) {
	files, err := parseProject(dir)
	if err != nil {
		return nil, allowlist.Outcome{}, err
	}

	b := newBuilder()

	// Pass 0: composite decorator definitions, project-wide — a composite
	// (e.g. @Auth(...)) can be defined in a different file than every
	// place it's used (docs/decisions/0006), same as role enums below.
	// Name collisions across files last-write-win; composite decorator
	// names are not expected to collide within one project, and resolving
	// that generally is out of scope for this heuristic.
	composites := make(map[string]compositeDecorator)
	for _, f := range files {
		for name, cd := range collectCompositeDecorators(f.tree.RootNode(), f.src) {
			composites[name] = cd
		}
	}

	// Pass 1: role declarations, project-wide, before resolving any
	// reference to them — a role can be declared in a different file than
	// the one that references it (e.g. roles.enum.ts vs. users.controller.ts).
	// Restricted to enums actually named by a @Roles() call somewhere,
	// literal or composite-resolved (see roles.go), so unrelated enums
	// (env config, status flags, ...) aren't mistaken for role registries.
	usedEnumNames := make(map[string]bool)
	for _, f := range files {
		for name := range collectRoleEnumNames(f.tree.RootNode(), f.src, composites) {
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
	outcome := allowlist.Outcome{AllowlistedEndpoints: make(map[model.ID]bool)}
	for _, f := range files {
		fileAnchors := extractControllers(f.tree.RootNode(), f.src, f.relPath, b, roleByName, composites)

		allowlisted, stale := allowlist.MatchFile(f.src, f.relPath, fileAnchors, b.nextID("finding"))
		for _, id := range allowlisted {
			outcome.AllowlistedEndpoints[id] = true
		}
		outcome.StaleMarkers = append(outcome.StaleMarkers, stale...)
	}

	// Pass 3: authentication requirements (ADR 0010) — derived from the
	// fully-assembled GuardApplication/RoleReference collections above, so
	// it runs after Pass 2 rather than incrementally per file: an
	// endpoint's guards can come from both class- and method-level
	// decorators, and the "zero resolved roles anywhere on this endpoint"
	// check needs all of them known first.
	b.model.GlobalGuards = detectGlobalGuards(files)
	b.model.GraphQL = detectGraphQL(files)

	b.model.AuthenticationRequirements = computeAuthenticationRequirements(&b.model, b.nextID("authreq"))

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

// IsSourceFile reports whether path is a file this extractor would parse.
// Exported so framework detection and the "did we look at anything?" count
// (docs/decisions/0019-cli-framework-selection.md §2) use this extractor's
// own rule rather than a second, drifting copy of it.
func IsSourceFile(path string) bool { return isExtractableSourceFile(path) }

// The two ways a NestJS application registers a guard that applies to
// every route without any decorator appearing at the endpoint: an
// APP_GUARD-token provider in a module, and app.useGlobalGuards() at
// bootstrap.
//
// Detecting them is ADR 0020 §4. On these patterns — the first is what
// NestJS's own docs recommend, with @Public() opting out — every
// endpoint is protected by default, so endpoint-level results understate
// protection across the board. Extraction still can't see what the guard
// requires, and this does not try to: it only establishes that the
// picture is inverted, which is the difference between a safe-direction
// gap and a silent one.
const (
	appGuardToken       = "APP_GUARD"
	useGlobalGuardsCall = "useGlobalGuards"
)

// detectGlobalGuards reports whether either global-registration form
// appears anywhere in the parsed project.
func detectGlobalGuards(files []parsedFile) model.GlobalGuardStatus {
	for _, f := range files {
		var mechanism string
		var walk func(n *sitter.Node)
		walk = func(n *sitter.Node) {
			if mechanism != "" {
				return
			}
			switch n.Type() {
			case "identifier":
				if n.Content(f.src) == appGuardToken {
					mechanism = "an APP_GUARD provider"
					return
				}
			case "member_expression":
				if prop := n.ChildByFieldName("property"); prop != nil &&
					prop.Content(f.src) == useGlobalGuardsCall {
					mechanism = "an app.useGlobalGuards() call"
					return
				}
			}
			for _, c := range namedChildren(n) {
				walk(c)
			}
		}
		walk(f.tree.RootNode())
		if mechanism != "" {
			return model.GlobalGuardStatus{Registered: true, Mechanism: mechanism}
		}
	}
	return model.GlobalGuardStatus{}
}
