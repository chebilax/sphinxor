package spring

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chebilax/sphinxor/internal/model"
)

// ScanKotlin counts the Kotlin source files under dir and which of them
// declare Spring controllers or Spring Security configuration —
// docs/decisions/0042-kotlin-sources-announced.md. Kotlin is not parsed
// (ADR 0011 §1); this finds out whether the run must say so.
//
// It follows ADR 0027's rule: a declaration counts, a mention never does.
// Comments, string literals (raw strings included) and import/package
// lines are removed before anything is matched, so a SecurityFilterChain
// named in a KDoc, an import or a log message announces nothing. What is
// matched is a declaration's shape: an annotation applied, a function
// returning a chain, a class extending the adapter.
//
// The same exclusions as the Java walk apply: build output, VCS metadata,
// directories named test, and files named like tests.
func ScanKotlin(dir string) (model.KotlinStatus, error) {
	var st model.KotlinStatus
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "target", "build", "out", ".git", "test", ".gradle":
				return filepath.SkipDir
			}
			return nil
		}
		if !isKotlinSourceFile(path) {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		st.Files++
		decl := kotlinDeclarationText(string(src))
		rel, _ := filepath.Rel(dir, path)
		if kotlinController.MatchString(decl) {
			st.ControllerFiles++
			st.ControllerExamples = appendExample(st.ControllerExamples, rel)
		}
		if kotlinChainBean.MatchString(decl) || kotlinAdapter.MatchString(decl) {
			st.URLSecurityFiles++
			st.SecurityExamples = appendExample(st.SecurityExamples, rel)
		}
		if kotlinMethodEnabler.MatchString(decl) {
			st.MethodEnablerFiles++
			st.SecurityExamples = appendExample(st.SecurityExamples, rel)
		}
		return nil
	})
	return st, err
}

func isKotlinSourceFile(path string) bool {
	if !strings.HasSuffix(path, ".kt") {
		return false
	}
	base := strings.TrimSuffix(filepath.Base(path), ".kt")
	for _, suffix := range []string{"Test", "Tests", "IT"} {
		if strings.HasSuffix(base, suffix) {
			return false
		}
	}
	return true
}

func appendExample(list []string, rel string) []string {
	for _, r := range list {
		if r == rel {
			return list
		}
	}
	if len(list) < 3 {
		list = append(list, rel)
	}
	return list
}

var (
	// An annotation applied: @RestController, or written qualified.
	kotlinController = regexp.MustCompile(`@(?:[\w.]+\.)?(?:RestController|Controller)\b`)
	// A function whose declared return type is a chain:
	// fun filterChain(http: HttpSecurity): SecurityFilterChain
	kotlinChainBean = regexp.MustCompile(`(?s)\bfun\b[^{]*?\)\s*:\s*(?:[\w.]+\.)?(?:SecurityFilterChain|SecurityWebFilterChain)\b`)
	// A class whose header extends the pre-5.7 adapter.
	kotlinAdapter = regexp.MustCompile(`(?s)\bclass\b[^{]*?:[^{]*?\bWebSecurityConfigurerAdapter\b`)
	// A method-security enabler applied.
	kotlinMethodEnabler = regexp.MustCompile(`@(?:[\w.]+\.)?(?:EnableMethodSecurity|EnableGlobalMethodSecurity|EnableReactiveMethodSecurity)\b`)
)

// kotlinDeclarationText returns src with comments (nested block comments
// included, as Kotlin allows), string and character literals, and
// import/package lines replaced by spaces, so only declarations remain to
// be matched. It is a lexer's first pass, not a parser: it never decides
// what a declaration means.
func kotlinDeclarationText(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	n := len(src)
	for i := 0; i < n; {
		switch {
		case strings.HasPrefix(src[i:], "//"):
			for i < n && src[i] != '\n' {
				i++
			}
		case strings.HasPrefix(src[i:], "/*"):
			depth := 0
			for i < n {
				if strings.HasPrefix(src[i:], "/*") {
					depth++
					i += 2
				} else if strings.HasPrefix(src[i:], "*/") {
					depth--
					i += 2
					if depth == 0 {
						break
					}
				} else {
					i++
				}
			}
			b.WriteByte(' ')
		case strings.HasPrefix(src[i:], `"""`):
			i += 3
			for i < n && !strings.HasPrefix(src[i:], `"""`) {
				i++
			}
			i += 3
			b.WriteString(`""`)
		case src[i] == '"' || src[i] == '\'':
			quote := src[i]
			i++
			for i < n && src[i] != quote && src[i] != '\n' {
				if src[i] == '\\' {
					i++
				}
				i++
			}
			i++
			b.WriteString(`""`)
		default:
			b.WriteByte(src[i])
			i++
		}
	}
	lines := strings.Split(b.String(), "\n")
	for k, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "import ") || strings.HasPrefix(t, "package ") {
			lines[k] = ""
		}
	}
	return strings.Join(lines, "\n")
}
