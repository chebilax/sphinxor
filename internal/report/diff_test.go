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
