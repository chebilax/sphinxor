// Package report renders an analyzed model and its findings as the RBAC
// matrix output described in docs/vision.md's v0.1 scope: Markdown or
// JSON.
//
// Per docs/decisions/0002-intermediate-model-structure.md, this output is
// a join/projection over the model's normalized collections, not the
// model's native shape — Matrix is that projection.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/chebilax/sphinxor/internal/model"
)

// Format selects the RBAC matrix output format.
type Format string

const (
	FormatMarkdown Format = "markdown"
	FormatJSON     Format = "json"
)

// Row is one endpoint's projection: its guards and role references
// resolved and deduplicated, plus any findings whose subject is this
// endpoint.
type Row struct {
	Controller string           `json:"controller"`
	Method     model.HTTPMethod `json:"method"`
	Path       string           `json:"path"`
	Handler    string           `json:"handler"`
	File       string           `json:"file"`
	Line       int              `json:"line"`
	Guards     []string         `json:"guards"`
	Roles      []string         `json:"roles"`
	Findings   []model.Finding  `json:"findings,omitempty"`
}

// Matrix is the full RBAC matrix: one row per endpoint, plus every
// finding produced by the run (including ones whose subject isn't an
// endpoint, e.g. an unreferenced role declaration or a stale allow
// marker).
type Matrix struct {
	Rows     []Row           `json:"endpoints"`
	Findings []model.Finding `json:"findings"`
}

// BuildMatrix joins m's normalized collections and findings into the
// endpoint-centric projection the RBAC matrix output renders.
func BuildMatrix(m *model.Model, findings []model.Finding) Matrix {
	controllerName := make(map[model.ID]string, len(m.Controllers))
	for _, c := range m.Controllers {
		controllerName[c.ID] = c.Name
	}

	guardsByEndpoint := make(map[model.ID][]string)
	guardAppByID := make(map[model.ID]model.GuardApplication, len(m.GuardApplications))
	for _, g := range m.GuardApplications {
		guardAppByID[g.ID] = g
		if g.GuardName == "Roles" {
			continue // surfaced under Roles below, not Guards
		}
		guardsByEndpoint[g.EndpointID] = appendUnique(guardsByEndpoint[g.EndpointID], g.GuardName)
	}

	rolesByEndpoint := make(map[model.ID][]string)
	for _, ref := range m.RoleReferences {
		app, ok := guardAppByID[ref.GuardApplicationID]
		if !ok {
			continue
		}
		rolesByEndpoint[app.EndpointID] = appendUnique(rolesByEndpoint[app.EndpointID], ref.RawLiteral)
	}

	findingsByEndpoint := make(map[model.ID][]model.Finding)
	for _, f := range findings {
		if f.SubjectKind == model.SubjectEndpoint {
			findingsByEndpoint[f.SubjectID] = append(findingsByEndpoint[f.SubjectID], f)
		}
	}

	rows := make([]Row, 0, len(m.Endpoints))
	for _, e := range m.Endpoints {
		rows = append(rows, Row{
			Controller: controllerName[e.ControllerID],
			Method:     e.HTTPMethod,
			Path:       e.Path,
			Handler:    e.HandlerName,
			File:       e.File,
			Line:       e.Line,
			Guards:     guardsByEndpoint[e.ID],
			Roles:      rolesByEndpoint[e.ID],
			Findings:   findingsByEndpoint[e.ID],
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Path != rows[j].Path {
			return rows[i].Path < rows[j].Path
		}
		return rows[i].Method < rows[j].Method
	})

	return Matrix{Rows: rows, Findings: findings}
}

func appendUnique(s []string, v string) []string {
	for _, existing := range s {
		if existing == v {
			return s
		}
	}
	return append(s, v)
}

// Write renders m and its findings to w in the given format.
func Write(w io.Writer, m *model.Model, findings []model.Finding, format Format) error {
	matrix := BuildMatrix(m, findings)

	switch format {
	case FormatJSON:
		return writeJSON(w, matrix)
	case FormatMarkdown, "":
		return writeMarkdown(w, matrix)
	default:
		return fmt.Errorf("report: unknown format %q", format)
	}
}

func writeJSON(w io.Writer, matrix Matrix) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(matrix)
}
