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

	"github.com/chebilax/sphinxor/internal/allowlist"
	"github.com/chebilax/sphinxor/internal/extract/collide"
	"github.com/chebilax/sphinxor/internal/model"
)

// Extract walks the Spring project rooted at dir and builds the
// intermediate model: controllers, endpoints, method-security guard
// applications, SecurityFilterChain-derived (URL layer) guard
// applications, role declarations/references, and authentication
// requirements — the full method×URL effective-policy surface ADR 0012
// describes, ready for internal/export/cerbos's Translate to reduce.
func Extract(dir string) (*model.Model, allowlist.Outcome, error) {
	files, err := parseProject(dir)
	if err != nil {
		return nil, allowlist.Outcome{}, err
	}

	b := newBuilder()

	// SecurityFilterChain rules are found once, up front: Pass 0 needs
	// their role literals (a role referenced only by a SecurityFilterChain
	// rule, never by any annotation, must still resolve), and the later
	// application pass (below) needs the same rules again.
	chainRules, chainFile, hasChain := findSecurityFilterChainRules(files)

	// Pass 0: role declarations, project-wide, before resolving any
	// reference to them — mirrors internal/extract/nestjs/extract.go's own
	// staging (a declaration can live in a different file than every
	// reference to it), restricted to literals actually referenced
	// somewhere (roles.go's own doc comment) so an unrelated enum/constant
	// in the project isn't mistaken for a role registry. "Referenced
	// somewhere" now includes SecurityFilterChain rules, not just
	// annotations.
	usedLiterals := make(map[string]bool)
	for _, f := range files {
		for lit := range collectUsedRoleLiterals(f.tree.RootNode(), f.src) {
			usedLiterals[lit] = true
		}
	}
	for _, r := range chainRules {
		for _, lit := range r.roles {
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

	// Pass 2: controllers, endpoints, method-security (method-layer)
	// guards, and sphinxor-allow marker matching. Matching is file-scoped
	// (a marker only ever exempts an endpoint in the same file), so it
	// happens per file alongside extraction rather than as a separate
	// project-wide pass — same staging as internal/extract/nestjs, and it
	// shares the same matcher (internal/allowlist.MatchFile) rather than
	// reimplementing it: the marker grammar is a `//` line comment in both
	// languages, and matching operates on line positions and anchors, not
	// on either language's syntax tree.
	// Controller-composing meta-annotations must be known before any
	// controller is walked, because the declaration and its uses live in
	// different files — shenyu declares @RestApi once and applies it 35
	// files away (ADR 0024 §1).
	controllerMetas := make(map[string]controllerMeta)
	for _, f := range files {
		scanControllerMetaAnnotations(f.tree.RootNode(), f.src, controllerMetas)
	}

	for _, f := range files {
		extractControllers(f.tree.RootNode(), f.src, f.relPath, b, roleByName, controllerMetas)
	}

	// Pass 2b: ADR 0023 §3. For each third-party authorization framework
	// whose annotations pass 2 actually recorded, say whether the wiring
	// that switches them on is present. It runs after pass 2 because it
	// only reports on frameworks that were seen, and before any consumer
	// reads b.model.ThirdPartyAuth.
	scanThirdPartyAuthStatus(files, b)

	// Two controllers declaring one route are two endpoints until proven
	// otherwise (ADR 0020 Amendment 2 §8). It runs after every file has
	// been walked, and before both the allowlist matching below and the
	// URL-layer pass further down, so each of those sees final IDs.
	collide.Resolve(collide.Input{
		Model:       &b.model,
		GuardOwner:  b.guardOwner,
		Anchors:     b.anchors,
		AnchorOwner: b.anchorOwner,
	})

	outcome := allowlist.Outcome{AllowlistedEndpoints: make(map[model.ID]bool)}
	for _, f := range files {
		var anchors []allowlist.Anchor
		for _, a := range b.anchors {
			if a.File == f.relPath {
				anchors = append(anchors, a)
			}
		}
		allowlisted, stale := allowlist.MatchFile(f.src, f.relPath, anchors, b.nextID("finding"))
		for _, id := range allowlisted {
			outcome.AllowlistedEndpoints[id] = true
		}
		outcome.StaleMarkers = append(outcome.StaleMarkers, stale...)
	}

	// Pass 3: SecurityFilterChain (URL-layer) guards, evaluated against
	// every endpoint discovered in Pass 2 — docs/decisions/0012-securityfilterchain-effective-policy.md,
	// docs/decisions/0018-unrecognized-rule-stops-evaluation.md. Only when
	// exactly one SecurityFilterChain bean was found project-wide;
	// otherwise the URL layer simply contributes nothing, the same as a
	// project with no SecurityFilterChain support to find.
	if hasChain {
		b.applySecurityFilterChain(chainRules, chainFile, roleByName)
	}

	// Record what became of the URL layer, per ADR 0020 §2. Presence is
	// counted independently of parseability: without that, "no URL layer"
	// and "a URL layer nobody could read" are indistinguishable, and the
	// second silently gets treated as the first — which let a two-chain
	// project export a Cerbos grant the running application denies.
	forms := countChainBeans(files)
	b.model.URLLayer = model.URLLayerStatus{
		Present:  forms.any(),
		Analyzed: hasChain && forms.legacyAdapter == 0 && forms.shiro == 0,
	}
	if b.model.URLLayer.Unknown() {
		b.model.URLLayer.Reason = unreadableLayerReason(forms, hasChain)
	}

	// Pass 4: authentication requirements (ADR 0010), per layer (ADR 0011
	// §3) — derived from the fully-assembled RoleReference collection
	// above (an endpoint-and-layer's authCandidate can only be resolved
	// once every guard contributing to that same layer is known), same
	// ordering reason as internal/extract/nestjs's own final pass.
	b.model.AuthenticationRequirements = computeAuthenticationRequirements(&b.model, b.authCandidates, b.nextID("authreq"))

	return &b.model, outcome, nil
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
	// It is keyed by controller as well as by ID: two handlers sharing
	// HTTPMethod+Path across *different* controllers are not content
	// negotiation but a route collision (ADR 0020 Amendment 2 §8), and
	// merging them is what silently dropped one of them.
	seenEndpoints map[endpointKey]int
	// Ownership bookkeeping for that amendment — see the NestJS twin and
	// internal/extract/collide.
	guardOwner  []int
	anchors     []allowlist.Anchor
	anchorOwner []int
	curEndpoint int
	// authCandidates accumulates every @PreAuthorize("isAuthenticated()")
	// occurrence found while applying guards, consumed by the final
	// computeAuthenticationRequirements pass (authentication.go).
	authCandidates []authCandidate
}

// endpointKey identifies one endpoint within one controller.
type endpointKey struct {
	id         model.ID
	controller model.ID
}

func newBuilder() *builder {
	return &builder{counters: make(map[string]int), seenEndpoints: make(map[endpointKey]int)}
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

// IsSourceFile reports whether path is a file this extractor would parse.
// Exported so framework detection and the "did we look at anything?" count
// (docs/decisions/0019-cli-framework-selection.md §2) use this extractor's
// own rule rather than a second, drifting copy of it.
func IsSourceFile(path string) bool { return isExtractableSourceFile(path) }

// unreadableLayerReason explains Present && !Analyzed, naming every
// unreadable URL layer the project declares — docs/decisions/0027-unannounced-url-layers.md §3.
//
// It is a list rather than a single clause because a project can declare
// more than one: JeecgBoot has both a reactive SecurityWebFilterChain and
// a ShiroFilterFactoryBean. The previous single-Reason switch would have
// let whichever case matched first hide the other, and both are real and
// unread.
func unreadableLayerReason(forms urlLayerForms, servletChainParsed bool) string {
	var parts []string
	switch {
	case forms.servlet > 1:
		parts = append(parts, fmt.Sprintf("%d SecurityFilterChain beans were found; which one governs a given request depends on @Order/securityMatcher, which is not resolved", forms.servlet))
	case forms.servlet == 1 && !servletChainParsed:
		parts = append(parts, "a SecurityFilterChain bean was found but its authorizeHttpRequests rules could not be parsed")
	}
	if forms.reactive > 0 {
		parts = append(parts, "a reactive SecurityWebFilterChain was found; reactive chain rules are not parsed yet")
	}
	if forms.legacyAdapter > 0 {
		parts = append(parts, "a WebSecurityConfigurerAdapter was found; this pre-Spring-Security-5.7 URL layer's authorizeRequests rules are not parsed")
	}
	if forms.shiro > 0 {
		// Named as Shiro deliberately: "your URL layer could not be
		// analyzed" sends a reader looking for a SecurityFilterChain
		// they do not have.
		parts = append(parts, "an Apache Shiro ShiroFilterFactoryBean was found; Shiro is not Spring Security and its filter chain definitions are not parsed")
	}
	if len(parts) == 0 {
		// Defensive: Unknown() was true, so something is present and
		// unread. Saying so beats an empty clause.
		return "a URL-authorization layer was found but could not be analyzed"
	}
	return strings.Join(parts, "; and ")
}
