// Package allowlist implements the sphinxor-allow comment marker grammar
// accepted in docs/decisions/0003-allowlist-format.md.
//
// This package only parses marker comments in isolation. Matching a parsed
// Marker to the endpoint it's meant to exempt (and producing the "stale
// allow marker" finding when no such endpoint exists) requires the
// extracted model and belongs to the extraction/lint pipeline, not here.
package allowlist

import (
	"regexp"
	"strings"
)

// Grammar (single line, case-sensitive keyword):
//
//	// sphinxor-allow: <reason>
//
// <reason> is required, free text, trimmed of surrounding whitespace. A
// marker with no reason (e.g. "// sphinxor-allow:" alone) does not match —
// vision.md's rationale for this mechanism is an auditable, explicit
// opt-out, and an empty reason defeats that.
var markerPattern = regexp.MustCompile(`^//\s*sphinxor-allow:\s*(.+)$`)

// Marker is one parsed sphinxor-allow comment.
type Marker struct {
	Reason string
	File   string
	Line   int
}

// ParseMarker parses a single line of source as a sphinxor-allow marker.
// ok is false if the line is not a marker (including a marker with an
// empty reason).
func ParseMarker(line string, file string, lineNumber int) (marker Marker, ok bool) {
	trimmed := strings.TrimSpace(line)

	matches := markerPattern.FindStringSubmatch(trimmed)
	if matches == nil {
		return Marker{}, false
	}

	reason := strings.TrimSpace(matches[1])
	if reason == "" {
		return Marker{}, false
	}

	return Marker{Reason: reason, File: file, Line: lineNumber}, true
}
