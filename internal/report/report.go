// Package report renders an analyzed model and its findings as the RBAC
// matrix output described in docs/vision.md's v0.1 scope: Markdown or
// JSON.
package report

import (
	"fmt"
	"io"

	"github.com/chebilax/sphinxor/internal/model"
)

// Format selects the RBAC matrix output format.
type Format string

const (
	FormatMarkdown Format = "markdown"
	FormatJSON     Format = "json"
)

// Write renders m and its findings to w in the given format.
//
// Not yet implemented.
func Write(w io.Writer, m *model.Model, findings []model.Finding, format Format) error {
	return fmt.Errorf("report: Write not implemented yet (format %q)", format)
}
