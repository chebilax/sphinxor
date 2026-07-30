package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/chebilax/sphinxor/internal/model"
)

func writeMarkdown(w io.Writer, matrix Matrix) error {
	var b strings.Builder

	b.WriteString("# RBAC Matrix\n\n")

	blocking, warnings, allowlisted := countByStatus(matrix.Findings)
	fmt.Fprintf(&b, "%d endpoint(s), %d finding(s): %d blocking, %d warning, %d allowlisted.\n\n",
		len(matrix.Rows), len(matrix.Findings), blocking, warnings, allowlisted)

	b.WriteString("## Endpoints\n\n")
	b.WriteString("| Method | Path | Handler | Controller | Guards | Roles | Findings |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")
	for _, row := range matrix.Rows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n",
			row.Method,
			row.Path,
			row.Handler,
			row.Controller,
			joinOrDash(row.Guards),
			joinOrDash(row.Roles),
			findingSummaries(row.Findings),
		)
	}
	b.WriteString("\n")

	b.WriteString("## Findings\n\n")
	if len(matrix.Findings) == 0 {
		b.WriteString("None.\n")
	}
	for _, f := range matrix.Findings {
		status := "warning"
		if f.Allowlisted {
			status = "allowlisted"
		} else if f.Confidence == model.ConfidenceHigh {
			status = "blocking"
		}
		fmt.Fprintf(&b, "- [%s/%s] `%s`: %s\n",
			strings.ToUpper(string(f.Confidence)), status, f.RuleID, f.Message)
	}

	_, err := io.WriteString(w, b.String())
	return err
}

func countByStatus(findings []model.Finding) (blocking, warnings, allowlisted int) {
	for _, f := range findings {
		switch {
		case f.Allowlisted:
			allowlisted++
		case f.Confidence == model.ConfidenceHigh:
			blocking++
		default:
			warnings++
		}
	}
	return blocking, warnings, allowlisted
}

func joinOrDash(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, ", ")
}

func findingSummaries(findings []model.Finding) string {
	if len(findings) == 0 {
		return "-"
	}
	parts := make([]string, len(findings))
	for i, f := range findings {
		marker := ""
		if f.Allowlisted {
			marker = " (allowlisted)"
		}
		parts[i] = fmt.Sprintf("%s%s", f.RuleID, marker)
	}
	return strings.Join(parts, "; ")
}
