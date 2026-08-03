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
