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
	b.WriteString("| Method | Path | Handler | Controller | Guards | Roles | Permissions | Findings |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	for _, row := range matrix.Rows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n",
			row.Method,
			renderPath(row),
			row.Handler,
			row.Controller,
			renderGuards(row),
			renderRoles(row),
			renderPermissions(row),
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

// renderGuards shows what protects an endpoint, marking the case where an
// access-control annotation was found and could not be identified (ADR
// 0022 §2). "-" asserts nothing guards this endpoint; "?" says something
// does and Sphinxor could not say what. The run's warning names the
// package it actually came from.
func renderGuards(row Row) string {
	if !row.UnrecognizedAuth {
		return joinOrDash(row.Guards)
	}
	if len(row.Guards) == 0 {
		return "?"
	}
	return strings.Join(row.Guards, ", ") + ", ?"
}

// renderRoles shows an endpoint's role requirement, marking the case
// where part or all of it could not be read (ADR 0020 Amendment 3 §11).
// "-" means "no role required" and would be a claim; "?" says the
// requirement exists and was not recovered. An endpoint with some roles
// read and another annotation unread gets both, since the roles shown are
// real but are not known to be the whole list.
func renderRoles(row Row) string {
	if !row.RolesUnresolved {
		return joinOrDash(row.Roles)
	}
	if len(row.Roles) == 0 {
		return "?"
	}
	return strings.Join(row.Roles, ", ") + ", ?"
}

// renderPermissions shows an endpoint's named requirements that are not
// roles, each with the bean call that named it
// (docs/decisions/0035-permissions-in-the-model.md §5).
//
// The "?"/"-" discipline is ADR 0020 Amendment 3 §11's, driven by the same
// flag as renderRoles: an unread expression leaves the requirement unknown
// without saying which kind it names, so "-" here would be a claim.
//
// This column is why renderRoles's "-" is safe on a permission-bearing
// endpoint. PUT /system/user in RuoYi-Vue renders Guards "-" (a
// role-declaring guard is surfaced under Roles, not Guards) and Roles "-"
// (the expression was read and names no role). Without this cell the row
// reads "- | -": a stronger and entirely false claim than the "?" it
// replaced.
func renderPermissions(row Row) string {
	if !row.RolesUnresolved {
		return joinOrDash(row.Permissions)
	}
	if len(row.Permissions) == 0 {
		return "?"
	}
	return strings.Join(row.Permissions, ", ") + ", ?"
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

// renderPath marks a path that is only a fragment of the real route,
// because the declared path could not be read (ADR 0020 Amendment 1 §5).
// The leading ellipsis is explained by the run's project-level warning;
// printing the fragment bare would present it as a route that exists.
func renderPath(row Row) string {
	path := row.Path
	if row.PathUnresolved {
		path = "\u2026" + path
	}
	// A declared version is part of what tells two same-path routes apart
	// (ADR 0020 Amendment 2 §7), so it is shown beside the path rather
	// than left to make the two rows look like duplicates. "?" is a
	// version that was declared but could not be read.
	switch {
	case row.VersionUnresolved:
		path += " @?"
	case row.Version != "":
		path += " @" + row.Version
	}
	return path
}
