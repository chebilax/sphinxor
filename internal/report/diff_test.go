package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/diff"
	"github.com/chebilax/sphinxor/internal/model"
)

func testDiffResult() diff.Result {
	return diff.Result{
		AddedEndpoints: []model.Endpoint{{ID: "POST /a", HTTPMethod: model.MethodPost, Path: "/a"}},
		BecamePublic:   []model.Endpoint{{ID: "POST /b", HTTPMethod: model.MethodPost, Path: "/b"}},
		Regressions: []diff.Regression{
			{
				Finding: model.Finding{RuleID: "empty-role", Message: "example message"},
				Reason:  diff.ReasonNew,
			},
		},
	}
}

func TestWriteDiff_Markdown(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteDiff(&buf, testDiffResult(), FormatMarkdown); err != nil {
		t.Fatalf("WriteDiff: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "# Model Diff") {
		t.Errorf("missing header: %s", out)
	}
	if !strings.Contains(out, "POST /a") {
		t.Errorf("missing added endpoint: %s", out)
	}
	if !strings.Contains(out, "POST /b") {
		t.Errorf("missing became-public endpoint: %s", out)
	}
	if !strings.Contains(out, "NEW") || !strings.Contains(out, "example message") {
		t.Errorf("missing regression: %s", out)
	}
}

func TestWriteDiff_JSON(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteDiff(&buf, testDiffResult(), FormatJSON); err != nil {
		t.Fatalf("WriteDiff: %v", err)
	}
	var decoded diff.Result
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON matching diff.Result: %v\n%s", err, buf.String())
	}
	if len(decoded.Regressions) != 1 {
		t.Errorf("decoded %d regressions, want 1", len(decoded.Regressions))
	}
}

func TestWriteDiff_NoChangesReadsCleanly(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteDiff(&buf, diff.Result{}, FormatMarkdown); err != nil {
		t.Fatalf("WriteDiff: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "0 regression(s)") {
		t.Errorf("expected zero regressions to be stated plainly: %s", out)
	}
}

// TestWriteDiff_ExcusedTransitionIsMarked is ADR 0036 §7's readability
// half: an allowlisted became-public transition stays in the report
// rather than vanishing because it was excused — but it says so, or a
// reader seeing "0 regression(s)" above a listed endpoint is left to work
// out why it did not fail the build.
func TestWriteDiff_ExcusedTransitionIsMarked(t *testing.T) {
	e := model.Endpoint{ID: "DELETE /things/1", HTTPMethod: model.MethodDelete, Path: "/things/1"}
	gatedEndpoint := model.Endpoint{ID: "DELETE /things/2", HTTPMethod: model.MethodDelete, Path: "/things/2"}

	result := diff.Result{
		BecamePublic: []model.Endpoint{e, gatedEndpoint},
		Regressions: []diff.Regression{{
			Finding: model.Finding{RuleID: "endpoint-became-public", SubjectID: gatedEndpoint.ID, SubjectKind: model.SubjectEndpoint, Message: "gated"},
			Reason:  diff.ReasonBecamePublic,
		}},
	}

	var b bytes.Buffer
	if err := WriteDiff(&b, result, FormatMarkdown); err != nil {
		t.Fatal(err)
	}
	out := b.String()

	if !strings.Contains(out, "- DELETE /things/1 (allowlisted — does not fail the build)") {
		t.Errorf("the excused transition is not marked as excused:\n%s", out)
	}
	if strings.Contains(out, "- DELETE /things/2 (allowlisted") {
		t.Errorf("a gated transition was marked as allowlisted:\n%s", out)
	}
	if !strings.Contains(out, "- DELETE /things/2\n") {
		t.Errorf("the gated transition is missing from Became Public:\n%s", out)
	}
}

// TestWriteDiff_PermissionReferencesRenderViaAndLocation pins ADR 0037
// §2's rendering. The entry must carry the callee, not the bare literal:
// RuoYi-Vue's one @ss.hasRole('admin') under a heading reading
// "Permission References" would assert exactly what ADR 0035 §3 declined
// to decide, and the diff must not be the one place that claim is
// unqualified.
func TestWriteDiff_PermissionReferencesRenderViaAndLocation(t *testing.T) {
	result := diff.Result{
		AddedPermissionReferences: []model.PermissionReference{
			{Via: "@ss.hasPermi", RawLiteral: "system:user:remove", File: "SysUserController.java", Line: 149},
		},
		RemovedPermissionReferences: []model.PermissionReference{
			{Via: "@ss.hasRole", RawLiteral: "admin", File: "GenController.java", Line: 128},
		},
	}

	var buf bytes.Buffer
	if err := WriteDiff(&buf, result, FormatMarkdown); err != nil {
		t.Fatalf("WriteDiff: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "## Permission References") {
		t.Errorf("missing section: %s", out)
	}
	if !strings.Contains(out, "+ @ss.hasPermi('system:user:remove') (SysUserController.java:149)") {
		t.Errorf("added permission not rendered in source form with its location: %s", out)
	}
	if !strings.Contains(out, "- @ss.hasRole('admin') (GenController.java:128)") {
		t.Errorf("removed permission not rendered in source form with its location: %s", out)
	}
}

// TestWriteDiff_PermissionSpellingMatchesTheMatrix is the point of
// sharing renderPermissionReference: a reader comparing the diff against
// the RBAC matrix must see one spelling, not two.
func TestWriteDiff_PermissionSpellingMatchesTheMatrix(t *testing.T) {
	ref := model.PermissionReference{Via: "@ss.hasPermi", RawLiteral: "system:user:edit", File: "a.java", Line: 1}

	var buf bytes.Buffer
	if err := WriteDiff(&buf, diff.Result{AddedPermissionReferences: []model.PermissionReference{ref}}, FormatMarkdown); err != nil {
		t.Fatalf("WriteDiff: %v", err)
	}

	matrix := BuildMatrix(&model.Model{
		Endpoints:            []model.Endpoint{{ID: "PUT /system/user", HTTPMethod: model.MethodPut, Path: "/system/user"}},
		GuardApplications:    []model.GuardApplication{{ID: "g1", EndpointID: "PUT /system/user", GuardName: "PreAuthorize", DeclaresPermissions: true}},
		PermissionReferences: []model.PermissionReference{{ID: "p1", GuardApplicationID: "g1", Via: ref.Via, RawLiteral: ref.RawLiteral}},
	}, nil)

	if len(matrix.Rows) != 1 || len(matrix.Rows[0].Permissions) != 1 {
		t.Fatalf("matrix did not project the permission: %+v", matrix.Rows)
	}
	spelling := matrix.Rows[0].Permissions[0]
	if !strings.Contains(buf.String(), spelling) {
		t.Errorf("diff renders a different spelling from the matrix's %q:\n%s", spelling, buf.String())
	}
}

// TestWriteDiff_NoPermissionChangeSaysSo keeps the new section's empty
// state consistent with every other section's, rather than leaving a
// bare heading a reader has to interpret.
func TestWriteDiff_NoPermissionChangeSaysSo(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteDiff(&buf, diff.Result{}, FormatMarkdown); err != nil {
		t.Fatalf("WriteDiff: %v", err)
	}
	section := buf.String()[strings.Index(buf.String(), "## Permission References"):]
	if !strings.Contains(section, "No change.") {
		t.Errorf("empty permission section should say so: %q", section)
	}
}
