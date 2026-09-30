package report

import (
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/export/cerbos"
	"github.com/chebilax/sphinxor/internal/model"
)

// TestExportReportStatesWildcardMeaning: the export report says what a "*"
// grant means whenever one is exported, where the report is read.
func TestExportReportStatesWildcardMeaning(t *testing.T) {
	e := model.Endpoint{ID: "GET /a", HTTPMethod: model.MethodGet, Path: "/a"}
	var b strings.Builder
	r := cerbos.Result{Rules: []cerbos.Rule{{Resource: "a", Action: "get", Roles: []string{"*"}, Endpoints: []model.Endpoint{e}}}}
	if err := WriteExport(&b, r, FormatMarkdown); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `## What "*" means`) || !strings.Contains(b.String(), cerbos.WildcardMeaning) {
		t.Errorf("export report with a \"*\" rule does not state its meaning:\n%s", b.String())
	}
}
