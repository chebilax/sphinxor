package cli

import (
	"testing"

	"github.com/chebilax/sphynxor/internal/model"
)

func TestHasBlockingFindings(t *testing.T) {
	cases := []struct {
		name     string
		findings []model.Finding
		want     bool
	}{
		{
			name:     "no findings",
			findings: nil,
			want:     false,
		},
		{
			name: "high confidence, not allowlisted",
			findings: []model.Finding{
				{Confidence: model.ConfidenceHigh, Allowlisted: false},
			},
			want: true,
		},
		{
			name: "high confidence, allowlisted",
			findings: []model.Finding{
				{Confidence: model.ConfidenceHigh, Allowlisted: true},
			},
			want: false,
		},
		{
			name: "only low confidence",
			findings: []model.Finding{
				{Confidence: model.ConfidenceLow, Allowlisted: false},
			},
			want: false,
		},
		{
			name: "mixed, one blocking",
			findings: []model.Finding{
				{Confidence: model.ConfidenceLow, Allowlisted: false},
				{Confidence: model.ConfidenceHigh, Allowlisted: true},
				{Confidence: model.ConfidenceHigh, Allowlisted: false},
			},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasBlockingFindings(tc.findings); got != tc.want {
				t.Errorf("hasBlockingFindings(%+v) = %v, want %v", tc.findings, got, tc.want)
			}
		})
	}
}
