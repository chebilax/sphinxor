package nestjs

import (
	"fmt"
	"strings"

	"github.com/chebilax/sphinxor/internal/allowlist"
	"github.com/chebilax/sphinxor/internal/model"
)

// matchFileAllowlist matches sphinxor-allow markers found in src against
// the endpoints extracted from the same file, per
// docs/decisions/0003-allowlist-format.md: a marker allowlists the
// endpoint whose anchor line is the next non-blank, non-comment line
// after it. A marker matching nothing produces a stale-allow-marker
// finding instead of being silently ignored.
func matchFileAllowlist(src []byte, file string, anchors []endpointAnchor, next func() model.ID) (allowlisted []model.ID, stale []model.Finding) {
	lines := strings.Split(string(src), "\n")

	anchorAtLine := make(map[int]model.ID, len(anchors))
	for _, a := range anchors {
		anchorAtLine[a.Line] = a.EndpointID
	}

	for i, line := range lines {
		lineNumber := i + 1
		marker, ok := allowlist.ParseMarker(line, file, lineNumber)
		if !ok {
			continue
		}

		matchedID, found := nextRelevantEndpoint(lines, lineNumber, anchorAtLine)
		if found {
			allowlisted = append(allowlisted, matchedID)
			continue
		}

		stale = append(stale, model.Finding{
			ID:          next(),
			RuleID:      "stale-allow-marker",
			Confidence:  model.ConfidenceHigh,
			SubjectID:   model.ID(fmt.Sprintf("%s:%d", file, lineNumber)),
			SubjectKind: model.SubjectAllowMarker,
			Message:     fmt.Sprintf("sphinxor-allow marker (%q) does not sit directly above a recognized endpoint", marker.Reason),
		})
	}

	return allowlisted, stale
}

// nextRelevantEndpoint scans forward from just after a marker, skipping
// blank lines and line comments, and reports whether the first
// substantive line found is a known endpoint anchor.
func nextRelevantEndpoint(lines []string, markerLine int, anchorAtLine map[int]model.ID) (model.ID, bool) {
	for lineNumber := markerLine + 1; lineNumber <= len(lines); lineNumber++ {
		text := strings.TrimSpace(lines[lineNumber-1])
		if text == "" || strings.HasPrefix(text, "//") {
			continue
		}
		id, ok := anchorAtLine[lineNumber]
		return id, ok
	}
	return "", false
}
