package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/chebilax/sphinxor/internal/diff"
)

// WriteDiff renders a diff.Result to w in the given format, per
// docs/decisions/0007-model-diff-design.md.
func WriteDiff(w io.Writer, result diff.Result, format Format) error {
	switch format {
	case FormatJSON:
		return writeDiffJSON(w, result)
	case FormatMarkdown, "":
		return writeDiffMarkdown(w, result)
	default:
		return fmt.Errorf("report: unknown format %q", format)
	}
}

func writeDiffJSON(w io.Writer, result diff.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func writeDiffMarkdown(w io.Writer, result diff.Result) error {
	var b strings.Builder

	b.WriteString("# Model Diff\n\n")
	fmt.Fprintf(&b, "%d regression(s).\n\n", len(result.Regressions))

	b.WriteString("## Regressions\n\n")
	if len(result.Regressions) == 0 {
		b.WriteString("None.\n\n")
	}
	for _, reg := range result.Regressions {
		fmt.Fprintf(&b, "- [%s] `%s`: %s\n", strings.ToUpper(string(reg.Reason)), reg.Finding.RuleID, reg.Finding.Message)
	}
	if len(result.Regressions) > 0 {
		b.WriteString("\n")
	}

	b.WriteString("## Endpoints\n\n")
	for _, e := range result.AddedEndpoints {
		fmt.Fprintf(&b, "+ %s %s\n", e.HTTPMethod, e.Path)
	}
	for _, e := range result.RemovedEndpoints {
		fmt.Fprintf(&b, "- %s %s\n", e.HTTPMethod, e.Path)
	}
	if len(result.AddedEndpoints) == 0 && len(result.RemovedEndpoints) == 0 {
		b.WriteString("No change.\n")
	}
	b.WriteString("\n")

	b.WriteString("## Became Public\n\n")
	if len(result.BecamePublic) == 0 {
		b.WriteString("None.\n")
	}
	for _, e := range result.BecamePublic {
		fmt.Fprintf(&b, "- %s %s\n", e.HTTPMethod, e.Path)
	}
	b.WriteString("\n")

	b.WriteString("## Role Declarations\n\n")
	for _, d := range result.AddedRoleDeclarations {
		fmt.Fprintf(&b, "+ %s\n", d.Name)
	}
	for _, d := range result.RemovedRoleDeclarations {
		fmt.Fprintf(&b, "- %s\n", d.Name)
	}
	if len(result.AddedRoleDeclarations) == 0 && len(result.RemovedRoleDeclarations) == 0 {
		b.WriteString("No change.\n")
	}
	b.WriteString("\n")

	b.WriteString("## Guard Applications\n\n")
	for _, g := range result.AddedGuardApplications {
		fmt.Fprintf(&b, "+ %s on %s (%s)\n", g.GuardName, g.EndpointID, g.AppliedAt)
	}
	for _, g := range result.RemovedGuardApplications {
		fmt.Fprintf(&b, "- %s on %s (%s)\n", g.GuardName, g.EndpointID, g.AppliedAt)
	}
	if len(result.AddedGuardApplications) == 0 && len(result.RemovedGuardApplications) == 0 {
		b.WriteString("No change.\n")
	}
	b.WriteString("\n")

	b.WriteString("## Role References\n\n")
	for _, r := range result.AddedRoleReferences {
		fmt.Fprintf(&b, "+ %s\n", r.RawLiteral)
	}
	for _, r := range result.RemovedRoleReferences {
		fmt.Fprintf(&b, "- %s\n", r.RawLiteral)
	}
	if len(result.AddedRoleReferences) == 0 && len(result.RemovedRoleReferences) == 0 {
		b.WriteString("No change.\n")
	}

	_, err := io.WriteString(w, b.String())
	return err
}
