package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
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

// TestProjectWarnings covers the three project-level caveats of ADR 0020
// §2 and §4. Each one exists because the report is otherwise correct as
// far as it goes and silently misleading about how far that is; the
// quiet case is tested alongside them because a caveat that fires on
// every project is a caveat nobody reads.
func TestProjectWarnings(t *testing.T) {
	// A method-layer guard that resolves a role: the subject of §4's
	// method-security caveat, and enough to make a report look confident.
	guardedEndpoint := func(m *model.Model) {
		m.Endpoints = append(m.Endpoints, model.Endpoint{ID: "e1", HTTPMethod: model.MethodGet, Path: "/x"})
		m.GuardApplications = append(m.GuardApplications, model.GuardApplication{
			ID: "g1", EndpointID: "e1", GuardName: "PreAuthorize",
			AppliedAt: model.ScopeMethod, DeclaresRoles: true,
		})
		m.RoleReferences = append(m.RoleReferences, model.RoleReference{
			ID: "r1", GuardApplicationID: "g1", RawLiteral: "ADMIN",
		})
	}

	cases := []struct {
		name  string
		build func(*model.Model)
		want  string // substring, or "" for no warning at all
	}{
		{
			// The baseline that keeps the other three honest: everything
			// was analyzed, so nothing is said.
			name: "fully analyzed project is quiet",
			build: func(m *model.Model) {
				guardedEndpoint(m)
				m.URLLayer = model.URLLayerStatus{Present: true, Analyzed: true}
				m.MethodSecurity = model.MethodSecurityStatus{Found: true}
			},
			want: "",
		},
		{
			name: "unknown URL layer",
			build: func(m *model.Model) {
				guardedEndpoint(m)
				m.MethodSecurity = model.MethodSecurityStatus{Found: true}
				m.URLLayer = model.URLLayerStatus{
					Present: true, Analyzed: false, Reason: "2 SecurityFilterChain beans were found",
				}
			},
			want: "may be BROADER",
		},
		{
			// §4/finding D: @PreAuthorize with no @EnableMethodSecurity in
			// sight. The roles are still reported (ADR 0015), so the only
			// thing standing between the reader and a wide-open endpoint
			// presented as ADMIN-only is this line.
			name: "method security never located",
			build: func(m *model.Model) {
				guardedEndpoint(m)
				m.MethodSecurity = model.MethodSecurityStatus{Found: false}
			},
			want: "are NOT protected",
		},
		{
			// The same caveat must not fire on NestJS, where
			// MethodSecurity.Found is false only because the concept does
			// not exist. Before this case it did, on every project using
			// @Roles(), naming a Spring annotation that isn't there.
			name: "nestjs role guard does not trigger the spring caveat",
			build: func(m *model.Model) {
				m.Endpoints = append(m.Endpoints, model.Endpoint{ID: "e1", HTTPMethod: model.MethodGet, Path: "/x"})
				m.GuardApplications = append(m.GuardApplications, model.GuardApplication{
					ID: "g1", EndpointID: "e1", GuardName: "RolesGuard",
					AppliedAt: model.ScopeMethod, DeclaresRoles: true,
				})
			},
			want: "",
		},
		{
			// §4/finding E: the inverted default. Here the error runs the
			// safe way — protection is understated — but an unexplained
			// wall of unguarded endpoints is its own kind of wrong answer.
			name: "global guard registered",
			build: func(m *model.Model) {
				m.Endpoints = append(m.Endpoints, model.Endpoint{ID: "e1", HTTPMethod: model.MethodGet, Path: "/x"})
				m.MethodSecurity = model.MethodSecurityStatus{Found: true}
				m.GlobalGuards = model.GlobalGuardStatus{Registered: true, Mechanism: "an APP_GUARD provider"}
			},
			want: "UNDERSTATE protection",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &model.Model{}
			tc.build(m)
			got := projectWarnings(m)

			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("want no warnings on a fully-analyzed project, got %q", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("want exactly 1 warning, got %d: %q", len(got), got)
			}
			if !strings.Contains(got[0], tc.want) {
				t.Errorf("warning = %q, want it to contain %q", got[0], tc.want)
			}
		})
	}
}

// TestAnalyzeDirectory_RealProjectStaysQuiet pins the noise floor against
// the real Pharmacy project: single servlet chain, method security
// enabled, no global guard. If ADR 0020's caveats start firing here they
// have stopped meaning anything.
func TestAnalyzeDirectory_RealProjectStaysQuiet(t *testing.T) {
	var notices bytes.Buffer
	if _, _, err := analyzeDirectory(&notices, pharmacyFixture, ""); err != nil {
		t.Fatalf("analyzeDirectory: %v", err)
	}
	if strings.Contains(notices.String(), "warning:") {
		t.Errorf("a fully-analyzed real project must produce no caveats, got:\n%s", notices.String())
	}
}
