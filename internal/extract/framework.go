// Package extract selects and runs the right framework extractor for a
// project directory, per docs/decisions/0019-cli-framework-selection.md.
//
// It exists so the CLI never has to know which extractor to call, and so
// the two rules that keep selection honest live in one place: detection
// must produce exactly one confident answer or refuse, and a run that
// parsed no source files must be an error rather than an empty, clean
// report.
package extract

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chebilax/sphinxor/internal/allowlist"
	"github.com/chebilax/sphinxor/internal/extract/nestjs"
	"github.com/chebilax/sphinxor/internal/extract/spring"
	"github.com/chebilax/sphinxor/internal/model"
)

// Framework identifies one supported extractor.
type Framework string

const (
	NestJS Framework = "nestjs"
	Spring Framework = "spring"
)

// All is every supported framework, in a stable order — used for error
// messages that list what detection looked for.
var All = []Framework{NestJS, Spring}

// ParseFramework converts a --framework flag value to a Framework.
func ParseFramework(s string) (Framework, error) {
	for _, f := range All {
		if string(f) == s {
			return f, nil
		}
	}
	return "", fmt.Errorf("unknown framework %q (supported: %s)", s, Join(All))
}

// signature is the source-level marker that identifies a framework, and
// the predicate for "a file this extractor would parse."
//
// Detection keys on source signatures rather than build files
// (pom.xml/package.json) deliberately, per ADR 0019 §1: the vendored
// fixtures this project validates against are source-only curated subsets
// (ADR 0005), so a build-file rule could only ever be tested against
// hand-made directories — exactly the synthetic-fixture trap
// docs/testing.md rejects, and least acceptable in the component that
// decides whether the tool looks at anything at all.
type signature struct {
	isSourceFile func(string) bool
	// marker appears in any source file of a project using this
	// framework: an import of the framework's own package namespace.
	marker string
}

var signatures = map[Framework]signature{
	NestJS: {isSourceFile: nestjs.IsSourceFile, marker: "@nestjs/"},
	Spring: {isSourceFile: spring.IsSourceFile, marker: "org.springframework"},
}

// skipDirs are never walked, for detection or counting — build output and
// vendored dependencies would otherwise make almost any directory look
// like every framework at once.
var skipDirs = map[string]bool{
	"node_modules": true,
	"dist":         true,
	"build":        true,
	"target":       true,
	"out":          true,
	"coverage":     true,
	".git":         true,
}

// Detect reports every framework whose signature appears in at least one
// source file under dir, sorted for stable output.
//
// It deliberately returns all matches rather than picking one: resolving
// ambiguity is the caller's job and, per ADR 0019 §1, ambiguity is a hard
// error rather than a guess. A monorepo with a Java backend and a Nest BFF
// is a real shape, and silently analyzing half of it is the failure this
// whole decision exists to prevent.
func Detect(dir string) ([]Framework, error) {
	found := make(map[Framework]bool)

	err := walkSource(dir, func(path string, read func() ([]byte, error)) error {
		for f, sig := range signatures {
			if found[f] || !sig.isSourceFile(path) {
				continue
			}
			src, err := read()
			if err != nil {
				return err
			}
			if bytes.Contains(src, []byte(sig.marker)) {
				found[f] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	out := make([]Framework, 0, len(found))
	for f := range found {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// SourceFileCount reports how many files under dir the given framework's
// extractor would parse — the "did we look at anything?" signal ADR 0019
// §2 turns into an error rather than an empty report.
func SourceFileCount(dir string, f Framework) (int, error) {
	sig, ok := signatures[f]
	if !ok {
		return 0, fmt.Errorf("unknown framework %q", f)
	}
	count := 0
	err := walkSource(dir, func(path string, _ func() ([]byte, error)) error {
		if sig.isSourceFile(path) {
			count++
		}
		return nil
	})
	return count, err
}

// Run extracts the model for dir using the named framework. Both
// extractors share this signature — including the allowlist outcome —
// since the Spring allowlist port, so dispatch is a plain switch with
// nothing to reconcile.
func Run(dir string, f Framework) (*model.Model, allowlist.Outcome, error) {
	switch f {
	case NestJS:
		return nestjs.Extract(dir)
	case Spring:
		return spring.Extract(dir)
	default:
		return nil, allowlist.Outcome{}, fmt.Errorf("unknown framework %q", f)
	}
}

// walkSource walks dir, skipping build/dependency directories, calling fn
// for each regular file with a lazy reader — so a caller that only needs
// paths (SourceFileCount) never pays to read file contents.
func walkSource(dir string, fn func(path string, read func() ([]byte, error)) error) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		return fn(path, func() ([]byte, error) { return os.ReadFile(path) })
	})
}

// Join renders a framework list for an error message, e.g. "nestjs, spring".
func Join(fs []Framework) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = string(f)
	}
	return strings.Join(parts, ", ")
}
