package allowlist

import "testing"

func TestParseMarker(t *testing.T) {
	cases := []struct {
		name       string
		line       string
		wantOK     bool
		wantReason string
	}{
		{
			name:       "valid marker with reason",
			line:       "// sphinxor-allow: public — health check endpoint, no auth by design",
			wantOK:     true,
			wantReason: "public — health check endpoint, no auth by design",
		},
		{
			name:       "valid marker, no space after slashes",
			line:       "//sphinxor-allow: public",
			wantOK:     true,
			wantReason: "public",
		},
		{
			name:       "leading whitespace before comment",
			line:       "    // sphinxor-allow: public",
			wantOK:     true,
			wantReason: "public",
		},
		{
			name:   "not a marker at all",
			line:   "// this is a regular comment",
			wantOK: false,
		},
		{
			name:   "marker keyword with no reason",
			line:   "// sphinxor-allow:",
			wantOK: false,
		},
		{
			name:   "marker keyword with only whitespace as reason",
			line:   "// sphinxor-allow:    ",
			wantOK: false,
		},
		{
			name:   "wrong keyword",
			line:   "// sphinxor-ignore: public",
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			marker, ok := ParseMarker(tc.line, "example.ts", 42)

			if ok != tc.wantOK {
				t.Fatalf("ParseMarker(%q) ok = %v, want %v", tc.line, ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if marker.Reason != tc.wantReason {
				t.Errorf("ParseMarker(%q) reason = %q, want %q", tc.line, marker.Reason, tc.wantReason)
			}
			if marker.File != "example.ts" || marker.Line != 42 {
				t.Errorf("ParseMarker(%q) file/line = %q:%d, want example.ts:42", tc.line, marker.File, marker.Line)
			}
		})
	}
}
