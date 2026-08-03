package cli

import (
	"fmt"

	"github.com/chebilax/sphinxor/internal/extract/nestjs"
	"github.com/chebilax/sphinxor/internal/lint"
	"github.com/chebilax/sphinxor/internal/model"
)

// analyzeDirectory extracts the model at dir and runs the full v0.1 rule
// set against it, including allowlist matching and stale-marker
// findings — the shared pipeline behind both `sphinxor lint` and
// `sphinxor diff`.
func analyzeDirectory(dir string) (*model.Model, []model.Finding, error) {
	m, allow, err := nestjs.Extract(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("extracting model at %s: %w", dir, err)
	}

	findings := lint.Run(m, lint.DefaultRules(), allow.AllowlistedEndpoints)
	findings = append(findings, allow.StaleMarkers...)

	return m, findings, nil
}
