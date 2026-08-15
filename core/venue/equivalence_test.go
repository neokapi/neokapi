package venue

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannelKey(t *testing.T) {
	tests := map[string]string{
		"web":            "web",
		"Web":            "web",
		"marketing-site": "marketingsite",
		"marketing_site": "marketingsite",
		"docs-v2":        "docsv2",
		"":               "",
		"---":            "",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, ChannelKey(in))
		})
	}
}

func TestProposeChannelAliases(t *testing.T) {
	tests := map[string]struct {
		proposed []string
		existing []string
		want     []ChannelAlias
	}{
		"a slug the workspace already holds is not fragmentation": {
			proposed: []string{"web"},
			existing: []string{"web", "docs"},
			want:     nil,
		},
		"separators and case": {
			proposed: []string{"marketing-site"},
			existing: []string{"marketingsite"},
			want: []ChannelAlias{
				{Proposed: "marketing-site", Existing: "marketingsite", Evidence: EvidenceSameNormalized},
			},
		},
		"a prefix inside one product": {
			proposed: []string{"website"},
			existing: []string{"web"},
			want: []ChannelAlias{
				{Proposed: "website", Existing: "web", Evidence: EvidencePrefix},
			},
		},
		"a prefix too short to say anything": {
			proposed: []string{"apps"},
			existing: []string{"ap"},
			want:     nil,
		},
		"unrelated slugs": {
			proposed: []string{"newsletter"},
			existing: []string{"web", "docs"},
			want:     nil,
		},
		"one slug can look like two": {
			proposed: []string{"docs"},
			existing: []string{"doc", "docsite"},
			want: []ChannelAlias{
				{Proposed: "docs", Existing: "doc", Evidence: EvidencePrefix},
				{Proposed: "docs", Existing: "docsite", Evidence: EvidencePrefix},
			},
		},
		"nothing to compare against": {
			proposed: []string{"web"},
			existing: nil,
			want:     nil,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.want, ProposeChannelAliases(tt.proposed, tt.existing))
		})
	}
}
