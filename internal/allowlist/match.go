package allowlist

import (
	"fmt"
	"strings"

	"github.com/chebilax/sphinxor/internal/model"
)

// Anchor is an Endpoint's textual starting line — the earliest line among
// the constructs that introduce it (decorators and the method definition
// in TypeScript; annotations and the method declaration in Java) — used to
// match a sphinxor-allow marker to "the endpoint below it"
// (docs/decisions/0003-allowlist-format.md).
//
// Computing anchors is framework-specific and stays in each extractor;
// matching a marker to one is not, and lives here.
type Anchor struct {
	EndpointID model.ID
	File       string
	Line       int
}

// Outcome is what extraction learned about sphinxor-allow markers, per
// docs/decisions/0003-allowlist-format.md: which endpoints they
// successfully exempt, and which ones didn't match anything.
type Outcome struct {
	AllowlistedEndpoints map[model.ID]bool
	StaleMarkers         []model.Finding
}

// MatchFile matches sphinxor-allow markers found in src against the
// endpoints extracted from the same file, per
// docs/decisions/0003-allowlist-format.md: a marker allowlists the
// endpoint whose anchor line is the next non-blank, non-comment line after
// it. A marker matching nothing produces a stale-allow-marker finding
// instead of being silently ignored.
//
// This is shared by every framework extractor rather than reimplemented
// per language: the marker grammar is a line comment (`//`, identical in
// TypeScript and Java), and everything below operates on line positions
// and anchors, not on any language's syntax tree. Only the anchors
// themselves are framework-specific.
func MatchFile(src []byte, file string, anchors []Anchor, next func() model.ID) (allowlisted []model.ID, stale []model.Finding) {
	lines := strings.Split(string(src), "\n")

	anchorAtLine := make(map[int]model.ID, len(anchors))
	for _, a := range anchors {
		anchorAtLine[a.Line] = a.EndpointID
	}

	for i, line := range lines {
		lineNumber := i + 1
		marker, ok := ParseMarker(line, file, lineNumber)
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
// blank lines and line comments, and reports whether the first substantive
// line found is a known endpoint anchor.
//
// Only `//` line comments are skipped, not `/* */` blocks — so a marker
// separated from its endpoint by a JSDoc/Javadoc block is treated as
// matching nothing and produces a stale-allow-marker finding. That is
// pre-existing NestJS behavior, preserved verbatim here rather than
// quietly changed while moving the code; it is a known shared gap, not a
// property of either language.
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
